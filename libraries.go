// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// LibraryImport pins a Pine library version. Resolution is explicit and offline
// unless the host supplies a resolver that performs network or filesystem IO.
type LibraryImport struct{ Path, Alias string }
type LibraryResolver interface {
	ResolveLibrary(path string) (string, error)
}
type LibrarySources map[string]string

func (s LibrarySources) ResolveLibrary(path string) (string, error) {
	source, ok := s[path]
	if !ok {
		return "", fmt.Errorf("library %q is not registered", path)
	}
	return source, nil
}
func (e *Engine) SetLibraryResolver(resolver LibraryResolver) { e.libraryResolver = resolver }

func (p *parser) parseImport() (Stmt, error) {
	p.next()
	author, err := p.expect(tokIdent)
	if err != nil {
		return Stmt{}, err
	}
	if _, err = p.expect(tokDiv); err != nil {
		return Stmt{}, err
	}
	name, err := p.expect(tokIdent)
	if err != nil {
		return Stmt{}, err
	}
	if _, err = p.expect(tokDiv); err != nil {
		return Stmt{}, err
	}
	version, err := p.expect(tokNumber)
	if err != nil {
		return Stmt{}, err
	}
	n, err := strconv.Atoi(version.Text)
	if err != nil || n < 1 {
		return Stmt{}, fmt.Errorf("line %d: import version must be a positive integer", version.Line)
	}
	alias := name.Text
	if p.matchIdent("as") {
		p.next()
		a, err := p.expect(tokIdent)
		if err != nil {
			return Stmt{}, err
		}
		alias = a.Text
	}
	if strings.Contains(alias, ".") || isReservedPineKeyword(alias) || isTypeKeyword(alias) {
		return Stmt{}, fmt.Errorf("invalid library alias %q", alias)
	}
	if !p.match(tokNewline) && !p.match(tokEOF) {
		return Stmt{}, fmt.Errorf("line %d: unexpected token after import", p.peek().Line)
	}
	return Stmt{Import: &LibraryImport{Path: author.Text + "/" + name.Text + "/" + version.Text, Alias: alias}}, nil
}

func (e *Engine) linkLibraries(program *Program) error {
	if !program.Library {
		for _, fn := range program.Functions {
			if fn.Exported {
				return fmt.Errorf("export requires a library declaration")
			}
		}
		for _, td := range program.Types {
			if td.Exported {
				return fmt.Errorf("export requires a library declaration")
			}
		}
	}
	budget := 128
	var prefixStatements []Stmt
	for _, imp := range program.Imports {
		if e.libraryResolver == nil {
			return fmt.Errorf("import %s requires a LibraryResolver", imp.Path)
		}
		if err := checkImportAlias(program, imp.Alias); err != nil {
			return err
		}
		lib, err := e.resolveLibrary(imp.Path, imp.Alias, map[string]bool{}, &budget)
		if err != nil {
			return err
		}
		for name, fn := range lib.Functions {
			if _, exists := program.Functions[name]; exists {
				return fmt.Errorf("duplicate imported function %s", name)
			}
			program.Functions[name] = fn
		}
		for name, td := range lib.Types {
			if _, exists := program.Types[name]; exists {
				return fmt.Errorf("duplicate imported type %s", name)
			}
			program.Types[name] = td
		}
		prefixStatements = append(prefixStatements, lib.Stmts...)
	}
	program.Stmts = append(prefixStatements, program.Stmts...)
	program.Imports = nil
	return nil
}

