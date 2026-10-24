package resolve

import (
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestFileFetcher(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "a.json")
	if err := os.WriteFile(plain, []byte(`{"x":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, "b.json.gz"))
	if err != nil {
		t.Fatal(err)
	}
	zw := gzip.NewWriter(f)
	_, _ = zw.Write([]byte(`{"y":2}`))
	_ = zw.Close()
	_ = f.Close()

	ff := FileFetcher{}
	ctx := context.Background()
	uri, _ := FileURI(plain)
	if b, err := ff.Fetch(ctx, uri); err != nil || string(b) != `{"x":1}` {
		t.Fatalf("plain fetch: %q %v", b, err)
	}
	uri, _ = FileURI(filepath.Join(dir, "b.json"))
	if b, err := ff.Fetch(ctx, uri); err != nil || len(b) < 2 || b[0] != 0x1f {
		t.Fatalf("gz fallback should return the compressed bytes: %v", err)
	}
	uri, _ = FileURI(filepath.Join(dir, "missing.json"))
	if _, err := ff.Fetch(ctx, uri); err == nil {
		t.Fatal("missing file must fail")
	}
	if _, err := ff.Fetch(ctx, "https://example.com/x.json"); !errors.Is(err, ErrRemoteDisabled) {
		t.Fatalf("remote must be disabled by default: %v", err)
	}
	if _, err := ff.Fetch(ctx, "ftp://example.com/x.json"); err == nil {
		t.Fatal("unsupported scheme must fail")
	}
	if _, err := ff.Fetch(ctx, "%zz"); err == nil {
		t.Fatal("bad uri must fail")
	}
}

func TestFileFetcherHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok.json" {
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	ff := FileFetcher{AllowRemote: true, Client: srv.Client()}
	b, err := ff.Fetch(context.Background(), srv.URL+"/ok.json")
	if err != nil || !strings.Contains(string(b), "ok") {
		t.Fatalf("http fetch: %q %v", b, err)
	}
	if _, err := ff.Fetch(context.Background(), srv.URL+"/missing.json"); err == nil {
		t.Fatal("404 must fail")
	}
}

func TestResolveFileRejectsNonProfiles(t *testing.T) {
	catalog := filepath.Join(nistDir, "NIST_SP-800-53_rev5_LOW-baseline-resolved-profile_catalog.json")
	if _, err := ResolveFile(context.Background(), catalog, fixedOptions()); err == nil || !strings.Contains(err.Error(), "not a profile") {
		t.Fatalf("want not-a-profile error, got %v", err)
	}
	if _, err := ResolveFile(context.Background(), "does-not-exist.json", fixedOptions()); err == nil {
		t.Fatal("missing profile must fail")
	}
}

func TestGlobMatch(t *testing.T) {
	cases := []struct {
		p, s string
		want bool
	}{
		{"ac-*", "ac-2", true},
		{"ac-*", "ac-2.1", true},
		{"ac-?", "ac-2", true},
		{"ac-?", "ac-12", false},
		{"*", "", true},
		{"", "", true},
		{"", "a", false},
		{"a*b*c", "aXXbYYc", true},
		{"a*b*c", "aXXbYY", false},
		{"*.1", "ac-2.1", true},
		{"**x", "x", true},
		{"ü?", "üß", true},
	}
	for _, c := range cases {
		if got := globMatch(c.p, c.s); got != c.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", c.p, c.s, got, c.want)
		}
	}
}

// FuzzGlobMatch checks the iterative matcher against a regexp oracle.
func FuzzGlobMatch(f *testing.F) {
	f.Add("ac-*", "ac-2.1")
	f.Add("a?c*", "abcd")
	f.Add("*.*", "x.y")
	f.Fuzz(func(t *testing.T, pattern, s string) {
		if len(pattern) > 64 || len(s) > 64 || !utf8Valid(pattern) || !utf8Valid(s) {
			return
		}
		var re strings.Builder
		re.WriteString(`^(?s)`)
		for _, r := range pattern {
			switch r {
			case '*':
				re.WriteString(".*")
			case '?':
				re.WriteString(".")
			default:
				re.WriteString(regexp.QuoteMeta(string(r)))
			}
		}
		re.WriteString("$")
		want := regexp.MustCompile(re.String()).MatchString(s)
		if got := globMatch(pattern, s); got != want {
			t.Fatalf("globMatch(%q, %q) = %v, oracle %v", pattern, s, got, want)
		}
	})
}

func utf8Valid(s string) bool { return strings.ToValidUTF8(s, "�") == s }
