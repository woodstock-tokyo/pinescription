// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

func (r *Runtime) setSeries(name string, ser SeriesExtended) {
	if strings.Contains(name, ".") || name == "" {
		return
	}
	if ser == nil {
		delete(r.namedSeries, name)
		return
	}
	r.namedSeries[name] = ser
}

func (r *Runtime) seriesForName(name string) (SeriesExtended, bool) {
	ser, ok := r.namedSeries[name]
	return ser, ok
}

func (r *Runtime) registerSeriesExpr(name string, expr *Expr) {
	if strings.Contains(name, ".") || name == "" {
		return
	}
	if expr == nil || !r.shouldTrackSeriesExpr(expr) {
		delete(r.seriesExprByName, name)
		r.setSeries(name, nil)
		return
	}
	r.seriesExprByName[name] = expr
}

func (r *Runtime) seriesExprForName(name string) (*Expr, bool) {
	expr, ok := r.seriesExprByName[name]
	return expr, ok
}

func (r *Runtime) shouldTrackSeriesExpr(expr *Expr) bool {
	if expr == nil {
		return false
	}
	if r.isLikelySeriesExpr(expr) {
		return true
	}
	if len(r.namedSeries) == 0 && len(r.seriesExprByName) == 0 {
		return false
	}
	return r.exprUsesTrackedSeries(expr)
}

func (r *Runtime) isLikelySeriesExpr(expr *Expr) bool {
	if expr == nil {
		return false
	}
	switch expr.KOp {
	case exprKindIdent:
		return isPriceIdentifierName(expr.Name)
	case exprKindCall:
		if expr.Left == nil || expr.Left.KOp != exprKindIdent {
			return false
		}
		switch expr.Left.Name {
		case "close_of", "open_of", "high_of", "low_of", "value_of":
			return true
		default:
			return false
		}
	case exprKindUnary:
		return r.isLikelySeriesExpr(expr.Right)
	case exprKindBinary:
		return r.isLikelySeriesExpr(expr.Left) || r.isLikelySeriesExpr(expr.Right)
	default:
		return false
	}
}

func (r *Runtime) exprUsesTrackedSeries(expr *Expr) bool {
	if expr == nil {
		return false
	}
	switch expr.KOp {
	case exprKindIdent:
		if _, ok := r.seriesForName(expr.Name); ok {
			return true
		}
		_, ok := r.seriesExprForName(expr.Name)
		return ok
	case exprKindUnary:
		return r.exprUsesTrackedSeries(expr.Right)
	case exprKindBinary:
		return r.exprUsesTrackedSeries(expr.Left) || r.exprUsesTrackedSeries(expr.Right)
	default:
		return false
	}
}

func (r *Runtime) evalSelfBinaryAssign(name string, expr *Expr) (bool, interface{}, error) {
	if expr == nil || expr.KOp != exprKindBinary || expr.Left == nil || expr.Right == nil {
		return false, nil, nil
	}
	if expr.Left.KOp != exprKindIdent || expr.Left.Name != name {
		return false, nil, nil
	}
	op := expr.BOp
	switch op {
	case binaryOpAdd, binaryOpSub, binaryOpMul, binaryOpDiv, binaryOpMod:
	default:
		return false, nil, nil
	}
	var right interface{}
	if expr.Right.KOp == exprKindIdent {
		if isPriceIdentifierName(expr.Right.Name) {
			right = r.currentValue(r.activeSymbol, expr.Right.Name)
		}
	}
	if right == nil {
		v, err := r.eval(expr.Right)
		if err != nil {
			return false, nil, err
		}
		right = v
	}
	left, ok := r.lookupAssignedValue(name)
	if !ok {
		return false, nil, nil
	}
	if op == binaryOpAdd {
		if ls, ok := left.(string); ok {
			return true, ls + toString(right), nil
		}
		if rs, ok := right.(string); ok {
			return true, toString(left) + rs, nil
		}
	}
	lf, lok := toFloat(left)
	rf, rok := toFloat(right)
	if !lok || !rok {
		return false, nil, nil
	}
	return true, evalBinaryArithmeticFloatByOpcode(op, lf, rf), nil
}

