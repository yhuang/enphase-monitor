#!/usr/bin/env bash
# migrate-history-layout.sh renames the legacy single-dataset history layout to
# the dataset-prefixed layout introduced when PG&E records joined Enphase records
# in the same directory:
#
#   history/<YYYY-MM-DD>.json  ->  history/enphase-<YYYY-MM-DD>.json
#   history/.index.json        ->  history/.enphase-index.json
#
# Idempotent: re-running after a partial/complete migration is a no-op. Files
# already carrying a dataset prefix (enphase-*, pge-*) are left untouched.
set -euo pipefail

dir="${1:-history}"

if [[ ! -d "$dir" ]]; then
  echo "no history directory at '$dir' — nothing to migrate"
  exit 0
fi

renamed=0

# Legacy day records: bare YYYY-MM-DD.json with no dataset prefix.
shopt -s nullglob
for f in "$dir"/[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9].json; do
  base="$(basename "$f")"
  dest="$dir/enphase-$base"
  if [[ -e "$dest" ]]; then
    echo "skip: $dest already exists"
    continue
  fi
  mv "$f" "$dest"
  renamed=$((renamed + 1))
done

# Legacy manifest.
if [[ -f "$dir/.index.json" && ! -e "$dir/.enphase-index.json" ]]; then
  mv "$dir/.index.json" "$dir/.enphase-index.json"
  renamed=$((renamed + 1))
fi

echo "migration complete: $renamed file(s) renamed under '$dir'"