func checkImportAlias(program *Program, alias string) error {
	for _, s := range program.Stmts {
		if s.Name == alias {
			return fmt.Errorf("library alias %s conflicts with a variable", alias)
		}
	}
	if _, ok := program.Functions[alias]; ok {
		return fmt.Errorf("library alias %s conflicts with a function", alias)
	}
	if _, ok := program.Types[alias]; ok {
		return fmt.Errorf("library alias %s conflicts with a type", alias)
	}
	count := 0
	for _, i := range program.Imports {
		if i.Alias == alias {
			count++
		}
	}
	if count > 1 {
		return fmt.Errorf("duplicate library alias %s", alias)
	}
	for _, root := range []string{"ta", "math", "str", "array", "map", "matrix", "input", "request", "strategy", "syminfo", "barstate", "session", "timeframe", "ticker", "runtime", "color", "line", "label", "box", "table", "chart", "log"} {
		if alias == root {
			return fmt.Errorf("library alias %s conflicts with a built-in namespace", alias)
		}
	}
	return nil
}

func (e *Engine) resolveLibrary(path, prefix string, stack map[string]bool, budget *int) (Program, error) {
	if stack[path] {
		return Program{}, fmt.Errorf("cyclic library import: %s", path)
	}
	if len(stack) >= 32 || *budget <= 0 {
		return Program{}, fmt.Errorf("library import limit exceeded")
	}
	*budget--
	stack[path] = true
	defer delete(stack, path)
	source, err := e.libraryResolver.ResolveLibrary(path)
	if err != nil {
		return Program{}, fmt.Errorf("resolve library %s: %w", path, err)
	}
	if len(source) > 4<<20 {
		return Program{}, fmt.Errorf("library %s exceeds 4 MiB", path)
	}
	lib, err := parseProgram(normalizePineScriptCompat(source))
	if err != nil {
		return Program{}, fmt.Errorf("parse library %s: %w", path, err)
	}
	if !lib.Library {
		return Program{}, fmt.Errorf("import %s has no library declaration", path)
	}
	lowerProgram(&lib)
	out := Program{Functions: map[string]FunctionDef{}, Types: map[string]TypeDef{}}
	renames := map[string]string{}
	internal := "__pine_lib_" + hex.EncodeToString([]byte(prefix)) + "_"
	for name, fn := range lib.Functions {
		target := internal + name
		if fn.Exported {
			target = prefix + "." + name
		}
		renames[name] = target
	}
	for name, td := range lib.Types {
		target := internal + name
		if td.Exported {
			target = prefix + "." + name
		}
		renames[name] = target
	}
	env := inputConstants(lib)
	for _, stmt := range lib.Stmts {
		if stmt.Kind == "expr" && stmt.Expr != nil && stmt.Expr.Kind == "call" && stmt.Expr.Left != nil && stmt.Expr.Left.Name == "library" {
			continue
		}
		if stmt.Kind != "decl" && stmt.Kind != "assign" {
			return Program{}, fmt.Errorf("library %s has executable global statements", path)
		}
		value, err := constantParameter(stmt.Expr, env)
		if err != nil {
			return Program{}, fmt.Errorf("library %s global %s must be constant: %w", path, stmt.Name, err)
		}
		switch value.(type) {
		case nil, bool, string, float64, int, EnumValue:
		default:
			return Program{}, fmt.Errorf("library %s global %s must be a scalar constant", path, stmt.Name)
		}
		env[stmt.Name] = value
		renames[stmt.Name] = internal + stmt.Name
	}
	for _, imp := range lib.Imports {
		if err := checkImportAlias(&lib, imp.Alias); err != nil {
			return Program{}, err
		}
		childPrefix := internal + "dependency_" + imp.Alias
		child, err := e.resolveLibrary(imp.Path, childPrefix, stack, budget)
		if err != nil {
			return Program{}, err
		}
		renames[imp.Alias] = childPrefix
		for n, f := range child.Functions {
			out.Functions[n] = f
		}
		for n, t := range child.Types {
			out.Types[n] = t
		}
		out.Stmts = append(out.Stmts, child.Stmts...)
	}
	keys := make([]string, 0, len(lib.Functions))
	for k := range lib.Functions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, name := range keys {
		fn := lib.Functions[name]
		fn.Name = renames[name]
		locals := map[string]bool{}
		for _, p := range fn.Params {
			locals[p] = true
		}
		for _, expr := range fn.ParamDefaults {
			renameLibraryExpr(expr, renames, locals)
		}
		renameLibraryExpr(fn.Expr, renames, locals)
		renameLibraryStmts(fn.Body, renames, locals)
		out.Functions[fn.Name] = fn
	}
	for name, td := range lib.Types {
		td.Name = renames[name]
		for i := range td.Fields {
			td.Fields[i].TypeName = qualifiedLibraryName(td.Fields[i].TypeName, renames, nil)
			renameLibraryExpr(td.Fields[i].Default, renames, nil)
		}
		out.Types[td.Name] = td
	}
	for _, stmt := range lib.Stmts {
		if stmt.Kind != "decl" && stmt.Kind != "assign" {
			continue
		}
		stmt.Name = renames[stmt.Name]
		stmt.Kind = "decl"
		stmt.Const = true
		renameLibraryExpr(stmt.Expr, renames, nil)
		out.Stmts = append(out.Stmts, stmt)
	}
	return out, nil
}

