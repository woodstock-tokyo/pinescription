// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"errors"
	"fmt"
	"strconv"
)

var errNotArrowFunction = errors.New("not arrow function")

type parser struct {
	tokens []token
	pos    int
	exprNL int
}

func parseProgram(input string) (Program, error) {
	tokens, err := lex(input)
	if err != nil {
		return Program{}, err
	}
	p := &parser{tokens: tokens}
	program := Program{Functions: map[string]FunctionDef{}, Types: map[string]TypeDef{}}
	for !p.match(tokEOF) {
		p.skipNewlines()
		if p.match(tokEOF) {
			break
		}
		stmt, err := p.parseStmt()
		if err != nil {
			return Program{}, err
		}
		if stmt.Kind == "function" {
			program.Functions[stmt.Func.Name] = *stmt.Func
		} else if stmt.Kind == "type" {
			if stmt.Type != nil {
				program.Types[stmt.Type.Name] = *stmt.Type
			}
		} else {
			program.Stmts = append(program.Stmts, stmt)
		}
		p.skipNewlines()
	}
	return program, nil
}

func (p *parser) parseStmt() (Stmt, error) {
	if p.match(tokLBrack) {
		stmt, ok, err := p.tryParseTupleAssign()
		if err != nil {
			return Stmt{}, err
		}
		if ok {
			return stmt, nil
		}
	}

	t := p.peek()
	if t.Typ == tokIdent {
		switch t.Text {
		case "if":
			return p.parseIf()
		case "while":
			return p.parseWhile()
		case "for":
			return p.parseFor()
		case "switch":
			return p.parseSwitch()
		case "break":
			p.next()
			return Stmt{Kind: "break"}, nil
		case "continue":
			p.next()
			return Stmt{Kind: "continue"}, nil
		case "return":
			p.next()
			if p.match(tokNewline) || p.match(tokDedent) || p.match(tokEOF) {
				return Stmt{Kind: "return"}, nil
			}
			expr, err := p.parseExpr(0)
			if err != nil {
				return Stmt{}, err
			}
			return Stmt{Kind: "return", Expr: expr}, nil
		case "var", "const", "varip":
			return p.parseDecl()
		case "simple", "series", "input":
			return p.parseQualifiedDecl()
		case "type":
			return p.parseTypeDecl()
		case "enum":
			return Stmt{}, fmt.Errorf("line %d: %s declarations are not supported", t.Line, t.Text)
		case "import", "export", "do", "as", "in":
			return Stmt{}, fmt.Errorf("line %d: keyword %q is recognized but not supported", t.Line, t.Text)
		case "method":
			p.next()
			return p.parseFunction(false)
		case "function":
			return p.parseFunction(true)
		}

		if p.isTypedDeclStart() {
			return p.parseTypedDecl()
		}

		if p.lookAhead(1).Typ == tokLParen {
			save := p.pos
			fnStmt, err := p.tryParseArrowFunction()
			if err == nil {
				return fnStmt, nil
			}
			if errors.Is(err, errNotArrowFunction) {
				p.pos = save
			} else {
				return Stmt{}, err
			}
		}

		if isAssignToken(p.lookAhead(1).Typ) {
			name := p.next().Text
			op := p.next().Typ
			expr, err := p.parseRHSExpr()
			if err != nil {
				return Stmt{}, err
			}
			expr = makeAssignExpr(name, op, expr)
			return Stmt{Kind: "assign", Name: name, Expr: expr}, nil
		}
	}

	expr, err := p.parseExpr(0)
	if err != nil {
		return Stmt{}, err
	}
	return Stmt{Kind: "expr", Expr: expr}, nil
}

