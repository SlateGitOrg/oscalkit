package cli

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/archit2501/oscalkit/oscal"
	"github.com/archit2501/oscalkit/schema"
	"github.com/archit2501/oscalkit/sspgen"
)

func runGenerate(ctx context.Context, e *env, args []string) int {
	fs := newFlags(e, "generate", "")
	var rf resolverFlags
	rf.register(fs)
	profile := fs.String("profile", "", "baseline profile (or resolved catalog) the SSP implements (required)")
	var components listFlag
	fs.Var(&components, "component", "component definition file (repeatable or comma-separated)")
	systemName := fs.String("system-name", "", "system name")
	impact := fs.String("impact", "", "FIPS-199 impact level: low, moderate or high")
	noUncovered := fs.Bool("no-uncovered", false, "omit controls that no component addresses")
	href := fs.String("profile-href", "", "import-profile href to record (default: path of --profile relative to the output file)")
	out := fs.String("o", "-", "output file (- for stdout)")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if *profile == "" || fs.NArg() != 0 {
		fs.Usage()
		return ExitUsage
	}
	cat, err := loadBaseline(ctx, e, *profile, &rf)
	if err != nil {
		return e.errorf("generate: baseline: %v", err)
	}
	var cds []*oscal.ComponentDefinition
	for _, path := range components {
		doc, err := oscal.ReadFile(path)
		if err != nil {
			return e.errorf("generate: %v", err)
		}
		if doc.ComponentDefinition == nil {
			return e.errorf("generate: %s is a %s, not a component-definition", path, doc.Model())
		}
		cds = append(cds, doc.ComponentDefinition)
	}
	profileHref := *href
	if profileHref == "" {
		profileHref = relativeHref(*out, *profile)
	}
	res, err := sspgen.Generate(cat, cds, sspgen.Options{
		ProfileHref:      profileHref,
		SystemName:       *systemName,
		ImpactLevel:      *impact,
		IncludeUncovered: !*noUncovered,
		UUIDs:            rf.uuids(),
		Now:              e.now,
	})
	if err != nil {
		return e.errorf("generate: %v", err)
	}
	data, err := oscal.Marshal(oscal.Document{SSP: res.SSP})
	if err != nil {
		return e.errorf("generate: %v", err)
	}
	// Self-check: a generated SSP must be schema-valid.
	vs, err := schema.Validate(schema.SSP, data)
	if err != nil {
		return e.errorf("generate: %v", err)
	}
	if len(vs) > 0 {
		return e.errorf("generate: produced a schema-invalid SSP (bug): %v", vs[0])
	}
	if err := oscal.WriteFile(*out, oscal.Document{SSP: res.SSP}); err != nil {
		return e.errorf("generate: %v", err)
	}
	st := res.Stats
	fmt.Fprintf(e.stderr, "generated SSP %q: %d baseline controls, %d addressed by %d components, %d uncovered\n",
		res.SSP.Metadata.Title, st.Controls, st.Covered, st.Components, st.Uncovered)
	for _, d := range res.Diagnostics {
		fmt.Fprintf(e.stderr, "warning: %s\n", d)
	}
	return ExitOK
}

// relativeHref expresses target relative to the directory of the output
// file, using forward slashes; it falls back to target unchanged.
func relativeHref(out, target string) string {
	if out == "-" || out == "" {
		return filepath.ToSlash(target)
	}
	absOut, err1 := filepath.Abs(filepath.Dir(out))
	absTarget, err2 := filepath.Abs(target)
	if err1 != nil || err2 != nil {
		return filepath.ToSlash(target)
	}
	rel, err := filepath.Rel(absOut, absTarget)
	if err != nil {
		return filepath.ToSlash(target)
	}
	return filepath.ToSlash(rel)
}

