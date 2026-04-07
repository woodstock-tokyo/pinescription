// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

type Program struct {
	Stmts      []Stmt                 `json:"stmts"`
	Functions  map[string]FunctionDef `json:"functions"`
	Types      map[string]TypeDef     `json:"types,omitempty"`
	Symbols    []string               `json:"symbols,omitempty"`
	ValueTypes []string               `json:"value_types,omitempty"`
	SeriesKeys []string               `json:"series_keys,omitempty"`
}

type TypeDef struct {
	Name   string      `json:"name"`
	Fields []TypeField `json:"fields,omitempty"`
}

type TypeField struct {
	Name     string `json:"name"`
	TypeName string `json:"type_name,omitempty"`
	Default  *Expr  `json:"default,omitempty"`
}

type FunctionDef struct {
	Name   string   `json:"name"`
	Params []string `json:"params"`
	Body   []Stmt   `json:"body"`
	Expr   *Expr    `json:"expr,omitempty"`
}

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

type SwitchCase struct {
	Match *Expr  `json:"match,omitempty"`
	Body  []Stmt `json:"body,omitempty"`
}

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

func (e *Expr) NamedArgValue() *Expr {
	if e == nil || e.Kind != "named_arg" {
		return nil
	}
	return e.Right
}
