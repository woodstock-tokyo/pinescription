// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Separate dispatch keeps additions local without changing the host's argument
// binding or function-registration interfaces.
var additionalBuiltinNames = map[string]struct{}{
	"library":    {},
	"array.fill": {}, "array.reverse": {}, "array.binary_search": {},
	"array.sort": {}, "array.sort_indices": {}, "array.some": {},
	"array.stdev": {}, "array.variance": {}, "array.standardize": {},
	"array.new_color": {}, "array.new_line": {}, "array.new_linefill": {}, "array.new_table": {},
	"map.put_all": {}, "str.tonumber": {}, "str.trim": {}, "str.repeat": {},
	"str.replace_all": {}, "str.pos": {}, "str.match": {}, "ta.rci": {},
	"runtime.error": {},
	"ta.vwap":       {}, "math.round_to_mintick": {}, "str.format_time": {},
	"year": {}, "month": {}, "weekofyear": {}, "dayofmonth": {}, "dayofweek": {}, "hour": {}, "minute": {}, "second": {},
	"syminfo.prefix": {}, "syminfo.ticker": {},
	"input.price": {}, "input.text_area": {}, "input.timeframe": {}, "input.enum": {},
	"ticker.new": {}, "ticker.modify": {}, "ticker.standard": {}, "ticker.inherit": {}, "ticker.heikinashi": {},
	"ticker.renko": {}, "ticker.kagi": {}, "ticker.linebreak": {}, "ticker.pointfigure": {},
}

func integerArg(v interface{}, name string, min, max int) (int, error) {
	if _, ok := v.(bool); ok {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	f, ok := toFloat(v)
	if !ok || math.IsNaN(f) || math.IsInf(f, 0) || f != math.Trunc(f) || f < float64(min) || f > float64(max) {
		return 0, fmt.Errorf("%s must be an integer from %d to %d", name, min, max)
	}
	return int(f), nil
}

func (r *Runtime) callAdditionalBuiltin(name string, raw []*Expr, args []interface{}) (interface{}, bool, error) {
	switch {
	case name == "footprint" || name == "volume_row" || strings.HasPrefix(name, "footprint.") || strings.HasPrefix(name, "volume_row."):
		if _, known := additionalBuiltinNames[name]; !known {
			return nil, false, nil
		}
		value, err := footprintBuiltin(name, args)
		return value, true, err
	case strings.HasPrefix(name, "ticker."):
		if _, ok := additionalBuiltinNames[name]; !ok {
			return nil, false, nil
		}
		v, err := r.builtinTicker(name, args)
		return v, true, err
	case name == "ta.vwap":
		v, err := r.builtinVWAP(raw, args)
		return v, true, err
	case name == "math.round_to_mintick":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("math.round_to_mintick expects 1 arg")
		}
		n, ok := toFloat(args[0])
		if !ok {
			return nil, true, fmt.Errorf("math.round_to_mintick requires a number")
		}
		tick, ok := toFloat(r.rootNamespaces["syminfo"].(map[string]interface{})["mintick"])
		if !ok || math.IsNaN(tick) || tick <= 0 {
			return nil, true, fmt.Errorf("math.round_to_mintick requires syminfo.mintick")
		}
		return roundToTick(n, tick), true, nil
	case name == "str.format_time":
		v, err := r.builtinFormatTime(args)
		return v, true, err
	case isCalendarFunction(name):
		v, err := r.calendarFunction(name, args)
		return v, true, err
	case name == "syminfo.prefix" || name == "syminfo.ticker":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("%s expects a symbol", name)
		}
		s, ok := args[0].(string)
		if !ok {
			return nil, true, fmt.Errorf("%s symbol must be a string", name)
		}
		id, err := decodeTicker(s)
		if err != nil {
			return nil, true, err
		}
		prefix, ticker, found := strings.Cut(id.symbol, ":")
		if !found {
			prefix, ticker = "", ""
		}
		if name == "syminfo.prefix" {
			return prefix, true, nil
		}
		return ticker, true, nil
	case strings.HasPrefix(name, "array.") && name != "array.new":
		if _, exists := additionalBuiltinNames[name]; !exists {
			return nil, false, nil
		}
		v, err := additionalArray(name, args)
		return v, true, err
	case name == "map.put_all":
		if len(args) != 2 {
			return nil, true, fmt.Errorf("map.put_all expects 2 args")
		}
		dst, err := asPineMap(args[0])
		if err != nil {
			return nil, true, err
		}
		src, err := asPineMap(args[1])
		if err != nil {
			return nil, true, err
		}
		for key, value := range src.data {
			dst.data[key] = value
		}
		return nil, true, nil
	case strings.HasPrefix(name, "str."):
		if _, exists := additionalBuiltinNames[name]; !exists {
			return nil, false, nil
		}
		v, err := additionalString(name, args)
		return v, true, err
	case name == "ta.rci":
		v, err := r.builtinRCI(raw, args)
		return v, true, err
	case name == "runtime.error":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("runtime.error expects 1 arg")
		}
		msg, ok := args[0].(string)
		if !ok {
			return nil, true, fmt.Errorf("runtime.error message must be a string")
		}
		return nil, true, fmt.Errorf("runtime.error at bar %d (%s): %s", r.effectiveBarIndex(), r.activeSymbol, msg)
	}
	return nil, false, nil
}

