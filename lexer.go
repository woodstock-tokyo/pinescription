// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

type tokenType string

const (
	tokEOF     tokenType = "EOF"
	tokNewline tokenType = "NEWLINE"
	tokIndent  tokenType = "INDENT"
	tokDedent  tokenType = "DEDENT"

	tokIdent  tokenType = "IDENT"
	tokNumber tokenType = "NUMBER"
	tokString tokenType = "STRING"

	tokLParen tokenType = "("
	tokRParen tokenType = ")"
	tokLBrack tokenType = "["
	tokRBrack tokenType = "]"
	tokComma  tokenType = ","
	tokQuest  tokenType = "?"
	tokColon  tokenType = ":"

	tokAssign      tokenType = "="
	tokReassign    tokenType = ":="
	tokArrow       tokenType = "=>"
	tokPlusAssign  tokenType = "+="
	tokMinusAssign tokenType = "-="
	tokMulAssign   tokenType = "*="
	tokDivAssign   tokenType = "/="
	tokModAssign   tokenType = "%="

	tokPlus  tokenType = "+"
	tokMinus tokenType = "-"
	tokMul   tokenType = "*"
	tokDiv   tokenType = "/"
	tokMod   tokenType = "%"

	tokEq  tokenType = "=="
	tokNeq tokenType = "!="
	tokLt  tokenType = "<"
	tokLte tokenType = "<="
	tokGt  tokenType = ">"
	tokGte tokenType = ">="
)

type token struct {
	Typ  tokenType
	Text string
	Line int
	Col  int
}

func lex(input string) ([]token, error) {
	var out []token
	lines := strings.Split(strings.ReplaceAll(input, "\r\n", "\n"), "\n")
	indentStack := []int{0}
	groupDepth := 0

	for lineNo, raw := range lines {
		lineNum := lineNo + 1
		trimmed := strings.TrimSpace(raw)
		if strings.HasPrefix(trimmed, "//@") || strings.HasPrefix(trimmed, "//") || trimmed == "" {
			continue
		}

		indent := countLeadingSpaces(raw)
		if groupDepth == 0 {
			if indent > indentStack[len(indentStack)-1] {
				indentStack = append(indentStack, indent)
				out = append(out, token{Typ: tokIndent, Text: "INDENT", Line: lineNum, Col: 1})
			} else {
				for indent < indentStack[len(indentStack)-1] {
					indentStack = indentStack[:len(indentStack)-1]
					out = append(out, token{Typ: tokDedent, Text: "DEDENT", Line: lineNum, Col: 1})
				}
				if indent != indentStack[len(indentStack)-1] {
					return nil, fmt.Errorf("invalid indentation at line %d", lineNum)
				}
			}
		}

		lineTokens, err := lexLine(raw[indent:], lineNum)
		if err != nil {
			return nil, err
		}
		out = append(out, lineTokens...)
		for _, tk := range lineTokens {
			switch tk.Typ {
			case tokLParen, tokLBrack:
				groupDepth++
			case tokRParen, tokRBrack:
				if groupDepth > 0 {
					groupDepth--
				}
			}
		}
		if groupDepth == 0 {
			out = append(out, token{Typ: tokNewline, Text: "NEWLINE", Line: lineNum, Col: len(raw) + 1})
		}
	}

	for len(indentStack) > 1 {
		indentStack = indentStack[:len(indentStack)-1]
		out = append(out, token{Typ: tokDedent, Text: "DEDENT"})
	}
	out = append(out, token{Typ: tokEOF, Text: "EOF"})
	return out, nil
}

func countLeadingSpaces(s string) int {
	n := 0
	for _, r := range s {
		if r == ' ' {
			n++
			continue
		}
		if r == '\t' {
			n += 4
			continue
		}
		break
	}
	return n
}

