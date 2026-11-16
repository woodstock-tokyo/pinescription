// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"
)

const maxStringSize = 1 << 20
const maxFormatSize = 4096

func replaceOccurrence(args []interface{}) (string, error) {
	if len(args) < 3 || len(args) > 4 {
		return "", fmt.Errorf("str.replace expects 3 or 4 args")
	}
	text, a := args[0].(string)
	target, b := args[1].(string)
	replacement, c := args[2].(string)
	if !a || !b || !c {
		return "", fmt.Errorf("str.replace requires string arguments")
	}
	occ := 0
	if len(args) == 4 {
		n, ok := toFloat(args[3])
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n != math.Trunc(n) {
			return "", fmt.Errorf("str.replace occurrence must be an integer")
		}
		if n < 0 || n > float64(len(text)) {
			return text, nil
		}
		occ = int(n)
	}
	if target == "" {
		positions := make([]int, 0, len([]rune(text))+1)
		for index := range text {
			positions = append(positions, index)
		}
		positions = append(positions, len(text))
		if occ >= len(positions) {
			return text, nil
		}
		if len(text)+len(replacement) > maxStringSize {
			return "", fmt.Errorf("str.replace result is too long")
		}
		pos := positions[occ]
		return text[:pos] + replacement + text[pos:], nil
	}
	start := 0
	for i := 0; start <= len(text); i++ {
		pos := strings.Index(text[start:], target)
		if pos < 0 {
			return text, nil
		}
		pos += start
		if i == occ {
			if len(text)-len(target)+len(replacement) > maxStringSize {
				return "", fmt.Errorf("str.replace result is too long")
			}
			return text[:pos] + replacement + text[pos+len(target):], nil
		}
		start = pos + 1
	}
	return text, nil
}

func (r *Runtime) builtinToString(args []interface{}) (string, error) {
	if len(args) < 1 || len(args) > 2 {
		return "", fmt.Errorf("str.tostring expects 1 or 2 args")
	}
	if isNA(args[0]) {
		return "NaN", nil
	}
	n, numeric := toFloat(args[0])
	if _, boolean := args[0].(bool); boolean {
		numeric = false
	}
	if !numeric {
		return toString(args[0]), nil
	}
	pattern := "#.##########"
	if len(args) == 2 {
		s, ok := args[1].(string)
		if !ok {
			return "", fmt.Errorf("str.tostring format must be a string")
		}
		if s != "" {
			pattern = s
		}
	}
	switch pattern {
	case "price", "inherit":
		pattern = "#.##########"
	case "percent":
		s, err := decimalFormat(n, "0.00", false)
		return s + "%", err
	case "volume":
		for _, unit := range []struct {
			size   float64
			suffix string
		}{{1e12, "T"}, {1e9, "B"}, {1e6, "M"}, {1e3, "K"}} {
			if math.Abs(n) >= unit.size {
				s, err := decimalFormat(n/unit.size, "#.###", false)
				return s + unit.suffix, err
			}
		}
		pattern = "#.###"
	case "mintick":
		tick, ok := toFloat(r.rootNamespaces["syminfo"].(map[string]interface{})["mintick"])
		if !ok || tick <= 0 || math.IsNaN(tick) || math.IsInf(tick, 0) {
			return "", fmt.Errorf("format.mintick requires syminfo.mintick")
		}
		n = roundToTick(n, tick)
		decimals := 0
		for decimals < 10 && math.Abs(tick-math.Round(tick)) > 1e-9 {
			decimals++
			tick *= 10
		}
		pattern = "0"
		if decimals > 0 {
			pattern += "." + strings.Repeat("0", decimals)
		}
	}
	return decimalFormat(n, pattern, false)
}

