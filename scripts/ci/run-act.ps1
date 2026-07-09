[CmdletBinding()]
param(
    [ValidateSet("linux-amd64", "linux-arm64")]
    [string]$Job = "linux-arm64",

    [switch]$DryRun,

    [switch]$RebuildRunner,

    [string]$ProxyUrl = "",

    [switch]$CollectOnly
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..\..")
Set-Location $repoRoot

function Assert-NativeCommand {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Name
    )

    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw "Required command was not found in PATH: $Name"
    }
}

function Invoke-Native {
    param(
        [Parameter(Mandatory = $true)]
        [string]$FilePath,

        [Parameter(ValueFromRemainingArguments = $true)]
        [string[]]$Arguments
    )

    & $FilePath @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$FilePath failed with exit code $LASTEXITCODE"
    }
}

function Test-ContainerExecution {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Image,

        [Parameter(Mandatory = $true)]
        [string]$Platform,

        [Parameter(Mandatory = $true)]
        [string[]]$Command
    )

    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        $output = @(& docker run --rm --platform $Platform $Image @Command 2>&1)
        $exitCode = $LASTEXITCODE
    }
    finally {
        $ErrorActionPreference = $previousErrorActionPreference
    }

    return [PSCustomObject]@{
        ExitCode = $exitCode
        Output = ($output -join [Environment]::NewLine).Trim()
    }
}

Assert-NativeCommand "docker"
Assert-NativeCommand "act"

$dockerOS = (& docker info --format '{{.OSType}}').Trim()
if ($LASTEXITCODE -ne 0) {
    throw "Docker Desktop is not available"
}
if ($dockerOS -ne "linux") {
    throw "act packaging requires Docker Desktop to use Linux containers; current mode: $dockerOS"
}

$containerArchitecture = switch ($Job) {
    "linux-amd64" { "linux/amd64" }
    "linux-arm64" { "linux/arm64" }
}
$runnerImage = switch ($Job) {
    "linux-amd64" { "nexusbridge-act-runner:24.04-amd64" }
    "linux-arm64" { "nexusbridge-act-runner:24.04-arm64" }
}

if (-not $DryRun -and -not $CollectOnly) {
    if ($Job -eq "linux-arm64") {
        $arm64Probe = Test-ContainerExecution `
            -Image "ubuntu:24.04" `
            -Platform $containerArchitecture `
            -Command @("uname", "-m")
        if ($arm64Probe.ExitCode -ne 0 -or $arm64Probe.Output -notmatch "(^|\s)aarch64(\s|$)") {
            throw @"
Docker Desktop cannot currently execute ARM64 Linux containers.
Probe command: docker run --rm --platform linux/arm64 ubuntu:24.04 uname -m
Probe output: $($arm64Probe.Output)

Restart Docker Desktop, confirm it is using Linux containers, and run the probe command above again. The expected output is 'aarch64'. This is an ARM64 emulation problem, not a Go compilation error.
"@
        }
    }

    $runnerImageID = [string](& docker images --quiet $runnerImage)
    $runnerExists = -not [string]::IsNullOrWhiteSpace($runnerImageID)
    if ($RebuildRunner -or -not $runnerExists) {
        Write-Host "Building local act runner image '$runnerImage'."
        Invoke-Native docker build `
            --platform $containerArchitecture `
            --file "scripts/ci/act-runner.Dockerfile" `
            --tag $runnerImage `
            "scripts/ci"
    }

    $runnerProbe = Test-ContainerExecution `
        -Image $runnerImage `
        -Platform $containerArchitecture `
        -Command @("bash", "-lc", "uname -m && node -p process.arch")
    if ($runnerProbe.ExitCode -ne 0) {
        throw "The local act runner image '$runnerImage' cannot execute correctly. Run this command again with -RebuildRunner. Probe output: $($runnerProbe.Output)"
    }
}

$actArguments = @(
    "workflow_dispatch",
    "-W", ".github/workflows/build-release.yml",
    "-j", $Job,
    "--input", "target_job=$Job",
    "--input", "publish_release=false",
    "--container-architecture", $containerArchitecture
)
if ($DryRun) {
    $actArguments += "--dryrun"
}
if (-not [string]::IsNullOrWhiteSpace($ProxyUrl)) {
    $actArguments += @(
        "--env", "HTTP_PROXY=$ProxyUrl",
        "--env", "HTTPS_PROXY=$ProxyUrl",
        "--env", "http_proxy=$ProxyUrl",
        "--env", "https_proxy=$ProxyUrl",
        "--env", "NO_PROXY=localhost,127.0.0.1,host.docker.internal"
    )
}

if (-not $CollectOnly) {
    Write-Host "Running GitHub Actions job '$Job' with container architecture '$containerArchitecture'."
    Invoke-Native act @actArguments
}

if (-not $DryRun) {
    $runnerContainers = @(& docker ps --quiet --filter "ancestor=$runnerImage")
    if ($runnerContainers.Count -ne 1) {
        throw "Expected one reusable act runner container for '$runnerImage', found $($runnerContainers.Count)"
    }
    $runnerContainerID = $runnerContainers[0]
    $containerDetails = @(& docker inspect $runnerContainerID | ConvertFrom-Json)[0]
    $repoName = Split-Path $repoRoot -Leaf
    $workspaceMounts = @($containerDetails.Mounts | Where-Object {
        $_.Type -eq "volume" -and $_.Destination.TrimEnd("/").EndsWith("/$repoName")
    })
    if ($workspaceMounts.Count -ne 1) {
        throw "Unable to resolve the repository workspace mount from act runner container"
    }
    $containerWorkspace = $workspaceMounts[0].Destination
    New-Item -ItemType Directory -Force (Join-Path $repoRoot "dist") | Out-Null
    Invoke-Native docker cp "${runnerContainerID}:$containerWorkspace/dist/." (Join-Path $repoRoot "dist")
    Write-Host "Packages are available under dist/."
}
