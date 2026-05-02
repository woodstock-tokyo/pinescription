// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"errors"
	"fmt"
	"math"
)

func (r *Runtime) execTopLevel() error {
	fl, err := r.execStmtList(r.program.Stmts)
	if err != nil {
		return err
	}
	if fl.kind == flowReturn {
		return nil
	}
	return nil
}

func (r *Runtime) commitBar() error {
	if len(r.envStack) == 0 {
		return nil
	}
	root := r.envStack[0]
	prevLast := r.lastValue
	for _, name := range r.rootHistoryVars {
		v, ok := root[name]
		if !ok {
			continue
		}
		if err := r.recordHistory(name, v); err != nil {
			return err
		}
	}
	r.lastValue = prevLast
	return nil
}

func (r *Runtime) execStmtList(stmts []Stmt) (flow, error) {
	var last interface{}
	hasLast := false
	for _, stmt := range stmts {
		fl, err := r.execStmt(stmt)
		if err != nil {
			return flow{}, err
		}
		if fl.kind != flowNone {
			return fl, nil
		}
		if fl.hasValue {
			last = fl.value
			hasLast = true
		}
	}
	return flow{kind: flowNone, value: last, hasValue: hasLast}, nil
}

func (r *Runtime) execStmt(stmt Stmt) (flow, error) {
	switch stmt.Kind {
	case "decl":
		if stmt.TypeName != "" && r.declaredTypes != nil {
			r.declaredTypes[stmt.Name] = stmt.TypeName
		}
		var v interface{}
		if stmt.Expr != nil {
			e, err := r.eval(stmt.Expr)
			if err != nil {
				return flow{}, err
			}
			v = e
		} else if stmt.TypeName == "bool" {
			v = false
		}
		if err := r.assign(stmt.Name, v, stmt.Const, false); err != nil {
			return flow{}, err
		}
		r.registerSeriesExpr(stmt.Name, stmt.Expr)
		return flow{kind: flowNone}, nil
	case "assign":
		if fast, v, err := r.evalSelfBinaryAssign(stmt.Name, stmt.Expr); err != nil {
			return flow{}, err
		} else if fast {
			if err := r.assign(stmt.Name, v, false, true); err != nil {
				return flow{}, err
			}
			r.registerSeriesExpr(stmt.Name, stmt.Expr)
			return flow{kind: flowNone}, nil
		}
		v, err := r.eval(stmt.Expr)
		if err != nil {
			return flow{}, err
		}
		if err := r.assign(stmt.Name, v, false, true); err != nil {
			return flow{}, err
		}
		r.registerSeriesExpr(stmt.Name, stmt.Expr)
		return flow{kind: flowNone}, nil
	case "tuple_assign":
		rhs, err := r.eval(stmt.Expr)
		if err != nil {
			return flow{}, err
		}
		var items []interface{}
		if rhs == nil {
			items = nil
		} else {
			switch v := rhs.(type) {
			case []interface{}:
				items = v
			case *pineArray:
				items = v.items
			default:
				return flow{}, fmt.Errorf("tuple assignment requires tuple/array RHS")
			}
		}
		for i, name := range stmt.TupleNames {
			if name == "_" {
				continue
			}
			var value interface{} = math.NaN()
			if i < len(items) {
				value = items[i]
			}
			if err := r.assign(name, value, false, false); err != nil {
				return flow{}, err
			}
		}
		return flow{kind: flowNone}, nil
	case "expr":
		v, err := r.eval(stmt.Expr)
		if err != nil {
			return flow{}, err
		}
		if n, ok := toFloat(v); ok {
			r.lastValue = n
		}
		return flow{kind: flowNone, value: v, hasValue: true}, nil
	case "if":
		c, err := r.eval(stmt.Cond)
		if err != nil {
			return flow{}, err
		}
		block := stmt.Else
		if truthy(c) {
			block = stmt.Then
		}
		return r.execStmtList(block)
	case "switch":
		return r.execSwitch(stmt)
	case "while":
		var last interface{}
		hasLast := false
		for {
			c, err := r.eval(stmt.Cond)
			if err != nil {
				return flow{}, err
			}
			if !truthy(c) {
				break
			}
			fl, err := r.execStmtList(stmt.Body)
			if err != nil {
				return flow{}, err
			}
			switch fl.kind {
			case flowNone:
				if fl.hasValue {
					last = fl.value
					hasLast = true
				}
			case flowBreak:
				return flow{kind: flowNone, value: last, hasValue: hasLast}, nil
			case flowContinue:
				continue
			default:
				return fl, nil
			}
		}
		return flow{kind: flowNone, value: last, hasValue: hasLast}, nil
	case "for":
		fromV, err := r.eval(stmt.From)
		if err != nil {
			return flow{}, err
		}
		toV, err := r.eval(stmt.To)
		if err != nil {
			return flow{}, err
		}
		from, _ := toFloat(fromV)
		to, _ := toFloat(toV)
		step := 1.0
		if stmt.By != nil {
			sv, err := r.eval(stmt.By)
			if err != nil {
				return flow{}, err
			}
			step, _ = toFloat(sv)
		}
		if step == 0 {
			return flow{}, errors.New("for step cannot be 0")
		}
		if disableLoopIteratorFastPath {
			var last interface{}
			hasLast := false
			cmp := func(i float64) bool {
				if step > 0 {
					return i <= to
				}
				return i >= to
			}
			for i := from; cmp(i); i += step {
				_ = r.assign(stmt.ForVar, i, false, false)
				fl, err := r.execStmtList(stmt.Body)
				if err != nil {
					return flow{}, err
				}
				switch fl.kind {
				case flowNone:
					if fl.hasValue {
						last = fl.value
						hasLast = true
					}
				case flowBreak:
					return flow{kind: flowNone, value: last, hasValue: hasLast}, nil
				case flowContinue:
					continue
				default:
					return fl, nil
				}
			}
			return flow{kind: flowNone, value: last, hasValue: hasLast}, nil
		}

		if r.consts[stmt.ForVar] {
			return flow{}, fmt.Errorf("cannot assign const variable %s", stmt.ForVar)
		}
		scope := r.envStack[len(r.envStack)-1]
		prevValue, hadPrev := scope[stmt.ForVar]
		defer func() {
			if hadPrev {
				scope[stmt.ForVar] = prevValue
			} else {
				delete(scope, stmt.ForVar)
			}
		}()
		r.loopBindings = append(r.loopBindings, loopBinding{name: stmt.ForVar, value: from})
		defer func() {
			r.loopBindings = r.loopBindings[:len(r.loopBindings)-1]
		}()

		var last interface{}
		hasLast := false
		cmp := func(i float64) bool {
			if step > 0 {
				return i <= to
			}
			return i >= to
		}
		for i := from; cmp(i); i += step {
			r.loopBindings[len(r.loopBindings)-1].value = i
			fl, err := r.execStmtList(stmt.Body)
			if err != nil {
				return flow{}, err
			}
			switch fl.kind {
			case flowNone:
				if fl.hasValue {
					last = fl.value
					hasLast = true
				}
			case flowBreak:
				return flow{kind: flowNone, value: last, hasValue: hasLast}, nil
			case flowContinue:
				continue
			default:
				return fl, nil
			}
		}
		return flow{kind: flowNone, value: last, hasValue: hasLast}, nil
	case "break":
		return flow{kind: flowBreak}, nil
	case "continue":
		return flow{kind: flowContinue}, nil
	case "return":
		if stmt.Expr == nil {
			return flow{kind: flowReturn}, nil
		}
		v, err := r.eval(stmt.Expr)
		if err != nil {
			return flow{}, err
		}
		return flow{kind: flowReturn, value: v}, nil
	default:
		return flow{}, fmt.Errorf("unsupported statement kind: %s", stmt.Kind)
	}
}

