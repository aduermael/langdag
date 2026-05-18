#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
catalog_path="$repo_root/internal/models/catalog.json"

if [[ -s "$catalog_path" ]] && grep -q '"schema_version"[[:space:]]*:[[:space:]]*"model-catalog/v1"' "$catalog_path"; then
	exit 0
fi

cat >&2 <<'EOF'
Missing embedded model catalog at internal/models/catalog.json.

Run:
  ./scripts/sync-model-catalog.sh

This file is generated from origin/model-catalog and is intentionally ignored
so main does not become a second catalog source of truth.
EOF

exit 1
