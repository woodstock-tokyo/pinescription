// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"errors"
	"fmt"
	"sort"
)

func isPriceIdentifierName(name string) bool {
	switch name {
	case "open", "high", "low", "close", "volume", "hl2", "hlc3", "hlcc4", "ohlc4":
		return true
	default:
		return false
	}
}

func derivedPriceValueFromOHLC(valueType string, open, high, low, close float64) (float64, bool) {
	switch valueType {
	case "hl2":
		return (high + low) / 2.0, true
	case "hlc3":
		return (high + low + close) / 3.0, true
	case "hlcc4":
		return (high + low + (2.0 * close)) / 4.0, true
	case "ohlc4":
		return (open + high + low + close) / 4.0, true
	default:
		return 0, false
	}
}

func (r *Runtime) getSeriesByIdentifier(symbol, valueType string) (SeriesExtended, error) {
	if symbol == "" || valueType == "" {
		return nil, errors.New("symbol and value_type are required")
	}
	if !isPriceIdentifierName(valueType) {
		return r.getSeries(symbol, valueType)
	}
	if valueType == "open" || valueType == "high" || valueType == "low" || valueType == "close" || valueType == "volume" {
		return r.getSeries(symbol, valueType)
	}
	key := r.cachedSeriesKey(symbol, valueType)
	if ser, ok := r.seriesByKey[key]; ok && ser != nil {
		return ser, nil
	}
	openSer, err := r.getSeries(symbol, "open")
	if err != nil {
		return nil, err
	}
	highSer, err := r.getSeries(symbol, "high")
	if err != nil {
		return nil, err
	}
	lowSer, err := r.getSeries(symbol, "low")
	if err != nil {
		return nil, err
	}
	closeSer, err := r.getSeries(symbol, "close")
	if err != nil {
		return nil, err
	}
	var ser SeriesExtended
	switch valueType {
	case "hl2":
		ser = highSer.Add(lowSer).Div(2.0)
	case "hlc3":
		ser = highSer.Add(lowSer).Add(closeSer).Div(3.0)
	case "hlcc4":
		ser = highSer.Add(lowSer).Add(closeSer.Mul(2.0)).Div(4.0)
	case "ohlc4":
		ser = openSer.Add(highSer).Add(lowSer).Add(closeSer).Div(4.0)
	default:
		return nil, fmt.Errorf("unknown price identifier: %s", valueType)
	}
	r.seriesByKey[key] = ser
	if _, ok := r.valueTypesBySymbol[symbol]; !ok {
		r.valueTypesBySymbol[symbol] = map[string]bool{}
	}
	r.valueTypesBySymbol[symbol][valueType] = true
	return ser, nil
}

// Snapshot returns a snapshot of the current runtime state, including the active
// bar index, last numeric result, the active symbol and value type, all available
// symbols and series keys, and a copy of the top-level variable map.
//
// Example:
//
//	rt, result, _ := engine.ExecuteWithRuntime(bytecode)
//	snap := rt.Snapshot()
//	fmt.Println("bar_index:", snap.BarIndex, "result:", snap.LastValue)
func (r *Runtime) Snapshot() RuntimeSnapshot {
	vars := map[string]interface{}{}
	for _, scope := range r.envStack {
		for k, v := range scope {
			vars[k] = v
		}
	}
	return RuntimeSnapshot{
		BarIndex:        r.barIndex,
		LastValue:       r.lastValue,
		ActiveSymbol:    r.activeSymbol,
		ActiveValueType: r.activeValueType,
		Symbols:         r.Symbols(),
		SeriesKeys:      r.SeriesKeys(),
		Variables:       vars,
	}
}

// Symbols returns the sorted list of all symbols referenced during execution.
// This includes the active symbol, any symbol explicitly used in Pine Script
// calls like close_of, and symbols discovered from multi-symbol indicators.
func (r *Runtime) Symbols() []string {
	seen := map[string]bool{}
	for symbol := range r.valueTypesBySymbol {
		seen[symbol] = true
	}
	for key := range r.seriesByKey {
		symbol, _, ok := splitSeriesKey(key)
		if ok {
			seen[symbol] = true
		}
	}
	if r.activeSymbol != "" {
		seen[r.activeSymbol] = true
	}
	out := make([]string, 0, len(seen))
	for symbol := range seen {
		out = append(out, symbol)
	}
	sort.Strings(out)
	return out
}