func (r *Runtime) findLoopBindingIndex(name string) int {
	for i := len(r.loopBindings) - 1; i >= 0; i-- {
		if r.loopBindings[i].name == name {
			return i
		}
	}
	return -1
}

func (r *Runtime) lookupLoopBinding(name string) (float64, bool) {
	idx := r.findLoopBindingIndex(name)
	if idx < 0 {
		return 0, false
	}
	return r.loopBindings[idx].value, true
}

func (r *Runtime) lookupAssignedValue(name string) (interface{}, bool) {
	if n, ok := r.lookupLoopBinding(name); ok {
		return n, true
	}
	if len(r.envStack) == 1 {
		v, ok := r.envStack[0][name]
		return v, ok
	}
	for i := len(r.envStack) - 1; i >= 0; i-- {
		if v, ok := r.envStack[i][name]; ok {
			return v, true
		}
	}
	return nil, false
}

func (r *Runtime) resolve(name string) (interface{}, error) {
	if r.evalOffset == 0 {
		if n, ok := r.lookupLoopBinding(name); ok {
			return n, nil
		}
	}
	if r.evalOffset == 0 && len(r.envStack) == 1 && !isSpecialIdentifierName(name) {
		if v, ok := r.envStack[0][name]; ok {
			return v, nil
		}
	}
	switch r.classifyIdentifier(name) {
	case identKindPrice:
		return r.currentValue(r.activeSymbol, name), nil
	case identKindMathConst:
		if v, ok := mathIdentifierValue(name); ok {
			return v, nil
		}
	case identKindSessionConst:
		if v, ok := sessionIdentifierValue(name); ok {
			return v, nil
		}
	case identKindBarIndex:
		idx := r.effectiveBarIndex()
		if idx < 0 {
			return math.NaN(), nil
		}
		return float64(idx), nil
	case identKindTime:
		if tv, ok := r.timeIdentifierValue(name); ok {
			return tv, nil
		}
	case identKindTimeframe:
		if tv, ok := r.timeframeIdentifierValue(name); ok {
			return tv, nil
		}
	case identKindDotted:
		if v, ok, err := r.resolveDottedIdentifier(name); ok || err != nil {
			return v, err
		}
	}
	if r.evalOffset == 0 {
		if len(r.envStack) == 1 {
			if v, ok := r.envStack[0][name]; ok {
				return v, nil
			}
		} else {
			for i := len(r.envStack) - 1; i >= 0; i-- {
				if v, ok := r.envStack[i][name]; ok {
					return v, nil
				}
			}
		}
	}
	if vals, ok := r.numericHistory[name]; ok && len(vals) > 0 {
		pos := len(vals) - 1
		if r.evalOffset > 0 {
			pos = len(vals) - r.evalOffset
		}
		if pos < 0 || pos >= len(vals) {
			return math.NaN(), nil
		}
		return vals[pos], nil
	}
	if vals, ok := r.history[name]; ok && len(vals) > 0 {
		pos := len(vals) - 1
		if r.evalOffset > 0 {
			pos = len(vals) - r.evalOffset
		}
		if pos < 0 || pos >= len(vals) {
			if r.declaredTypes != nil && r.declaredTypes[name] == "bool" {
				return false, nil
			}
			return math.NaN(), nil
		}
		return vals[pos], nil
	}
	if r.evalOffset == 0 {
		if len(r.envStack) == 1 {
			if v, ok := r.envStack[0][name]; ok {
				return v, nil
			}
		} else {
			for i := len(r.envStack) - 1; i >= 0; i-- {
				if v, ok := r.envStack[i][name]; ok {
					return v, nil
				}
			}
		}
	}
	if r.evalOffset > 0 {
		if r.declaredTypes != nil && r.declaredTypes[name] == "bool" {
			return false, nil
		}
		return math.NaN(), nil
	}
	return nil, fmt.Errorf("unknown identifier: %s", name)
}