func (r *Runtime) execSwitch(stmt Stmt) (flow, error) {
	hasSwitchExpr := stmt.SwitchExpr != nil
	var switchValue interface{}
	if hasSwitchExpr {
		v, err := r.eval(stmt.SwitchExpr)
		if err != nil {
			return flow{}, err
		}
		switchValue = v
	}

	matched := false
	for _, c := range stmt.Cases {
		ok, err := r.matchSwitchCase(hasSwitchExpr, switchValue, c.Match)
		if err != nil {
			return flow{}, err
		}
		if !ok {
			continue
		}
		matched = true
		fl, err := r.execStmtList(c.Body)
		if err != nil {
			return flow{}, err
		}
		return fl, nil
	}

	if !matched && len(stmt.Default) > 0 {
		return r.execStmtList(stmt.Default)
	}
	return flow{kind: flowNone}, nil
}

func (r *Runtime) matchSwitchCase(hasSwitchExpr bool, switchValue interface{}, caseExpr *Expr) (bool, error) {
	if !hasSwitchExpr {
		condValue, err := r.eval(caseExpr)
		if err != nil {
			return false, err
		}
		return truthy(condValue), nil
	}

	if !disableSwitchCaseConstFastPath {
		if cv, ok := constExprValue(caseExpr); ok {
			return compareSwitchValue(switchValue, cv), nil
		}
	}

	condValue, err := r.eval(caseExpr)
	if err != nil {
		return false, err
	}
	return compareEq(switchValue, condValue), nil
}

