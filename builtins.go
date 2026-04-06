// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	wseries "github.com/woodstock-tokyo/pinescription/series"
)

func (r *Runtime) callBuiltinFast(id uint16, rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	switch id {
	case builtinFastNZ:
		if len(args) < 1 || len(args) > 2 {
			return nil, true, fmt.Errorf("nz() expects 1 or 2 args")
		}
		for _, a := range args {
			if _, ok := a.(bool); ok {
				return nil, true, fmt.Errorf("nz() does not accept bool")
			}
		}
		if !isNA(args[0]) {
			return args[0], true, nil
		}
		if len(args) == 2 {
			return args[1], true, nil
		}
		return float64(0), true, nil
	case builtinFastMathMax:
		if len(args) < 1 {
			return nil, true, fmt.Errorf("math.max() expects at least 1 arg")
		}
		m, _ := toFloat(args[0])
		for i := 1; i < len(args); i++ {
			v, _ := toFloat(args[i])
			m = math.Max(m, v)
		}
		return m, true, nil
	case builtinFastMathMin:
		if len(args) < 1 {
			return nil, true, fmt.Errorf("math.min() expects at least 1 arg")
		}
		m, _ := toFloat(args[0])
		for i := 1; i < len(args); i++ {
			v, _ := toFloat(args[i])
			m = math.Min(m, v)
		}
		return m, true, nil
	case builtinFastMathLog:
		if len(args) != 1 {
			return nil, true, fmt.Errorf("math.log() expects 1 arg")
		}
		n, _ := toFloat(args[0])
		return math.Log(n), true, nil
	case builtinFastHighest:
		return r.builtinHighest(rawArgs, args)
	case builtinFastLowest:
		return r.builtinLowest(rawArgs, args)
	default:
		return nil, false, nil
	}
}

type pineMap struct {
	data map[interface{}]interface{}
}

type pineArray struct {
	items []interface{}
}