func isSpecialIdentifierName(name string) bool {
	switch name {
	case "open", "high", "low", "close", "volume", "hl2", "hlc3", "hlcc4", "ohlc4",
		"bar_index",
		"time", "time_close", "timenow", "time_tradingday", "year", "month", "dayofmonth", "dayofweek", "hour", "minute", "second",
		"timeframe.period", "timeframe.main_period", "timeframe.multiplier", "timeframe.isdaily", "timeframe.isweekly", "timeframe.ismonthly", "timeframe.isdwm", "timeframe.isseconds", "timeframe.isticks", "timeframe.isminutes", "timeframe.isintraday":
		return true
	default:
		if strings.HasPrefix(name, "math.") || strings.HasPrefix(name, "session.") {
			return true
		}
		return strings.Contains(name, ".")
	}
}

func (r *Runtime) classifyIdentifier(name string) identifierKind {
	if k, ok := r.identKindCache[name]; ok {
		return k
	}
	k := identKindGeneric
	switch name {
	case "open", "high", "low", "close", "volume", "hl2", "hlc3", "hlcc4", "ohlc4":
		k = identKindPrice
	case "bar_index":
		k = identKindBarIndex
	case "time", "time_close", "timenow", "time_tradingday", "year", "month", "dayofmonth", "dayofweek", "hour", "minute", "second":
		k = identKindTime
	case "timeframe.period", "timeframe.main_period", "timeframe.multiplier", "timeframe.isdaily", "timeframe.isweekly", "timeframe.ismonthly", "timeframe.isdwm", "timeframe.isseconds", "timeframe.isticks", "timeframe.isminutes", "timeframe.isintraday":
		k = identKindTimeframe
	default:
		if strings.HasPrefix(name, "math.") {
			if _, ok := mathIdentifierValue(name); ok {
				k = identKindMathConst
			}
		} else if strings.HasPrefix(name, "session.") {
			if _, ok := sessionIdentifierValue(name); ok {
				k = identKindSessionConst
			}
		} else if strings.Contains(name, ".") {
			k = identKindDotted
		}
	}
	r.identKindCache[name] = k
	return k
}

func (r *Runtime) timeframeIdentifierValue(name string) (interface{}, bool) {
	switch name {
	case "timeframe.period", "timeframe.main_period":
		return r.timeframePeriod, true
	case "timeframe.multiplier":
		return float64(r.timeframeMult), true
	case "timeframe.isdaily":
		return r.timeframeBase == "D", true
	case "timeframe.isweekly":
		return r.timeframeBase == "W", true
	case "timeframe.ismonthly":
		return r.timeframeBase == "M", true
	case "timeframe.isdwm":
		return r.timeframeBase == "D" || r.timeframeBase == "W" || r.timeframeBase == "M", true
	case "timeframe.isseconds":
		return r.timeframeBase == "S", true
	case "timeframe.isticks":
		return false, true
	case "timeframe.isminutes", "timeframe.isintraday":
		return r.timeframeBase == "MIN", true
	default:
		return nil, false
	}
}

func (r *Runtime) assign(name string, v interface{}, isConst bool, mustExist bool) error {
	if mustExist && r.consts[name] {
		return fmt.Errorf("cannot assign const variable %s", name)
	}
	if isConst {
		r.consts[name] = true
	}
	if r.declaredTypes != nil && r.declaredTypes[name] == "bool" {
		if isNA(v) {
			return fmt.Errorf("bool value cannot be na")
		}
		if vb, ok := v.(bool); ok {
			v = vb
		} else {
			return fmt.Errorf("bool value must be bool")
		}
	}
	if strings.Contains(name, ".") {
		if err := r.assignDottedIdentifier(name, v, mustExist); err != nil {
			return err
		}
		if n, ok := toFloat(v); ok {
			r.lastValue = n
		}
		return nil
	}
	if idx := r.findLoopBindingIndex(name); idx >= 0 {
		if n, ok := toFloat(v); ok {
			r.loopBindings[idx].value = n
			r.lastValue = n
			return nil
		}
	}
	r.envStack[len(r.envStack)-1][name] = v
	if len(r.envStack) == 1 {
		if _, ok := r.rootHistorySet[name]; !ok {
			r.rootHistorySet[name] = struct{}{}
			r.rootHistoryVars = append(r.rootHistoryVars, name)
		}
	}
	if n, ok := toFloat(v); ok {
		r.lastValue = n
	}
	return nil
}

