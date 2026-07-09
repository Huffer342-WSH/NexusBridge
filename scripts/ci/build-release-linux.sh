#!/usr/bin/env bash
set -euo pipefail

readonly default_wails_version="v3.0.0-alpha2.117"

arch="${ARCH:-amd64}"
artifact_arch="${ARTIFACT_ARCH:-}"
wails_version="${WAILS_VERSION:-$default_wails_version}"
install_system_deps=0

usage() {
  cat <<'EOF'
Usage: scripts/ci/build-release-linux.sh [options]

Options:
  --arch <amd64|arm64>          Go target architecture. Defaults to ARCH or amd64.
  --artifact-arch <name>        Architecture label used in artifact names.
                                Defaults to amd64 for amd64, aarch64 for arm64.
  --install-system-deps         Install GTK/WebKit build dependencies through apt-get.
  --wails-version <version>     Wails CLI version. Defaults to WAILS_VERSION or v3.0.0-alpha2.117.
  -h, --help                    Show this help text.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --arch)
      arch="${2:?missing value for --arch}"
      shift 2
      ;;
    --artifact-arch)
      artifact_arch="${2:?missing value for --artifact-arch}"
      shift 2
      ;;
    --install-system-deps)
      install_system_deps=1
      shift
      ;;
    --wails-version)
      wails_version="${2:?missing value for --wails-version}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

case "$arch" in
  amd64)
    artifact_arch="${artifact_arch:-amd64}"
    ;;
  arm64)
    artifact_arch="${artifact_arch:-aarch64}"
    ;;
  *)
    echo "Unsupported Linux arch: $arch" >&2
    exit 2
    ;;
esac

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "$script_dir/../.." && pwd)"
cd "$repo_root"

if [[ "$install_system_deps" == "1" ]]; then
  if ! command -v apt-get >/dev/null 2>&1; then
    echo "--install-system-deps requires apt-get" >&2
    exit 2
  fi
  # Ubuntu 容器默认在每次 apt 调用后清空下载缓存。保留缓存可让下面的
  # 有限重试只补拉失败的包，避免单个镜像站错误导致重新下载全部桌面依赖。
  sudo rm -f /etc/apt/apt.conf.d/docker-clean
  sudo apt-get -o Acquire::Retries=5 update
  for attempt in 1 2 3; do
    if sudo apt-get -o Acquire::Retries=5 install -y --fix-missing \
      build-essential pkg-config libgtk-4-dev libwebkitgtk-6.0-dev; then
      break
    fi
    if [[ "$attempt" == "3" ]]; then
      echo "Unable to install Linux build dependencies after $attempt attempts" >&2
      exit 1
    fi
    echo "apt-get install failed; retrying dependency installation ($((attempt + 1))/3)" >&2
    sleep $((attempt * 5))
  done
fi

go install "github.com/wailsapp/wails/v3/cmd/wails3@$wails_version"
wails3 build GOOS=linux ARCH="$arch"
wails3 task build:webui GOOS=linux ARCH="$arch"

require_file() {
  local path="$1"
  if [[ ! -f "$path" ]]; then
    echo "Expected file was not created: $path" >&2
    exit 1
  fi
}

require_file "bin/nexusbridge-webui"
require_file "bin/nexusbridge-desktop"
require_file "data/config.example.json"
require_file "nexusbridge.bootstrap.example.json"

rm -rf "$repo_root/dist/webui" "$repo_root/dist/desktop"
mkdir -p "$repo_root/dist/webui/data" "$repo_root/dist/desktop/data"

cp "bin/nexusbridge-webui" "$repo_root/dist/webui/"
cp "bin/nexusbridge-desktop" "$repo_root/dist/desktop/"
cp "data/config.example.json" "$repo_root/dist/webui/data/"
cp "data/config.example.json" "$repo_root/dist/desktop/data/"
cp "nexusbridge.bootstrap.example.json" "$repo_root/dist/webui/"
cp "nexusbridge.bootstrap.example.json" "$repo_root/dist/desktop/"

tar -C "$repo_root/dist/webui" -czf "$repo_root/dist/nexusbridge-webui-linux-$artifact_arch.tar.gz" .
tar -C "$repo_root/dist/desktop" -czf "$repo_root/dist/nexusbridge-desktop-linux-$artifact_arch.tar.gz" .
