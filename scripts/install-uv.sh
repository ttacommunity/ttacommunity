#!/usr/bin/env bash
# Install a pinned uv release after checking it against its published sha256.
#
# Usage: scripts/install-uv.sh <version> <bin dir>
set -euo pipefail

version=${1:?usage: install-uv.sh <version> <bin dir>}
bindir=${2:?usage: install-uv.sh <version> <bin dir>}

libc=gnu
[ -f /etc/alpine-release ] && libc=musl
asset="uv-$(uname -m)-unknown-linux-${libc}.tar.gz"
base="https://github.com/astral-sh/uv/releases/download/${version}"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cd "$tmp"
curl -fsSLO "$base/$asset"
curl -fsSLO "$base/$asset.sha256"
sha256sum -c "$asset.sha256"
tar xzf "$asset"
install -m 0755 "${asset%.tar.gz}/uv" "${asset%.tar.gz}/uvx" "$bindir/"
"$bindir/uv" --version
