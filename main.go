package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

const verbose = false

var features map[string]bool
var parsedFileCache = make(map[string]*ast.File)

func init() {
	features = make(map[string]bool)
}

func list() (fs []string) {
	for f := range features {
		fs = append(fs, f)
	}
	sort.Strings(fs)
	return
}

func main() {
	if len(os.Args) < 2 {
		os.Args = append(os.Args, "/home/anomalroil/code/drand")
	}
	path := os.Args[1]
	cfg := &packages.Config{
		Mode:  packages.NeedName | packages.NeedTypes | packages.NeedFiles, // Load all data needed to type check
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
		if internalPkg.MatchString(pkg.PkgPath) {
			if verbose {
				log.Println("skipping internal pkg", pkg)
			}
			continue
		}
		filtered = append(filtered, pkg)
	}

	packages.Visit(filtered, func(p *packages.Package) bool {
		return false
	}, func(p *packages.Package) {
		export(p.Types)
	})

	list := list()
	for _, v := range list {
		fmt.Println(v)
	}

}

func goCmd() string {
	var exeSuffix string
	if runtime.GOOS == "windows" {
		exeSuffix = ".exe"
	}
	path := filepath.Join(runtime.GOROOT(), "bin", "go"+exeSuffix)
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return "go"
}

var internalPkg = regexp.MustCompile(`(^|/)internal($|/)`)
var gitFiles = regexp.MustCompile(`(^|/).git($|/)`)

var exitCode = 0

// fitlerFiles will ignore test files and internal ones.
func filterFiles(info fs.FileInfo) bool {
	log.Println("filterFilesr", info)
	if strings.HasSuffix(info.Name(), "_test.go") {
		return false
	}

	return true
}

func callFuncs(pkgPath string) {
	set := token.NewFileSet()
	packs, err := parser.ParseDir(set, pkgPath, filterFiles, 0)
	if err != nil {
		log.Fatal("Failed to parse package:", err)
	}

	for _, pack := range packs {
		ast.PackageExports(pack)
		for _, f := range pack.Files {
			for _, d := range f.Decls {
				switch decl := d.(type) {
				// Handle function declarations
				case *ast.FuncDecl:
					fmt.Printf("Function: (%s) %s\n", f.Name.Name, decl.Name)
				// Handle general declarations (variables, constants, types)
				case *ast.GenDecl:
					// We're only interested in variable declarations here
					if decl.Tok == token.VAR {
						for _, spec := range decl.Specs {
							if vs, ok := spec.(*ast.ValueSpec); ok {
								for _, name := range vs.Names {
									fmt.Printf("Variable: (%s) %s\n", f.Name.Name, name)
								}
							}
						}
					}
				}
			}
		}
	}
}

