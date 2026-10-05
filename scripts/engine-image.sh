#!/usr/bin/env bash
# Build the engine image without Docker or privileges, and push it.
# Wheels are downloaded for linux/amd64, layered onto the pinned Python base
# with crane, and configured to match engine/Dockerfile. Prints the digest.
#
# Usage: scripts/engine-image.sh <image ref, e.g. ghcr.io/org/repo/engine:tag>
set -euo pipefail

ref=${1:?usage: engine-image.sh <image ref>}
BASE=docker.io/library/python:3.12-slim@sha256:f77ac9e44ae96ef2c90b8053ea08c31f8be030f824196b0ae4db6d462c84e51f

cd "$(dirname "$0")/../engine"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/root/engine/deps"

uv export --frozen --no-dev --no-hashes --no-emit-project --quiet -o "$work/requirements.txt"
uv pip install --quiet --target "$work/root/engine/deps" --python-version 3.12 \
  --python-platform x86_64-manylinux_2_28 --only-binary :all: -r "$work/requirements.txt"
git ls-files -co --exclude-standard -z . | xargs -0 -I{} cp --parents {} "$work/root/engine/"

tar -C "$work/root" --sort=name --owner=0 --group=0 --numeric-owner --mtime=@0 \
  -cf "$work/layer.tar" engine
crane append --platform linux/amd64 -b "$BASE" -f "$work/layer.tar" -t "$ref.tmp" >/dev/null
crane mutate "$ref.tmp" -t "$ref" \
  --entrypoint python --cmd=-c,"import numpy; print('engine image ok')" \
  --env PYTHONPATH=/engine/deps:/engine/src --env PYTHONDONTWRITEBYTECODE=1 \
  --user 65532:65532 --workdir /engine >/dev/null
crane digest "$ref"
