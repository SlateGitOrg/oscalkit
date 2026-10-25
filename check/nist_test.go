package check

import (
	"path/filepath"
	"testing"

	"github.com/SlateGitOrg/oscalkit/oscal"
)

// TestNISTContentIsClean ensures the semantic checks raise no false
// positives on the official NIST catalog, profiles and resolved baselines.
func TestNISTContentIsClean(t *testing.T) {
	dir := "../testdata/oscal-content/nist.gov/SP800-53/rev5/json"
	files, err := filepath.Glob(filepath.Join(dir, "*.json*"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}
	for _, f := range files {
		doc, err := oscal.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if fs := Document(doc); len(fs) > 0 {
			t.Errorf("%s: %d findings, first: %v", filepath.Base(f), len(fs), fs[0])
		}
	}
}