func (p *parser) parseFunction(requireKeyword bool) (Stmt, error) {
	if requireKeyword {
		if _, err := p.expectIdent("function"); err != nil {
			return Stmt{}, err
		}
	}
	nameTok, err := p.expect(tokIdent)
	if err != nil {
		return Stmt{}, err
	}
	params, err := p.parseParams()
	if err != nil {
		return Stmt{}, err
	}
	if p.match(tokArrow) {
		p.next()
		if p.match(tokNewline) {
			if err := p.expectNewlineAndIndent(); err != nil {
				return Stmt{}, err
			}
			body, err := p.parseBlock()
			if err != nil {
				return Stmt{}, err
			}
			fn := &FunctionDef{Name: nameTok.Text, Params: params, Body: body}
			return Stmt{Kind: "function", Func: fn}, nil
		}
		expr, err := p.parseExpr(0)
		if err != nil {
			return Stmt{}, err
		}
		fn := &FunctionDef{Name: nameTok.Text, Params: params, Expr: expr}
		return Stmt{Kind: "function", Func: fn}, nil
	}

	if err := p.expectNewlineAndIndent(); err != nil {
		return Stmt{}, err
	}
	body, err := p.parseBlock()
	if err != nil {
		return Stmt{}, err
	}
	fn := &FunctionDef{Name: nameTok.Text, Params: params, Body: body}
	return Stmt{Kind: "function", Func: fn}, nil
}

func (p *parser) tryParseArrowFunction() (Stmt, error) {
	nameTok, err := p.expect(tokIdent)
	if err != nil {
		return Stmt{}, err
	}
	if !p.arrowFollowsParenGroup() {
		return Stmt{}, errNotArrowFunction
	}
	params, err := p.parseParams()
	if err != nil {
		return Stmt{}, err
	}
	if !p.match(tokArrow) {
		return Stmt{}, errNotArrowFunction
	}
	p.next()

	if p.match(tokNewline) {
		if err := p.expectNewlineAndIndent(); err != nil {
			return Stmt{}, err
		}
		body, err := p.parseBlock()
		if err != nil {
			return Stmt{}, err
		}
		fn := &FunctionDef{Name: nameTok.Text, Params: params, Body: body}
		return Stmt{Kind: "function", Func: fn}, nil
	}

	expr, err := p.parseExpr(0)
	if err != nil {
		return Stmt{}, err
	}
	fn := &FunctionDef{Name: nameTok.Text, Params: params, Expr: expr}
	return Stmt{Kind: "function", Func: fn}, nil
}

func (p *parser) arrowFollowsParenGroup() bool {
	if !p.match(tokLParen) {
		return false
	}
	depth := 0
	for i := p.pos; i < len(p.tokens); i++ {
		switch p.tokens[i].Typ {
		case tokLParen:
			depth++
		case tokRParen:
			depth--
			if depth == 0 {
				if i+1 < len(p.tokens) && p.tokens[i+1].Typ == tokArrow {
					return true
				}
				return false
			}
		}
	}
	return false
}

func (p *parser) parseDecl() (Stmt, error) {
	k := p.next().Text
	stmt := Stmt{Kind: "decl", Const: k == "const"}
	if p.match(tokIdent) && isTypeKeyword(p.peek().Text) && p.lookAhead(1).Typ == tokIdent {
		stmt.TypeName = p.next().Text
		nameTok, err := p.expect(tokIdent)
		if err != nil {
			return Stmt{}, err
		}
		stmt.Name = nameTok.Text
	} else {
		nameTok, err := p.expect(tokIdent)
		if err != nil {
			return Stmt{}, err
		}
		stmt.Name = nameTok.Text
	}
	if p.match(tokAssign) {
		p.next()
		expr, err := p.parseRHSExpr()
		if err != nil {
			return Stmt{}, err
		}
		stmt.Expr = expr
	}
	return stmt, nil
}

func (p *parser) parseRHSExpr() (*Expr, error) {
	if p.match(tokNewline) {
		p.skipNewlines()
		if _, err := p.expect(tokIndent); err != nil {
			return nil, err
		}
		p.exprNL++
		expr, err := p.parseExpr(0)
		p.exprNL--
		if err != nil {
			return nil, err
		}
		p.skipNewlines()
		if _, err := p.expect(tokDedent); err != nil {
			return nil, err
		}
		return expr, nil
	}
	return p.parseExpr(0)
}

