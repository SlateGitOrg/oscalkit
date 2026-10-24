// Package resolve implements OSCAL profile resolution as defined by the NIST
// "OSCAL Profile Resolution" specification: the import phase (include/
// exclude selection, nested profiles, cycle detection), the merge phase
// (combine use-first/keep; flat, as-is and custom structuring), the modify
// phase (set-parameters, alters with adds/removes) and the final operations
// (metadata, back-matter combination and pruning).
package resolve

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/archit2501/oscalkit/oscal"
)

// ToolOSCALVersion is the OSCAL version this resolver implements. Output
// catalogs never claim a newer version.
const ToolOSCALVersion = "1.2.3"

// DefaultMaxDepth bounds profile import nesting.
const DefaultMaxDepth = 32

// Sentinel errors returned (wrapped) by Resolve.
var (
	ErrCircularImport  = errors.New("circular import")
	ErrNoImports       = errors.New("profile has no imports")
	ErrBadImport       = errors.New("import cannot be resolved")
	ErrMergeDirective  = errors.New("invalid merge directive")
	ErrVersionMismatch = errors.New("incompatible OSCAL major versions")
)

// Diagnostic is a non-fatal problem found during resolution (the
// specification's "SHOULD warn" cases).
type Diagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (d Diagnostic) String() string { return d.Code + ": " + d.Message }

// Options configures a resolution.
type Options struct {
	// Fetcher retrieves imported documents. Defaults to a FileFetcher with
	// remote access disabled.
	Fetcher Fetcher
	// UUIDs generates the output catalog UUID. Defaults to random v4.
	UUIDs oscal.UUIDGen
	// Now supplies the last-modified timestamp. Defaults to time.Now.
	Now func() time.Time
	// Logger receives debug events. Defaults to a discarding logger.
	Logger *slog.Logger
	// ToolName is recorded in the resolution-tool metadata prop.
	ToolName string
	// MaxDepth bounds import nesting (default DefaultMaxDepth).
	MaxDepth int
	// IncludeAncestors makes including a control also include its ancestor
	// controls, as the draft specification's prose describes. It is off by
	// default because NIST's published resolved baselines (produced by the
	// reference OPRXP resolver) do not do this: an enhancement selected
	// without its parent is up-leveled instead (see the PRIVACY baseline).
	IncludeAncestors bool
}

// Result is the outcome of a successful resolution.
type Result struct {
	Catalog  *oscal.Catalog
	Warnings []Diagnostic
	// Sources lists the URIs of every document read, in first-read order.
	Sources []string
}

type resolver struct {
	opts     Options
	docs     map[string]*oscal.Document
	resolved map[string]*oscal.Catalog
	warnings []Diagnostic
	sources  []string
}

// ResolveFile reads the profile at path and resolves it.
func ResolveFile(ctx context.Context, path string, opts Options) (*Result, error) {
	uri, err := FileURI(path)
	if err != nil {
		return nil, err
	}
	r := newResolver(opts)
	doc, err := r.load(ctx, uri)
	if err != nil {
		return nil, err
	}
	if doc.Profile == nil {
		return nil, fmt.Errorf("%s is a %s, not a profile", path, doc.Model())
	}
	return r.run(ctx, doc.Profile, uri)
}

// Resolve resolves an in-memory profile. baseURI is the absolute URI that
// relative import hrefs are resolved against (e.g. from FileURI).
func Resolve(ctx context.Context, p *oscal.Profile, baseURI string, opts Options) (*Result, error) {
	return newResolver(opts).run(ctx, p, baseURI)
}

func newResolver(opts Options) *resolver {
	if opts.Fetcher == nil {
		opts.Fetcher = FileFetcher{}
	}
	if opts.UUIDs == nil {
		opts.UUIDs = oscal.RandomUUIDs{}
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.DiscardHandler)
	}
	if opts.ToolName == "" {
		opts.ToolName = "oscalkit"
	}
	if opts.MaxDepth <= 0 {
		opts.MaxDepth = DefaultMaxDepth
	}
	return &resolver{
		opts:     opts,
		docs:     map[string]*oscal.Document{},
		resolved: map[string]*oscal.Catalog{},
	}
}

func (r *resolver) run(ctx context.Context, p *oscal.Profile, baseURI string) (*Result, error) {
	cat, err := r.resolveProfile(ctx, p, baseURI, []string{baseURI})
	if err != nil {
		return nil, err
	}
	return &Result{Catalog: cat, Warnings: r.warnings, Sources: r.sources}, nil
}

func (r *resolver) warn(code, format string, args ...any) {
	d := Diagnostic{Code: code, Message: fmt.Sprintf(format, args...)}
	r.opts.Logger.Debug("resolve warning", "code", d.Code, "message", d.Message)
	r.warnings = append(r.warnings, d)
}

// importedSet is the outcome of the import phase for one import directive.
type importedSet struct {
	href     string
	catalog  *oscal.Catalog
	included map[string]bool
}