func (r *Runtime) callBuiltin(name string, rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	switch name {
	case "int":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("int() expects 1 arg")
		}
		v, _ := toFloat(args[0])
		return int(v), true, nil
	case "float":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("float() expects 1 arg")
		}
		v, _ := toFloat(args[0])
		return v, true, nil
	case "bool":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("bool() expects 1 arg")
		}
		return truthy(args[0]), true, nil
	case "string", "str.tostring":
		if len(args) < 1 || len(args) > 2 {
			return nil, true, fmt.Errorf("string() expects 1 or 2 args")
		}
		return toString(args[0]), true, nil
	case "indicator":
		return nil, true, nil
	case "input", "input.int", "input.float", "input.bool", "input.string", "input.color", "input.time", "input.session", "input.symbol", "input.source":
		if len(args) == 0 {
			switch name {
			case "input.bool":
				return false, true, nil
			case "input.string", "input.color", "input.session", "input.symbol":
				return "", true, nil
			default:
				return float64(0), true, nil
			}
		}
		return args[0], true, nil
	case "alert":
		if len(args) < 1 || len(args) > 2 {
			return nil, true, fmt.Errorf("alert() expects 1 or 2 args")
		}
		msg := toString(args[0])
		freq := ""
		if len(args) == 2 {
			freq = toString(args[1])
		}
		r.emitAlert(msg, freq)
		return nil, true, nil
	case "alertcondition":
		if len(args) < 1 {
			return nil, true, fmt.Errorf("alertcondition() expects at least 1 arg")
		}
		if truthy(args[0]) {
			msg := ""
			if len(args) >= 3 {
				msg = toString(args[2])
			} else if len(args) >= 2 {
				msg = toString(args[1])
			}
			r.emitAlert(msg, "")
		}
		return nil, true, nil
	case "color":
		if len(args) < 3 {
			return nil, true, fmt.Errorf("color() expects at least 3 args")
		}
		return map[string]interface{}{"r": args[0], "g": args[1], "b": args[2]}, true, nil
	case "color.new":
		if len(args) < 1 || len(args) > 2 {
			return nil, true, fmt.Errorf("color.new() expects 1 or 2 args")
		}
		base := args[0]
		transp := float64(0)
		if len(args) == 2 {
			transp, _ = toFloat(args[1])
		}
		return map[string]interface{}{"base": base, "transp": transp}, true, nil
	case "color.rgb":
		if len(args) != 3 {
			return nil, true, fmt.Errorf("color.rgb() expects 3 args")
		}
		return map[string]interface{}{"r": args[0], "g": args[1], "b": args[2]}, true, nil
	case "box.new":
		if len(args) < 4 {
			return nil, true, fmt.Errorf("box.new() expects at least 4 args")
		}
		left, _ := toFloat(args[0])
		top, _ := toFloat(args[1])
		right, _ := toFloat(args[2])
		bottom, _ := toFloat(args[3])
		return map[string]interface{}{"type": "box", "left": left, "top": top, "right": right, "bottom": bottom}, true, nil
	case "box.get_bottom":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("box.get_bottom() expects 1 arg")
		}
		bx, ok := args[0].(map[string]interface{})
		if !ok {
			return math.NaN(), true, nil
		}
		v, _ := toFloat(bx["bottom"])
		return v, true, nil
	case "box.get_top":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("box.get_top() expects 1 arg")
		}
		bx, ok := args[0].(map[string]interface{})
		if !ok {
			return math.NaN(), true, nil
		}
		v, _ := toFloat(bx["top"])
		return v, true, nil
	case "box.set_right":
		if len(args) != 2 {
			return nil, true, fmt.Errorf("box.set_right() expects 2 args")
		}
		bx, ok := args[0].(map[string]interface{})
		if !ok {
			return nil, true, nil
		}
		right, _ := toFloat(args[1])
		bx["right"] = right
		return bx, true, nil
	case "box.delete":
		return nil, true, nil
	case "table.new":
		return map[string]interface{}{"type": "table", "args": args}, true, nil
	case "table.cell":
		return nil, true, nil
	case "linefill.new":
		return map[string]interface{}{"type": "linefill", "args": args}, true, nil
	case "barcolor":
		return nil, true, nil
	case "line.new":
		return map[string]interface{}{"type": "line", "args": args}, true, nil
	case "label.new":
		return map[string]interface{}{"type": "label", "args": args}, true, nil
	case "line.set_xy1":
		if len(args) != 3 {
			return nil, true, fmt.Errorf("line.set_xy1() expects 3 args")
		}
		ln, ok := args[0].(map[string]interface{})
		if !ok {
			return nil, true, nil
		}
		x1, _ := toFloat(args[1])
		y1, _ := toFloat(args[2])
		ln["x1"] = x1
		ln["y1"] = y1
		return ln, true, nil
	case "line.set_xy2":
		if len(args) != 3 {
			return nil, true, fmt.Errorf("line.set_xy2() expects 3 args")
		}
		ln, ok := args[0].(map[string]interface{})
		if !ok {
			return nil, true, nil
		}
		x2, _ := toFloat(args[1])
		y2, _ := toFloat(args[2])
		ln["x2"] = x2
		ln["y2"] = y2
		return ln, true, nil
	case "line.set_color":
		if len(args) != 2 {
			return nil, true, fmt.Errorf("line.set_color() expects 2 args")
		}
		ln, ok := args[0].(map[string]interface{})
		if !ok {
			return nil, true, nil
		}
		ln["color"] = args[1]
		return ln, true, nil
	case "label.set_xy":
		if len(args) != 3 {
			return nil, true, fmt.Errorf("label.set_xy() expects 3 args")
		}
		lb, ok := args[0].(map[string]interface{})
		if !ok {
			return nil, true, nil
		}
		x, _ := toFloat(args[1])
		y, _ := toFloat(args[2])
		lb["x"] = x
		lb["y"] = y
		return lb, true, nil
	case "label.set_text":
		if len(args) != 2 {
			return nil, true, fmt.Errorf("label.set_text() expects 2 args")
		}
		lb, ok := args[0].(map[string]interface{})
		if !ok {
			return nil, true, nil
		}
		lb["text"] = toString(args[1])
		return lb, true, nil
	case "label.set_tooltip":
		if len(args) != 2 {
			return nil, true, fmt.Errorf("label.set_tooltip() expects 2 args")
		}
		lb, ok := args[0].(map[string]interface{})
		if !ok {
			return nil, true, nil
		}
		lb["tooltip"] = toString(args[1])
		return lb, true, nil
	case "log.info":
		return r.builtinLog("info", args)
	case "log.warning":
		return r.builtinLog("warning", args)
	case "log.error":
		return r.builtinLog("error", args)
	case "map.new":
		if len(args) != 0 {
			return nil, true, fmt.Errorf("map.new expects 0 args")
		}
		return &pineMap{data: map[interface{}]interface{}{}}, true, nil
	case "map.clear":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("map.clear expects 1 arg")
		}
		pm, err := asPineMap(args[0])
		if err != nil {
			return nil, true, err
		}
		pm.data = map[interface{}]interface{}{}
		return pm, true, nil
	case "map.copy":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("map.copy expects 1 arg")
		}
		pm, err := asPineMap(args[0])
		if err != nil {
			return nil, true, err
		}
		cp := make(map[interface{}]interface{}, len(pm.data))
		for k, v := range pm.data {
			cp[k] = v
		}
		return &pineMap{data: cp}, true, nil
	case "map.size":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("map.size expects 1 arg")
		}
		pm, err := asPineMap(args[0])
		if err != nil {
			return nil, true, err
		}
		return float64(len(pm.data)), true, nil
	case "map.put":
		if len(args) != 3 {
			return nil, true, fmt.Errorf("map.put expects 3 args")
		}
		pm, err := asPineMap(args[0])
		if err != nil {
			return nil, true, err
		}
		key, err := normalizeMapKey(args[1])
		if err != nil {
			return nil, true, err
		}
		pm.data[key] = args[2]
		return pm, true, nil
	case "map.get":
		if len(args) != 2 {
			return nil, true, fmt.Errorf("map.get expects 2 args")
		}
		pm, err := asPineMap(args[0])
		if err != nil {
			return nil, true, err
		}
		key, err := normalizeMapKey(args[1])
		if err != nil {
			return nil, true, err
		}
		if v, ok := pm.data[key]; ok {
			return v, true, nil
		}
		return math.NaN(), true, nil
	case "map.contains":
		if len(args) != 2 {
			return nil, true, fmt.Errorf("map.contains expects 2 args")
		}
		pm, err := asPineMap(args[0])
		if err != nil {
			return nil, true, err
		}
		key, err := normalizeMapKey(args[1])
		if err != nil {
			return nil, true, err
		}
		_, ok := pm.data[key]
		return ok, true, nil
	case "map.remove":
		if len(args) != 2 {
			return nil, true, fmt.Errorf("map.remove expects 2 args")
		}
		pm, err := asPineMap(args[0])
		if err != nil {
			return nil, true, err
		}
		key, err := normalizeMapKey(args[1])
		if err != nil {
			return nil, true, err
		}
		delete(pm.data, key)
		return pm, true, nil
	case "map.keys":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("map.keys expects 1 arg")
		}
		pm, err := asPineMap(args[0])
		if err != nil {
			return nil, true, err
		}
		keys := stableMapKeys(pm.data)
		out := make([]interface{}, 0, len(keys))
		for _, k := range keys {
			out = append(out, k)
		}
		return out, true, nil
	case "map.values":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("map.values expects 1 arg")
		}
		pm, err := asPineMap(args[0])
		if err != nil {
			return nil, true, err
		}
		keys := stableMapKeys(pm.data)
		out := make([]interface{}, 0, len(keys))
		for _, k := range keys {
			out = append(out, pm.data[k])
		}
		return out, true, nil
	case "array.new_int", "array.new_float", "array.new_bool", "array.new_string", "array.new_box":
		sz := 0
		if len(args) > 0 {
			f, _ := toFloat(args[0])
			sz = int(f)
		}
		var init interface{}
		if len(args) > 1 {
			init = args[1]
		}
		out := make([]interface{}, sz)
		for i := range out {
			out[i] = init
		}
		return &pineArray{items: out}, true, nil
	case "array.size":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("array.size expects 1 arg")
		}
		arr, err := asArrayArg(args[0], "array.size")
		if err != nil {
			return nil, true, err
		}
		return len(arr), true, nil
	case "array.get":
		if len(args) != 2 {
			return nil, true, fmt.Errorf("array.get expects 2 args")
		}
		arr, err := asArrayArg(args[0], "array.get")
		if err != nil {
			return nil, true, err
		}
		idxF, _ := toFloat(args[1])
		idx := int(idxF)
		if idx < 0 || idx >= len(arr) {
			return nil, true, fmt.Errorf("array.get index out of range")
		}
		return arr[idx], true, nil
	case "array.set":
		if len(args) != 3 {
			return nil, true, fmt.Errorf("array.set expects 3 args")
		}
		idxF, _ := toFloat(args[1])
		idx := int(idxF)
		switch a := args[0].(type) {
		case *pineArray:
			if idx < 0 || idx >= len(a.items) {
				return nil, true, fmt.Errorf("array.set index out of range")
			}
			a.items[idx] = args[2]
			return a, true, nil
		case []interface{}:
			if idx < 0 || idx >= len(a) {
				return nil, true, fmt.Errorf("array.set index out of range")
			}
			a[idx] = args[2]
			return a, true, nil
		default:
			return nil, true, fmt.Errorf("array.set requires array")
		}
	case "array.push":
		if len(args) != 2 {
			return nil, true, fmt.Errorf("array.push expects 2 args")
		}
		switch a := args[0].(type) {
		case *pineArray:
			a.items = append(a.items, args[1])
			return a, true, nil
		case []interface{}:
			a = append(a, args[1])
			return a, true, nil
		default:
			return nil, true, fmt.Errorf("array.push requires array")
		}
	case "array.pop":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("array.pop expects 1 arg")
		}
		switch a := args[0].(type) {
		case *pineArray:
			if len(a.items) == 0 {
				return nil, true, fmt.Errorf("array.pop from empty array")
			}
			v := a.items[len(a.items)-1]
			a.items = a.items[:len(a.items)-1]
			return v, true, nil
		case []interface{}:
			if len(a) == 0 {
				return nil, true, fmt.Errorf("array.pop from empty array")
			}
			return a[len(a)-1], true, nil
		default:
			return nil, true, fmt.Errorf("array.pop requires array")
		}
	case "array.unshift":
		if len(args) != 2 {
			return nil, true, fmt.Errorf("array.unshift expects 2 args")
		}
		switch a := args[0].(type) {
		case *pineArray:
			a.items = append([]interface{}{args[1]}, a.items...)
			return a, true, nil
		case []interface{}:
			a = append([]interface{}{args[1]}, a...)
			return a, true, nil
		default:
			return nil, true, fmt.Errorf("array.unshift requires array")
		}
	case "array.shift":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("array.shift expects 1 arg")
		}
		switch a := args[0].(type) {
		case *pineArray:
			if len(a.items) == 0 {
				return nil, true, fmt.Errorf("array.shift from empty array")
			}
			v := a.items[0]
			a.items = a.items[1:]
			return v, true, nil
		case []interface{}:
			if len(a) == 0 {
				return nil, true, fmt.Errorf("array.shift from empty array")
			}
			return a[0], true, nil
		default:
			return nil, true, fmt.Errorf("array.shift requires array")
		}
	case "array.clear":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("array.clear expects 1 arg")
		}
		switch a := args[0].(type) {
		case *pineArray:
			a.items = nil
			return a, true, nil
		case []interface{}:
			return []interface{}{}, true, nil
		default:
			return nil, true, fmt.Errorf("array.clear requires array")
		}
	case "array.remove":
		if len(args) != 2 {
			return nil, true, fmt.Errorf("array.remove expects 2 args")
		}
		idxF, _ := toFloat(args[1])
		idx := int(idxF)
		switch a := args[0].(type) {
		case *pineArray:
			if idx < 0 || idx >= len(a.items) {
				return nil, true, fmt.Errorf("array.remove index out of range")
			}
			v := a.items[idx]
			a.items = append(a.items[:idx], a.items[idx+1:]...)
			return v, true, nil
		case []interface{}:
			if idx < 0 || idx >= len(a) {
				return nil, true, fmt.Errorf("array.remove index out of range")
			}
			return a[idx], true, nil
		default:
			return nil, true, fmt.Errorf("array.remove requires array")
		}
	case "array.concat":
		if len(args) != 2 {
			return nil, true, fmt.Errorf("array.concat expects 2 args")
		}
		as, err := asArrayArg(args[0], "array.concat")
		if err != nil {
			return nil, true, err
		}
		bs, err := asArrayArg(args[1], "array.concat")
		if err != nil {
			return nil, true, err
		}
		out := append([]interface{}{}, as...)
		out = append(out, bs...)
		if isPineArrayArg(args[0]) || isPineArrayArg(args[1]) {
			return &pineArray{items: out}, true, nil
		}
		return out, true, nil
	case "array.slice":
		if len(args) != 2 && len(args) != 3 {
			return nil, true, fmt.Errorf("array.slice expects 2 or 3 args")
		}
		arr, err := asArrayArg(args[0], "array.slice")
		if err != nil {
			return nil, true, err
		}
		fromF, _ := toFloat(args[1])
		from := int(fromF)
		to := len(arr)
		if len(args) == 3 {
			toF, _ := toFloat(args[2])
			to = int(toF)
		}
		if from < 0 {
			from = 0
		}
		if to > len(arr) {
			to = len(arr)
		}
		if from > to {
			from = to
		}
		out := append([]interface{}{}, arr[from:to]...)
		if isPineArrayArg(args[0]) {
			return &pineArray{items: out}, true, nil
		}
		return out, true, nil
	case "array.includes":
		if len(args) != 2 {
			return nil, true, fmt.Errorf("array.includes expects 2 args")
		}
		arr, err := asArrayArg(args[0], "array.includes")
		if err != nil {
			return nil, true, err
		}
		for _, v := range arr {
			if compareEq(v, args[1]) {
				return true, true, nil
			}
		}
		return false, true, nil
	case "array.indexof":
		if len(args) != 2 {
			return nil, true, fmt.Errorf("array.indexof expects 2 args")
		}
		arr, err := asArrayArg(args[0], "array.indexof")
		if err != nil {
			return nil, true, err
		}
		for i, v := range arr {
			if compareEq(v, args[1]) {
				return float64(i), true, nil
			}
		}
		return float64(-1), true, nil
	case "array.lastindexof":
		if len(args) != 2 {
			return nil, true, fmt.Errorf("array.lastindexof expects 2 args")
		}
		arr, err := asArrayArg(args[0], "array.lastindexof")
		if err != nil {
			return nil, true, err
		}
		for i := len(arr) - 1; i >= 0; i-- {
			if compareEq(arr[i], args[1]) {
				return float64(i), true, nil
			}
		}
		return float64(-1), true, nil
	case "array.copy":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("array.copy expects 1 arg")
		}
		arr, err := asArrayArg(args[0], "array.copy")
		if err != nil {
			return nil, true, err
		}
		out := append([]interface{}{}, arr...)
		if isPineArrayArg(args[0]) {
			return &pineArray{items: out}, true, nil
		}
		return out, true, nil
	case "array.from":
		out := make([]interface{}, len(args))
		copy(out, args)
		return &pineArray{items: out}, true, nil
	case "array.insert":
		if len(args) != 3 {
			return nil, true, fmt.Errorf("array.insert expects 3 args")
		}
		idxF, _ := toFloat(args[1])
		idx := int(idxF)
		switch a := args[0].(type) {
		case *pineArray:
			if idx < 0 || idx > len(a.items) {
				return nil, true, fmt.Errorf("array.insert index out of range")
			}
			a.items = append(a.items, nil)
			copy(a.items[idx+1:], a.items[idx:])
			a.items[idx] = args[2]
			return a, true, nil
		case []interface{}:
			if idx < 0 || idx > len(a) {
				return nil, true, fmt.Errorf("array.insert index out of range")
			}
			a = append(a, nil)
			copy(a[idx+1:], a[idx:])
			a[idx] = args[2]
			return a, true, nil
		default:
			return nil, true, fmt.Errorf("array.insert requires array")
		}
	case "array.first":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("array.first expects 1 arg")
		}
		arr, err := asArrayArg(args[0], "array.first")
		if err != nil {
			return nil, true, err
		}
		if len(arr) == 0 {
			return nil, true, fmt.Errorf("array.first from empty array")
		}
		return arr[0], true, nil
	case "array.last":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("array.last expects 1 arg")
		}
		arr, err := asArrayArg(args[0], "array.last")
		if err != nil {
			return nil, true, err
		}
		if len(arr) == 0 {
			return nil, true, fmt.Errorf("array.last from empty array")
		}
		return arr[len(arr)-1], true, nil
	case "array.join":
		if len(args) != 2 {
			return nil, true, fmt.Errorf("array.join expects 2 args")
		}
		arr, err := asArrayArg(args[0], "array.join")
		if err != nil {
			return nil, true, err
		}
		sep, ok := args[1].(string)
		if !ok {
			return nil, true, fmt.Errorf("array.join separator must be string")
		}
		parts := make([]string, 0, len(arr))
		for _, v := range arr {
			parts = append(parts, toString(v))
		}
		return strings.Join(parts, sep), true, nil
	case "array.every":
		if len(args) != 1 && len(args) != 2 {
			return nil, true, fmt.Errorf("array.every expects 1 or 2 args")
		}
		arr, err := asArrayArg(args[0], "array.every")
		if err != nil {
			return nil, true, err
		}
		if len(args) == 1 {
			for _, v := range arr {
				if !truthy(v) {
					return false, true, nil
				}
			}
			return true, true, nil
		}
		for _, v := range arr {
			if !compareEq(v, args[1]) {
				return false, true, nil
			}
		}
		return true, true, nil
	case "array.abs":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("array.abs expects 1 arg")
		}
		arr, err := asArrayArg(args[0], "array.abs")
		if err != nil {
			return nil, true, err
		}
		out := make([]interface{}, len(arr))
		for i, v := range arr {
			f, ok := toFloat(v)
			if !ok {
				return nil, true, fmt.Errorf("array.abs requires numeric array")
			}
			out[i] = math.Abs(f)
		}
		if isPineArrayArg(args[0]) {
			return &pineArray{items: out}, true, nil
		}
		return out, true, nil
	case "array.sum":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("array.sum expects 1 arg")
		}
		vals, err := arrayNumericValues(args[0], "array.sum")
		if err != nil {
			return nil, true, err
		}
		total := 0.0
		for _, v := range vals {
			total += v
		}
		return total, true, nil
	case "array.avg":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("array.avg expects 1 arg")
		}
		vals, err := arrayNumericValues(args[0], "array.avg")
		if err != nil {
			return nil, true, err
		}
		if len(vals) == 0 {
			return math.NaN(), true, nil
		}
		total := 0.0
		for _, v := range vals {
			total += v
		}
		return total / float64(len(vals)), true, nil
	case "array.max":
		if len(args) != 1 && len(args) != 2 {
			return nil, true, fmt.Errorf("array.max expects 1 or 2 args")
		}
		nth := 0
		if len(args) == 2 {
			nthF, _ := toFloat(args[1])
			nth = int(nthF)
		}
		v, err := arrayExtremaNth(args[0], nth, true, "array.max")
		if err != nil {
			return nil, true, err
		}
		return v, true, nil
	case "array.min":
		if len(args) != 1 && len(args) != 2 {
			return nil, true, fmt.Errorf("array.min expects 1 or 2 args")
		}
		nth := 0
		if len(args) == 2 {
			nthF, _ := toFloat(args[1])
			nth = int(nthF)
		}
		v, err := arrayExtremaNth(args[0], nth, false, "array.min")
		if err != nil {
			return nil, true, err
		}
		return v, true, nil
	case "array.range":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("array.range expects 1 arg")
		}
		vals, err := arrayNumericValues(args[0], "array.range")
		if err != nil {
			return nil, true, err
		}
		if len(vals) == 0 {
			return math.NaN(), true, nil
		}
		lo, hi := vals[0], vals[0]
		for i := 1; i < len(vals); i++ {
			if vals[i] < lo {
				lo = vals[i]
			}
			if vals[i] > hi {
				hi = vals[i]
			}
		}
		return hi - lo, true, nil
	case "array.median":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("array.median expects 1 arg")
		}
		vals, err := arrayNumericValues(args[0], "array.median")
		if err != nil {
			return nil, true, err
		}
		if len(vals) == 0 {
			return math.NaN(), true, nil
		}
		s := append([]float64(nil), vals...)
		sort.Float64s(s)
		m := len(s) / 2
		if len(s)%2 == 1 {
			return s[m], true, nil
		}
		return (s[m-1] + s[m]) / 2.0, true, nil
	case "array.mode":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("array.mode expects 1 arg")
		}
		vals, err := arrayNumericValues(args[0], "array.mode")
		if err != nil {
			return nil, true, err
		}
		if len(vals) == 0 {
			return math.NaN(), true, nil
		}
		counts := map[string]int{}
		values := map[string]float64{}
		bestCnt := -1
		bestVal := math.NaN()
		for _, v := range vals {
			k := strconv.FormatFloat(v, 'g', 15, 64)
			counts[k]++
			values[k] = v
			cnt := counts[k]
			if cnt > bestCnt || (cnt == bestCnt && (math.IsNaN(bestVal) || v < bestVal)) {
				bestCnt = cnt
				bestVal = v
			}
		}
		return bestVal, true, nil
	case "array.percentrank":
		if len(args) != 2 {
			return nil, true, fmt.Errorf("array.percentrank expects 2 args")
		}
		vals, err := arrayNumericValues(args[0], "array.percentrank")
		if err != nil {
			return nil, true, err
		}
		idxF, _ := toFloat(args[1])
		idx := int(idxF)
		if idx < 0 || idx >= len(vals) || len(vals) <= 1 {
			return math.NaN(), true, nil
		}
		cur := vals[idx]
		countLE := 0
		for _, v := range vals {
			if v <= cur {
				countLE++
			}
		}
		rank := float64(countLE-1) / float64(len(vals)-1) * 100.0
		return rank, true, nil
	case "array.percentile_linear_interpolation":
		if len(args) != 2 {
			return nil, true, fmt.Errorf("array.percentile_linear_interpolation expects 2 args")
		}
		vals, err := arrayNumericValues(args[0], "array.percentile_linear_interpolation")
		if err != nil {
			return nil, true, err
		}
		if len(vals) == 0 {
			return math.NaN(), true, nil
		}
		p, _ := toFloat(args[1])
		return percentileLinear(vals, p), true, nil
	case "array.percentile_nearest_rank", "array.percentile_neareast_rank":
		if len(args) != 2 {
			return nil, true, fmt.Errorf("array.percentile_nearest_rank expects 2 args")
		}
		vals, err := arrayNumericValues(args[0], "array.percentile_nearest_rank")
		if err != nil {
			return nil, true, err
		}
		if len(vals) == 0 {
			return math.NaN(), true, nil
		}
		p, _ := toFloat(args[1])
		return percentileNearest(vals, p), true, nil
	case "array.binary_search_leftmost":
		if len(args) != 2 {
			return nil, true, fmt.Errorf("array.binary_search_leftmost expects 2 args")
		}
		vals, err := arrayNumericValues(args[0], "array.binary_search_leftmost")
		if err != nil {
			return nil, true, err
		}
		target, _ := toFloat(args[1])
		idx := sort.Search(len(vals), func(i int) bool { return vals[i] >= target })
		if idx >= len(vals) || vals[idx] != target {
			return float64(-1), true, nil
		}
		return float64(idx), true, nil
	case "array.binary_search_rightmost":
		if len(args) != 2 {
			return nil, true, fmt.Errorf("array.binary_search_rightmost expects 2 args")
		}
		vals, err := arrayNumericValues(args[0], "array.binary_search_rightmost")
		if err != nil {
			return nil, true, err
		}
		target, _ := toFloat(args[1])
		idx := sort.Search(len(vals), func(i int) bool { return vals[i] > target }) - 1
		if idx < 0 || vals[idx] != target {
			return float64(-1), true, nil
		}
		return float64(idx), true, nil
	case "array.covariance":
		if len(args) != 2 && len(args) != 3 {
			return nil, true, fmt.Errorf("array.covariance expects 2 or 3 args")
		}
		xs, err := arrayNumericValues(args[0], "array.covariance")
		if err != nil {
			return nil, true, err
		}
		ys, err := arrayNumericValues(args[1], "array.covariance")
		if err != nil {
			return nil, true, err
		}
		if len(xs) == 0 || len(ys) == 0 || len(xs) != len(ys) {
			return math.NaN(), true, nil
		}
		biased := true
		if len(args) == 3 {
			biased = truthy(args[2])
		}
		n := len(xs)
		meanX := 0.0
		meanY := 0.0
		for i := 0; i < n; i++ {
			meanX += xs[i]
			meanY += ys[i]
		}
		meanX /= float64(n)
		meanY /= float64(n)
		cov := 0.0
		for i := 0; i < n; i++ {
			cov += (xs[i] - meanX) * (ys[i] - meanY)
		}
		den := float64(n)
		if !biased {
			if n < 2 {
				return math.NaN(), true, nil
			}
			den = float64(n - 1)
		}
		return cov / den, true, nil
	case "str.length":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("str.length expects 1 arg")
		}
		s, ok := args[0].(string)
		if !ok {
			return nil, true, fmt.Errorf("str.length requires string")
		}
		return float64(len([]rune(s))), true, nil
	case "str.upper":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("str.upper expects 1 arg")
		}
		s, ok := args[0].(string)
		if !ok {
			return nil, true, fmt.Errorf("str.upper requires string")
		}
		return strings.ToUpper(s), true, nil
	case "str.lower":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("str.lower expects 1 arg")
		}
		s, ok := args[0].(string)
		if !ok {
			return nil, true, fmt.Errorf("str.lower requires string")
		}
		return strings.ToLower(s), true, nil
	case "str.contains":
		if len(args) != 2 {
			return nil, true, fmt.Errorf("str.contains expects 2 args")
		}
		s, ok := args[0].(string)
		if !ok {
			return nil, true, fmt.Errorf("str.contains requires string")
		}
		sub, ok := args[1].(string)
		if !ok {
			return nil, true, fmt.Errorf("str.contains requires substring string")
		}
		return strings.Contains(s, sub), true, nil
	case "str.startswith":
		if len(args) != 2 {
			return nil, true, fmt.Errorf("str.startswith expects 2 args")
		}
		s, ok := args[0].(string)
		if !ok {
			return nil, true, fmt.Errorf("str.startswith requires string")
		}
		prefix, ok := args[1].(string)
		if !ok {
			return nil, true, fmt.Errorf("str.startswith requires prefix string")
		}
		return strings.HasPrefix(s, prefix), true, nil
	case "str.endswith":
		if len(args) != 2 {
			return nil, true, fmt.Errorf("str.endswith expects 2 args")
		}
		s, ok := args[0].(string)
		if !ok {
			return nil, true, fmt.Errorf("str.endswith requires string")
		}
		suffix, ok := args[1].(string)
		if !ok {
			return nil, true, fmt.Errorf("str.endswith requires suffix string")
		}
		return strings.HasSuffix(s, suffix), true, nil
	case "str.replace":
		if len(args) != 3 {
			return nil, true, fmt.Errorf("str.replace expects 3 args")
		}
		s, ok := args[0].(string)
		if !ok {
			return nil, true, fmt.Errorf("str.replace requires string")
		}
		old, ok := args[1].(string)
		if !ok {
			return nil, true, fmt.Errorf("str.replace requires old string")
		}
		repl, ok := args[2].(string)
		if !ok {
			return nil, true, fmt.Errorf("str.replace requires replacement string")
		}
		return strings.ReplaceAll(s, old, repl), true, nil
	case "str.substring":
		if len(args) != 2 && len(args) != 3 {
			return nil, true, fmt.Errorf("str.substring expects 2 or 3 args")
		}
		s, ok := args[0].(string)
		if !ok {
			return nil, true, fmt.Errorf("str.substring requires string")
		}
		runes := []rune(s)
		fromF, _ := toFloat(args[1])
		from := int(fromF)
		to := len(runes)
		if len(args) == 3 {
			toF, _ := toFloat(args[2])
			to = int(toF)
		}
		if from < 0 {
			from = 0
		}
		if to > len(runes) {
			to = len(runes)
		}
		if from > to {
			from = to
		}
		return string(runes[from:to]), true, nil
	case "str.split":
		if len(args) != 2 {
			return nil, true, fmt.Errorf("str.split expects 2 args")
		}
		s, ok := args[0].(string)
		if !ok {
			return nil, true, fmt.Errorf("str.split requires string")
		}
		sep, ok := args[1].(string)
		if !ok {
			return nil, true, fmt.Errorf("str.split requires separator string")
		}
		parts := strings.Split(s, sep)
		out := make([]interface{}, 0, len(parts))
		for _, p := range parts {
			out = append(out, p)
		}
		return out, true, nil
	case "str.format":
		if len(args) < 1 {
			return nil, true, fmt.Errorf("str.format expects at least 1 arg")
		}
		format, ok := args[0].(string)
		if !ok {
			return nil, true, fmt.Errorf("str.format requires format string")
		}
		return formatStringTemplate(format, args[1:]), true, nil
	case "na":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("na() expects 1 arg")
		}
		if _, ok := args[0].(bool); ok {
			return nil, true, fmt.Errorf("na() does not accept bool")
		}
		return isNA(args[0]), true, nil
	case "nz":
		if len(args) < 1 || len(args) > 2 {
			return nil, true, fmt.Errorf("nz() expects 1 or 2 args")
		}
		for _, a := range args {
			if _, ok := a.(bool); ok {
				return nil, true, fmt.Errorf("nz() does not accept bool")
			}
		}
		if !isNA(args[0]) {
			return args[0], true, nil
		}
		if len(args) == 2 {
			return args[1], true, nil
		}
		return float64(0), true, nil
	case "fixnan":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("fixnan() expects 1 arg")
		}
		if _, ok := args[0].(bool); ok {
			return nil, true, fmt.Errorf("fixnan() does not accept bool")
		}
		if isNA(args[0]) {
			return math.NaN(), true, nil
		}
		return args[0], true, nil
	case "math.abs", "abs":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("math.abs() expects 1 arg")
		}
		n, _ := toFloat(args[0])
		return math.Abs(n), true, nil
	case "math.max", "max":
		if len(args) < 1 {
			return nil, true, fmt.Errorf("math.max() expects at least 1 arg")
		}
		m, _ := toFloat(args[0])
		for i := 1; i < len(args); i++ {
			v, _ := toFloat(args[i])
			m = math.Max(m, v)
		}
		return m, true, nil
	case "math.min", "min":
		if len(args) < 1 {
			return nil, true, fmt.Errorf("math.min() expects at least 1 arg")
		}
		m, _ := toFloat(args[0])
		for i := 1; i < len(args); i++ {
			v, _ := toFloat(args[i])
			m = math.Min(m, v)
		}
		return m, true, nil
	case "math.round", "round":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("math.round() expects 1 arg")
		}
		n, _ := toFloat(args[0])
		return math.Round(n), true, nil
	case "math.floor", "floor":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("math.floor() expects 1 arg")
		}
		n, _ := toFloat(args[0])
		return math.Floor(n), true, nil
	case "math.ceil", "ceil":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("math.ceil() expects 1 arg")
		}
		n, _ := toFloat(args[0])
		return math.Ceil(n), true, nil
	case "math.pow", "pow":
		if len(args) != 2 {
			return nil, true, fmt.Errorf("math.pow() expects 2 args")
		}
		a, _ := toFloat(args[0])
		b, _ := toFloat(args[1])
		return math.Pow(a, b), true, nil
	case "math.sqrt", "sqrt":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("math.sqrt() expects 1 arg")
		}
		n, _ := toFloat(args[0])
		return math.Sqrt(n), true, nil
	case "math.log":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("math.log() expects 1 arg")
		}
		n, _ := toFloat(args[0])
		return math.Log(n), true, nil
	case "math.log10", "log10":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("math.log10() expects 1 arg")
		}
		n, _ := toFloat(args[0])
		return math.Log10(n), true, nil
	case "math.avg", "avg":
		if len(args) < 1 {
			return nil, true, fmt.Errorf("math.avg() expects at least 1 arg")
		}
		total := 0.0
		for _, a := range args {
			v, _ := toFloat(a)
			total += v
		}
		return total / float64(len(args)), true, nil
	case "math.sum", "sum":
		if len(args) < 1 {
			return nil, true, fmt.Errorf("math.sum() expects at least 1 arg")
		}
		total := 0.0
		for _, a := range args {
			v, _ := toFloat(a)
			total += v
		}
		return total, true, nil
	case "math.exp", "exp":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("math.exp() expects 1 arg")
		}
		n, _ := toFloat(args[0])
		return math.Exp(n), true, nil
	case "math.sin", "sin":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("math.sin() expects 1 arg")
		}
		n, _ := toFloat(args[0])
		return math.Sin(n), true, nil
	case "math.cos", "cos":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("math.cos() expects 1 arg")
		}
		n, _ := toFloat(args[0])
		return math.Cos(n), true, nil
	case "math.tan", "tan":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("math.tan() expects 1 arg")
		}
		n, _ := toFloat(args[0])
		return math.Tan(n), true, nil
	case "math.acos", "acos":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("math.acos() expects 1 arg")
		}
		n, _ := toFloat(args[0])
		return math.Acos(n), true, nil
	case "math.asin", "asin":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("math.asin() expects 1 arg")
		}
		n, _ := toFloat(args[0])
		return math.Asin(n), true, nil
	case "math.atan", "atan":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("math.atan() expects 1 arg")
		}
		n, _ := toFloat(args[0])
		return math.Atan(n), true, nil
	case "math.sign", "sign":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("math.sign() expects 1 arg")
		}
		n, _ := toFloat(args[0])
		if math.IsNaN(n) {
			return math.NaN(), true, nil
		}
		if n > 0 {
			return float64(1), true, nil
		}
		if n < 0 {
			return float64(-1), true, nil
		}
		return float64(0), true, nil
	case "math.todegrees", "todegrees":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("math.todegrees() expects 1 arg")
		}
		n, _ := toFloat(args[0])
		return n * 180 / math.Pi, true, nil
	case "math.toradians", "toradians":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("math.toradians() expects 1 arg")
		}
		n, _ := toFloat(args[0])
		return n * math.Pi / 180, true, nil
	case "math.random":
		switch len(args) {
		case 0:
			return rand.Float64(), true, nil
		case 1:
			maxV, _ := toFloat(args[0])
			return rand.Float64() * maxV, true, nil
		case 2:
			minV, _ := toFloat(args[0])
			maxV, _ := toFloat(args[1])
			if maxV < minV {
				minV, maxV = maxV, minV
			}
			return minV + rand.Float64()*(maxV-minV), true, nil
		default:
			return nil, true, fmt.Errorf("math.random() expects 0-2 args")
		}
	case "timeframe.change":
		return r.builtinTimeframeChange(args)
	case "timeframe.in_seconds":
		return r.builtinTimeframeInSeconds(args)
	case "timeframe.from_seconds":
		return r.builtinTimeframeFromSeconds(args)
	case "time":
		return r.builtinTime(args)
	case "time_close":
		return r.builtinTimeClose(args)
	case "timenow":
		return r.builtinTimeNow(args)
	case "time_tradingday":
		return r.builtinTimeTradingDay(args)
	case "timestamp":
		return r.builtinTimestamp(args)
	case "value_of":
		if len(args) != 2 {
			return nil, true, fmt.Errorf("value_of(symbol, value_type) expects 2 args")
		}
		symbol, ok := args[0].(string)
		if !ok || symbol == "" {
			return nil, true, errors.New("value_of requires non-empty symbol string")
		}
		valueType, ok := args[1].(string)
		if !ok || valueType == "" {
			return nil, true, errors.New("value_of requires non-empty value_type string")
		}
		return r.currentValue(symbol, valueType), true, nil
	case "close_of", "open_of", "high_of", "low_of":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("%s expects 1 arg", name)
		}
		symbol, ok := args[0].(string)
		if !ok || symbol == "" {
			return nil, true, fmt.Errorf("%s requires non-empty symbol string", name)
		}
		valueType := "close"
		switch name {
		case "open_of":
			valueType = "open"
		case "high_of":
			valueType = "high"
		case "low_of":
			valueType = "low"
		}
		return r.currentValue(symbol, valueType), true, nil
	case "atr", "ta.atr":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("atr(length) expects 1 arg")
		}
		lengthF, _ := toFloat(args[0])
		closeSeries, err := r.getSeries(r.activeSymbol, "close")
		if err != nil {
			return nil, true, err
		}
		return atrFromSeries(closeSeries, int(lengthF)), true, nil
	case "change", "ta.change":
		return r.builtinChange(rawArgs, args)
	case "highest", "ta.highest":
		return r.builtinHighest(rawArgs, args)
	case "lowest", "ta.lowest":
		return r.builtinLowest(rawArgs, args)
	case "stdev", "ta.stdev":
		return r.builtinStdev(rawArgs, args)
	case "correlation", "ta.correlation":
		return r.builtinCorrelation(rawArgs, args)
	case "sma", "ta.sma":
		return r.builtinSMA(rawArgs, args)
	case "ema", "ta.ema":
		return r.builtinEMA(rawArgs, args)
	case "rsi", "ta.rsi":
		return r.builtinRSI(rawArgs, args)
	case "crossover", "ta.crossover":
		return r.builtinCross(rawArgs, false)
	case "crossunder", "ta.crossunder":
		return r.builtinCross(rawArgs, true)
	case "cross", "ta.cross":
		return r.builtinCrossAny(rawArgs)
	case "rma", "ta.rma":
		return r.builtinRMA(rawArgs, args)
	case "wma", "ta.wma":
		return r.builtinWMA(rawArgs, args)
	case "swma", "ta.swma":
		return r.builtinSWMA(rawArgs, args)
	case "hma", "ta.hma":
		return r.builtinHMA(rawArgs, args)
	case "alma", "ta.alma":
		return r.builtinALMA(rawArgs, args)
	case "linreg", "ta.linreg":
		return r.builtinLinReg(rawArgs, args)
	case "vwma", "ta.vwma":
		return r.builtinVWMA(rawArgs, args)
	case "cci", "ta.cci":
		return r.builtinCCI(rawArgs, args)
	case "cmo", "ta.cmo":
		return r.builtinCMO(rawArgs, args)
	case "cog", "ta.cog":
		return r.builtinCOG(rawArgs, args)
	case "macd", "ta.macd":
		return r.builtinMACD(rawArgs, args)
	case "mom", "ta.mom":
		return r.builtinMOM(rawArgs, args)
	case "roc", "ta.roc":
		return r.builtinROC(rawArgs, args)
	case "barssince", "ta.barssince":
		return r.builtinBarsSince(rawArgs)
	case "cum", "ta.cum":
		return r.builtinCum(rawArgs)
	case "valuewhen", "ta.valuewhen":
		return r.builtinValueWhen(rawArgs, args)
	case "highestbars", "ta.highestbars":
		return r.builtinHighestBars(rawArgs, args)
	case "lowestbars", "ta.lowestbars":
		return r.builtinLowestBars(rawArgs, args)
	case "ta.max":
		return r.builtinTAMax(rawArgs)
	case "ta.min":
		return r.builtinTAMin(rawArgs)
	case "ta.median":
		return r.builtinTAMedian(rawArgs, args)
	case "ta.mode":
		return r.builtinTAMode(rawArgs, args)
	case "ta.percentile_linear_interpolation":
		return r.builtinPercentileLinearInterpolation(rawArgs, args)
	case "ta.percentile_nearest_rank":
		return r.builtinPercentileNearestRank(rawArgs, args)
	case "ta.percentrank":
		return r.builtinPercentRank(rawArgs, args)
	case "ta.range":
		return r.builtinTARange(rawArgs, args)
	case "ta.variance":
		return r.builtinTAVariance(rawArgs, args)
	case "ta.dev":
		return r.builtinTADev(rawArgs, args)
	case "ta.rising":
		return r.builtinTARising(rawArgs, args)
	case "ta.falling":
		return r.builtinTAFalling(rawArgs, args)
	case "tr", "ta.tr":
		return r.builtinTR(args)
	case "ta.pivothigh":
		return r.builtinPivotHigh(rawArgs, args)
	case "ta.pivotlow":
		return r.builtinPivotLow(rawArgs, args)
	case "ta.pivot_point_levels":
		return r.builtinPivotPointLevels(args)
	case "bb", "ta.bb":
		return r.builtinBB(rawArgs, args)
	case "bbw", "ta.bbw":
		return r.builtinBBW(rawArgs, args)
	case "kc", "ta.kc":
		return r.builtinKC(rawArgs, args)
	case "kcw", "ta.kcw":
		return r.builtinKCW(rawArgs, args)
	case "stoch", "ta.stoch":
		return r.builtinStoch(rawArgs, args)
	case "mfi", "ta.mfi":
		return r.builtinMFI(rawArgs, args)
	case "tsi", "ta.tsi":
		return r.builtinTSI(rawArgs, args)
	case "wpr", "ta.wpr":
		return r.builtinWPR(args)
	case "dmi", "ta.dmi":
		return r.builtinDMI(args)
	case "sar", "ta.sar":
		return r.builtinSAR(args)
	case "supertrend", "ta.supertrend":
		return r.builtinSupertrend(args)
	case "sma_of":
		return r.builtinSMAOf(args)
	case "ema_of":
		return r.builtinEMAOf(args)
	case "rsi_of":
		return r.builtinRSIOf(args)
	default:
		if strings.HasPrefix(name, "matrix.") {
			return r.callMatrixBuiltin(name, args)
		}
		return nil, false, nil
	}
}

