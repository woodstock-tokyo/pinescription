// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type flowKind string

const (
	flowNone     flowKind = "none"
	flowBreak    flowKind = "break"
	flowContinue flowKind = "continue"
	flowReturn   flowKind = "return"
)

const maxPooledInterfaceCap = 256
const maxPooledEnvMapEntries = 64

var interfaceSlicePool = sync.Pool{New: func() interface{} { return make([]interface{}, 0, 8) }}
var envMapPool = sync.Pool{New: func() interface{} { return make(map[string]interface{}, 8) }}

var disableCallArgPooling bool
var disableEnvMapPooling bool
var disableLoopIteratorFastPath bool
var disableSwitchCaseConstFastPath bool

func acquireInterfaceSlice(size int) []interface{} {
	v := interfaceSlicePool.Get()
	buf, ok := v.([]interface{})
	if !ok {
		return make([]interface{}, 0, size)
	}
	if cap(buf) < size {
		return make([]interface{}, 0, size)
	}
	return buf[:0]
}

func releaseInterfaceSlice(buf []interface{}) {
	if buf == nil {
		return
	}
	if cap(buf) > maxPooledInterfaceCap {
		return
	}
	for i := range buf {
		buf[i] = nil
	}
	interfaceSlicePool.Put(buf[:0])
}

func acquireEnvMap() map[string]interface{} {
	v := envMapPool.Get()
	m, ok := v.(map[string]interface{})
	if !ok {
		return map[string]interface{}{}
	}
	clear(m)
	return m
}

func releaseEnvMap(m map[string]interface{}) {
	if m == nil {
		return
	}
	if len(m) > maxPooledEnvMapEntries {
		return
	}
	clear(m)
	envMapPool.Put(m)
}

type flow struct {
	kind     flowKind
	value    interface{}
	hasValue bool
}

type RuntimeSnapshot struct {
	BarIndex        int
	LastValue       float64
	ActiveSymbol    string
	ActiveValueType string
	Symbols         []string
	SeriesKeys      []string
	Variables       map[string]interface{}
}

type Runtime struct {
	program Program
	userFns map[string]UserFunction

	rootNamespaces map[string]interface{}

	seriesByKey         map[string]SeriesExtended
	namedSeries         map[string]SeriesExtended
	seriesExprByName    map[string]*Expr
	seriesExprResolving map[string]bool
	indicatorState      map[string]interface{}
	extremaState        map[extremaStateKey]*extremaIndicatorState
	valueTypesBySymbol  map[string]map[string]bool
	loadSeries          func(symbol, valueType string) (SeriesExtended, error)

	activeSymbol    string
	activeValueType string
	timeframe       string
	session         string
	timeframePeriod string
	timeframeBase   string
	timeframeMult   int
	timeframeSecs   int
	timeframeSecsOK bool
	currentTime     time.Time
	startTime       time.Time
	logSink         func(level, message string, ts time.Time)
	alertSink       func(AlertEvent)
	barStep         time.Duration
	lastTimeIndex   int
	lastBarOpen     time.Time
	lastBarClose    time.Time
	lastTradingDay  time.Time
	seriesKeyCache  map[string]map[string]string
	identKindCache  map[string]identifierKind
	expectedBars    int
	rootHistoryVars []string
	rootHistorySet  map[string]struct{}
	priceCacheBar   int
	priceCacheMask  uint8
	priceCacheOpen  float64
	priceCacheHigh  float64
	priceCacheLow   float64
	priceCacheClose float64
	priceCacheVol   float64
	loopBindings    []loopBinding

	barIndex   int
	evalOffset int
	lastValue  float64

	envStack       []map[string]interface{}
	consts         map[string]bool
	declaredTypes  map[string]string
	history        map[string][]interface{}
	numericHistory map[string][]float64
	historyKind    map[string]historyStorageKind
}

type historyStorageKind uint8

const (
	historyStorageUnknown historyStorageKind = iota
	historyStorageNumeric
	historyStorageGeneric
)

type customTypeInstance struct {
	TypeName string
	Fields   map[string]interface{}
}

type identifierKind uint8

const (
	identKindGeneric identifierKind = iota
	identKindPrice
	identKindBarIndex
	identKindMathConst
	identKindSessionConst
	identKindTime
	identKindTimeframe
	identKindDotted
)

type loopBinding struct {
	name  string
	value float64
}

type callParamSpec struct {
	Names []string
}

func (s callParamSpec) indexOf(name string) int {
	for i, candidate := range s.Names {
		if candidate == name {
			return i
		}
	}
	return -1
}

var builtinCallParamSpecs = map[string]callParamSpec{
	"alert":        {Names: []string{"message", "freq"}},
	"barcolor":     {Names: []string{"color", "offset", "editable", "show_last", "title", "display"}},
	"box.new":      {Names: []string{"left", "top", "right", "bottom", "border_color", "border_width", "border_style", "extend", "xloc", "bgcolor", "text", "text_size", "text_color", "text_halign", "text_valign", "text_wrap", "force_overlay"}},
	"color.new":    {Names: []string{"color", "transp"}},
	"indicator":    {Names: []string{"title", "shorttitle", "overlay", "format", "precision", "scale", "max_bars_back", "timeframe", "timeframe_gaps", "explicit_plot_zorder", "max_lines_count", "max_labels_count", "max_boxes_count", "max_polylines_count", "dynamic_requests", "behind_chart"}},
	"input":        {Names: []string{"defval", "title", "tooltip", "inline", "group", "display"}},
	"input.bool":   {Names: []string{"defval", "title", "tooltip", "inline", "group", "display"}},
	"input.color":  {Names: []string{"defval", "title", "tooltip", "inline", "group", "display"}},
	"input.float":  {Names: []string{"defval", "title", "tooltip", "inline", "group", "display", "step", "minval", "maxval", "confirm"}},
	"input.int":    {Names: []string{"defval", "title", "tooltip", "inline", "group", "display", "step", "minval", "maxval", "confirm"}},
	"input.source": {Names: []string{"defval", "title", "tooltip", "inline", "group", "display"}},
	"input.string": {Names: []string{"defval", "title", "tooltip", "inline", "group", "display", "options", "confirm"}},
	"table.cell":   {Names: []string{"table_id", "column", "row", "text", "width", "height", "text_color", "text_halign", "text_valign", "text_size", "bgcolor", "tooltip", "text_font_family"}},
	"table.new":    {Names: []string{"position", "columns", "rows", "bgcolor", "frame_color", "frame_width", "border_color", "border_width"}},
}

