package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/SlateGitOrg/oscalkit/gaps"
	"github.com/SlateGitOrg/oscalkit/oscal"
)

func runGaps(ctx context.Context, e *env, args []string) int {
	fs := newFlags(e, "gaps", "SSP")
	var rf resolverFlags
	rf.register(fs)
	baseline := fs.String("baseline", "", "baseline profile or resolved catalog (default: the SSP's import-profile href)")
	format := fs.String("format", "text", "output format: text, markdown or json")
	failOn := fs.String("fail-on", "error", "exit 1 when a gap at or above this severity exists: error, warning, info or never")
	maxShown := fs.Int("max", 25, "maximum gaps listed in text/markdown output (0 = all)")
	ignoreParams := fs.Bool("ignore-params", false, "do not report unset organization-defined parameters")
	pos, code, ok := interleave(fs, args)
	if !ok {
		return code
	}
	if len(pos) != 1 {
		fs.Usage()
		return ExitUsage
	}
	doc, err := oscal.ReadFile(pos[0])
	if err != nil {
		return e.errorf("gaps: %v", err)
	}
	if doc.SSP == nil {
		return e.errorf("gaps: %s is a %s, not a system-security-plan", pos[0], doc.Model())
	}
	basePath := *baseline
	if basePath == "" {
		basePath, err = localHref(pos[0], doc.SSP.ImportProfile.Href)
		if err != nil {
			return e.errorf("gaps: %v (pass --baseline)", err)
		}
	}
	cat, err := loadBaseline(ctx, e, basePath, &rf)
	if err != nil {
		return e.errorf("gaps: baseline: %v", err)
	}
	rep := gaps.Analyze(doc.SSP, cat, gaps.Options{IgnoreParameters: *ignoreParams})
	switch *format {
	case "json":
		enc := json.NewEncoder(e.stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			return e.errorf("%v", err)
		}
	case "text", "markdown":
		printGaps(e, rep, *format == "markdown", *maxShown)
	default:
		return e.errorf("gaps: unknown format %q", *format)
	}
	if *failOn == "never" {
		return ExitOK
	}
	sev, err := gaps.ParseSeverity(*failOn)
	if err != nil {
		return e.errorf("gaps: --fail-on: %v", err)
	}
	if rep.Count(sev) > 0 {
		return ExitFindings
	}
	return ExitOK
}

// localHref resolves an SSP's import-profile href relative to the SSP file.
// Only local references are followed.
func localHref(sspPath, href string) (string, error) {
	if href == "" {
		return "", fmt.Errorf("the SSP has no import-profile href")
	}
	u, err := url.Parse(href)
	if err != nil {
		return "", fmt.Errorf("bad import-profile href %q: %w", href, err)
	}
	if u.Scheme != "" && u.Scheme != "file" {
		return "", fmt.Errorf("import-profile href %q is not a local file", href)
	}
	if strings.HasPrefix(href, "#") {
		return "", fmt.Errorf("import-profile href %q points into back-matter, which is not supported", href)
	}
	p := filepath.FromSlash(u.Path)
	if filepath.IsAbs(p) {
		return p, nil
	}
	return filepath.Join(filepath.Dir(sspPath), p), nil
}

func printGaps(e *env, rep *gaps.Report, md bool, maxShown int) {
	w := e.stdout
	s := rep.Summary
	statuses := []gaps.Status{gaps.StatImplemented, gaps.StatNotApplicable, gaps.StatAlternative, gaps.StatUnspecified, gaps.StatPartial, gaps.StatPlanned, gaps.StatMissing}
	var parts []string
	for _, st := range statuses {
		if n := s.ByStatus[st]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", st, n))
		}
	}
	sev := fmt.Sprintf("%d error, %d warning, %d info", s.GapsBySeverity[gaps.Error], s.GapsBySeverity[gaps.Warning], s.GapsBySeverity[gaps.Info])
	if md {
		fmt.Fprintf(w, "# Gap analysis: %s\n\n", rep.SSPTitle)
		fmt.Fprintf(w, "- **Baseline:** %s\n", rep.BaselineTitle)
		fmt.Fprintf(w, "- **Controls:** %d in baseline, %d addressed, %d satisfied (%.1f%%)\n", s.BaselineControls, s.Addressed, s.Satisfied, s.CoveragePercent)
		fmt.Fprintf(w, "- **Status:** %s\n", strings.Join(parts, ", "))
		fmt.Fprintf(w, "- **Gaps:** %s\n\n", sev)
		fmt.Fprintln(w, "| Family | Title | Satisfied | Missing |")
		fmt.Fprintln(w, "|---|---|---|---|")
		for _, f := range rep.Families {
			fmt.Fprintf(w, "| %s | %s | %d/%d | %d |\n", strings.ToUpper(f.Family), f.Title, f.Satisfied, f.Total, f.Missing)
		}
		fmt.Fprintln(w, "\n| Severity | Control | Kind | Detail |")
		fmt.Fprintln(w, "|---|---|---|---|")
	} else {
		fmt.Fprintf(w, "Gap analysis: %s\n", rep.SSPTitle)
		fmt.Fprintf(w, "Baseline:     %s\n", rep.BaselineTitle)
		fmt.Fprintf(w, "Controls:     %d in baseline, %d addressed, %d satisfied (%.1f%%)\n", s.BaselineControls, s.Addressed, s.Satisfied, s.CoveragePercent)
		fmt.Fprintf(w, "Status:       %s\n", strings.Join(parts, ", "))
		fmt.Fprintf(w, "Gaps:         %s\n\n", sev)
		fmt.Fprintf(w, "%-6s %-44s %9s %8s\n", "FAMILY", "TITLE", "SATISFIED", "MISSING")
		for _, f := range rep.Families {
			fmt.Fprintf(w, "%-6s %-44s %9s %8d\n", strings.ToUpper(f.Family), truncate(f.Title, 44), fmt.Sprintf("%d/%d", f.Satisfied, f.Total), f.Missing)
		}
		fmt.Fprintln(w)
	}
	shown := rep.Gaps
	if maxShown > 0 && len(shown) > maxShown {
		shown = shown[:maxShown]
	}
	for _, g := range shown {
		if md {
			fmt.Fprintf(w, "| %s | %s | %s | %s |\n", g.Severity, g.ControlID, g.Kind, strings.ReplaceAll(g.Detail, "|", "\\|"))
		} else {
			fmt.Fprintf(w, "%-8s %-10s %-27s %s\n", strings.ToUpper(g.Severity.String()), g.ControlID, g.Kind, g.Detail)
		}
	}
	if len(shown) < len(rep.Gaps) {
		fmt.Fprintf(w, "\n... %d more gaps (use --max 0 to list all)\n", len(rep.Gaps)-len(shown))
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
