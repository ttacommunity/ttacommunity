#!/usr/bin/env bash
# Install a pinned crane release (linux/amd64) after checking its published sha256.
#
# Usage: scripts/install-crane.sh <version, e.g. v0.22.1> <bin dir>
set -euo pipefail

version=${1:?usage: install-crane.sh <version> <bin dir>}
bindir=${2:?usage: install-crane.sh <version> <bin dir>}
asset=go-containerregistry_Linux_x86_64.tar.gz
base="https://github.com/google/go-containerregistry/releases/download/${version}"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cd "$tmp"
curl -fsSLO "$base/$asset"
curl -fsSLO "$base/checksums.txt"
grep " ${asset}\$" checksums.txt | sha256sum -c -
tar xzf "$asset" crane
install -m 0755 crane "$bindir/crane"
"$bindir/crane" version