func (r *Runtime) Release() {
	if r == nil {
		return
	}
	r.program = Program{}
	r.userFns = nil
	r.rootNamespaces = nil
	r.seriesByKey = nil
	r.namedSeries = nil
	r.seriesExprByName = nil
	r.seriesExprResolving = nil
	r.indicatorState = nil
	r.extremaState = nil
	r.valueTypesBySymbol = nil
	r.loadSeries = nil
	r.activeSymbol = ""
	r.activeValueType = ""
	r.timeframe = ""
	r.session = ""
	r.timeframePeriod = ""
	r.timeframeBase = ""
	r.timeframeMult = 0
	r.timeframeSecs = 0
	r.timeframeSecsOK = false
	r.currentTime = time.Time{}
	r.startTime = time.Time{}
	r.logSink = nil
	r.alertSink = nil
	r.barStep = 0
	r.lastTimeIndex = -1
	r.lastBarOpen = time.Time{}
	r.lastBarClose = time.Time{}
	r.lastTradingDay = time.Time{}
	r.seriesKeyCache = nil
	r.identKindCache = nil
	r.expectedBars = 0
	r.rootHistoryVars = nil
	r.rootHistorySet = nil
	r.priceCacheBar = -1
	r.priceCacheMask = 0
	r.priceCacheOpen = math.NaN()
	r.priceCacheHigh = math.NaN()
	r.priceCacheLow = math.NaN()
	r.priceCacheClose = math.NaN()
	r.priceCacheVol = math.NaN()
	r.loopBindings = nil
	r.barIndex = 0
	r.evalOffset = 0
	r.lastValue = math.NaN()
	r.envStack = nil
	r.consts = nil
	r.declaredTypes = nil
	r.history = nil
	r.numericHistory = nil
	r.historyKind = nil
}

func newRuntime(
	program Program,
	userFns map[string]UserFunction,
	seriesByKey map[string]SeriesExtended,
	valueTypesBySymbol map[string]map[string]bool,
	activeSymbol string,
	activeValueType string,
	timeframe string,
	session string,
	currentTime time.Time,
	startTime time.Time,
	expectedBars int,
	loadSeries func(symbol, valueType string) (SeriesExtended, error),
	logSink func(level, message string, ts time.Time),
	alertSink func(AlertEvent),
) *Runtime {
	if seriesByKey == nil {
		seriesByKey = map[string]SeriesExtended{}
	}
	if valueTypesBySymbol == nil {
		valueTypesBySymbol = map[string]map[string]bool{}
	}
	r := &Runtime{
		program:             program,
		userFns:             userFns,
		seriesByKey:         seriesByKey,
		namedSeries:         map[string]SeriesExtended{},
		seriesExprByName:    map[string]*Expr{},
		seriesExprResolving: map[string]bool{},
		indicatorState:      map[string]interface{}{},
		extremaState:        map[extremaStateKey]*extremaIndicatorState{},
		valueTypesBySymbol:  valueTypesBySymbol,
		loadSeries:          loadSeries,
		activeSymbol:        activeSymbol,
		activeValueType:     activeValueType,
		timeframe:           timeframe,
		session:             session,
		currentTime:         currentTime,
		startTime:           startTime,
		logSink:             logSink,
		alertSink:           alertSink,
		lastTimeIndex:       -1,
		expectedBars:        expectedBars,
		lastValue:           math.NaN(),
		envStack:            []map[string]interface{}{{}},
		consts:              map[string]bool{},
		declaredTypes:       map[string]string{},
		history:             map[string][]interface{}{},
		numericHistory:      map[string][]float64{},
		historyKind:         map[string]historyStorageKind{},
		seriesKeyCache:      map[string]map[string]string{},
		identKindCache:      map[string]identifierKind{},
		rootHistorySet:      map[string]struct{}{},
		priceCacheBar:       -1,
		priceCacheMask:      0,
		priceCacheOpen:      math.NaN(),
		priceCacheHigh:      math.NaN(),
		priceCacheLow:       math.NaN(),
		priceCacheClose:     math.NaN(),
		priceCacheVol:       math.NaN(),
		loopBindings:        nil,
	}
	r.initTimeframeCache()
	r.initRootEnv()
	return r
}