func lexLine(line string, lineNum int) ([]token, error) {
	var out []token
	for i := 0; i < len(line); {
		ch := line[i]
		if ch == ' ' || ch == '\t' {
			i++
			continue
		}

		if ch == '/' && i+1 < len(line) && line[i+1] == '/' {
			break
		}

		if ch == '"' || ch == '\'' {
			quote := ch
			j := i + 1
			for j < len(line) && line[j] != quote {
				if line[j] == '\\' && j+1 < len(line) {
					j += 2
					continue
				}
				j++
			}
			if j >= len(line) {
				return nil, fmt.Errorf("unterminated string at line %d", lineNum)
			}
			lit := line[i : j+1]
			var v string
			var err error
			if quote == '"' {
				v, err = strconv.Unquote(lit)
			} else {
				v, err = unescapeSingleQuotedString(lit)
			}
			if err != nil {
				return nil, fmt.Errorf("bad string at line %d: %w", lineNum, err)
			}
			out = append(out, token{Typ: tokString, Text: v, Line: lineNum, Col: i + 1})
			i = j + 1
			continue
		}

		if ch == '#' {
			j := i + 1
			for j < len(line) && isHexDigit(line[j]) {
				j++
			}
			digits := line[i+1 : j]
			if len(digits) != 6 && len(digits) != 8 {
				return nil, fmt.Errorf("bad color literal at line %d", lineNum)
			}
			out = append(out, token{Typ: tokString, Text: "#" + digits, Line: lineNum, Col: i + 1})
			i = j
			continue
		}

		if isDigit(ch) || (ch == '.' && i+1 < len(line) && isDigit(line[i+1])) {
			j := i + 1
			dot := ch == '.'
			for j < len(line) {
				if line[j] == '.' {
					if dot {
						break
					}
					dot = true
					j++
					continue
				}
				if !isDigit(line[j]) {
					break
				}
				j++
			}
			out = append(out, token{Typ: tokNumber, Text: line[i:j], Line: lineNum, Col: i + 1})
			i = j
			continue
		}

		if isIdentStart(rune(ch)) {
			j := i + 1
			for j < len(line) && isIdentPart(rune(line[j])) {
				j++
			}
			out = append(out, token{Typ: tokIdent, Text: line[i:j], Line: lineNum, Col: i + 1})
			i = j
			continue
		}

		if i+1 < len(line) {
			two := line[i : i+2]
			switch two {
			case ":=":
				out = append(out, token{Typ: tokReassign, Text: two, Line: lineNum, Col: i + 1})
				i += 2
				continue
			case "=>":
				out = append(out, token{Typ: tokArrow, Text: two, Line: lineNum, Col: i + 1})
				i += 2
				continue
			case "+=":
				out = append(out, token{Typ: tokPlusAssign, Text: two, Line: lineNum, Col: i + 1})
				i += 2
				continue
			case "-=":
				out = append(out, token{Typ: tokMinusAssign, Text: two, Line: lineNum, Col: i + 1})
				i += 2
				continue
			case "*=":
				out = append(out, token{Typ: tokMulAssign, Text: two, Line: lineNum, Col: i + 1})
				i += 2
				continue
			case "/=":
				out = append(out, token{Typ: tokDivAssign, Text: two, Line: lineNum, Col: i + 1})
				i += 2
				continue
			case "%=":
				out = append(out, token{Typ: tokModAssign, Text: two, Line: lineNum, Col: i + 1})
				i += 2
				continue
			case "==":
				out = append(out, token{Typ: tokEq, Text: two, Line: lineNum, Col: i + 1})
				i += 2
				continue
			case "!=":
				out = append(out, token{Typ: tokNeq, Text: two, Line: lineNum, Col: i + 1})
				i += 2
				continue
			case "<=":
				out = append(out, token{Typ: tokLte, Text: two, Line: lineNum, Col: i + 1})
				i += 2
				continue
			case ">=":
				out = append(out, token{Typ: tokGte, Text: two, Line: lineNum, Col: i + 1})
				i += 2
				continue
			}
		}

		t := token{Text: string(ch), Line: lineNum, Col: i + 1}
		switch ch {
		case '(':
			t.Typ = tokLParen
		case ')':
			t.Typ = tokRParen
		case '[':
			t.Typ = tokLBrack
		case ']':
			t.Typ = tokRBrack
		case ',':
			t.Typ = tokComma
		case '?':
			t.Typ = tokQuest
		case ':':
			t.Typ = tokColon
		case '=':
			t.Typ = tokAssign
		case '+':
			t.Typ = tokPlus
		case '-':
			t.Typ = tokMinus
		case '*':
			t.Typ = tokMul
		case '/':
			t.Typ = tokDiv
		case '%':
			t.Typ = tokMod
		case '<':
			t.Typ = tokLt
		case '>':
			t.Typ = tokGt
		default:
			return nil, fmt.Errorf("unexpected character %q at line %d", ch, lineNum)
		}
		out = append(out, t)
		i++
	}

	return out, nil
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isHexDigit(ch byte) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

func unescapeSingleQuotedString(lit string) (string, error) {
	if len(lit) < 2 || lit[0] != '\'' || lit[len(lit)-1] != '\'' {
		return "", fmt.Errorf("expected single-quoted literal")
	}
	s := lit[1 : len(lit)-1]
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			continue
		}
		if i+1 >= len(s) {
			return "", fmt.Errorf("bad escape")
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case '\\':
			b.WriteByte('\\')
		case '\'':
			b.WriteByte('\'')
		case '"':
			b.WriteByte('"')
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String(), nil
}

func isIdentStart(r rune) bool {
	return unicode.IsLetter(r) || r == '_'
}

func isIdentPart(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.'
}