// contexts are the default contexts which are scanned.
var contexts = []*build.Context{
	//	{GOOS: "linux", GOARCH: "386", CgoEnabled: true},
	//	{GOOS: "linux", GOARCH: "386"},
	//	{GOOS: "linux", GOARCH: "amd64", CgoEnabled: true},
	{GOOS: "linux", GOARCH: "amd64"},
	// {GOOS: "linux", GOARCH: "arm", CgoEnabled: true},
	// {GOOS: "linux", GOARCH: "arm"},
	// {GOOS: "darwin", GOARCH: "amd64", CgoEnabled: true},
	// {GOOS: "darwin", GOARCH: "amd64"},
	// {GOOS: "darwin", GOARCH: "arm64", CgoEnabled: true},
	// {GOOS: "darwin", GOARCH: "arm64"},
	// {GOOS: "windows", GOARCH: "amd64"},
	// {GOOS: "windows", GOARCH: "386"},
	// {GOOS: "freebsd", GOARCH: "386", CgoEnabled: true},
	// {GOOS: "freebsd", GOARCH: "386"},
	// {GOOS: "freebsd", GOARCH: "amd64", CgoEnabled: true},
	// {GOOS: "freebsd", GOARCH: "amd64"},
	// {GOOS: "freebsd", GOARCH: "arm", CgoEnabled: true},
	// {GOOS: "freebsd", GOARCH: "arm"},
	// {GOOS: "freebsd", GOARCH: "arm64", CgoEnabled: true},
	// {GOOS: "freebsd", GOARCH: "arm64"},
	// {GOOS: "freebsd", GOARCH: "riscv64", CgoEnabled: true},
	// {GOOS: "freebsd", GOARCH: "riscv64"},
	// {GOOS: "netbsd", GOARCH: "386", CgoEnabled: true},
	// {GOOS: "netbsd", GOARCH: "386"},
	// {GOOS: "netbsd", GOARCH: "amd64", CgoEnabled: true},
	// {GOOS: "netbsd", GOARCH: "amd64"},
	// {GOOS: "netbsd", GOARCH: "arm", CgoEnabled: true},
	// {GOOS: "netbsd", GOARCH: "arm"},
	// {GOOS: "netbsd", GOARCH: "arm64", CgoEnabled: true},
	// {GOOS: "netbsd", GOARCH: "arm64"},
	// {GOOS: "openbsd", GOARCH: "386", CgoEnabled: true},
	// {GOOS: "openbsd", GOARCH: "386"},
	// {GOOS: "openbsd", GOARCH: "amd64", CgoEnabled: true},
	// {GOOS: "openbsd", GOARCH: "amd64"},
}

func check(root string) {
	checkFiles, err := filepath.Glob(filepath.Join(root, "api/go1*.txt"))
	if err != nil {
		log.Fatal(err)
	}

	var nextFiles []string
	if v := runtime.Version(); strings.Contains(v, "devel") || strings.Contains(v, "beta") {
		next, err := filepath.Glob(filepath.Join(root, "api/next/*.txt"))
		if err != nil {
			log.Fatal(err)
		}
		nextFiles = next
	}

	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err // return error to stop the walking
		}

		// we ignore internal files and packages
		if internalPkg.MatchString(path) {
			return nil
		}
		if d.IsDir() {
			set := token.NewFileSet()
			packs, err := parser.ParseDir(set, path, filterFiles, 0)
			if err != nil {
				log.Fatal("Failed to parse package:", err)
			}

			// Type checking the packages
			conf := types.Config{Importer: importer.Default()}
			for pkgName, pkg := range packs {
				files := make([]*ast.File, 0, len(pkg.Files))

				// Create the []*ast.File slice from map for type-checking
				for _, f := range pkg.Files {
					files = append(files, f)
				}

				// Type-check the package
				fmt.Println(pkgName, set, files)
				pkgTypes, err := conf.Check(pkgName, set, files, nil)
				if err != nil {
					log.Fatal("Type checking failed: ", err)
				}
				export(pkgTypes)
			}
			return nil
		}
		return nil
	})
	if err != nil {
		fmt.Printf("Error walking the directory: %v\n", err)
	}

	bw := bufio.NewWriter(os.Stdout)
	defer bw.Flush()

	var required []string
	for _, file := range checkFiles {
		required = append(required, fileFeatures(file)...)
	}
	for _, file := range nextFiles {
		required = append(required, fileFeatures(file)...)
	}

	//exception := fileFeatures(filepath.Join(root, "api/except.txt"), false)

	if exitCode == 1 {
		log.Fatalf("API database problems found")
	}
	//	if !compareAPI(bw, features, required, exception) {
	//		log.Fatalf("API differences found")
	//	}
}

