package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	exProfile = "../../examples/profiles/acme-low-tailored_profile.json"
	exComps   = "../../examples/components"
	nistDir   = "../../testdata/oscal-content/nist.gov/SP800-53/rev5/json"
)

func runCLI(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(context.Background(), args, &out, &errb, func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) })
	return out.String(), errb.String(), code
}

func components(t *testing.T) string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(exComps, "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatal("no example components")
	}
	return strings.Join(files, ",")
}

func TestHelpVersionAndUnknown(t *testing.T) {
	if out, _, code := runCLI(t, "help"); code != ExitOK || !strings.Contains(out, "Commands:") {
		t.Fatalf("help: %d %s", code, out)
	}
	if _, _, code := runCLI(t); code != ExitUsage {
		t.Fatal("no args is a usage error")
	}
	if out, _, code := runCLI(t, "version"); code != ExitOK || !strings.Contains(out, "oscalkit "+Version) {
		t.Fatal("version")
	}
	if _, errOut, code := runCLI(t, "frobnicate"); code != ExitUsage || !strings.Contains(errOut, "unknown command") {
		t.Fatal("unknown command")
	}
	if _, _, code := runCLI(t, "resolve", "-h"); code != ExitOK {
		t.Fatal("-h exits 0")
	}
	if _, _, code := runCLI(t, "resolve", "--bogus"); code != ExitUsage {
		t.Fatal("bad flag")
	}
	for _, cmd := range []string{"resolve", "validate", "gaps", "generate", "render", "compare"} {
		if _, _, code := runCLI(t, cmd); code != ExitUsage {
			t.Errorf("%s without args must be a usage error", cmd)
		}
	}
	if _, errOut, _ := runCLI(t, "-v", "version"); errOut != "" {
		t.Fatal("verbose version logs nothing")
	}
}

func TestResolveCommand(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "baseline.json")
	_, errOut, code := runCLI(t, "resolve", exProfile, "-o", out, "--seed", "x")
	if code != ExitOK || !strings.Contains(errOut, "143 controls in 17 groups") {
		t.Fatalf("resolve: %d %s", code, errOut)
	}
	first, _ := os.ReadFile(out)
	runCLI(t, "resolve", "-q", "--seed", "x", "-o", out, exProfile)
	second, _ := os.ReadFile(out)
	if !bytes.Equal(first, second) {
		t.Fatal("seeded resolution must be reproducible")
	}
	stdout, _, code := runCLI(t, "resolve", "-q", exProfile)
	if code != ExitOK || !strings.HasPrefix(stdout, "{\n  \"catalog\"") {
		t.Fatal("default output is stdout")
	}
	if _, errOut, code := runCLI(t, "resolve", "missing.json"); code != ExitUsage || !strings.Contains(errOut, "resolve:") {
		t.Fatal("missing profile")
	}
	if _, _, code := runCLI(t, "resolve", exProfile, "-o", filepath.Join(dir, "no", "such", "dir.json")); code != ExitUsage {
		t.Fatal("unwritable output")
	}
}

func TestValidateCommand(t *testing.T) {
	out, _, code := runCLI(t, "validate", exProfile, filepath.Join(nistDir, "NIST_SP-800-53_rev5_LOW-baseline_profile.json"))
	if code != ExitOK || strings.Count(out, " valid ") != 2 {
		t.Fatalf("validate: %d\n%s", code, out)
	}
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	_ = os.WriteFile(bad, []byte(`{"catalog":{"uuid":"x","metadata":{"title":"t"},"controls":[{"id":"a","title":"A","links":[{"href":"#11111111-1111-4111-8111-111111111111"}]}]}}`), 0o600)
	out, _, code = runCLI(t, "validate", "--max", "2", bad)
	if code != ExitFindings || !strings.Contains(out, "INVALID") || !strings.Contains(out, "more (use --max 0") {
		t.Fatalf("invalid doc: %d\n%s", code, out)
	}
	out, _, _ = runCLI(t, "validate", "--format", "json", bad)
	var reps []fileReport
	if err := json.Unmarshal([]byte(out), &reps); err != nil || len(reps) != 1 || reps[0].Valid || len(reps[0].Semantic) == 0 {
		t.Fatalf("json output: %v %s", err, out)
	}
	out, _, _ = runCLI(t, "validate", "--schema-only", "--format", "json", bad)
	_ = json.Unmarshal([]byte(out), &reps)
	if len(reps[0].Semantic) != 0 {
		t.Fatal("schema-only skips semantics")
	}
	notOSCAL := filepath.Join(dir, "x.json")
	_ = os.WriteFile(notOSCAL, []byte(`{"hello":1}`), 0o600)
	out, _, code = runCLI(t, "validate", notOSCAL, filepath.Join(dir, "missing.json"))
	if code != ExitFindings || strings.Count(out, "ERROR") != 2 {
		t.Fatalf("errors per file: %d %s", code, out)
	}
	if _, _, code := runCLI(t, "validate", "--format", "xml", exProfile); code != ExitUsage {
		t.Fatal("bad format")
	}
}

