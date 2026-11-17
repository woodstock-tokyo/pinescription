// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/woodstock-tokyo/pinescription/series"
)

// SecurityData is one immutable requested symbol/timeframe context. Series keys
// are value types (close, open, volume...), not symbol|type. Actual timestamps
// are mandatory; the engine never aligns independently fabricated clocks.
type SecurityData struct {
	Series   map[string]SeriesExtended
	Bars     []BarTime
	Info     SymbolInfo
	Sessions MarketSessions
}
type SecurityDataProvider interface {
	GetSecurityData(symbol, timeframe string) (SecurityData, error)
}

func (e *Engine) SetSecurityDataProvider(provider SecurityDataProvider) {
	e.securityProvider = provider
}

func (e *Engine) securityLoader(catalog *providerCatalog) func(string, string) (SecurityData, error) {
	return func(symbol, tf string) (SecurityData, error) {
		if e.securityProvider != nil {
			return e.securityProvider.GetSecurityData(symbol, tf)
		}
		provider := catalog.providerBySymbol[symbol]
		if provider == nil {
			if d, err := ParseTickerID(symbol); err == nil {
				provider = catalog.providerBySymbol[d.Symbol]
			}
		}
		if source, ok := provider.(SecurityDataProvider); ok {
			return source.GetSecurityData(symbol, tf)
		}
		return SecurityData{}, fmt.Errorf("request %s/%s requires a SecurityDataProvider", symbol, tf)
	}
}

type requestSession struct {
	loader         func(string, string) (SecurityData, error)
	data           map[string]SecurityData
	failures       map[string]error
	depth, streams int
}
type requestKey struct {
	expr              *Expr
	symbol, tf, scope string
	count             int
}
type requestStream struct {
	runtime *Runtime
	expr    *Expr
	data    SecurityData
	values  []interface{}
}

