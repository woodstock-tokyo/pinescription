// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

type tickerID struct {
	symbol, chart string
	modifiers     map[string]interface{}
}

func decodeTicker(s string) (tickerID, error) {
	base, chart, _ := strings.Cut(s, ";")
	id := tickerID{symbol: base, chart: chart, modifiers: map[string]interface{}{}}
	if strings.HasPrefix(base, "=") {
		if err := json.Unmarshal([]byte(base[1:]), &id.modifiers); err != nil {
			return id, fmt.Errorf("invalid encoded ticker: %w", err)
		}
		symbol, ok := id.modifiers["symbol"].(string)
		if !ok || symbol == "" {
			return id, fmt.Errorf("encoded ticker requires symbol")
		}
		id.symbol = symbol
		delete(id.modifiers, "symbol")
	}
	return id, nil
}

func (id tickerID) encode() (string, error) {
	s := id.symbol
	if len(id.modifiers) > 0 {
		m := map[string]interface{}{"symbol": s}
		for k, v := range id.modifiers {
			m[k] = v
		}
		data, err := json.Marshal(m)
		if err != nil {
			return "", err
		}
		s = "=" + string(data)
	}
	if id.chart != "" {
		s += ";" + id.chart
	}
	return s, nil
}

func applyTickerModifiers(id *tickerID, args []interface{}) error {
	for i, arg := range args {
		if arg == nil {
			continue
		}
		s, ok := arg.(string)
		if !ok {
			return fmt.Errorf("ticker modifier must be a string")
		}
		if s == "" || s == "inherit" {
			continue
		}
		switch i {
		case 0:
			switch s {
			case "regular":
				delete(id.modifiers, "session")
			case "extended":
				id.modifiers["session"] = s
			default:
				return fmt.Errorf("invalid ticker session %q", s)
			}
		case 1:
			switch s {
			case "none", "splits", "dividends":
				id.modifiers["adjustment"] = s
			default:
				return fmt.Errorf("invalid ticker adjustment %q", s)
			}
		case 2:
			switch s {
			case "on":
				id.modifiers["backadjustment"] = "default"
			case "off":
				delete(id.modifiers, "backadjustment")
			default:
				return fmt.Errorf("invalid backadjustment %q", s)
			}
		case 3:
			switch s {
			case "on":
				id.modifiers["settlement-as-close"] = true
			case "off":
				id.modifiers["settlement-as-close"] = false
			default:
				return fmt.Errorf("invalid settlement_as_close %q", s)
			}
		}
	}
	return nil
}

func (r *Runtime) builtinTicker(name string, args []interface{}) (interface{}, error) {
	switch name {
	case "ticker.renko", "ticker.kagi", "ticker.linebreak", "ticker.pointfigure":
		return alternativeTicker(name, args)
	}
	if name == "ticker.standard" && len(args) == 1 && isNA(args[0]) {
		args = nil
	}
	min, max := 1, 1
	switch name {
	case "ticker.new":
		min, max = 2, 6
	case "ticker.modify":
		min, max = 1, 5
	case "ticker.inherit":
		min, max = 2, 2
	case "ticker.standard":
		min, max = 0, 1
	}
	if len(args) < min || len(args) > max {
		return nil, fmt.Errorf("%s expects %d to %d args", name, min, max)
	}
	texts := make([]string, len(args))
	for i, arg := range args {
		if arg == nil && i > 0 {
			continue
		}
		s, ok := arg.(string)
		if !ok {
			return nil, fmt.Errorf("%s argument %d must be a string", name, i+1)
		}
		texts[i] = s
	}
	if name == "ticker.new" {
		symbol := texts[0]
		if symbol != "" && texts[1] != "" {
			symbol += ":"
		}
		symbol += texts[1]
		if symbol == "" {
			return nil, fmt.Errorf("ticker.new requires a symbol")
		}
		id := tickerID{symbol: symbol, modifiers: map[string]interface{}{}}
		if err := applyTickerModifiers(&id, args[2:]); err != nil {
			return nil, err
		}
		return id.encode()
	}
	s := r.activeSymbol
	if len(texts) > 0 {
		s = texts[0]
	}
	id, err := decodeTicker(s)
	if err != nil {
		return nil, err
	}
	switch name {
	case "ticker.standard":
		return id.symbol, nil
	case "ticker.heikinashi":
		delete(id.modifiers, "chart-type")
		delete(id.modifiers, "chart-parameters")
		id.chart = "heikinashi"
	case "ticker.inherit":
		target, err := decodeTicker(texts[1])
		if err != nil {
			return nil, err
		}
		id.symbol = target.symbol
	case "ticker.modify":
		if err := applyTickerModifiers(&id, args[1:]); err != nil {
			return nil, err
		}
	}
	return id.encode()
}

