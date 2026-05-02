// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
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

// RuntimeSnapshot is a point-in-time view of the execution state produced by Runtime.Snapshot.
// BarIndex is the zero-based index of the current bar. LastValue is the numeric result
// of the last evaluated expression. Symbols and SeriesKeys describe the data in use.
// Variables holds the current value of every top-level variable and function parameter
// in scope, keyed by name.
type RuntimeSnapshot struct {
	BarIndex        int
	LastValue       float64
	ActiveSymbol    string
	ActiveValueType string
	Symbols         []string
	SeriesKeys      []string
	Variables       map[string]interface{}
}

// Runtime represents the execution state of a compiled Pine Script program. It is
// produced by Engine.ExecuteWithRuntime and must not be mutated by callers. After
// a Runtime is no longer needed, call Release to return pooled memory.
type Runtime struct {
	program          Program
	userFns          map[string]UserFunction
	userFnParamSpecs map[string]callParamSpec

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
	Names    []string
	Required int
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
	"alert":        {Names: []string{"message", "freq"}, Required: 1},
	"barcolor":     {Names: []string{"color", "offset", "editable", "show_last", "title", "display"}},
	"box.new":      {Names: []string{"left", "top", "right", "bottom", "border_color", "border_width", "border_style", "extend", "xloc", "bgcolor", "text", "text_size", "text_color", "text_halign", "text_valign", "text_wrap", "force_overlay"}, Required: 4},
	"color.new":    {Names: []string{"color", "transp"}, Required: 1},
	"indicator":    {Names: []string{"title", "shorttitle", "overlay", "format", "precision", "scale", "max_bars_back", "timeframe", "timeframe_gaps", "explicit_plot_zorder", "max_lines_count", "max_labels_count", "max_boxes_count", "max_polylines_count", "dynamic_requests", "behind_chart"}},
	"input":        {Names: []string{"defval", "title", "tooltip", "inline", "group", "display"}},
	"input.bool":   {Names: []string{"defval", "title", "tooltip", "inline", "group", "display"}},
	"input.color":  {Names: []string{"defval", "title", "tooltip", "inline", "group", "display"}},
	"input.float":  {Names: []string{"defval", "title", "tooltip", "inline", "group", "display", "step", "minval", "maxval", "confirm"}},
	"input.int":    {Names: []string{"defval", "title", "tooltip", "inline", "group", "display", "step", "minval", "maxval", "confirm"}},
	"input.source": {Names: []string{"defval", "title", "tooltip", "inline", "group", "display"}},
	"input.string": {Names: []string{"defval", "title", "tooltip", "inline", "group", "display", "options", "confirm"}},
	"table.cell":   {Names: []string{"table_id", "column", "row", "text", "width", "height", "text_color", "text_halign", "text_valign", "text_size", "bgcolor", "tooltip", "text_font_family"}},
	"table.new":    {Names: []string{"position", "columns", "rows", "bgcolor", "frame_color", "frame_width", "border_color", "border_width"}, Required: 3},
}
