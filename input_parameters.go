// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"time"
)

// EnumValue identifies an enum member; Title is its user-facing string.
type EnumValue struct{ Type, Name, Title string }

func (v EnumValue) String() string { return v.Title }

// InputSource is a built-in price series selected before execution. Its value
// still follows the selected series on each bar.
type InputSource string

// InputParameter describes a Pine input without running the script or fetching
// market data. ID is its declaration name, or input#N for an anonymous call.
type InputParameter struct {
	ID, Kind, Title, Tooltip, Group, Inline string
	Default                                 interface{}
	Options                                 []interface{}
	Min, Max, Step                          *float64
	Confirm                                 bool
}

// InputResolver supplies decisions once, before any provider or runtime call.
// Missing keys use the script defaults; unknown keys and invalid values fail.
type InputResolver interface {
	ResolveInputs(parameters []InputParameter) (map[string]interface{}, error)
}

func (e *Engine) SetInputResolver(resolver InputResolver) { e.inputResolver = resolver }

// SetInputParameters copies overrides for subsequent executions. Values are
// validated against the compiled parameter definitions before execution.
func (e *Engine) SetInputParameters(values map[string]interface{}) {
	e.inputParameters = make(map[string]interface{}, len(values))
	for k, v := range values {
		e.inputParameters[k] = v
	}
}

// InputParameters discovers definitions without invoking providers, registered
// functions, or the script runtime.
func (e *Engine) InputParameters(bytecode []byte) ([]InputParameter, error) {
	program, err := e.decodeProgramCached(bytecode)
	if err != nil {
		return nil, err
	}
	defs, _, err := discoverInputs(program)
	return defs, err
}

type inputBinding struct {
	value  interface{}
	source *Expr
}
type inputCall struct {
	expr  *Expr
	owner string
}

func inputConstants(program Program) map[string]interface{} {
	env := map[string]interface{}{"na": nil, "display.all": float64(1), "display.none": float64(0),
		"format.price": "price", "format.percent": "percent", "format.volume": "volume"}
	for name, td := range program.Types {
		if td.Enum {
			for _, f := range td.Fields {
				env[name+"."+f.Name] = EnumValue{Type: name, Name: f.Name, Title: f.Title}
			}
		}
	}
	return env
}

func constantParameter(expr *Expr, env map[string]interface{}) (interface{}, error) {
	if expr == nil {
		return nil, nil
	}
	switch expr.Kind {
	case "number":
		return expr.Number, nil
	case "string":
		return expr.String, nil
	case "bool":
		return expr.Bool, nil
	case "ident":
		if v, ok := env[expr.Name]; ok {
			return v, nil
		}
		if v, ok := mathIdentifierValue(expr.Name); ok {
			return v, nil
		}
	case "array", "tuple":
		out := make([]interface{}, len(expr.Elems))
		for i, elem := range expr.Elems {
			v, err := constantParameter(elem, env)
			if err != nil {
				return nil, err
			}
			out[i] = v
		}
		return out, nil
	case "binary":
		l, err := constantParameter(expr.Left, env)
		if err != nil {
			return nil, err
		}
		r, err := constantParameter(expr.Right, env)
		if err != nil {
			return nil, err
		}
		return evalBinary(expr.BOp, l, r)
	case "unary":
		v, err := constantParameter(expr.Right, env)
		if err != nil {
			return nil, err
		}
		if expr.Op == "not" {
			return !truthy(v), nil
		}
		if n, ok := toFloat(v); ok {
			if expr.Op == "-" {
				n = -n
			}
			return n, nil
		}
	case "ternary":
		v, err := constantParameter(expr.Left, env)
		if err != nil {
			return nil, err
		}
		if truthy(v) {
			return constantParameter(expr.Right, env)
		}
		return constantParameter(expr.Else, env)
	}
	return nil, fmt.Errorf("input metadata must be constant (found %s %s)", expr.Kind, expr.Name)
}

func inputParameterNames(kind string) []string {
	if kind == "int" || kind == "float" {
		return []string{"defval", "title", "minval", "maxval", "step", "tooltip", "inline", "group", "confirm", "display", "active"}
	}
	if kind == "string" || kind == "enum" || kind == "timeframe" {
		return []string{"defval", "title", "options", "tooltip", "inline", "group", "confirm", "display", "active"}
	}
	return []string{"defval", "title", "tooltip", "inline", "group", "confirm", "display", "active"}
}