func asArrayArg(v interface{}, fn string) ([]interface{}, error) {
	switch arr := v.(type) {
	case []interface{}:
		return arr, nil
	case *pineArray:
		return arr.items, nil
	default:
		return nil, fmt.Errorf("%s requires array", fn)
	}
}

func isPineArrayArg(v interface{}) bool {
	_, ok := v.(*pineArray)
	return ok
}

func arrayNumericValues(v interface{}, fn string) ([]float64, error) {
	arr, err := asArrayArg(v, fn)
	if err != nil {
		return nil, err
	}
	out := make([]float64, len(arr))
	for i, item := range arr {
		f, ok := toFloat(item)
		if !ok {
			return nil, fmt.Errorf("%s requires numeric array", fn)
		}
		out[i] = f
	}
	return out, nil
}

func arrayExtremaNth(v interface{}, nth int, max bool, fn string) (float64, error) {
	vals, err := arrayNumericValues(v, fn)
	if err != nil {
		return 0, err
	}
	if len(vals) == 0 || nth < 0 || nth >= len(vals) {
		return math.NaN(), nil
	}
	s := append([]float64(nil), vals...)
	if max {
		sort.Sort(sort.Reverse(sort.Float64Slice(s)))
	} else {
		sort.Float64s(s)
	}
	return s[nth], nil
}

func (r *Runtime) seriesHistory(symbol, valueType string) []float64 {
	ser, err := r.getSeries(symbol, valueType)
	if err != nil || ser == nil {
		return nil
	}
	eff := r.effectiveBarIndex()
	if eff < 0 {
		return nil
	}
	serLen := ser.Length()
	count := eff + 1
	if count > serLen {
		count = serLen
	}
	if count <= 0 {
		return nil
	}
	out := make([]float64, 0, count)
	for i := 0; i < count; i++ {
		idx := serLen - 1 - i
		out = append(out, ser.Last(idx))
	}
	return out
}

func (r *Runtime) seriesFromRawExpr(raw *Expr) (SeriesExtended, bool, error) {
	if raw == nil {
		return nil, false, fmt.Errorf("missing series expression")
	}
	switch raw.Kind {
	case "ident":
		if isPriceIdentifierName(raw.Name) {
			ser, err := r.getSeriesByIdentifier(r.activeSymbol, raw.Name)
			if err != nil {
				return nil, false, err
			}
			return ser, true, nil
		}
		return nil, false, nil
	case "call":
		if raw.Left == nil || raw.Left.Kind != "ident" {
			return nil, false, nil
		}
		switch raw.Left.Name {
		case "close_of", "open_of", "high_of", "low_of":
			if len(raw.Args) == 1 && raw.Args[0] != nil && raw.Args[0].Kind == "string" {
				valueType := "close"
				switch raw.Left.Name {
				case "open_of":
					valueType = "open"
				case "high_of":
					valueType = "high"
				case "low_of":
					valueType = "low"
				}
				ser, err := r.getSeries(raw.Args[0].String, valueType)
				if err != nil {
					return nil, false, err
				}
				return ser, true, nil
			}
		case "value_of":
			if len(raw.Args) == 2 && raw.Args[0] != nil && raw.Args[1] != nil && raw.Args[0].Kind == "string" && raw.Args[1].Kind == "string" {
				ser, err := r.getSeriesByIdentifier(raw.Args[0].String, raw.Args[1].String)
				if err != nil {
					return nil, false, err
				}
				return ser, true, nil
			}
		}
		return nil, false, nil
	case "unary":
		if raw.UOp != unaryOpNeg {
			return nil, false, nil
		}
		base, ok, err := r.seriesFromRawExpr(raw.Right)
		if err != nil || !ok {
			return nil, ok, err
		}
		return base.Mul(-1.0), true, nil
	case "binary":
		if !isArithmeticOpcode(raw.BOp) || raw.BOp == binaryOpMod {
			return nil, false, nil
		}
		leftSeries, leftOK, leftErr := r.seriesFromRawExpr(raw.Left)
		if leftErr != nil {
			return nil, true, leftErr
		}
		rightSeries, rightOK, rightErr := r.seriesFromRawExpr(raw.Right)
		if rightErr != nil {
			return nil, true, rightErr
		}
		if leftOK && rightOK {
			switch raw.BOp {
			case binaryOpAdd:
				return leftSeries.Add(rightSeries), true, nil
			case binaryOpSub:
				return leftSeries.Minus(rightSeries), true, nil
			case binaryOpMul:
				return leftSeries.Mul(rightSeries), true, nil
			case binaryOpDiv:
				return leftSeries.Div(rightSeries), true, nil
			}
			return nil, false, nil
		}
		leftConst, leftConstOK := exprConstFloat(raw.Left)
		rightConst, rightConstOK := exprConstFloat(raw.Right)
		if leftOK && rightConstOK {
			switch raw.BOp {
			case binaryOpAdd:
				return leftSeries.Add(rightConst), true, nil
			case binaryOpSub:
				return leftSeries.Minus(rightConst), true, nil
			case binaryOpMul:
				return leftSeries.Mul(rightConst), true, nil
			case binaryOpDiv:
				return leftSeries.Div(rightConst), true, nil
			}
			return nil, false, nil
		}
		if leftConstOK && rightOK {
			switch raw.BOp {
			case binaryOpAdd:
				return rightSeries.Add(leftConst), true, nil
			case binaryOpMul:
				return rightSeries.Mul(leftConst), true, nil
			case binaryOpSub, binaryOpDiv:
				left := wseries.SwitchIface(leftConst)
				if left == nil {
					return nil, false, nil
				}
				leftExt, ok := left.(SeriesExtended)
				if !ok {
					return nil, false, nil
				}
				if raw.BOp == binaryOpSub {
					return leftExt.Minus(rightSeries), true, nil
				}
				return leftExt.Div(rightSeries), true, nil
			}
		}
		return nil, false, nil
	default:
		return nil, false, nil
	}
}

func (r *Runtime) valueAtOffset(symbol, valueType string, offset int) float64 {
	if offset < 0 {
		return math.NaN()
	}
	return r.valueAt(symbol, valueType, r.evalOffset+offset)
}

type indicatorBaseState struct {
	lastBar int
}

type rollingWindowState struct {
	buf   []float64
	start int
	count int
	cap   int
}

func newRollingWindowState(size int) rollingWindowState {
	if size < 1 {
		size = 1
	}
	return rollingWindowState{buf: make([]float64, size), cap: size}
}

func (w *rollingWindowState) push(v float64) (float64, bool) {
	if w.count < w.cap {
		idx := (w.start + w.count) % w.cap
		w.buf[idx] = v
		w.count++
		return 0, false
	}
	old := w.buf[w.start]
	w.buf[w.start] = v
	w.start = (w.start + 1) % w.cap
	return old, true
}

func (w *rollingWindowState) len() int { return w.count }

type smaIndicatorState struct {
	indicatorBaseState
	window rollingWindowState
	sum    float64
}

func newSMAIndicatorState(length int) *smaIndicatorState {
	return &smaIndicatorState{
		indicatorBaseState: indicatorBaseState{lastBar: -1},
		window:             newRollingWindowState(length),
		sum:                0,
	}
}

func (s *smaIndicatorState) Update(v float64) {
	old, replaced := s.window.push(v)
	s.sum += v
	if replaced {
		s.sum -= old
	}
}

func (s *smaIndicatorState) Value() float64 {
	if s.window.len() < s.window.cap {
		return math.NaN()
	}
	return s.sum / float64(s.window.cap)
}

type emaIndicatorState struct {
	indicatorBaseState
	window int
	count  int
	k      float64
	value  float64
	has    bool
}

func newEMAIndicatorState(length int) *emaIndicatorState {
	return &emaIndicatorState{indicatorBaseState: indicatorBaseState{lastBar: -1}, window: length, k: 2.0 / (float64(length) + 1.0)}
}

func (s *emaIndicatorState) Update(v float64) {
	if !s.has {
		s.value = v
		s.has = true
		s.count = 1
		return
	}
	s.value = v*s.k + s.value*(1.0-s.k)
	s.count++
}

func (s *emaIndicatorState) Value() float64 {
	if s.window <= 0 || s.count < s.window {
		return math.NaN()
	}
	return s.value
}

type rmaIndicatorState struct {
	indicatorBaseState
	window int
	count  int
	sum    float64
	value  float64
	has    bool
}

func newRMAIndicatorState(length int) *rmaIndicatorState {
	return &rmaIndicatorState{indicatorBaseState: indicatorBaseState{lastBar: -1}, window: length}
}

func (s *rmaIndicatorState) Update(v float64) {
	s.count++
	if !s.has {
		s.sum += v
		if s.count < s.window {
			return
		}
		s.value = s.sum / float64(s.window)
		s.has = true
		return
	}
	s.value = (s.value*float64(s.window-1) + v) / float64(s.window)
}

func (s *rmaIndicatorState) Value() float64 {
	if s.window <= 0 || !s.has {
		return math.NaN()
	}
	return s.value
}

type rsiIndicatorState struct {
	indicatorBaseState
	window    int
	hasPrev   bool
	prev      float64
	diffs     rollingWindowState
	sumGain   float64
	sumLoss   float64
	priceSeen int
}

func newRSIIndicatorState(length int) *rsiIndicatorState {
	return &rsiIndicatorState{indicatorBaseState: indicatorBaseState{lastBar: -1}, window: length, diffs: newRollingWindowState(length)}
}

func (s *rsiIndicatorState) removeDiff(d float64) {
	if d > 0 {
		s.sumGain -= d
	} else if d < 0 {
		s.sumLoss -= -d
	}
}

func (s *rsiIndicatorState) addDiff(d float64) {
	if d > 0 {
		s.sumGain += d
	} else if d < 0 {
		s.sumLoss += -d
	}
}

func (s *rsiIndicatorState) Update(v float64) {
	s.priceSeen++
	if !s.hasPrev {
		s.prev = v
		s.hasPrev = true
		return
	}
	d := v - s.prev
	s.prev = v
	old, replaced := s.diffs.push(d)
	if replaced {
		s.removeDiff(old)
	}
	s.addDiff(d)
}

func (s *rsiIndicatorState) Value() float64 {
	if s.window <= 0 || s.priceSeen <= s.window || s.diffs.len() < s.window {
		return math.NaN()
	}
	avgGain := s.sumGain / float64(s.window)
	avgLoss := s.sumLoss / float64(s.window)
	if avgLoss == 0 {
		if avgGain == 0 {
			return 50.0
		}
		return 100.0
	}
	rs := avgGain / avgLoss
	return 100.0 - (100.0 / (1.0 + rs))
}

type macdIndicatorState struct {
	indicatorBaseState
	fast   *emaIndicatorState
	slow   *emaIndicatorState
	signal *emaIndicatorState
	macd   float64
	sig    float64
	hist   float64
}

type bbIndicatorState struct {
	indicatorBaseState
	window rollingWindowState
	sum    float64
	sumSq  float64
}

type extremaIndicatorState struct {
	indicatorBaseState
	window rollingWindowState
	isMax  bool
}

type extremaStateKey struct {
	raw       *Expr
	length    int
	isMax     bool
	symbol    string
	valueType string
}

func newBBIndicatorState(length int) *bbIndicatorState {
	return &bbIndicatorState{
		indicatorBaseState: indicatorBaseState{lastBar: -1},
		window:             newRollingWindowState(length),
		sum:                0,
		sumSq:              0,
	}
}

func newExtremaIndicatorState(length int, isMax bool) *extremaIndicatorState {
	return &extremaIndicatorState{
		indicatorBaseState: indicatorBaseState{lastBar: -1},
		window:             newRollingWindowState(length),
		isMax:              isMax,
	}
}

func (s *extremaIndicatorState) Update(v float64) {
	_, _ = s.window.push(v)
}

func (s *extremaIndicatorState) Value() float64 {
	if s.window.len() < s.window.cap {
		return math.NaN()
	}
	idx := s.window.start
	best := s.window.buf[idx]
	for i := 1; i < s.window.count; i++ {
		idx = (s.window.start + i) % s.window.cap
		v := s.window.buf[idx]
		if s.isMax {
			if v > best {
				best = v
			}
		} else {
			if v < best {
				best = v
			}
		}
	}
	return best
}

func (s *bbIndicatorState) Update(v float64) {
	old, replaced := s.window.push(v)
	s.sum += v
	s.sumSq += v * v
	if replaced {
		s.sum -= old
		s.sumSq -= old * old
	}
}

func (s *bbIndicatorState) Values(mult float64) (float64, float64, float64) {
	if s.window.len() < s.window.cap {
		na := math.NaN()
		return na, na, na
	}
	n := float64(s.window.cap)
	basis := s.sum / n
	variance := (s.sumSq / n) - basis*basis
	if variance < 0 {
		variance = 0
	}
	dev := math.Sqrt(variance)
	upper := basis + mult*dev
	lower := basis - mult*dev
	return basis, upper, lower
}

func newMACDIndicatorState(fast, slow, sig int) *macdIndicatorState {
	return &macdIndicatorState{
		indicatorBaseState: indicatorBaseState{lastBar: -1},
		fast:               newEMAIndicatorState(fast),
		slow:               newEMAIndicatorState(slow),
		signal:             newEMAIndicatorState(sig),
		macd:               math.NaN(),
		sig:                math.NaN(),
		hist:               math.NaN(),
	}
}

func (s *macdIndicatorState) Update(v float64) {
	s.fast.Update(v)
	s.slow.Update(v)
	fastV := s.fast.Value()
	slowV := s.slow.Value()
	if math.IsNaN(fastV) || math.IsNaN(slowV) {
		s.macd = math.NaN()
		s.sig = math.NaN()
		s.hist = math.NaN()
		return
	}
	s.macd = fastV - slowV
	s.signal.Update(s.macd)
	s.sig = s.signal.Value()
	if math.IsNaN(s.sig) {
		s.hist = math.NaN()
		return
	}
	s.hist = s.macd - s.sig
}

func (r *Runtime) indicatorStateKey(name string, raw *Expr, params ...interface{}) string {
	b := strings.Builder{}
	b.WriteString(name)
	b.WriteString("|")
	b.WriteString(r.activeSymbol)
	b.WriteString("|")
	b.WriteString(r.activeValueType)
	b.WriteString("|")
	b.WriteString(fmt.Sprintf("%p", raw))
	for _, p := range params {
		b.WriteString("|")
		b.WriteString(fmt.Sprint(p))
	}
	return b.String()
}

func (r *Runtime) evalCurrentFloat(raw *Expr) (float64, error) {
	v, err := r.eval(raw)
	if err != nil {
		return math.NaN(), err
	}
	f, _ := toFloat(v)
	return f, nil
}

func (r *Runtime) builtinSMA(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if len(args) != 2 || len(rawArgs) != 2 {
		return nil, true, fmt.Errorf("sma(series, length) expects 2 args")
	}
	lenF, _ := toFloat(args[1])
	length := int(lenF)
	if length <= 0 {
		return math.NaN(), true, nil
	}
	if r.evalOffset == 0 {
		key := r.indicatorStateKey("sma", rawArgs[0], length)
		stAny, ok := r.indicatorState[key]
		if !ok {
			stAny = newSMAIndicatorState(length)
			r.indicatorState[key] = stAny
		}
		st, ok := stAny.(*smaIndicatorState)
		if ok {
			if st.lastBar != r.barIndex {
				v, err := r.evalCurrentFloat(rawArgs[0])
				if err != nil {
					return nil, true, err
				}
				st.Update(v)
				st.lastBar = r.barIndex
			}
			return st.Value(), true, nil
		}
	}
	if !disableWindowOptimizations {
		window, pooled, err := r.seriesWindowFromExpr(rawArgs[0], length)
		if err != nil {
			return nil, true, err
		}
		if pooled {
			defer releaseFloat64Slice(window)
		}
		if len(window) < length {
			return math.NaN(), true, nil
		}
		sum := 0.0
		for _, v := range window {
			sum += v
		}
		return sum / float64(length), true, nil
	}
	seriesVals, err := r.seriesFromExpr(rawArgs[0])
	if err != nil {
		return nil, true, err
	}
	return smaFromSeries(seriesVals, length), true, nil
}

func (r *Runtime) builtinEMA(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if len(args) != 2 || len(rawArgs) != 2 {
		return nil, true, fmt.Errorf("ema(series, length) expects 2 args")
	}
	lenF, _ := toFloat(args[1])
	length := int(lenF)
	if length <= 0 {
		return math.NaN(), true, nil
	}
	if r.evalOffset == 0 {
		key := r.indicatorStateKey("ema", rawArgs[0], length)
		stAny, ok := r.indicatorState[key]
		if !ok {
			stAny = newEMAIndicatorState(length)
			r.indicatorState[key] = stAny
		}
		st, ok := stAny.(*emaIndicatorState)
		if ok {
			if st.lastBar != r.barIndex {
				v, err := r.evalCurrentFloat(rawArgs[0])
				if err != nil {
					return nil, true, err
				}
				st.Update(v)
				st.lastBar = r.barIndex
			}
			return st.Value(), true, nil
		}
	}
	seriesVals, err := r.seriesFromExpr(rawArgs[0])
	if err != nil {
		return nil, true, err
	}
	return emaFromSeries(seriesVals, length), true, nil
}

