package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/archit2501/oscalkit/oscal"
	"github.com/archit2501/oscalkit/param"
)

func runRender(ctx context.Context, e *env, args []string) int {
	fs := newFlags(e, "render", "CONTROL-ID...")
	var rf resolverFlags
	rf.register(fs)
	baseline := fs.String("baseline", "", "profile or catalog containing the controls (required)")
	sspPath := fs.String("ssp", "", "apply the parameter values set in this SSP")
	var sets listFlag
	fs.Var(&sets, "set", "override a parameter: id=value (repeatable)")
	pos, code, ok := interleave(fs, args)
	if !ok {
		return code
	}
	if *baseline == "" || len(pos) == 0 {
		fs.Usage()
		return ExitUsage
	}
	cat, err := loadBaseline(ctx, e, *baseline, &rf)
	if err != nil {
		return e.errorf("render: %v", err)
	}
	var ssp *oscal.SSP
	if *sspPath != "" {
		doc, err := oscal.ReadFile(*sspPath)
		if err != nil {
			return e.errorf("render: %v", err)
		}
		if doc.SSP == nil {
			return e.errorf("render: %s is not an SSP", *sspPath)
		}
		ssp = doc.SSP
	}
	for i, id := range pos {
		ctl, ok := cat.Control(strings.ToLower(id))
		if !ok {
			return e.errorf("render: control %q is not in the baseline", id)
		}
		r := param.NewRenderer(cat)
		if ssp != nil {
			applySSPValues(r, ssp, ctl.ID)
		}
		for _, s := range sets {
			k, v, found := strings.Cut(s, "=")
			if !found {
				return e.errorf("render: --set %q must be id=value", s)
			}
			r.SetValues(k, []string{v})
		}
		if i > 0 {
			fmt.Fprintln(e.stdout)
		}
		fmt.Fprintf(e.stdout, "## %s %s\n\n", strings.ToUpper(ctl.Label()), ctl.Title)
		fmt.Fprint(e.stdout, param.FormatMarkdown(r.Statement(ctl)))
		if refs := param.StatementRefs(ctl); len(refs) > 0 {
			fmt.Fprintln(e.stdout, "\nParameters:")
			for _, id := range refs {
				state := "unset"
				if r.IsSet(id) {
					state = "set"
				}
				fmt.Fprintf(e.stdout, "- %s (%s): %s\n", id, state, r.Text(id))
			}
		}
	}
	return ExitOK
}

// applySSPValues applies an SSP's global set-parameters and those of the
// given control's implemented-requirements (by-component values first, then
// requirement-level values, which win).
func applySSPValues(r *param.Renderer, ssp *oscal.SSP, controlID string) {
	for _, sp := range ssp.ControlImplementation.SetParameters {
		r.SetValues(sp.ParamID, sp.Values)
	}
	for _, ir := range ssp.ControlImplementation.ImplementedRequirements {
		if ir.ControlID != controlID {
			continue
		}
		for _, bc := range ir.ByComponents {
			for _, sp := range bc.SetParameters {
				r.SetValues(sp.ParamID, sp.Values)
			}
		}
		for _, sp := range ir.SetParameters {
			r.SetValues(sp.ParamID, sp.Values)
		}
	}
}