func compareSwitchValue(switchValue interface{}, caseValue interface{}) bool {
	switch sv := switchValue.(type) {
	case float64:
		cv, ok := toFloat(caseValue)
		if !ok {
			return false
		}
		if math.IsNaN(sv) || math.IsNaN(cv) {
			return false
		}
		return sv == cv
	case string:
		cv, ok := caseValue.(string)
		return ok && sv == cv
	case bool:
		cv, ok := caseValue.(bool)
		return ok && sv == cv
	default:
		return compareEq(switchValue, caseValue)
	}
}

func constExprValue(expr *Expr) (interface{}, bool) {
	if expr == nil {
		return nil, false
	}
	switch expr.KOp {
	case exprKindNumber:
		return expr.Number, true
	case exprKindString:
		return expr.String, true
	case exprKindBool:
		return expr.Bool, true
	case exprKindNA:
		return math.NaN(), true
	case exprKindUnary:
		uop := expr.UOp
		if uop == unaryOpNeg || uop == unaryOpPos {
			v, ok := constExprValue(expr.Right)
			if !ok {
				return nil, false
			}
			f, ok := toFloat(v)
			if !ok {
				return nil, false
			}
			if uop == unaryOpPos {
				return f, true
			}
			return -f, true
		}
	}
	return nil, false
}