func (r *Runtime) builtinRSI(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if len(args) != 2 || len(rawArgs) != 2 {
		return nil, true, fmt.Errorf("rsi(series, length) expects 2 args")
	}
	lenF, _ := toFloat(args[1])
	length := int(lenF)
	if length <= 0 {
		return math.NaN(), true, nil
	}
	if r.evalOffset == 0 {
		key := r.indicatorStateKey("rsi", rawArgs[0], length)
		stAny, ok := r.indicatorState[key]
		if !ok {
			stAny = newRSIIndicatorState(length)
			r.indicatorState[key] = stAny
		}
		st, ok := stAny.(*rsiIndicatorState)
		if ok {
			if st.lastBar != r.barIndex {
				v, err := r.evalCurrentFloat(rawArgs[0])
				if err != nil {
					return nil, true, err
				}
				st.Update(v)
				st.lastBar = r.barIndex
			}
			return st.Value(), true, nil
		}
	}
	seriesVals, err := r.seriesFromExpr(rawArgs[0])
	if err != nil {
		return nil, true, err
	}
	return rsiFromSeries(seriesVals, length), true, nil
}

func (r *Runtime) builtinRMA(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if len(args) != 2 || len(rawArgs) != 2 {
		return nil, true, fmt.Errorf("rma(series, length) expects 2 args")
	}
	lenF, _ := toFloat(args[1])
	length := int(lenF)
	if length <= 0 {
		return math.NaN(), true, nil
	}
	if r.evalOffset == 0 {
		key := r.indicatorStateKey("rma", rawArgs[0], length)
		stAny, ok := r.indicatorState[key]
		if !ok {
			stAny = newRMAIndicatorState(length)
			r.indicatorState[key] = stAny
		}
		st, ok := stAny.(*rmaIndicatorState)
		if ok {
			if st.lastBar != r.barIndex {
				v, err := r.evalCurrentFloat(rawArgs[0])
				if err != nil {
					return nil, true, err
				}
				st.Update(v)
				st.lastBar = r.barIndex
			}
			return st.Value(), true, nil
		}
	}
	seriesVals, err := r.seriesFromExpr(rawArgs[0])
	if err != nil {
		return nil, true, err
	}
	return rmaFromSeries(seriesVals, length), true, nil
}

func (r *Runtime) builtinWMA(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if len(args) != 2 || len(rawArgs) != 2 {
		return nil, true, fmt.Errorf("wma(series, length) expects 2 args")
	}
	lenF, _ := toFloat(args[1])
	length := int(lenF)
	if !disableWindowOptimizations {
		window, pooled, err := r.seriesWindowFromExpr(rawArgs[0], length)
		if err != nil {
			return nil, true, err
		}
		if pooled {
			defer releaseFloat64Slice(window)
		}
		if length <= 0 || len(window) < length {
			return math.NaN(), true, nil
		}
		den := float64(length*(length+1)) / 2.0
		num := 0.0
		for i, v := range window {
			num += v * float64(i+1)
		}
		return num / den, true, nil
	}
	seriesVals, err := r.seriesFromExpr(rawArgs[0])
	if err != nil {
		return nil, true, err
	}
	return wmaFromSeries(seriesVals, length), true, nil
}

func (r *Runtime) builtinSWMA(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if len(args) != 1 || len(rawArgs) != 1 {
		return nil, true, fmt.Errorf("swma(series) expects 1 arg")
	}
	if !disableWindowOptimizations {
		window, pooled, err := r.seriesWindowFromExpr(rawArgs[0], 4)
		if err != nil {
			return nil, true, err
		}
		if pooled {
			defer releaseFloat64Slice(window)
		}
		if len(window) < 4 {
			return math.NaN(), true, nil
		}
		return (window[0] + 2*window[1] + 2*window[2] + window[3]) / 6.0, true, nil
	}
	seriesVals, err := r.seriesFromExpr(rawArgs[0])
	if err != nil {
		return nil, true, err
	}
	return swmaFromSeries(seriesVals), true, nil
}

func (r *Runtime) builtinHMA(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if len(args) != 2 || len(rawArgs) != 2 {
		return nil, true, fmt.Errorf("hma(series, length) expects 2 args")
	}
	seriesVals, err := r.seriesFromExpr(rawArgs[0])
	if err != nil {
		return nil, true, err
	}
	lenF, _ := toFloat(args[1])
	return hmaFromSeries(seriesVals, int(lenF)), true, nil
}

func (r *Runtime) builtinALMA(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if (len(args) != 4 && len(args) != 5) || (len(rawArgs) != 4 && len(rawArgs) != 5) {
		return nil, true, fmt.Errorf("alma(series, length, offset, sigma, [floor]) expects 4 or 5 args")
	}
	lenF, _ := toFloat(args[1])
	offset, _ := toFloat(args[2])
	sigma, _ := toFloat(args[3])
	floorFlag := false
	if len(args) == 5 {
		floorFlag = truthy(args[4])
	}
	length := int(lenF)
	if !disableWindowOptimizations {
		window, pooled, err := r.seriesWindowFromExpr(rawArgs[0], length)
		if err != nil {
			return nil, true, err
		}
		if pooled {
			defer releaseFloat64Slice(window)
		}
		return almaFromWindow(window, length, offset, sigma, floorFlag), true, nil
	}
	seriesVals, err := r.seriesFromExpr(rawArgs[0])
	if err != nil {
		return nil, true, err
	}
	return almaFromSeries(seriesVals, length, offset, sigma, floorFlag), true, nil
}

func (r *Runtime) builtinLinReg(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if len(args) != 3 || len(rawArgs) != 3 {
		return nil, true, fmt.Errorf("linreg(series, length, offset) expects 3 args")
	}
	lenF, _ := toFloat(args[1])
	offF, _ := toFloat(args[2])
	length := int(lenF)
	offset := int(offF)
	if !disableWindowOptimizations {
		window, pooled, err := r.seriesWindowFromExpr(rawArgs[0], length)
		if err != nil {
			return nil, true, err
		}
		if pooled {
			defer releaseFloat64Slice(window)
		}
		return linregFromWindow(window, length, offset), true, nil
	}
	seriesVals, err := r.seriesFromExpr(rawArgs[0])
	if err != nil {
		return nil, true, err
	}
	return linregFromSeries(seriesVals, length, offset), true, nil
}

func (r *Runtime) builtinVWMA(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if len(args) != 2 || len(rawArgs) != 2 {
		return nil, true, fmt.Errorf("vwma(series, length) expects 2 args")
	}
	lenF, _ := toFloat(args[1])
	length := int(lenF)
	if !disableWindowOptimizations {
		srcWindow, srcPooled, err := r.seriesWindowFromExpr(rawArgs[0], length)
		if err != nil {
			return nil, true, err
		}
		if srcPooled {
			defer releaseFloat64Slice(srcWindow)
		}
		volWindow, volPooled, err := r.seriesWindowForValueType(r.activeSymbol, "volume", length)
		if err != nil {
			return nil, true, err
		}
		if volPooled {
			defer releaseFloat64Slice(volWindow)
		}
		return vwmaFromWindow(srcWindow, volWindow, length), true, nil
	}
	src, err := r.seriesFromExpr(rawArgs[0])
	if err != nil {
		return nil, true, err
	}
	vol, err := r.getSeries(r.activeSymbol, "volume")
	if err != nil {
		return nil, true, err
	}
	return vwmaFromSeries(src, vol, length), true, nil
}

func (r *Runtime) builtinCCI(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if len(args) != 2 || len(rawArgs) != 2 {
		return nil, true, fmt.Errorf("cci(series, length) expects 2 args")
	}
	lenF, _ := toFloat(args[1])
	length := int(lenF)
	if !disableWindowOptimizations {
		window, pooled, err := r.seriesWindowFromExpr(rawArgs[0], length)
		if err != nil {
			return nil, true, err
		}
		if pooled {
			defer releaseFloat64Slice(window)
		}
		return cciFromWindow(window, length), true, nil
	}
	seriesVals, err := r.seriesFromExpr(rawArgs[0])
	if err != nil {
		return nil, true, err
	}
	return cciFromSeries(seriesVals, length), true, nil
}

func (r *Runtime) builtinCMO(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if len(args) != 2 || len(rawArgs) != 2 {
		return nil, true, fmt.Errorf("cmo(series, length) expects 2 args")
	}
	lenF, _ := toFloat(args[1])
	length := int(lenF)
	if !disableWindowOptimizations {
		window, pooled, err := r.seriesWindowFromExpr(rawArgs[0], length+1)
		if err != nil {
			return nil, true, err
		}
		if pooled {
			defer releaseFloat64Slice(window)
		}
		return cmoFromWindow(window, length), true, nil
	}
	seriesVals, err := r.seriesFromExpr(rawArgs[0])
	if err != nil {
		return nil, true, err
	}
	return cmoFromSeries(seriesVals, length), true, nil
}

func (r *Runtime) builtinCOG(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if len(args) != 2 || len(rawArgs) != 2 {
		return nil, true, fmt.Errorf("cog(series, length) expects 2 args")
	}
	lenF, _ := toFloat(args[1])
	length := int(lenF)
	if !disableWindowOptimizations {
		window, pooled, err := r.seriesWindowFromExpr(rawArgs[0], length)
		if err != nil {
			return nil, true, err
		}
		if pooled {
			defer releaseFloat64Slice(window)
		}
		return cogFromWindow(window, length), true, nil
	}
	seriesVals, err := r.seriesFromExpr(rawArgs[0])
	if err != nil {
		return nil, true, err
	}
	return cogFromSeries(seriesVals, length), true, nil
}

func (r *Runtime) builtinMACD(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if len(args) != 4 || len(rawArgs) != 4 {
		return nil, true, fmt.Errorf("macd(series, fastlen, slowlen, siglen) expects 4 args")
	}
	fastF, _ := toFloat(args[1])
	slowF, _ := toFloat(args[2])
	sigF, _ := toFloat(args[3])
	fastLen := int(fastF)
	slowLen := int(slowF)
	sigLen := int(sigF)
	if fastLen <= 0 || slowLen <= 0 || sigLen <= 0 {
		na := math.NaN()
		return []interface{}{na, na, na}, true, nil
	}
	if r.evalOffset == 0 {
		key := r.indicatorStateKey("macd", rawArgs[0], fastLen, slowLen, sigLen)
		stAny, ok := r.indicatorState[key]
		if !ok {
			stAny = newMACDIndicatorState(fastLen, slowLen, sigLen)
			r.indicatorState[key] = stAny
		}
		st, ok := stAny.(*macdIndicatorState)
		if ok {
			if st.lastBar != r.barIndex {
				v, err := r.evalCurrentFloat(rawArgs[0])
				if err != nil {
					return nil, true, err
				}
				st.Update(v)
				st.lastBar = r.barIndex
			}
			return []interface{}{st.macd, st.sig, st.hist}, true, nil
		}
	}
	seriesVals, err := r.seriesFromExpr(rawArgs[0])
	if err != nil {
		return nil, true, err
	}
	macd, sig, hist := macdFromSeries(seriesVals, fastLen, slowLen, sigLen)
	return []interface{}{macd, sig, hist}, true, nil
}

func stddevFromWindow(window []float64, biased bool) float64 {
	if len(window) == 0 {
		return math.NaN()
	}
	mean := 0.0
	for _, v := range window {
		mean += v
	}
	mean /= float64(len(window))
	ss := 0.0
	for _, v := range window {
		d := v - mean
		ss += d * d
	}
	den := float64(len(window))
	if !biased {
		if len(window) < 2 {
			return math.NaN()
		}
		den = float64(len(window) - 1)
	}
	return math.Sqrt(ss / den)
}

func (r *Runtime) builtinBB(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if len(rawArgs) != 3 || len(args) != 3 {
		return nil, true, fmt.Errorf("bb(source, length, mult) expects 3 args")
	}
	lf, _ := toFloat(args[1])
	mult, _ := toFloat(args[2])
	l := int(lf)
	if l <= 0 {
		na := math.NaN()
		return []interface{}{na, na, na}, true, nil
	}

	if r.evalOffset == 0 && !disableIncrementalBB {
		key := r.indicatorStateKey("bb", rawArgs[0], l)
		stAny, ok := r.indicatorState[key]
		if !ok {
			stAny = newBBIndicatorState(l)
			r.indicatorState[key] = stAny
		}
		st, ok := stAny.(*bbIndicatorState)
		if ok {
			if st.lastBar != r.barIndex {
				v, err := r.evalCurrentFloat(rawArgs[0])
				if err != nil {
					return nil, true, err
				}
				st.Update(v)
				st.lastBar = r.barIndex
			}
			basis, upper, lower := st.Values(mult)
			return []interface{}{basis, upper, lower}, true, nil
		}
	}

	vals, err := r.seriesFromExpr(rawArgs[0])
	if err != nil {
		return nil, true, err
	}
	if vals.Length() < l {
		na := math.NaN()
		return []interface{}{na, na, na}, true, nil
	}
	basis := smaFromSeries(vals, l)
	window, pooled := seriesWindowFromSeries(vals, l)
	if pooled {
		defer releaseFloat64Slice(window)
	}
	if len(window) < l {
		na := math.NaN()
		return []interface{}{na, na, na}, true, nil
	}
	dev := stddevFromWindow(window, true)
	upper := basis + mult*dev
	lower := basis - mult*dev
	return []interface{}{basis, upper, lower}, true, nil
}

func (r *Runtime) builtinBBW(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	b, _, err := r.builtinBB(rawArgs, args)
	if err != nil {
		return nil, true, err
	}
	t, ok := b.([]interface{})
	if !ok || len(t) != 3 {
		return math.NaN(), true, nil
	}
	basis, _ := toFloat(t[0])
	upper, _ := toFloat(t[1])
	lower, _ := toFloat(t[2])
	if basis == 0 || math.IsNaN(basis) || math.IsNaN(upper) || math.IsNaN(lower) {
		return math.NaN(), true, nil
	}
	return (upper - lower) / basis, true, nil
}

func trueRangeSeries(high, low, close []float64) []float64 {
	n := len(high)
	if len(low) < n {
		n = len(low)
	}
	if len(close) < n {
		n = len(close)
	}
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		if i == 0 {
			out[i] = high[i] - low[i]
			continue
		}
		a := high[i] - low[i]
		b := math.Abs(high[i] - close[i-1])
		c := math.Abs(low[i] - close[i-1])
		out[i] = math.Max(a, math.Max(b, c))
	}
	return out
}

func emaSeries(vals []float64, length int) []float64 {
	out := make([]float64, len(vals))
	if length <= 0 || len(vals) == 0 {
		for i := range out {
			out[i] = math.NaN()
		}
		return out
	}
	k := 2.0 / (float64(length) + 1.0)
	out[0] = vals[0]
	for i := 1; i < len(vals); i++ {
		if math.IsNaN(vals[i]) {
			out[i] = out[i-1]
			continue
		}
		out[i] = vals[i]*k + out[i-1]*(1.0-k)
	}
	return out
}

func rmaSeries(vals []float64, length int) []float64 {
	out := make([]float64, len(vals))
	if length <= 0 || len(vals) == 0 {
		for i := range out {
			out[i] = math.NaN()
		}
		return out
	}
	alpha := 1.0 / float64(length)
	out[0] = vals[0]
	for i := 1; i < len(vals); i++ {
		if math.IsNaN(vals[i]) {
			out[i] = out[i-1]
			continue
		}
		out[i] = alpha*vals[i] + (1.0-alpha)*out[i-1]
	}
	return out
}

func hlRangeSeries(high, low []float64) []float64 {
	n := len(high)
	if len(low) < n {
		n = len(low)
	}
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		out[i] = high[i] - low[i]
	}
	return out
}

func (r *Runtime) builtinKC(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if (len(rawArgs) != 3 && len(rawArgs) != 4) || (len(args) != 3 && len(args) != 4) {
		return nil, true, fmt.Errorf("kc(source, length, mult, [useTrueRange]) expects 3 or 4 args")
	}
	src, err := r.seriesFromExpr(rawArgs[0])
	if err != nil {
		return nil, true, err
	}
	lf, _ := toFloat(args[1])
	mult, _ := toFloat(args[2])
	l := int(lf)
	if l <= 0 || src.Length() < l {
		na := math.NaN()
		return []interface{}{na, na, na}, true, nil
	}
	useTR := true
	if len(args) == 4 {
		useTR = truthy(args[3])
	}
	h, err := r.getSeries(r.activeSymbol, "high")
	if err != nil {
		return nil, true, err
	}
	lo, err := r.getSeries(r.activeSymbol, "low")
	if err != nil {
		return nil, true, err
	}
	c, err := r.getSeries(r.activeSymbol, "close")
	if err != nil {
		return nil, true, err
	}
	rangeSeries := hlRangeSeriesFromSeries(r.seriesAtEvalSeries(h), r.seriesAtEvalSeries(lo))
	if useTR {
		rangeSeries = trueRangeSeriesFromSeries(r.seriesAtEvalSeries(h), r.seriesAtEvalSeries(lo), r.seriesAtEvalSeries(c))
	}
	basis := emaFromSeries(src, l)
	rangeEMA := emaFromSeries(rangeSeries, l)
	upper := basis + mult*rangeEMA
	lower := basis - mult*rangeEMA
	return []interface{}{basis, upper, lower}, true, nil
}

func trueRangeSeriesFromSeries(high, low, close SeriesExtended) SeriesExtended {
	n := seriesLen(high)
	if seriesLen(low) < n {
		n = seriesLen(low)
	}
	if seriesLen(close) < n {
		n = seriesLen(close)
	}
	q := wseries.NewQueue(n)
	for i := 0; i < n; i++ {
		hv := seriesChronoValue(high, i)
		lv := seriesChronoValue(low, i)
		if i == 0 {
			q.Update(hv - lv)
			continue
		}
		prevC := seriesChronoValue(close, i-1)
		a := hv - lv
		b := math.Abs(hv - prevC)
		c := math.Abs(lv - prevC)
		q.Update(math.Max(a, math.Max(b, c)))
	}
	return q
}

func hlRangeSeriesFromSeries(high, low SeriesExtended) SeriesExtended {
	n := seriesLen(high)
	if seriesLen(low) < n {
		n = seriesLen(low)
	}
	q := wseries.NewQueue(n)
	for i := 0; i < n; i++ {
		q.Update(seriesChronoValue(high, i) - seriesChronoValue(low, i))
	}
	return q
}

func (r *Runtime) builtinKCW(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	v, _, err := r.builtinKC(rawArgs, args)
	if err != nil {
		return nil, true, err
	}
	t, ok := v.([]interface{})
	if !ok || len(t) != 3 {
		return math.NaN(), true, nil
	}
	basis, _ := toFloat(t[0])
	upper, _ := toFloat(t[1])
	lower, _ := toFloat(t[2])
	if basis == 0 || math.IsNaN(basis) || math.IsNaN(upper) || math.IsNaN(lower) {
		return math.NaN(), true, nil
	}
	return (upper - lower) / basis, true, nil
}

func (r *Runtime) builtinStoch(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if len(rawArgs) != 4 || len(args) != 4 {
		return nil, true, fmt.Errorf("stoch(source, high, low, length) expects 4 args")
	}
	src, err := r.seriesFromExpr(rawArgs[0])
	if err != nil {
		return nil, true, err
	}
	highVals, err := r.seriesFromExpr(rawArgs[1])
	if err != nil {
		return nil, true, err
	}
	lowVals, err := r.seriesFromExpr(rawArgs[2])
	if err != nil {
		return nil, true, err
	}
	lf, _ := toFloat(args[3])
	l := int(lf)
	if l <= 0 || src.Length() < l || highVals.Length() < l || lowVals.Length() < l {
		return math.NaN(), true, nil
	}
	hh := highVals.Last(0)
	ll := lowVals.Last(0)
	for off := 1; off < l; off++ {
		hv := highVals.Last(off)
		lv := lowVals.Last(off)
		if hv > hh {
			hh = hv
		}
		if lv < ll {
			ll = lv
		}
	}
	den := hh - ll
	if den == 0 {
		return math.NaN(), true, nil
	}
	cur := src.Last(0)
	return 100.0 * (cur - ll) / den, true, nil
}

func (r *Runtime) builtinMFI(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if len(rawArgs) != 2 || len(args) != 2 {
		return nil, true, fmt.Errorf("mfi(source, length) expects 2 args")
	}
	src, err := r.seriesFromExpr(rawArgs[0])
	if err != nil {
		return nil, true, err
	}
	vol, err := r.getSeries(r.activeSymbol, "volume")
	if err != nil {
		return nil, true, err
	}
	vol = r.seriesAtEvalSeries(vol)
	lf, _ := toFloat(args[1])
	l := int(lf)
	n := src.Length()
	if vol.Length() < n {
		n = vol.Length()
	}
	if l <= 0 || n <= l {
		return math.NaN(), true, nil
	}
	start := n - l
	if start < 1 {
		start = 1
	}
	pos := 0.0
	neg := 0.0
	for i := start; i < n; i++ {
		curSrc := seriesChronoValue(src, i)
		prevSrc := seriesChronoValue(src, i-1)
		flow := curSrc * seriesChronoValue(vol, i)
		if curSrc > prevSrc {
			pos += flow
		} else if curSrc < prevSrc {
			neg += flow
		}
	}
	if neg == 0 {
		return 100.0, true, nil
	}
	if pos == 0 {
		return 0.0, true, nil
	}
	mr := pos / neg
	return 100.0 - 100.0/(1.0+mr), true, nil
}

