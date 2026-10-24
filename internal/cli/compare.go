package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/archit2501/oscalkit/catdiff"
)

func runCompare(ctx context.Context, e *env, args []string) int {
	fs := newFlags(e, "compare", "A B")
	var rf resolverFlags
	rf.register(fs)
	exact := fs.Bool("exact", false, "compare strings exactly (default: normalize whitespace)")
	format := fs.String("format", "text", "output format: text or json")
	failOnDiff := fs.Bool("fail-on-diff", false, "exit 1 when the catalogs differ")
	pos, code, ok := interleave(fs, args)
	if !ok {
		return code
	}
	if len(pos) != 2 {
		fs.Usage()
		return ExitUsage
	}
	a, err := loadBaseline(ctx, e, pos[0], &rf)
	if err != nil {
		return e.errorf("compare: %v", err)
	}
	b, err := loadBaseline(ctx, e, pos[1], &rf)
	if err != nil {
		return e.errorf("compare: %v", err)
	}
	rep, err := catdiff.Compare(a, b, catdiff.Options{ExactWhitespace: *exact})
	if err != nil {
		return e.errorf("compare: %v", err)
	}
	switch *format {
	case "json":
		enc := json.NewEncoder(e.stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			return e.errorf("%v", err)
		}
	case "text":
		printCompare(e, rep, pos[0], pos[1])
	default:
		return e.errorf("compare: unknown format %q", *format)
	}
	if *failOnDiff && !rep.Identical() {
		return ExitFindings
	}
	return ExitOK
}

func printCompare(e *env, rep *catdiff.Report, a, b string) {
	w := e.stdout
	fmt.Fprintf(w, "A: %s (%d controls)\nB: %s (%d controls)\n", a, rep.ControlsA, b, rep.ControlsB)
	if rep.Identical() {
		fmt.Fprintln(w, "identical: same controls, order, nesting, content, groups and back-matter")
		return
	}
	list := func(label string, ids []string) {
		if len(ids) > 0 {
			fmt.Fprintf(w, "%s (%d): %s\n", label, len(ids), strings.Join(ids, ", "))
		}
	}
	list("only in A", rep.OnlyInA)
	list("only in B", rep.OnlyInB)
	if rep.OrderDiffers {
		fmt.Fprintln(w, "shared controls appear in a different order")
	}
	for _, m := range rep.Moved {
		fmt.Fprintf(w, "moved %s: A under %s, B under %s\n", m.ID, m.A, m.B)
	}
	list("content differs", rep.Changed)
	list("groups only in A", rep.GroupsOnlyInA)
	list("groups only in B", rep.GroupsOnlyInB)
	list("group content differs", rep.ChangedGroups)
	list("resources only in A", rep.ResourcesOnlyInA)
	list("resources only in B", rep.ResourcesOnlyInB)
}