func fileFeatures(filename string) []string {
	bs, err := os.ReadFile(filename)
	if errors.Is(err, os.ErrNotExist) {
		return []string{}
	} else if err != nil {
		log.Fatal(err)
	}
	s := string(bs)

	// Diagnose common mistakes people make,
	// since there is no apifmt to format these files.
	// The missing final newline is important for the
	// final release step of cat next/*.txt >go1.X.txt.
	// If the files don't end in full lines, the concatenation goes awry.
	if strings.Contains(s, "\r") {
		log.Printf("%s: contains CRLFs", filename)
		exitCode = 1
	}
	if strings.HasPrefix(s, "\n") || strings.Contains(s, "\n\n") {
		log.Printf("%s: contains a blank line", filename)
		exitCode = 1
	}
	if s == "" {
		log.Printf("%s: empty file", filename)
		exitCode = 1
	} else if s[len(s)-1] != '\n' {
		log.Printf("%s: missing final newline", filename)
		exitCode = 1
	}
	lines := strings.Split(s, "\n")
	var nonblank []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		nonblank = append(nonblank, line)
	}
	return nonblank
}

var fset = token.NewFileSet()

// export emits the exported package features.
func export(pkg *types.Package) {
	if verbose {
		log.Println(pkg)
	}
	pop := pushScope("pkg " + pkg.Path())
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		if token.IsExported(name) {
			emitObj(scope.Lookup(name))
		}
	}
	pop()
}

// tagKey returns the tag-based key to use in the pkgCache.
// It is a comma-separated string; the first part is dir, the rest tags.
// The satisfied tags are derived from context but only those that
// matter (the ones listed in the tags argument plus GOOS and GOARCH) are used.
// The tags list, which came from go/build's Package.AllTags,
// is known to be sorted.
func tagKey(dir string, context *build.Context, tags []string) string {
	ctags := map[string]bool{
		context.GOOS:   true,
		context.GOARCH: true,
	}
	if context.CgoEnabled {
		ctags["cgo"] = true
	}
	for _, tag := range context.BuildTags {
		ctags[tag] = true
	}
	// TODO: ReleaseTags (need to load default)
	key := dir

	// explicit on GOOS and GOARCH as global cache will use "all" cached packages for
	// an indirect imported package. See https://github.com/golang/go/issues/21181
	// for more detail.
	tags = append(tags, context.GOOS, context.GOARCH)
	sort.Strings(tags)

	for _, tag := range tags {
		if ctags[tag] {
			key += "," + tag
			ctags[tag] = false
		}
	}
	return key
}

func sortedMethodNames(typ *types.Interface) []string {
	n := typ.NumMethods()
	list := make([]string, n)
	for i := range list {
		list[i] = typ.Method(i).Name()
	}
	sort.Strings(list)
	return list
}

// sortedEmbeddeds returns constraint types embedded in an
// interface. It does not include embedded interface types or methods.
func sortedEmbeddeds(typ *types.Interface) []string {
	n := typ.NumEmbeddeds()
	list := make([]string, 0, n)
	for i := 0; i < n; i++ {
		emb := typ.EmbeddedType(i)
		switch emb := emb.(type) {
		case *types.Interface:
			list = append(list, sortedEmbeddeds(emb)...)
		case *types.Union:
			var buf bytes.Buffer
			nu := emb.Len()
			for i := 0; i < nu; i++ {
				if i > 0 {
					buf.WriteString(" | ")
				}
				term := emb.Term(i)
				if term.Tilde() {
					buf.WriteByte('~')
				}
				writeType(&buf, term.Type())
			}
			list = append(list, buf.String())
		}
	}
	sort.Strings(list)
	return list
}