func (r *Runtime) evalSecurity(call *Expr) (interface{}, error) {
	name := call.Left.Name
	lower := name == "request.security_lower_tf"
	names := []string{"symbol", "timeframe", "expression", "gaps", "lookahead", "ignore_invalid_symbol", "currency", "calc_bars_count"}
	if lower {
		names = []string{"symbol", "timeframe", "expression", "ignore_invalid_symbol", "currency", "ignore_invalid_timeframe", "calc_bars_count"}
	}
	raw, err := bindNamedCallArgs(name, call.Args, callParamSpec{Names: names, Required: 3})
	if err != nil {
		return nil, err
	}
	opts := map[string]interface{}{}
	for i, arg := range raw {
		if i == 2 || arg == nil {
			continue
		}
		v, err := r.eval(arg)
		if err != nil {
			return nil, err
		}
		opts[names[i]] = v
	}
	symbol, ok := opts["symbol"].(string)
	if !ok {
		return nil, fmt.Errorf("%s symbol must be a string", name)
	}
	tf, ok := opts["timeframe"].(string)
	if !ok {
		return nil, fmt.Errorf("%s timeframe must be a string", name)
	}
	if symbol == "" {
		symbol = r.activeSymbol
	}
	if tf == "" {
		tf = r.timeframe
	}
	tf = normalizeTimeframe(tf)
	seconds, valid := timeframeInSeconds(tf)
	if !valid {
		return nil, fmt.Errorf("%s invalid time-based timeframe %q", name, tf)
	}
	boolean := func(key string) (bool, error) {
		v, exists := opts[key]
		if !exists {
			return false, nil
		}
		b, ok := v.(bool)
		if !ok {
			return false, fmt.Errorf("%s %s must be bool", name, key)
		}
		return b, nil
	}
	ignore, err := boolean("ignore_invalid_symbol")
	if err != nil {
		return nil, err
	}
	ignoreTF, err := boolean("ignore_invalid_timeframe")
	if err != nil {
		return nil, err
	}
	chartSeconds, chartOK := timeframeInSeconds(r.timeframe)
	if !chartOK {
		return nil, fmt.Errorf("%s requires time-based chart bars", name)
	}
	if lower && seconds > chartSeconds {
		if ignoreTF {
			return emptyIntrabars(raw[2], r.program), nil
		}
		return nil, fmt.Errorf("%s timeframe must not exceed chart timeframe", name)
	}
	currency := ""
	if v, exists := opts["currency"]; exists {
		var ok bool
		currency, ok = v.(string)
		if !ok {
			return nil, fmt.Errorf("%s currency must be a string", name)
		}
	}
	gaps, lookahead := "gaps_off", "lookahead_off"
	for key, dst := range map[string]*string{"gaps": &gaps, "lookahead": &lookahead} {
		if v, exists := opts[key]; exists {
			s, ok := v.(string)
			if !ok || (s != key+"_on" && s != key+"_off") {
				return nil, fmt.Errorf("%s invalid %s", name, key)
			}
			*dst = s
		}
	}
	count := 0
	if v, exists := opts["calc_bars_count"]; exists {
		count, err = integerArg(v, "calc_bars_count", 1, maxArrayNewSize)
		if err != nil {
			return nil, err
		}
	}
	if symbol == r.activeSymbol && tf == normalizeTimeframe(r.timeframe) {
		if currency != "" && r.rootNamespaces["syminfo"].(map[string]interface{})["currency"] != currency {
			return nil, fmt.Errorf("request currency conversion is not configured")
		}
		value, err := r.eval(raw[2])
		if err != nil {
			return nil, err
		}
		if lower {
			return intrabarValues([]interface{}{value})
		}
		return cloneRequestValue(value)
	}
	if r.requests == nil {
		return nil, fmt.Errorf("%s requires an engine request context", name)
	}
	if r.requests.depth >= 16 {
		return nil, fmt.Errorf("nested request depth limit exceeded")
	}
	key := requestKey{call, symbol, tf, r.scriptCallPath, count}
	if r.requestCache == nil {
		r.requestCache = map[requestKey]*requestStream{}
	}
	stream := r.requestCache[key]
	if stream == nil {
		contextKey := symbol + "\x00" + tf
		data, exists := r.requests.data[contextKey]
		loadErr := r.requests.failures[contextKey]
		if !exists && loadErr == nil {
			if len(r.requests.data)+len(r.requests.failures) >= 40 {
				return nil, fmt.Errorf("unique request context limit (40) exceeded")
			}
			data, loadErr = r.requests.loader(symbol, tf)
			if loadErr == nil {
				loadErr = validateSecurityData(data)
			}
			if loadErr != nil {
				r.requests.failures[contextKey] = loadErr
			} else {
				r.requests.data[contextKey] = data
			}
		}
		if loadErr != nil {
			if ignore {
				return math.NaN(), nil
			}
			return nil, loadErr
		}
		if count > 0 && count < len(data.Bars) {
			data = sliceSecurityData(data, len(data.Bars)-count)
		}
		if r.requests.streams >= 128 {
			return nil, fmt.Errorf("request expression stream limit (128) exceeded")
		}
		expr, err := r.requestExpression(raw[2], map[string]bool{})
		if err != nil {
			return nil, err
		}
		source := r.program
		if r.executingStatement >= 0 && r.executingStatement < len(source.Stmts) {
			source.Stmts = source.Stmts[:r.executingStatement+1]
		}
		program, err := requestProgram(source, expr, call)
		if err != nil {
			return nil, err
		}
		child := newRuntime(program, r.userFns, map[string]SeriesExtended{}, map[string]map[string]bool{symbol: {}}, symbol, "close", tf, r.session, r.currentTime, data.Bars[0].Open, len(data.Bars), nil, r.logSink, r.alertSink)
		child.strictSeries = true
		child.footprintLoader = r.footprintLoader
		child.requests = r.requests
		child.inputBindings = r.inputBindings
		child.userFnParamSpecs = r.userFnParamSpecs
		for vt, ser := range data.Series {
			child.seriesByKey[makeSeriesKey(symbol, vt)] = ser
			child.valueTypesBySymbol[symbol][vt] = true
		}
		info := SymbolInfo{}
		for k, v := range data.Info {
			info[k] = v
		}
		loc := time.UTC
		if zone, ok := info["timezone"].(string); ok {
			loc, err = checkedTimezone(zone)
			if err != nil {
				child.Release()
				return nil, err
			}
		}
		child.applyMarketContext(marketContext{info: info, bars: data.Bars, sessions: data.Sessions, location: loc})
		stream = &requestStream{runtime: child, expr: expr, data: data}
		r.requestCache[key] = stream
		r.requests.streams++
	}
	if currency != "" && stream.data.Info["currency"] != currency {
		return nil, fmt.Errorf("request currency conversion from %v to %s is not configured", stream.data.Info["currency"], currency)
	}
	open, close, _ := r.barTimes()
	if lower {
		first := sort.Search(len(stream.data.Bars), func(i int) bool { return !stream.data.Bars[i].Open.Before(open) })
		end := first
		for end < len(stream.data.Bars) && stream.data.Bars[end].Open.Before(close) && !stream.data.Bars[end].Close.After(close) {
			end++
		}
		if end == first {
			return emptyIntrabars(raw[2], r.program), nil
		}
		if err := stream.advance(end - 1); err != nil {
			return nil, err
		}
		return intrabarValues(stream.values[first:end])
	}
	bars := stream.data.Bars
	index := -1
	if seconds < chartSeconds && lookahead == "lookahead_on" {
		first := sort.Search(len(bars), func(i int) bool { return !bars[i].Open.Before(open) })
		if first < len(bars) && bars[first].Open.Before(close) && !bars[first].Close.After(close) {
			index = first
		}
	} else if lookahead == "lookahead_on" {
		index = sort.Search(len(bars), func(i int) bool { return bars[i].Open.After(open) }) - 1
	} else {
		index = sort.Search(len(bars), func(i int) bool { return bars[i].Close.After(close) }) - 1
	}
	if index < 0 && gaps == "gaps_off" {
		index = sort.Search(len(bars), func(i int) bool { return bars[i].Close.After(open) }) - 1
	}
	if index < 0 {
		return math.NaN(), nil
	}
	if gaps == "gaps_on" {
		boundary := open
		if r.effectiveBarIndex() > 0 {
			po, pc, _ := r.timesAtBar(r.effectiveBarIndex() - 1)
			if lookahead == "lookahead_on" {
				boundary = po
			} else {
				boundary = pc
			}
		}
		if lookahead == "lookahead_on" {
			if !bars[index].Open.After(boundary) && !bars[index].Open.Equal(open) {
				return math.NaN(), nil
			}
		} else if !bars[index].Close.After(boundary) {
			return math.NaN(), nil
		}
	}
	if err := stream.advance(index); err != nil {
		return nil, err
	}
	return cloneRequestValue(stream.values[index])
}

