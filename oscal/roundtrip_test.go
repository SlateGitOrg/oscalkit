package oscal

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
)

const nistDir = "../testdata/oscal-content/nist.gov/SP800-53/rev5/json"

// TestRoundTripNIST proves the typed model is lossless for the official NIST
// content: decode -> encode must reproduce the same JSON value.
func TestRoundTripNIST(t *testing.T) {
	files := []string{
		"NIST_SP-800-53_rev5_catalog.json.gz",
		"NIST_SP-800-53_rev5_MODERATE-baseline_profile.json",
		"NIST_SP-800-53_rev5_HIGH-baseline-resolved-profile_catalog.json.gz",
		"NIST_SP-800-53_rev5_PRIVACY-baseline-resolved-profile_catalog.json.gz",
	}
	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			doc, err := ReadFile(filepath.Join(nistDir, f))
			if err != nil {
				t.Fatal(err)
			}
			out, err := Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			var want, got any
			if err := json.Unmarshal(doc.JSON, &want); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(out, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(want, got) {
				t.Fatalf("round trip of %s changed the document", f)
			}
		})
	}
}
