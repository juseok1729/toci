#!/bin/sh
# Installs the latest (or $TOCI_VERSION) toci release for this OS/arch.
#
#   curl -fsSL https://raw.githubusercontent.com/juseok1729/toci/main/install.sh | sh
#
# Static Go binary (CGO_ENABLED=0) — no glibc dependency, so this works the
# same on Oracle Linux, Ubuntu, other glibc/musl distros, and WSL.
set -eu

repo="juseok1729/toci"
version="${TOCI_VERSION:-latest}"

os="$(uname -s)"
case "$os" in
	Linux) os=linux ;;
	Darwin) os=darwin ;;
	*)
		echo "toci: unsupported OS '$os' — see https://github.com/$repo/releases" >&2
		exit 1
		;;
esac

arch="$(uname -m)"
case "$arch" in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	*)
		echo "toci: unsupported architecture '$arch' — see https://github.com/$repo/releases" >&2
		exit 1
		;;
esac

if [ "$version" = "latest" ]; then
	base_url="https://github.com/$repo/releases/latest/download"
else
	base_url="https://github.com/$repo/releases/download/$version"
fi

archive="toci_${os}_${arch}.tar.gz"
work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

echo "toci: downloading $archive ($version)..."
curl -fsSL "$base_url/$archive" -o "$work_dir/$archive"
curl -fsSL "$base_url/checksums.txt" -o "$work_dir/checksums.txt"

echo "toci: verifying checksum..."
(
	cd "$work_dir"
	if command -v sha256sum >/dev/null 2>&1; then
		grep " $archive\$" checksums.txt | sha256sum -c -
	elif command -v shasum >/dev/null 2>&1; then
		grep " $archive\$" checksums.txt | shasum -a 256 -c -
	else
		echo "toci: no sha256sum/shasum found, skipping checksum verification" >&2
	fi
)

tar -xzf "$work_dir/$archive" -C "$work_dir" toci

# Prefer /usr/local/bin (root, or already writable by this user); fall back
# to ~/.local/bin, the standard no-root user-local bin directory, otherwise.
install_dir="/usr/local/bin"
if [ "$(id -u)" != "0" ] && [ ! -w "$install_dir" ]; then
	install_dir="$HOME/.local/bin"
	mkdir -p "$install_dir"
fi

mv "$work_dir/toci" "$install_dir/toci"
chmod +x "$install_dir/toci"

echo "toci: installed to $install_dir/toci"

case ":$PATH:" in
	*":$install_dir:"*) ;;
	*) echo "toci: add $install_dir to your PATH to run 'toci' directly" ;;
esac