func (p *parser) parseQualifiedDecl() (Stmt, error) {
	for p.match(tokIdent) {
		kw := p.peek().Text
		if kw != "simple" && kw != "series" && kw != "input" {
			break
		}
		p.next()
	}
	if p.match(tokIdent) {
		kw := p.peek().Text
		if kw == "var" || kw == "varip" || kw == "const" {
			return p.parseDecl()
		}
		return p.parseTypedDecl()
	}
	return Stmt{}, fmt.Errorf("line %d: expected declaration after qualifiers", p.peek().Line)
}

func (p *parser) parseTypedDecl() (Stmt, error) {
	typeName, err := p.consumeTypeAnnotation()
	if err != nil {
		return Stmt{}, err
	}
	nameTok, err := p.expect(tokIdent)
	if err != nil {
		return Stmt{}, err
	}
	stmt := Stmt{Kind: "decl", Name: nameTok.Text, TypeName: typeName}
	if p.match(tokAssign) {
		p.next()
		expr, err := p.parseExpr(0)
		if err != nil {
			return Stmt{}, err
		}
		stmt.Expr = expr
	}
	return stmt, nil
}

func (p *parser) parseTypeDecl() (Stmt, error) {
	if _, err := p.expectIdent("type"); err != nil {
		return Stmt{}, err
	}
	nameTok, err := p.expect(tokIdent)
	if err != nil {
		return Stmt{}, err
	}
	if err := p.expectNewlineAndIndent(); err != nil {
		return Stmt{}, err
	}
	fields := make([]TypeField, 0)
	for !p.match(tokDedent) && !p.match(tokEOF) {
		p.skipNewlines()
		if p.match(tokDedent) || p.match(tokEOF) {
			break
		}
		typeName, err := p.consumeTypeAnnotation()
		if err != nil {
			return Stmt{}, err
		}
		fieldTok, err := p.expect(tokIdent)
		if err != nil {
			return Stmt{}, err
		}
		var fieldDefault *Expr
		if p.match(tokAssign) {
			p.next()
			fieldDefault, err = p.parseExpr(0)
			if err != nil {
				return Stmt{}, err
			}
		}
		fields = append(fields, TypeField{Name: fieldTok.Text, TypeName: typeName, Default: fieldDefault})
		p.skipNewlines()
	}
	if _, err := p.expect(tokDedent); err != nil {
		return Stmt{}, err
	}
	typeDef := &TypeDef{Name: nameTok.Text, Fields: fields}
	return Stmt{Kind: "type", Type: typeDef}, nil
}

func (p *parser) parseIf() (Stmt, error) {
	_, _ = p.expectIdent("if")
	cond, err := p.parseExpr(0)
	if err != nil {
		return Stmt{}, err
	}
	if err := p.expectNewlineAndIndent(); err != nil {
		return Stmt{}, err
	}
	thenBlock, err := p.parseBlock()
	if err != nil {
		return Stmt{}, err
	}
	stmt := Stmt{Kind: "if", Cond: cond, Then: thenBlock}
	p.skipNewlines()
	if p.matchIdent("else") {
		p.next()
		p.skipNewlines()
		if p.matchIdent("if") {
			nested, err := p.parseIf()
			if err != nil {
				return Stmt{}, err
			}
			stmt.Else = []Stmt{nested}
			return stmt, nil
		}
		if err := p.expectNewlineAndIndent(); err != nil {
			return Stmt{}, err
		}
		elseBlock, err := p.parseBlock()
		if err != nil {
			return Stmt{}, err
		}
		stmt.Else = elseBlock
	}
	return stmt, nil
}

func (p *parser) parseWhile() (Stmt, error) {
	_, _ = p.expectIdent("while")
	cond, err := p.parseExpr(0)
	if err != nil {
		return Stmt{}, err
	}
	if err := p.expectNewlineAndIndent(); err != nil {
		return Stmt{}, err
	}
	body, err := p.parseBlock()
	if err != nil {
		return Stmt{}, err
	}
	return Stmt{Kind: "while", Cond: cond, Body: body}, nil
}