func (r *Runtime) recordHistory(name string, v interface{}) error {
	if v != nil {
		if _, ok := v.(bool); ok {
			r.historyKind[name] = historyStorageGeneric
			if _, exists := r.history[name]; !exists {
				r.history[name] = make([]interface{}, 0, r.historyCapHint(1))
			}
			r.history[name] = append(r.history[name], v)
			return nil
		}
	}

	kind := r.historyKind[name]
	if n, ok := toFloat(v); ok {
		switch kind {
		case historyStorageUnknown, historyStorageNumeric:
			r.historyKind[name] = historyStorageNumeric
			if _, exists := r.numericHistory[name]; !exists {
				r.numericHistory[name] = make([]float64, 0, r.historyCapHint(1))
			}
			r.numericHistory[name] = append(r.numericHistory[name], n)
		case historyStorageGeneric:
			if _, exists := r.history[name]; !exists {
				r.history[name] = make([]interface{}, 0, r.historyCapHint(1))
			}
			r.history[name] = append(r.history[name], v)
		}
		r.lastValue = n
		return nil
	}

	if kind == historyStorageNumeric {
		nums := r.numericHistory[name]
		generic := make([]interface{}, len(nums), r.historyCapHint(len(nums)+1))
		for i, nv := range nums {
			generic[i] = nv
		}
		r.history[name] = append(generic, v)
		delete(r.numericHistory, name)
		r.historyKind[name] = historyStorageGeneric
		return nil
	}

	r.historyKind[name] = historyStorageGeneric
	if _, exists := r.history[name]; !exists {
		r.history[name] = make([]interface{}, 0, r.historyCapHint(1))
	}
	r.history[name] = append(r.history[name], v)
	if n, ok := toFloat(v); ok {
		r.lastValue = n
	}
	return nil
}

func (r *Runtime) historyCapHint(min int) int {
	if r.expectedBars > min {
		return r.expectedBars
	}
	if min < 1 {
		return 1
	}
	return min
}

func (r *Runtime) lookupEnvValue(name string) (interface{}, bool) {
	if len(r.envStack) == 1 {
		v, ok := r.envStack[0][name]
		return v, ok
	}
	for i := len(r.envStack) - 1; i >= 0; i-- {
		if v, ok := r.envStack[i][name]; ok {
			return v, true
		}
	}
	return nil, false
}

func objectFieldGet(obj interface{}, field string) (interface{}, bool, error) {
	switch o := obj.(type) {
	case *customTypeInstance:
		if o == nil {
			return math.NaN(), true, nil
		}
		if v, ok := o.Fields[field]; ok {
			return v, true, nil
		}
		return math.NaN(), true, nil
	case map[string]interface{}:
		if v, ok := o[field]; ok {
			return v, true, nil
		}
		return math.NaN(), true, nil
	case *pineMap:
		if o == nil {
			return math.NaN(), true, nil
		}
		if o.data == nil {
			o.data = map[interface{}]interface{}{}
		}
		if v, ok := o.data[field]; ok {
			return v, true, nil
		}
		return math.NaN(), true, nil
	default:
		return nil, false, nil
	}
}

