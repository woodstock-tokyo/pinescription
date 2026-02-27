// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

func lowerProgram(program *Program) {
	if program == nil {
		return
	}
	for i := range program.Stmts {
		lowerStmt(&program.Stmts[i])
	}
	for name, fn := range program.Functions {
		lowerFunctionDef(&fn)
		program.Functions[name] = fn
	}
	for name, td := range program.Types {
		for i := range td.Fields {
			lowerExpr(td.Fields[i].Default)
		}
		program.Types[name] = td
	}
}

func lowerFunctionDef(fn *FunctionDef) {
	if fn == nil {
		return
	}
	for i := range fn.Body {
		lowerStmt(&fn.Body[i])
	}
	lowerExpr(fn.Expr)
}

func lowerStmt(stmt *Stmt) {
	if stmt == nil {
		return
	}
	lowerExpr(stmt.Expr)
	lowerExpr(stmt.Target)
	lowerExpr(stmt.Cond)
	lowerExpr(stmt.From)
	lowerExpr(stmt.To)
	lowerExpr(stmt.By)
	lowerExpr(stmt.SwitchExpr)

	for i := range stmt.Then {
		lowerStmt(&stmt.Then[i])
	}
	for i := range stmt.Else {
		lowerStmt(&stmt.Else[i])
	}
	for i := range stmt.Body {
		lowerStmt(&stmt.Body[i])
	}
	for i := range stmt.Default {
		lowerStmt(&stmt.Default[i])
	}
	for i := range stmt.Cases {
		lowerExpr(stmt.Cases[i].Match)
		for j := range stmt.Cases[i].Body {
			lowerStmt(&stmt.Cases[i].Body[j])
		}
	}
	lowerFunctionDef(stmt.Func)
	if stmt.Type != nil {
		for i := range stmt.Type.Fields {
			lowerExpr(stmt.Type.Fields[i].Default)
		}
	}
}

func lowerExpr(expr *Expr) {
	if expr == nil {
		return
	}
	expr.KOp = exprKindOpcodeFromString(expr.Kind)
	lowerExpr(expr.Left)
	lowerExpr(expr.Right)
	lowerExpr(expr.Else)
	for i := range expr.Args {
		lowerExpr(expr.Args[i])
	}
	for i := range expr.Elems {
		lowerExpr(expr.Elems[i])
	}

	switch expr.Kind {
	case "unary":
		expr.UOp = unaryOpcodeFromString(expr.Op)
	case "binary":
		expr.BOp = binaryOpcodeFromString(expr.Op)
	case "call":
		if tryLowerClampCall(expr) {
			expr.KOp = exprKindCiscClamp
			return
		}
		if expr.Left != nil && expr.Left.Kind == "ident" {
			expr.BID = builtinFastID(expr.Left.Name)
		}
	}
}

func tryLowerClampCall(expr *Expr) bool {
	if expr == nil || expr.Kind != "call" || expr.Left == nil || expr.Left.Kind != "ident" || len(expr.Args) != 2 {
		return false
	}
	name := expr.Left.Name
	arg0 := expr.Args[0]
	arg1 := expr.Args[1]
	if arg0 == nil || arg1 == nil {
		return false
	}
	if (name == "math.max" || name == "max") && isCallNamed(arg0, "math.min", "min") && len(arg0.Args) == 2 {
		value := arg0.Args[0]
		upper := arg0.Args[1]
		lower := arg1
		expr.Kind = "cisc_clamp"
		expr.KOp = exprKindCiscClamp
		expr.Args = []*Expr{value, lower, upper}
		expr.Left, expr.Right, expr.Else = nil, nil, nil
		expr.BID = builtinFastUnknown
		return true
	}
	if (name == "math.min" || name == "min") && isCallNamed(arg0, "math.max", "max") && len(arg0.Args) == 2 {
		value := arg0.Args[0]
		lower := arg0.Args[1]
		upper := arg1
		expr.Kind = "cisc_clamp"
		expr.KOp = exprKindCiscClamp
		expr.Args = []*Expr{value, lower, upper}
		expr.Left, expr.Right, expr.Else = nil, nil, nil
		expr.BID = builtinFastUnknown
		return true
	}
	return false
}

func isCallNamed(expr *Expr, names ...string) bool {
	if expr == nil || expr.Kind != "call" || expr.Left == nil || expr.Left.Kind != "ident" {
		return false
	}
	for _, n := range names {
		if expr.Left.Name == n {
			return true
		}
	}
	return false
}