func (p *parser) parseFor() (Stmt, error) {
	_, _ = p.expectIdent("for")
	nameTok, err := p.expect(tokIdent)
	if err != nil {
		return Stmt{}, err
	}
	if _, err := p.expect(tokAssign); err != nil {
		return Stmt{}, err
	}
	from, err := p.parseExpr(0)
	if err != nil {
		return Stmt{}, err
	}
	if _, err := p.expectIdent("to"); err != nil {
		return Stmt{}, err
	}
	to, err := p.parseExpr(0)
	if err != nil {
		return Stmt{}, err
	}
	var by *Expr
	if p.matchIdent("by") {
		p.next()
		by, err = p.parseExpr(0)
		if err != nil {
			return Stmt{}, err
		}
	}
	if err := p.expectNewlineAndIndent(); err != nil {
		return Stmt{}, err
	}
	body, err := p.parseBlock()
	if err != nil {
		return Stmt{}, err
	}
	return Stmt{Kind: "for", ForVar: nameTok.Text, From: from, To: to, By: by, Body: body}, nil
}

func (p *parser) parseSwitch() (Stmt, error) {
	_, _ = p.expectIdent("switch")
	stmt := Stmt{Kind: "switch"}
	if !p.match(tokNewline) {
		expr, err := p.parseExpr(0)
		if err != nil {
			return Stmt{}, err
		}
		stmt.SwitchExpr = expr
	}
	if err := p.expectNewlineAndIndent(); err != nil {
		return Stmt{}, err
	}

	for !p.match(tokDedent) && !p.match(tokEOF) {
		p.skipNewlines()
		if p.match(tokDedent) || p.match(tokEOF) {
			break
		}

		if p.match(tokArrow) {
			p.next()
			action, err := p.parseSwitchCaseAction()
			if err != nil {
				return Stmt{}, err
			}
			stmt.Default = append(stmt.Default, action)
			p.skipNewlines()
			continue
		}

		matchExpr, err := p.parseExpr(0)
		if err != nil {
			return Stmt{}, err
		}
		if _, err := p.expect(tokArrow); err != nil {
			return Stmt{}, err
		}
		action, err := p.parseSwitchCaseAction()
		if err != nil {
			return Stmt{}, err
		}
		stmt.Cases = append(stmt.Cases, SwitchCase{Match: matchExpr, Body: []Stmt{action}})
		p.skipNewlines()
	}

	if _, err := p.expect(tokDedent); err != nil {
		return Stmt{}, err
	}
	return stmt, nil
}

func (p *parser) parseSwitchCaseAction() (Stmt, error) {
	if p.match(tokNewline) {
		return Stmt{}, fmt.Errorf("switch case action must be inline")
	}

	t := p.peek()
	if t.Typ == tokIdent {
		switch t.Text {
		case "var", "const", "varip":
			return p.parseDecl()
		case "simple", "series", "input":
			return p.parseQualifiedDecl()
		case "import", "export", "do", "as", "in":
			return Stmt{}, fmt.Errorf("line %d: keyword %q is recognized but not supported", t.Line, t.Text)
		case "method":
			p.next()
			return p.parseFunction(false)
		}
		if p.isTypedDeclStart() {
			return p.parseTypedDecl()
		}
		if isAssignToken(p.lookAhead(1).Typ) {
			name := p.next().Text
			op := p.next().Typ
			expr, err := p.parseExpr(0)
			if err != nil {
				return Stmt{}, err
			}
			expr = makeAssignExpr(name, op, expr)
			return Stmt{Kind: "assign", Name: name, Expr: expr}, nil
		}
	}

	expr, err := p.parseExpr(0)
	if err != nil {
		return Stmt{}, err
	}
	return Stmt{Kind: "expr", Expr: expr}, nil
}

func (p *parser) parseBlock() ([]Stmt, error) {
	var out []Stmt
	for !p.match(tokDedent) && !p.match(tokEOF) {
		p.skipNewlines()
		if p.match(tokDedent) || p.match(tokEOF) {
			break
		}
		stmt, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		out = append(out, stmt)
		p.skipNewlines()
	}
	if _, err := p.expect(tokDedent); err != nil {
		return nil, err
	}
	return out, nil
}