func (r *Runtime) builtinTSI(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if len(rawArgs) != 3 || len(args) != 3 {
		return nil, true, fmt.Errorf("tsi(source, short_length, long_length) expects 3 args")
	}
	src, err := r.seriesFromExpr(rawArgs[0])
	if err != nil {
		return nil, true, err
	}
	shortF, _ := toFloat(args[1])
	longF, _ := toFloat(args[2])
	shortLen := int(shortF)
	longLen := int(longF)
	if shortLen <= 0 || longLen <= 0 || src.Length() < 2 {
		return math.NaN(), true, nil
	}
	n := src.Length()
	mom := wseries.NewQueue(n - 1)
	absMom := wseries.NewQueue(n - 1)
	for i := 1; i < n; i++ {
		d := seriesChronoValue(src, i) - seriesChronoValue(src, i-1)
		mom.Update(d)
		absMom.Update(math.Abs(d))
	}
	num := emaSeriesFromSeries(emaSeriesFromSeries(mom, shortLen), longLen)
	den := emaSeriesFromSeries(emaSeriesFromSeries(absMom, shortLen), longLen)
	if num.Length() == 0 || den.Length() == 0 {
		return math.NaN(), true, nil
	}
	lastNum := num.Last(0)
	lastDen := den.Last(0)
	if lastDen == 0 || math.IsNaN(lastNum) || math.IsNaN(lastDen) {
		return math.NaN(), true, nil
	}
	return 100.0 * lastNum / lastDen, true, nil
}

func emaSeriesFromSeries(vals SeriesExtended, length int) SeriesExtended {
	n := vals.Length()
	out := wseries.NewQueue(n)
	if length <= 0 || n == 0 {
		for i := 0; i < n; i++ {
			out.Update(math.NaN())
		}
		return out
	}
	k := 2.0 / (float64(length) + 1.0)
	ema := seriesChronoValue(vals, 0)
	out.Update(ema)
	for i := 1; i < n; i++ {
		v := seriesChronoValue(vals, i)
		if math.IsNaN(v) {
			out.Update(out.Last(0))
			continue
		}
		ema = v*k + out.Last(0)*(1.0-k)
		out.Update(ema)
	}
	return out
}

func (r *Runtime) builtinWPR(args []interface{}) (interface{}, bool, error) {
	if len(args) != 1 {
		return nil, true, fmt.Errorf("wpr(length) expects 1 arg")
	}
	lf, _ := toFloat(args[0])
	l := int(lf)
	h, err := r.getSeries(r.activeSymbol, "high")
	if err != nil {
		return nil, true, err
	}
	lo, err := r.getSeries(r.activeSymbol, "low")
	if err != nil {
		return nil, true, err
	}
	c, err := r.getSeries(r.activeSymbol, "close")
	if err != nil {
		return nil, true, err
	}
	h = r.seriesAtEvalSeries(h)
	lo = r.seriesAtEvalSeries(lo)
	c = r.seriesAtEvalSeries(c)
	n := h.Length()
	if lo.Length() < n {
		n = lo.Length()
	}
	if c.Length() < n {
		n = c.Length()
	}
	if l <= 0 || n < l {
		return math.NaN(), true, nil
	}
	hh := h.Last(0)
	ll := lo.Last(0)
	for i := 1; i < l; i++ {
		hv := h.Last(i)
		lv := lo.Last(i)
		if hv > hh {
			hh = hv
		}
		if lv < ll {
			ll = lv
		}
	}
	den := hh - ll
	if den == 0 {
		return math.NaN(), true, nil
	}
	cur := c.Last(0)
	return -100.0 * (hh - cur) / den, true, nil
}

func (r *Runtime) builtinDMI(args []interface{}) (interface{}, bool, error) {
	if len(args) != 2 {
		return nil, true, fmt.Errorf("dmi(diLength, adxSmoothing) expects 2 args")
	}
	diF, _ := toFloat(args[0])
	adxF, _ := toFloat(args[1])
	diLen := int(diF)
	adxLen := int(adxF)
	if diLen <= 0 || adxLen <= 0 {
		na := math.NaN()
		return []interface{}{na, na, na}, true, nil
	}
	h, err := r.getSeries(r.activeSymbol, "high")
	if err != nil {
		return nil, true, err
	}
	lo, err := r.getSeries(r.activeSymbol, "low")
	if err != nil {
		return nil, true, err
	}
	c, err := r.getSeries(r.activeSymbol, "close")
	if err != nil {
		return nil, true, err
	}
	h = r.seriesAtEvalSeries(h)
	lo = r.seriesAtEvalSeries(lo)
	c = r.seriesAtEvalSeries(c)
	n := h.Length()
	if lo.Length() < n {
		n = lo.Length()
	}
	if c.Length() < n {
		n = c.Length()
	}
	if n < 2 {
		na := math.NaN()
		return []interface{}{na, na, na}, true, nil
	}
	alphaDI := 1.0 / float64(diLen)
	alphaADX := 1.0 / float64(adxLen)

	h0 := seriesChronoValue(h, 0)
	lo0 := seriesChronoValue(lo, 0)
	trRMA := h0 - lo0
	plusRMA := 0.0
	minusRMA := 0.0

	dx0 := math.NaN()
	if trRMA != 0 && !math.IsNaN(trRMA) {
		dx0 = 0
	}
	adx := dx0

	for i := 1; i < n; i++ {
		hv := seriesChronoValue(h, i)
		pvH := seriesChronoValue(h, i-1)
		lv := seriesChronoValue(lo, i)
		pvL := seriesChronoValue(lo, i-1)
		pvC := seriesChronoValue(c, i-1)

		up := hv - pvH
		down := pvL - lv
		plusDM := 0.0
		minusDM := 0.0
		if up > down && up > 0 {
			plusDM = up
		}
		if down > up && down > 0 {
			minusDM = down
		}
		a := hv - lv
		b := math.Abs(hv - pvC)
		d := math.Abs(lv - pvC)
		tr := math.Max(a, math.Max(b, d))

		plusRMA = alphaDI*plusDM + (1.0-alphaDI)*plusRMA
		minusRMA = alphaDI*minusDM + (1.0-alphaDI)*minusRMA
		trRMA = alphaDI*tr + (1.0-alphaDI)*trRMA

		dx := math.NaN()
		if trRMA != 0 && !math.IsNaN(trRMA) {
			pdi := 100.0 * plusRMA / trRMA
			mdi := 100.0 * minusRMA / trRMA
			den := pdi + mdi
			if den == 0 {
				dx = 0
			} else {
				dx = 100.0 * math.Abs(pdi-mdi) / den
			}
		}
		if i == 1 {
			adx = dx
		} else if !math.IsNaN(dx) {
			if math.IsNaN(adx) {
				adx = dx
			} else {
				adx = alphaADX*dx + (1.0-alphaADX)*adx
			}
		}
	}

	if trRMA == 0 || math.IsNaN(trRMA) {
		na := math.NaN()
		return []interface{}{na, na, na}, true, nil
	}
	pdi := 100.0 * plusRMA / trRMA
	mdi := 100.0 * minusRMA / trRMA
	return []interface{}{pdi, mdi, adx}, true, nil
}

func (r *Runtime) builtinSAR(args []interface{}) (interface{}, bool, error) {
	if len(args) != 3 {
		return nil, true, fmt.Errorf("sar(start, increment, maximum) expects 3 args")
	}
	start, _ := toFloat(args[0])
	inc, _ := toFloat(args[1])
	maxAF, _ := toFloat(args[2])
	h, err := r.getSeries(r.activeSymbol, "high")
	if err != nil {
		return nil, true, err
	}
	lo, err := r.getSeries(r.activeSymbol, "low")
	if err != nil {
		return nil, true, err
	}
	h = r.seriesAtEvalSeries(h)
	lo = r.seriesAtEvalSeries(lo)
	n := h.Length()
	if lo.Length() < n {
		n = lo.Length()
	}
	if n < 2 || start <= 0 || inc <= 0 || maxAF <= 0 {
		return math.NaN(), true, nil
	}
	h0 := seriesChronoValue(h, 0)
	h1 := seriesChronoValue(h, 1)
	lo0 := seriesChronoValue(lo, 0)
	isLong := h1 >= h0
	sar := lo0
	ep := h0
	if !isLong {
		sar = h0
		ep = lo0
	}
	af := start
	for i := 1; i < n; i++ {
		hv := seriesChronoValue(h, i)
		lv := seriesChronoValue(lo, i)
		prevH := seriesChronoValue(h, i-1)
		prevL := seriesChronoValue(lo, i-1)
		sar = sar + af*(ep-sar)
		if isLong {
			sar = math.Min(sar, prevL)
			if i > 1 {
				sar = math.Min(sar, seriesChronoValue(lo, i-2))
			}
			if lv < sar {
				isLong = false
				sar = ep
				ep = lv
				af = start
			} else if hv > ep {
				ep = hv
				af = math.Min(af+inc, maxAF)
			}
		} else {
			sar = math.Max(sar, prevH)
			if i > 1 {
				sar = math.Max(sar, seriesChronoValue(h, i-2))
			}
			if hv > sar {
				isLong = true
				sar = ep
				ep = hv
				af = start
			} else if lv < ep {
				ep = lv
				af = math.Min(af+inc, maxAF)
			}
		}
	}
	return sar, true, nil
}

func (r *Runtime) builtinSupertrend(args []interface{}) (interface{}, bool, error) {
	if len(args) != 2 {
		return nil, true, fmt.Errorf("supertrend(factor, atrPeriod) expects 2 args")
	}
	factor, _ := toFloat(args[0])
	atrF, _ := toFloat(args[1])
	atrLen := int(atrF)
	if atrLen <= 0 {
		na := math.NaN()
		return []interface{}{na, na}, true, nil
	}
	h, err := r.getSeries(r.activeSymbol, "high")
	if err != nil {
		return nil, true, err
	}
	lo, err := r.getSeries(r.activeSymbol, "low")
	if err != nil {
		return nil, true, err
	}
	c, err := r.getSeries(r.activeSymbol, "close")
	if err != nil {
		return nil, true, err
	}
	h = r.seriesAtEvalSeries(h)
	lo = r.seriesAtEvalSeries(lo)
	c = r.seriesAtEvalSeries(c)
	n := h.Length()
	if lo.Length() < n {
		n = lo.Length()
	}
	if c.Length() < n {
		n = c.Length()
	}
	if n < 2 {
		na := math.NaN()
		return []interface{}{na, na}, true, nil
	}
	tr := trueRangeSeriesFromSeries(h, lo, c)
	atrSeries := rmaSeriesFromSeries(tr, atrLen)

	h0 := seriesChronoValue(h, 0)
	lo0 := seriesChronoValue(lo, 0)
	atr0 := seriesChronoValue(atrSeries, 0)
	hl20 := (h0 + lo0) / 2.0
	prevFinalUpper := hl20 + factor*atr0
	prevFinalLower := hl20 - factor*atr0
	prevSuper := prevFinalUpper
	prevDir := -1.0
	prevClose := seriesChronoValue(c, 0)

	for i := 1; i < n; i++ {
		hv := seriesChronoValue(h, i)
		lv := seriesChronoValue(lo, i)
		cv := seriesChronoValue(c, i)
		atri := seriesChronoValue(atrSeries, i)
		hl2 := (hv + lv) / 2.0
		upper := hl2 + factor*atri
		lower := hl2 - factor*atri

		finalUpper := prevFinalUpper
		if upper < prevFinalUpper || prevClose > prevFinalUpper {
			finalUpper = upper
		}
		finalLower := prevFinalLower
		if lower > prevFinalLower || prevClose < prevFinalLower {
			finalLower = lower
		}

		super := finalUpper
		dir := -1.0
		if prevSuper == prevFinalUpper {
			if cv <= finalUpper {
				super = finalUpper
				dir = -1
			} else {
				super = finalLower
				dir = 1
			}
		} else {
			if cv >= finalLower {
				super = finalLower
				dir = 1
			} else {
				super = finalUpper
				dir = -1
			}
		}

		prevFinalUpper = finalUpper
		prevFinalLower = finalLower
		prevSuper = super
		prevDir = dir
		prevClose = cv
	}
	return []interface{}{prevSuper, prevDir}, true, nil
}

func rmaSeriesFromSeries(vals SeriesExtended, length int) SeriesExtended {
	n := vals.Length()
	out := wseries.NewQueue(n)
	if length <= 0 || n == 0 {
		for i := 0; i < n; i++ {
			out.Update(math.NaN())
		}
		return out
	}
	alpha := 1.0 / float64(length)
	rma := seriesChronoValue(vals, 0)
	out.Update(rma)
	for i := 1; i < n; i++ {
		v := seriesChronoValue(vals, i)
		if math.IsNaN(v) {
			out.Update(out.Last(0))
			continue
		}
		rma = alpha*v + (1.0-alpha)*out.Last(0)
		out.Update(rma)
	}
	return out
}

const maxPooledFloat64Cap = 1 << 14

var float64SlicePool = sync.Pool{New: func() interface{} { return make([]float64, 0, 64) }}
var disableWindowOptimizations bool
var disableIncrementalBB bool

func acquireFloat64Slice(size int) []float64 {
	v := float64SlicePool.Get()
	buf, ok := v.([]float64)
	if !ok {
		return make([]float64, 0, size)
	}
	if cap(buf) < size {
		return make([]float64, 0, size)
	}
	return buf[:0]
}

func releaseFloat64Slice(buf []float64) {
	if buf == nil {
		return
	}
	if cap(buf) > maxPooledFloat64Cap {
		return
	}
	float64SlicePool.Put(buf[:0])
}

func (r *Runtime) seriesFromExpr(raw *Expr) (SeriesExtended, error) {
	if raw == nil {
		return nil, fmt.Errorf("missing series expression")
	}
	if out, ok, err := r.seriesFromExprFast(raw); ok || err != nil {
		if err != nil {
			return nil, err
		}
		return r.seriesAtEvalSeries(out), nil
	}
	out, err := r.seriesFromExprSlow(raw)
	if err != nil {
		return nil, err
	}
	return r.seriesAtEvalSeries(out), nil
}

func (r *Runtime) seriesFromExprSlow(raw *Expr) (SeriesExtended, error) {
	eff := r.effectiveBarIndex()
	if eff < 0 {
		return wseries.NewQueue(0), nil
	}
	q := wseries.NewQueue(eff + 1)
	for off := eff; off >= 0; off-- {
		v, err := r.evalWithOffset(raw, off)
		if err != nil {
			return nil, err
		}
		f, _ := toFloat(v)
		q.Update(f)
	}
	return q, nil
}

func queueFromFloat64(vals []float64) SeriesExtended {
	q := wseries.NewQueue(len(vals))
	for _, v := range vals {
		q.Update(v)
	}
	return q
}

func (r *Runtime) seriesAtEvalSeries(ser SeriesExtended) SeriesExtended {
	if ser == nil {
		return wseries.NewQueue(0)
	}
	total := ser.Length()
	if total <= 0 {
		return wseries.NewQueue(0)
	}
	eff := r.effectiveBarIndex()
	if eff < 0 {
		return wseries.NewQueue(0)
	}
	count := eff + 1
	if count > total {
		count = total
	}
	if count <= 0 {
		return wseries.NewQueue(0)
	}
	shift := total - count
	q := wseries.NewQueue(count)
	for off := count - 1; off >= 0; off-- {
		q.Update(ser.Last(off + shift))
	}
	return q
}

func seriesWindowFromSeries(ser SeriesExtended, length int) ([]float64, bool) {
	if ser == nil || length <= 0 {
		return nil, false
	}
	if n := ser.Length(); n < length {
		length = n
	}
	if length <= 0 {
		return nil, false
	}
	buf := acquireFloat64Slice(length)
	for off := length - 1; off >= 0; off-- {
		buf = append(buf, ser.Last(off))
	}
	return buf, true
}

func (r *Runtime) seriesWindowAtEvalPoint(ser SeriesExtended, length int) ([]float64, bool) {
	if ser == nil || length <= 0 {
		return nil, false
	}
	total := ser.Length()
	if total <= 0 {
		return nil, false
	}
	eff := r.effectiveBarIndex()
	if eff < 0 {
		return nil, false
	}
	count := eff + 1
	if count > total {
		count = total
	}
	if count <= 0 {
		return nil, false
	}
	if length > count {
		length = count
	}
	if length <= 0 {
		return nil, false
	}
	shift := total - count
	buf := acquireFloat64Slice(length)
	for off := length - 1; off >= 0; off-- {
		buf = append(buf, ser.Last(off+shift))
	}
	return buf, true
}

func (r *Runtime) seriesWindowFromExpr(raw *Expr, length int) ([]float64, bool, error) {
	if raw == nil {
		return nil, false, fmt.Errorf("missing series expression")
	}
	if length <= 0 {
		return nil, false, nil
	}

	eff := r.effectiveBarIndex()
	if eff < 0 {
		return nil, false, nil
	}
	if length > eff+1 {
		length = eff + 1
	}
	if length <= 0 {
		return nil, false, nil
	}

	if raw.Kind == "ident" {
		name := raw.Name
		if isPriceIdentifierName(name) {
			return r.seriesWindowForValueType(r.activeSymbol, name, length)
		}
		if nums, ok := r.numericHistory[name]; ok {
			end := len(nums) - r.evalOffset
			if end < 0 {
				end = 0
			}
			start := end - length
			if start < 0 {
				start = 0
			}
			return nums[start:end], false, nil
		}
		if vals, ok := r.history[name]; ok {
			end := len(vals) - r.evalOffset
			if end < 0 {
				end = 0
			}
			start := end - length
			if start < 0 {
				start = 0
			}
			buf := acquireFloat64Slice(end - start)
			for i := start; i < end; i++ {
				f, _ := toFloat(vals[i])
				buf = append(buf, f)
			}
			return buf, true, nil
		}
		if ser, ok, err := r.seriesFromExprFast(raw); err != nil {
			return nil, false, err
		} else if ok {
			window, pooled := r.seriesWindowAtEvalPoint(ser, length)
			return window, pooled, nil
		}
	}

	if raw.Kind == "call" && raw.Left != nil && raw.Left.Kind == "ident" {
		name := raw.Left.Name
		switch name {
		case "close_of", "open_of", "high_of", "low_of":
			if len(raw.Args) == 1 && raw.Args[0] != nil && raw.Args[0].Kind == "string" {
				valueType := "close"
				switch name {
				case "open_of":
					valueType = "open"
				case "high_of":
					valueType = "high"
				case "low_of":
					valueType = "low"
				}
				return r.seriesWindowForValueType(raw.Args[0].String, valueType, length)
			}
		case "value_of":
			if len(raw.Args) == 2 && raw.Args[0] != nil && raw.Args[1] != nil && raw.Args[0].Kind == "string" && raw.Args[1].Kind == "string" {
				return r.seriesWindowForValueType(raw.Args[0].String, raw.Args[1].String, length)
			}
		}
	}

	if raw.Kind == "unary" && raw.UOp == unaryOpNeg {
		base, pooled, err := r.seriesWindowFromExpr(raw.Right, length)
		if err != nil {
			return nil, false, err
		}
		if len(base) == 0 {
			return base, pooled, nil
		}
		out, outPooled := ensureMutableWindow(base, pooled)
		for i := range out {
			out[i] = -out[i]
		}
		return out, outPooled, nil
	}

	if raw.Kind == "binary" && isArithmeticOpcode(raw.BOp) {
		if scalar, ok := exprConstFloat(raw.Right); ok {
			left, pooled, err := r.seriesWindowFromExpr(raw.Left, length)
			if err != nil {
				return nil, false, err
			}
			if len(left) == 0 {
				return left, pooled, nil
			}
			out, outPooled := ensureMutableWindow(left, pooled)
			for i := range out {
				out[i] = evalBinaryArithmeticFloatByOpcode(raw.BOp, out[i], scalar)
			}
			return out, outPooled, nil
		}
		if scalar, ok := exprConstFloat(raw.Left); ok {
			right, pooled, err := r.seriesWindowFromExpr(raw.Right, length)
			if err != nil {
				return nil, false, err
			}
			if len(right) == 0 {
				return right, pooled, nil
			}
			out, outPooled := ensureMutableWindow(right, pooled)
			for i := range out {
				out[i] = evalBinaryArithmeticFloatByOpcode(raw.BOp, scalar, out[i])
			}
			return out, outPooled, nil
		}
	}

	buf := acquireFloat64Slice(length)
	for off := length - 1; off >= 0; off-- {
		v, err := r.evalWithOffset(raw, off)
		if err != nil {
			releaseFloat64Slice(buf)
			return nil, false, err
		}
		f, _ := toFloat(v)
		buf = append(buf, f)
	}
	return buf, true, nil
}

func isArithmeticOpcode(op uint8) bool {
	switch op {
	case binaryOpAdd, binaryOpSub, binaryOpMul, binaryOpDiv, binaryOpMod:
		return true
	default:
		return false
	}
}

