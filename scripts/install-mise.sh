#!/usr/bin/env bash
# Install a pinned mise release (linux/amd64) after checking its published sha256.
# mise then installs the toolchain that mise.toml pins.
#
# Usage: scripts/install-mise.sh <version, e.g. v2026.9.14> <bin dir>
set -euo pipefail

version=${1:?usage: install-mise.sh <version> <bin dir>}
bindir=${2:?usage: install-mise.sh <version> <bin dir>}
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