func discoverInputs(program Program) ([]InputParameter, []inputCall, error) {
	var calls []inputCall
	env := inputConstants(program)
	var walkExpr func(*Expr, string)
	var walkStmts func([]Stmt)
	walkExpr = func(expr *Expr, owner string) {
		if expr == nil {
			return
		}
		if expr.Kind == "call" && expr.Left != nil && (expr.Left.Name == "input" || strings.HasPrefix(expr.Left.Name, "input.")) {
			calls = append(calls, inputCall{expr, owner})
			return
		}
		walkExpr(expr.Left, "")
		walkExpr(expr.Right, "")
		walkExpr(expr.Else, "")
		for _, arg := range expr.Args {
			walkExpr(arg, "")
		}
		for _, elem := range expr.Elems {
			walkExpr(elem, "")
		}
		walkExpr(expr.SwitchExpr, "")
		for _, c := range expr.Cases {
			walkExpr(c.Match, "")
			walkStmts(c.Body)
		}
		walkStmts(expr.Default)
	}
	walkStmts = func(stmts []Stmt) {
		for _, stmt := range stmts {
			owner := ""
			if stmt.Kind == "decl" || stmt.Kind == "assign" {
				owner = stmt.Name
			}
			walkExpr(stmt.Expr, owner)
			walkExpr(stmt.Cond, "")
			walkExpr(stmt.Target, "")
			walkExpr(stmt.From, "")
			walkExpr(stmt.To, "")
			walkExpr(stmt.By, "")
			walkExpr(stmt.ForIn, "")
			walkExpr(stmt.SwitchExpr, "")
			walkStmts(stmt.Then)
			walkStmts(stmt.Else)
			walkStmts(stmt.Body)
			walkStmts(stmt.Default)
			for _, c := range stmt.Cases {
				walkExpr(c.Match, "")
				walkStmts(c.Body)
			}
			if stmt.Kind == "decl" && stmt.Const {
				if v, err := constantParameter(stmt.Expr, env); err == nil {
					env[stmt.Name] = v
				}
			}
		}
	}
	walkStmts(program.Stmts)
	names := make([]string, 0, len(program.Functions))
	for name := range program.Functions {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fn := program.Functions[name]
		walkExpr(fn.Expr, "")
		walkStmts(fn.Body)
	}
	defs := make([]InputParameter, 0, len(calls))
	used := map[string]bool{}
	for i, call := range calls {
		kind := strings.TrimPrefix(call.expr.Left.Name, "input.")
		switch kind {
		case "input", "int", "float", "bool", "string", "color", "time", "session", "symbol", "source", "price", "text_area", "timeframe", "enum":
		default:
			return nil, nil, fmt.Errorf("unknown input kind %s", kind)
		}
		id := call.owner
		if id == "" || used[id] {
			id = fmt.Sprintf("input#%d", i+1)
		}
		used[id] = true
		params := inputParameterNames(kind)
		if (kind == "int" || kind == "float") && len(call.expr.Args) > 2 && call.expr.Args[2].Kind == "array" {
			params = inputParameterNames("string")
		}
		values := map[string]interface{}{}
		pos := 0
		for _, arg := range call.expr.Args {
			key := ""
			expr := arg
			if arg.Kind == "named_arg" {
				key, expr = arg.Name, arg.NamedArgValue()
			} else {
				if pos >= len(params) {
					return nil, nil, fmt.Errorf("input %s has too many arguments", id)
				}
				key = params[pos]
				pos++
			}
			known := false
			if (kind == "int" || kind == "float") && key == "options" {
				known = true
			}
			for _, p := range params {
				if p == key {
					known = true
				}
			}
			if !known {
				return nil, nil, fmt.Errorf("input %s unknown parameter %s", id, key)
			}
			if _, exists := values[key]; exists {
				return nil, nil, fmt.Errorf("input %s duplicate parameter %s", id, key)
			}
			var v interface{}
			var err error
			if key == "defval" && kind == "source" && expr != nil && expr.Kind == "ident" && isPriceIdentifierName(expr.Name) {
				v = InputSource(expr.Name)
			} else {
				v, err = constantParameter(expr, env)
			}
			if err != nil {
				return nil, nil, fmt.Errorf("input %s %s: %w", id, key, err)
			}
			values[key] = v
		}
		defval, exists := values["defval"]
		if !exists {
			return nil, nil, fmt.Errorf("input %s requires defval", id)
		}
		if kind == "input" {
			switch defval.(type) {
			case bool:
				kind = "bool"
			case string:
				kind = "string"
			default:
				kind = "float"
			}
		}
		d := InputParameter{ID: id, Kind: kind, Default: defval, Title: id}
		for key, dst := range map[string]*string{"title": &d.Title, "tooltip": &d.Tooltip, "inline": &d.Inline, "group": &d.Group} {
			if v, ok := values[key]; ok {
				s, ok := v.(string)
				if !ok {
					return nil, nil, fmt.Errorf("input %s %s must be string", id, key)
				}
				*dst = s
			}
		}
		for key, dst := range map[string]**float64{"minval": &d.Min, "maxval": &d.Max, "step": &d.Step} {
			if v, ok := values[key]; ok {
				n, ok := toFloat(v)
				if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
					return nil, nil, fmt.Errorf("input %s invalid %s", id, key)
				}
				*dst = &n
			}
		}
		if d.Min != nil && d.Max != nil && *d.Min > *d.Max {
			return nil, nil, fmt.Errorf("input %s minval exceeds maxval", id)
		}
		if d.Step != nil && *d.Step <= 0 {
			return nil, nil, fmt.Errorf("input %s step must be positive", id)
		}
		if v, ok := values["confirm"]; ok {
			b, ok := v.(bool)
			if !ok {
				return nil, nil, fmt.Errorf("input confirm must be bool")
			}
			d.Confirm = b
		}
		if v, ok := values["options"]; ok {
			opts, ok := v.([]interface{})
			if !ok {
				return nil, nil, fmt.Errorf("input options must be a tuple")
			}
			d.Options = opts
		}
		if kind == "enum" && d.Options == nil {
			v, ok := defval.(EnumValue)
			if !ok {
				return nil, nil, fmt.Errorf("input.enum %s requires an enum member", id)
			}
			for _, f := range program.Types[v.Type].Fields {
				d.Options = append(d.Options, EnumValue{Type: v.Type, Name: f.Name, Title: f.Title})
			}
		}
		v, err := validateInputValue(d, defval)
		if err != nil {
			return nil, nil, err
		}
		d.Default = v
		defs = append(defs, d)
	}
	return defs, calls, nil
}

