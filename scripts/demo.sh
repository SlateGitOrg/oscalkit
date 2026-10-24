#!/usr/bin/env bash
# Offline end-to-end demo of oscalkit against bundled fixtures: official NIST
# SP 800-53 rev5 content (testdata/oscal-content) and a fictional "Acme"
# tailored profile plus component definitions (examples/).
set -euo pipefail
cd "$(dirname "$0")/.."

BIN=./bin/oscalkit
OUT=out
NIST=testdata/oscal-content/nist.gov/SP800-53/rev5/json
PROFILE=examples/profiles/acme-low-tailored_profile.json
COMPONENTS=$(ls examples/components/*.json | paste -sd, -)
mkdir -p "$OUT"

if [ -t 1 ]; then B1=$'\033[1m'; B0=$'\033[0m'; else B1=""; B0=""; fi
step() { printf '\n%s== %s%s\n$ %s\n' "$B1" "$1" "$B0" "$2"; }

step "1. Resolve the official NIST MODERATE baseline and compare it with NIST's own resolution" \
     "oscalkit resolve NIST_SP-800-53_rev5_MODERATE-baseline_profile.json && oscalkit compare ..."
$BIN resolve "$NIST/NIST_SP-800-53_rev5_MODERATE-baseline_profile.json" --seed demo -o "$OUT/moderate-resolved.json"
$BIN compare --fail-on-diff "$OUT/moderate-resolved.json" "$NIST/NIST_SP-800-53_rev5_MODERATE-baseline-resolved-profile_catalog.json"

step "2. Validate the Acme tailored profile and component definitions (schema + semantics)" \
     "oscalkit validate $PROFILE examples/components/*.json"
$BIN validate "$PROFILE" examples/components/*.json

step "3. Resolve the tailored profile (imports the NIST LOW profile, excludes PE-*, adds 4 MODERATE controls, sets params, alters AC-2/AC-11)" \
     "oscalkit resolve $PROFILE -o $OUT/acme-baseline.json"
$BIN resolve "$PROFILE" --seed demo -o "$OUT/acme-baseline.json"
$BIN validate "$OUT/acme-baseline.json"

step "4. What did tailoring change relative to the NIST LOW baseline?" \
     "oscalkit compare $OUT/acme-baseline.json NIST_SP-800-53_rev5_LOW-baseline-resolved-profile_catalog.json"
$BIN compare "$OUT/acme-baseline.json" "$NIST/NIST_SP-800-53_rev5_LOW-baseline-resolved-profile_catalog.json"

step "5. Generate an SSP skeleton from the four component definitions" \
     "oscalkit generate --profile $PROFILE --component <4 files> -o $OUT/acme-ssp.json"
$BIN generate --profile "$PROFILE" --component "$COMPONENTS" \
  --system-name "Acme Customer Portal" --impact low --seed demo -o "$OUT/acme-ssp.json"
$BIN validate "$OUT/acme-ssp.json"

step "6. Gap analysis of the generated SSP against its resolved baseline" \
     "oscalkit gaps $OUT/acme-ssp.json --max 12"
$BIN gaps "$OUT/acme-ssp.json" --max 12 --fail-on never
$BIN gaps "$OUT/acme-ssp.json" --format markdown --max 0 --fail-on never > "$OUT/acme-gaps.md"
$BIN gaps "$OUT/acme-ssp.json" --format json --fail-on never > "$OUT/acme-gaps.json"
echo "(full reports written to $OUT/acme-gaps.md and $OUT/acme-gaps.json)"

step "7. Parameter substitution: AC-7 and AC-11 rendered with the SSP's values" \
     "oscalkit render --baseline $PROFILE --ssp $OUT/acme-ssp.json ac-7 ac-11"
$BIN render --baseline "$PROFILE" --ssp "$OUT/acme-ssp.json" ac-7 ac-11

step "8. A hand-edited draft SSP with deliberate mistakes: validation and gaps catch them (non-zero exits are expected)" \
     "oscalkit validate examples/ssp/acme-ssp-draft.json; oscalkit gaps examples/ssp/acme-ssp-draft.json --max 8"
$BIN validate examples/ssp/acme-ssp-draft.json || echo "(validate exit code $? = findings)"
$BIN gaps examples/ssp/acme-ssp-draft.json --max 8 || echo "(gaps exit code $? = gaps at or above --fail-on error)"

printf '\ndemo complete: outputs in %s/\n' "$OUT"
