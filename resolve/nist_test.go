package resolve

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/archit2501/oscalkit/catdiff"
	"github.com/archit2501/oscalkit/oscal"
	"github.com/archit2501/oscalkit/schema"
)

const nistDir = "../testdata/oscal-content/nist.gov/SP800-53/rev5/json"

func fixedOptions() Options {
	return Options{
		UUIDs: oscal.DeterministicUUIDs{Seed: "test"},
		Now:   func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) },
	}
}

// TestNISTBaselines resolves each official SP 800-53 rev5 baseline profile
// against the pinned official catalog and compares the result with NIST's
// own published resolved-profile catalog: same control set, same order and
// nesting, same per-control content (modulo prose whitespace), same groups
// and the same pruned back-matter.
func TestNISTBaselines(t *testing.T) {
	if testing.Short() {
		t.Skip("NIST conformance test reads ~11 MB of catalogs")
	}
	for _, level := range []string{"LOW", "MODERATE", "HIGH", "PRIVACY"} {
		t.Run(level, func(t *testing.T) {
			profile := filepath.Join(nistDir, "NIST_SP-800-53_rev5_"+level+"-baseline_profile.json")
			res, err := ResolveFile(context.Background(), profile, fixedOptions())
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if len(res.Warnings) > 0 {
				t.Errorf("unexpected warnings: %v", res.Warnings)
			}
			want, err := oscal.ReadFile(filepath.Join(nistDir, "NIST_SP-800-53_rev5_"+level+"-baseline-resolved-profile_catalog.json.gz"))
			if err != nil {
				t.Fatal(err)
			}
			rep, err := catdiff.Compare(res.Catalog, want.Catalog, catdiff.Options{})
			if err != nil {
				t.Fatal(err)
			}
			if !rep.SameControlSet() {
				t.Fatalf("control sets differ: only ours %v, only NIST %v", rep.OnlyInA, rep.OnlyInB)
			}
			if !rep.Identical() {
				t.Fatalf("resolved catalog differs from NIST reference: %+v", rep)
			}
			if got, exp := res.Catalog.Metadata.Title, want.Catalog.Metadata.Title; got != exp {
				t.Errorf("title %q, want %q", got, exp)
			}
			data, err := oscal.Marshal(oscal.Document{Catalog: res.Catalog})
			if err != nil {
				t.Fatal(err)
			}
			vs, err := schema.Validate(schema.Catalog, data)
			if err != nil {
				t.Fatal(err)
			}
			if len(vs) > 0 {
				t.Fatalf("resolved catalog is not schema-valid: %v", vs)
			}
			t.Logf("%s: %d controls, %d back-matter resources match NIST", level, rep.ControlsA, len(res.Catalog.BackMatter.Resources))
		})
	}
}
