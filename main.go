package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/AnomalRoil/gapi/api"
	"golang.org/x/tools/go/packages"
)

var verbose = flag.Bool("verbose", false, "Prints more informations, as well as all encountered APIs")

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
	path := flag.Arg(0)
	if len(path) < 1 {
		flag.Usage()
		fmt.Println()
		log.Fatal("Please provide a path to a Go codebase as argument.")
	}
	cfg := &packages.Config{
		Mode:  packages.NeedName | packages.NeedTypes,
		Dir:   path,
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
		if api.IsInternal(pkg.PkgPath) {
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

	list := api.List()
	//	for _, v := range list {
	//		fmt.Println(v)
	//	}
	err = api.Check(path, list)
	if err != nil {
		log.Fatal(err)
	}
}
