# Threat model

oscalkit is a defensive compliance tool. It reads OSCAL documents and writes resolved catalogs,
SSPs and reports. It does not talk to any system under assessment. This document covers the risks
of running it on documents you did not write, for example a profile from a vendor or a pull
request.

## Assets

- The machine running oscalkit (developer laptop or CI runner): its files and network position.
- The integrity of compliance artifacts. A wrong resolution or a missed gap could lead someone to
  certify an incomplete SSP.

## Untrusted inputs

Profiles, catalogs, component definitions and SSPs, together with every document they import.

| Threat | Mitigation |
|---|---|
| A profile imports `https://internal-host/...` (SSRF), or uses imports to fetch or beacon remote content | Remote fetching is **off by default**. `http(s)` imports fail with `ErrRemoteDisabled` unless `--allow-remote` is given. Only the `file`, `http` and `https` schemes are accepted at all. |
| A profile imports a local file outside the project (`../../../etc/...`) | Imports resolve relative to the profile, as the specification requires. The file must parse as an OSCAL catalog or profile, or resolution stops. Its content is not echoed: errors name the path, not the data. Residual risk: an OSCAL file elsewhere on disk could be pulled into a resolution, so run on untrusted profiles in a sandbox or CI container. |
| Decompression bomb (`.gz`) or a very large document | Reads are capped at 256 MiB, both before and after gzip decompression (`oscal.MaxDocumentBytes`). |
| YAML alias bomb ("billion laughs") | YAML expansion is budgeted at 20 M nodes (`TestYAMLAliasBomb`). |
| Import cycles or very deep nesting (denial of service) | Cycle detection on the import stack, plus a 32-level depth limit (`MaxDepth`). |
| Self-referential parameter inserts | Parameter substitution stops at depth 8 (`[...]`). |
| Pathological glob patterns | The matcher is iterative with an O(|p|·|s|) worst case, with no recursion or regexp backtracking explosion. It is fuzzed against a regexp oracle. |
| Malformed input that crashes the parser | Decoder and renderer fuzz targets (`FuzzDecode`, `FuzzRender`). Errors are returned, never panics. |
| Tampered reference content making conformance tests pass falsely | Pinned NIST files are verified against SHA-256 sums of the upstream bytes on every `make check` (`TestPinnedContentChecksums`). |
| Output that leaks local paths | The resolved catalog's `source-profile` records only the profile's file name. A generated SSP's `import-profile` is relative to the output file. |

## Out of scope

- Authenticity of OSCAL content. oscalkit does not verify signatures. rlink `hashes` are not
  checked yet; see the roadmap.
- Truthfulness of SSP claims. Gap analysis is structural; see the README's Limitations section.
- Running untrusted documents with `--allow-remote` on a privileged network.