// Decimal rounding starts from the shortest decimal representation of the
// float, avoiding binary artifacts for values such as 1.005. MessageFormat
// uses half-even; tostring uses half-up.
func roundedDecimal(n float64, decimals int, even bool) string {
	r := new(big.Rat)
	r.SetString(strconv.FormatFloat(math.Abs(n), 'f', -1, 64))
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	r.Mul(r, new(big.Rat).SetInt(scale))
	q, rem := new(big.Int), new(big.Int)
	q.QuoRem(r.Num(), r.Denom(), rem)
	cmp := new(big.Int).Lsh(rem, 1).Cmp(r.Denom())
	if cmp > 0 || (cmp == 0 && (!even || q.Bit(0) != 0)) {
		q.Add(q, big.NewInt(1))
	}
	s := q.String()
	if decimals > 0 {
		if len(s) <= decimals {
			s = strings.Repeat("0", decimals+1-len(s)) + s
		}
		s = s[:len(s)-decimals] + "." + s[len(s)-decimals:]
	}
	return s
}

func decimalFormat(n float64, pattern string, even bool) (string, error) {
	if len(pattern) > maxFormatSize {
		return "", fmt.Errorf("number format is too long")
	}
	if math.IsNaN(n) {
		return "NaN", nil
	}
	if math.IsInf(n, 0) {
		return strconv.FormatFloat(n, 'f', -1, 64), nil
	}
	parts := strings.SplitN(pattern, ";", 2)
	negative := n < 0
	selectedNegative := negative && len(parts) == 2
	pattern = parts[0]
	if selectedNegative {
		pattern = parts[1]
	}
	first, last := -1, -1
	quoted := false
	for i, c := range pattern {
		if c == '\'' {
			quoted = !quoted
		} else if !quoted && (c == '#' || c == '0') {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		return strings.ReplaceAll(pattern, "'", ""), nil
	}
	prefix := strings.ReplaceAll(pattern[:first], "'", "")
	suffix := strings.ReplaceAll(pattern[last+1:], "'", "")
	if strings.Contains(prefix+suffix, "%") {
		n *= 100
	}
	if math.IsInf(n, 0) {
		return "", fmt.Errorf("number format overflow")
	}
	digits := strings.SplitN(pattern[first:last+1], ".", 2)
	integer := digits[0]
	frac := ""
	if len(digits) == 2 {
		frac = digits[1]
	}
	if strings.Trim(integer, "#0,") != "" || strings.Trim(frac, "#0") != "" {
		return "", fmt.Errorf("invalid number format %q", pattern)
	}
	if len(frac) > 100 {
		return "", fmt.Errorf("number format allows at most 100 fractional digits")
	}
	text := strings.SplitN(roundedDecimal(n, len(frac), even), ".", 2)
	for len(text[0]) < strings.Count(integer, "0") {
		text[0] = "0" + text[0]
	}
	if strings.Contains(integer, ",") {
		for i := len(text[0]) - 3; i > 0; i -= 3 {
			text[0] = text[0][:i] + "," + text[0][i:]
		}
	}
	result := text[0]
	if len(text) == 2 {
		f := text[1]
		for len(f) > strings.Count(frac, "0") && strings.HasSuffix(f, "0") {
			f = f[:len(f)-1]
		}
		if f != "" {
			result += "." + f
		}
	}
	if negative && !selectedNegative {
		prefix = "-" + prefix
	}
	return prefix + result + suffix, nil
}

func formatMessage(pattern string, args []interface{}) (string, error) {
	if len(pattern) > maxFormatSize {
		return "", fmt.Errorf("str.format pattern is too long")
	}
	var out strings.Builder
	quoted := false
	for i := 0; i < len(pattern); {
		if out.Len() > maxStringSize {
			return "", fmt.Errorf("str.format result is too long")
		}
		if pattern[i] == '\'' {
			if i+1 < len(pattern) && pattern[i+1] == '\'' {
				out.WriteByte('\'')
				i += 2
			} else {
				quoted = !quoted
				i++
			}
			continue
		}
		// Preserve the project's existing escaped-brace extension.
		if !quoted && i+1 < len(pattern) && (pattern[i:i+2] == "{{" || pattern[i:i+2] == "}}") {
			out.WriteByte(pattern[i])
			i += 2
			continue
		}
		if quoted || pattern[i] != '{' {
			out.WriteByte(pattern[i])
			i++
			continue
		}
		end := strings.IndexByte(pattern[i:], '}')
		if end < 0 {
			return "", fmt.Errorf("str.format has an unclosed placeholder")
		}
		end += i
		token := strings.SplitN(pattern[i+1:end], ",", 3)
		index, err := strconv.Atoi(strings.TrimSpace(token[0]))
		if err != nil || index < 0 {
			return "", fmt.Errorf("str.format has an invalid argument index")
		}
		if index >= len(args) {
			out.WriteString(pattern[i : end+1])
			i = end + 1
			continue
		}
		value := args[index]
		kind, style := "number", "#,##0.###"
		if len(token) > 1 {
			kind = strings.TrimSpace(token[1])
		}
		if len(token) > 2 {
			style = strings.TrimSpace(token[2])
		}
		var text string
		switch kind {
		case "number":
			n, ok := toFloat(value)
			if _, boolean := value.(bool); boolean && len(token) == 1 {
				ok = false
			}
			if isNA(value) {
				n, ok = math.NaN(), true
			}
			if !ok {
				if len(token) > 1 {
					return "", fmt.Errorf("str.format number argument %d is not numeric", index)
				}
				text = toString(value)
			} else {
				switch style {
				case "integer":
					style = "#,##0"
				case "percent":
					style = "#,##0%"
				case "currency":
					style = "$#,##0.00"
				}
				text, err = decimalFormat(n, style, true)
			}
		case "date", "time":
			n, ok := toFloat(value)
			if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
				return "", fmt.Errorf("str.format date/time requires a timestamp")
			}
			if len(token) < 3 || style == "short" || style == "medium" || style == "long" || style == "full" {
				style = "yyyy-MM-dd"
				if kind == "time" {
					style = "HH:mm:ss"
				}
			}
			text, err = formatPineTime(time.UnixMilli(int64(n)).UTC(), style)
		default:
			return "", fmt.Errorf("str.format unsupported format type %q", kind)
		}
		if err != nil {
			return "", err
		}
		out.WriteString(text)
		i = end + 1
	}
	if out.Len() > maxStringSize {
		return "", fmt.Errorf("str.format result is too long")
	}
	return out.String(), nil
}

