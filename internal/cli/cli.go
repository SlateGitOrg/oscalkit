// Package cli implements the oscalkit command-line interface.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/SlateGitOrg/oscalkit/oscal"
	"github.com/SlateGitOrg/oscalkit/resolve"
)

// Version is the oscalkit version, overridable at link time.
var Version = "0.1.0"

// Exit codes.
const (
	ExitOK       = 0
	ExitFindings = 1 // validation errors or gaps at/above the failure threshold
	ExitUsage    = 2 // bad arguments, unreadable input, internal errors
)

type command struct {
	name    string
	summary string
	run     func(ctx context.Context, env *env, args []string) int
}

// env carries the output streams and shared settings for one invocation.
type env struct {
	stdout io.Writer
	stderr io.Writer
	logger *slog.Logger
	now    func() time.Time
}

func (e *env) errorf(format string, args ...any) int {
	fmt.Fprintf(e.stderr, "oscalkit: "+format+"\n", args...)
	return ExitUsage
}

var commands []command

func init() {
	commands = []command{
		{"resolve", "resolve a profile into a catalog", runResolve},
		{"validate", "schema + semantic validation of OSCAL documents", runValidate},
		{"gaps", "report control gaps in an SSP against its resolved baseline", runGaps},
		{"generate", "generate an SSP skeleton from component definitions", runGenerate},
		{"render", "render control statements with parameter substitution", runRender},
		{"compare", "compare the controls of two catalogs or profiles", runCompare},
		{"version", "print the version", runVersion},
	}
}

// Run executes the CLI with args (without the program name) and returns the
// process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	return run(context.Background(), args, stdout, stderr, time.Now)
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, now func() time.Time) int {
	e := &env{stdout: stdout, stderr: stderr, now: now, logger: slog.New(slog.DiscardHandler)}
	if len(args) > 0 && (args[0] == "-v" || args[0] == "--verbose") {
		e.logger = slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
		args = args[1:]
	}
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		usage(stdout)
		if len(args) == 0 {
			return ExitUsage
		}
		return ExitOK
	}
	for _, c := range commands {
		if c.name == args[0] {
			return c.run(ctx, e, args[1:])
		}
	}
	fmt.Fprintf(stderr, "oscalkit: unknown command %q\n\n", args[0])
	usage(stderr)
	return ExitUsage
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "oscalkit - NIST OSCAL toolkit (profile resolution, validation, SSP generation, gap analysis)")
	fmt.Fprintln(w, "\nUsage: oscalkit [-v] <command> [flags] [args]")
	fmt.Fprintln(w, "\nCommands:")
	for _, c := range commands {
		fmt.Fprintf(w, "  %-10s %s\n", c.name, c.summary)
	}
	fmt.Fprintln(w, "\nRun 'oscalkit <command> -h' for command flags.")
}

// newFlags creates a flag set that reports errors instead of exiting.
func newFlags(e *env, name, argsUsage string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(e.stderr)
	fs.Usage = func() {
		fmt.Fprintf(e.stderr, "Usage: oscalkit %s [flags] %s\n\nFlags:\n", name, argsUsage)
		fs.PrintDefaults()
	}
	return fs
}

// parse parses flags; ok is false when the command should exit with code.
func parse(fs *flag.FlagSet, args []string) (code int, ok bool) {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return ExitOK, false
		}
		return ExitUsage, false
	}
	return 0, true
}

// interleave lets flags appear after positional arguments
// ("oscalkit resolve p.json -o out.json") by parsing repeatedly.
func interleave(fs *flag.FlagSet, args []string) ([]string, int, bool) {
	var positional []string
	for {
		if code, ok := parse(fs, args); !ok {
			return nil, code, false
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return positional, 0, true
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
}

// resolverFlags are shared by every command that may resolve a profile.
type resolverFlags struct {
	allowRemote      bool
	includeAncestors bool
	seed             string
}

func (r *resolverFlags) register(fs *flag.FlagSet) {
	fs.BoolVar(&r.allowRemote, "allow-remote", false, "allow fetching http(s) imports")
	fs.BoolVar(&r.includeAncestors, "include-ancestors", false, "including a control also includes its parent controls (spec prose; NIST's resolver does not)")
	fs.StringVar(&r.seed, "seed", "", "derive UUIDs deterministically from this seed (reproducible output)")
}

func (r *resolverFlags) uuids() oscal.UUIDGen {
	if r.seed != "" {
		return oscal.DeterministicUUIDs{Seed: r.seed}
	}
	return oscal.RandomUUIDs{}
}

func (r *resolverFlags) options(e *env) resolve.Options {
	return resolve.Options{
		Fetcher:          resolve.FileFetcher{AllowRemote: r.allowRemote},
		UUIDs:            r.uuids(),
		Now:              e.now,
		Logger:           e.logger,
		ToolName:         "oscalkit " + Version,
		IncludeAncestors: r.includeAncestors,
	}
}

// loadBaseline returns a catalog for path: a catalog is used as-is, a
// profile is resolved first. Resolution warnings are printed to stderr.
func loadBaseline(ctx context.Context, e *env, path string, rf *resolverFlags) (*oscal.Catalog, error) {
	doc, err := oscal.ReadFile(path)
	if err != nil {
		return nil, err
	}
	switch {
	case doc.Catalog != nil:
		return doc.Catalog, nil
	case doc.Profile != nil:
		res, err := resolve.ResolveFile(ctx, path, rf.options(e))
		if err != nil {
			return nil, err
		}
		for _, w := range res.Warnings {
			fmt.Fprintf(e.stderr, "warning: %s\n", w)
		}
		return res.Catalog, nil
	}
	return nil, fmt.Errorf("%s is a %s; expected a catalog or profile", path, doc.Model())
}

func runVersion(_ context.Context, e *env, _ []string) int {
	fmt.Fprintf(e.stdout, "oscalkit %s (OSCAL %s)\n", Version, resolve.ToolOSCALVersion)
	return ExitOK
}

// listFlag collects repeated string flags.
type listFlag []string

func (l *listFlag) String() string { return strings.Join(*l, ",") }

func (l *listFlag) Set(v string) error {
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			*l = append(*l, p)
		}
	}
	return nil
}

// writeDoc writes v as JSON to path, or to the command's stdout for "-".
func writeDoc(e *env, path string, v any) error {
	if path == "-" || path == "" {
		return oscal.Encode(e.stdout, v)
	}
	return oscal.WriteFile(path, v)
}

// multiFlag collects repeated string flags verbatim (values may contain commas).
type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, "; ") }

func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}
