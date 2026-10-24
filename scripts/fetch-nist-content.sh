#!/usr/bin/env bash
# Re-fetch the pinned NIST OSCAL content and schemas that oscalkit's tests use,
# verify every file against the committed SHA-256 sums, and store them in the
# layout the tests expect. Large catalogs are stored gzip-compressed; the sums
# are always of the *uncompressed* upstream bytes.
#
# Usage: scripts/fetch-nist-content.sh            # fetch + verify (fails on mismatch)
#        scripts/fetch-nist-content.sh --update   # fetch + rewrite the sum files
set -euo pipefail

CONTENT_TAG="v1.6.0"   # usnistgov/oscal-content
SCHEMA_TAG="v1.2.3"    # usnistgov/OSCAL release
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CONTENT_DIR="$ROOT/testdata/oscal-content/nist.gov/SP800-53/rev5/json"
SCHEMA_DIR="$ROOT/schema/oscal-$SCHEMA_TAG"
CONTENT_BASE="https://raw.githubusercontent.com/usnistgov/oscal-content/$CONTENT_TAG/nist.gov/SP800-53/rev5/json"
SCHEMA_BASE="https://github.com/usnistgov/OSCAL/releases/download/$SCHEMA_TAG"
UPDATE=0
[[ "${1:-}" == "--update" ]] && UPDATE=1

sha() { shasum -a 256 "$1" | awk '{print $1}'; }

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

mkdir -p "$CONTENT_DIR" "$SCHEMA_DIR"

# name|compress
content_files=(
  "NIST_SP-800-53_rev5_catalog.json|gz"
  "NIST_SP-800-53_rev5_LOW-baseline_profile.json|raw"
  "NIST_SP-800-53_rev5_MODERATE-baseline_profile.json|raw"
  "NIST_SP-800-53_rev5_HIGH-baseline_profile.json|raw"
  "NIST_SP-800-53_rev5_PRIVACY-baseline_profile.json|raw"
  "NIST_SP-800-53_rev5_LOW-baseline-resolved-profile_catalog.json|gz"
  "NIST_SP-800-53_rev5_MODERATE-baseline-resolved-profile_catalog.json|gz"
  "NIST_SP-800-53_rev5_HIGH-baseline-resolved-profile_catalog.json|gz"
  "NIST_SP-800-53_rev5_PRIVACY-baseline-resolved-profile_catalog.json|gz"
)
schema_files=(
  oscal_catalog_schema.json
  oscal_profile_schema.json
  oscal_component_schema.json
  oscal_ssp_schema.json
)

verify_or_record() { # sumfile name file
  local sumfile="$1" name="$2" file="$3" got want
  got="$(sha "$file")"
  if [[ $UPDATE -eq 1 ]]; then
    echo "$got  $name" >>"$sumfile.new"
    return
  fi
  want="$(awk -v n="$name" '$2==n {print $1}' "$sumfile")"
  if [[ "$got" != "$want" ]]; then
    echo "checksum mismatch for $name: got $got want ${want:-<none>}" >&2
    exit 1
  fi
}

rm -f "$CONTENT_DIR/SHA256SUMS.new" "$SCHEMA_DIR/SHA256SUMS.new"

for entry in "${content_files[@]}"; do
  name="${entry%%|*}"; mode="${entry##*|}"
  echo "fetch $name"
  curl -fsSL "$CONTENT_BASE/$name" -o "$work/$name"
  verify_or_record "$CONTENT_DIR/SHA256SUMS" "$name" "$work/$name"
  if [[ "$mode" == "gz" ]]; then
    gzip -9 -n -c "$work/$name" >"$CONTENT_DIR/$name.gz"
  else
    cp "$work/$name" "$CONTENT_DIR/$name"
  fi
done

for name in "${schema_files[@]}"; do
  echo "fetch $name"
  curl -fsSL "$SCHEMA_BASE/$name" -o "$work/$name"
  verify_or_record "$SCHEMA_DIR/SHA256SUMS" "$name" "$work/$name"
  cp "$work/$name" "$SCHEMA_DIR/$name"
done

if [[ $UPDATE -eq 1 ]]; then
  mv "$CONTENT_DIR/SHA256SUMS.new" "$CONTENT_DIR/SHA256SUMS"
  mv "$SCHEMA_DIR/SHA256SUMS.new" "$SCHEMA_DIR/SHA256SUMS"
  echo "sum files updated"
else
  echo "all files verified"
fi