func objectFieldSet(obj interface{}, field string, value interface{}) (bool, error) {
	switch o := obj.(type) {
	case *customTypeInstance:
		if o == nil {
			return false, fmt.Errorf("cannot assign field %s on nil object", field)
		}
		o.Fields[field] = value
		return true, nil
	case map[string]interface{}:
		o[field] = value
		return true, nil
	case *pineMap:
		if o == nil {
			return false, fmt.Errorf("cannot assign field %s on nil map", field)
		}
		if o.data == nil {
			o.data = map[interface{}]interface{}{}
		}
		o.data[field] = value
		return true, nil
	default:
		return false, nil
	}
}

func (r *Runtime) resolveDottedIdentifier(name string) (interface{}, bool, error) {
	parts := strings.Split(name, ".")
	if len(parts) < 2 {
		return nil, false, nil
	}
	base := parts[0]
	cur, ok := interface{}(nil), false
	if r.rootNamespaces != nil {
		cur, ok = r.rootNamespaces[base]
	}
	if !ok {
		cur, ok = r.lookupEnvValue(base)
		if !ok {
			return nil, false, nil
		}
	}
	for i := 1; i < len(parts); i++ {
		next, handled, err := objectFieldGet(cur, parts[i])
		if err != nil {
			return nil, true, err
		}
		if !handled {
			return nil, true, fmt.Errorf("identifier %s has no field %s", strings.Join(parts[:i], "."), parts[i])
		}
		cur = next
	}
	return cur, true, nil
}

func (r *Runtime) assignDottedIdentifier(name string, v interface{}, mustExist bool) error {
	parts := strings.Split(name, ".")
	if len(parts) < 2 {
		return fmt.Errorf("invalid dotted assignment target %s", name)
	}
	baseName := parts[0]
	if r.consts[baseName] {
		return fmt.Errorf("cannot assign const variable %s", baseName)
	}
	base, ok := r.lookupEnvValue(baseName)
	if !ok {
		if mustExist {
			return fmt.Errorf("unknown variable %s", baseName)
		}
		base = &customTypeInstance{Fields: map[string]interface{}{}}
		r.envStack[len(r.envStack)-1][baseName] = base
	}
	cur := base
	for i := 1; i < len(parts)-1; i++ {
		field := parts[i]
		next, handled, err := objectFieldGet(cur, field)
		if err != nil {
			return err
		}
		if !handled {
			return fmt.Errorf("identifier %s has no field %s", strings.Join(parts[:i], "."), field)
		}
		if _, ok := next.(map[string]interface{}); !ok {
			if _, ok := next.(*customTypeInstance); !ok {
				if _, ok := next.(*pineMap); !ok {
					next = map[string]interface{}{}
					if _, err := objectFieldSet(cur, field, next); err != nil {
						return err
					}
				}
			}
		}
		cur = next
	}
	handled, err := objectFieldSet(cur, parts[len(parts)-1], v)
	if err != nil {
		return err
	}
	if !handled {
		return fmt.Errorf("identifier %s has no field %s", strings.Join(parts[:len(parts)-1], "."), parts[len(parts)-1])
	}
	return nil
}

func (r *Runtime) currentValue(symbol, valueType string) float64 {
	if symbol == r.activeSymbol && r.evalOffset == 0 {
		if valueType == "hl2" || valueType == "hlc3" || valueType == "hlcc4" || valueType == "ohlc4" {
			r.ensureActivePriceValue("open")
			r.ensureActivePriceValue("high")
			r.ensureActivePriceValue("low")
			r.ensureActivePriceValue("close")
			if v, ok := derivedPriceValueFromOHLC(valueType, r.priceCacheOpen, r.priceCacheHigh, r.priceCacheLow, r.priceCacheClose); ok {
				return v
			}
		}
		r.ensureActivePriceValue(valueType)
		switch valueType {
		case "open":
			return r.priceCacheOpen
		case "high":
			return r.priceCacheHigh
		case "low":
			return r.priceCacheLow
		case "close":
			return r.priceCacheClose
		case "volume":
			return r.priceCacheVol
		}
	}
	return r.valueAt(symbol, valueType, r.evalOffset)
}

