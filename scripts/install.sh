#!/bin/sh
# Install Vigia on Linux or macOS: curl -fsSL https://raw.githubusercontent.com/denerFernandes/vigia/main/scripts/install.sh | sh
# Options: --service (run at boot with systemd, Linux), --version vX.Y.Z, --dir DIR
set -eu

repo="denerFernandes/vigia"
version="latest"
dir="${HOME}/.local/bin"
service=0
while [ $# -gt 0 ]; do
  case "$1" in
    --service) service=1 ;;
    --version) version="$2"; shift ;;
    --dir) dir="$2"; shift ;;
    *) echo "unknown option $1" >&2; exit 2 ;;
  esac
  shift
done

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  armv7l|armv7) arch=armv7 ;;
  *) echo "unsupported CPU $(uname -m)" >&2; exit 1 ;;
esac
case "$os" in linux|darwin) ;; *) echo "unsupported system $os; on Windows use the installer from the releases page" >&2; exit 1 ;; esac

if [ "$version" = "latest" ]; then
  base="https://github.com/$repo/releases/latest/download"
else
  base="https://github.com/$repo/releases/download/$version"
fi
base="${VIGIA_BASE_URL:-$base}"
archive="vigia_${os}_${arch}.tar.gz"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading $archive…"
curl -fsSL "$base/$archive" -o "$tmp/$archive"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"
expected=$(grep " $archive\$" "$tmp/checksums.txt" | cut -d' ' -f1)
if command -v sha256sum >/dev/null; then actual=$(sha256sum "$tmp/$archive" | cut -d' ' -f1); else actual=$(shasum -a 256 "$tmp/$archive" | cut -d' ' -f1); fi
if [ -z "$expected" ] || [ "$expected" != "$actual" ]; then
  echo "checksum mismatch for $archive; not installing" >&2
  exit 1
fi

tar -xzf "$tmp/$archive" -C "$tmp"
mkdir -p "$dir"
install -m 0755 "$tmp/vigia" "$dir/vigia"
echo "Installed $("$dir/vigia" version) to $dir/vigia"
case ":$PATH:" in *":$dir:"*) ;; *) echo "Add $dir to your PATH." ;; esac

if [ "$service" = 1 ]; then
  if [ "$os" != linux ] || ! command -v systemctl >/dev/null; then
    echo "--service needs systemd; on macOS use the desktop app, which starts at login" >&2
    exit 1
  fi
  unit="$HOME/.config/systemd/user/vigia.service"
  mkdir -p "$(dirname "$unit")"
  cat > "$unit" <<UNIT
[Unit]
Description=Vigia personal agent
After=network-online.target

[Service]
ExecStart=$dir/vigia serve
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
UNIT
  systemctl --user daemon-reload
  systemctl --user enable --now vigia
  loginctl enable-linger "$(id -un)" 2>/dev/null || true
  echo "Vigia runs at boot. Open link: journalctl --user -u vigia | grep auth"
else
  echo "Start it with: vigia serve"
fi
