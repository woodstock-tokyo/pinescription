// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"fmt"
	"strings"
)

type staticExprType uint8

const (
	staticTypeUnknown staticExprType = iota
	staticTypeBool
	staticTypeNumber
	staticTypeString
	staticTypeNA
)

type staticTypeEnv struct {
	vars map[string]staticExprType
}

func newStaticTypeEnv() *staticTypeEnv {
	return &staticTypeEnv{vars: map[string]staticExprType{}}
}

func (e *staticTypeEnv) get(name string) (staticExprType, bool) {
	v, ok := e.vars[name]
	return v, ok
}

func (e *staticTypeEnv) set(name string, typ staticExprType) {
	if name == "" {
		return
	}
	e.vars[name] = typ
}

func validateNoNumericToBoolAutoConversion(program *Program) error {
	if program == nil {
		return nil
	}

	env := newStaticTypeEnv()
	if err := validateStmtListNoNumericToBool(program.Stmts, env); err != nil {
		return err
	}

	for _, fn := range program.Functions {
		fnEnv := newStaticTypeEnv()
		for name, typ := range env.vars {
			fnEnv.vars[name] = typ
		}
		for _, p := range fn.Params {
			fnEnv.set(p, staticTypeUnknown)
		}
		if fn.Expr != nil {
			if _, err := validateExprNoNumericToBool(fn.Expr, fnEnv); err != nil {
				return fmt.Errorf("function %s: %w", fn.Name, err)
			}
		}
		if err := validateStmtListNoNumericToBool(fn.Body, fnEnv); err != nil {
			return fmt.Errorf("function %s: %w", fn.Name, err)
		}
	}

	return nil
}

func validateStmtListNoNumericToBool(stmts []Stmt, env *staticTypeEnv) error {
	for _, stmt := range stmts {
		if err := validateStmtNoNumericToBool(stmt, env); err != nil {
			return err
		}
	}
	return nil
}

func validateStmtNoNumericToBool(stmt Stmt, env *staticTypeEnv) error {
	switch stmt.Kind {
	case "decl":
		valueType := staticTypeUnknown
		if stmt.Expr != nil {
			inferred, err := validateExprNoNumericToBool(stmt.Expr, env)
			if err != nil {
				return err
			}
			valueType = inferred
		}
		declared := staticTypeFromName(stmt.TypeName)
		if declared == staticTypeBool && valueType == staticTypeNumber {
			return fmt.Errorf("cannot assign int/float expression to bool variable %s", stmt.Name)
		}
		if declared != staticTypeUnknown {
			env.set(stmt.Name, declared)
			return nil
		}
		if valueType != staticTypeUnknown && valueType != staticTypeNA {
			env.set(stmt.Name, valueType)
		}
		return nil
	case "assign":
		inferred, err := validateExprNoNumericToBool(stmt.Expr, env)
		if err != nil {
			return err
		}
		if declared, ok := env.get(stmt.Name); ok {
			if declared == staticTypeBool && inferred == staticTypeNumber {
				return fmt.Errorf("cannot assign int/float expression to bool variable %s", stmt.Name)
			}
			if declared == staticTypeUnknown && inferred != staticTypeUnknown && inferred != staticTypeNA {
				env.set(stmt.Name, inferred)
			}
			return nil
		}
		if inferred != staticTypeUnknown && inferred != staticTypeNA {
			env.set(stmt.Name, inferred)
		}
		return nil
	case "tuple_assign":
		_, err := validateExprNoNumericToBool(stmt.Expr, env)
		return err
	case "expr", "return":
		_, err := validateExprNoNumericToBool(stmt.Expr, env)
		return err
	case "if":
		if err := ensureBoolContextNoNumeric(stmt.Cond, env, "if condition"); err != nil {
			return err
		}
		if err := validateStmtListNoNumericToBool(stmt.Then, env); err != nil {
			return err
		}
		return validateStmtListNoNumericToBool(stmt.Else, env)
	case "while":
		if err := ensureBoolContextNoNumeric(stmt.Cond, env, "while condition"); err != nil {
			return err
		}
		return validateStmtListNoNumericToBool(stmt.Body, env)
	case "for":
		if _, err := validateExprNoNumericToBool(stmt.From, env); err != nil {
			return err
		}
		if _, err := validateExprNoNumericToBool(stmt.To, env); err != nil {
			return err
		}
		if _, err := validateExprNoNumericToBool(stmt.By, env); err != nil {
			return err
		}
		return validateStmtListNoNumericToBool(stmt.Body, env)
	case "switch":
		if stmt.SwitchExpr != nil {
			if _, err := validateExprNoNumericToBool(stmt.SwitchExpr, env); err != nil {
				return err
			}
			for _, c := range stmt.Cases {
				if _, err := validateExprNoNumericToBool(c.Match, env); err != nil {
					return err
				}
				if err := validateStmtListNoNumericToBool(c.Body, env); err != nil {
					return err
				}
			}
		} else {
			for _, c := range stmt.Cases {
				if err := ensureBoolContextNoNumeric(c.Match, env, "switch condition"); err != nil {
					return err
				}
				if err := validateStmtListNoNumericToBool(c.Body, env); err != nil {
					return err
				}
			}
		}
		return validateStmtListNoNumericToBool(stmt.Default, env)
	default:
		return nil
	}
}

func ensureBoolContextNoNumeric(expr *Expr, env *staticTypeEnv, context string) error {
	typ, err := validateExprNoNumericToBool(expr, env)
	if err != nil {
		return err
	}
	if typ == staticTypeNumber {
		return fmt.Errorf("cannot use int/float expression as bool in %s", context)
	}
	return nil
}

