package resolve

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/SlateGitOrg/oscalkit/oscal"
)

// Fetcher retrieves the bytes of an imported document given its absolute URI.
type Fetcher interface {
	Fetch(ctx context.Context, uri string) ([]byte, error)
}

// ErrRemoteDisabled is returned when an http(s) import is encountered and
// remote fetching has not been enabled.
var ErrRemoteDisabled = errors.New("remote fetching is disabled (enable it explicitly to resolve http(s) imports)")

// FileFetcher reads file:// URIs from disk and, when AllowRemote is set,
// http(s) URIs over the network. A missing file "x.json" falls back to
// "x.json.gz" so compressed copies of large catalogs can stand in for the
// originals.
type FileFetcher struct {
	AllowRemote bool
	Client      *http.Client
}

// Fetch implements Fetcher.
func (f FileFetcher) Fetch(ctx context.Context, uri string) ([]byte, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("parse uri %q: %w", uri, err)
	}
	switch u.Scheme {
	case "file":
		return readLocal(filepath.FromSlash(u.Path))
	case "http", "https":
		if !f.AllowRemote {
			return nil, fmt.Errorf("%s: %w", uri, ErrRemoteDisabled)
		}
		return f.fetchHTTP(ctx, uri)
	}
	return nil, fmt.Errorf("unsupported uri scheme %q in %s", u.Scheme, uri)
}

func readLocal(path string) ([]byte, error) {
	data, err := readCapped(path)
	if err == nil {
		return data, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		if gz, gzErr := readCapped(path + ".gz"); gzErr == nil {
			return gz, nil
		}
	}
	return nil, err
}

func readCapped(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(io.LimitReader(f, oscal.MaxDocumentBytes+1))
}

func (f FileFetcher) fetchHTTP(ctx context.Context, uri string) ([]byte, error) {
	client := f.Client
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", uri, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, oscal.MaxDocumentBytes+1))
}

// MapFetcher serves documents from memory, keyed by absolute URI. It is
// useful for tests and for embedding fixed content.
type MapFetcher map[string][]byte

// Fetch implements Fetcher.
func (m MapFetcher) Fetch(_ context.Context, uri string) ([]byte, error) {
	if b, ok := m[uri]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("%s: %w", uri, fs.ErrNotExist)
}

// FileURI converts a filesystem path into an absolute file:// URI.
func FileURI(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}).String(), nil
}