func additionalArray(name string, args []interface{}) (interface{}, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("%s requires an array", name)
	}
	arr, err := asArrayArg(args[0], name)
	if err != nil {
		return nil, err
	}
	limit := 1
	switch name {
	case "array.fill":
		limit = 4
	case "array.binary_search", "array.sort", "array.sort_indices", "array.stdev", "array.variance":
		limit = 2
	}
	if len(args) > limit {
		return nil, fmt.Errorf("%s expects at most %d args", name, limit)
	}
	switch name {
	case "array.fill":
		if len(args) < 2 {
			return nil, fmt.Errorf("array.fill expects at least 2 args")
		}
		start, end := 0, len(arr)
		if len(args) > 2 {
			start, err = integerArg(args[2], "index_from", 0, len(arr))
			if err != nil {
				return nil, err
			}
		}
		if len(args) > 3 {
			end, err = integerArg(args[3], "index_to", start, len(arr))
			if err != nil {
				return nil, err
			}
		}
		for i := start; i < end; i++ {
			arr[i] = args[1]
		}
		return nil, nil
	case "array.reverse":
		for i, j := 0, len(arr)-1; i < j; i, j = i+1, j-1 {
			arr[i], arr[j] = arr[j], arr[i]
		}
		return nil, nil
	case "array.some":
		for _, item := range arr {
			if !isNA(item) && truthy(item) {
				return true, nil
			}
		}
		return false, nil
	case "array.sort", "array.sort_indices":
		desc := false
		if len(args) == 2 {
			order, err := integerArg(args[1], "order", 0, 1)
			if err != nil {
				return nil, err
			}
			desc = order == 1
		}
		// Validate before sorting so errors never leave a partially sorted array.
		kind := ""
		for _, item := range arr {
			if isNA(item) {
				continue
			}
			k := "number"
			if _, ok := item.(string); ok {
				k = "string"
			} else if _, ok := toFloat(item); !ok {
				return nil, fmt.Errorf("%s requires numbers or strings", name)
			}
			if kind != "" && k != kind {
				return nil, fmt.Errorf("%s requires homogeneous elements", name)
			}
			kind = k
		}
		indices := make([]int, len(arr))
		for i := range indices {
			indices[i] = i
		}
		sort.SliceStable(indices, func(i, j int) bool {
			a, b := arr[indices[i]], arr[indices[j]]
			if isNA(a) {
				return false
			}
			if isNA(b) {
				return true
			}
			if kind == "string" {
				if desc {
					return a.(string) > b.(string)
				}
				return a.(string) < b.(string)
			}
			x, _ := toFloat(a)
			y, _ := toFloat(b)
			if desc {
				return x > y
			}
			return x < y
		})
		out := make([]interface{}, len(arr))
		if name == "array.sort_indices" {
			for i, v := range indices {
				out[i] = v
			}
			return &pineArray{items: out}, nil
		}
		for i, v := range indices {
			out[i] = arr[v]
		}
		copy(arr, out)
		return nil, nil
	case "array.binary_search":
		if len(args) != 2 {
			return nil, fmt.Errorf("array.binary_search expects 2 args")
		}
		target, ok := toFloat(args[1])
		if !ok {
			return nil, fmt.Errorf("array.binary_search value must be numeric")
		}
		vals, err := arrayNumericValues(args[0], name)
		if err != nil {
			return nil, err
		}
		i := sort.Search(len(vals), func(i int) bool { return vals[i] >= target })
		if i < len(vals) && vals[i] == target {
			return i, nil
		}
		return -1, nil
	case "array.stdev", "array.variance", "array.standardize":
		vals, err := arrayNumericValues(args[0], name)
		if err != nil {
			return nil, err
		}
		biased := true
		if len(args) == 2 {
			var ok bool
			biased, ok = args[1].(bool)
			if !ok {
				return nil, fmt.Errorf("%s biased must be bool", name)
			}
		}
		mean, count := 0.0, 0
		for _, v := range vals {
			if !math.IsNaN(v) {
				count++
				mean += (v - mean) / float64(count)
			}
		}
		sum := 0.0
		for _, v := range vals {
			if !math.IsNaN(v) {
				sum += (v - mean) * (v - mean)
			}
		}
		divisor := count
		if !biased {
			divisor--
		}
		variance := math.NaN()
		if divisor > 0 {
			variance = sum / float64(divisor)
		}
		if name == "array.variance" {
			return variance, nil
		}
		stdev := math.Sqrt(variance)
		if name == "array.stdev" {
			return stdev, nil
		}
		out := make([]interface{}, len(vals))
		for i, v := range vals {
			if stdev == 0 && !math.IsNaN(v) {
				out[i] = float64(1)
			} else {
				out[i] = (v - mean) / stdev
			}
		}
		return &pineArray{items: out}, nil
	}
	return nil, fmt.Errorf("unknown array function %s", name)
}

