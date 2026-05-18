#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
catalog_ref="${LANGDAG_MODEL_CATALOG_REF:-origin/model-catalog}"
catalog_source_path="${LANGDAG_MODEL_CATALOG_PATH:-docs/model-catalog/v1/catalog.json}"
catalog_dest="$repo_root/internal/models/catalog.json"
tmp_dest="${catalog_dest}.tmp"

cd "$repo_root"

if [[ "${LANGDAG_MODEL_CATALOG_FETCH:-1}" != "0" ]]; then
	if git fetch --depth=1 origin model-catalog; then
		catalog_ref="FETCH_HEAD"
	elif ! git rev-parse --verify --quiet "${catalog_ref}^{commit}" >/dev/null; then
		cat >&2 <<EOF
Could not fetch origin/model-catalog and no local $catalog_ref ref exists.
Cannot sync $catalog_source_path into internal/models/catalog.json.
EOF
		exit 1
	else
		echo "warning: using existing local $catalog_ref; fetch failed" >&2
	fi
fi

mkdir -p "$(dirname "$catalog_dest")"
if ! git show "${catalog_ref}:${catalog_source_path}" > "$tmp_dest"; then
	rm -f "$tmp_dest"
	cat >&2 <<EOF
Could not read ${catalog_source_path} from ${catalog_ref}.
Run the model-catalog publishing workflow first, then retry.
EOF
	exit 1
fi

if ! grep -q '"schema_version"[[:space:]]*:[[:space:]]*"model-catalog/v1"' "$tmp_dest"; then
	rm -f "$tmp_dest"
	echo "Fetched catalog is not a model-catalog/v1 artifact" >&2
	exit 1
fi

mv "$tmp_dest" "$catalog_dest"
echo "Synced internal/models/catalog.json from ${catalog_ref}:${catalog_source_path}"
echo "Commit internal/models/catalog.json if you want releases to embed this catalog snapshot."