type fillNAState struct {
	values  map[int]interface{}
	indices []int
}

func (r *Runtime) builtinFixnan(raw []*Expr, value interface{}) (interface{}, bool, error) {
	if len(raw) != 1 {
		return nil, true, fmt.Errorf("fixnan requires a source expression")
	}
	key := fmt.Sprintf("fixnan:%p:%s:%s", raw[0], r.activeSymbol, r.scriptCallPath)
	state, ok := r.indicatorState[key].(*fillNAState)
	if !ok {
		state = &fillNAState{values: map[int]interface{}{}}
		r.indicatorState[key] = state
	}
	idx := r.effectiveBarIndex()
	if !isNA(value) {
		if _, exists := state.values[idx]; !exists {
			state.indices = insertStateIndex(state.indices, idx)
		}
		state.values[idx] = value
		return value, true, nil
	}
	// A same-bar update must not retain a tentative value from that bar.
	delete(state.values, idx)
	pos := sort.SearchInts(state.indices, idx)
	if pos < len(state.indices) && state.indices[pos] == idx {
		state.indices = append(state.indices[:pos], state.indices[pos+1:]...)
	}
	if pos > 0 {
		return state.values[state.indices[pos-1]], true, nil
	}
	return math.NaN(), true, nil
}

func insertStateIndex(indices []int, idx int) []int {
	pos := sort.SearchInts(indices, idx)
	if pos < len(indices) && indices[pos] == idx {
		return indices
	}
	indices = append(indices, 0)
	copy(indices[pos+1:], indices[pos:])
	indices[pos] = idx
	return indices
}
