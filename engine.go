// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	series "github.com/woodstock-tokyo/pinescription/series"
)

var arrayNewGenericRe = regexp.MustCompile(`array\.new\s*<\s*(int|float|bool|string)\s*>`)

func normalizePineScriptCompat(source string) string {
	return arrayNewGenericRe.ReplaceAllStringFunc(source, func(m string) string {
		parts := arrayNewGenericRe.FindStringSubmatch(m)
		if len(parts) != 2 {
			return m
		}
		return "array.new_" + strings.ToLower(parts[1])
	})
}

// SeriesExtended is an alias for the series extension interface defined in the
// woodstock-utils series package. It represents a time series of float values
// with variable-length lookback, used to supply OHLCV and derived data to the
// runtime. The interface includes Length, Last, and arithmetic methods.
type SeriesExtended = series.SeriesExtend

// Provider is the interface that market data backends must implement to supply
// time-series data to the engine. A provider is registered on an Engine with
// RegisterMarketDataProvider. The same provider instance can serve multiple
// symbols and value types, but each distinct symbol/value-type pair is keyed
// as "symbol|valueType" in calls to GetSeries.
//
// Example provider implementation skeleton:
//
//	type myProvider struct{}
//
//	func (p *myProvider) GetSeries(key string) (pinego.SeriesExtended, error) {
//	    // key is "SYMBOL|valueType", e.g. "AAPL|close"
//	    symbol, vt, _ := strings.Cut(key, "|")
//	    return myLoadSeries(symbol, vt)
//	}
//	func (p *myProvider) GetSymbols() ([]string, error) { return []string{"AAPL"}, nil }
//	func (p *myProvider) GetValuesTypes() ([]string, error) { return []string{"close"}, nil }
//	func (p *myProvider) SetTimeframe(tf string) error { return nil }
//	func (p *myProvider) GetTimeframe() string { return "1D" }
//	func (p *myProvider) SetSession(s string) error { return nil }
//	func (p *myProvider) GetSession() string { return "" }
type Provider interface {
	// GetSeries returns the time series identified by seriesKey, which is
	// formatted as "symbol|valueType" (e.g. "AAPL|close"). Returns an error
	// if the series is unavailable. The series must contain at least one data point.
	GetSeries(seriesKey string) (SeriesExtended, error)
	// GetSymbols returns the list of ticker symbols this provider can serve.
	GetSymbols() ([]string, error)
	// GetValuesTypes returns the list of value types available for each symbol.
	// Common types include "open", "high", "low", "close", "volume", and derived
	// types such as "hl2", "hlc3", "ohlc4".
	GetValuesTypes() ([]string, error)
	// SetTimeframe sets the bar timeframe for subsequent GetSeries calls.
	// Common values are "1m", "5m", "1h", "1D". Providers may ignore this
	// if they only serve a single timeframe.
	SetTimeframe(timeframe string) error
	// GetTimeframe returns the current bar timeframe string.
	GetTimeframe() string
	// SetSession sets the trading session for time-based filtering.
	SetSession(session string) error
	// GetSession returns the current trading session string.
	GetSession() string
}

// UserFunction is the signature for functions registered via RegisterFunction.
// The function receives the evaluated Pine Script argument values and returns
// a result (float, string, bool, or nil) and an error. Returning a non-nil error
// aborts script execution with that error.
type UserFunction func(args ...interface{}) (interface{}, error)

// EngineLogEntry represents a single log entry produced during script execution.
// Level is one of "info", "warning", or "error". Timestamp is the wall-clock
// time at which the entry was recorded (UTC). Message contains the formatted
// log text.
type EngineLogEntry struct {
	Timestamp time.Time
	Level     string
	Message   string
}

// Engine is the Pine Script compiler and runtime. It holds all configuration
// for a single execution context: registered providers, user functions, the
// active symbol and value type, and timing parameters. An Engine may be
// reused across multiple Compile/Execute cycles, but is not safe for concurrent
// use by multiple goroutines without external synchronization.
//
// Use NewEngine to create an instance, then configure providers and options
// before calling Compile and Execute.
type Engine struct {
	providers        []Provider
	functions        map[string]UserFunction
	functionParams   map[string]callParamSpec
	defaultSymbol    string
	defaultValueType string
	timeframe        string
	session          string
	currentTime      time.Time
	startTime        time.Time
	lastRuntime      *Runtime
	logs             []EngineLogEntry
	alertSink        func(AlertEvent)
	cachedBytecode   []byte
	cachedProgram    Program
	cacheValid       bool
}

// AlertEvent describes an alert triggered by a Pine Script alert() call during execution.
// BarIndex is the zero-based index of the bar on which the alert fired. Time is the
// UTC close time of that bar. Symbol is the active ticker at the time of firing.
type AlertEvent struct {
	Message   string
	Frequency string
	BarIndex  int
	Time      time.Time
	Symbol    string
}

type providerCatalog struct {
	providerBySymbol   map[string]Provider
	valueTypesBySymbol map[string]map[string]bool
	orderedSymbols     []string
}

