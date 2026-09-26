#!/usr/bin/env bash
# Run every test natively: Go (unit and integration), the engine, and web.
# It is the test half of scripts/ci.sh, so the two cannot disagree.
exec "$(dirname "$0")/ci.sh" --tests-only "$@"