func (r *Runtime) ensureActivePriceValue(valueType string) {
	if r.priceCacheBar != r.barIndex {
		r.priceCacheBar = r.barIndex
		r.priceCacheMask = 0
	}
	var bit uint8
	switch valueType {
	case "open":
		bit = 1 << 0
	case "high":
		bit = 1 << 1
	case "low":
		bit = 1 << 2
	case "close":
		bit = 1 << 3
	case "volume":
		bit = 1 << 4
	default:
		return
	}
	if r.priceCacheMask&bit != 0 {
		return
	}
	v := r.valueAt(r.activeSymbol, valueType, 0)
	switch valueType {
	case "open":
		r.priceCacheOpen = v
	case "high":
		r.priceCacheHigh = v
	case "low":
		r.priceCacheLow = v
	case "close":
		r.priceCacheClose = v
	case "volume":
		r.priceCacheVol = v
	}
	r.priceCacheMask |= bit
}

func (r *Runtime) valueAt(symbol, valueType string, offset int) float64 {
	if offset < 0 {
		return math.NaN()
	}
	switch valueType {
	case "hl2", "hlc3", "hlcc4", "ohlc4":
		if v, ok := derivedPriceValueFromOHLC(valueType,
			r.valueAt(symbol, "open", offset),
			r.valueAt(symbol, "high", offset),
			r.valueAt(symbol, "low", offset),
			r.valueAt(symbol, "close", offset),
		); ok {
			return v
		}
	}
	ser, err := r.getSeriesByIdentifier(symbol, valueType)
	if err != nil {
		return math.NaN()
	}
	idx := ser.Length() - 1 - r.barIndex + offset
	if idx < 0 || idx >= ser.Length() {
		return math.NaN()
	}
	return ser.Last(idx)
}

func (r *Runtime) effectiveBarIndex() int {
	return r.barIndex - r.evalOffset
}

func (r *Runtime) barTimeframeSeconds() int {
	if r.timeframeSecs <= 0 {
		return 60
	}
	return r.timeframeSecs
}

func (r *Runtime) barTimes() (time.Time, time.Time, time.Time) {
	idx := r.effectiveBarIndex()
	if idx < 0 {
		idx = 0
	}
	if r.lastTimeIndex == idx && !r.lastBarOpen.IsZero() {
		return r.lastBarOpen, r.lastBarClose, r.lastTradingDay
	}
	base := r.startTime
	if base.IsZero() {
		base = r.currentTime
	}
	if base.IsZero() {
		base = time.Now().UTC()
	}
	open := base.Add(time.Duration(idx) * r.barStep).UTC()
	close := open.Add(r.barStep)
	trading := time.Date(open.Year(), open.Month(), open.Day(), 0, 0, 0, 0, time.UTC)
	r.lastTimeIndex = idx
	r.lastBarOpen = open
	r.lastBarClose = close
	r.lastTradingDay = trading
	return open, close, trading
}

func (r *Runtime) barOpenTime() time.Time {
	open, _, _ := r.barTimes()
	return open
}

func (r *Runtime) barCloseTime() time.Time {
	_, close, _ := r.barTimes()
	return close
}

func (r *Runtime) timeIdentifierValue(name string) (interface{}, bool) {
	open, close, trading := r.barTimes()
	switch name {
	case "time":
		return float64(open.UnixMilli()), true
	case "time_close":
		return float64(close.UnixMilli()), true
	case "timenow":
		now := r.currentTime
		if now.IsZero() {
			now = time.Now().UTC()
		}
		return float64(now.UnixMilli()), true
	case "time_tradingday":
		return float64(trading.UnixMilli()), true
	case "year":
		return float64(open.Year()), true
	case "month":
		return float64(open.Month()), true
	case "dayofmonth":
		return float64(open.Day()), true
	case "dayofweek":
		return float64(open.Weekday()) + 1, true
	case "hour":
		return float64(open.Hour()), true
	case "minute":
		return float64(open.Minute()), true
	case "second":
		return float64(open.Second()), true
	default:
		return nil, false
	}
}

