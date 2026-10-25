package param

import (
	"strings"
	"testing"

	"github.com/SlateGitOrg/oscalkit/oscal"
)

func loadLow(t *testing.T) *oscal.Catalog {
	t.Helper()
	doc, err := oscal.ReadFile("../testdata/oscal-content/nist.gov/SP800-53/rev5/json/NIST_SP-800-53_rev5_LOW-baseline-resolved-profile_catalog.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	return doc.Catalog
}

func TestRenderNISTAC1(t *testing.T) {
	cat := loadLow(t)
	r := NewRenderer(cat)
	ac1, ok := cat.Control("ac-1")
	if !ok {
		t.Fatal("ac-1 missing")
	}
	md := FormatMarkdown(r.Statement(ac1))
	t.Log("\n" + md)
	if !strings.Contains(md, "[Assignment: organization-defined personnel or roles]") {
		t.Errorf("expected assignment placeholder, got:\n%s", md)
	}
	if !strings.Contains(md, "[Selection (one or more): organization-level; mission/business process-level; system-level]") {
		t.Errorf("expected selection, got:\n%s", md)
	}
	r.SetValues("ac-01_odp.01", []string{"all ISSOs"})
	r.SetValues("ac-01_odp.02", []string{"the CISO"})
	r.SetValues("ac-1_prm_2", []string{"system-level"}) // legacy alias of ac-01_odp.03
	md = FormatMarkdown(r.Statement(ac1))
	if !strings.Contains(md, "disseminate to all ISSOs; the CISO:") {
		t.Errorf("aggregate param not rendered from parts:\n%s", md)
	}
	if !strings.Contains(md, "system-level access control policy") {
		t.Errorf("alias override not applied:\n%s", md)
	}
}