func validateSecurityData(data SecurityData) error {
	if raw, exists := data.Info["mintick"]; exists {
		tick, ok := toFloat(raw)
		if !ok || math.IsNaN(tick) || math.IsInf(tick, 0) || tick <= 0 {
			return fmt.Errorf("security syminfo.mintick must be positive and finite")
		}
	}
	close := data.Series["close"]
	if close == nil || len(data.Bars) == 0 || close.Length() != len(data.Bars) {
		return fmt.Errorf("security data requires close series and matching nonempty bar timestamps")
	}
	for vt, s := range data.Series {
		if s == nil || s.Length() != len(data.Bars) {
			return fmt.Errorf("security series %s has a mismatched length", vt)
		}
	}
	for i, b := range data.Bars {
		if b.Open.IsZero() || !b.Close.After(b.Open) || i > 0 && (!b.Open.After(data.Bars[i-1].Open) || !b.Close.After(data.Bars[i-1].Close)) {
			return fmt.Errorf("security timestamps must be ordered at index %d", i)
		}
	}
	return nil
}
func sliceSecurityData(data SecurityData, start int) SecurityData {
	out := data
	out.Series = map[string]SeriesExtended{}
	out.Bars = append([]BarTime(nil), data.Bars[start:]...)
	for vt, s := range data.Series {
		q := series.NewQueue(s.Length() - start + 8)
		for i := start; i < s.Length(); i++ {
			q.Update(s.Last(s.Length() - 1 - i))
		}
		out.Series[vt] = q
	}
	return out
}
func (s *requestStream) advance(index int) error {
	s.runtime.requests.depth++
	defer func() { s.runtime.requests.depth-- }()
	for len(s.values) <= index {
		s.runtime.SetBarIndex(len(s.values))
		if err := s.runtime.execTopLevel(); err != nil {
			return fmt.Errorf("requested context: %w", err)
		}
		value, err := s.runtime.eval(s.expr)
		if err != nil {
			return fmt.Errorf("requested expression: %w", err)
		}
		snapshot, err := cloneRequestValue(value)
		if err != nil {
			return err
		}
		s.values = append(s.values, snapshot)
		if err := s.runtime.commitBar(); err != nil {
			return err
		}
	}
	return nil
}
func cloneRequestValue(value interface{}) (interface{}, error) {
	type identity struct {
		kind    uint8
		pointer uintptr
		length  int
	}
	seen := map[identity]interface{}{}
	budget := maxArrayNewSize
	var clone func(interface{}, int) (interface{}, error)
	clone = func(value interface{}, depth int) (interface{}, error) {
		budget--
		if budget < 0 || depth > 64 {
			return nil, fmt.Errorf("request value snapshot limit exceeded")
		}
		switch v := value.(type) {
		case []interface{}:
			key := identity{1, reflect.ValueOf(v).Pointer(), len(v)}
			if out, ok := seen[key]; ok {
				return out, nil
			}
			if len(v) > budget {
				return nil, fmt.Errorf("request collection is too large")
			}
			out := make([]interface{}, len(v))
			seen[key] = out
			for i, x := range v {
				copy, err := clone(x, depth+1)
				if err != nil {
					return nil, err
				}
				out[i] = copy
			}
			return out, nil
		case *pineArray:
			if v == nil {
				return nil, nil
			}
			key := identity{2, reflect.ValueOf(v).Pointer(), 0}
			if out, ok := seen[key]; ok {
				return out, nil
			}
			out := &pineArray{}
			seen[key] = out
			items, err := clone(v.items, depth+1)
			if err != nil {
				return nil, err
			}
			out.items = items.([]interface{})
			return out, nil
		case *customTypeInstance:
			if v == nil {
				return nil, nil
			}
			key := identity{3, reflect.ValueOf(v).Pointer(), 0}
			if out, ok := seen[key]; ok {
				return out, nil
			}
			out := &customTypeInstance{TypeName: v.TypeName, Fields: map[string]interface{}{}}
			seen[key] = out
			for k, x := range v.Fields {
				copy, err := clone(x, depth+1)
				if err != nil {
					return nil, err
				}
				out.Fields[k] = copy
			}
			return out, nil
		case *pineMap:
			if v == nil {
				return nil, nil
			}
			key := identity{4, reflect.ValueOf(v).Pointer(), 0}
			if out, ok := seen[key]; ok {
				return out, nil
			}
			out := &pineMap{data: map[interface{}]interface{}{}}
			seen[key] = out
			for k, x := range v.data {
				copy, err := clone(x, depth+1)
				if err != nil {
					return nil, err
				}
				out.data[k] = copy
			}
			return out, nil
		case *Matrix:
			if v == nil {
				return v.copy(), nil
			}
			key := identity{5, reflect.ValueOf(v).Pointer(), 0}
			if out, ok := seen[key]; ok {
				return out, nil
			}
			size := v.elementsCount()
			if size > budget {
				return nil, fmt.Errorf("request matrix snapshot limit exceeded")
			}
			budget -= size
			out := v.copy()
			seen[key] = out
			return out, nil
		}
		return value, nil
	}
	return clone(value, 0)
}
func requestTupleSize(expr *Expr, program Program) int {
	var shape func(*Expr, int) int
	var bodyShape func([]Stmt, int) int
	bodyShape = func(body []Stmt, depth int) int {
		if len(body) == 0 || depth > 32 {
			return 0
		}
		tail := body[len(body)-1]
		if tail.Kind == "if" {
			a, b := bodyShape(tail.Then, depth+1), bodyShape(tail.Else, depth+1)
			if a == b {
				return a
			}
			return 0
		}
		return shape(tail.Expr, depth+1)
	}
	shape = func(expr *Expr, depth int) int {
		if expr == nil || depth > 32 {
			return 0
		}
		if expr.Kind == "array" || expr.Kind == "tuple" {
			return len(expr.Elems)
		}
		if expr.Kind == "ident" {
			for i := len(program.Stmts) - 1; i >= 0; i-- {
				s := program.Stmts[i]
				if s.Name == expr.Name {
					return shape(s.Expr, depth+1)
				}
			}
		}
		if expr.Kind == "call" && expr.Left != nil {
			if fn, ok := program.Functions[expr.Left.Name]; ok {
				if fn.Expr != nil {
					return shape(fn.Expr, depth+1)
				}
				return bodyShape(fn.Body, depth+1)
			}
			switch expr.Left.Name {
			case "ta.bb", "bb", "ta.kc", "kc", "ta.macd", "macd", "ta.dmi", "dmi":
				return 3
			case "ta.supertrend", "supertrend":
				return 2
			case "ta.vwap":
				if len(expr.Args) == 3 {
					return 3
				}
			}
		}
		return 0
	}
	return shape(expr, 0)
}
func emptyIntrabars(expr *Expr, program Program) interface{} {
	if n := requestTupleSize(expr, program); n > 0 {
		out := make([]interface{}, n)
		for i := range out {
			out[i] = &pineArray{}
		}
		return out
	}
	return &pineArray{}
}
func intrabarValues(values []interface{}) (interface{}, error) {
	if first, ok := values[0].([]interface{}); ok {
		out := make([]interface{}, len(first))
		for i := range out {
			out[i] = &pineArray{}
		}
		for _, value := range values {
			tuple, ok := value.([]interface{})
			if !ok || len(tuple) != len(out) {
				return nil, fmt.Errorf("request tuple shape changed")
			}
			for i, x := range tuple {
				a := out[i].(*pineArray)
				copy, err := cloneRequestValue(x)
				if err != nil {
					return nil, err
				}
				a.items = append(a.items, copy)
			}
		}
		return out, nil
	}
	out := &pineArray{}
	for _, v := range values {
		copy, err := cloneRequestValue(v)
		if err != nil {
			return nil, err
		}
		out.items = append(out.items, copy)
	}
	return out, nil
}

