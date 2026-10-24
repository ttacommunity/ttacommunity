#!/usr/bin/env bash
# Install the mise release pinned here (linux/amd64) after checking its published sha256.
# mise then installs the toolchain that mise.toml pins. Every workflow installs mise this way.
#
# Usage: scripts/install-mise.sh <bin dir>
set -euo pipefail

version=v2026.9.14
bindir=${1:?usage: install-mise.sh <bin dir>}
asset="mise-${version}-linux-x64"
base="https://github.com/jdx/mise/releases/download/${version}"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cd "$tmp"
curl -fsSLO "$base/$asset"
curl -fsSLO "$base/SHASUMS256.txt"
grep -E " (\./)?${asset}\$" SHASUMS256.txt | sed 's| \./| |' | sha256sum -c -
install -m 0755 "$asset" "$bindir/mise"
"$bindir/mise" --version
