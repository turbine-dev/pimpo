#!/bin/sh
# Install Pimpo on Linux or macOS: curl -fsSL https://raw.githubusercontent.com/turbine-dev/pimpo/main/scripts/install.sh | sh
# Options: --service (run at boot with systemd, Linux), --version vX.Y.Z, --dir DIR
set -eu

repo="turbine-dev/pimpo"
# Release public keys (base64 Ed25519, the same list as releaseKeys in
# cmd/pimpo/release.go); more than one while a key is being rotated.
release_keys="Axmu5Iq5+WmaiZin5Dr77QoOGVFD0No5cu2sBGUNBGQ="
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
base="${PIMPO_BASE_URL:-$base}"
# A mirror must be https: the checksums come from the same place as the archive.
case "$base" in
  https://*) ;;
  *) echo "PIMPO_BASE_URL must start with https://, got $base" >&2; exit 1 ;;
esac
archive="pimpo_${os}_${arch}.tar.gz"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading $archive…"
curl --proto '=https' --proto-redir '=https' -fsSL "$base/$archive" -o "$tmp/$archive"
curl --proto '=https' --proto-redir '=https' -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"

# checksums.txt comes from the same place as the archive, so it is trusted
# only when signed with a release key. That needs OpenSSL 3 (or anything
# else that verifies Ed25519); without it the checksum still catches a
# broken download, not a changed release.
can_verify() {
  command -v openssl >/dev/null 2>&1 || return 1
  # RFC 8032, test 2: a known good signature of the byte "r".
  printf -- '-----BEGIN PUBLIC KEY-----\nMCowBQYDK2VwAyEAPUAXw+hDiVqStwqnTRt+vJyYLM8uxJaMwM1V8Sr0Zgw=\n-----END PUBLIC KEY-----\n' > "$tmp/probe.pem"
  printf r > "$tmp/probe.txt"
  printf %s 'kqAJqfDUyrhyDoILX2QlQKKye1QWUD+Ps3YiI+vbadoIWsHkPhWZbkWPNhPQ8R2MOHsurrQwKu6wDSkWErsMAA==' | openssl base64 -d -A -out "$tmp/probe.sig" 2>/dev/null || return 1
  openssl pkeyutl -verify -pubin -inkey "$tmp/probe.pem" -rawin -in "$tmp/probe.txt" -sigfile "$tmp/probe.sig" >/dev/null 2>&1
}
if can_verify; then
  if ! curl --proto '=https' --proto-redir '=https' -fsSL "$base/checksums.txt.sig" -o "$tmp/checksums.txt.sig"; then
    echo "this release has no signature for its checksums; not installing" >&2
    exit 1
  fi
  tr -d ' \r\n' < "$tmp/checksums.txt.sig" | openssl base64 -d -A -out "$tmp/checksums.sig.bin" 2>/dev/null || true
  signed=0
  for key in $release_keys; do
    # An Ed25519 SubjectPublicKeyInfo is a fixed 12-byte DER prefix
    # (302a300506032b6570032100, "MCowBQYDK2VwAyEA" in base64) and the key.
    printf -- '-----BEGIN PUBLIC KEY-----\nMCowBQYDK2VwAyEA%s\n-----END PUBLIC KEY-----\n' "$key" > "$tmp/release.pem"
    if openssl pkeyutl -verify -pubin -inkey "$tmp/release.pem" -rawin -in "$tmp/checksums.txt" -sigfile "$tmp/checksums.sig.bin" >/dev/null 2>&1; then
      signed=1
      break
    fi
  done
  if [ "$signed" != 1 ]; then
    echo "the signature of the checksums does not match Pimpo's release key; not installing" >&2
    exit 1
  fi
  echo "Signature verified."
else
  echo "warning: this openssl cannot verify Ed25519 signatures (OpenSSL 3 can); checking the checksum only" >&2
fi
expected=$(grep " $archive\$" "$tmp/checksums.txt" | cut -d' ' -f1)
if command -v sha256sum >/dev/null; then actual=$(sha256sum "$tmp/$archive" | cut -d' ' -f1); else actual=$(shasum -a 256 "$tmp/$archive" | cut -d' ' -f1); fi
if [ -z "$expected" ] || [ "$expected" != "$actual" ]; then
  echo "checksum mismatch for $archive; not installing" >&2
  exit 1
fi

tar -xzf "$tmp/$archive" -C "$tmp"
mkdir -p "$dir"
install -m 0755 "$tmp/pimpo" "$dir/pimpo"
echo "Installed $("$dir/pimpo" version) to $dir/pimpo"
case ":$PATH:" in *":$dir:"*) ;; *) echo "Add $dir to your PATH." ;; esac

if [ "$service" = 1 ]; then
  if [ "$os" != linux ] || ! command -v systemctl >/dev/null; then
    echo "--service needs systemd; on macOS use the desktop app, which starts at login" >&2
    exit 1
  fi
  unit="$HOME/.config/systemd/user/pimpo.service"
  mkdir -p "$(dirname "$unit")"
  cat > "$unit" <<UNIT
[Unit]
Description=Pimpo personal agent
After=network-online.target

[Service]
ExecStart=$dir/pimpo serve
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
UNIT
  systemctl --user daemon-reload
  systemctl --user enable --now pimpo
  loginctl enable-linger "$(id -un)" 2>/dev/null || true
  echo "Pimpo runs at boot. Login link: pimpo token"
else
  echo "Start it with: pimpo serve"
fi
