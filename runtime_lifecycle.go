// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"math"
	"time"
)

// Release returns all internal memory held by the Runtime to the object pools.
// After Release the Runtime is in an invalid state and must not be used.
// Release is safe to call on a nil Runtime.
func (r *Runtime) Release() {
	if r == nil {
		return
	}
	r.program = Program{}
	r.userFns = nil
	r.userFnParamSpecs = nil
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

// SetBarIndex advances the runtime to the given zero-based bar index.
// This updates the cached OHLCV values and the barstate.islast flag.
// It is called automatically by Engine during ExecuteWithRuntime, but can
// also be called manually for step-through inspection or replay.
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