func ensureMutableWindow(vals []float64, pooled bool) ([]float64, bool) {
	if pooled {
		return vals, true
	}
	out := acquireFloat64Slice(len(vals))
	out = append(out, vals...)
	return out, true
}

func (r *Runtime) seriesWindowForValueType(symbol, valueType string, length int) ([]float64, bool, error) {
	ser, err := r.getSeriesByIdentifier(symbol, valueType)
	if err != nil {
		return nil, false, err
	}
	serLen := ser.Length()
	if serLen == 0 || length <= 0 {
		return nil, false, nil
	}
	eff := r.effectiveBarIndex()
	if eff < 0 {
		return nil, false, nil
	}
	avail := eff + 1
	if avail > serLen {
		avail = serLen
	}
	if length > avail {
		length = avail
	}
	if length <= 0 {
		return nil, false, nil
	}
	buf := acquireFloat64Slice(length)
	base := serLen - 1 - r.barIndex + r.evalOffset
	for off := length - 1; off >= 0; off-- {
		idx := base + off
		if idx < 0 || idx >= serLen {
			buf = append(buf, math.NaN())
			continue
		}
		buf = append(buf, ser.Last(idx))
	}
	return buf, true, nil
}

func (r *Runtime) seriesFromExprFast(raw *Expr) (SeriesExtended, bool, error) {
	if raw == nil {
		return nil, false, fmt.Errorf("missing series expression")
	}

	switch raw.Kind {
	case "ident":
		name := raw.Name
		if ser, ok := r.seriesForName(name); ok {
			return ser, true, nil
		}
		if expr, ok := r.seriesExprForName(name); ok && expr != nil {
			if r.seriesExprResolving[name] {
				return nil, false, nil
			}
			r.seriesExprResolving[name] = true
			derived, derivedOK, derivedErr := r.seriesFromExprFast(expr)
			delete(r.seriesExprResolving, name)
			if derivedErr != nil {
				return nil, true, derivedErr
			}
			if derivedOK {
				r.setSeries(name, derived)
				return derived, true, nil
			}
		}
		if typeSet, ok := r.valueTypesBySymbol[r.activeSymbol]; ok && typeSet[name] {
			ser, err := r.getSeriesByIdentifier(r.activeSymbol, name)
			if err != nil {
				return nil, true, err
			}
			return ser, true, nil
		}
		if isPriceIdentifierName(name) {
			ser, err := r.getSeriesByIdentifier(r.activeSymbol, name)
			if err != nil {
				return nil, true, err
			}
			return ser, true, nil
		}
		if vals, ok := r.numericHistory[name]; ok {
			end := len(vals) - r.evalOffset
			if end < 0 {
				end = 0
			}
			return queueFromFloat64(vals[:end]), true, nil
		}
		if vals, ok := r.history[name]; ok {
			return queueFromFloat64(numericHistory(vals, r.evalOffset)), true, nil
		}
		return nil, false, nil
	case "call":
		if raw.Left != nil && raw.Left.Kind == "ident" {
			switch raw.Left.Name {
			case "close_of", "open_of", "high_of", "low_of":
				if len(raw.Args) == 1 && raw.Args[0] != nil && raw.Args[0].Kind == "string" {
					valueType := "close"
					switch raw.Left.Name {
					case "open_of":
						valueType = "open"
					case "high_of":
						valueType = "high"
					case "low_of":
						valueType = "low"
					}
					ser, err := r.getSeries(raw.Args[0].String, valueType)
					if err != nil {
						return nil, true, err
					}
					return ser, true, nil
				}
			case "value_of":
				if len(raw.Args) == 2 && raw.Args[0] != nil && raw.Args[1] != nil && raw.Args[0].Kind == "string" && raw.Args[1].Kind == "string" {
					ser, err := r.getSeriesByIdentifier(raw.Args[0].String, raw.Args[1].String)
					if err != nil {
						return nil, true, err
					}
					return ser, true, nil
				}
			}
		}
		return nil, false, nil
	case "unary":
		if raw.UOp == unaryOpNeg {
			base, ok, err := r.seriesFromExprFast(raw.Right)
			if err != nil || !ok {
				return nil, ok, err
			}
			return base.Mul(-1.0), true, nil
		}
		return nil, false, nil
	case "binary":
		leftSeries, leftOK, leftErr := r.seriesFromExprFast(raw.Left)
		if leftErr != nil {
			return nil, true, leftErr
		}
		rightSeries, rightOK, rightErr := r.seriesFromExprFast(raw.Right)
		if rightErr != nil {
			return nil, true, rightErr
		}
		leftConst, leftConstOK := exprConstFloat(raw.Left)
		rightConst, rightConstOK := exprConstFloat(raw.Right)

		switch {
		case leftOK && rightOK:
			switch raw.BOp {
			case binaryOpAdd:
				return leftSeries.Add(rightSeries), true, nil
			case binaryOpSub:
				return leftSeries.Minus(rightSeries), true, nil
			case binaryOpMul:
				return leftSeries.Mul(rightSeries), true, nil
			case binaryOpDiv:
				return leftSeries.Div(rightSeries), true, nil
			default:
				out, err := applySeriesBinary(raw.BOp, leftSeries, rightSeries)
				if err != nil {
					return nil, true, err
				}
				return out, true, nil
			}
		case leftOK && rightConstOK:
			switch raw.BOp {
			case binaryOpAdd:
				return leftSeries.Add(rightConst), true, nil
			case binaryOpSub:
				return leftSeries.Minus(rightConst), true, nil
			case binaryOpMul:
				return leftSeries.Mul(rightConst), true, nil
			case binaryOpDiv:
				return leftSeries.Div(rightConst), true, nil
			default:
				out, err := applySeriesScalar(raw.BOp, leftSeries, rightConst, false)
				if err != nil {
					return nil, true, err
				}
				return out, true, nil
			}
		case leftConstOK && rightOK:
			switch raw.BOp {
			case binaryOpAdd:
				return rightSeries.Add(leftConst), true, nil
			case binaryOpMul:
				return rightSeries.Mul(leftConst), true, nil
			case binaryOpSub, binaryOpDiv:
				left := wseries.SwitchIface(leftConst)
				if left == nil {
					return nil, false, nil
				}
				leftExt, ok := left.(SeriesExtended)
				if !ok {
					return nil, false, nil
				}
				if raw.BOp == binaryOpSub {
					return leftExt.Minus(rightSeries), true, nil
				}
				return leftExt.Div(rightSeries), true, nil
			default:
				out, err := applySeriesScalar(raw.BOp, rightSeries, leftConst, true)
				if err != nil {
					return nil, true, err
				}
				return out, true, nil
			}
		default:
			return nil, false, nil
		}
	default:
		return nil, false, nil
	}
}

func numericHistory(vals []interface{}, offset int) []float64 {
	end := len(vals) - offset
	if end < 0 {
		end = 0
	}
	out := make([]float64, 0, end)
	for i := 0; i < end; i++ {
		f, _ := toFloat(vals[i])
		out = append(out, f)
	}
	return out
}

func exprConstFloat(expr *Expr) (float64, bool) {
	if expr == nil {
		return 0, false
	}
	switch expr.Kind {
	case "number":
		return expr.Number, true
	case "bool":
		if expr.Bool {
			return 1, true
		}
		return 0, true
	case "na":
		return math.NaN(), true
	case "unary":
		if expr.UOp == unaryOpNeg {
			v, ok := exprConstFloat(expr.Right)
			if !ok {
				return 0, false
			}
			return -v, true
		}
		return 0, false
	default:
		return 0, false
	}
}

func applySeriesBinary(op uint8, left, right SeriesExtended) (SeriesExtended, error) {
	n := left.Length()
	if right.Length() < n {
		n = right.Length()
	}
	if n == 0 {
		return wseries.NewQueue(0), nil
	}
	out := wseries.NewQueue(n)
	for off := n - 1; off >= 0; off-- {
		lv := left.Last(off)
		rv := right.Last(off)
		if fast, ok := evalBinaryFloatResult(op, lv, rv); ok {
			out.Update(fast)
			continue
		}
		v, err := evalBinary(op, lv, rv)
		if err != nil {
			return nil, err
		}
		f, _ := toFloat(v)
		out.Update(f)
	}
	return out, nil
}

func applySeriesScalar(op uint8, series SeriesExtended, scalar float64, scalarOnLeft bool) (SeriesExtended, error) {
	n := series.Length()
	out := wseries.NewQueue(n)
	for off := n - 1; off >= 0; off-- {
		v := series.Last(off)
		lv := v
		rv := scalar
		if scalarOnLeft {
			lv, rv = scalar, v
		}
		if fast, ok := evalBinaryFloatResult(op, lv, rv); ok {
			out.Update(fast)
			continue
		}
		res, err := evalBinary(op, lv, rv)
		if err != nil {
			return nil, err
		}
		f, _ := toFloat(res)
		out.Update(f)
	}
	return out, nil
}

func evalBinaryFloatResult(op uint8, lv, rv float64) (float64, bool) {
	switch op {
	case binaryOpAdd:
		return lv + rv, true
	case binaryOpSub:
		return lv - rv, true
	case binaryOpMul:
		return lv * rv, true
	case binaryOpDiv:
		if rv == 0 {
			return math.NaN(), true
		}
		return lv / rv, true
	case binaryOpMod:
		if rv == 0 {
			return math.NaN(), true
		}
		return math.Mod(lv, rv), true
	case binaryOpLT:
		if lv < rv {
			return 1, true
		}
		return 0, true
	case binaryOpLTE:
		if lv <= rv {
			return 1, true
		}
		return 0, true
	case binaryOpGT:
		if lv > rv {
			return 1, true
		}
		return 0, true
	case binaryOpGTE:
		if lv >= rv {
			return 1, true
		}
		return 0, true
	case binaryOpEq:
		if math.IsNaN(lv) && math.IsNaN(rv) {
			return 1, true
		}
		if lv == rv {
			return 1, true
		}
		return 0, true
	case binaryOpNeq:
		if math.IsNaN(lv) && math.IsNaN(rv) {
			return 0, true
		}
		if lv != rv {
			return 1, true
		}
		return 0, true
	case binaryOpAnd:
		if floatTruthy(lv) && floatTruthy(rv) {
			return 1, true
		}
		return 0, true
	case binaryOpOr:
		if floatTruthy(lv) || floatTruthy(rv) {
			return 1, true
		}
		return 0, true
	default:
		return 0, false
	}
}

func floatTruthy(v float64) bool {
	return !math.IsNaN(v) && v != 0
}

func (r *Runtime) builtinChange(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if len(rawArgs) < 1 || len(rawArgs) > 2 {
		return nil, true, fmt.Errorf("ta.change(source, [length]) expects 1 or 2 args")
	}
	length := 1
	if len(args) >= 2 {
		l, _ := toFloat(args[1])
		length = int(l)
	}
	if length <= 0 {
		return math.NaN(), true, nil
	}
	curV, err := r.eval(rawArgs[0])
	if err != nil {
		return nil, true, err
	}
	prevV, err := r.evalWithOffset(rawArgs[0], length)
	if err != nil {
		return nil, true, err
	}
	cur, _ := toFloat(curV)
	prev, _ := toFloat(prevV)
	return cur - prev, true, nil
}

func (r *Runtime) builtinHighest(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if len(rawArgs) != 2 || len(args) != 2 {
		return nil, true, fmt.Errorf("ta.highest(source, length) expects 2 args")
	}
	lf, _ := toFloat(args[1])
	length := int(lf)
	if r.evalOffset == 0 && length > 0 {
		key := extremaStateKey{raw: rawArgs[0], length: length, isMax: true, symbol: r.activeSymbol, valueType: r.activeValueType}
		stAny, ok := r.extremaState[key]
		if !ok {
			stAny = newExtremaIndicatorState(length, true)
			r.extremaState[key] = stAny
		}
		if stAny.lastBar != r.barIndex {
			v, err := r.evalCurrentFloat(rawArgs[0])
			if err != nil {
				return nil, true, err
			}
			stAny.Update(v)
			stAny.lastBar = r.barIndex
		}
		return stAny.Value(), true, nil
	}
	window, pooled, err := r.seriesWindowFromExpr(rawArgs[0], length)
	if err != nil {
		return nil, true, err
	}
	if pooled {
		defer releaseFloat64Slice(window)
	}
	if length <= 0 || len(window) < length {
		return math.NaN(), true, nil
	}
	maxV := window[0]
	for _, v := range window[1:] {
		if v > maxV {
			maxV = v
		}
	}
	return maxV, true, nil
}

func (r *Runtime) builtinLowest(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if len(rawArgs) != 2 || len(args) != 2 {
		return nil, true, fmt.Errorf("ta.lowest(source, length) expects 2 args")
	}
	lf, _ := toFloat(args[1])
	length := int(lf)
	if r.evalOffset == 0 && length > 0 {
		key := extremaStateKey{raw: rawArgs[0], length: length, isMax: false, symbol: r.activeSymbol, valueType: r.activeValueType}
		stAny, ok := r.extremaState[key]
		if !ok {
			stAny = newExtremaIndicatorState(length, false)
			r.extremaState[key] = stAny
		}
		if stAny.lastBar != r.barIndex {
			v, err := r.evalCurrentFloat(rawArgs[0])
			if err != nil {
				return nil, true, err
			}
			stAny.Update(v)
			stAny.lastBar = r.barIndex
		}
		return stAny.Value(), true, nil
	}
	window, pooled, err := r.seriesWindowFromExpr(rawArgs[0], length)
	if err != nil {
		return nil, true, err
	}
	if pooled {
		defer releaseFloat64Slice(window)
	}
	if length <= 0 || len(window) < length {
		return math.NaN(), true, nil
	}
	minV := window[0]
	for _, v := range window[1:] {
		if v < minV {
			minV = v
		}
	}
	return minV, true, nil
}

func (r *Runtime) builtinStdev(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if len(rawArgs) < 2 || len(rawArgs) > 3 || len(args) < 2 || len(args) > 3 {
		return nil, true, fmt.Errorf("ta.stdev(source, length, [biased]) expects 2 or 3 args")
	}
	lf, _ := toFloat(args[1])
	length := int(lf)
	window, pooled, err := r.seriesWindowFromExpr(rawArgs[0], length)
	if err != nil {
		return nil, true, err
	}
	if pooled {
		defer releaseFloat64Slice(window)
	}
	if length <= 0 || len(window) < length {
		return math.NaN(), true, nil
	}
	biased := true
	if len(args) == 3 {
		if b, ok := args[2].(bool); ok {
			biased = b
		} else {
			v, _ := toFloat(args[2])
			biased = v != 0
		}
	}
	mean := 0.0
	for _, v := range window {
		mean += v
	}
	mean /= float64(length)
	ss := 0.0
	for _, v := range window {
		d := v - mean
		ss += d * d
	}
	den := float64(length)
	if !biased {
		if length <= 1 {
			return math.NaN(), true, nil
		}
		den = float64(length - 1)
	}
	return math.Sqrt(ss / den), true, nil
}

func (r *Runtime) builtinCorrelation(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if len(rawArgs) != 3 || len(args) != 3 {
		return nil, true, fmt.Errorf("ta.correlation(source1, source2, length) expects 3 args")
	}
	lf, _ := toFloat(args[2])
	length := int(lf)
	x, xPooled, err := r.seriesWindowFromExpr(rawArgs[0], length)
	if err != nil {
		return nil, true, err
	}
	if xPooled {
		defer releaseFloat64Slice(x)
	}
	y, yPooled, err := r.seriesWindowFromExpr(rawArgs[1], length)
	if err != nil {
		return nil, true, err
	}
	if yPooled {
		defer releaseFloat64Slice(y)
	}
	if length <= 1 || len(x) < length || len(y) < length {
		return math.NaN(), true, nil
	}
	mx := 0.0
	my := 0.0
	for i := 0; i < length; i++ {
		mx += x[i]
		my += y[i]
	}
	mx /= float64(length)
	my /= float64(length)
	cov := 0.0
	vx := 0.0
	vy := 0.0
	for i := 0; i < length; i++ {
		dx := x[i] - mx
		dy := y[i] - my
		cov += dx * dy
		vx += dx * dx
		vy += dy * dy
	}
	den := math.Sqrt(vx * vy)
	if den == 0 || math.IsNaN(den) {
		return math.NaN(), true, nil
	}
	return cov / den, true, nil
}

func (r *Runtime) builtinSMAOf(args []interface{}) (interface{}, bool, error) {
	symbol, length, valueType, err := parseOfArgs("sma_of", args)
	if err != nil {
		return nil, true, err
	}
	ser, err := r.getSeries(symbol, valueType)
	if err != nil {
		return nil, true, err
	}
	return smaFromSeries(ser, length), true, nil
}

func (r *Runtime) builtinEMAOf(args []interface{}) (interface{}, bool, error) {
	symbol, length, valueType, err := parseOfArgs("ema_of", args)
	if err != nil {
		return nil, true, err
	}
	ser, err := r.getSeries(symbol, valueType)
	if err != nil {
		return nil, true, err
	}
	return emaFromSeries(ser, length), true, nil
}

func (r *Runtime) builtinRSIOf(args []interface{}) (interface{}, bool, error) {
	symbol, length, valueType, err := parseOfArgs("rsi_of", args)
	if err != nil {
		return nil, true, err
	}
	ser, err := r.getSeries(symbol, valueType)
	if err != nil {
		return nil, true, err
	}
	return rsiFromSeries(ser, length), true, nil
}

func parseOfArgs(name string, args []interface{}) (string, int, string, error) {
	if len(args) != 2 && len(args) != 3 {
		return "", 0, "", fmt.Errorf("%s(symbol, length, [value_type]) expects 2 or 3 args", name)
	}
	symbol, ok := args[0].(string)
	if !ok || symbol == "" {
		return "", 0, "", fmt.Errorf("%s requires non-empty symbol string", name)
	}
	lenF, _ := toFloat(args[1])
	length := int(lenF)
	valueType := "close"
	if len(args) == 3 {
		vt, ok := args[2].(string)
		if !ok || vt == "" {
			return "", 0, "", fmt.Errorf("%s third arg must be non-empty value_type string", name)
		}
		valueType = vt
	}
	return symbol, length, valueType, nil
}

func seriesLen(ser SeriesExtended) int {
	if ser == nil {
		return 0
	}
	return ser.Length()
}

func seriesChronoValue(ser SeriesExtended, idx int) float64 {
	n := seriesLen(ser)
	if idx < 0 || idx >= n {
		return math.NaN()
	}
	return ser.Last(n - 1 - idx)
}

func smaFromSeries(seriesVals SeriesExtended, length int) float64 {
	n := seriesLen(seriesVals)
	if length <= 0 || n < length {
		return math.NaN()
	}
	total := 0.0
	for off := 0; off < length; off++ {
		total += seriesVals.Last(off)
	}
	return total / float64(length)
}

func rsiFromSeries(seriesVals SeriesExtended, length int) float64 {
	n := seriesLen(seriesVals)
	if length <= 0 || n <= length {
		return math.NaN()
	}
	gain := 0.0
	loss := 0.0
	start := n - length
	for i := start; i < n; i++ {
		if i == 0 {
			continue
		}
		delta := seriesChronoValue(seriesVals, i) - seriesChronoValue(seriesVals, i-1)
		if delta >= 0 {
			gain += delta
		} else {
			loss -= delta
		}
	}
	if loss == 0 {
		return 100.0
	}
	rs := (gain / float64(length)) / (loss / float64(length))
	return 100.0 - (100.0 / (1.0 + rs))
}

func emaFromSeries(seriesVals SeriesExtended, length int) float64 {
	n := seriesLen(seriesVals)
	if length <= 0 || n < length {
		return math.NaN()
	}
	k := 2.0 / (float64(length) + 1.0)
	ema := seriesChronoValue(seriesVals, 0)
	for i := 1; i < n; i++ {
		ema = seriesChronoValue(seriesVals, i)*k + ema*(1-k)
	}
	return ema
}

func atrFromSeries(seriesVals SeriesExtended, length int) float64 {
	n := seriesLen(seriesVals)
	if length <= 0 || n < length+1 {
		return math.NaN()
	}
	tr := wseries.NewQueue(n)
	prev := seriesChronoValue(seriesVals, 0)
	tr.Update(0)
	for i := 1; i < n; i++ {
		cur := seriesChronoValue(seriesVals, i)
		tr.Update(math.Abs(cur - prev))
		prev = cur
	}
	return rmaFromSeries(tr, length)
}

func rmaFromSeries(seriesVals SeriesExtended, length int) float64 {
	n := seriesLen(seriesVals)
	if length <= 0 || n < length {
		return math.NaN()
	}
	sum := 0.0
	for i := 0; i < length; i++ {
		sum += seriesChronoValue(seriesVals, i)
	}
	rma := sum / float64(length)
	for i := length; i < n; i++ {
		rma = (rma*float64(length-1) + seriesChronoValue(seriesVals, i)) / float64(length)
	}
	return rma
}

func wmaFromSeries(seriesVals SeriesExtended, length int) float64 {
	n := seriesLen(seriesVals)
	if length <= 0 || n < length {
		return math.NaN()
	}
	den := float64(length*(length+1)) / 2.0
	num := 0.0
	for i := 0; i < length; i++ {
		num += seriesVals.Last(length-1-i) * float64(i+1)
	}
	return num / den
}