var decimalNumberRE = regexp.MustCompile(`^[+-]?(?:[0-9]+\.?[0-9]*|\.[0-9]+)$`)

func additionalString(name string, args []interface{}) (interface{}, error) {
	required, max := 1, 1
	switch name {
	case "str.pos", "str.match":
		required, max = 2, 2
	case "str.repeat":
		required, max = 2, 3
	case "str.replace_all":
		required, max = 3, 3
	}
	if len(args) < required || len(args) > max {
		return nil, fmt.Errorf("%s expects %d to %d args", name, required, max)
	}
	texts := make([]string, len(args))
	for i, arg := range args {
		if name == "str.repeat" && i == 1 {
			continue
		}
		if isNA(arg) {
			continue
		}
		s, ok := arg.(string)
		if !ok {
			return nil, fmt.Errorf("%s argument %d must be a string", name, i+1)
		}
		if len(s) > maxStringSize {
			return nil, fmt.Errorf("%s string exceeds maximum length", name)
		}
		texts[i] = s
	}
	src := texts[0]
	switch name {
	case "str.tonumber":
		s := strings.TrimSpace(src)
		if !decimalNumberRE.MatchString(s) {
			return math.NaN(), nil
		}
		n, err := strconv.ParseFloat(s, 64)
		if err != nil || math.IsInf(n, 0) {
			return math.NaN(), nil
		}
		return n, nil
	case "str.trim":
		return strings.TrimSpace(src), nil
	case "str.pos":
		i := strings.Index(src, texts[1])
		if i < 0 {
			return math.NaN(), nil
		}
		return len([]rune(src[:i])), nil
	case "str.match":
		re, err := regexp.Compile(texts[1])
		if err != nil {
			return nil, fmt.Errorf("str.match unsupported or invalid regular expression: %w", err)
		}
		return re.FindString(src), nil
	case "str.repeat":
		count, err := integerArg(args[1], "repeat", 0, maxArrayNewSize)
		if err != nil {
			return nil, err
		}
		separator := ""
		if len(texts) == 3 {
			separator = texts[2]
		}
		size := int64(count) * int64(len(src))
		if count > 0 {
			size += int64(count-1) * int64(len(separator))
		}
		if size > maxStringSize {
			return nil, fmt.Errorf("str.repeat result is too long")
		}
		if count == 0 {
			return "", nil
		}
		var b strings.Builder
		b.Grow(int(size))
		for i := 0; i < count; i++ {
			if i > 0 {
				b.WriteString(separator)
			}
			b.WriteString(src)
		}
		return b.String(), nil
	case "str.replace_all":
		count := strings.Count(src, texts[1])
		size := int64(len(src)) + int64(count)*int64(len(texts[2])-len(texts[1]))
		if size > maxStringSize {
			return nil, fmt.Errorf("str.replace_all result is too long")
		}
		return strings.ReplaceAll(src, texts[1], texts[2]), nil
	}
	return nil, fmt.Errorf("unknown string function %s", name)
}

func (r *Runtime) builtinRCI(raw []*Expr, args []interface{}) (interface{}, error) {
	if len(args) != 2 || len(raw) != 2 {
		return nil, fmt.Errorf("ta.rci expects source and length")
	}
	n, err := integerArg(args[1], "ta.rci length", 1, maxArrayNewSize)
	if err != nil {
		return nil, err
	}
	if n < 2 || r.effectiveBarIndex() < n {
		return math.NaN(), nil
	}
	window, pooled, err := r.seriesWindowFromExpr(raw[0], n)
	if err != nil {
		return nil, err
	}
	if pooled {
		defer releaseFloat64Slice(window)
	}
	if len(window) < n {
		return math.NaN(), nil
	}
	indices := make([]int, n)
	for i, v := range window {
		if math.IsNaN(v) {
			return math.NaN(), nil
		}
		indices[i] = i
	}
	sort.Slice(indices, func(i, j int) bool { return window[indices[i]] < window[indices[j]] })
	mean := float64(n-1) / 2
	cov, vx, vy := 0.0, 0.0, 0.0
	for i := 0; i < n; {
		j := i + 1
		for j < n && window[indices[j]] == window[indices[i]] {
			j++
		}
		rank := float64(i+j-1)/2 - mean
		for k := i; k < j; k++ {
			x := float64(indices[k]) - mean
			cov += x * rank
			vx += x * x
			vy += rank * rank
		}
		i = j
	}
	if vy == 0 {
		return math.NaN(), nil
	}
	return 100 * cov / math.Sqrt(vx*vy), nil
}