func writeType(buf *bytes.Buffer, typ types.Type) {
	switch typ := typ.(type) {
	case *types.Basic:
		s := typ.Name()
		switch typ.Kind() {
		case types.UnsafePointer:
			s = "unsafe.Pointer"
		case types.UntypedBool:
			s = "ideal-bool"
		case types.UntypedInt:
			s = "ideal-int"
		case types.UntypedRune:
			s = "ideal-rune"
		case types.UntypedFloat:
			s = "ideal-float"
		case types.UntypedComplex:
			s = "ideal-complex"
		case types.UntypedString:
			s = "ideal-string"
		case types.UntypedNil:
			panic("should never see untyped nil type")
		default:
			switch s {
			case "byte":
				s = "uint8"
			case "rune":
				s = "int32"
			}
		}
		buf.WriteString(s)

	case *types.Array:
		fmt.Fprintf(buf, "[%d]", typ.Len())
		writeType(buf, typ.Elem())

	case *types.Slice:
		buf.WriteString("[]")
		writeType(buf, typ.Elem())

	case *types.Struct:
		buf.WriteString("struct")

	case *types.Pointer:
		buf.WriteByte('*')
		writeType(buf, typ.Elem())

	case *types.Tuple:
		panic("should never see a tuple type")

	case *types.Signature:
		buf.WriteString("func")
		writeSignature(buf, typ)

	case *types.Interface:
		buf.WriteString("interface{")
		if typ.NumMethods() > 0 || typ.NumEmbeddeds() > 0 {
			buf.WriteByte(' ')
		}
		if typ.NumMethods() > 0 {
			buf.WriteString(strings.Join(sortedMethodNames(typ), ", "))
		}
		if typ.NumEmbeddeds() > 0 {
			buf.WriteString(strings.Join(sortedEmbeddeds(typ), ", "))
		}
		if typ.NumMethods() > 0 || typ.NumEmbeddeds() > 0 {
			buf.WriteByte(' ')
		}
		buf.WriteString("}")

	case *types.Map:
		buf.WriteString("map[")
		writeType(buf, typ.Key())
		buf.WriteByte(']')
		writeType(buf, typ.Elem())

	case *types.Chan:
		var s string
		switch typ.Dir() {
		case types.SendOnly:
			s = "chan<- "
		case types.RecvOnly:
			s = "<-chan "
		case types.SendRecv:
			s = "chan "
		default:
			panic("unreachable")
		}
		buf.WriteString(s)
		writeType(buf, typ.Elem())

	case *types.Named:
		obj := typ.Obj()
		pkg := obj.Pkg()
		if pkg != nil && pkg != current {
			buf.WriteString(pkg.Name())
			buf.WriteByte('.')
		}
		buf.WriteString(typ.Obj().Name())

	case *types.TypeParam:
		// Type parameter names may change, so use a placeholder instead.
		fmt.Fprintf(buf, "$%d", typ.Index())

	default:
		panic(fmt.Sprintf("unknown type %T", typ))
	}
}

var current *types.Package

func writeSignature(buf *bytes.Buffer, sig *types.Signature) {
	if tparams := sig.TypeParams(); tparams != nil {
		writeTypeParams(buf, tparams, true)
	}
	writeParams(buf, sig.Params(), sig.Variadic())
	switch res := sig.Results(); res.Len() {
	case 0:
		// nothing to do
	case 1:
		buf.WriteByte(' ')
		writeType(buf, res.At(0).Type())
	default:
		buf.WriteByte(' ')
		writeParams(buf, res, false)
	}
}

func writeTypeParams(buf *bytes.Buffer, tparams *types.TypeParamList, withConstraints bool) {
	buf.WriteByte('[')
	c := tparams.Len()
	for i := 0; i < c; i++ {
		if i > 0 {
			buf.WriteString(", ")
		}
		tp := tparams.At(i)
		writeType(buf, tp)
		if withConstraints {
			buf.WriteByte(' ')
			writeType(buf, tp.Constraint())
		}
	}
	buf.WriteByte(']')
}

func writeParams(buf *bytes.Buffer, t *types.Tuple, variadic bool) {
	buf.WriteByte('(')
	for i, n := 0, t.Len(); i < n; i++ {
		if i > 0 {
			buf.WriteString(", ")
		}
		typ := t.At(i).Type()
		if variadic && i+1 == n {
			buf.WriteString("...")
			typ = typ.(*types.Slice).Elem()
		}
		writeType(buf, typ)
	}
	buf.WriteByte(')')
}

func typeString(typ types.Type) string {
	var buf bytes.Buffer
	writeType(&buf, typ)
	return buf.String()
}