// TickerDescriptor is the provider contract for modified ticker IDs. Alternative
// constructors describe requested bars; they never silently substitute candles.
type TickerDescriptor struct {
	Symbol, Chart string
	Parameters    map[string]interface{}
	Modifiers     map[string]interface{}
}

func ParseTickerID(value string) (TickerDescriptor, error) {
	id, err := decodeTicker(value)
	if err != nil {
		return TickerDescriptor{}, err
	}
	out := TickerDescriptor{Symbol: id.symbol, Chart: "standard", Modifiers: id.modifiers}
	if id.chart != "" {
		out.Chart = id.chart
	}
	if chart, ok := id.modifiers["chart-type"].(string); ok {
		out.Chart = chart
	}
	if params, ok := id.modifiers["chart-parameters"].(map[string]interface{}); ok {
		out.Parameters = params
	}
	return out, nil
}

func alternativeTicker(name string, args []interface{}) (interface{}, error) {
	min, max := 2, 2
	switch name {
	case "ticker.renko":
		min, max = 3, 5
	case "ticker.pointfigure":
		min, max = 5, 5
	}
	if len(args) < min || len(args) > max {
		return nil, fmt.Errorf("%s expects %d to %d args", name, min, max)
	}
	symbol, ok := args[0].(string)
	if !ok || symbol == "" {
		return nil, fmt.Errorf("%s requires a symbol", name)
	}
	id, err := decodeTicker(symbol)
	if err != nil {
		return nil, err
	}
	params := map[string]interface{}{}
	text := func(i int) (string, error) {
		s, ok := args[i].(string)
		if !ok {
			return "", fmt.Errorf("%s argument %d must be a string", name, i+1)
		}
		return s, nil
	}
	number := func(i int) (float64, error) {
		if _, b := args[i].(bool); b {
			return 0, fmt.Errorf("%s argument %d must be positive numeric", name, i+1)
		}
		n, ok := toFloat(args[i])
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n <= 0 {
			return 0, fmt.Errorf("%s argument %d must be positive numeric", name, i+1)
		}
		return n, nil
	}
	style := func(i int) error {
		s, e := text(i)
		if e != nil {
			return e
		}
		if s != "ATR" && s != "Traditional" {
			return fmt.Errorf("%s style must be ATR or Traditional", name)
		}
		params["style"] = s
		return nil
	}
	switch name {
	case "ticker.linebreak":
		n, e := integerArg(args[1], "number_of_lines", 1, maxArrayNewSize)
		if e != nil {
			return nil, e
		}
		params["number_of_lines"] = n
	case "ticker.kagi":
		n, e := number(1)
		if e != nil {
			return nil, e
		}
		params["reversal"] = n
	case "ticker.renko":
		if e := style(1); e != nil {
			return nil, e
		}
		n, e := number(2)
		if e != nil {
			return nil, e
		}
		params["param"] = n
		if params["style"] == "ATR" && n != math.Trunc(n) {
			return nil, fmt.Errorf("ATR length must be an integer")
		}
		params["request_wicks"], params["source"] = false, "Close"
		if len(args) > 3 {
			b, ok := args[3].(bool)
			if !ok {
				return nil, fmt.Errorf("request_wicks must be bool")
			}
			params["request_wicks"] = b
		}
		if len(args) > 4 {
			s, e := text(4)
			if e != nil {
				return nil, e
			}
			if s != "Close" && s != "OHLC" {
				return nil, fmt.Errorf("Renko source must be Close or OHLC")
			}
			params["source"] = s
		}
	case "ticker.pointfigure":
		s, e := text(1)
		if e != nil {
			return nil, e
		}
		if s != "hl" && s != "close" {
			return nil, fmt.Errorf("Point & Figure source must be hl or close")
		}
		params["source"] = s
		if e := style(2); e != nil {
			return nil, e
		}
		n, e := number(3)
		if e != nil {
			return nil, e
		}
		params["param"] = n
		n, e = number(4)
		if e != nil {
			return nil, e
		}
		params["reversal"] = n
	}
	id.chart = ""
	id.modifiers["chart-type"] = strings.TrimPrefix(name, "ticker.")
	id.modifiers["chart-parameters"] = params
	return id.encode()
}