func (r *resolver) resolveProfile(ctx context.Context, p *oscal.Profile, base string, stack []string) (*oscal.Catalog, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(p.Imports) == 0 {
		return nil, fmt.Errorf("%s: %w", base, ErrNoImports)
	}
	r.opts.Logger.Debug("resolving profile", "uri", base, "imports", len(p.Imports))

	// Import phase.
	sets := make([]importedSet, 0, len(p.Imports))
	versions := []string{p.Metadata.OSCALVersion}
	for i, imp := range p.Imports {
		src, err := r.loadImport(ctx, p, base, imp.Href, stack)
		if err != nil {
			return nil, fmt.Errorf("import %d (href %q) of %s: %w", i+1, imp.Href, base, err)
		}
		inc := r.selectImport(src, imp)
		sets = append(sets, importedSet{href: imp.Href, catalog: src, included: inc})
		versions = append(versions, src.Metadata.OSCALVersion)
	}

	// Merge phase.
	out, err := r.merge(p.Merge, sets)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", base, err)
	}

	// Modify phase.
	if p.Modify != nil {
		r.modify(out, p.Modify)
	}

	// Final operations.
	oscalVersion, err := resolveOSCALVersion(versions)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", base, err)
	}
	out.UUID = r.opts.UUIDs.New("resolved-catalog:" + base)
	out.Metadata = r.buildMetadata(p, base, oscalVersion, sets)
	bms := make([]*oscal.BackMatter, 0, len(sets)+1)
	for _, s := range sets {
		bms = append(bms, s.catalog.BackMatter)
	}
	bms = append(bms, p.BackMatter)
	out.BackMatter = combineBackMatter(bms)
	keepParams := map[string]bool{}
	if p.Modify != nil {
		for _, sp := range p.Modify.SetParameters {
			keepParams[sp.ParamID] = true
		}
	}
	prune(out, keepParams)
	r.checkDuplicates(out)
	return out, nil
}

// loadImport fetches the document behind an import href and returns it as a
// catalog, resolving it first when it is a profile.
func (r *resolver) loadImport(ctx context.Context, p *oscal.Profile, base, href string, stack []string) (*oscal.Catalog, error) {
	target, inline, err := locate(p, base, href)
	if err != nil {
		return nil, err
	}
	if slices.Contains(stack, target) {
		chain := append(slices.Clone(stack), target)
		return nil, fmt.Errorf("%w: %s", ErrCircularImport, strings.Join(chain, " -> "))
	}
	if len(stack) >= r.opts.MaxDepth {
		return nil, fmt.Errorf("%w: import nesting exceeds %d levels", ErrBadImport, r.opts.MaxDepth)
	}
	var doc *oscal.Document
	if inline != nil {
		doc, err = oscal.Decode(inline, target)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrBadImport, err)
		}
	} else {
		doc, err = r.load(ctx, target)
		if err != nil {
			return nil, err
		}
	}
	switch {
	case doc.Catalog != nil:
		return doc.Catalog, nil
	case doc.Profile != nil:
		if cat, ok := r.resolved[target]; ok {
			return cat, nil
		}
		cat, err := r.resolveProfile(ctx, doc.Profile, target, append(slices.Clone(stack), target))
		if err != nil {
			return nil, err
		}
		r.resolved[target] = cat
		return cat, nil
	}
	return nil, fmt.Errorf("%w: %s is a %s, not a catalog or profile", ErrBadImport, target, doc.Model())
}

func (r *resolver) load(ctx context.Context, uri string) (*oscal.Document, error) {
	if d, ok := r.docs[uri]; ok {
		return d, nil
	}
	data, err := r.opts.Fetcher.Fetch(ctx, uri)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBadImport, err)
	}
	doc, err := oscal.Decode(data, uri)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBadImport, err)
	}
	r.docs[uri] = doc
	r.sources = append(r.sources, uri)
	return doc, nil
}

// locate turns an import href into an absolute URI. Internal "#uuid" hrefs
// are looked up in the profile's back-matter; a JSON rlink is preferred,
// then YAML, then any other rlink, then embedded base64 content (returned as
// inline bytes).
func locate(p *oscal.Profile, base, href string) (string, []byte, error) {
	if href == "" {
		return "", nil, fmt.Errorf("%w: empty href", ErrBadImport)
	}
	if !strings.HasPrefix(href, "#") {
		u, err := resolveRef(base, href)
		return u, nil, err
	}
	id := strings.TrimPrefix(href, "#")
	res, ok := p.BackMatter.Resource(id)
	if !ok {
		return "", nil, fmt.Errorf("%w: no back-matter resource with uuid %s", ErrBadImport, id)
	}
	if rl := preferredRLink(res.RLinks); rl != nil {
		u, err := resolveRef(base, rl.Href)
		return u, nil, err
	}
	if res.Base64 != nil {
		data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(res.Base64.Value))
		if err != nil {
			return "", nil, fmt.Errorf("%w: resource %s: bad base64: %w", ErrBadImport, id, err)
		}
		return base + "#" + id, data, nil
	}
	return "", nil, fmt.Errorf("%w: resource %s has no usable rlink or base64 content", ErrBadImport, id)
}