func signatureString(sig *types.Signature) string {
	var buf bytes.Buffer
	writeSignature(&buf, sig)
	return buf.String()
}

func emitObj(obj types.Object) {
	switch obj := obj.(type) {
	case *types.Const:
		emitf("const %s %s", obj.Name(), typeString(obj.Type()))
		x := obj.Val()
		short := x.String()
		exact := x.ExactString()
		if short == exact {
			emitf("const %s = %s", obj.Name(), short)
		} else {
			emitf("const %s = %s  // %s", obj.Name(), short, exact)
		}
	case *types.Var:
		emitf("var %s %s", obj.Name(), typeString(obj.Type()))
	case *types.TypeName:
		emitType(obj)
	case *types.Func:
		emitFunc(obj)
	default:
		panic("unknown object: " + obj.String())
	}
}

func emitType(obj *types.TypeName) {
	name := obj.Name()
	typ := obj.Type()
	if obj.IsAlias() {
		emitf("type %s = %s", name, typeString(typ))
		return
	}
	if tparams := obj.Type().(*types.Named).TypeParams(); tparams != nil {
		var buf bytes.Buffer
		buf.WriteString(name)
		writeTypeParams(&buf, tparams, true)
		name = buf.String()
	}
	switch typ := typ.Underlying().(type) {
	case *types.Struct:
		emitStructType(name, typ)
	case *types.Interface:
		emitIfaceType(name, typ)
		return // methods are handled by emitIfaceType
	default:
		emitf("type %s %s", name, typeString(typ.Underlying()))
	}

	// emit methods with value receiver
	var methodNames map[string]bool
	vset := types.NewMethodSet(typ)
	for i, n := 0, vset.Len(); i < n; i++ {
		m := vset.At(i)
		if m.Obj().Exported() {
			emitMethod(m)
			if methodNames == nil {
				methodNames = make(map[string]bool)
			}
			methodNames[m.Obj().Name()] = true
		}
	}

	// emit methods with pointer receiver; exclude
	// methods that we have emitted already
	// (the method set of *T includes the methods of T)
	pset := types.NewMethodSet(types.NewPointer(typ))
	for i, n := 0, pset.Len(); i < n; i++ {
		m := pset.At(i)
		if m.Obj().Exported() && !methodNames[m.Obj().Name()] {
			emitMethod(m)
		}
	}
}

func emitStructType(name string, typ *types.Struct) {
	typeStruct := fmt.Sprintf("type %s struct", name)
	emitf(typeStruct)
	defer pushScope(typeStruct)()

	for i := 0; i < typ.NumFields(); i++ {
		f := typ.Field(i)
		if !f.Exported() {
			continue
		}
		typ := f.Type()
		if f.Anonymous() {
			emitf("embedded %s", typeString(typ))
			continue
		}
		emitf("%s %s", f.Name(), typeString(typ))
	}
}

func emitIfaceType(name string, typ *types.Interface) {
	pop := pushScope("type " + name + " interface")

	var methodNames []string
	complete := true
	mset := types.NewMethodSet(typ)
	for i, n := 0, mset.Len(); i < n; i++ {
		m := mset.At(i).Obj().(*types.Func)
		if !m.Exported() {
			complete = false
			continue
		}
		methodNames = append(methodNames, m.Name())
		emitf("%s%s", m.Name(), signatureString(m.Type().(*types.Signature)))
	}

	if !complete {
		// The method set has unexported methods, so all the
		// implementations are provided by the same package,
		// so the method set can be extended. Instead of recording
		// the full set of names (below), record only that there were
		// unexported methods. (If the interface shrinks, we will notice
		// because a method signature emitted during the last loop
		// will disappear.)
		emitf("unexported methods")
	}

	pop()

	if !complete {
		return
	}

	if len(methodNames) == 0 {
		emitf("type %s interface {}", name)
		return
	}

	sort.Strings(methodNames)
	emitf("type %s interface { %s }", name, strings.Join(methodNames, ", "))
}

