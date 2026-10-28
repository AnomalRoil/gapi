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
)

func init() {
	flag.Usage = func() {
		_, _ = fmt.Fprintf(flag.CommandLine.Output(), `Usage: %s [OPTION]... [SOURCE]

Checks the public API of the Go module in SOURCE (default ".") against the
API declared in SOURCE/api/v*.txt, SOURCE/api/next/*.txt and SOURCE/api/except.txt.

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

	if *nextFile == "" {
		return api.Check(path, features)
	}

	added, removed, err := api.Diff(path, features)
	if err != nil {
		return err
	}
	if len(added) > 0 {
		file := *nextFile
		if !filepath.IsAbs(file) {
			file = filepath.Join(path, file)
		}
		if err := appendLines(file, added); err != nil {
			return err
		}
		log.Printf("Declared %d new APIs in %s", len(added), file)
	}
	for _, f := range removed {
		fmt.Printf("-%s\n", f)
	}
	if len(removed) > 0 {
		return fmt.Errorf("API differences found: %d removed (-)", len(removed))
	}
	return nil
}

// load type-checks all the packages of the module in path and records their API.
func load(path string) error {
	cfg := &packages.Config{
		Mode:  packages.NeedName | packages.NeedTypes,
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