func (r *Runtime) initRootEnv() {
	if r == nil || len(r.envStack) == 0 {
		return
	}
	root := r.envStack[0]
	root["na"] = nil
	root["last_bar_index"] = float64(r.expectedBars - 1)
	barstate := map[string]interface{}{"islast": false}
	syminfo := map[string]interface{}{"ticker": r.activeSymbol}
	alertNs := map[string]interface{}{
		"freq_all":                "all",
		"freq_once_per_bar":       "once_per_bar",
		"freq_once_per_bar_close": "once_per_bar_close",
	}
	displayNs := map[string]interface{}{"none": float64(0), "all": float64(1), "status_line": float64(0)}
	positionNs := map[string]interface{}{"top_right": float64(0), "bottom_right": float64(1), "top_left": float64(2), "bottom_left": float64(3)}
	xlocNs := map[string]interface{}{"bar_index": float64(0), "bar_time": float64(1)}
	ylocNs := map[string]interface{}{"price": float64(0), "abovebar": float64(1), "belowbar": float64(2)}
	extendNs := map[string]interface{}{"none": float64(0), "right": float64(1), "left": float64(2), "both": float64(3)}
	sizeNs := map[string]interface{}{"tiny": float64(0), "small": float64(1), "normal": float64(2), "large": float64(3), "huge": float64(4), "auto": float64(5)}
	textNs := map[string]interface{}{"align_left": float64(0), "align_center": float64(1), "align_right": float64(2)}
	formatNs := map[string]interface{}{"volume": "volume", "price": "price", "percent": "percent"}
	colorNs := map[string]interface{}{
		"white":       "#ffffff",
		"black":       "#000000",
		"gray":        "#808080",
		"silver":      "#c0c0c0",
		"red":         "#ff0000",
		"green":       "#00ff00",
		"blue":        "#0000ff",
		"yellow":      "#ffff00",
		"orange":      "#ffa500",
		"transparent": "#00000000",
	}
	lineNs := map[string]interface{}{"style_solid": float64(0), "style_dashed": float64(1), "style_dotted": float64(2)}
	labelNs := map[string]interface{}{"style_label_left": float64(0), "style_label_right": float64(1), "style_label_up": float64(2), "style_label_down": float64(3)}

	root["barstate"] = barstate
	root["syminfo"] = syminfo
	root["alert"] = alertNs
	root["display"] = displayNs
	root["position"] = positionNs
	root["xloc"] = xlocNs
	root["yloc"] = ylocNs
	root["extend"] = extendNs
	root["size"] = sizeNs
	root["text"] = textNs
	root["format"] = formatNs
	root["color"] = colorNs
	root["line"] = lineNs
	root["label"] = labelNs

	r.rootNamespaces = map[string]interface{}{
		"barstate": barstate,
		"syminfo":  syminfo,
		"alert":    alertNs,
		"display":  displayNs,
		"position": positionNs,
		"xloc":     xlocNs,
		"yloc":     ylocNs,
		"extend":   extendNs,
		"size":     sizeNs,
		"text":     textNs,
		"format":   formatNs,
		"color":    colorNs,
		"line":     lineNs,
		"label":    labelNs,
	}
}

func (r *Runtime) SetBarIndex(i int) {
	r.barIndex = i
	if len(r.envStack) == 0 {
		return
	}
	root := r.envStack[0]
	root["last_bar_index"] = float64(r.expectedBars - 1)
	isLast := i == r.expectedBars-1
	if r.rootNamespaces != nil {
		if bs, ok := r.rootNamespaces["barstate"].(map[string]interface{}); ok {
			bs["islast"] = isLast
		}
	}
	if bs, ok := root["barstate"].(map[string]interface{}); ok {
		bs["islast"] = isLast
	}
}

func (r *Runtime) emitAlert(message, frequency string) {
	if r == nil || r.alertSink == nil {
		return
	}
	idx := r.effectiveBarIndex()
	if idx < 0 {
		idx = 0
	}
	r.alertSink(AlertEvent{
		Message:   message,
		Frequency: frequency,
		BarIndex:  idx,
		Time:      r.barCloseTime().UTC(),
		Symbol:    r.activeSymbol,
	})
}

func (r *Runtime) initTimeframeCache() {
	tf := normalizeTimeframe(r.timeframe)
	if tf == "" {
		tf = "1D"
	}
	base, mult, ok := parseTimeframe(tf)
	if !ok {
		base = "D"
		mult = 1
	}
	r.timeframePeriod = tf
	r.timeframeBase = base
	r.timeframeMult = mult
	r.timeframeSecs, r.timeframeSecsOK = timeframeSecondsFromParsed(base, mult)
	if r.timeframeSecsOK && r.timeframeSecs > 0 {
		r.barStep = time.Duration(r.timeframeSecs) * time.Second
	} else {
		r.timeframeSecs = 60
		r.barStep = 60 * time.Second
	}
	r.lastTimeIndex = -1
	r.lastBarOpen = time.Time{}
	r.lastBarClose = time.Time{}
	r.lastTradingDay = time.Time{}
}

func (r *Runtime) cachedSeriesKey(symbol, valueType string) string {
	if symbol == "" || valueType == "" {
		return makeSeriesKey(symbol, valueType)
	}
	inner, ok := r.seriesKeyCache[symbol]
	if !ok {
		inner = map[string]string{}
		r.seriesKeyCache[symbol] = inner
	}
	if key, ok := inner[valueType]; ok {
		return key
	}
	key := makeSeriesKey(symbol, valueType)
	inner[valueType] = key
	return key
}

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

