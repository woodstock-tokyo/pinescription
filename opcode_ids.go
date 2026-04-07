// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

const (
	exprKindUnknown uint8 = iota
	exprKindNumber
	exprKindString
	exprKindBool
	exprKindNA
	exprKindIdent
	exprKindArray
	exprKindTuple
	exprKindIndex
	exprKindUnary
	exprKindBinary
	exprKindTernary
	exprKindCall
	exprKindNamedArg
	exprKindCiscClamp
)

const (
	unaryOpUnknown uint8 = iota
	unaryOpNeg
	unaryOpPos
	unaryOpNot
)

const (
	binaryOpUnknown uint8 = iota
	binaryOpOr
	binaryOpAnd
	binaryOpEq
	binaryOpNeq
	binaryOpLT
	binaryOpLTE
	binaryOpGT
	binaryOpGTE
	binaryOpAdd
	binaryOpSub
	binaryOpMul
	binaryOpDiv
	binaryOpMod
)

const (
	builtinFastUnknown uint16 = iota
	builtinFastNZ
	builtinFastMathMax
	builtinFastMathMin
	builtinFastMathLog
	builtinFastHighest
	builtinFastLowest
)

func unaryOpcodeFromString(op string) uint8 {
	switch op {
	case "-":
		return unaryOpNeg
	case "+":
		return unaryOpPos
	case "not":
		return unaryOpNot
	default:
		return unaryOpUnknown
	}
}

func exprKindOpcodeFromString(kind string) uint8 {
	switch kind {
	case "number":
		return exprKindNumber
	case "string":
		return exprKindString
	case "bool":
		return exprKindBool
	case "na":
		return exprKindNA
	case "ident":
		return exprKindIdent
	case "array":
		return exprKindArray
	case "tuple":
		return exprKindTuple
	case "index":
		return exprKindIndex
	case "unary":
		return exprKindUnary
	case "binary":
		return exprKindBinary
	case "ternary":
		return exprKindTernary
	case "call":
		return exprKindCall
	case "named_arg":
		return exprKindNamedArg
	case "cisc_clamp":
		return exprKindCiscClamp
	default:
		return exprKindUnknown
	}
}

func binaryOpcodeFromString(op string) uint8 {
	switch op {
	case "or":
		return binaryOpOr
	case "and":
		return binaryOpAnd
	case "==":
		return binaryOpEq
	case "!=":
		return binaryOpNeq
	case "<":
		return binaryOpLT
	case "<=":
		return binaryOpLTE
	case ">":
		return binaryOpGT
	case ">=":
		return binaryOpGTE
	case "+":
		return binaryOpAdd
	case "-":
		return binaryOpSub
	case "*":
		return binaryOpMul
	case "/":
		return binaryOpDiv
	case "%":
		return binaryOpMod
	default:
		return binaryOpUnknown
	}
}

func builtinFastID(name string) uint16 {
	switch name {
	case "nz":
		return builtinFastNZ
	case "math.max", "max":
		return builtinFastMathMax
	case "math.min", "min":
		return builtinFastMathMin
	case "math.log":
		return builtinFastMathLog
	case "highest", "ta.highest":
		return builtinFastHighest
	case "lowest", "ta.lowest":
		return builtinFastLowest
	default:
		return builtinFastUnknown
	}
}