// NewEngine returns a new, empty Engine configured with no providers or user functions.
// Call RegisterMarketDataProvider to add a data source, then RegisterFunction for any
// custom functions before calling Compile and Execute.
//
// Example:
//
//	engine := pinescription.NewEngine()
//	engine.RegisterMarketDataProvider(myProvider)
func NewEngine() *Engine {
	return &Engine{functions: map[string]UserFunction{}, functionParams: map[string]callParamSpec{}}
}

// RegisterFunction registers a user-defined function callable from Pine Script.
// The function's name in Pine Script is the name string provided here. Functions
// are invoked with positional arguments evaluated according to Pine Script rules.
// A function with the same name replaces any previously registered function.
func (e *Engine) RegisterFunction(name string, function UserFunction) {
	if e.functions == nil {
		e.functions = map[string]UserFunction{}
	}
	e.functions[name] = function
	delete(e.functionParams, name)
}

// RegisterFunctionWithParamNames registers an ordinary custom function or an
// exact unsupported feature hook with parameter names used to bind Pine Script
// named arguments. Positional calls are still passed through in source order. A
// function with the same name replaces any previously registered function and
// parameter metadata.
//
// It returns an error when name is empty, parser-reserved, a Pine type keyword
// that is not an unsupported hook target, or already handled by the built-in
// runtime dispatcher. It also returns an error when paramNames contains empty or
// duplicate names.
func (e *Engine) RegisterFunctionWithParamNames(name string, paramNames []string, function UserFunction) error {
	if err := validateRegisteredFunctionName(name); err != nil {
		return err
	}
	if err := validateRegisteredFunctionParamNames(name, paramNames); err != nil {
		return err
	}
	if e.functions == nil {
		e.functions = map[string]UserFunction{}
	}
	if e.functionParams == nil {
		e.functionParams = map[string]callParamSpec{}
	}
	e.functions[name] = function
	e.functionParams[name] = callParamSpec{Names: append([]string(nil), paramNames...)}
	return nil
}

func validateRegisteredFunctionParamNames(functionName string, paramNames []string) error {
	seen := make(map[string]struct{}, len(paramNames))
	for i, paramName := range paramNames {
		if paramName == "" {
			return fmt.Errorf("registered function %q parameter name at index %d must not be empty", functionName, i)
		}
		if _, ok := seen[paramName]; ok {
			return fmt.Errorf("registered function %q parameter name %q is duplicated", functionName, paramName)
		}
		seen[paramName] = struct{}{}
	}
	return nil
}

func validateRegisteredFunctionName(name string) error {
	if name == "" {
		return errors.New("registered function name must not be empty")
	}
	if isReservedPineKeyword(name) || (isTypeKeyword(name) && !isUnsupportedFeatureCallName(name)) {
		return fmt.Errorf("registered function name %q is reserved", name)
	}
	if isImplementedBuiltinFunctionName(name) {
		return fmt.Errorf("registered function name %q conflicts with a built-in function", name)
	}
	return nil
}

func isReservedPineKeyword(name string) bool {
	switch name {
	case "if", "else", "while", "for", "switch", "break", "continue", "return", "var", "const", "varip", "simple", "series", "input", "type", "enum", "import", "export", "do", "as", "in", "method", "function", "by":
		return true
	default:
		return false
	}
}

func isImplementedBuiltinFunctionName(name string) bool {
	if _, ok := builtinCallParamSpecs[name]; ok {
		return true
	}
	if builtinFastID(name) != builtinFastUnknown {
		return true
	}
	_, ok := implementedBuiltinFunctionNames[name]
	return ok
}