func validateInputValue(d InputParameter, v interface{}) (interface{}, error) {
	fail := func() (interface{}, error) {
		return nil, fmt.Errorf("invalid value %v for input %s (%s)", v, d.ID, d.Kind)
	}
	switch d.Kind {
	case "bool":
		if _, ok := v.(bool); !ok {
			return fail()
		}
	case "float", "int", "price", "time":
		if _, ok := v.(bool); ok {
			return fail()
		}
		n, ok := toFloat(v)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
			return fail()
		}
		if (d.Kind == "int" || d.Kind == "time") && n != math.Trunc(n) {
			return fail()
		}
		if d.Min != nil && n < *d.Min || d.Max != nil && n > *d.Max {
			return fail()
		}
		v = n
	case "source":
		source, ok := v.(InputSource)
		if s, isString := v.(string); isString {
			source, ok = InputSource(s), true
		}
		if !ok || !isPriceIdentifierName(string(source)) {
			return fail()
		}
		v = source
	case "enum":
		defaultEnum, ok := d.Default.(EnumValue)
		if !ok {
			return fail()
		}
		if s, isString := v.(string); isString {
			for _, option := range d.Options {
				member, ok := option.(EnumValue)
				if ok && (s == member.Name || s == member.Type+"."+member.Name) {
					v = member
					break
				}
			}
		}
		member, ok := v.(EnumValue)
		if !ok || member.Type != defaultEnum.Type {
			return fail()
		}
	default:
		s, ok := v.(string)
		if !ok {
			return fail()
		}
		if d.Kind == "timeframe" && s != "" {
			if _, _, ok := parseTimeframe(s); !ok {
				return fail()
			}
		}
		if d.Kind == "session" {
			if _, _, err := sessionAt(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), s); err != nil {
				return fail()
			}
		}
	}
	if len(d.Options) > 0 {
		found := false
		for _, option := range d.Options {
			if reflect.DeepEqual(option, v) {
				found = true
				break
			}
			if a, ok := toFloat(option); ok {
				if b, ok := toFloat(v); ok && a == b {
					found = true
					break
				}
			}
		}
		if !found {
			return fail()
		}
	}
	return v, nil
}

func (e *Engine) prepareInputs(program Program) (map[*Expr]inputBinding, error) {
	defs, calls, err := discoverInputs(program)
	if err != nil {
		return nil, err
	}
	values := make(map[string]interface{}, len(e.inputParameters))
	for k, v := range e.inputParameters {
		values[k] = v
	}
	if e.inputResolver != nil {
		resolved, err := e.inputResolver.ResolveInputs(defs)
		if err != nil {
			return nil, fmt.Errorf("resolve inputs: %w", err)
		}
		for k, v := range resolved {
			values[k] = v
		}
	}
	bindings := make(map[*Expr]inputBinding, len(defs))
	for i, def := range defs {
		value := def.Default
		if v, ok := values[def.ID]; ok {
			value = v
			delete(values, def.ID)
		}
		value, err = validateInputValue(def, value)
		if err != nil {
			return nil, err
		}
		binding := inputBinding{value: value}
		if source, ok := value.(InputSource); ok {
			binding.source = &Expr{Kind: "ident", Name: string(source)}
			lowerExpr(binding.source)
		}
		bindings[calls[i].expr] = binding
	}
	for name := range values {
		return nil, fmt.Errorf("unknown input parameter %q", name)
	}
	return bindings, nil
}
