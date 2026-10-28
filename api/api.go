// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package api computes the exported API of a set of Go packages and compares
// it against the API declared in an "api" folder, the same way the Go project
// guards its own compatibility promise.
//
// Declared API files live in DIR/api:
//   - v*.txt: the API of released versions (e.g. api/v1.2.0.txt),
//   - next/*.txt: new APIs accepted for the upcoming release,
//   - except.txt: features that were removed or changed on purpose.
//
// Each line is one feature, as printed by List. Blank lines are not allowed
// and lines starting with '#' are comments.
package api

import (
	"bytes"
	"errors"
	"fmt"
	"go/token"
	"go/types"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	features = make(map[string]bool)
	// Verbose enables logging of every package and feature encountered.
	Verbose bool
)

// List returns the sorted list of all features recorded by Export so far.
func List() (fs []string) {
	for f := range features {
		fs = append(fs, f)
	}
	sort.Strings(fs)
	return
}

// Diff compares the given features against the API declared in path/api.
// It returns the features that are not declared yet (added) and the declared
// features that are missing and not listed in api/except.txt (removed).
func Diff(path string, list []string) (added, removed []string, err error) {
	required, exception, err := declared(path)
	if err != nil {
		return nil, nil, err
	}
	added, removed = compareAPI(list, required, exception)
	return added, removed, nil
}

// Check compares the given features against the API declared in path/api.
// It prints the differences on stdout, removed features prefixed by "-" and
// undeclared new features prefixed by "+", and returns an error if any.
func Check(path string, list []string) error {
	added, removed, err := Diff(path, list)
	if err != nil {
		return err
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
	if Verbose {
		log.Print("Successful run, no difference with declared APIs detected.")
	}
	return nil
}

// IsInternal reports whether the import path is an internal package.
func IsInternal(pkg string) bool {
	return internalPkg.MatchString(pkg)
}

var internalPkg = regexp.MustCompile(`(^|/)internal($|/)`)

// declared loads the required features (api/v*.txt and api/next/*.txt) and
// the exceptions (api/except.txt) found under path.
func declared(path string) (required, exception []string, err error) {
	var files []string
	for _, pattern := range []string{"api/v*.txt", "api/next/*.txt"} {
		m, err := filepath.Glob(filepath.Join(path, pattern))
		if err != nil {
			return nil, nil, err
		}
		files = append(files, m...)
	}
	if Verbose {
		log.Print("Loaded declaration files: ", files)
	}

	var errs []error
	for _, file := range files {
		fs, err := fileFeatures(file)
		errs = append(errs, err)
		required = append(required, fs...)
	}
	exception, err = fileFeatures(filepath.Join(path, "api", "except.txt"))
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	errs = append(errs, err)
	if err := errors.Join(errs...); err != nil {
		return nil, nil, fmt.Errorf("API database problems found:\n%w", err)
	}
	if Verbose {
		log.Printf("Loaded %d exceptions from api/except.txt file.", len(exception))
	}
	return required, exception, nil
}

// ==========================================================================================
// Code below is adapted from Go's internal src/cmd/api tooling to control its public APIs
// most notable changes are:
//  - removed the notion of approvals
//  - removed the Walkers and the contexts that allowed to test against multiple archs
//  - do not attempt to walk through dependencies

func fileFeatures(filename string) ([]string, error) {
	bs, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	s := string(bs)

	// Diagnose common mistakes people make,
	// since there is no apifmt to format these files.
	// The missing final newline is important for the
	// release step of cat next/*.txt >vX.Y.Z.txt.
	var errs []error
	if strings.Contains(s, "\r") {
		errs = append(errs, fmt.Errorf("%s: contains CRLFs", filename))
	}
	if strings.HasPrefix(s, "\n") || strings.Contains(s, "\n\n") {
		errs = append(errs, fmt.Errorf("%s: contains a blank line", filename))
	}
	if s == "" {
		errs = append(errs, fmt.Errorf("%s: empty file", filename))
	} else if s[len(s)-1] != '\n' {
		errs = append(errs, fmt.Errorf("%s: missing final newline", filename))
	}
	var nonblank []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		nonblank = append(nonblank, line)
	}
	return nonblank, errors.Join(errs...)
}

// Export records the exported features of pkg, to be returned by List.
func Export(pkg *types.Package) {
	if Verbose {
		log.Println(pkg)
	}
	current = pkg
	defer func() { current = nil }()
	pop := pushScope("pkg " + pkg.Path())
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		if token.IsExported(name) {
			emitObj(scope.Lookup(name))
		}
	}
	pop()
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

// sortedEmbeddeds returns the type constraints embedded in an interface,
// such as "~int | ~uint" or "comparable". It does not include methods, nor
// the embedded interfaces themselves, only their own type constraints.
func sortedEmbeddeds(typ *types.Interface) []string {
	n := typ.NumEmbeddeds()
	list := make([]string, 0, n)
	for i := 0; i < n; i++ {
		emb := typ.EmbeddedType(i)
		if iface, ok := emb.Underlying().(*types.Interface); ok {
			if !isComparable(emb) {
				list = append(list, sortedEmbeddeds(iface)...)
				continue
			}
		}
		switch emb := emb.(type) {
		case *types.Union:
			terms := make([]string, emb.Len())
			for i := range terms {
				term := emb.Term(i)
				terms[i] = typeString(term.Type())
				if term.Tilde() {
					terms[i] = "~" + terms[i]
				}
			}
			// The order of the terms of a union does not matter.
			sort.Strings(terms)
			list = append(list, strings.Join(terms, " | "))
		default:
			list = append(list, typeString(emb))
		}
	}
	sort.Strings(list)
	return list
}

// isComparable reports whether typ is the predeclared comparable interface.
func isComparable(typ types.Type) bool {
	return typ == types.Universe.Lookup("comparable").Type()
}

// interfaceElems returns the sorted method names followed by the sorted
// type constraints of an interface.
func interfaceElems(typ *types.Interface) []string {
	return append(sortedMethodNames(typ), sortedEmbeddeds(typ)...)
}

func writeType(buf *bytes.Buffer, typ types.Type) {
	// Aliases are transparent: report the aliased type, so that the output
	// does not depend on the gotypesalias GODEBUG setting.
	switch typ := types.Unalias(typ).(type) {
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
		if elems := interfaceElems(typ); len(elems) > 0 {
			buf.WriteString(" " + strings.Join(elems, ", ") + " ")
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
	emitf("%s", typeStruct)
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

	// Unlike Go's own tool, also record type constraints, since narrowing
	// them is a breaking change.
	sort.Strings(methodNames)
	elems := append(methodNames, sortedEmbeddeds(typ)...)
	if len(elems) == 0 {
		emitf("type %s interface {}", name)
		return
	}
	emitf("type %s interface { %s }", name, strings.Join(elems, ", "))
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
// for mismatched pushes & pops).
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

	if Verbose {
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

func compareAPI(features, required, exception []string) (added, removed []string) {
	featureSet := set(features)
	requiredSet := set(required)
	exceptionSet := set(exception)

	for f := range requiredSet {
		if !featureSet[f] && !exceptionSet[f] {
			// The feature was declared but is gone: compatibility broken,
			// unless acknowledged in api/except.txt.
			removed = append(removed, f)
		}
	}
	for f := range featureSet {
		if !requiredSet[f] {
			// New feature not declared in api/v*.txt nor api/next/*.txt.
			added = append(added, f)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
}