var implementedBuiltinFunctionNames = map[string]struct{}{
	"int": {}, "float": {}, "bool": {}, "string": {}, "str.tostring": {},
	"input.time": {}, "input.session": {}, "input.symbol": {},
	"alertcondition": {}, "color": {}, "color.rgb": {},
	"box.get_bottom": {}, "box.get_top": {}, "box.set_right": {}, "box.delete": {},
	"linefill.new": {}, "line.new": {}, "label.new": {}, "line.set_xy1": {}, "line.set_xy2": {}, "line.set_color": {},
	"label.set_xy": {}, "label.set_text": {}, "label.set_tooltip": {},
	"log.info": {}, "log.warning": {}, "log.error": {},
	"map.new": {}, "map.clear": {}, "map.copy": {}, "map.size": {}, "map.put": {}, "map.get": {}, "map.contains": {}, "map.remove": {}, "map.keys": {}, "map.values": {},
	"array.new_int": {}, "array.new_float": {}, "array.new_bool": {}, "array.new_string": {}, "array.new_box": {},
	"array.size": {}, "array.get": {}, "array.set": {}, "array.push": {}, "array.pop": {}, "array.unshift": {}, "array.shift": {}, "array.clear": {}, "array.remove": {}, "array.concat": {}, "array.slice": {}, "array.includes": {}, "array.indexof": {}, "array.lastindexof": {}, "array.copy": {}, "array.from": {}, "array.insert": {}, "array.first": {}, "array.last": {}, "array.join": {}, "array.every": {}, "array.abs": {}, "array.sum": {}, "array.avg": {}, "array.max": {}, "array.min": {}, "array.range": {}, "array.median": {}, "array.mode": {}, "array.percentrank": {}, "array.percentile_linear_interpolation": {}, "array.percentile_nearest_rank": {}, "array.percentile_neareast_rank": {}, "array.binary_search_leftmost": {}, "array.binary_search_rightmost": {}, "array.covariance": {},
	"str.length": {}, "str.upper": {}, "str.lower": {}, "str.contains": {}, "str.startswith": {}, "str.endswith": {}, "str.replace": {}, "str.substring": {}, "str.split": {}, "str.format": {},
	"na": {}, "fixnan": {},
	"math.abs": {}, "abs": {}, "math.round": {}, "round": {}, "math.floor": {}, "floor": {}, "math.ceil": {}, "ceil": {}, "math.pow": {}, "pow": {}, "math.sqrt": {}, "sqrt": {}, "math.log10": {}, "log10": {}, "math.avg": {}, "avg": {}, "math.sum": {}, "sum": {}, "math.exp": {}, "exp": {}, "math.sin": {}, "sin": {}, "math.cos": {}, "cos": {}, "math.tan": {}, "tan": {}, "math.acos": {}, "acos": {}, "math.asin": {}, "asin": {}, "math.atan": {}, "atan": {}, "math.sign": {}, "sign": {}, "math.todegrees": {}, "todegrees": {}, "math.toradians": {}, "toradians": {}, "math.random": {},
	"timeframe.change": {}, "timeframe.in_seconds": {}, "timeframe.from_seconds": {}, "time": {}, "time_close": {}, "timenow": {}, "time_tradingday": {}, "timestamp": {},
	"value_of": {}, "close_of": {}, "open_of": {}, "high_of": {}, "low_of": {},
	"atr": {}, "ta.atr": {}, "change": {}, "ta.change": {}, "stdev": {}, "ta.stdev": {}, "correlation": {}, "ta.correlation": {}, "sma": {}, "ta.sma": {}, "ema": {}, "ta.ema": {}, "rsi": {}, "ta.rsi": {}, "crossover": {}, "ta.crossover": {}, "crossunder": {}, "ta.crossunder": {}, "cross": {}, "ta.cross": {}, "rma": {}, "ta.rma": {}, "wma": {}, "ta.wma": {}, "swma": {}, "ta.swma": {}, "hma": {}, "ta.hma": {}, "alma": {}, "ta.alma": {}, "linreg": {}, "ta.linreg": {}, "vwma": {}, "ta.vwma": {}, "cci": {}, "ta.cci": {}, "cmo": {}, "ta.cmo": {}, "cog": {}, "ta.cog": {}, "macd": {}, "ta.macd": {}, "mom": {}, "ta.mom": {}, "roc": {}, "ta.roc": {}, "barssince": {}, "ta.barssince": {}, "cum": {}, "ta.cum": {}, "valuewhen": {}, "ta.valuewhen": {}, "highestbars": {}, "ta.highestbars": {}, "lowestbars": {}, "ta.lowestbars": {}, "ta.max": {}, "ta.min": {}, "ta.median": {}, "ta.mode": {}, "ta.percentile_linear_interpolation": {}, "ta.percentile_nearest_rank": {}, "ta.percentrank": {}, "ta.range": {}, "ta.variance": {}, "ta.dev": {}, "ta.rising": {}, "ta.falling": {}, "tr": {}, "ta.tr": {}, "ta.pivothigh": {}, "ta.pivotlow": {}, "ta.pivot_point_levels": {}, "bb": {}, "ta.bb": {}, "bbw": {}, "ta.bbw": {}, "kc": {}, "ta.kc": {}, "kcw": {}, "ta.kcw": {}, "stoch": {}, "ta.stoch": {}, "mfi": {}, "ta.mfi": {}, "tsi": {}, "ta.tsi": {}, "wpr": {}, "ta.wpr": {}, "dmi": {}, "ta.dmi": {}, "sar": {}, "ta.sar": {}, "supertrend": {}, "ta.supertrend": {}, "sma_of": {}, "ema_of": {}, "rsi_of": {},
}

// RegisterMarketDataProvider adds a market data provider to the engine.
// Providers are queried in the order they were registered when fetching
// symbol and value-type data. At least one provider must be registered
// before Execute is called. If multiple providers are registered, the first
// provider's timeframe and session values are used as defaults unless
// overridden by SetTimeframe or SetSession.
func (e *Engine) RegisterMarketDataProvider(provider Provider) {
	e.providers = append(e.providers, provider)
	if e.timeframe == "" {
		e.timeframe = provider.GetTimeframe()
	}
	if e.session == "" {
		e.session = provider.GetSession()
	}
}

