package api

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

const testMod = "testdata/mod"

// loadTestMod exports the API of the test module, without internal packages.
func loadTestMod(t *testing.T) []string {
	t.Helper()
	features = make(map[string]bool)
	t.Cleanup(func() { features = make(map[string]bool) })

	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedTypes, Dir: testMod}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		t.Fatal(err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		t.Fatal("package loading errors")
	}
	for _, pkg := range pkgs {
		if !IsInternal(pkg.PkgPath) {
			Export(pkg.Types)
		}
	}
	return List()
}

func TestExport(t *testing.T) {
	want := []string{
		"pkg example.com/mod/p, const Answer = 42",
		"pkg example.com/mod/p, const Answer ideal-int",
		"pkg example.com/mod/p, const Pi = 3.14  // 7070651414971679/2251799813685248",
		"pkg example.com/mod/p, const Pi float64",
		"pkg example.com/mod/p, func Events() <-chan Option",
		"pkg example.com/mod/p, func Keys[$0 Key, $1 interface{}](map[$0]$1) []$0",
		"pkg example.com/mod/p, func NewClient(string, ...Option) (*Client, error)",
		"pkg example.com/mod/p, func Sum[$0 Number]([]$0) $0",
		"pkg example.com/mod/p, method (*Client) Close() error",
		"pkg example.com/mod/p, method (Client) Read([]uint8) (int, error)",
		"pkg example.com/mod/p, method (Client) String() string",
		"pkg example.com/mod/p, method (Set[$0]) Has($0) bool",
		"pkg example.com/mod/p, type Alias = Client",
		"pkg example.com/mod/p, type Client struct",
		"pkg example.com/mod/p, type Client struct, Name string",
		"pkg example.com/mod/p, type Client struct, embedded io.Reader",
		"pkg example.com/mod/p, type Ints interface { int, ~int | ~uint }",
		"pkg example.com/mod/p, type Key interface { String, comparable }",
		"pkg example.com/mod/p, type Key interface, String() string",
		"pkg example.com/mod/p, type Number interface { ~float64 | ~int }",
		"pkg example.com/mod/p, type Open interface { Get, Set }",
		"pkg example.com/mod/p, type Open interface, Get() string",
		"pkg example.com/mod/p, type Open interface, Set(string)",
		"pkg example.com/mod/p, type Option func(*Client)",
		"pkg example.com/mod/p, type Sealed interface, Do(int) bool",
		"pkg example.com/mod/p, type Sealed interface, unexported methods",
		"pkg example.com/mod/p, type Set[$0 comparable] map[$0]struct",
		"pkg example.com/mod/p, var Default *Client",
	}
	got := loadTestMod(t)
	if !slices.Equal(got, want) {
		t.Errorf("unexpected API:\ngot:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestDiff(t *testing.T) {
	added, removed, err := Diff(testMod, loadTestMod(t))
	if err != nil {
		t.Fatal(err)
	}
	// Declared in v1.0.0.txt, gone, and not in except.txt.
	if want := []string{"pkg example.com/mod/p, func Removed()"}; !slices.Equal(removed, want) {
		t.Errorf("removed = %q, want %q", removed, want)
	}
	// Answer and Events are declared (in v1.0.0.txt and next/), the rest is new.
	for _, f := range added {
		if strings.Contains(f, "Answer =") || strings.Contains(f, "Events") {
			t.Errorf("declared feature reported as added: %q", f)
		}
	}
	if len(added) != 26 {
		t.Errorf("got %d added features, want 26", len(added))
	}
}

func TestCheck(t *testing.T) {
	if err := Check(testMod, loadTestMod(t)); err == nil {
		t.Fatal("expected API differences")
	}
}

func TestFileFeatures(t *testing.T) {
	for name, tc := range map[string]struct {
		content string
		want    []string
		err     string
	}{
		"ok":         {"# comment\na\nb\n", []string{"a", "b"}, ""},
		"crlf":       {"a\r\n", []string{"a"}, "contains CRLFs"},
		"blank":      {"a\n\nb\n", []string{"a", "b"}, "contains a blank line"},
		"leading":    {"\na\n", []string{"a"}, "contains a blank line"},
		"no newline": {"a", []string{"a"}, "missing final newline"},
		"empty":      {"", nil, "empty file"},
	} {
		t.Run(name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "v1.txt")
			if err := os.WriteFile(file, []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := fileFeatures(file)
			if !slices.Equal(got, tc.want) {
				t.Errorf("got %q, want %q", got, tc.want)
			}
			switch {
			case tc.err == "" && err != nil:
				t.Errorf("unexpected error: %v", err)
			case tc.err != "" && (err == nil || !strings.Contains(err.Error(), tc.err)):
				t.Errorf("error = %v, want %q", err, tc.err)
			}
		})
	}
}

func TestDeclaredBadFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "api", "v1.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Diff(dir, []string{"a"}); err == nil {
		t.Fatal("expected error on malformed API file")
	}
}

func TestIsInternal(t *testing.T) {
	for pkg, want := range map[string]bool{
		"internal":       true,
		"a/internal":     true,
		"a/internal/b":   true,
		"a/internalized": false,
		"a/b":            false,
		"myinternal/b":   false,
	} {
		if got := IsInternal(pkg); got != want {
			t.Errorf("IsInternal(%q) = %v, want %v", pkg, got, want)
		}
	}
}
