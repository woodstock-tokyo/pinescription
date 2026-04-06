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
}

func TestParseProgramCallNamedArg(t *testing.T) {
	_, err := parseProgram("x = box.new(_left, _top, _right, _bottom, _border_color, _border_width, _border_style, bgcolor = _border_color)\n")
	if err != nil {
		t.Fatalf("parseProgram: %v", err)
	}
}