// Substitute a series parameter's source expression, never the chart's scalar
// value, when security is called through a user-defined function.
func (r *Runtime) requestExpression(expr *Expr, seen map[string]bool) (*Expr, error) {
	if expr == nil {
		return nil, nil
	}
	if _, bound := r.inputBindings[expr]; bound {
		return expr, nil
	}
	if expr.Kind == "ident" {
		for i := len(r.envStack) - 1; i > 0; i-- {
			if value, ok := r.envStack[i][expr.Name]; ok {
				arg, ok := value.(seriesArgument)
				if !ok || arg.expr == nil {
					return nil, fmt.Errorf("request expression captures unsupported mutable local %s", expr.Name)
				}
				if seen[expr.Name] {
					return nil, fmt.Errorf("cyclic request parameter %s", expr.Name)
				}
				seen[expr.Name] = true
				out, err := r.requestExpression(arg.expr, seen)
				delete(seen, expr.Name)
				return out, err
			}
		}
	}
	copy := *expr
	var err error
	copy.Left, err = r.requestExpression(expr.Left, seen)
	if err != nil {
		return nil, err
	}
	copy.Right, err = r.requestExpression(expr.Right, seen)
	if err != nil {
		return nil, err
	}
	copy.Else, err = r.requestExpression(expr.Else, seen)
	if err != nil {
		return nil, err
	}
	copy.Args = make([]*Expr, len(expr.Args))
	for i, e := range expr.Args {
		copy.Args[i], err = r.requestExpression(e, seen)
		if err != nil {
			return nil, err
		}
	}
	copy.Elems = make([]*Expr, len(expr.Elems))
	for i, e := range expr.Elems {
		copy.Elems[i], err = r.requestExpression(e, seen)
		if err != nil {
			return nil, err
		}
	}
	copy.SwitchExpr, err = r.requestExpression(expr.SwitchExpr, seen)
	if err != nil {
		return nil, err
	}
	copy.Cases = make([]SwitchCase, len(expr.Cases))
	for i, c := range expr.Cases {
		copy.Cases[i] = c
		copy.Cases[i].Match, err = r.requestExpression(c.Match, seen)
		if err != nil {
			return nil, err
		}
		copy.Cases[i].Body, err = r.requestSwitchBody(c.Body, seen)
		if err != nil {
			return nil, err
		}
	}
	copy.Default, err = r.requestSwitchBody(expr.Default, seen)
	if err != nil {
		return nil, err
	}
	return &copy, nil
}