func (r *Runtime) builtinTime(args []interface{}) (interface{}, bool, error) {
	if len(args) > 2 {
		return nil, true, fmt.Errorf("time([timeframe], [session]) expects up to 2 args")
	}
	return float64(r.barOpenTime().UTC().UnixMilli()), true, nil
}

func (r *Runtime) builtinTimeClose(args []interface{}) (interface{}, bool, error) {
	if len(args) > 2 {
		return nil, true, fmt.Errorf("time_close([timeframe], [session]) expects up to 2 args")
	}
	return float64(r.barCloseTime().UTC().UnixMilli()), true, nil
}

func (r *Runtime) builtinTimeNow(args []interface{}) (interface{}, bool, error) {
	if len(args) != 0 {
		return nil, true, fmt.Errorf("timenow() expects 0 args")
	}
	now := r.currentTime
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return float64(now.UnixMilli()), true, nil
}

func (r *Runtime) builtinTimeTradingDay(args []interface{}) (interface{}, bool, error) {
	if len(args) != 0 {
		return nil, true, fmt.Errorf("time_tradingday expects 0 args")
	}
	_, _, trading := r.barTimes()
	return float64(trading.UnixMilli()), true, nil
}

func (r *Runtime) builtinLog(level string, args []interface{}) (interface{}, bool, error) {
	if len(args) < 1 {
		return nil, true, fmt.Errorf("log.%s expects at least 1 arg", level)
	}
	message := ""
	if len(args) == 1 {
		message = toString(args[0])
	} else if format, ok := args[0].(string); ok {
		message = formatStringTemplate(format, args[1:])
	} else {
		var b strings.Builder
		for i, a := range args {
			if i > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(toString(a))
		}
		message = b.String()
	}
	if r.logSink != nil {
		open, _, _ := r.barTimes()
		r.logSink(level, message, open)
	}
	return math.NaN(), true, nil
}

func parseTimezone(loc string) *time.Location {
	loc = strings.TrimSpace(loc)
	if loc == "" {
		return time.UTC
	}
	if strings.EqualFold(loc, "UTC") || strings.EqualFold(loc, "GMT") {
		return time.UTC
	}
	if strings.HasPrefix(strings.ToUpper(loc), "UTC") || strings.HasPrefix(strings.ToUpper(loc), "GMT") {
		off := strings.TrimSpace(loc[3:])
		if off == "" {
			return time.UTC
		}
		sign := 1
		if off[0] == '-' {
			sign = -1
			off = off[1:]
		} else if off[0] == '+' {
			off = off[1:]
		}
		parts := strings.Split(off, ":")
		h, _ := strconv.Atoi(parts[0])
		m := 0
		if len(parts) > 1 {
			m, _ = strconv.Atoi(parts[1])
		}
		return time.FixedZone(loc, sign*(h*3600+m*60))
	}
	zone, err := time.LoadLocation(loc)
	if err != nil {
		return time.UTC
	}
	return zone
}

func (r *Runtime) builtinTimestamp(args []interface{}) (interface{}, bool, error) {
	if len(args) < 3 {
		return nil, true, fmt.Errorf("timestamp expects at least 3 args")
	}
	loc := time.UTC
	pos := 0
	if s, ok := args[0].(string); ok {
		loc = parseTimezone(s)
		pos = 1
	}
	if len(args)-pos < 3 || len(args)-pos > 6 {
		return nil, true, fmt.Errorf("timestamp expects [timezone,] year, month, day[, hour[, minute[, second]]]")
	}
	vals := []int{0, 0, 0, 0, 0, 0}
	for i := 0; i < len(args)-pos; i++ {
		f, _ := toFloat(args[pos+i])
		vals[i] = int(f)
	}
	ts := time.Date(vals[0], time.Month(vals[1]), vals[2], vals[3], vals[4], vals[5], 0, loc)
	return float64(ts.UTC().UnixMilli()), true, nil
}
