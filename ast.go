// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

// Program is the top-level intermediate representation of a compiled Pine Script script.
// It contains the list of top-level statements (Stmts), user-defined functions
// (Functions), and custom type definitions (Types). Symbols, ValueTypes, and
// SeriesKeys are populated by the compiler to record all market data referenced
// in the script.
type Program struct {
	Stmts      []Stmt                 `json:"stmts"`
	Functions  map[string]FunctionDef `json:"functions"`
	Types      map[string]TypeDef     `json:"types,omitempty"`
	Symbols    []string               `json:"symbols,omitempty"`
	ValueTypes []string               `json:"value_types,omitempty"`
	SeriesKeys []string               `json:"series_keys,omitempty"`
}

// TypeDef describes a user-defined composite type in Pine Script, including its
// name and the ordered list of fields. Instances of the type are created by
// calling the TypeName.new constructor from Pine Script.
type TypeDef struct {
	Name   string      `json:"name"`
	Fields []TypeField `json:"fields,omitempty"`
}

// TypeField describes a single field within a TypeDef. TypeName is the Pine Script
// type of the field (e.g. "float", "string"). Default is an optional expression
// evaluated at instantiation time if the field is omitted.
type TypeField struct {
	Name     string `json:"name"`
	TypeName string `json:"type_name,omitempty"`
	Default  *Expr  `json:"default,omitempty"`
}

// FunctionDef describes a user-defined Pine Script function, including its name,
// parameter names, body statements, and optionally a single-expression body.
type FunctionDef struct {
	Name   string   `json:"name"`
	Params []string `json:"params"`
	Body   []Stmt   `json:"body"`
	Expr   *Expr    `json:"expr,omitempty"`
}

// Stmt represents a single statement in the Pine Script AST. Kind is one of:
// "decl", "assign", "tuple_assign", "expr", "if", "switch", "while", "for",
// "break", "continue", "return". The remaining fields hold the operands
// specific to each statement kind, as defined by the Pine Script v6 grammar.
type Stmt struct {
	Kind string `json:"kind"`
	SOp  uint8  `json:"sop,omitempty"`

	Name       string   `json:"name,omitempty"`
	TypeName   string   `json:"type_name,omitempty"`
	Const      bool     `json:"const,omitempty"`
	Expr       *Expr    `json:"expr,omitempty"`
	Target     *Expr    `json:"target,omitempty"`
	TupleNames []string `json:"tuple_names,omitempty"`

	Cond *Expr  `json:"cond,omitempty"`
	Then []Stmt `json:"then,omitempty"`
	Else []Stmt `json:"else,omitempty"`

	Body []Stmt `json:"body,omitempty"`

	ForVar string `json:"for_var,omitempty"`
	From   *Expr  `json:"from,omitempty"`
	To     *Expr  `json:"to,omitempty"`
	By     *Expr  `json:"by,omitempty"`

	Func *FunctionDef `json:"func,omitempty"`
	Type *TypeDef     `json:"type,omitempty"`

	SwitchExpr *Expr        `json:"switch_expr,omitempty"`
	Cases      []SwitchCase `json:"cases,omitempty"`
	Default    []Stmt       `json:"default,omitempty"`
}

// SwitchCase represents a single case clause within a switch statement.
// Match is the expression to compare against the switch expression.
// Body holds the statements executed when the case matches.
type SwitchCase struct {
	Match *Expr  `json:"match,omitempty"`
	Body  []Stmt `json:"body,omitempty"`
}

// Expr represents a Pine Script expression. Kind is the node kind (e.g. "number",
// "ident", "call"). KOp is the internal opcode used by the runtime evaluator.
// For literals, Number, String, and Bool hold the value. For identifiers, Name
// holds the name string. For calls, Left is the function identifier and Args
// holds the argument list. For binary and unary operators, BOp and UOp hold
// the opcode. For ternary, Left is the condition, Right is the consequent,
// and Else is the alternate.
type Expr struct {
	Kind string `json:"kind"`
	KOp  uint8  `json:"kop,omitempty"`

	Number float64 `json:"number,omitempty"`
	String string  `json:"string,omitempty"`
	Bool   bool    `json:"bool,omitempty"`
	Name   string  `json:"name,omitempty"`

	Op    string `json:"op,omitempty"`
	UOp   uint8  `json:"uop,omitempty"`
	BOp   uint8  `json:"bop,omitempty"`
	BID   uint16 `json:"bid,omitempty"`
	Left  *Expr  `json:"left,omitempty"`
	Right *Expr  `json:"right,omitempty"`
	Else  *Expr  `json:"else,omitempty"`

	Args  []*Expr `json:"args,omitempty"`
	Elems []*Expr `json:"elems,omitempty"`
}

// NamedArgValue returns the value expression for a "named_arg" expression node,
// or nil if the receiver is nil or not a named argument.
func (e *Expr) NamedArgValue() *Expr {
	if e == nil || e.Kind != "named_arg" {
		return nil
	}
	return e.Right
}
