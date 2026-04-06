// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import "testing"

func TestParseArgsNamed(t *testing.T) {
	tokens, err := lex("(bgcolor = 1)")
	if err != nil {
		t.Fatalf("lex: %v", err)
	}
	if len(tokens) == 0 {
		t.Fatalf("lex returned no tokens")
	}

	p := &parser{tokens: tokens}
	args, err := p.parseArgs()
	if err != nil {
		for i, tok := range tokens {
			t.Logf("tok[%d]=%s %q line=%d col=%d", i, tok.Typ, tok.Text, tok.Line, tok.Col)
		}
		t.Fatalf("parseArgs: %v", err)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	if args[0] == nil || args[0].Kind != "named_arg" {
		t.Fatalf("expected named_arg, got %#v", args[0])
	}
	if args[0].Name != "bgcolor" {
		t.Fatalf("expected name bgcolor, got %q", args[0].Name)
	}
	if args[0].NamedArgValue() == nil || args[0].NamedArgValue().Kind != "number" || args[0].NamedArgValue().Number != 1 {
		t.Fatalf("expected numeric named arg value 1, got %#v", args[0].NamedArgValue())
	}
}

func TestParseProgramCallNamedArg(t *testing.T) {
	program, err := parseProgram("x = box.new(_left, _top, _right, _bottom, _border_color, _border_width, _border_style, bgcolor = _border_color)\n")
	if err != nil {
		t.Fatalf("parseProgram: %v", err)
	}
	if len(program.Stmts) != 1 || program.Stmts[0].Expr == nil {
		t.Fatalf("expected single expr stmt, got %#v", program.Stmts)
	}
	call := program.Stmts[0].Expr
	if call.Kind != "call" {
		t.Fatalf("expected call expr, got %#v", call)
	}
	if len(call.Args) != 8 {
		t.Fatalf("expected 8 call args, got %d", len(call.Args))
	}
	last := call.Args[7]
	if last == nil || last.Kind != "named_arg" || last.Name != "bgcolor" {
		t.Fatalf("expected trailing named bgcolor arg, got %#v", last)
	}
}
