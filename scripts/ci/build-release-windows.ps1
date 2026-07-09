[CmdletBinding()]
param(
    [ValidateSet("amd64")]
    [string]$Arch = "amd64",

    [string]$ArtifactArch = "",

    [string]$WailsVersion = ""
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

if ([string]::IsNullOrWhiteSpace($WailsVersion)) {
    if ([string]::IsNullOrWhiteSpace($env:WAILS_VERSION)) {
        $WailsVersion = "v3.0.0-alpha2.117"
    } else {
        $WailsVersion = $env:WAILS_VERSION
    }
}

if ([string]::IsNullOrWhiteSpace($ArtifactArch)) {
    $ArtifactArch = $Arch
}

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..\..")
Set-Location $repoRoot

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

function Assert-File {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw "Expected file was not created: $Path"
    }
}

Invoke-Native go install "github.com/wailsapp/wails/v3/cmd/wails3@$WailsVersion"
Invoke-Native wails3 build "GOOS=windows" "ARCH=$Arch"
Invoke-Native wails3 task build:webui "GOOS=windows" "ARCH=$Arch"

Assert-File "bin\nexusbridge-webui.exe"
Assert-File "bin\nexusbridge-desktop.exe"
Assert-File "data\config.example.json"
Assert-File "nexusbridge.bootstrap.example.json"

$webuiDir = Join-Path $repoRoot "dist\webui"
$desktopDir = Join-Path $repoRoot "dist\desktop"
$webuiDataDir = Join-Path $webuiDir "data"
$desktopDataDir = Join-Path $desktopDir "data"

Remove-Item -LiteralPath $webuiDir, $desktopDir -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force $webuiDataDir, $desktopDataDir | Out-Null

Copy-Item -LiteralPath "bin\nexusbridge-webui.exe" -Destination $webuiDir
Copy-Item -LiteralPath "bin\nexusbridge-desktop.exe" -Destination $desktopDir
Copy-Item -LiteralPath "data\config.example.json" -Destination $webuiDataDir
Copy-Item -LiteralPath "data\config.example.json" -Destination $desktopDataDir
Copy-Item -LiteralPath "nexusbridge.bootstrap.example.json" -Destination $webuiDir
Copy-Item -LiteralPath "nexusbridge.bootstrap.example.json" -Destination $desktopDir

$webuiZip = Join-Path $repoRoot "dist\nexusbridge-webui-windows-$ArtifactArch.zip"
$desktopZip = Join-Path $repoRoot "dist\nexusbridge-desktop-windows-$ArtifactArch.zip"

Remove-Item -LiteralPath $webuiZip, $desktopZip -Force -ErrorAction SilentlyContinue
Compress-Archive -Path (Join-Path $webuiDir "*") -DestinationPath $webuiZip -Force
Compress-Archive -Path (Join-Path $desktopDir "*") -DestinationPath $desktopZip -Force