// SetDefaultSymbol sets the symbol used as the default source of OHLCV data when
// Pine Script references price identifiers like close, open, or high without an
// explicit symbol prefix. If not set, the first symbol from the first registered
// provider is used.
func (e *Engine) SetDefaultSymbol(symbol string) {
	e.defaultSymbol = symbol
}

// SetDefaultValueType sets the default value type used when Pine Script references
// a price identifier without an explicit value type suffix. Common values are
// "close", "open", "high", "low", "volume". If not set, "close" is preferred
// if available, otherwise the first value type from the provider.
func (e *Engine) SetDefaultValueType(valueType string) {
	e.defaultValueType = valueType
}

// SetTimeframe sets the bar timeframe for all registered providers.
// Common values include "1m", "5m", "1h", "4h", "1D". This overrides any
// timeframe returned by individual providers.
func (e *Engine) SetTimeframe(timeframe string) {
	e.timeframe = timeframe
}

// Timeframe returns the currently configured bar timeframe string.
func (e *Engine) Timeframe() string {
	return e.timeframe
}

// SetSession sets the trading session used for time-based filtering in the runtime.
func (e *Engine) SetSession(session string) {
	e.session = session
}

// Session returns the currently configured trading session string.
func (e *Engine) Session() string {
	return e.session
}

// SetCurrentTime sets the wall-clock time used as the current moment during
// script execution. If not set, time.Now().UTC() is used. This is useful for
// deterministic testing or for replaying historical data at a known timestamp.
func (e *Engine) SetCurrentTime(now time.Time) {
	e.currentTime = now
}

// CurrentTime returns the currently configured current time.
func (e *Engine) CurrentTime() time.Time {
	return e.currentTime
}

// SetStartTime sets the time of the first bar in the dataset. When combined
// with the bar timeframe, this determines the timestamp of every bar in the
// series. If not set, it is inferred from CurrentTime and the number of bars.
func (e *Engine) SetStartTime(start time.Time) {
	e.startTime = start
}

// StartTime returns the currently configured start time.
func (e *Engine) StartTime() time.Time {
	return e.startTime
}

// Logs returns a copy of all log entries produced during the last execution.
// Each entry includes the UTC timestamp, level, and formatted message.
func (e *Engine) Logs() []EngineLogEntry {
	out := make([]EngineLogEntry, len(e.logs))
	copy(out, e.logs)
	return out
}

// ClearLogs removes all accumulated log entries from the engine.
func (e *Engine) ClearLogs() {
	e.logs = nil
}

func (e *Engine) appendLog(level, message string, ts time.Time) {
	if ts.IsZero() {
		ts = e.currentTime
	}
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	e.logs = append(e.logs, EngineLogEntry{Timestamp: ts.UTC(), Level: level, Message: message})
}

// Runtime returns the Runtime instance produced by the most recent Execute call,
// or nil if Execute has not been called or ClearRuntime was called.
// The returned Runtime is retained by the Engine and released on the next Execute
// or on ClearRuntime. Call Snapshot on the returned Runtime to inspect variables
// and series after execution.
func (e *Engine) Runtime() *Runtime {
	return e.lastRuntime
}

// ClearRuntime releases the Runtime from the last Execute call and clears the
// bytecode cache. Call this to force a full recompilation on the next Compile
// or to free memory between unrelated executions.
func (e *Engine) ClearRuntime() {
	if e.lastRuntime != nil {
		e.lastRuntime.Release()
	}
	e.lastRuntime = nil
}

// SetAlertSink installs a callback invoked each time a Pine Script alert() call
// executes. The callback receives an AlertEvent describing the alert. SetAlertSink
// is optional; if not set, alerts are silently dropped.
func (e *Engine) SetAlertSink(sink func(AlertEvent)) {
	e.alertSink = sink
}