func generateSSP(t *testing.T, dir string, extra ...string) string {
	t.Helper()
	out := filepath.Join(dir, "ssp.json")
	args := append([]string{"generate", "--profile", exProfile, "--component", components(t), "--system-name", "Portal", "--impact", "low", "--seed", "s", "-o", out}, extra...)
	_, errOut, code := runCLI(t, args...)
	if code != ExitOK {
		t.Fatalf("generate: %d %s", code, errOut)
	}
	if !strings.Contains(errOut, "143 baseline controls, 39 addressed by 4 components") {
		t.Fatalf("generate summary: %s", errOut)
	}
	return out
}

func TestGenerateAndGaps(t *testing.T) {
	dir := t.TempDir()
	ssp := generateSSP(t, dir)
	data, _ := os.ReadFile(ssp)
	if !strings.Contains(string(data), `"href": "`) || strings.Contains(string(data), dir) {
		t.Fatal("import-profile href must be relative to the output file")
	}
	// The SSP's import-profile href is followed when --baseline is omitted.
	out, _, code := runCLI(t, "gaps", ssp)
	if code != ExitOK || !strings.Contains(out, "143 in baseline, 143 addressed") || !strings.Contains(out, "FAMILY") {
		t.Fatalf("gaps text: %d\n%s", code, out)
	}
	if _, _, code := runCLI(t, "gaps", "--fail-on", "warning", ssp); code != ExitFindings {
		t.Fatal("warnings exist, so --fail-on warning must fail")
	}
	out, _, _ = runCLI(t, "gaps", "--format", "markdown", "--max", "3", "--baseline", exProfile, ssp)
	if !strings.Contains(out, "| Family | Title |") || !strings.Contains(out, "more gaps") {
		t.Fatalf("markdown:\n%s", out)
	}
	out, _, _ = runCLI(t, "gaps", "--format", "json", "--fail-on", "never", ssp)
	var rep map[string]any
	if err := json.Unmarshal([]byte(out), &rep); err != nil || rep["summary"] == nil {
		t.Fatalf("json: %v", err)
	}
	// Without uncovered controls, the missing ones are errors.
	sparse := filepath.Join(dir, "sparse.json")
	runCLI(t, "generate", "--profile", exProfile, "--component", components(t), "--no-uncovered", "-o", sparse)
	out, _, code = runCLI(t, "gaps", "--max", "1", sparse)
	if code != ExitFindings || !strings.Contains(out, "missing-control") {
		t.Fatalf("missing controls must fail: %d\n%s", code, out)
	}
	for _, bad := range [][]string{
		{"gaps", "--format", "pdf", ssp},
		{"gaps", "--fail-on", "sometimes", ssp},
		{"gaps", exProfile},
		{"gaps", "missing.json"},
		{"gaps", "--baseline", "missing.json", ssp},
		{"generate", "--profile", exProfile, "--component", exProfile},
		{"generate", "--profile", exProfile, "--component", "missing.json"},
		{"generate", "--profile", "missing.json"},
		{"generate", "--profile", exProfile, "--impact", "severe"},
		{"generate", "--profile", exProfile, "extra-arg"},
	} {
		if _, _, code := runCLI(t, bad...); code != ExitUsage {
			t.Errorf("%v: want usage error, got %d", bad, code)
		}
	}
	stdout, _, code := runCLI(t, "generate", "--profile", filepath.Join(nistDir, "NIST_SP-800-53_rev5_LOW-baseline-resolved-profile_catalog.json"), "--profile-href", "https://example.com/p.json")
	if code != ExitOK || !strings.Contains(stdout, "https://example.com/p.json") {
		t.Fatal("generate from a resolved catalog to stdout")
	}
}

