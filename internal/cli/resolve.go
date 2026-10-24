package cli

import (
	"context"
	"fmt"

	"github.com/archit2501/oscalkit/oscal"
	"github.com/archit2501/oscalkit/resolve"
)

func runResolve(ctx context.Context, e *env, args []string) int {
	fs := newFlags(e, "resolve", "PROFILE")
	var rf resolverFlags
	rf.register(fs)
	out := fs.String("o", "-", "write the resolved catalog to this file (- for stdout)")
	quiet := fs.Bool("q", false, "do not print the summary and warnings")
	pos, code, ok := interleave(fs, args)
	if !ok {
		return code
	}
	if len(pos) != 1 {
		fs.Usage()
		return ExitUsage
	}
	res, err := resolve.ResolveFile(ctx, pos[0], rf.options(e))
	if err != nil {
		return e.errorf("resolve: %v", err)
	}
	if err := oscal.WriteFile(*out, oscal.Document{Catalog: res.Catalog}); err != nil {
		return e.errorf("%v", err)
	}
	if !*quiet {
		cat := res.Catalog
		nControls := len(cat.ControlIDs())
		fmt.Fprintf(e.stderr, "resolved %q: %d controls in %d groups, %d loose params, %d back-matter resources (%d source documents)\n",
			cat.Metadata.Title, nControls, len(cat.Groups), len(cat.Params), resourceCount(cat), len(res.Sources))
		for _, w := range res.Warnings {
			fmt.Fprintf(e.stderr, "warning: %s\n", w)
		}
	}
	return ExitOK
}

func resourceCount(cat *oscal.Catalog) int {
	if cat.BackMatter == nil {
		return 0
	}
	return len(cat.BackMatter.Resources)
}