func (r *Runtime) requestSwitchBody(body []Stmt, seen map[string]bool) ([]Stmt, error) {
	if body == nil {
		return nil, nil
	}
	out := make([]Stmt, len(body))
	for i, stmt := range body {
		out[i] = stmt
		if stmt.Kind != "expr" {
			return nil, fmt.Errorf("request expression switch body has unsupported statement kind %s", stmt.Kind)
		}
		var err error
		out[i].Expr, err = r.requestExpression(stmt.Expr, seen)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Slice only global definitions needed by the expression. Unrelated plots,
// alerts, host callbacks, and other requests are not replayed in the secondary.
func requestProgram(source Program, expr, call *Expr) (Program, error) {
	out := source
	out.Stmts = nil
	out.Imports = nil
	writes := map[string]bool{}
	var collectWrites func([]Stmt)
	collectWrites = func(stmts []Stmt) {
		for _, s := range stmts {
			if s.Name != "" {
				writes[s.Name] = true
			}
			for _, n := range s.TupleNames {
				writes[n] = true
			}
			collectWrites(s.Then)
			collectWrites(s.Else)
			collectWrites(s.Body)
			for _, c := range s.Cases {
				collectWrites(c.Body)
			}
			collectWrites(s.Default)
		}
	}
	collectWrites(source.Stmts)
	needed := map[string]bool{}
	visitedFns := map[string]bool{}
	var scan func(*Expr)
	var scanStatements func([]Stmt)
	var scopedScan func(*Expr, map[string]bool)
	var scopedStatements func([]Stmt, map[string]bool)
	scopedStatements = func(stmts []Stmt, locals map[string]bool) {
		for _, s := range stmts {
			for _, e := range []*Expr{s.Expr, s.Cond, s.Target, s.From, s.To, s.By, s.ForIn, s.SwitchExpr} {
				scopedScan(e, locals)
			}
			scopedStatements(s.Then, locals)
			scopedStatements(s.Else, locals)
			scopedStatements(s.Body, locals)
			for _, c := range s.Cases {
				scopedScan(c.Match, locals)
				scopedStatements(c.Body, locals)
			}
			scopedStatements(s.Default, locals)
		}
	}
	scopedScan = func(e *Expr, locals map[string]bool) {
		if e == nil {
			return
		}
		if e.Kind == "ident" && writes[e.Name] && !locals[strings.SplitN(e.Name, ".", 2)[0]] {
			needed[e.Name] = true
		}
		if e.Kind == "call" && e.Left != nil && !visitedFns[e.Left.Name] {
			if f, ok := source.Functions[e.Left.Name]; ok {
				visitedFns[e.Left.Name] = true
				bound := map[string]bool{}
				for _, p := range f.Params {
					bound[p] = true
				}
				var bind func([]Stmt)
				bind = func(stmts []Stmt) {
					for _, s := range stmts {
						if s.Name != "" {
							bound[s.Name] = true
						}
						for _, n := range s.TupleNames {
							bound[n] = true
						}
						if s.ForVar != "" {
							bound[s.ForVar] = true
						}
						bind(s.Then)
						bind(s.Else)
						bind(s.Body)
						bind(s.Default)
						for _, c := range s.Cases {
							bind(c.Body)
						}
					}
				}
				bind(f.Body)
				scopedScan(f.Expr, bound)
				scopedStatements(f.Body, bound)
				for _, d := range f.ParamDefaults {
					scopedScan(d, bound)
				}
			}
			if t, ok := source.Types[strings.TrimSuffix(e.Left.Name, ".new")]; ok {
				for _, f := range t.Fields {
					scopedScan(f.Default, nil)
				}
			}
		}
		scopedScan(e.Left, locals)
		scopedScan(e.Right, locals)
		scopedScan(e.Else, locals)
		for _, a := range e.Args {
			scopedScan(a, locals)
		}
		for _, a := range e.Elems {
			scopedScan(a, locals)
		}
		scopedScan(e.SwitchExpr, locals)
		for _, c := range e.Cases {
			scopedScan(c.Match, locals)
			scopedStatements(c.Body, locals)
		}
		scopedStatements(e.Default, locals)
	}
	scan = func(e *Expr) { scopedScan(e, nil) }
	scanStatements = func(stmts []Stmt) { scopedStatements(stmts, nil) }
	scan(expr)
	var selected func(Stmt) bool
	selected = func(s Stmt) bool {
		if requestMutation(s.Expr, needed) {
			return true
		}
		if needed[s.Name] {
			return true
		}
		for _, n := range s.TupleNames {
			if needed[n] {
				return true
			}
		}
		for _, block := range [][]Stmt{s.Then, s.Else, s.Body, s.Default} {
			for _, c := range block {
				if selected(c) {
					return true
				}
			}
		}
		for _, c := range s.Cases {
			for _, b := range c.Body {
				if selected(b) {
					return true
				}
			}
		}
		return false
	}
	for pass := 0; pass <= len(writes); pass++ {
		before := len(needed)
		// Reference assignments can alias a needed collection in either
		// direction. Include their declarations and mutations transitively.
		var aliases func([]Stmt)
		aliases = func(stmts []Stmt) {
			for _, s := range stmts {
				if (s.Kind == "decl" || s.Kind == "assign") && s.Expr != nil && s.Expr.Kind == "ident" {
					if needed[s.Name] || needed[s.Expr.Name] {
						needed[s.Name], needed[s.Expr.Name] = true, true
					}
				}
				for _, block := range [][]Stmt{s.Then, s.Else, s.Body, s.Default} {
					aliases(block)
				}
				for _, c := range s.Cases {
					aliases(c.Body)
				}
			}
		}
		aliases(source.Stmts)
		for _, s := range source.Stmts {
			if selected(s) {
				scanStatements([]Stmt{s})
			}
		}
		if len(needed) == before {
			break
		}
	}
	var containsCall func(*Expr) bool
	containsCall = func(e *Expr) bool {
		if e == nil {
			return false
		}
		if e == call {
			return true
		}
		if containsCall(e.Left) || containsCall(e.Right) || containsCall(e.Else) {
			return true
		}
		for _, a := range e.Args {
			if containsCall(a) {
				return true
			}
		}
		for _, a := range e.Elems {
			if containsCall(a) {
				return true
			}
		}
		return false
	}
	var filter func([]Stmt) ([]Stmt, error)
	filter = func(stmts []Stmt) ([]Stmt, error) {
		var result []Stmt
		for _, s := range stmts {
			if !selected(s) {
				continue
			}
			if containsCall(s.Expr) {
				return nil, fmt.Errorf("cyclic requested global dependency %s", s.Name)
			}
			var err error
			s.Then, err = filter(s.Then)
			if err != nil {
				return nil, err
			}
			s.Else, err = filter(s.Else)
			if err != nil {
				return nil, err
			}
			s.Body, err = filter(s.Body)
			if err != nil {
				return nil, err
			}
			s.Default, err = filter(s.Default)
			if err != nil {
				return nil, err
			}
			s.Cases = append([]SwitchCase(nil), s.Cases...)
			for i := range s.Cases {
				s.Cases[i].Body, err = filter(s.Cases[i].Body)
				if err != nil {
					return nil, err
				}
			}
			result = append(result, s)
		}
		return result, nil
	}
	var err error
	out.Stmts, err = filter(source.Stmts)
	return out, err
}

func requestMutation(expr *Expr, needed map[string]bool) bool {
	if expr == nil {
		return false
	}
	// Mutations can be nested in a declaration's initializer or arguments.
	for _, child := range []*Expr{expr.Left, expr.Right, expr.Else} {
		if requestMutation(child, needed) {
			return true
		}
	}
	for _, child := range append(append([]*Expr(nil), expr.Args...), expr.Elems...) {
		if requestMutation(child, needed) {
			return true
		}
	}
	if expr.Kind != "call" || expr.Left == nil {
		return false
	}
	name := expr.Left.Name
	root, method, ok := strings.Cut(name, ".")
	if !ok {
		return false
	}
	mutators := " push pop unshift shift insert remove clear set fill reverse sort concat put put_all add_row add_col remove_row remove_col swap_rows swap_columns reshape "
	if !strings.Contains(mutators, " "+method+" ") {
		return false
	}
	if needed[root] {
		return true
	}
	if root != "array" && root != "map" && root != "matrix" {
		return false
	}
	if len(expr.Args) == 0 {
		return false
	}
	first := expr.Args[0]
	if first.Kind == "named_arg" {
		first = first.NamedArgValue()
	}
	return first != nil && first.Kind == "ident" && needed[first.Name]
}