func qualifiedLibraryName(name string, renames map[string]string, locals map[string]bool) string {
	root := strings.SplitN(name, ".", 2)[0]
	if locals[root] {
		return name
	}
	if target, ok := renames[name]; ok {
		return target
	}
	if target, ok := renames[root]; ok {
		return target + strings.TrimPrefix(name, root)
	}
	return name
}
func copyLibraryLocals(locals map[string]bool) map[string]bool {
	out := map[string]bool{}
	for k, v := range locals {
		out[k] = v
	}
	return out
}

func renameLibraryExpr(expr *Expr, renames map[string]string, locals map[string]bool) {
	if expr == nil {
		return
	}
	if expr.Kind == "ident" {
		expr.Name = qualifiedLibraryName(expr.Name, renames, locals)
	}
	renameLibraryExpr(expr.Left, renames, locals)
	renameLibraryExpr(expr.Right, renames, locals)
	renameLibraryExpr(expr.Else, renames, locals)
	for _, a := range expr.Args {
		renameLibraryExpr(a, renames, locals)
	}
	for _, a := range expr.Elems {
		renameLibraryExpr(a, renames, locals)
	}
	renameLibraryExpr(expr.SwitchExpr, renames, locals)
	for i := range expr.Cases {
		renameLibraryExpr(expr.Cases[i].Match, renames, locals)
		renameLibraryStmts(expr.Cases[i].Body, renames, copyLibraryLocals(locals))
	}
	renameLibraryStmts(expr.Default, renames, copyLibraryLocals(locals))
}
func renameLibraryStmts(stmts []Stmt, renames map[string]string, locals map[string]bool) {
	for i := range stmts {
		s := &stmts[i]
		if s.Name != "" {
			locals[strings.SplitN(s.Name, ".", 2)[0]] = true
		}
		for _, name := range s.TupleNames {
			locals[name] = true
		}
		s.TypeName = qualifiedLibraryName(s.TypeName, renames, nil)
		for _, expr := range []*Expr{s.Expr, s.Target, s.Cond, s.From, s.To, s.By, s.ForIn, s.SwitchExpr} {
			renameLibraryExpr(expr, renames, locals)
		}
		bodyLocals := copyLibraryLocals(locals)
		if s.ForVar != "" {
			bodyLocals[s.ForVar] = true
		}
		renameLibraryStmts(s.Body, renames, bodyLocals)
		renameLibraryStmts(s.Then, renames, copyLibraryLocals(locals))
		renameLibraryStmts(s.Else, renames, copyLibraryLocals(locals))
		renameLibraryStmts(s.Default, renames, copyLibraryLocals(locals))
		for j := range s.Cases {
			renameLibraryExpr(s.Cases[j].Match, renames, locals)
			renameLibraryStmts(s.Cases[j].Body, renames, copyLibraryLocals(locals))
		}
	}
}