func emitFunc(f *types.Func) {
	sig := f.Type().(*types.Signature)
	if sig.Recv() != nil {
		panic("method considered a regular function: " + f.String())
	}
	emitf("func %s%s", f.Name(), signatureString(sig))
}

func emitMethod(m *types.Selection) {
	sig := m.Type().(*types.Signature)
	recv := sig.Recv().Type()
	// report exported methods with unexported receiver base type
	if true {
		base := recv
		if p, _ := recv.(*types.Pointer); p != nil {
			base = p.Elem()
		}
		if obj := base.(*types.Named).Obj(); !obj.Exported() {
			log.Fatalf("exported method with unexported receiver base type: %s", m)
		}
	}
	tps := ""
	if rtp := sig.RecvTypeParams(); rtp != nil {
		var buf bytes.Buffer
		writeTypeParams(&buf, rtp, false)
		tps = buf.String()
	}
	emitf("method (%s%s) %s%s", typeString(recv), tps, m.Obj().Name(), signatureString(sig))
}

var scope []string

// pushScope enters a new scope (walking a package, type, node, etc)
// and returns a function that will leave the scope (with sanity checking
// for mismatched pushes & pops)
func pushScope(name string) (popFunc func()) {
	scope = append(scope, name)
	return func() {
		if len(scope) == 0 {
			log.Fatalf("attempt to leave scope %q with empty scope list", name)
		}
		if scope[len(scope)-1] != name {
			log.Fatalf("attempt to leave scope %q, but scope is currently %#v", name, scope)
		}
		scope = scope[:len(scope)-1]
	}
}

func emitf(format string, args ...any) {
	f := strings.Join(scope, ", ") + ", " + fmt.Sprintf(format, args...)
	if strings.Contains(f, "\n") {
		panic("feature contains newlines: " + f)
	}

	if _, dup := features[f]; dup {
		panic("duplicate feature inserted: " + f)
	}
	features[f] = true

	if verbose {
		log.Printf("feature: %s", f)
	}
}

func set(items []string) map[string]bool {
	s := make(map[string]bool)
	for _, v := range items {
		s[v] = true
	}
	return s
}

// portRemoved reports whether the given port-specific API feature is
// okay to no longer exist because its port was removed.
func portRemoved(feature string) bool {
	return strings.Contains(feature, "(darwin-386)") ||
		strings.Contains(feature, "(darwin-386-cgo)")
}

var spaceParensRx = regexp.MustCompile(` \(\S+?\)`)

func featureWithoutContext(f string) string {
	if !strings.Contains(f, "(") {
		return f
	}
	return spaceParensRx.ReplaceAllString(f, "")
}

func compareAPI(w io.Writer, features, required, exception []string) (ok bool) {
	ok = true

	featureSet := set(features)
	exceptionSet := set(exception)

	sort.Strings(features)
	sort.Strings(required)

	take := func(sl *[]string) string {
		s := (*sl)[0]
		*sl = (*sl)[1:]
		return s
	}

	for len(features) > 0 || len(required) > 0 {
		switch {
		case len(features) == 0 || (len(required) > 0 && required[0] < features[0]):
			feature := take(&required)
			if exceptionSet[feature] {
				// An "unfortunate" case: the feature was once
				// included in the API (e.g. go1.txt), but was
				// subsequently removed. These are already
				// acknowledged by being in the file
				// "api/except.txt". No need to print them out
				// here.
			} else if portRemoved(feature) {
				// okay.
			} else if featureSet[featureWithoutContext(feature)] {
				// okay.
			} else {
				fmt.Fprintf(w, "-%s\n", feature)
				ok = false // broke compatibility
			}
		case len(required) == 0 || (len(features) > 0 && required[0] > features[0]):
			newFeature := take(&features)
			fmt.Fprintf(w, "+%s\n", newFeature)
			ok = false // feature not in api/next/*
		default:
			take(&required)
			take(&features)
		}
	}

	return ok
}