func (r *Runtime) SeriesKeys() []string {
	out := make([]string, 0, len(r.seriesByKey))
	for key := range r.seriesByKey {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

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

func (r *Runtime) execTopLevel() error {
	fl, err := r.execStmtList(r.program.Stmts)
	if err != nil {
		return err
	}
	if fl.kind == flowReturn {
		return nil
	}
	return nil
}

func (r *Runtime) commitBar() error {
	if len(r.envStack) == 0 {
		return nil
	}
	root := r.envStack[0]
	prevLast := r.lastValue
	for _, name := range r.rootHistoryVars {
		v, ok := root[name]
		if !ok {
			continue
		}
		if err := r.recordHistory(name, v); err != nil {
			return err
		}
	}
	r.lastValue = prevLast
	return nil
}

func (r *Runtime) execStmtList(stmts []Stmt) (flow, error) {
	var last interface{}
	hasLast := false
	for _, stmt := range stmts {
		fl, err := r.execStmt(stmt)
		if err != nil {
			return flow{}, err
		}
		if fl.kind != flowNone {
			return fl, nil
		}
		if fl.hasValue {
			last = fl.value
			hasLast = true
		}
	}
	return flow{kind: flowNone, value: last, hasValue: hasLast}, nil
}

func (r *Runtime) execStmt(stmt Stmt) (flow, error) {
	switch stmt.Kind {
	case "decl":
		if stmt.TypeName != "" && r.declaredTypes != nil {
			r.declaredTypes[stmt.Name] = stmt.TypeName
		}
		var v interface{}
		if stmt.Expr != nil {
			e, err := r.eval(stmt.Expr)
			if err != nil {
				return flow{}, err
			}
			v = e
		} else if stmt.TypeName == "bool" {
			v = false
		}
		if err := r.assign(stmt.Name, v, stmt.Const, false); err != nil {
			return flow{}, err
		}
		r.registerSeriesExpr(stmt.Name, stmt.Expr)
		return flow{kind: flowNone}, nil
	case "assign":
		if fast, v, err := r.evalSelfBinaryAssign(stmt.Name, stmt.Expr); err != nil {
			return flow{}, err
		} else if fast {
			if err := r.assign(stmt.Name, v, false, true); err != nil {
				return flow{}, err
			}
			r.registerSeriesExpr(stmt.Name, stmt.Expr)
			return flow{kind: flowNone}, nil
		}
		v, err := r.eval(stmt.Expr)
		if err != nil {
			return flow{}, err
		}
		if err := r.assign(stmt.Name, v, false, true); err != nil {
			return flow{}, err
		}
		r.registerSeriesExpr(stmt.Name, stmt.Expr)
		return flow{kind: flowNone}, nil
	case "tuple_assign":
		rhs, err := r.eval(stmt.Expr)
		if err != nil {
			return flow{}, err
		}
		var items []interface{}
		if rhs == nil {
			items = nil
		} else {
			switch v := rhs.(type) {
			case []interface{}:
				items = v
			case *pineArray:
				items = v.items
			default:
				return flow{}, fmt.Errorf("tuple assignment requires tuple/array RHS")
			}
		}
		for i, name := range stmt.TupleNames {
			if name == "_" {
				continue
			}
			var value interface{} = math.NaN()
			if i < len(items) {
				value = items[i]
			}
			if err := r.assign(name, value, false, false); err != nil {
				return flow{}, err
			}
		}
		return flow{kind: flowNone}, nil
	case "expr":
		v, err := r.eval(stmt.Expr)
		if err != nil {
			return flow{}, err
		}
		if n, ok := toFloat(v); ok {
			r.lastValue = n
		}
		return flow{kind: flowNone, value: v, hasValue: true}, nil
	case "if":
		c, err := r.eval(stmt.Cond)
		if err != nil {
			return flow{}, err
		}
		block := stmt.Else
		if truthy(c) {
			block = stmt.Then
		}
		return r.execStmtList(block)
	case "switch":
		return r.execSwitch(stmt)
	case "while":
		var last interface{}
		hasLast := false
		for {
			c, err := r.eval(stmt.Cond)
			if err != nil {
				return flow{}, err
			}
			if !truthy(c) {
				break
			}
			fl, err := r.execStmtList(stmt.Body)
			if err != nil {
				return flow{}, err
			}
			switch fl.kind {
			case flowNone:
				if fl.hasValue {
					last = fl.value
					hasLast = true
				}
			case flowBreak:
				return flow{kind: flowNone, value: last, hasValue: hasLast}, nil
			case flowContinue:
				continue
			default:
				return fl, nil
			}
		}
		return flow{kind: flowNone, value: last, hasValue: hasLast}, nil
	case "for":
		fromV, err := r.eval(stmt.From)
		if err != nil {
			return flow{}, err
		}
		toV, err := r.eval(stmt.To)
		if err != nil {
			return flow{}, err
		}
		from, _ := toFloat(fromV)
		to, _ := toFloat(toV)
		step := 1.0
		if stmt.By != nil {
			sv, err := r.eval(stmt.By)
			if err != nil {
				return flow{}, err
			}
			step, _ = toFloat(sv)
		}
		if step == 0 {
			return flow{}, errors.New("for step cannot be 0")
		}
		if disableLoopIteratorFastPath {
			var last interface{}
			hasLast := false
			cmp := func(i float64) bool {
				if step > 0 {
					return i <= to
				}
				return i >= to
			}
			for i := from; cmp(i); i += step {
				_ = r.assign(stmt.ForVar, i, false, false)
				fl, err := r.execStmtList(stmt.Body)
				if err != nil {
					return flow{}, err
				}
				switch fl.kind {
				case flowNone:
					if fl.hasValue {
						last = fl.value
						hasLast = true
					}
				case flowBreak:
					return flow{kind: flowNone, value: last, hasValue: hasLast}, nil
				case flowContinue:
					continue
				default:
					return fl, nil
				}
			}
			return flow{kind: flowNone, value: last, hasValue: hasLast}, nil
		}

		if r.consts[stmt.ForVar] {
			return flow{}, fmt.Errorf("cannot assign const variable %s", stmt.ForVar)
		}
		scope := r.envStack[len(r.envStack)-1]
		prevValue, hadPrev := scope[stmt.ForVar]
		defer func() {
			if hadPrev {
				scope[stmt.ForVar] = prevValue
			} else {
				delete(scope, stmt.ForVar)
			}
		}()
		r.loopBindings = append(r.loopBindings, loopBinding{name: stmt.ForVar, value: from})
		defer func() {
			r.loopBindings = r.loopBindings[:len(r.loopBindings)-1]
		}()

		var last interface{}
		hasLast := false
		cmp := func(i float64) bool {
			if step > 0 {
				return i <= to
			}
			return i >= to
		}
		for i := from; cmp(i); i += step {
			r.loopBindings[len(r.loopBindings)-1].value = i
			fl, err := r.execStmtList(stmt.Body)
			if err != nil {
				return flow{}, err
			}
			switch fl.kind {
			case flowNone:
				if fl.hasValue {
					last = fl.value
					hasLast = true
				}
			case flowBreak:
				return flow{kind: flowNone, value: last, hasValue: hasLast}, nil
			case flowContinue:
				continue
			default:
				return fl, nil
			}
		}
		return flow{kind: flowNone, value: last, hasValue: hasLast}, nil
	case "break":
		return flow{kind: flowBreak}, nil
	case "continue":
		return flow{kind: flowContinue}, nil
	case "return":
		if stmt.Expr == nil {
			return flow{kind: flowReturn}, nil
		}
		v, err := r.eval(stmt.Expr)
		if err != nil {
			return flow{}, err
		}
		return flow{kind: flowReturn, value: v}, nil
	default:
		return flow{}, fmt.Errorf("unsupported statement kind: %s", stmt.Kind)
	}
}

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

func (r *Runtime) execSwitch(stmt Stmt) (flow, error) {
	hasSwitchExpr := stmt.SwitchExpr != nil
	var switchValue interface{}
	if hasSwitchExpr {
		v, err := r.eval(stmt.SwitchExpr)
		if err != nil {
			return flow{}, err
		}
		switchValue = v
	}

	matched := false
	for _, c := range stmt.Cases {
		ok, err := r.matchSwitchCase(hasSwitchExpr, switchValue, c.Match)
		if err != nil {
			return flow{}, err
		}
		if !ok {
			continue
		}
		matched = true
		fl, err := r.execStmtList(c.Body)
		if err != nil {
			return flow{}, err
		}
		return fl, nil
	}

	if !matched && len(stmt.Default) > 0 {
		return r.execStmtList(stmt.Default)
	}
	return flow{kind: flowNone}, nil
}

func (r *Runtime) matchSwitchCase(hasSwitchExpr bool, switchValue interface{}, caseExpr *Expr) (bool, error) {
	if !hasSwitchExpr {
		condValue, err := r.eval(caseExpr)
		if err != nil {
			return false, err
		}
		return truthy(condValue), nil
	}

	if !disableSwitchCaseConstFastPath {
		if cv, ok := constExprValue(caseExpr); ok {
			return compareSwitchValue(switchValue, cv), nil
		}
	}

	condValue, err := r.eval(caseExpr)
	if err != nil {
		return false, err
	}
	return compareEq(switchValue, condValue), nil
}

func compareSwitchValue(switchValue interface{}, caseValue interface{}) bool {
	switch sv := switchValue.(type) {
	case float64:
		cv, ok := toFloat(caseValue)
		if !ok {
			return false
		}
		if math.IsNaN(sv) || math.IsNaN(cv) {
			return false
		}
		return sv == cv
	case string:
		cv, ok := caseValue.(string)
		return ok && sv == cv
	case bool:
		cv, ok := caseValue.(bool)
		return ok && sv == cv
	default:
		return compareEq(switchValue, caseValue)
	}
}

func constExprValue(expr *Expr) (interface{}, bool) {
	if expr == nil {
		return nil, false
	}
	switch expr.KOp {
	case exprKindNumber:
		return expr.Number, true
	case exprKindString:
		return expr.String, true
	case exprKindBool:
		return expr.Bool, true
	case exprKindNA:
		return math.NaN(), true
	case exprKindUnary:
		uop := expr.UOp
		if uop == unaryOpNeg || uop == unaryOpPos {
			v, ok := constExprValue(expr.Right)
			if !ok {
				return nil, false
			}
			f, ok := toFloat(v)
			if !ok {
				return nil, false
			}
			if uop == unaryOpPos {
				return f, true
			}
			return -f, true
		}
	}
	return nil, false
}

func (r *Runtime) eval(expr *Expr) (interface{}, error) {
	if expr == nil {
		return nil, nil
	}
	switch expr.KOp {
	case exprKindNumber:
		return expr.Number, nil
	case exprKindString:
		return expr.String, nil
	case exprKindBool:
		return expr.Bool, nil
	case exprKindNA:
		return math.NaN(), nil
	case exprKindIdent:
		return r.resolve(expr.Name)
	case exprKindArray:
		arr := make([]interface{}, 0, len(expr.Elems))
		for _, e := range expr.Elems {
			v, err := r.eval(e)
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		return arr, nil
	case exprKindTuple:
		out := make([]interface{}, 0, len(expr.Elems))
		for _, e := range expr.Elems {
			v, err := r.eval(e)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	case exprKindIndex:
		idxV, err := r.eval(expr.Right)
		if err != nil {
			return nil, err
		}
		idxF, _ := toFloat(idxV)
		return r.evalIndex(expr.Left, int(idxF))
	case exprKindUnary:
		rv, err := r.eval(expr.Right)
		if err != nil {
			return nil, err
		}
		uop := expr.UOp
		switch uop {
		case unaryOpNeg:
			n, _ := toFloat(rv)
			return -n, nil
		case unaryOpPos:
			n, _ := toFloat(rv)
			return n, nil
		case unaryOpNot:
			return !truthy(rv), nil
		default:
			return nil, fmt.Errorf("unsupported unary opcode: %d", uop)
		}
	case exprKindBinary:
		lv, err := r.eval(expr.Left)
		if err != nil {
			return nil, err
		}
		rv, err := r.eval(expr.Right)
		if err != nil {
			return nil, err
		}
		op := expr.BOp
		if op != binaryOpUnknown {
			return evalBinary(op, lv, rv)
		}
		return nil, fmt.Errorf("unsupported binary opcode")
	case exprKindTernary:
		cond, err := r.eval(expr.Left)
		if err != nil {
			return nil, err
		}
		if truthy(cond) {
			return r.eval(expr.Right)
		}
		return r.eval(expr.Else)
	case exprKindCall:
		return r.evalCall(expr)
	case exprKindNamedArg:
		return nil, fmt.Errorf("named argument %q cannot be evaluated outside call binding", expr.Name)
	case exprKindCiscClamp:
		if len(expr.Args) != 3 {
			return nil, fmt.Errorf("invalid cisc_clamp args")
		}
		v0, err := r.eval(expr.Args[0])
		if err != nil {
			return nil, err
		}
		v1, err := r.eval(expr.Args[1])
		if err != nil {
			return nil, err
		}
		v2, err := r.eval(expr.Args[2])
		if err != nil {
			return nil, err
		}
		x, _ := toFloat(v0)
		low, _ := toFloat(v1)
		high, _ := toFloat(v2)
		if x < low {
			return low, nil
		}
		if x > high {
			return high, nil
		}
		return x, nil
	default:
		return nil, fmt.Errorf("unsupported expression opcode")
	}
}

func (r *Runtime) evalCall(expr *Expr) (interface{}, error) {
	if expr.Left == nil || expr.Left.KOp != exprKindIdent {
		return nil, errors.New("call target must be identifier")
	}
	name := expr.Left.Name
	if strings.HasPrefix(name, "strategy.") || strings.HasPrefix(name, "plot") {
		return nil, fmt.Errorf("unsupported feature: %s", name)
	}

	useArgPool := !disableCallArgPooling
	if len(expr.Args) <= 4 {
		useArgPool = false
	}
	rawArgs, args, releaseArgs, err := r.prepareCallArgs(name, expr.Args, useArgPool)
	if err != nil {
		return nil, err
	}
	defer releaseArgs()

	if expr.BID != builtinFastUnknown {
		if v, ok, err := r.callBuiltinFast(expr.BID, rawArgs, args); ok || err != nil {
			return v, err
		}
	}

	if v, ok, err := r.callBuiltin(name, rawArgs, args); ok || err != nil {
		return v, err
	}
	if typeName, ok := splitTypeConstructorCallName(name); ok {
		if typeDef, exists := r.program.Types[typeName]; exists {
			instance, err := r.instantiateType(typeDef, rawArgs, args)
			return instance, err
		}
	}
	if fn, ok := r.program.Functions[name]; ok {
		result, err := r.callScriptFunction(fn, args)
		return result, err
	}
	if userFn, ok := r.userFns[name]; ok {
		if useArgPool {
			copied := append([]interface{}(nil), args...)
			return userFn(copied...)
		}
		result, err := userFn(args...)
		return result, err
	}
	if recvName, methodName, ok := splitMethodCallName(name); ok {
		recv, err := r.resolve(recvName)
		if err == nil {
			var methodArgs []interface{}
			var smallMethodArgs [5]interface{}
			if len(args)+1 <= len(smallMethodArgs) {
				methodArgs = smallMethodArgs[:0]
			} else {
				methodArgs = make([]interface{}, 0, len(args)+1)
			}
			methodArgs = append(methodArgs, recv)
			methodArgs = append(methodArgs, args...)
			if builtinName := methodBuiltinNameForReceiver(recv, methodName); builtinName != "" {
				if v, ok, err := r.callBuiltin(builtinName, nil, methodArgs); ok || err != nil {
					return v, err
				}
			}
			if fn, ok := r.program.Functions[methodName]; ok {
				result, err := r.callScriptFunction(fn, methodArgs)
				return result, err
			}
			if userFn, ok := r.userFns[methodName]; ok {
				result, err := userFn(methodArgs...)
				return result, err
			}
		}
	}
	return nil, fmt.Errorf("unknown function: %s", name)
}

func (r *Runtime) prepareCallArgs(name string, argExprs []*Expr, useArgPool bool) ([]*Expr, []interface{}, func(), error) {
	if !callHasNamedArgs(argExprs) {
		var args []interface{}
		var smallArgs [4]interface{}
		if useArgPool {
			args = acquireInterfaceSlice(len(argExprs))
		} else if len(argExprs) <= len(smallArgs) {
			args = smallArgs[:0]
		} else {
			args = make([]interface{}, 0, len(argExprs))
		}
		release := func() {
			if useArgPool {
				releaseInterfaceSlice(args)
			}
		}
		for _, argExpr := range argExprs {
			v, err := r.eval(argExpr)
			if err != nil {
				release()
				return nil, nil, nil, err
			}
			args = append(args, v)
		}
		return argExprs, args, release, nil
	}

	spec, ok := r.callParamSpec(name)
	if !ok {
		return nil, nil, nil, fmt.Errorf("named arguments are not supported for %s", name)
	}
	boundRaw, err := bindNamedCallArgs(name, argExprs, spec)
	if err != nil {
		return nil, nil, nil, err
	}
	var args []interface{}
	var smallArgs [4]interface{}
	if useArgPool {
		args = acquireInterfaceSlice(len(boundRaw))
	} else if len(boundRaw) <= len(smallArgs) {
		args = smallArgs[:0]
	} else {
		args = make([]interface{}, 0, len(boundRaw))
	}
	release := func() {
		if useArgPool {
			releaseInterfaceSlice(args)
		}
	}
	for _, rawArg := range boundRaw {
		if rawArg == nil {
			args = append(args, nil)
			continue
		}
		v, err := r.eval(rawArg)
		if err != nil {
			release()
			return nil, nil, nil, err
		}
		args = append(args, v)
	}
	return boundRaw, args, release, nil
}

func callHasNamedArgs(argExprs []*Expr) bool {
	for _, arg := range argExprs {
		if arg != nil && arg.Kind == "named_arg" {
			return true
		}
	}
	return false
}

func bindNamedCallArgs(name string, argExprs []*Expr, spec callParamSpec) ([]*Expr, error) {
	assigned := make([]bool, len(spec.Names))
	bound := make([]*Expr, len(spec.Names))
	nextPositional := 0
	highestAssigned := -1
	for _, arg := range argExprs {
		if arg != nil && arg.Kind == "named_arg" {
			idx := spec.indexOf(arg.Name)
			if idx < 0 {
				return nil, fmt.Errorf("unknown named argument %q for %s", arg.Name, name)
			}
			if assigned[idx] {
				return nil, fmt.Errorf("duplicate argument %q for %s", arg.Name, name)
			}
			bound[idx] = arg.NamedArgValue()
			assigned[idx] = true
			if idx > highestAssigned {
				highestAssigned = idx
			}
			continue
		}
		for nextPositional < len(spec.Names) && assigned[nextPositional] {
			nextPositional++
		}
		if nextPositional >= len(spec.Names) {
			return nil, fmt.Errorf("too many arguments for %s", name)
		}
		bound[nextPositional] = arg
		assigned[nextPositional] = true
		if nextPositional > highestAssigned {
			highestAssigned = nextPositional
		}
		nextPositional++
	}
	if highestAssigned < 0 {
		return nil, nil
	}
	return bound[:highestAssigned+1], nil
}

func (r *Runtime) callParamSpec(name string) (callParamSpec, bool) {
	if fn, ok := r.program.Functions[name]; ok {
		return callParamSpec{Names: append([]string(nil), fn.Params...)}, true
	}
	if typeName, ok := splitTypeConstructorCallName(name); ok {
		if typeDef, exists := r.program.Types[typeName]; exists {
			names := make([]string, 0, len(typeDef.Fields))
			for _, field := range typeDef.Fields {
				names = append(names, field.Name)
			}
			return callParamSpec{Names: names}, true
		}
	}
	spec, ok := builtinCallParamSpecs[name]
	return spec, ok
}

func splitMethodCallName(name string) (string, string, bool) {
	idx := strings.LastIndex(name, ".")
	if idx <= 0 || idx >= len(name)-1 {
		return "", "", false
	}
	prefix := name[:idx]
	method := name[idx+1:]
	switch prefix {
	case "math", "ta", "array", "matrix", "str", "timeframe", "session", "log":
		return "", "", false
	default:
		return prefix, method, true
	}
}

func splitTypeConstructorCallName(name string) (string, bool) {
	if !strings.HasSuffix(name, ".new") {
		return "", false
	}
	typeName := strings.TrimSuffix(name, ".new")
	if typeName == "" || strings.Contains(typeName, ".") {
		return "", false
	}
	return typeName, true
}

func methodBuiltinNameForReceiver(receiver interface{}, method string) string {
	switch receiver.(type) {
	case []interface{}:
		return "array." + method
	case *Matrix:
		return "matrix." + method
	case *pineMap:
		return "map." + method
	case string:
		return "str." + method
	default:
		return ""
	}
}

func (r *Runtime) instantiateType(typeDef TypeDef, rawArgs []*Expr, args []interface{}) (interface{}, error) {
	if len(args) > len(typeDef.Fields) {
		return nil, fmt.Errorf("%s.new expects at most %d args", typeDef.Name, len(typeDef.Fields))
	}
	instance := &customTypeInstance{TypeName: typeDef.Name, Fields: map[string]interface{}{}}
	for i, field := range typeDef.Fields {
		if i < len(rawArgs) && rawArgs[i] != nil {
			instance.Fields[field.Name] = args[i]
			continue
		}
		if field.Default != nil {
			v, err := r.eval(field.Default)
			if err != nil {
				return nil, err
			}
			instance.Fields[field.Name] = v
			continue
		}
		instance.Fields[field.Name] = math.NaN()
	}
	return instance, nil
}

func (r *Runtime) callScriptFunction(fn FunctionDef, args []interface{}) (interface{}, error) {
	useEnvPool := !disableEnvMapPooling
	var env map[string]interface{}
	if useEnvPool {
		env = acquireEnvMap()
	} else {
		env = map[string]interface{}{}
	}
	for i, p := range fn.Params {
		if i < len(args) {
			env[p] = args[i]
		} else {
			env[p] = nil
		}
	}
	r.envStack = append(r.envStack, env)
	defer func() {
		r.envStack = r.envStack[:len(r.envStack)-1]
		if useEnvPool {
			releaseEnvMap(env)
		}
	}()

	if fn.Expr != nil {
		return r.eval(fn.Expr)
	}
	var last interface{}
	hasLast := false
	for _, stmt := range fn.Body {
		fl, err := r.execStmt(stmt)
		if err != nil {
			return nil, err
		}
		if fl.kind == flowReturn {
			return fl.value, nil
		}
		if fl.kind == flowNone && fl.hasValue {
			last = fl.value
			hasLast = true
		}
	}
	if hasLast {
		return last, nil
	}
	return nil, nil
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

func (r *Runtime) evalWithOffset(expr *Expr, offset int) (interface{}, error) {
	if offset < 0 {
		return nil, errors.New("negative evaluation offset")
	}
	prev := r.evalOffset
	r.evalOffset = prev + offset
	defer func() { r.evalOffset = prev }()
	return r.eval(expr)
}

func (r *Runtime) evalIndex(left *Expr, idx int) (interface{}, error) {
	if idx < 0 {
		return nil, errors.New("negative index")
	}
	if left == nil {
		return nil, errors.New("index target is nil")
	}

	if left.KOp == exprKindIdent {
		name := left.Name
		if idx == 0 {
			return r.resolve(name)
		}
		if isPriceIdentifierName(name) {
			return r.valueAt(r.activeSymbol, name, r.evalOffset+idx), nil
		}
		if name == "bar_index" {
			value := r.effectiveBarIndex() - idx
			if value < 0 {
				return math.NaN(), nil
			}
			return float64(value), nil
		}
		if nums, ok := r.numericHistory[name]; ok {
			pos := len(nums) - r.evalOffset - idx
			if pos < 0 || pos >= len(nums) {
				return math.NaN(), nil
			}
			return nums[pos], nil
		}
		if vals, ok := r.history[name]; ok {
			pos := len(vals) - r.evalOffset - idx
			if pos < 0 || pos >= len(vals) {
				if r.declaredTypes != nil && r.declaredTypes[name] == "bool" {
					return false, nil
				}
				return math.NaN(), nil
			}
			return vals[pos], nil
		}
	}

	if left.KOp == exprKindCall && left.Left != nil && left.Left.KOp == exprKindIdent {
		callName := left.Left.Name
		if callName == "close_of" || callName == "open_of" || callName == "high_of" || callName == "low_of" {
			if len(left.Args) != 1 {
				return nil, fmt.Errorf("%s expects 1 arg", callName)
			}
			symV, err := r.eval(left.Args[0])
			if err != nil {
				return nil, err
			}
			symbol, ok := symV.(string)
			if !ok || symbol == "" {
				return nil, fmt.Errorf("%s requires non-empty symbol string", callName)
			}
			valueType := "close"
			switch callName {
			case "open_of":
				valueType = "open"
			case "high_of":
				valueType = "high"
			case "low_of":
				valueType = "low"
			}
			return r.valueAt(symbol, valueType, r.evalOffset+idx), nil
		}
		if callName == "value_of" {
			if len(left.Args) != 2 {
				return nil, errors.New("value_of expects 2 args")
			}
			symV, err := r.eval(left.Args[0])
			if err != nil {
				return nil, err
			}
			vtV, err := r.eval(left.Args[1])
			if err != nil {
				return nil, err
			}
			symbol, ok := symV.(string)
			if !ok || symbol == "" {
				return nil, errors.New("value_of requires symbol string")
			}
			valueType, ok := vtV.(string)
			if !ok || valueType == "" {
				return nil, errors.New("value_of requires value_type string")
			}
			return r.valueAt(symbol, valueType, r.evalOffset+idx), nil
		}
	}

	leftVal, err := r.eval(left)
	if err != nil {
		return nil, err
	}
	if _, ok := leftVal.([]interface{}); ok {
		return indexValue(leftVal, idx, r.barIndex)
	}

	historical, err := r.evalWithOffset(left, idx)
	if err == nil {
		return historical, nil
	}

	return indexValue(leftVal, idx, r.barIndex)
}

func evalBinary(op uint8, lv interface{}, rv interface{}) (interface{}, error) {
	return evalBinaryByOpcode(op, lv, rv)
}

func evalBinaryArithmeticFloatByOpcode(op uint8, a, b float64) float64 {
	switch op {
	case binaryOpAdd:
		return a + b
	case binaryOpSub:
		return a - b
	case binaryOpMul:
		return a * b
	case binaryOpDiv:
		if b == 0 {
			return math.NaN()
		}
		return a / b
	case binaryOpMod:
		if b == 0 {
			return math.NaN()
		}
		return math.Mod(a, b)
	default:
		return math.NaN()
	}
}

func evalBinaryByOpcode(op uint8, lv interface{}, rv interface{}) (interface{}, error) {
	switch op {
	case binaryOpOr:
		return truthy(lv) || truthy(rv), nil
	case binaryOpAnd:
		return truthy(lv) && truthy(rv), nil
	case binaryOpEq:
		return compareEq(lv, rv), nil
	case binaryOpNeq:
		return !compareEq(lv, rv), nil
	case binaryOpLT, binaryOpLTE, binaryOpGT, binaryOpGTE, binaryOpAdd, binaryOpSub, binaryOpMul, binaryOpDiv, binaryOpMod:
		lf, _ := toFloat(lv)
		rf, _ := toFloat(rv)
		switch op {
		case binaryOpLT:
			return lf < rf, nil
		case binaryOpLTE:
			return lf <= rf, nil
		case binaryOpGT:
			return lf > rf, nil
		case binaryOpGTE:
			return lf >= rf, nil
		case binaryOpAdd:
			if ls, ok := lv.(string); ok {
				return ls + toString(rv), nil
			}
			if rs, ok := rv.(string); ok {
				return toString(lv) + rs, nil
			}
			return lf + rf, nil
		case binaryOpSub:
			return lf - rf, nil
		case binaryOpMul:
			return lf * rf, nil
		case binaryOpDiv:
			if rf == 0 {
				return math.NaN(), nil
			}
			return lf / rf, nil
		case binaryOpMod:
			if rf == 0 {
				return math.NaN(), nil
			}
			return math.Mod(lf, rf), nil
		}
	}
	return nil, fmt.Errorf("unsupported binary opcode %d", op)
}

func indexValue(v interface{}, idx int, bar int) (interface{}, error) {
	if idx < 0 {
		return nil, errors.New("negative index")
	}
	switch arr := v.(type) {
	case []interface{}:
		if idx >= len(arr) {
			return nil, fmt.Errorf("index out of range: %d", idx)
		}
		return arr[idx], nil
	default:
		return nil, fmt.Errorf("value is not indexable at bar %d", bar)
	}
}

func compareEq(a, b interface{}) bool {
	af, aok := toFloat(a)
	bf, bok := toFloat(b)
	if aok && bok {
		if math.IsNaN(af) || math.IsNaN(bf) {
			return false
		}
		return af == bf
	}
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

func truthy(v interface{}) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return t != ""
	default:
		f, ok := toFloat(v)
		if !ok {
			return true
		}
		return !math.IsNaN(f) && f != 0
	}
}

func toFloat(v interface{}) (float64, bool) {
	switch t := v.(type) {
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case bool:
		if t {
			return 1, true
		}
		return 0, true
	case nil:
		return math.NaN(), true
	default:
		return 0, false
	}
}

func toString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return "na"
	default:
		return fmt.Sprintf("%v", t)
	}
}