func TestLocalHref(t *testing.T) {
	if p, err := localHref("a/b/ssp.json", "../p.json"); err != nil || filepath.ToSlash(p) != "a/p.json" {
		t.Fatalf("relative: %s %v", p, err)
	}
	if p, err := localHref("ssp.json", "file:///abs/p.json"); err != nil || filepath.ToSlash(p) != "/abs/p.json" {
		t.Fatalf("file uri: %s %v", p, err)
	}
	for _, bad := range []string{"", "https://x/p.json", "#uuid", "%zz"} {
		if _, err := localHref("ssp.json", bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
	if relativeHref("-", "x/y.json") != "x/y.json" || truncate("abcdef", 4) != "abc…" || truncate("ab", 4) != "ab" {
		t.Fatal("helpers")
	}
}

func TestRenderCommand(t *testing.T) {
	dir := t.TempDir()
	ssp := generateSSP(t, dir)
	out, _, code := runCLI(t, "render", "--baseline", exProfile, "--ssp", ssp, "AC-7", "ac-11")
	if code != ExitOK || !strings.Contains(out, "Enforce a limit of 5 consecutive invalid logon attempts") || !strings.Contains(out, "device lock after 15 minutes of inactivity") {
		t.Fatalf("render:\n%s", out)
	}
	out, _, _ = runCLI(t, "render", "--baseline", exProfile, "--set", "ac-02_odp.02=group, role", "ac-2")
	if !strings.Contains(out, "ac-02_odp.02 (set): group, role") || !strings.Contains(out, "(unset)") {
		t.Fatalf("render --set:\n%s", out)
	}
	for _, bad := range [][]string{
		{"render", "--baseline", exProfile, "zz-99"},
		{"render", "--baseline", exProfile, "--set", "novalue", "ac-2"},
		{"render", "--baseline", exProfile, "--ssp", exProfile, "ac-2"},
		{"render", "--baseline", exProfile, "--ssp", "missing.json", "ac-2"},
		{"render", "--baseline", "missing.json", "ac-2"},
		{"render", "--baseline", filepath.Join(exComps, "acme-sso_component.json"), "ac-2"},
	} {
		if _, _, code := runCLI(t, bad...); code != ExitUsage {
			t.Errorf("%v: want usage error, got %d", bad, code)
		}
	}
}

func TestCompareCommand(t *testing.T) {
	nistProfile := filepath.Join(nistDir, "NIST_SP-800-53_rev5_MODERATE-baseline_profile.json")
	nistResolved := filepath.Join(nistDir, "NIST_SP-800-53_rev5_MODERATE-baseline-resolved-profile_catalog.json")
	out, _, code := runCLI(t, "compare", "--fail-on-diff", nistProfile, nistResolved)
	if code != ExitOK || !strings.Contains(out, "identical") {
		t.Fatalf("our MODERATE resolution must equal NIST's: %d\n%s", code, out)
	}
	low := filepath.Join(nistDir, "NIST_SP-800-53_rev5_LOW-baseline-resolved-profile_catalog.json")
	out, _, code = runCLI(t, "compare", "--fail-on-diff", exProfile, low)
	if code != ExitFindings || !strings.Contains(out, "only in A (4): ac-6, ac-11, sc-8, sc-28") || !strings.Contains(out, "content differs (2): ac-1, ac-2") {
		t.Fatalf("tailored vs LOW:\n%s", out)
	}
	out, _, _ = runCLI(t, "compare", "--format", "json", "--exact", exProfile, low)
	if !strings.Contains(out, `"only-in-b"`) {
		t.Fatal("json compare")
	}
	if _, _, code := runCLI(t, "compare", "--format", "yaml", exProfile, low); code != ExitUsage {
		t.Fatal("bad format")
	}
	if _, _, code := runCLI(t, "compare", "missing.json", low); code != ExitUsage {
		t.Fatal("missing A")
	}
	if _, _, code := runCLI(t, "compare", low, "missing.json"); code != ExitUsage {
		t.Fatal("missing B")
	}
}

func TestRunEntryPoint(t *testing.T) {
	var out, errb bytes.Buffer
	if code := Run([]string{"version"}, &out, &errb); code != ExitOK {
		t.Fatal("Run")
	}
	var l listFlag
	_ = l.Set("a, b,,c")
	if l.String() != "a,b,c" {
		t.Fatal(l.String())
	}
	var m multiFlag
	_ = m.Set("x=1,2")
	_ = m.Set("y=3")
	if m.String() != "x=1,2; y=3" {
		t.Fatal(m.String())
	}
}

// TestExampleDraftSSP pins the findings the demo shows for the deliberately
// flawed hand-edited SSP.
func TestExampleDraftSSP(t *testing.T) {
	draft := "../../examples/ssp/acme-ssp-draft.json"
	out, _, code := runCLI(t, "validate", draft)
	if code != ExitFindings || !strings.Contains(out, "2 semantic errors") || !strings.Contains(out, "unknown-component") || !strings.Contains(out, "unknown-role") {
		t.Fatalf("validate draft: %d\n%s", code, out)
	}
	out, _, code = runCLI(t, "gaps", "--format", "json", draft)
	if code != ExitFindings {
		t.Fatalf("gaps draft must fail on errors, got %d", code)
	}
	var rep struct {
		Summary struct {
			BaselineControls int `json:"baseline-controls"`
			Addressed        int `json:"addressed"`
			Satisfied        int `json:"satisfied"`
		} `json:"summary"`
		Gaps []struct {
			ControlID string `json:"control-id"`
			Kind      string `json:"kind"`
		} `json:"gaps"`
	}
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Summary.BaselineControls != 143 || rep.Summary.Addressed != 6 || rep.Summary.Satisfied != 2 {
		t.Fatalf("summary = %+v", rep.Summary)
	}
	want := map[string]string{"ac-2": "unknown-statement", "ac-7": "unknown-component", "ac-8": "not-applicable-unjustified", "au-2": "unknown-parameter", "pe-2": "not-in-baseline", "ac-3": "missing-control"}
	for ctl, kind := range want {
		found := false
		for _, g := range rep.Gaps {
			if g.ControlID == ctl && g.Kind == kind {
				found = true
			}
		}
		if !found {
			t.Errorf("missing %s gap for %s", kind, ctl)
		}
	}
}

// Regression: a component that repeats its controls in a second
// control-implementation used to produce an SSP with duplicate uuids, and
// generate wrote it with exit 0 because its self-check was schema-only.
func TestGenerateRepeatedControlImplementationIsValid(t *testing.T) {
	dir := t.TempDir()
	raw, err := os.ReadFile(filepath.Join(exComps, "acme-sso_component.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	comp := doc["component-definition"].(map[string]any)["components"].([]any)[0].(map[string]any)
	cis := comp["control-implementations"].([]any)
	var second map[string]any
	_ = json.Unmarshal(mustJSON(t, cis[0]), &second)
	second["uuid"] = "bbbbbbbb-0000-4000-8000-000000000002"
	second["source"] = "https://example.org/NIST_SP-800-53_rev5_LOW-baseline_profile.json"
	for i, ir := range second["implemented-requirements"].([]any) {
		irm := ir.(map[string]any)
		irm["uuid"] = fmt.Sprintf("bbbbbbbb-0000-4000-8000-1000000000%02d", i)
		sts, _ := irm["statements"].([]any)
		for j, st := range sts {
			st.(map[string]any)["uuid"] = fmt.Sprintf("bbbbbbbb-0000-4000-8000-2000000%02d%03d", i, j)
		}
	}
	comp["control-implementations"] = append(cis, second)
	twice := filepath.Join(dir, "sso-twice.json")
	if err := os.WriteFile(twice, mustJSON(t, doc), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, _, code := runCLI(t, "validate", twice); code != ExitOK {
		t.Fatalf("the input itself must be valid: %d %s", code, out)
	}
	ssp := filepath.Join(dir, "ssp.json")
	_, errOut, code := runCLI(t, "generate", "--seed", "s", "--profile", exProfile, "--component", twice, "-o", ssp)
	if code != ExitOK || !strings.Contains(errOut, "merged-implementations") {
		t.Fatalf("generate: %d %s", code, errOut)
	}
	if out, _, code := runCLI(t, "validate", ssp); code != ExitOK {
		t.Fatalf("generated SSP must validate: %d %s", code, out)
	}
}

// The generate self-check includes the semantic checks: an input that
// yields a schema-valid but semantically invalid SSP (here an
// implementation-status outside OSCAL's allowed values) is refused and
// nothing is written.
func TestGenerateSemanticSelfCheck(t *testing.T) {
	dir := t.TempDir()
	raw, err := os.ReadFile(filepath.Join(exComps, "acme-sso_component.json"))
	if err != nil {
		t.Fatal(err)
	}
	bad := strings.Replace(string(raw), `"value": "partial"`, `"value": "mostly-done"`, 1)
	if bad == string(raw) {
		t.Fatal("fixture no longer has a partial status to corrupt")
	}
	in := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(in, []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "ssp.json")
	_, errOut, code := runCLI(t, "generate", "--seed", "s", "--profile", exProfile, "--component", in, "-o", out)
	if code == ExitOK || !strings.Contains(errOut, "semantic check") || !strings.Contains(errOut, "mostly-done") {
		t.Fatalf("want a semantic self-check failure: %d %s", code, errOut)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("nothing may be written when the self-check fails")
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