func (p *parser) parseParams() ([]string, error) {
	if _, err := p.expect(tokLParen); err != nil {
		return nil, err
	}
	var params []string
	for !p.match(tokRParen) {
		if p.match(tokIdent) && (isTypeKeyword(p.peek().Text) || p.lookAhead(1).Typ == tokIdent || p.lookAhead(1).Typ == tokLt) {
			if _, err := p.consumeTypeAnnotation(); err != nil {
				return nil, err
			}
		}
		nameTok, err := p.expect(tokIdent)
		if err != nil {
			return nil, err
		}
		name := nameTok.Text
		params = append(params, name)
		if p.match(tokComma) {
			p.next()
			continue
		}
		break
	}
	_, err := p.expect(tokRParen)
	return params, err
}

func (p *parser) parseExpr(minPrec int) (*Expr, error) {
	if p.exprNL > 0 {
		p.skipNewlines()
	}
	left, err := p.parsePrefix()
	if err != nil {
		return nil, err
	}

	for {
		if p.exprNL > 0 {
			p.skipNewlines()
		}
		t := p.peek()
		if t.Typ == tokLParen {
			args, err := p.parseArgs()
			if err != nil {
				return nil, err
			}
			left = &Expr{Kind: "call", Left: left, Args: args}
			continue
		}
		if t.Typ == tokLBrack {
			p.next()
			idx, err := p.parseExpr(0)
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(tokRBrack); err != nil {
				return nil, err
			}
			left = &Expr{Kind: "index", Left: left, Right: idx}
			continue
		}

		op, prec, ok := infixInfo(t)
		if !ok || prec < minPrec {
			break
		}
		p.next()
		right, err := p.parseExpr(prec + 1)
		if err != nil {
			return nil, err
		}
		left = &Expr{Kind: "binary", Op: op, Left: left, Right: right}
	}

	if p.exprNL > 0 {
		p.skipNewlines()
	}
	if p.match(tokQuest) && minPrec <= 0 {
		p.next()
		whenTrue, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tokColon); err != nil {
			return nil, err
		}
		whenFalse, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		left = &Expr{Kind: "ternary", Left: left, Right: whenTrue, Else: whenFalse}
	}

	return left, nil
}

func (p *parser) parsePrefix() (*Expr, error) {
	if p.exprNL > 0 {
		p.skipNewlines()
	}
	t := p.peek()
	if t.Typ == tokIdent {
		p.next()
		switch t.Text {
		case "true":
			return &Expr{Kind: "bool", Bool: true}, nil
		case "false":
			return &Expr{Kind: "bool", Bool: false}, nil
		case "na":
			if p.match(tokLParen) {
				return &Expr{Kind: "ident", Name: "na"}, nil
			}
			return &Expr{Kind: "na"}, nil
		case "not":
			right, err := p.parseExpr(9)
			if err != nil {
				return nil, err
			}
			return &Expr{Kind: "unary", Op: "not", Right: right}, nil
		default:
			return &Expr{Kind: "ident", Name: t.Text}, nil
		}
	}

	if t.Typ == tokNumber {
		p.next()
		n, err := strconv.ParseFloat(t.Text, 64)
		if err != nil {
			return nil, fmt.Errorf("line %d: invalid number %q", t.Line, t.Text)
		}
		return &Expr{Kind: "number", Number: n}, nil
	}

	if t.Typ == tokString {
		p.next()
		return &Expr{Kind: "string", String: t.Text}, nil
	}

	if t.Typ == tokMinus {
		p.next()
		right, err := p.parseExpr(9)
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: "unary", Op: "-", Right: right}, nil
	}
	if t.Typ == tokPlus {
		p.next()
		right, err := p.parseExpr(9)
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: "unary", Op: "+", Right: right}, nil
	}

	if t.Typ == tokLParen {
		p.next()
		first, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		if p.match(tokComma) {
			elems := []*Expr{first}
			for p.match(tokComma) {
				p.next()
				e, err := p.parseExpr(0)
				if err != nil {
					return nil, err
				}
				elems = append(elems, e)
			}
			if _, err := p.expect(tokRParen); err != nil {
				return nil, err
			}
			return &Expr{Kind: "tuple", Elems: elems}, nil
		}
		if _, err := p.expect(tokRParen); err != nil {
			return nil, err
		}
		return first, nil
	}

	if t.Typ == tokLBrack {
		p.next()
		var elems []*Expr
		for !p.match(tokRBrack) {
			e, err := p.parseExpr(0)
			if err != nil {
				return nil, err
			}
			elems = append(elems, e)
			if p.match(tokComma) {
				p.next()
				continue
			}
			break
		}
		if _, err := p.expect(tokRBrack); err != nil {
			return nil, err
		}
		return &Expr{Kind: "array", Elems: elems}, nil
	}

	return nil, fmt.Errorf("line %d col %d: unexpected token %q", t.Line, t.Col, t.Text)
}

