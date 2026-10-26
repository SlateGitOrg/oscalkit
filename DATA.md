# Data

Every external file used by oscalkit's tests and demo is pinned to an upstream release. Each one
is verified against a committed SHA-256 sum of the *uncompressed* upstream bytes
(`oscal/content_test.go` runs during `make check`). `scripts/fetch-nist-content.sh` re-downloads
the files and verifies them against the same sums.

## NIST SP 800-53 rev5 OSCAL content

- **Source:** https://github.com/usnistgov/oscal-content, tag `v1.6.0`, path
  `nist.gov/SP800-53/rev5/json/`. This is SP 800-53 Release 5.2.0, expressed in OSCAL 1.2.3.
- **Files:** `NIST_SP-800-53_rev5_catalog.json`, the LOW/MODERATE/HIGH/PRIVACY `-baseline_profile.json`
  files and their `-baseline-resolved-profile_catalog.json` counterparts (9 files).
- **Stored at:** `testdata/oscal-content/nist.gov/SP800-53/rev5/json/`, mirroring the upstream
  layout so that the profiles' relative back-matter links (`../../../../nist.gov/...`) resolve
  unchanged. The catalog and the resolved catalogs (about 11 MB of JSON) are stored gzip-compressed
  (`gzip -9 -n`). The profiles are stored as-is. Total on disk: 1.8 MB.
- **Checksums:** `testdata/oscal-content/nist.gov/SP800-53/rev5/json/SHA256SUMS`.
- **License / terms:** public domain. Per the repository's LICENSE.md, the content is a work of
  the U.S. government, public domain in the United States, and dedicated worldwide under CC0 1.0
  Universal. Redistribution here is unmodified apart from compression.
- **Use:** the four resolved catalogs are the *expected output* for the conformance test
  (`resolve/nist_test.go`). They are never used to tune the resolver's behaviour beyond the one
  documented decision about ancestor inclusion (DECISIONS.md).

## OSCAL JSON schemas

- **Source:** https://github.com/usnistgov/OSCAL/releases/tag/v1.2.3 (release assets
  `oscal_{catalog,profile,component,ssp}_schema.json`).
- **Stored at:** `schema/oscal-v1.2.3/`, embedded into the binary with `go:embed`. Checksums are
  in `schema/oscal-v1.2.3/SHA256SUMS`.
- **License / terms:** worldwide public domain. The usnistgov/OSCAL LICENSE.md places the
  project in the public domain as a U.S. government work.
- **Note:** the profile schema has a defect: `merge.combine.method` is missing. It is repaired in
  memory at compile time, and the file on disk stays byte-identical (see DECISIONS.md).

## Synthetic example content (this repository)

- `examples/profiles/acme-low-tailored_profile.json`, `examples/components/*.json` and
  `examples/ssp/acme-ssp-draft.json` describe a **fictional** organization ("Acme") and system.
  They were written for this repository to exercise tailoring, SSP generation and gap analysis.
  They do not describe any real system or vendor product. MIT licensed with the code.
- Test fixtures inside `*_test.go` files (such as the "abc" catalog) are synthetic. The abc
  catalog mirrors the shape of the examples in the NIST profile-resolution specification.
