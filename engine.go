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

type SeriesExtended = series.SeriesExtend

type Provider interface {
	GetSeries(seriesKey string) (SeriesExtended, error)
	GetSymbols() ([]string, error)
	GetValuesTypes() ([]string, error)
	SetTimeframe(timeframe string) error
	GetTimeframe() string
	SetSession(session string) error
	GetSession() string
}

type UserFunction func(args ...interface{}) (interface{}, error)

type EngineLogEntry struct {
	Timestamp time.Time
	Level     string
	Message   string
}

type Engine struct {
	providers        []Provider
	functions        map[string]UserFunction
	defaultSymbol    string
	defaultValueType string
	timeframe        string
	session          string
	currentTime      time.Time
	startTime        time.Time
	lastRuntime      *Runtime
	logs             []EngineLogEntry
	cachedBytecode   []byte
	cachedProgram    Program
	cacheValid       bool
}

type providerCatalog struct {
	providerBySymbol   map[string]Provider
	valueTypesBySymbol map[string]map[string]bool
	orderedSymbols     []string
}

func NewEngine() *Engine {
	return &Engine{functions: map[string]UserFunction{}}
}

func (e *Engine) RegisterFunction(name string, function UserFunction) {
	e.functions[name] = function
}

func (e *Engine) RegisterMarketDataProvider(provider Provider) {
	e.providers = append(e.providers, provider)
	if e.timeframe == "" {
		e.timeframe = provider.GetTimeframe()
	}
	if e.session == "" {
		e.session = provider.GetSession()
	}
}

func (e *Engine) SetDefaultSymbol(symbol string) {
	e.defaultSymbol = symbol
}

func (e *Engine) SetDefaultValueType(valueType string) {
	e.defaultValueType = valueType
}

func (e *Engine) SetTimeframe(timeframe string) {
	e.timeframe = timeframe
}

func (e *Engine) Timeframe() string {
	return e.timeframe
}

func (e *Engine) SetSession(session string) {
	e.session = session
}

func (e *Engine) Session() string {
	return e.session
}

func (e *Engine) SetCurrentTime(now time.Time) {
	e.currentTime = now
}

func (e *Engine) CurrentTime() time.Time {
	return e.currentTime
}

func (e *Engine) SetStartTime(start time.Time) {
	e.startTime = start
}

func (e *Engine) StartTime() time.Time {
	return e.startTime
}

func (e *Engine) Logs() []EngineLogEntry {
	out := make([]EngineLogEntry, len(e.logs))
	copy(out, e.logs)
	return out
}

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

func (e *Engine) Runtime() *Runtime {
	return e.lastRuntime
}

func (e *Engine) ClearRuntime() {
	if e.lastRuntime != nil {
		e.lastRuntime.Release()
	}
	e.lastRuntime = nil
}

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

func RegisterFunction(name string, function func(args ...interface{}) (interface{}, error)) {
	defaultEngine.RegisterFunction(name, function)
}

func RegisterMarketDataProvider(provider Provider) {
	defaultEngine.RegisterMarketDataProvider(provider)
}

func SetTimeframe(timeframe string) {
	defaultEngine.SetTimeframe(timeframe)
}

func SetSession(session string) {
	defaultEngine.SetSession(session)
}

func SetCurrentTime(now time.Time) {
	defaultEngine.SetCurrentTime(now)
}

func SetStartTime(start time.Time) {
	defaultEngine.SetStartTime(start)
}

func Logs() []EngineLogEntry {
	return defaultEngine.Logs()
}

func ClearLogs() {
	defaultEngine.ClearLogs()
}

func Compile(pinescript string) ([]byte, error) {
	return defaultEngine.Compile(pinescript)
}

func Execute(bytecode []byte) (interface{}, error) {
	return defaultEngine.Execute(bytecode)
}

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

func (e *Engine) Execute(bytecode []byte) (interface{}, error) {
	_, v, err := e.ExecuteWithRuntime(bytecode)
	return v, err
}

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
	rt := newRuntime(program, functions, seriesByKey, catalog.valueTypesBySymbol, activeSymbol, activeValueType, timeframe, session, currentTime, startTime, baseSeries.Length(), catalog.fetchSeries, e.appendLog)
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
		rt.barIndex = i
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