// SeriesKeys returns the sorted list of all series keys in the format "symbol|valueType"
// that were loaded or derived during execution.
func (r *Runtime) SeriesKeys() []string {
	out := make([]string, 0, len(r.seriesByKey))
	for key := range r.seriesByKey {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// ValueTypes returns the sorted list of value types available for the given symbol,
// such as "close", "high", "volume". Returns nil if the symbol was not used.
func (r *Runtime) ValueTypes(symbol string) []string {
	set, ok := r.valueTypesBySymbol[symbol]
	if !ok {
		return nil
	}
	out := make([]string, 0, len(set))
	for valueType := range set {
		out = append(out, valueType)
	}
	sort.Strings(out)
	return out
}

// Series returns the SeriesExtended for the given series key (e.g. "AAPL|close")
// and a boolean indicating whether it was found. For price-derived value types
// (hl2, hlc3, ohlc4), this lazily computes the series from the base OHLC data.
func (r *Runtime) Series(seriesKey string) (SeriesExtended, bool) {
	if seriesKey == "" {
		return nil, false
	}
	if ser, ok := r.seriesByKey[seriesKey]; ok {
		return ser, true
	}
	symbol, valueType, ok := splitSeriesKey(seriesKey)
	if !ok {
		return nil, false
	}
	ser, err := r.getSeries(symbol, valueType)
	if err != nil {
		return nil, false
	}
	return ser, true
}

// Value returns the most recent value of the named variable in the execution scope,
// and a boolean indicating whether the variable exists. For history-tracked variables
// it returns the value at the last bar; for function parameters it returns the
// current stack value.
func (r *Runtime) Value(name string) (interface{}, bool) {
	for i := len(r.envStack) - 1; i >= 0; i-- {
		if v, ok := r.envStack[i][name]; ok {
			return v, true
		}
	}
	if vals, ok := r.numericHistory[name]; ok && len(vals) > 0 {
		return vals[len(vals)-1], true
	}
	vals, ok := r.history[name]
	if !ok || len(vals) == 0 {
		return nil, false
	}
	return vals[len(vals)-1], true
}

func (r *Runtime) getSeries(symbol, valueType string) (SeriesExtended, error) {
	if symbol == "" || valueType == "" {
		return nil, errors.New("symbol and value_type are required")
	}
	candidates := []string{valueType}
	defaultVT := r.defaultValueTypeForSymbol(symbol)
	if defaultVT != "" && defaultVT != valueType {
		candidates = append(candidates, defaultVT)
	}

	var lastErr error
	for _, vt := range candidates {
		key := r.cachedSeriesKey(symbol, vt)
		if ser, ok := r.seriesByKey[key]; ok && ser != nil {
			return ser, nil
		}
		if r.loadSeries == nil {
			continue
		}
		ser, err := r.loadSeries(symbol, vt)
		if err != nil {
			lastErr = err
			continue
		}
		if ser == nil || ser.Length() == 0 {
			continue
		}
		r.seriesByKey[key] = ser
		if _, ok := r.valueTypesBySymbol[symbol]; !ok {
			r.valueTypesBySymbol[symbol] = map[string]bool{}
		}
		r.valueTypesBySymbol[symbol][vt] = true
		return ser, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("series for %q with value_type %q is unavailable", symbol, valueType)
}

func (r *Runtime) defaultValueTypeForSymbol(symbol string) string {
	if symbol == r.activeSymbol && r.activeValueType != "" {
		return r.activeValueType
	}
	set := r.valueTypesBySymbol[symbol]
	if len(set) == 0 {
		return ""
	}
	if set["close"] {
		return "close"
	}
	vts := make([]string, 0, len(set))
	for vt := range set {
		vts = append(vts, vt)
	}
	sort.Strings(vts)
	return vts[0]
}