func (p *parser) parseArgs() ([]*Expr, error) {
	if _, err := p.expect(tokLParen); err != nil {
		return nil, err
	}
	var args []*Expr
	p.skipNewlines()
	for !p.match(tokRParen) {
		a, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		if p.match(tokAssign) && a != nil && a.Kind == "ident" {
			p.next()
			p.skipNewlines()
			v, err := p.parseExpr(0)
			if err != nil {
				return nil, err
			}
			a = v
		}
		args = append(args, a)
		if p.match(tokComma) {
			p.next()
			p.skipNewlines()
			continue
		}
		p.skipNewlines()
		break
	}
	_, err := p.expect(tokRParen)
	return args, err
}

func (p *parser) tryParseTupleAssign() (Stmt, bool, error) {
	if !p.match(tokLBrack) {
		return Stmt{}, false, nil
	}
	save := p.pos
	p.next()
	if p.match(tokRBrack) {
		p.pos = save
		return Stmt{}, false, nil
	}

	names := make([]string, 0, 2)
	for {
		if !p.match(tokIdent) {
			p.pos = save
			return Stmt{}, false, nil
		}
		names = append(names, p.next().Text)
		if p.match(tokComma) {
			p.next()
			continue
		}
		break
	}

	if !p.match(tokRBrack) {
		p.pos = save
		return Stmt{}, false, nil
	}
	p.next()

	if !p.match(tokAssign) {
		p.pos = save
		return Stmt{}, false, nil
	}
	p.next()
	rhs, err := p.parseExpr(0)
	if err != nil {
		return Stmt{}, false, err
	}
	return Stmt{Kind: "tuple_assign", TupleNames: names, Expr: rhs}, true, nil
}

func isTypeKeyword(name string) bool {
	switch name {
	case "int", "float", "bool", "string", "array", "matrix", "map", "plot", "hline", "color", "line", "label", "box", "table", "linefill":
		return true
	default:
		return false
	}
}

func (p *parser) isTypedDeclStart() bool {
	if !p.match(tokIdent) {
		return false
	}
	if p.lookAhead(1).Typ == tokLParen {
		return false
	}
	if isTypeKeyword(p.peek().Text) {
		return true
	}
	if p.lookAhead(1).Typ == tokIdent {
		next := p.lookAhead(2).Typ
		return next == tokAssign || next == tokNewline || next == tokDedent || next == tokEOF
	}
	if p.lookAhead(1).Typ == tokLt {
		save := p.pos
		_, err := p.consumeTypeAnnotation()
		ok := false
		if err == nil && p.match(tokIdent) {
			next := p.lookAhead(1).Typ
			ok = next == tokAssign || next == tokNewline || next == tokDedent || next == tokEOF
		}
		p.pos = save
		return ok
	}
	return false
}