// Symbols returns the sorted list of all ticker symbols available from the
// registered market data providers. Returns an error if no provider is registered.
func (e *Engine) Symbols() ([]string, error) {
	if len(e.providers) == 0 {
		return nil, errors.New("market data provider is not registered")
	}
	seen := map[string]bool{}
	out := make([]string, 0)
	for _, p := range e.providers {
		syms, err := p.GetSymbols()
		if err != nil {
			return nil, fmt.Errorf("load symbols: %w", err)
		}
		for _, s := range syms {
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

// ValueTypes returns the sorted list of all value types available across all registered
// providers. Common values include "close", "open", "high", "low", "volume", as well
// as derived types such as "hl2", "hlc3", "ohlc4". Returns an error if no provider
// is registered.
func (e *Engine) ValueTypes() ([]string, error) {
	if len(e.providers) == 0 {
		return nil, errors.New("market data provider is not registered")
	}
	seen := map[string]bool{}
	out := make([]string, 0)
	for _, p := range e.providers {
		vts, err := p.GetValuesTypes()
		if err != nil {
			return nil, fmt.Errorf("load value_types: %w", err)
		}
		for _, vt := range vts {
			if vt == "" || seen[vt] {
				continue
			}
			seen[vt] = true
			out = append(out, vt)
		}
	}
	sort.Strings(out)
	return out, nil
}

func buildProviderCatalog(providers []Provider) (*providerCatalog, error) {
	catalog := &providerCatalog{
		providerBySymbol:   map[string]Provider{},
		valueTypesBySymbol: map[string]map[string]bool{},
		orderedSymbols:     make([]string, 0),
	}

	for _, p := range providers {
		syms, err := p.GetSymbols()
		if err != nil {
			return nil, fmt.Errorf("load symbols: %w", err)
		}
		vts, err := p.GetValuesTypes()
		if err != nil {
			return nil, fmt.Errorf("load value_types: %w", err)
		}
		if len(vts) == 0 {
			return nil, errors.New("provider returned no value_types")
		}

		vtSet := map[string]bool{}
		for _, vt := range vts {
			if vt != "" {
				vtSet[vt] = true
			}
		}
		if len(vtSet) == 0 {
			return nil, errors.New("provider returned empty value_types")
		}

		for _, s := range syms {
			if _, exists := catalog.providerBySymbol[s]; exists {
				continue
			}
			catalog.providerBySymbol[s] = p
			catalog.valueTypesBySymbol[s] = cloneBoolSet(vtSet)
			catalog.orderedSymbols = append(catalog.orderedSymbols, s)
		}
	}

	if len(catalog.providerBySymbol) == 0 {
		return nil, errors.New("providers returned no symbols")
	}
	return catalog, nil
}

func (c *providerCatalog) fetchSeries(symbol, valueType string) (SeriesExtended, error) {
	p, ok := c.providerBySymbol[symbol]
	if !ok {
		return nil, fmt.Errorf("symbol %q is not available in registered providers", symbol)
	}
	if !c.valueTypesBySymbol[symbol][valueType] {
		return nil, fmt.Errorf("value_type %q is not available for symbol %q", valueType, symbol)
	}
	key := makeSeriesKey(symbol, valueType)
	ser, err := p.GetSeries(key)
	if err != nil {
		return nil, fmt.Errorf("get series %q: %w", key, err)
	}
	if ser == nil || ser.Length() == 0 {
		return nil, fmt.Errorf("series for %q is empty", key)
	}
	return ser, nil
}

func (e *Engine) resolveActiveSelection(c *providerCatalog) (string, string, error) {
	activeSymbol := e.defaultSymbol
	if activeSymbol == "" {
		activeSymbol = c.orderedSymbols[0]
	}
	if _, ok := c.providerBySymbol[activeSymbol]; !ok {
		return "", "", fmt.Errorf("default symbol %q is not available in registered providers", activeSymbol)
	}

	activeValueType := e.defaultValueType
	if activeValueType == "" {
		activeValueType = chooseDefaultValueType(c.valueTypesBySymbol[activeSymbol])
	}
	if activeValueType == "" {
		return "", "", fmt.Errorf("no usable value_type for symbol %q", activeSymbol)
	}
	if !c.valueTypesBySymbol[activeSymbol][activeValueType] {
		return "", "", fmt.Errorf("default value_type %q is not available for symbol %q", activeValueType, activeSymbol)
	}

	return activeSymbol, activeValueType, nil
}

func copyUserFunctions(in map[string]UserFunction) map[string]UserFunction {
	out := map[string]UserFunction{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyCallParamSpecs(in map[string]callParamSpec) map[string]callParamSpec {
	if len(in) == 0 {
		return nil
	}
	out := map[string]callParamSpec{}
	for k, v := range in {
		out[k] = callParamSpec{Names: append([]string(nil), v.Names...), Required: v.Required}
	}
	return out
}

func collectNeededSeriesKeys(program Program, activeSymbol, activeValueType string) map[string]struct{} {
	needed := map[string]struct{}{}
	needed[makeSeriesKey(activeSymbol, activeValueType)] = struct{}{}
	for _, vt := range program.ValueTypes {
		if vt != "" {
			needed[makeSeriesKey(activeSymbol, vt)] = struct{}{}
		}
	}
	for _, s := range program.Symbols {
		if s != "" {
			needed[makeSeriesKey(s, activeValueType)] = struct{}{}
		}
	}
	for _, key := range program.SeriesKeys {
		if key != "" {
			needed[key] = struct{}{}
		}
	}
	return needed
}

var defaultEngine = NewEngine()

// RegisterFunction is a convenience wrapper around defaultEngine.RegisterFunction.
// It registers a user-defined function on the package-level default Engine.
// This Engine is shared globally, so registration is permanent for the process
// lifetime. Prefer creating a private Engine with NewEngine when registering
// functions that should be isolated between different uses.
func RegisterFunction(name string, function func(args ...interface{}) (interface{}, error)) {
	defaultEngine.RegisterFunction(name, function)
}

// RegisterFunctionWithParamNames is a convenience wrapper around
// defaultEngine.RegisterFunctionWithParamNames. Use it for ordinary custom
// functions or exact unsupported feature hooks that may receive Pine Script
// named arguments.
func RegisterFunctionWithParamNames(name string, paramNames []string, function func(args ...interface{}) (interface{}, error)) error {
	return defaultEngine.RegisterFunctionWithParamNames(name, paramNames, function)
}

// RegisterMarketDataProvider is a convenience wrapper around defaultEngine.RegisterMarketDataProvider.
// It registers a provider on the package-level default Engine. See RegisterFunction for
// caveats about the shared global Engine.
func RegisterMarketDataProvider(provider Provider) {
	defaultEngine.RegisterMarketDataProvider(provider)
}

// SetTimeframe is a convenience wrapper around defaultEngine.SetTimeframe.
func SetTimeframe(timeframe string) {
	defaultEngine.SetTimeframe(timeframe)
}

// SetSession is a convenience wrapper around defaultEngine.SetSession.
func SetSession(session string) {
	defaultEngine.SetSession(session)
}

// SetCurrentTime is a convenience wrapper around defaultEngine.SetCurrentTime.
func SetCurrentTime(now time.Time) {
	defaultEngine.SetCurrentTime(now)
}

// SetStartTime is a convenience wrapper around defaultEngine.SetStartTime.
func SetStartTime(start time.Time) {
	defaultEngine.SetStartTime(start)
}

// Logs is a convenience wrapper around defaultEngine.Logs.
func Logs() []EngineLogEntry {
	return defaultEngine.Logs()
}

// ClearLogs is a convenience wrapper around defaultEngine.ClearLogs.
func ClearLogs() {
	defaultEngine.ClearLogs()
}

// Compile compiles the given Pine Script source code to bytecode using the
// package-level default Engine. On success the bytecode can be passed to
// Execute. Compilation errors include the source location of the syntax error.
//
// Use the Engine method directly if you need to control which Engine instance
// is used, or to access the Runtime after execution.
func Compile(pinescript string) ([]byte, error) {
	return defaultEngine.Compile(pinescript)
}

// Execute runs pre-compiled bytecode against the registered market data providers
// using the package-level default Engine and returns the result. The result is
// the value of the final expression in the script, or nil if the script produces
// NaN as its final value. Returns an error if the bytecode is invalid, a
// provider fails, or a runtime error occurs during execution.
func Execute(bytecode []byte) (interface{}, error) {
	return defaultEngine.Execute(bytecode)
}

// Compile compiles Pine Script source to bytecode. The result is cached internally
// so that repeated calls with the same source do not re-parse. Compilation
// validates the AST and applies lowering passes before encoding.
//
// Returns an error describing the parse failure with source location if the
// script has syntax errors.
func (e *Engine) Compile(pinescript string) ([]byte, error) {
	pinescript = normalizePineScriptCompat(pinescript)
	program, err := parseProgram(pinescript)
	if err != nil {
		return nil, err
	}
	if err := validateNoNumericToBoolAutoConversion(&program); err != nil {
		return nil, err
	}
	lowerProgram(&program)
	program.Symbols, program.ValueTypes, program.SeriesKeys = collectProgramRequirements(program)
	bytecode, err := encodeProgram(program)
	if err != nil {
		return nil, err
	}
	e.cachedBytecode = bytecode
	e.cachedProgram = program
	e.cacheValid = true
	return bytecode, nil
}

// Execute runs pre-compiled bytecode against the registered market data providers.
// The result is the value of the last expression in the compiled script, or nil
// if the script produces NaN. After execution, Runtime returns the Runtime
// instance for inspection.
//
// Returns an error if no market data provider is registered, the bytecode is
// corrupt, a provider fails to supply a required series, or a runtime error
// occurs (such as an unknown identifier or unsupported operation).
func (e *Engine) Execute(bytecode []byte) (interface{}, error) {
	_, v, err := e.ExecuteWithRuntime(bytecode)
	return v, err
}

// ExecuteWithRuntime is like Execute but also returns the Runtime instance,
// which holds the execution state after the run completes. Use Runtime.Snapshot
// to inspect variables, Runtime.Series to retrieve computed series, and
// Runtime.ValueTypes to list available value types per symbol.
//
// After this call, Runtime returns the same Runtime until the next Execute or
// ClearRuntime. The caller must not mutate the returned Runtime.
func (e *Engine) ExecuteWithRuntime(bytecode []byte) (*Runtime, interface{}, error) {
	program, err := e.decodeProgramCached(bytecode)
	if err != nil {
		return nil, nil, err
	}
	if len(e.providers) == 0 {
		return nil, nil, errors.New("market data provider is not registered")
	}

	timeframe := e.timeframe
	if timeframe == "" {
		timeframe = e.providers[0].GetTimeframe()
	}
	session := e.session
	if session == "" {
		session = e.providers[0].GetSession()
	}
	for _, p := range e.providers {
		if timeframe != "" {
			if err := p.SetTimeframe(timeframe); err != nil {
				return nil, nil, fmt.Errorf("set timeframe %q: %w", timeframe, err)
			}
		}
		if session != "" {
			if err := p.SetSession(session); err != nil {
				return nil, nil, fmt.Errorf("set session %q: %w", session, err)
			}
		}
	}

	functions := copyUserFunctions(e.functions)
	functionParams := copyCallParamSpecs(e.functionParams)

	catalog, err := buildProviderCatalog(e.providers)
	if err != nil {
		return nil, nil, err
	}

	activeSymbol, activeValueType, err := e.resolveActiveSelection(catalog)
	if err != nil {
		return nil, nil, err
	}
	baseSeries, err := catalog.fetchSeries(activeSymbol, activeValueType)
	if err != nil {
		return nil, nil, err
	}
	currentTime := e.currentTime
	if currentTime.IsZero() {
		currentTime = time.Now().UTC()
	}
	startTime := e.startTime
	if startTime.IsZero() {
		startTime = inferStartTime(currentTime, timeframe, baseSeries.Length())
	}

	neededKeys := collectNeededSeriesKeys(program, activeSymbol, activeValueType)

	seriesByKey := map[string]SeriesExtended{}
	seriesByKey[makeSeriesKey(activeSymbol, activeValueType)] = baseSeries
	rt := newRuntime(program, functions, seriesByKey, catalog.valueTypesBySymbol, activeSymbol, activeValueType, timeframe, session, currentTime, startTime, baseSeries.Length(), catalog.fetchSeries, e.appendLog, e.alertSink)
	rt.userFnParamSpecs = functionParams
	for key := range neededKeys {
		symbol, valueType, ok := splitSeriesKey(key)
		if !ok {
			return nil, nil, fmt.Errorf("invalid series key in bytecode: %q", key)
		}
		ser, err := rt.getSeries(symbol, valueType)
		if err != nil {
			return nil, nil, err
		}
		seriesByKey[key] = ser
	}

	for i := 0; i < baseSeries.Length(); i++ {
		rt.SetBarIndex(i)
		if err := rt.execTopLevel(); err != nil {
			return nil, nil, err
		}
		if err := rt.commitBar(); err != nil {
			return nil, nil, err
		}
	}

	e.lastRuntime = rt
	if math.IsNaN(rt.lastValue) {
		return rt, nil, nil
	}
	return rt, rt.lastValue, nil
}

func (e *Engine) decodeProgramCached(bytecode []byte) (Program, error) {
	if e.cacheValid && sameByteSlice(e.cachedBytecode, bytecode) {
		return e.cachedProgram, nil
	}
	if e.cacheValid && len(e.cachedBytecode) == len(bytecode) && bytes.Equal(e.cachedBytecode, bytecode) {
		return e.cachedProgram, nil
	}
	program, err := decodeProgram(bytecode)
	if err != nil {
		return Program{}, err
	}
	e.cachedBytecode = bytecode
	e.cachedProgram = program
	e.cacheValid = true
	return program, nil
}

func sameByteSlice(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	return &a[0] == &b[0]
}

func inferStartTime(current time.Time, timeframe string, bars int) time.Time {
	if current.IsZero() {
		current = time.Now().UTC()
	}
	if bars <= 1 {
		return current
	}
	seconds, ok := timeframeInSeconds(timeframe)
	if !ok || seconds <= 0 {
		seconds = 60
	}
	step := time.Duration(seconds) * time.Second
	return current.Add(-step * time.Duration(bars-1))
}

func makeSeriesKey(symbol, valueType string) string {
	return symbol + "|" + valueType
}

func splitSeriesKey(key string) (string, string, bool) {
	parts := strings.SplitN(key, "|", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func chooseDefaultValueType(valueTypeSet map[string]bool) string {
	if valueTypeSet["close"] {
		return "close"
	}
	if len(valueTypeSet) == 0 {
		return ""
	}
	vts := make([]string, 0, len(valueTypeSet))
	for vt := range valueTypeSet {
		vts = append(vts, vt)
	}
	sort.Strings(vts)
	return vts[0]
}

func cloneBoolSet(in map[string]bool) map[string]bool {
	out := map[string]bool{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

func collectProgramRequirements(program Program) ([]string, []string, []string) {
	seenSymbols := map[string]bool{}
	seenValueTypes := map[string]bool{}
	seenSeriesKeys := map[string]bool{}

	addSymbol := func(symbol string) {
		if symbol != "" {
			seenSymbols[symbol] = true
		}
	}
	addValueType := func(valueType string) {
		if valueType != "" {
			seenValueTypes[valueType] = true
		}
	}
	addIdentifierValueTypes := func(name string) {
		switch name {
		case "open", "high", "low", "close", "volume":
			addValueType(name)
		case "hl2":
			addValueType("high")
			addValueType("low")
		case "hlc3", "hlcc4":
			addValueType("high")
			addValueType("low")
			addValueType("close")
		case "ohlc4":
			addValueType("open")
			addValueType("high")
			addValueType("low")
			addValueType("close")
		}
	}
	addSeriesKey := func(symbol, valueType string) {
		if symbol == "" || valueType == "" {
			return
		}
		seenSeriesKeys[makeSeriesKey(symbol, valueType)] = true
		seenSymbols[symbol] = true
		seenValueTypes[valueType] = true
	}

	valueTypeFromExpr := func(expr *Expr) (string, bool) {
		if expr == nil {
			return "", false
		}
		if expr.Kind == "ident" {
			switch expr.Name {
			case "open", "high", "low", "close", "volume":
				return expr.Name, true
			case "hl2", "hlc3", "hlcc4", "ohlc4":
				return "close", true
			}
		}
		if expr.Kind == "call" && expr.Left != nil && expr.Left.Kind == "ident" {
			switch expr.Left.Name {
			case "open_of":
				return "open", true
			case "high_of":
				return "high", true
			case "low_of":
				return "low", true
			case "close_of":
				return "close", true
			case "value_of":
				if len(expr.Args) >= 2 && expr.Args[1] != nil && expr.Args[1].Kind == "string" {
					return expr.Args[1].String, true
				}
			}
		}
		return "", false
	}

	var visitExpr func(expr *Expr)
	visitExpr = func(expr *Expr) {
		if expr == nil {
			return
		}

		if expr.Kind == "ident" {
			addIdentifierValueTypes(expr.Name)
		}

		if expr.Kind == "call" && expr.Left != nil && expr.Left.Kind == "ident" {
			name := expr.Left.Name
			switch name {
			case "close_of", "open_of", "high_of", "low_of":
				valueType := "close"
				switch name {
				case "open_of":
					valueType = "open"
				case "high_of":
					valueType = "high"
				case "low_of":
					valueType = "low"
				}
				addValueType(valueType)
				if len(expr.Args) >= 1 && expr.Args[0] != nil && expr.Args[0].Kind == "string" {
					addSeriesKey(expr.Args[0].String, valueType)
				}
			case "value_of":
				var symbol string
				var valueType string
				if len(expr.Args) >= 1 && expr.Args[0] != nil && expr.Args[0].Kind == "string" {
					symbol = expr.Args[0].String
					addSymbol(symbol)
				}
				if len(expr.Args) >= 2 && expr.Args[1] != nil && expr.Args[1].Kind == "string" {
					valueType = expr.Args[1].String
					addValueType(valueType)
				}
				if symbol != "" && valueType != "" {
					addSeriesKey(symbol, valueType)
				}
			case "sma_of", "ema_of", "rsi_of":
				valueType := "close"
				if len(expr.Args) >= 3 && expr.Args[2] != nil && expr.Args[2].Kind == "string" && expr.Args[2].String != "" {
					valueType = expr.Args[2].String
				}
				addValueType(valueType)
				if len(expr.Args) >= 1 && expr.Args[0] != nil && expr.Args[0].Kind == "string" {
					addSeriesKey(expr.Args[0].String, valueType)
				}
			case "sma", "ema", "rsi", "atr", "ta.sma", "ta.ema", "ta.rsi", "ta.atr":
				if len(expr.Args) > 0 {
					if vt, ok := valueTypeFromExpr(expr.Args[0]); ok {
						addValueType(vt)
					}
				}
			case "crossover", "crossunder", "ta.crossover", "ta.crossunder":
				for _, arg := range expr.Args {
					if vt, ok := valueTypeFromExpr(arg); ok {
						addValueType(vt)
					}
				}
			}
		}

		visitExpr(expr.Left)
		visitExpr(expr.Right)
		visitExpr(expr.Else)
		for _, a := range expr.Args {
			visitExpr(a)
		}
		for _, e := range expr.Elems {
			visitExpr(e)
		}
	}

	var visitStmt func(stmt Stmt)
	visitStmt = func(stmt Stmt) {
		visitExpr(stmt.Expr)
		visitExpr(stmt.Target)
		visitExpr(stmt.Cond)
		visitExpr(stmt.From)
		visitExpr(stmt.To)
		visitExpr(stmt.By)
		for _, s := range stmt.Then {
			visitStmt(s)
		}
		for _, s := range stmt.Else {
			visitStmt(s)
		}
		for _, s := range stmt.Body {
			visitStmt(s)
		}
		if stmt.Func != nil {
			for _, s := range stmt.Func.Body {
				visitStmt(s)
			}
			visitExpr(stmt.Func.Expr)
		}
	}

	for _, s := range program.Stmts {
		visitStmt(s)
	}
	for _, fn := range program.Functions {
		for _, s := range fn.Body {
			visitStmt(s)
		}
		visitExpr(fn.Expr)
	}

	symbols := make([]string, 0, len(seenSymbols))
	for s := range seenSymbols {
		symbols = append(symbols, s)
	}
	valueTypes := make([]string, 0, len(seenValueTypes))
	for vt := range seenValueTypes {
		valueTypes = append(valueTypes, vt)
	}
	seriesKeys := make([]string, 0, len(seenSeriesKeys))
	for k := range seenSeriesKeys {
		seriesKeys = append(seriesKeys, k)
	}
	sort.Strings(symbols)
	sort.Strings(valueTypes)
	sort.Strings(seriesKeys)
	return symbols, valueTypes, seriesKeys
}