func preferredRLink(links []oscal.RLink) *oscal.RLink {
	rank := func(l oscal.RLink) int {
		mt := strings.ToLower(l.MediaType)
		h := strings.ToLower(l.Href)
		switch {
		case strings.Contains(mt, "json") || strings.HasSuffix(h, ".json"):
			return 0
		case strings.Contains(mt, "yaml") || strings.HasSuffix(h, ".yaml") || strings.HasSuffix(h, ".yml"):
			return 1
		case strings.Contains(mt, "xml") || strings.HasSuffix(h, ".xml"):
			return 3
		}
		return 2
	}
	var best *oscal.RLink
	for i := range links {
		if rank(links[i]) == 3 {
			continue // XML is not supported
		}
		if best == nil || rank(links[i]) < rank(*best) {
			best = &links[i]
		}
	}
	return best
}

func resolveRef(base, ref string) (string, error) {
	b, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("%w: bad base uri %q: %w", ErrBadImport, base, err)
	}
	u, err := url.Parse(ref)
	if err != nil {
		return "", fmt.Errorf("%w: bad href %q: %w", ErrBadImport, ref, err)
	}
	return b.ResolveReference(u).String(), nil
}

// buildMetadata produces the target metadata: the profile's metadata with a
// fresh timestamp, the computed oscal-version, source-profile and
// resolution-tool markers, plus roles and parties marked keep=always in the
// imported sources.
func (r *resolver) buildMetadata(p *oscal.Profile, base, oscalVersion string, sets []importedSet) oscal.Metadata {
	md := p.Metadata
	md.LastModified = r.opts.Now().UTC().Format(time.RFC3339Nano)
	md.OSCALVersion = oscalVersion
	src := displayURI(base)
	md.Props = append(slices.Clone(md.Props),
		oscal.Property{Name: "resolution-tool", Value: r.opts.ToolName},
		oscal.Property{Name: "source-profile", Value: src},
	)
	md.Links = append(slices.Clone(md.Links), oscal.Link{Href: src, Rel: "source-profile"})
	md.Roles = slices.Clone(md.Roles)
	md.Parties = slices.Clone(md.Parties)
	for _, s := range sets {
		for _, role := range s.catalog.Metadata.Roles {
			if keepAlways(role.Props) && !slices.ContainsFunc(md.Roles, func(x oscal.Role) bool { return x.ID == role.ID }) {
				md.Roles = append(md.Roles, role)
			}
		}
		for _, party := range s.catalog.Metadata.Parties {
			if keepAlways(party.Props) && !slices.ContainsFunc(md.Parties, func(x oscal.Party) bool { return x.UUID == party.UUID }) {
				md.Parties = append(md.Parties, party)
			}
		}
	}
	return md
}

// displayURI shortens file URIs to their final path element so resolved
// catalogs do not leak local directory layouts.
func displayURI(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return uri
	}
	if i := strings.LastIndex(u.Path, "/"); i >= 0 {
		return u.Path[i+1:]
	}
	return u.Path
}

func keepAlways(props []oscal.Property) bool {
	for _, p := range props {
		if p.Name == "keep" && p.Value == "always" {
			return true
		}
	}
	return false
}

type semver struct{ major, minor, patch int }

func parseSemver(s string) (semver, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return semver{}, false
	}
	var v [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return semver{}, false
		}
		v[i] = n
	}
	return semver{v[0], v[1], v[2]}, true
}

func (a semver) less(b semver) bool {
	if a.major != b.major {
		return a.major < b.major
	}
	if a.minor != b.minor {
		return a.minor < b.minor
	}
	return a.patch < b.patch
}

func (a semver) String() string { return fmt.Sprintf("%d.%d.%d", a.major, a.minor, a.patch) }

// resolveOSCALVersion takes the newest version among the inputs, capped at
// the tool's own version, and rejects mixed major versions.
func resolveOSCALVersion(versions []string) (string, error) {
	tool, _ := parseSemver(ToolOSCALVersion)
	var best semver
	found := false
	for _, s := range versions {
		v, ok := parseSemver(s)
		if !ok {
			continue
		}
		if v.major != tool.major {
			return "", fmt.Errorf("%w: input uses OSCAL %s, tool supports %s", ErrVersionMismatch, s, ToolOSCALVersion)
		}
		if !found || best.less(v) {
			best, found = v, true
		}
	}
	if !found || tool.less(best) {
		return tool.String(), nil
	}
	return best.String(), nil
}

// checkDuplicates warns about clashing control and parameter ids in the
// output (expected under combine method "keep" with overlapping imports).
func (r *resolver) checkDuplicates(cat *oscal.Catalog) {
	seen := map[string]bool{}
	reported := map[string]bool{}
	cat.WalkControls(func(ref oscal.ControlRef) bool {
		id := ref.Control.ID
		if seen[id] && !reported[id] {
			reported[id] = true
			r.warn("duplicate-control", "control id %q appears more than once in the resolved catalog", id)
		}
		seen[id] = true
		return true
	})
}