func (r *Runtime) eval(expr *Expr) (interface{}, error) {
	if expr == nil {
		return nil, nil
	}
	switch expr.KOp {
	case exprKindNumber:
		return expr.Number, nil
	case exprKindString:
		return expr.String, nil
	case exprKindBool:
		return expr.Bool, nil
	case exprKindNA:
		return math.NaN(), nil
	case exprKindIdent:
		return r.resolve(expr.Name)
	case exprKindArray:
		arr := make([]interface{}, 0, len(expr.Elems))
		for _, e := range expr.Elems {
			v, err := r.eval(e)
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		return arr, nil
	case exprKindTuple:
		out := make([]interface{}, 0, len(expr.Elems))
		for _, e := range expr.Elems {
			v, err := r.eval(e)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	case exprKindIndex:
		idxV, err := r.eval(expr.Right)
		if err != nil {
			return nil, err
		}
		idxF, _ := toFloat(idxV)
		return r.evalIndex(expr.Left, int(idxF))
	case exprKindUnary:
		rv, err := r.eval(expr.Right)
		if err != nil {
			return nil, err
		}
		uop := expr.UOp
		switch uop {
		case unaryOpNeg:
			n, _ := toFloat(rv)
			return -n, nil
		case unaryOpPos:
			n, _ := toFloat(rv)
			return n, nil
		case unaryOpNot:
			return !truthy(rv), nil
		default:
			return nil, fmt.Errorf("unsupported unary opcode: %d", uop)
		}
	case exprKindBinary:
		lv, err := r.eval(expr.Left)
		if err != nil {
			return nil, err
		}
		rv, err := r.eval(expr.Right)
		if err != nil {
			return nil, err
		}
		op := expr.BOp
		if op != binaryOpUnknown {
			return evalBinary(op, lv, rv)
		}
		return nil, fmt.Errorf("unsupported binary opcode")
	case exprKindTernary:
		cond, err := r.eval(expr.Left)
		if err != nil {
			return nil, err
		}
		if truthy(cond) {
			return r.eval(expr.Right)
		}
		return r.eval(expr.Else)
	case exprKindCall:
		return r.evalCall(expr)
	case exprKindNamedArg:
		return nil, fmt.Errorf("named argument %q cannot be evaluated outside call binding", expr.Name)
	case exprKindCiscClamp:
		if len(expr.Args) != 3 {
			return nil, fmt.Errorf("invalid cisc_clamp args")
		}
		v0, err := r.eval(expr.Args[0])
		if err != nil {
			return nil, err
		}
		v1, err := r.eval(expr.Args[1])
		if err != nil {
			return nil, err
		}
		v2, err := r.eval(expr.Args[2])
		if err != nil {
			return nil, err
		}
		x, _ := toFloat(v0)
		low, _ := toFloat(v1)
		high, _ := toFloat(v2)
		if x < low {
			return low, nil
		}
		if x > high {
			return high, nil
		}
		return x, nil
	default:
		return nil, fmt.Errorf("unsupported expression opcode")
	}
}

func (r *Runtime) evalWithOffset(expr *Expr, offset int) (interface{}, error) {
	if offset < 0 {
		return nil, errors.New("negative evaluation offset")
	}
	prev := r.evalOffset
	r.evalOffset = prev + offset
	defer func() { r.evalOffset = prev }()
	return r.eval(expr)
}

func (r *Runtime) evalIndex(left *Expr, idx int) (interface{}, error) {
	if idx < 0 {
		return nil, errors.New("negative index")
	}
	if left == nil {
		return nil, errors.New("index target is nil")
	}

	if left.KOp == exprKindIdent {
		name := left.Name
		if idx == 0 {
			return r.resolve(name)
		}
		if isPriceIdentifierName(name) {
			return r.valueAt(r.activeSymbol, name, r.evalOffset+idx), nil
		}
		if name == "bar_index" {
			value := r.effectiveBarIndex() - idx
			if value < 0 {
				return math.NaN(), nil
			}
			return float64(value), nil
		}
		if nums, ok := r.numericHistory[name]; ok {
			pos := len(nums) - r.evalOffset - idx
			if pos < 0 || pos >= len(nums) {
				return math.NaN(), nil
			}
			return nums[pos], nil
		}
		if vals, ok := r.history[name]; ok {
			pos := len(vals) - r.evalOffset - idx
			if pos < 0 || pos >= len(vals) {
				if r.declaredTypes != nil && r.declaredTypes[name] == "bool" {
					return false, nil
				}
				return math.NaN(), nil
			}
			return vals[pos], nil
		}
	}

	if left.KOp == exprKindCall && left.Left != nil && left.Left.KOp == exprKindIdent {
		callName := left.Left.Name
		if callName == "close_of" || callName == "open_of" || callName == "high_of" || callName == "low_of" {
			if len(left.Args) != 1 {
				return nil, fmt.Errorf("%s expects 1 arg", callName)
			}
			symV, err := r.eval(left.Args[0])
			if err != nil {
				return nil, err
			}
			symbol, ok := symV.(string)
			if !ok || symbol == "" {
				return nil, fmt.Errorf("%s requires non-empty symbol string", callName)
			}
			valueType := "close"
			switch callName {
			case "open_of":
				valueType = "open"
			case "high_of":
				valueType = "high"
			case "low_of":
				valueType = "low"
			}
			return r.valueAt(symbol, valueType, r.evalOffset+idx), nil
		}
		if callName == "value_of" {
			if len(left.Args) != 2 {
				return nil, errors.New("value_of expects 2 args")
			}
			symV, err := r.eval(left.Args[0])
			if err != nil {
				return nil, err
			}
			vtV, err := r.eval(left.Args[1])
			if err != nil {
				return nil, err
			}
			symbol, ok := symV.(string)
			if !ok || symbol == "" {
				return nil, errors.New("value_of requires symbol string")
			}
			valueType, ok := vtV.(string)
			if !ok || valueType == "" {
				return nil, errors.New("value_of requires value_type string")
			}
			return r.valueAt(symbol, valueType, r.evalOffset+idx), nil
		}
	}

	leftVal, err := r.eval(left)
	if err != nil {
		return nil, err
	}
	if _, ok := leftVal.([]interface{}); ok {
		return indexValue(leftVal, idx, r.barIndex)
	}

	historical, err := r.evalWithOffset(left, idx)
	if err == nil {
		return historical, nil
	}

	return indexValue(leftVal, idx, r.barIndex)
}

func evalBinary(op uint8, lv interface{}, rv interface{}) (interface{}, error) {
	return evalBinaryByOpcode(op, lv, rv)
}

func evalBinaryArithmeticFloatByOpcode(op uint8, a, b float64) float64 {
	switch op {
	case binaryOpAdd:
		return a + b
	case binaryOpSub:
		return a - b
	case binaryOpMul:
		return a * b
	case binaryOpDiv:
		if b == 0 {
			return math.NaN()
		}
		return a / b
	case binaryOpMod:
		if b == 0 {
			return math.NaN()
		}
		return math.Mod(a, b)
	default:
		return math.NaN()
	}
}

func evalBinaryByOpcode(op uint8, lv interface{}, rv interface{}) (interface{}, error) {
	switch op {
	case binaryOpOr:
		return truthy(lv) || truthy(rv), nil
	case binaryOpAnd:
		return truthy(lv) && truthy(rv), nil
	case binaryOpEq:
		return compareEq(lv, rv), nil
	case binaryOpNeq:
		return !compareEq(lv, rv), nil
	case binaryOpLT, binaryOpLTE, binaryOpGT, binaryOpGTE, binaryOpAdd, binaryOpSub, binaryOpMul, binaryOpDiv, binaryOpMod:
		lf, _ := toFloat(lv)
		rf, _ := toFloat(rv)
		switch op {
		case binaryOpLT:
			return lf < rf, nil
		case binaryOpLTE:
			return lf <= rf, nil
		case binaryOpGT:
			return lf > rf, nil
		case binaryOpGTE:
			return lf >= rf, nil
		case binaryOpAdd:
			if ls, ok := lv.(string); ok {
				return ls + toString(rv), nil
			}
			if rs, ok := rv.(string); ok {
				return toString(lv) + rs, nil
			}
			return lf + rf, nil
		case binaryOpSub:
			return lf - rf, nil
		case binaryOpMul:
			return lf * rf, nil
		case binaryOpDiv:
			if rf == 0 {
				return math.NaN(), nil
			}
			return lf / rf, nil
		case binaryOpMod:
			if rf == 0 {
				return math.NaN(), nil
			}
			return math.Mod(lf, rf), nil
		}
	}
	return nil, fmt.Errorf("unsupported binary opcode %d", op)
}

func indexValue(v interface{}, idx int, bar int) (interface{}, error) {
	if idx < 0 {
		return nil, errors.New("negative index")
	}
	switch arr := v.(type) {
	case []interface{}:
		if idx >= len(arr) {
			return nil, fmt.Errorf("index out of range: %d", idx)
		}
		return arr[idx], nil
	default:
		return nil, fmt.Errorf("value is not indexable at bar %d", bar)
	}
}

func compareEq(a, b interface{}) bool {
	af, aok := toFloat(a)
	bf, bok := toFloat(b)
	if aok && bok {
		if math.IsNaN(af) || math.IsNaN(bf) {
			return false
		}
		return af == bf
	}
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

func truthy(v interface{}) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return t != ""
	default:
		f, ok := toFloat(v)
		if !ok {
			return true
		}
		return !math.IsNaN(f) && f != 0
	}
}

func toFloat(v interface{}) (float64, bool) {
	switch t := v.(type) {
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case bool:
		if t {
			return 1, true
		}
		return 0, true
	case nil:
		return math.NaN(), true
	default:
		return 0, false
	}
}

func toString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return "na"
	default:
		return fmt.Sprintf("%v", t)
	}
}
