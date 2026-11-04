package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path"

	"github.com/AnomalRoil/gapi/api"
	"golang.org/x/tools/go/packages"
)

var verbose = flag.Bool("verbose", false, "Prints more informations, as well as all encountered APIs")
var includeInternal = flag.Bool("internal", false, "Include internal packages")
var next = flag.Bool("next", false, "Uses the new APIs in the api/next folder rather")

func init() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "%s [OPTION]... SOURCE\n\nUsage:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Println("Args:")
		fmt.Println("  SOURCE: the path to the root of your module, e.g. `gapi .` (Required)")
	}
	flag.Parse()
	api.Verbose = *verbose
}

func main() {
	target := flag.Arg(0)
	if len(target) < 1 {
		flag.Usage()
		fmt.Println()
		log.Fatal("Please provide a path to a Go codebase as argument.")
	}
	apiBaseFolder := path.Join(target, "api")
	if newPath, ok := os.LookupEnv("GAPI_FOLDER"); ok {
		apiBaseFolder = newPath
	}
	cfg := &packages.Config{
		Mode:  packages.NeedName | packages.NeedTypes,
		Dir:   target,
		Tests: false, // Set to true if test files should be included
	}

	// Load the packages based on the configuration.
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		log.Printf("Failed to load packages: %v", err)
	}

	if packages.PrintErrors(pkgs) > 0 { // Print out any errors encountered
		log.Printf("Encountered package loading errors")
	}

	filtered := pkgs[:0]
	for _, pkg := range pkgs {
		if !*includeInternal && api.IsInternal(pkg.PkgPath) {
			if *verbose {
				log.Println("Skipping internal pkg", pkg)
			}
			continue
		}
		filtered = append(filtered, pkg)
	}

	packages.Visit(
		filtered,
		func(p *packages.Package) bool {
			api.Export(p.Types)
			return false
		},
		nil)

	err = api.Check(apiBaseFolder, api.List(), *next)
	if err != nil {
		log.Fatal(err)
	}
}
