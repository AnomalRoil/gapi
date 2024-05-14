package main

import (
	"flag"
	"log"
	"os"

	"github.com/anomalroil/gapi/api"
	"golang.org/x/tools/go/packages"
)

var verbose = flag.Bool("verbose", false, "Prints more informations, as well as all encountered APIs")

func init() {
	flag.Parse()
	api.Verbose = *verbose
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("please provide a path to a Go codebase as argument")
	}
	path := os.Args[1]
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
				log.Println("skipping internal pkg", pkg)
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
	api.Check(path, list)

}