func validateExprNoNumericToBool(expr *Expr, env *staticTypeEnv) (staticExprType, error) {
	if expr == nil {
		return staticTypeUnknown, nil
	}

	switch expr.Kind {
	case "number":
		return staticTypeNumber, nil
	case "bool":
		return staticTypeBool, nil
	case "string":
		return staticTypeString, nil
	case "na":
		return staticTypeNA, nil
	case "ident":
		return staticTypeForIdentifier(expr.Name, env), nil
	case "index":
		leftType, err := validateExprNoNumericToBool(expr.Left, env)
		if err != nil {
			return staticTypeUnknown, err
		}
		if _, err := validateExprNoNumericToBool(expr.Right, env); err != nil {
			return staticTypeUnknown, err
		}
		switch leftType {
		case staticTypeBool, staticTypeNumber, staticTypeString:
			return leftType, nil
		default:
			return staticTypeUnknown, nil
		}
	case "unary":
		rightType, err := validateExprNoNumericToBool(expr.Right, env)
		if err != nil {
			return staticTypeUnknown, err
		}
		switch expr.Op {
		case "not":
			if rightType == staticTypeNumber {
				return staticTypeBool, fmt.Errorf("cannot use int/float expression as bool in not expression")
			}
			return staticTypeBool, nil
		case "+", "-":
			return staticTypeNumber, nil
		default:
			return staticTypeUnknown, nil
		}
	case "binary":
		leftType, err := validateExprNoNumericToBool(expr.Left, env)
		if err != nil {
			return staticTypeUnknown, err
		}
		rightType, err := validateExprNoNumericToBool(expr.Right, env)
		if err != nil {
			return staticTypeUnknown, err
		}
		switch expr.Op {
		case "and", "or":
			if leftType == staticTypeNumber || rightType == staticTypeNumber {
				return staticTypeBool, fmt.Errorf("cannot use int/float expression as bool in logical expression")
			}
			return staticTypeBool, nil
		case "==", "!=", "<", "<=", ">", ">=":
			return staticTypeBool, nil
		case "+", "-", "*", "/", "%":
			if expr.Op == "+" && (leftType == staticTypeString || rightType == staticTypeString) {
				return staticTypeString, nil
			}
			return staticTypeNumber, nil
		default:
			return staticTypeUnknown, nil
		}
	case "ternary":
		condType, err := validateExprNoNumericToBool(expr.Left, env)
		if err != nil {
			return staticTypeUnknown, err
		}
		if condType == staticTypeNumber {
			return staticTypeUnknown, fmt.Errorf("cannot use int/float expression as bool in ternary condition")
		}
		whenTrueType, err := validateExprNoNumericToBool(expr.Right, env)
		if err != nil {
			return staticTypeUnknown, err
		}
		whenFalseType, err := validateExprNoNumericToBool(expr.Else, env)
		if err != nil {
			return staticTypeUnknown, err
		}
		if whenTrueType == whenFalseType {
			return whenTrueType, nil
		}
		return staticTypeUnknown, nil
	case "call":
		if _, err := validateExprNoNumericToBool(expr.Left, env); err != nil {
			return staticTypeUnknown, err
		}
		for _, arg := range expr.Args {
			if _, err := validateExprNoNumericToBool(arg, env); err != nil {
				return staticTypeUnknown, err
			}
		}
		if expr.Left != nil && expr.Left.Kind == "ident" {
			return staticTypeForBuiltinCall(expr.Left.Name), nil
		}
		return staticTypeUnknown, nil
	case "array", "tuple":
		for _, elem := range expr.Elems {
			if _, err := validateExprNoNumericToBool(elem, env); err != nil {
				return staticTypeUnknown, err
			}
		}
		return staticTypeUnknown, nil
	default:
		return staticTypeUnknown, nil
	}
}

func staticTypeForIdentifier(name string, env *staticTypeEnv) staticExprType {
	if env != nil {
		if t, ok := env.get(name); ok {
			return t
		}
	}
	switch name {
	case "open", "high", "low", "close", "volume", "bar_index",
		"time", "time_close", "timenow", "time_tradingday",
		"year", "month", "dayofmonth", "dayofweek", "hour", "minute", "second",
		"timeframe.multiplier", "math.e", "math.pi", "math.phi", "math.rphi":
		return staticTypeNumber
	case "timeframe.isdaily", "timeframe.isweekly", "timeframe.ismonthly", "timeframe.isdwm",
		"timeframe.isseconds", "timeframe.isticks", "timeframe.isminutes", "timeframe.isintraday":
		return staticTypeBool
	default:
		return staticTypeUnknown
	}
}

func staticTypeForBuiltinCall(name string) staticExprType {
	switch name {
	case "bool", "na", "map.contains", "array.includes", "array.every",
		"str.contains", "str.startswith", "str.endswith",
		"timeframe.change",
		"crossover", "ta.crossover", "crossunder", "ta.crossunder", "cross", "ta.cross",
		"ta.rising", "ta.falling":
		return staticTypeBool
	case "int", "float", "close_of", "open_of", "high_of", "low_of", "value_of":
		return staticTypeNumber
	default:
		if strings.HasPrefix(name, "math.") {
			return staticTypeNumber
		}
		return staticTypeUnknown
	}
}

func staticTypeFromName(name string) staticExprType {
	switch name {
	case "bool":
		return staticTypeBool
	case "int", "float":
		return staticTypeNumber
	case "string":
		return staticTypeString
	default:
		return staticTypeUnknown
	}
}