func swmaFromSeries(seriesVals SeriesExtended) float64 {
	if seriesLen(seriesVals) < 4 {
		return math.NaN()
	}
	return (seriesVals.Last(3) + 2*seriesVals.Last(2) + 2*seriesVals.Last(1) + seriesVals.Last(0)) / 6.0
}

func hmaFromSeries(seriesVals SeriesExtended, length int) float64 {
	n := seriesLen(seriesVals)
	if length <= 0 || n < length {
		return math.NaN()
	}
	half := length / 2
	if half < 1 {
		half = 1
	}
	sqrtLen := int(math.Round(math.Sqrt(float64(length))))
	if sqrtLen < 1 {
		sqrtLen = 1
	}
	prefix := wseries.NewQueue(n)
	diff := wseries.NewQueue(n)
	for i := 0; i < n; i++ {
		prefix.Update(seriesChronoValue(seriesVals, i))
		w1 := wmaFromSeries(prefix, half)
		w2 := wmaFromSeries(prefix, length)
		diff.Update(2*w1 - w2)
	}
	return wmaFromSeries(diff, sqrtLen)
}

func almaFromSeries(seriesVals SeriesExtended, length int, offset, sigma float64, floorFlag bool) float64 {
	n := seriesLen(seriesVals)
	if length <= 0 || sigma == 0 || n < length {
		return math.NaN()
	}
	m := offset * float64(length-1)
	if floorFlag {
		m = math.Floor(m)
	}
	s := float64(length) / sigma
	norm := 0.0
	sum := 0.0
	for i := 0; i < length; i++ {
		w := math.Exp(-math.Pow(float64(i)-m, 2.0) / (2.0 * math.Pow(s, 2.0)))
		norm += w
		sum += seriesVals.Last(length-1-i) * w
	}
	if norm == 0 {
		return math.NaN()
	}
	return sum / norm
}

func almaFromWindow(window []float64, length int, offset, sigma float64, floorFlag bool) float64 {
	if length <= 0 || sigma == 0 || len(window) < length {
		return math.NaN()
	}
	m := offset * float64(length-1)
	if floorFlag {
		m = math.Floor(m)
	}
	s := float64(length) / sigma
	if s == 0 {
		return math.NaN()
	}
	norm := 0.0
	sum := 0.0
	for i := 0; i < length; i++ {
		w := math.Exp(-math.Pow(float64(i)-m, 2.0) / (2.0 * math.Pow(s, 2.0)))
		norm += w
		sum += window[i] * w
	}
	if norm == 0 {
		return math.NaN()
	}
	return sum / norm
}

func linregFromSeries(seriesVals SeriesExtended, length, offset int) float64 {
	nvals := seriesLen(seriesVals)
	if length <= 1 || nvals < length {
		return math.NaN()
	}
	n := float64(length)
	sumX := n * float64(length-1) / 2.0
	sumXX := float64((length-1)*length*(2*length-1)) / 6.0
	sumY := 0.0
	sumXY := 0.0
	for i := 0; i < length; i++ {
		y := seriesVals.Last(length - 1 - i)
		x := float64(i)
		sumY += y
		sumXY += x * y
	}
	den := n*sumXX - sumX*sumX
	if den == 0 {
		return math.NaN()
	}
	slope := (n*sumXY - sumX*sumY) / den
	intercept := (sumY - slope*sumX) / n
	targetX := float64(length - 1 - offset)
	return intercept + slope*targetX
}

func linregFromWindow(window []float64, length, offset int) float64 {
	if length <= 1 || len(window) < length {
		return math.NaN()
	}
	n := float64(length)
	sumX := n * float64(length-1) / 2.0
	sumXX := float64((length-1)*length*(2*length-1)) / 6.0
	sumY := 0.0
	sumXY := 0.0
	for i := 0; i < length; i++ {
		y := window[i]
		x := float64(i)
		sumY += y
		sumXY += x * y
	}
	den := n*sumXX - sumX*sumX
	if den == 0 {
		return math.NaN()
	}
	slope := (n*sumXY - sumX*sumY) / den
	intercept := (sumY - slope*sumX) / n
	xPred := float64(length - 1 - offset)
	return intercept + slope*xPred
}

func vwmaFromSeries(src, vol SeriesExtended, length int) float64 {
	if length <= 0 || seriesLen(src) < length || seriesLen(vol) < length {
		return math.NaN()
	}
	num := 0.0
	den := 0.0
	for i := 0; i < length; i++ {
		sv := src.Last(length - 1 - i)
		vv := vol.Last(length - 1 - i)
		num += sv * vv
		den += vv
	}
	if den == 0 {
		return math.NaN()
	}
	return num / den
}

func vwmaFromWindow(src, vol []float64, length int) float64 {
	if length <= 0 || len(src) < length || len(vol) < length {
		return math.NaN()
	}
	num := 0.0
	den := 0.0
	for i := 0; i < length; i++ {
		num += src[i] * vol[i]
		den += vol[i]
	}
	if den == 0 {
		return math.NaN()
	}
	return num / den
}

func cciFromSeries(seriesVals SeriesExtended, length int) float64 {
	n := seriesLen(seriesVals)
	if length <= 0 || n < length {
		return math.NaN()
	}
	sma := smaFromSeries(seriesVals, length)
	if math.IsNaN(sma) {
		return math.NaN()
	}
	dev := 0.0
	for off := 0; off < length; off++ {
		v := seriesVals.Last(off)
		dev += math.Abs(v - sma)
	}
	meanDev := dev / float64(length)
	if meanDev == 0 {
		return 0
	}
	return (seriesVals.Last(0) - sma) / (0.015 * meanDev)
}

func cciFromWindow(window []float64, length int) float64 {
	if length <= 0 || len(window) < length {
		return math.NaN()
	}
	sum := 0.0
	for i := 0; i < length; i++ {
		sum += window[i]
	}
	sma := sum / float64(length)
	dev := 0.0
	for i := 0; i < length; i++ {
		dev += math.Abs(window[i] - sma)
	}
	meanDev := dev / float64(length)
	if meanDev == 0 {
		return 0
	}
	return (window[length-1] - sma) / (0.015 * meanDev)
}

func cmoFromSeries(seriesVals SeriesExtended, length int) float64 {
	n := seriesLen(seriesVals)
	if length <= 0 || n <= length {
		return math.NaN()
	}
	up := 0.0
	down := 0.0
	start := n - length
	for i := start; i < n; i++ {
		if i == 0 {
			continue
		}
		d := seriesChronoValue(seriesVals, i) - seriesChronoValue(seriesVals, i-1)
		if d > 0 {
			up += d
		} else if d < 0 {
			down -= d
		}
	}
	den := up + down
	if den == 0 {
		return 0
	}
	return 100.0 * (up - down) / den
}

func cmoFromWindow(window []float64, length int) float64 {
	if length <= 0 || len(window) <= length {
		return math.NaN()
	}
	up := 0.0
	down := 0.0
	start := len(window) - length
	for i := start; i < len(window); i++ {
		if i == 0 {
			continue
		}
		d := window[i] - window[i-1]
		if d > 0 {
			up += d
		} else if d < 0 {
			down -= d
		}
	}
	den := up + down
	if den == 0 {
		return 0
	}
	return 100.0 * (up - down) / den
}

func cogFromSeries(seriesVals SeriesExtended, length int) float64 {
	n := seriesLen(seriesVals)
	if length <= 0 || n < length {
		return math.NaN()
	}
	num := 0.0
	den := 0.0
	for i := 0; i < length; i++ {
		v := seriesVals.Last(length - 1 - i)
		num += float64(i+1) * v
		den += v
	}
	if den == 0 {
		return math.NaN()
	}
	return -num / den
}

func cogFromWindow(window []float64, length int) float64 {
	if length <= 0 || len(window) < length {
		return math.NaN()
	}
	num := 0.0
	den := 0.0
	for i := 0; i < length; i++ {
		v := window[i]
		num += float64(i+1) * v
		den += v
	}
	if den == 0 {
		return math.NaN()
	}
	return -num / den
}

func macdFromSeries(seriesVals SeriesExtended, fastLen, slowLen, sigLen int) (float64, float64, float64) {
	n := seriesLen(seriesVals)
	need := fastLen
	if slowLen > need {
		need = slowLen
	}
	if fastLen <= 0 || slowLen <= 0 || sigLen <= 0 || n < need {
		return math.NaN(), math.NaN(), math.NaN()
	}
	prefix := wseries.NewQueue(n)
	macdSeries := wseries.NewQueue(n)
	for i := 0; i < n; i++ {
		prefix.Update(seriesChronoValue(seriesVals, i))
		fast := emaFromSeries(prefix, fastLen)
		slow := emaFromSeries(prefix, slowLen)
		if math.IsNaN(fast) || math.IsNaN(slow) {
			macdSeries.Update(math.NaN())
		} else {
			macdSeries.Update(fast - slow)
		}
	}
	macd := macdSeries.Last(0)
	clean := wseries.NewQueue(macdSeries.Length())
	for off := macdSeries.Length() - 1; off >= 0; off-- {
		v := macdSeries.Last(off)
		if !math.IsNaN(v) {
			clean.Update(v)
		}
	}
	signal := emaFromSeries(clean, sigLen)
	hist := macd - signal
	return macd, signal, hist
}

func (r *Runtime) builtinCross(rawArgs []*Expr, under bool) (interface{}, bool, error) {
	if len(rawArgs) != 2 {
		name := "crossover"
		if under {
			name = "crossunder"
		}
		return nil, true, fmt.Errorf("%s(source1, source2) expects 2 args", name)
	}
	curA, err := r.eval(rawArgs[0])
	if err != nil {
		return nil, true, err
	}
	curB, err := r.eval(rawArgs[1])
	if err != nil {
		return nil, true, err
	}
	prevA, err := r.evalWithOffset(rawArgs[0], 1)
	if err != nil {
		return nil, true, err
	}
	prevB, err := r.evalWithOffset(rawArgs[1], 1)
	if err != nil {
		return nil, true, err
	}

	ca, _ := toFloat(curA)
	cb, _ := toFloat(curB)
	pa, _ := toFloat(prevA)
	pb, _ := toFloat(prevB)
	if math.IsNaN(ca) || math.IsNaN(cb) || math.IsNaN(pa) || math.IsNaN(pb) {
		return false, true, nil
	}
	if under {
		return pa >= pb && ca < cb, true, nil
	}
	return pa <= pb && ca > cb, true, nil
}

func (r *Runtime) builtinCrossAny(rawArgs []*Expr) (interface{}, bool, error) {
	cu, _, err := r.builtinCross(rawArgs, false)
	if err != nil {
		return nil, true, err
	}
	cd, _, err := r.builtinCross(rawArgs, true)
	if err != nil {
		return nil, true, err
	}
	return truthy(cu) || truthy(cd), true, nil
}

func (r *Runtime) builtinMOM(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if len(rawArgs) != 2 || len(args) != 2 {
		return nil, true, fmt.Errorf("mom(source, length) expects 2 args")
	}
	seriesVals, err := r.seriesFromExpr(rawArgs[0])
	if err != nil {
		return nil, true, err
	}
	lenF, _ := toFloat(args[1])
	l := int(lenF)
	if l <= 0 || seriesVals.Length() <= l {
		return math.NaN(), true, nil
	}
	cur := seriesVals.Last(0)
	prev := seriesVals.Last(l)
	return cur - prev, true, nil
}

func (r *Runtime) builtinROC(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if len(rawArgs) != 2 || len(args) != 2 {
		return nil, true, fmt.Errorf("roc(source, length) expects 2 args")
	}
	seriesVals, err := r.seriesFromExpr(rawArgs[0])
	if err != nil {
		return nil, true, err
	}
	lenF, _ := toFloat(args[1])
	l := int(lenF)
	if l <= 0 || seriesVals.Length() <= l {
		return math.NaN(), true, nil
	}
	cur := seriesVals.Last(0)
	prev := seriesVals.Last(l)
	if prev == 0 || math.IsNaN(cur) || math.IsNaN(prev) {
		return math.NaN(), true, nil
	}
	return 100.0 * (cur - prev) / prev, true, nil
}

func (r *Runtime) builtinBarsSince(rawArgs []*Expr) (interface{}, bool, error) {
	if len(rawArgs) != 1 {
		return nil, true, fmt.Errorf("barssince(condition) expects 1 arg")
	}
	eff := r.effectiveBarIndex()
	if eff < 0 {
		return math.NaN(), true, nil
	}
	for off := 0; off <= eff; off++ {
		v, err := r.evalWithOffset(rawArgs[0], off)
		if err != nil {
			return nil, true, err
		}
		if truthy(v) {
			return float64(off), true, nil
		}
	}
	return math.NaN(), true, nil
}

func (r *Runtime) builtinCum(rawArgs []*Expr) (interface{}, bool, error) {
	if len(rawArgs) != 1 {
		return nil, true, fmt.Errorf("cum(source) expects 1 arg")
	}
	seriesVals, err := r.seriesFromExpr(rawArgs[0])
	if err != nil {
		return nil, true, err
	}
	sum := 0.0
	for i := 0; i < seriesVals.Length(); i++ {
		sum += seriesVals.Last(i)
	}
	return sum, true, nil
}

func (r *Runtime) builtinValueWhen(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if len(rawArgs) != 3 || len(args) != 3 {
		return nil, true, fmt.Errorf("valuewhen(condition, source, occurrence) expects 3 args")
	}
	occurF, _ := toFloat(args[2])
	occur := int(occurF)
	if occur < 0 {
		return math.NaN(), true, nil
	}
	eff := r.effectiveBarIndex()
	if eff < 0 {
		return math.NaN(), true, nil
	}
	found := 0
	for off := 0; off <= eff; off++ {
		c, err := r.evalWithOffset(rawArgs[0], off)
		if err != nil {
			return nil, true, err
		}
		if truthy(c) {
			if found == occur {
				v, err := r.evalWithOffset(rawArgs[1], off)
				if err != nil {
					return nil, true, err
				}
				return v, true, nil
			}
			found++
		}
	}
	return math.NaN(), true, nil
}

func highestOffset(window []float64) int {
	bestIdx := 0
	bestVal := window[0]
	for i := 1; i < len(window); i++ {
		if window[i] >= bestVal {
			bestVal = window[i]
			bestIdx = i
		}
	}
	return len(window) - 1 - bestIdx
}

func lowestOffset(window []float64) int {
	bestIdx := 0
	bestVal := window[0]
	for i := 1; i < len(window); i++ {
		if window[i] <= bestVal {
			bestVal = window[i]
			bestIdx = i
		}
	}
	return len(window) - 1 - bestIdx
}

func (r *Runtime) builtinHighestBars(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	var src *Expr
	var l int
	switch len(rawArgs) {
	case 1:
		src = &Expr{Kind: "ident", Name: "high"}
		lf, _ := toFloat(args[0])
		l = int(lf)
	case 2:
		src = rawArgs[0]
		lf, _ := toFloat(args[1])
		l = int(lf)
	default:
		return nil, true, fmt.Errorf("highestbars(source, length) expects 1 or 2 args")
	}
	if l <= 0 {
		return math.NaN(), true, nil
	}
	window, pooled, err := r.seriesWindowFromExpr(src, l)
	if err != nil {
		return nil, true, err
	}
	if pooled {
		defer releaseFloat64Slice(window)
	}
	if len(window) < l {
		return math.NaN(), true, nil
	}
	return float64(highestOffset(window)), true, nil
}

func (r *Runtime) builtinLowestBars(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	var src *Expr
	var l int
	switch len(rawArgs) {
	case 1:
		src = &Expr{Kind: "ident", Name: "low"}
		lf, _ := toFloat(args[0])
		l = int(lf)
	case 2:
		src = rawArgs[0]
		lf, _ := toFloat(args[1])
		l = int(lf)
	default:
		return nil, true, fmt.Errorf("lowestbars(source, length) expects 1 or 2 args")
	}
	if l <= 0 {
		return math.NaN(), true, nil
	}
	window, pooled, err := r.seriesWindowFromExpr(src, l)
	if err != nil {
		return nil, true, err
	}
	if pooled {
		defer releaseFloat64Slice(window)
	}
	if len(window) < l {
		return math.NaN(), true, nil
	}
	return float64(lowestOffset(window)), true, nil
}

func (r *Runtime) builtinTAMax(rawArgs []*Expr) (interface{}, bool, error) {
	if len(rawArgs) != 1 {
		return nil, true, fmt.Errorf("ta.max(source) expects 1 arg")
	}
	vals, err := r.seriesFromExpr(rawArgs[0])
	if err != nil {
		return nil, true, err
	}
	if vals.Length() == 0 {
		return math.NaN(), true, nil
	}
	return vals.Max(), true, nil
}

func (r *Runtime) builtinTAMin(rawArgs []*Expr) (interface{}, bool, error) {
	if len(rawArgs) != 1 {
		return nil, true, fmt.Errorf("ta.min(source) expects 1 arg")
	}
	vals, err := r.seriesFromExpr(rawArgs[0])
	if err != nil {
		return nil, true, err
	}
	if vals.Length() == 0 {
		return math.NaN(), true, nil
	}
	return vals.Min(), true, nil
}

func percentileLinear(window []float64, p float64) float64 {
	if len(window) == 0 {
		return math.NaN()
	}
	sorted := append([]float64(nil), window...)
	sort.Float64s(sorted)
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[len(sorted)-1]
	}
	pos := (p / 100.0) * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return sorted[lo]
	}
	w := pos - float64(lo)
	return sorted[lo]*(1.0-w) + sorted[hi]*w
}

