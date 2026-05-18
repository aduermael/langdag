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

Then commit internal/models/catalog.json. Release tags need this committed file
because Go embeds catalog data from repository source.
EOF

exit 1