func (p *parser) consumeTypeAnnotation() (string, error) {
	if !p.match(tokIdent) {
		return "", fmt.Errorf("line %d: expected type annotation", p.peek().Line)
	}
	base := p.next().Text
	if p.match(tokLt) {
		depth := 0
		for {
			if p.match(tokEOF) || p.match(tokNewline) {
				return "", fmt.Errorf("line %d: unterminated generic type annotation", p.peek().Line)
			}
			if p.match(tokLt) {
				depth++
				p.next()
				continue
			}
			if p.match(tokGt) {
				depth--
				p.next()
				if depth == 0 {
					break
				}
				continue
			}
			p.next()
		}
	}
	if p.match(tokLBrack) && p.lookAhead(1).Typ == tokRBrack {
		p.next()
		p.next()
	}
	return base, nil
}

func infixInfo(t token) (string, int, bool) {
	if t.Typ == tokIdent {
		switch t.Text {
		case "or":
			return "or", 1, true
		case "and":
			return "and", 2, true
		}
	}
	switch t.Typ {
	case tokEq:
		return "==", 3, true
	case tokNeq:
		return "!=", 3, true
	case tokLt:
		return "<", 4, true
	case tokLte:
		return "<=", 4, true
	case tokGt:
		return ">", 4, true
	case tokGte:
		return ">=", 4, true
	case tokPlus:
		return "+", 5, true
	case tokMinus:
		return "-", 5, true
	case tokMul:
		return "*", 6, true
	case tokDiv:
		return "/", 6, true
	case tokMod:
		return "%", 6, true
	default:
		return "", 0, false
	}
}

func isAssignToken(typ tokenType) bool {
	switch typ {
	case tokAssign, tokReassign, tokPlusAssign, tokMinusAssign, tokMulAssign, tokDivAssign, tokModAssign:
		return true
	default:
		return false
	}
}

func makeAssignExpr(name string, op tokenType, rhs *Expr) *Expr {
	if op == tokAssign || op == tokReassign {
		return rhs
	}
	lhs := &Expr{Kind: "ident", Name: name}
	switch op {
	case tokPlusAssign:
		return &Expr{Kind: "binary", Op: "+", Left: lhs, Right: rhs}
	case tokMinusAssign:
		return &Expr{Kind: "binary", Op: "-", Left: lhs, Right: rhs}
	case tokMulAssign:
		return &Expr{Kind: "binary", Op: "*", Left: lhs, Right: rhs}
	case tokDivAssign:
		return &Expr{Kind: "binary", Op: "/", Left: lhs, Right: rhs}
	case tokModAssign:
		return &Expr{Kind: "binary", Op: "%", Left: lhs, Right: rhs}
	default:
		return rhs
	}
}

func (p *parser) expectNewlineAndIndent() error {
	p.skipNewlines()
	if p.match(tokNewline) {
		p.next()
	}
	p.skipNewlines()
	_, err := p.expect(tokIndent)
	return err
}

func (p *parser) skipNewlines() {
	for p.match(tokNewline) {
		p.next()
	}
}

func (p *parser) expectIdent(text string) (token, error) {
	t := p.peek()
	if t.Typ != tokIdent || t.Text != text {
		return token{}, fmt.Errorf("line %d: expected %q, got %q", t.Line, text, t.Text)
	}
	p.next()
	return t, nil
}

func (p *parser) matchIdent(text string) bool {
	t := p.peek()
	return t.Typ == tokIdent && t.Text == text
}

func (p *parser) expect(typ tokenType) (token, error) {
	t := p.peek()
	if t.Typ != typ {
		return token{}, fmt.Errorf("line %d: expected %s, got %s", t.Line, typ, t.Typ)
	}
	p.next()
	return t, nil
}

func (p *parser) match(typ tokenType) bool {
	return p.peek().Typ == typ
}

func (p *parser) next() token {
	t := p.tokens[p.pos]
	p.pos++
	return t
}

func (p *parser) peek() token {
	if p.pos >= len(p.tokens) {
		return token{Typ: tokEOF}
	}
	return p.tokens[p.pos]
}

func (p *parser) lookAhead(n int) token {
	idx := p.pos + n
	if idx >= len(p.tokens) {
		return token{Typ: tokEOF}
	}
	return p.tokens[idx]
}
