// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"time"
)

func roundToTick(n, tick float64) float64 {
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return n
	}
	a, b := new(big.Rat), new(big.Rat)
	a.SetString(strconv.FormatFloat(n, 'f', -1, 64))
	b.SetString(strconv.FormatFloat(tick, 'f', -1, 64))
	a.Quo(a, b)
	a.Add(a, big.NewRat(1, 2))
	q, rem := new(big.Int), new(big.Int)
	q.QuoRem(a.Num(), a.Denom(), rem)
	if a.Sign() < 0 && rem.Sign() != 0 {
		q.Sub(q, big.NewInt(1))
	}
	a.Mul(new(big.Rat).SetInt(q), b)
	out, _ := a.Float64()
	return out
}

func isCalendarFunction(name string) bool {
	switch name {
	case "year", "month", "weekofyear", "dayofmonth", "dayofweek", "hour", "minute", "second":
		return true
	}
	return false
}

func (r *Runtime) calendarFunction(name string, args []interface{}) (interface{}, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, fmt.Errorf("%s expects a timestamp and optional timezone", name)
	}
	n, ok := toFloat(args[0])
	if !ok || math.IsInf(n, 0) {
		return nil, fmt.Errorf("%s timestamp must be numeric", name)
	}
	if math.IsNaN(n) {
		return math.NaN(), nil
	}
	loc := r.exchangeTimezone()
	if len(args) == 2 {
		s, ok := args[1].(string)
		if !ok {
			return nil, fmt.Errorf("%s timezone must be a string", name)
		}
		var err error
		loc, err = checkedTimezone(s)
		if err != nil {
			return nil, err
		}
	}
	t := time.UnixMilli(int64(n)).In(loc)
	var out int
	switch name {
	case "year":
		out = t.Year()
	case "month":
		out = int(t.Month())
	case "weekofyear":
		_, out = t.ISOWeek()
	case "dayofmonth":
		out = t.Day()
	case "dayofweek":
		out = int(t.Weekday()) + 1
	case "hour":
		out = t.Hour()
	case "minute":
		out = t.Minute()
	case "second":
		out = t.Second()
	}
	return float64(out), nil
}

func (r *Runtime) builtinFormatTime(args []interface{}) (interface{}, error) {
	if len(args) < 1 || len(args) > 3 {
		return nil, fmt.Errorf("str.format_time expects time[, format[, timezone]]")
	}
	n, ok := toFloat(args[0])
	if !ok || math.IsInf(n, 0) {
		return nil, fmt.Errorf("str.format_time requires a timestamp")
	}
	if math.IsNaN(n) {
		return "NaN", nil
	}
	pattern := "yyyy-MM-dd'T'HH:mm:ss'Z'"
	loc := r.exchangeTimezone()
	if len(args) > 1 {
		s, ok := args[1].(string)
		if !ok {
			return nil, fmt.Errorf("str.format_time format must be a string")
		}
		pattern = s
	}
	if len(args) > 2 {
		s, ok := args[2].(string)
		if !ok {
			return nil, fmt.Errorf("str.format_time timezone must be a string")
		}
		var err error
		loc, err = checkedTimezone(s)
		if err != nil {
			return nil, err
		}
	}
	return formatPineTime(time.UnixMilli(int64(n)).In(loc), pattern)
}

type vwapBar struct {
	pv, volume, ppv float64
	started         bool
	date            string
}
type vwapState struct {
	bars    map[int]vwapBar
	outputs map[int]interface{}
	indices []int
}

func (r *Runtime) builtinVWAP(raw []*Expr, args []interface{}) (interface{}, error) {
	if len(args) > 3 {
		return nil, fmt.Errorf("ta.vwap expects source[, anchor[, stdev_mult]]")
	}
	idx := r.effectiveBarIndex()
	if idx < 0 {
		return math.NaN(), nil
	}
	key := fmt.Sprintf("vwap:%s:%s", r.activeSymbol, r.scriptCallPath)
	if len(raw) > 0 {
		key += fmt.Sprintf(":%p", raw[0])
	}
	st, ok := r.indicatorState[key].(*vwapState)
	if !ok {
		st = &vwapState{bars: map[int]vwapBar{}, outputs: map[int]interface{}{}}
		r.indicatorState[key] = st
	}
	if r.evalOffset > 0 {
		if out, exists := st.outputs[idx]; exists {
			return out, nil
		}
	}
	previous := vwapBar{}
	pos := sort.SearchInts(st.indices, idx)
	if pos > 0 {
		previous = st.bars[st.indices[pos-1]]
	}
	current := previous
	open, _, _ := r.barTimes()
	date := open.In(r.exchangeTimezone()).Format("2006-01-02")
	reset := date != previous.date
	if len(args) >= 2 {
		b, ok := args[1].(bool)
		if !ok {
			return nil, fmt.Errorf("ta.vwap anchor must be bool")
		}
		reset = b
	}
	if reset {
		current = vwapBar{started: true}
	}
	current.date = date
	var price float64
	if len(args) > 0 {
		var ok bool
		price, ok = toFloat(args[0])
		if !ok {
			return nil, fmt.Errorf("ta.vwap source must be numeric")
		}
	} else {
		h, l, c := r.exactSeriesValue("high", idx), r.exactSeriesValue("low", idx), r.exactSeriesValue("close", idx)
		price = (h + l + c) / 3
	}
	volume := r.exactSeriesValue("volume", idx)
	if current.started {
		current.pv += price * volume
		current.volume += volume
		current.ppv += price * price * volume
	}
	vwap := math.NaN()
	if current.started && current.volume != 0 {
		vwap = current.pv / current.volume
	}
	var out interface{} = vwap
	if len(args) == 3 {
		mult, ok := toFloat(args[2])
		if !ok {
			return nil, fmt.Errorf("ta.vwap stdev_mult must be numeric")
		}
		deviation := math.Sqrt(math.Max(0, current.ppv/current.volume-vwap*vwap))
		out = []interface{}{vwap, vwap + mult*deviation, vwap - mult*deviation}
	}
	st.bars[idx], st.outputs[idx] = current, out
	st.indices = insertStateIndex(st.indices, idx)
	return out, nil
}