func percentileNearest(window []float64, p float64) float64 {
	if len(window) == 0 {
		return math.NaN()
	}
	sorted := append([]float64(nil), window...)
	sort.Float64s(sorted)
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[len(sorted)-1]
	}
	rank := int(math.Ceil((p / 100.0) * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}

func (r *Runtime) percentileWindow(rawArgs []*Expr, args []interface{}) ([]float64, float64, error) {
	if len(rawArgs) != 3 || len(args) != 3 {
		return nil, 0, fmt.Errorf("expects (source, length, percentage)")
	}
	lf, _ := toFloat(args[1])
	p, _ := toFloat(args[2])
	l := int(lf)
	if l <= 0 {
		return nil, p, nil
	}
	window, pooled, err := r.seriesWindowFromExpr(rawArgs[0], l)
	if err != nil {
		return nil, 0, err
	}
	if pooled {
		stable := append([]float64(nil), window...)
		releaseFloat64Slice(window)
		window = stable
	}
	if len(window) < l {
		return nil, p, nil
	}
	return window, p, nil
}

func (r *Runtime) builtinPercentileLinearInterpolation(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	window, p, err := r.percentileWindow(rawArgs, args)
	if err != nil {
		return nil, true, err
	}
	if len(window) == 0 {
		return math.NaN(), true, nil
	}
	return percentileLinear(window, p), true, nil
}

func (r *Runtime) builtinPercentileNearestRank(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	window, p, err := r.percentileWindow(rawArgs, args)
	if err != nil {
		return nil, true, err
	}
	if len(window) == 0 {
		return math.NaN(), true, nil
	}
	return percentileNearest(window, p), true, nil
}

func (r *Runtime) builtinPercentRank(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if len(rawArgs) != 2 || len(args) != 2 {
		return nil, true, fmt.Errorf("percentrank(source, length) expects 2 args")
	}
	lf, _ := toFloat(args[1])
	l := int(lf)
	if l <= 1 {
		return math.NaN(), true, nil
	}
	window, pooled, err := r.seriesWindowFromExpr(rawArgs[0], l)
	if err != nil {
		return nil, true, err
	}
	if pooled {
		defer releaseFloat64Slice(window)
	}
	if len(window) < l {
		return math.NaN(), true, nil
	}
	cur := window[len(window)-1]
	countLE := 0
	for _, v := range window {
		if v <= cur {
			countLE++
		}
	}
	rank := float64(countLE-1) / float64(l-1) * 100.0
	return rank, true, nil
}

func (r *Runtime) builtinTARange(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if len(rawArgs) != 2 || len(args) != 2 {
		return nil, true, fmt.Errorf("ta.range(source, length) expects 2 args")
	}
	lf, _ := toFloat(args[1])
	l := int(lf)
	if l <= 0 {
		return math.NaN(), true, nil
	}
	window, pooled, err := r.seriesWindowFromExpr(rawArgs[0], l)
	if err != nil {
		return nil, true, err
	}
	if pooled {
		defer releaseFloat64Slice(window)
	}
	if len(window) < l {
		return math.NaN(), true, nil
	}
	lo := window[0]
	hi := window[0]
	for i := 1; i < len(window); i++ {
		if window[i] < lo {
			lo = window[i]
		}
		if window[i] > hi {
			hi = window[i]
		}
	}
	return hi - lo, true, nil
}

func (r *Runtime) builtinTAVariance(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if (len(rawArgs) != 2 && len(rawArgs) != 3) || (len(args) != 2 && len(args) != 3) {
		return nil, true, fmt.Errorf("ta.variance(source, length, [biased]) expects 2 or 3 args")
	}
	lf, _ := toFloat(args[1])
	l := int(lf)
	if l <= 0 {
		return math.NaN(), true, nil
	}
	biased := true
	if len(args) == 3 {
		biased = truthy(args[2])
	}
	window, pooled, err := r.seriesWindowFromExpr(rawArgs[0], l)
	if err != nil {
		return nil, true, err
	}
	if pooled {
		defer releaseFloat64Slice(window)
	}
	if len(window) < l {
		return math.NaN(), true, nil
	}
	mean := 0.0
	for _, v := range window {
		mean += v
	}
	mean /= float64(l)
	ss := 0.0
	for _, v := range window {
		d := v - mean
		ss += d * d
	}
	den := float64(l)
	if !biased {
		if l < 2 {
			return math.NaN(), true, nil
		}
		den = float64(l - 1)
	}
	return ss / den, true, nil
}

func (r *Runtime) builtinTADev(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if len(rawArgs) != 2 || len(args) != 2 {
		return nil, true, fmt.Errorf("ta.dev(source, length) expects 2 args")
	}
	lf, _ := toFloat(args[1])
	l := int(lf)
	if l <= 0 {
		return math.NaN(), true, nil
	}
	window, pooled, err := r.seriesWindowFromExpr(rawArgs[0], l)
	if err != nil {
		return nil, true, err
	}
	if pooled {
		defer releaseFloat64Slice(window)
	}
	if len(window) < l {
		return math.NaN(), true, nil
	}
	mean := 0.0
	for _, v := range window {
		mean += v
	}
	mean /= float64(l)
	dev := 0.0
	for _, v := range window {
		dev += math.Abs(v - mean)
	}
	return dev / float64(l), true, nil
}

func (r *Runtime) builtinTARising(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if len(rawArgs) != 2 || len(args) != 2 {
		return nil, true, fmt.Errorf("ta.rising(source, length) expects 2 args")
	}
	lf, _ := toFloat(args[1])
	l := int(lf)
	if l <= 1 {
		return false, true, nil
	}
	window, pooled, err := r.seriesWindowFromExpr(rawArgs[0], l)
	if err != nil {
		return nil, true, err
	}
	if pooled {
		defer releaseFloat64Slice(window)
	}
	if len(window) < l {
		return false, true, nil
	}
	cur := window[len(window)-1]
	maxPrev := window[0]
	for i := 0; i < len(window)-1; i++ {
		if window[i] > maxPrev {
			maxPrev = window[i]
		}
	}
	return cur > maxPrev, true, nil
}

func (r *Runtime) builtinTAFalling(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if len(rawArgs) != 2 || len(args) != 2 {
		return nil, true, fmt.Errorf("ta.falling(source, length) expects 2 args")
	}
	lf, _ := toFloat(args[1])
	l := int(lf)
	if l <= 1 {
		return false, true, nil
	}
	window, pooled, err := r.seriesWindowFromExpr(rawArgs[0], l)
	if err != nil {
		return nil, true, err
	}
	if pooled {
		defer releaseFloat64Slice(window)
	}
	if len(window) < l {
		return false, true, nil
	}
	cur := window[len(window)-1]
	minPrev := window[0]
	for i := 0; i < len(window)-1; i++ {
		if window[i] < minPrev {
			minPrev = window[i]
		}
	}
	return cur < minPrev, true, nil
}

func (r *Runtime) builtinTR(args []interface{}) (interface{}, bool, error) {
	if len(args) > 1 {
		return nil, true, fmt.Errorf("ta.tr([useNa]) expects 0 or 1 arg")
	}
	curH := r.currentValue(r.activeSymbol, "high")
	curL := r.currentValue(r.activeSymbol, "low")
	prevC := r.valueAtOffset(r.activeSymbol, "close", 1)
	if math.IsNaN(curH) || math.IsNaN(curL) {
		return math.NaN(), true, nil
	}
	if math.IsNaN(prevC) {
		return curH - curL, true, nil
	}
	a := curH - curL
	b := math.Abs(curH - prevC)
	c := math.Abs(curL - prevC)
	return math.Max(a, math.Max(b, c)), true, nil
}

func pivotFromArgs(rawArgs []*Expr, args []interface{}, defaultSeries string) (*Expr, int, int, error) {
	var src *Expr
	var left, right int
	switch len(rawArgs) {
	case 2:
		src = &Expr{Kind: "ident", Name: defaultSeries}
		lf, _ := toFloat(args[0])
		rf, _ := toFloat(args[1])
		left, right = int(lf), int(rf)
	case 3:
		src = rawArgs[0]
		lf, _ := toFloat(args[1])
		rf, _ := toFloat(args[2])
		left, right = int(lf), int(rf)
	default:
		return nil, 0, 0, fmt.Errorf("expects (left, right) or (source, left, right)")
	}
	if left < 0 || right < 0 {
		return nil, 0, 0, fmt.Errorf("left/right must be >= 0")
	}
	return src, left, right, nil
}

func (r *Runtime) builtinPivotHigh(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	src, left, right, err := pivotFromArgs(rawArgs, args, "high")
	if err != nil {
		return nil, true, fmt.Errorf("ta.pivothigh %w", err)
	}
	vals, err := r.seriesFromExpr(src)
	if err != nil {
		return nil, true, err
	}
	n := vals.Length()
	candidate := n - 1 - right
	if candidate-left < 0 || candidate+right >= n {
		return math.NaN(), true, nil
	}
	cp := seriesChronoValue(vals, candidate)
	for i := candidate - left; i <= candidate+right; i++ {
		if seriesChronoValue(vals, i) > cp {
			return math.NaN(), true, nil
		}
	}
	return cp, true, nil
}

func (r *Runtime) builtinPivotLow(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	src, left, right, err := pivotFromArgs(rawArgs, args, "low")
	if err != nil {
		return nil, true, fmt.Errorf("ta.pivotlow %w", err)
	}
	vals, err := r.seriesFromExpr(src)
	if err != nil {
		return nil, true, err
	}
	n := vals.Length()
	candidate := n - 1 - right
	if candidate-left < 0 || candidate+right >= n {
		return math.NaN(), true, nil
	}
	cp := seriesChronoValue(vals, candidate)
	for i := candidate - left; i <= candidate+right; i++ {
		if seriesChronoValue(vals, i) < cp {
			return math.NaN(), true, nil
		}
	}
	return cp, true, nil
}

func (r *Runtime) builtinPivotPointLevels(args []interface{}) (interface{}, bool, error) {
	if len(args) < 1 || len(args) > 3 {
		return nil, true, fmt.Errorf("ta.pivot_point_levels(type, anchor[, developing]) expects 1-3 args")
	}
	h := r.valueAtOffset(r.activeSymbol, "high", 1)
	l := r.valueAtOffset(r.activeSymbol, "low", 1)
	c := r.valueAtOffset(r.activeSymbol, "close", 1)
	if math.IsNaN(h) || math.IsNaN(l) || math.IsNaN(c) {
		h = r.currentValue(r.activeSymbol, "high")
		l = r.currentValue(r.activeSymbol, "low")
		c = r.currentValue(r.activeSymbol, "close")
	}
	if math.IsNaN(h) || math.IsNaN(l) || math.IsNaN(c) {
		na := math.NaN()
		return []interface{}{na, na, na, na, na, na, na}, true, nil
	}
	pp := (h + l + c) / 3.0
	r1 := 2*pp - l
	s1 := 2*pp - h
	r2 := pp + (h - l)
	s2 := pp - (h - l)
	r3 := h + 2*(pp-l)
	s3 := l - 2*(h-pp)
	return []interface{}{pp, r1, r2, r3, s1, s2, s3}, true, nil
}

func (r *Runtime) builtinTAMedian(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if len(rawArgs) != 2 || len(args) != 2 {
		return nil, true, fmt.Errorf("ta.median(source, length) expects 2 args")
	}
	lf, _ := toFloat(args[1])
	l := int(lf)
	if l <= 0 {
		return math.NaN(), true, nil
	}
	window, pooled, err := r.seriesWindowFromExpr(rawArgs[0], l)
	if err != nil {
		return nil, true, err
	}
	if pooled {
		defer releaseFloat64Slice(window)
	}
	if len(window) < l {
		return math.NaN(), true, nil
	}
	s := append([]float64(nil), window...)
	sort.Float64s(s)
	m := len(s) / 2
	if len(s)%2 == 1 {
		return s[m], true, nil
	}
	return (s[m-1] + s[m]) / 2, true, nil
}

func (r *Runtime) builtinTAMode(rawArgs []*Expr, args []interface{}) (interface{}, bool, error) {
	if len(rawArgs) != 2 || len(args) != 2 {
		return nil, true, fmt.Errorf("ta.mode(source, length) expects 2 args")
	}
	lf, _ := toFloat(args[1])
	l := int(lf)
	if l <= 0 {
		return math.NaN(), true, nil
	}
	window, pooled, err := r.seriesWindowFromExpr(rawArgs[0], l)
	if err != nil {
		return nil, true, err
	}
	if pooled {
		defer releaseFloat64Slice(window)
	}
	if len(window) < l {
		return math.NaN(), true, nil
	}
	counts := map[string]int{}
	values := map[string]float64{}
	bestKey := ""
	bestCnt := -1
	bestVal := math.NaN()
	for _, v := range window {
		k := strconv.FormatFloat(v, 'g', 15, 64)
		counts[k]++
		values[k] = v
		cnt := counts[k]
		if cnt > bestCnt || (cnt == bestCnt && (math.IsNaN(bestVal) || v < bestVal)) {
			bestCnt = cnt
			bestKey = k
			bestVal = v
		}
	}
	if bestKey == "" {
		return math.NaN(), true, nil
	}
	return values[bestKey], true, nil
}

func isNA(v interface{}) bool {
	if v == nil {
		return true
	}
	f, ok := toFloat(v)
	if !ok {
		return false
	}
	return math.IsNaN(f)
}

func asPineMap(v interface{}) (*pineMap, error) {
	pm, ok := v.(*pineMap)
	if ok && pm != nil {
		if pm.data == nil {
			pm.data = map[interface{}]interface{}{}
		}
		return pm, nil
	}
	return nil, fmt.Errorf("map operation requires map")
}

func normalizeMapKey(v interface{}) (interface{}, error) {
	switch k := v.(type) {
	case nil:
		return nil, errors.New("map key cannot be nil")
	case int:
		return float64(k), nil
	case int8:
		return float64(k), nil
	case int16:
		return float64(k), nil
	case int32:
		return float64(k), nil
	case int64:
		return float64(k), nil
	case uint:
		return float64(k), nil
	case uint8:
		return float64(k), nil
	case uint16:
		return float64(k), nil
	case uint32:
		return float64(k), nil
	case uint64:
		return float64(k), nil
	case float32:
		return float64(k), nil
	case float64, string, bool:
		return k, nil
	default:
		t := reflect.TypeOf(v)
		if t != nil && t.Comparable() {
			return v, nil
		}
		return nil, fmt.Errorf("map key type %T is not comparable", v)
	}
}

func stableMapKeys(m map[interface{}]interface{}) []interface{} {
	if len(m) == 0 {
		return []interface{}{}
	}
	type pair struct {
		key interface{}
		rep string
	}
	items := make([]pair, 0, len(m))
	for k := range m {
		items = append(items, pair{key: k, rep: fmt.Sprintf("%T:%v", k, k)})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].rep < items[j].rep
	})
	out := make([]interface{}, 0, len(items))
	for _, it := range items {
		out = append(out, it.key)
	}
	return out
}

func mathIdentifierValue(name string) (interface{}, bool) {
	switch name {
	case "math.e":
		return math.E, true
	case "math.pi":
		return math.Pi, true
	case "math.phi":
		return (1 + math.Sqrt(5)) / 2, true
	case "math.rphi":
		return (math.Sqrt(5) - 1) / 2, true
	default:
		return nil, false
	}
}

func sessionIdentifierValue(name string) (interface{}, bool) {
	switch name {
	case "session.regular":
		return "regular", true
	case "session.extended":
		return "extended", true
	default:
		return nil, false
	}
}

func timeframeIdentifierValue(name, configured string) (interface{}, bool) {
	tf := normalizeTimeframe(configured)
	if tf == "" {
		tf = "1D"
	}
	base, mult, ok := parseTimeframe(tf)
	if !ok {
		return nil, false
	}
	switch name {
	case "timeframe.period", "timeframe.main_period":
		return tf, true
	case "timeframe.multiplier":
		return float64(mult), true
	case "timeframe.isdaily":
		return base == "D", true
	case "timeframe.isweekly":
		return base == "W", true
	case "timeframe.ismonthly":
		return base == "M", true
	case "timeframe.isdwm":
		return base == "D" || base == "W" || base == "M", true
	case "timeframe.isseconds":
		return base == "S", true
	case "timeframe.isticks":
		return false, true
	case "timeframe.isminutes", "timeframe.isintraday":
		return base == "MIN", true
	default:
		return nil, false
	}
}

func parsePositiveIntFast(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}

func normalizeTimeframe(tf string) string {
	tf = strings.TrimSpace(strings.ToUpper(tf))
	if tf == "" {
		return ""
	}
	if tf == "D" || tf == "W" || tf == "M" || tf == "S" || tf == "T" {
		return "1" + tf
	}
	if n, ok := parsePositiveIntFast(tf); ok && n > 0 {
		return tf
	}
	if strings.HasSuffix(tf, "H") {
		v, ok := parsePositiveIntFast(strings.TrimSuffix(tf, "H"))
		if ok && v > 0 {
			return strconv.Itoa(v * 60)
		}
	}
	return tf
}

func parseTimeframe(tf string) (string, int, bool) {
	tf = normalizeTimeframe(tf)
	if tf == "" {
		return "", 0, false
	}
	if n, ok := parsePositiveIntFast(tf); ok && n > 0 {
		return "MIN", n, true
	}
	unit := tf[len(tf)-1:]
	n := tf[:len(tf)-1]
	if n == "" {
		n = "1"
	}
	mult, ok := parsePositiveIntFast(n)
	if !ok || mult <= 0 {
		return "", 0, false
	}
	switch unit {
	case "S", "T", "D", "W", "M":
		return unit, mult, true
	default:
		return "", 0, false
	}
}

func timeframeInSeconds(tf string) (int, bool) {
	base, mult, ok := parseTimeframe(tf)
	if !ok {
		return 0, false
	}
	return timeframeSecondsFromParsed(base, mult)
}

func timeframeSecondsFromParsed(base string, mult int) (int, bool) {
	switch base {
	case "MIN":
		return mult * 60, true
	case "S":
		return mult, true
	case "D":
		return mult * 86400, true
	case "W":
		return mult * 7 * 86400, true
	case "M":
		return mult * 30 * 86400, true
	case "T":
		return 0, false
	default:
		return 0, false
	}
}

func timeframeFromSeconds(seconds int) string {
	if seconds <= 0 {
		return ""
	}
	if seconds%2592000 == 0 {
		m := seconds / 2592000
		if m == 1 {
			return "1M"
		}
		return fmt.Sprintf("%dM", m)
	}
	if seconds%604800 == 0 {
		w := seconds / 604800
		if w == 1 {
			return "1W"
		}
		return fmt.Sprintf("%dW", w)
	}
	if seconds%86400 == 0 {
		d := seconds / 86400
		if d == 1 {
			return "1D"
		}
		return fmt.Sprintf("%dD", d)
	}
	if seconds%60 == 0 {
		return strconv.Itoa(seconds / 60)
	}
	if seconds == 1 {
		return "1S"
	}
	return fmt.Sprintf("%dS", seconds)
}

func (r *Runtime) builtinTimeframeInSeconds(args []interface{}) (interface{}, bool, error) {
	if len(args) > 1 {
		return nil, true, fmt.Errorf("timeframe.in_seconds([period]) expects 0 or 1 arg")
	}
	if len(args) == 0 {
		if !r.timeframeSecsOK {
			return math.NaN(), true, nil
		}
		return float64(r.timeframeSecs), true, nil
	}
	tf := r.timeframe
	s, ok := args[0].(string)
	if !ok || strings.TrimSpace(s) == "" {
		return nil, true, fmt.Errorf("timeframe.in_seconds period must be a non-empty string")
	}
	tf = s
	seconds, ok := timeframeInSeconds(tf)
	if !ok {
		return math.NaN(), true, nil
	}
	return float64(seconds), true, nil
}

func (r *Runtime) builtinTimeframeFromSeconds(args []interface{}) (interface{}, bool, error) {
	if len(args) != 1 {
		return nil, true, fmt.Errorf("timeframe.from_seconds(seconds) expects 1 arg")
	}
	v, _ := toFloat(args[0])
	seconds := int(v)
	if seconds <= 0 {
		return "", true, nil
	}
	return timeframeFromSeconds(seconds), true, nil
}

func (r *Runtime) builtinTimeframeChange(args []interface{}) (interface{}, bool, error) {
	if len(args) > 1 {
		return nil, true, fmt.Errorf("timeframe.change([period]) expects 0 or 1 arg")
	}
	target := r.timeframe
	if len(args) == 1 {
		s, ok := args[0].(string)
		if !ok || strings.TrimSpace(s) == "" {
			return nil, true, fmt.Errorf("timeframe.change period must be a non-empty string")
		}
		target = s
	}
	curSeconds := r.timeframeSecs
	curOK := r.timeframeSecsOK
	targetSeconds, targetOK := timeframeInSeconds(target)
	if !curOK || !targetOK || targetSeconds <= 0 {
		return false, true, nil
	}
	if targetSeconds <= curSeconds {
		return true, true, nil
	}
	ratio := targetSeconds / curSeconds
	if targetSeconds%curSeconds != 0 {
		ratio++
	}
	if ratio <= 1 {
		return true, true, nil
	}
	idx := r.effectiveBarIndex()
	if idx < 0 {
		return false, true, nil
	}
	return (idx+1)%ratio == 0, true, nil
}

var formatPlaceholderRE = regexp.MustCompile(`\{(\d+)(?:[^}]*)\}`)

func formatStringTemplate(format string, args []interface{}) string {
	const leftSentinel = "\x00LEFT_BRACE\x00"
	const rightSentinel = "\x00RIGHT_BRACE\x00"
	text := strings.ReplaceAll(format, "{{", leftSentinel)
	text = strings.ReplaceAll(text, "}}", rightSentinel)
	text = formatPlaceholderRE.ReplaceAllStringFunc(text, func(token string) string {
		m := formatPlaceholderRE.FindStringSubmatch(token)
		if len(m) < 2 {
			return token
		}
		idx, ok := parsePositiveIntFast(m[1])
		if !ok || idx < 0 || idx >= len(args) {
			return token
		}
		spec := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(token, "{"), "}"))
		parts := strings.Split(spec, ",")
		if len(parts) >= 3 {
			kind := strings.TrimSpace(parts[1])
			if strings.EqualFold(kind, "number") {
				pattern := strings.TrimSpace(strings.Join(parts[2:], ","))
				if out, ok := formatNumberPattern(args[idx], pattern); ok {
					return out
				}
			}
		}
		return toString(args[idx])
	})
	text = strings.ReplaceAll(text, leftSentinel, "{")
	text = strings.ReplaceAll(text, rightSentinel, "}")
	return text
}

func formatNumberPattern(v interface{}, pattern string) (string, bool) {
	n, ok := toFloat(v)
	if !ok {
		return "", false
	}
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return fmt.Sprintf("%v", n), true
	}
	if pattern == "" {
		return toString(v), true
	}
	normalized := strings.ReplaceAll(pattern, ",", "")
	dot := strings.Index(normalized, ".")
	intPattern := normalized
	fracPattern := ""
	if dot >= 0 {
		intPattern = normalized[:dot]
		fracPattern = normalized[dot+1:]
	}
	for _, ch := range intPattern {
		if ch != '#' && ch != '0' {
			return "", false
		}
	}
	for _, ch := range fracPattern {
		if ch != '#' && ch != '0' {
			return "", false
		}
	}
	minInt := strings.Count(intPattern, "0")
	minFrac := strings.Count(fracPattern, "0")
	maxFrac := len(fracPattern)

	text := strconv.FormatFloat(n, 'f', maxFrac, 64)
	if maxFrac > 0 {
		sign := ""
		if strings.HasPrefix(text, "-") {
			sign = "-"
			text = text[1:]
		}
		parts := strings.SplitN(text, ".", 2)
		frac := ""
		if len(parts) == 2 {
			frac = parts[1]
		}
		for len(frac) > minFrac && strings.HasSuffix(frac, "0") {
			frac = frac[:len(frac)-1]
		}
		text = sign + parts[0]
		if frac != "" {
			text += "." + frac
		}
	}

	if minInt > 1 {
		sign := ""
		if strings.HasPrefix(text, "-") {
			sign = "-"
			text = text[1:]
		}
		parts := strings.SplitN(text, ".", 2)
		for len(parts[0]) < minInt {
			parts[0] = "0" + parts[0]
		}
		text = sign + parts[0]
		if len(parts) == 2 {
			text += "." + parts[1]
		}
	}
	return text, true
}
