// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This package computes the exported API of a set of Go packages.
// It is only a test, not a command, nor a usefully importable package.

package api

import (
	"bufio"
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

var features map[string]bool
var Verbose bool

func init() {
	features = make(map[string]bool)
}

func List() (fs []string) {
	for f := range features {
		fs = append(fs, f)
	}
	sort.Strings(fs)
	return
}

func Check(path string, list []string) error {
	checkFiles, err := filepath.Glob(filepath.Join(path, "api/v*.txt"))
	if err != nil {
		return err
	}

	var nextFiles []string
	next, err := filepath.Glob(filepath.Join(path, "api/next/*.txt"))
	if err != nil {
		return err
	}
	nextFiles = next

	var required []string
	for _, file := range checkFiles {
		required = append(required, fileFeatures(file)...)
	}
	for _, file := range nextFiles {
		required = append(required, fileFeatures(file)...)
	}

	exception := fileFeatures(filepath.Join(path, "api/except.txt"))

	if exitCode == 1 {
		return errors.New("API database problems found")
	}
	if !compareAPI(list, required, exception) {
		return errors.New("API differences found")
	}
	return nil
}

func IsInternal(pkg string) bool {
	return internalPkg.MatchString(pkg)
}

// ==========================================================================================
// Code below is adapted from Go's internal src/api tooling to control its public APIs
// most notable changes are:
//  - removed the notion of approvals
//  - removed the Walkers and the contexts that allowed to test against multiple archs
//  - do not attempt to walk through dependencies

var internalPkg = regexp.MustCompile(`(^|/)internal($|/)`)
var exitCode = 0

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

// export emits the exported package features.
func Export(pkg *types.Package) {
	if Verbose {
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

func compareAPI(features, required, exception []string) (ok bool) {
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()

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
