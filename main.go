// Command gapi lists the public API of a Go module and checks it against the
// API declared in its api folder, to catch unintended API changes.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/AnomalRoil/gapi/api"
	"golang.org/x/tools/go/packages"
)

var (
	verbose         = flag.Bool("verbose", false, "Print more information, as well as all encountered APIs")
	includeInternal = flag.Bool("internal", false, "Include internal packages")
	printAPI        = flag.Bool("print", false, "Print the current API on stdout instead of checking it, e.g. gapi -print . > api/v1.0.0.txt")
	nextFile        = flag.String("next", "", "Append undeclared new APIs to this `file`, relative to SOURCE (e.g. api/next/feature.txt), instead of failing on them")
	apiDir          = flag.String("api", defaultAPIDir(), "API folder `dir`, relative to SOURCE; defaults to $GAPI_FOLDER if set")
)

func defaultAPIDir() string {
	if dir := os.Getenv("GAPI_FOLDER"); dir != "" {
		return dir
	}
	return "api"
}

func init() {
	flag.Usage = func() {
		_, _ = fmt.Fprintf(flag.CommandLine.Output(), `Usage: %s [OPTION]... [SOURCE]

Checks the public API of the Go module in SOURCE (default ".") against the
API declared in the API folder (default SOURCE/api): its v*.txt, next/*.txt
and except.txt files.

Options:
`, os.Args[0])
		flag.PrintDefaults()
	}
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("gapi: ")
	flag.Parse()
	api.Verbose = *verbose

	if flag.NArg() > 1 {
		flag.Usage()
		os.Exit(2)
	}
	path := "."
	if flag.NArg() == 1 {
		path = flag.Arg(0)
	}

	if err := run(path); err != nil {
		log.Fatal(err)
	}
}

func run(path string) error {
	if err := load(path); err != nil {
		return err
	}
	features := api.List()

	if *printAPI {
		for _, f := range features {
			fmt.Println(f)
		}
		return nil
	}

	added, removed, err := api.Diff(underSource(path, *apiDir), features)
	if err != nil {
		return err
	}
	if *nextFile != "" && len(added) > 0 {
		file := underSource(path, *nextFile)
		if err := appendLines(file, added); err != nil {
			return err
		}
		log.Printf("Declared %d new APIs in %s", len(added), file)
		added = nil
	}

	for _, f := range removed {
		fmt.Printf("-%s\n", f)
	}
	for _, f := range added {
		fmt.Printf("+%s\n", f)
	}
	switch {
	case len(removed) > 0 && len(added) > 0:
		return fmt.Errorf("API differences found: %d removed (-), %d undeclared (+)", len(removed), len(added))
	case len(removed) > 0:
		return fmt.Errorf("API differences found: %d removed (-)", len(removed))
	case len(added) > 0:
		return fmt.Errorf("API differences found: %d undeclared (+)", len(added))
	}
	if *verbose {
		log.Print("Successful run, no difference with declared APIs detected.")
	}
	return nil
}

// underSource resolves file relative to the SOURCE path, unless it is absolute.
func underSource(path, file string) string {
	if filepath.IsAbs(file) {
		return file
	}
	return filepath.Join(path, file)
}

// load type-checks all the packages of the module in path and records their API.
func load(path string) error {
	cfg := &packages.Config{
		// Type-check everything from source (NeedDeps) instead of reading the
		// compiler's export data, whose format changes with Go releases and
		// would require a newer golang.org/x/tools to decode.
		Mode:  packages.NeedName | packages.NeedTypes | packages.NeedSyntax | packages.NeedImports | packages.NeedDeps,
		Dir:   path,
		Tests: false,
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return fmt.Errorf("failed to load packages: %w", err)
	}
	// An incomplete package would show up as a removed API: fail instead.
	if packages.PrintErrors(pkgs) > 0 {
		return errors.New("failed to load packages, aborting")
	}
	if len(pkgs) == 0 {
		return fmt.Errorf("no Go packages found in %s", path)
	}

	for _, pkg := range pkgs {
		switch {
		case pkg.Name == "main":
			// Commands cannot be imported, they have no public API.
			continue
		case !*includeInternal && api.IsInternal(pkg.PkgPath):
			if *verbose {
				log.Println("Skipping internal pkg", pkg)
			}
			continue
		}
		api.Export(pkg.Types)
	}
	return nil
}

func appendLines(file string, lines []string) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, err = f.WriteString(strings.Join(lines, "\n") + "\n")
	return errors.Join(err, f.Close())
}
