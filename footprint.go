// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"fmt"
	"math"
	"strings"
)

// FootprintLevel is executed buy/sell volume at a price, not inferred from the
// candle's direction. Providers must supply genuine order-flow data.
type FootprintLevel struct{ Price, BuyVolume, SellVolume float64 }
type FootprintProvider interface {
	GetFootprintLevels(symbol, timeframe string, bar BarTime) ([]FootprintLevel, error)
}

func (e *Engine) SetFootprintProvider(provider FootprintProvider) { e.footprintProvider = provider }
func (e *Engine) footprintSource(catalog *providerCatalog) func(string, string, BarTime) ([]FootprintLevel, error) {
	explicit := e.footprintProvider
	security := e.securityProvider
	return func(symbol, tf string, bar BarTime) ([]FootprintLevel, error) {
		if explicit != nil {
			return explicit.GetFootprintLevels(symbol, tf, bar)
		}
		if p, ok := security.(FootprintProvider); ok {
			return p.GetFootprintLevels(symbol, tf, bar)
		}
		provider := catalog.providerBySymbol[symbol]
		if provider == nil {
			if id, err := ParseTickerID(symbol); err == nil {
				provider = catalog.providerBySymbol[id.Symbol]
			}
		}
		if p, ok := provider.(FootprintProvider); ok {
			return p.GetFootprintLevels(symbol, tf, bar)
		}
		return nil, nil
	}
}

type volumeRow struct {
	down, up, buy, sell         float64
	buyImbalance, sellImbalance bool
}
type footprintValue struct {
	rows               []*volumeRow
	poc, val, vah      int
	buy, sell, rowSize float64
	first              int
}
type footprintLevels struct{ values []FootprintLevel }

func init() {
	for _, method := range strings.Fields("buy_volume sell_volume total_volume delta poc vah val rows get_row_by_price") {
		additionalBuiltinNames["footprint."+method] = struct{}{}
	}
	for _, method := range strings.Fields("up_price down_price buy_volume sell_volume total_volume delta has_buy_imbalance has_sell_imbalance") {
		additionalBuiltinNames["volume_row."+method] = struct{}{}
	}
	additionalBuiltinNames["footprint"] = struct{}{}
	additionalBuiltinNames["volume_row"] = struct{}{}
}
func (r *Runtime) evalFootprintRequest(call *Expr) (interface{}, error) {
	raw, err := bindNamedCallArgs("request.footprint", call.Args, callParamSpec{Names: []string{"ticks_per_row", "va_percent", "imbalance_percent"}, Required: 1})
	if err != nil {
		return nil, err
	}
	args := []interface{}{nil, 70.0, 300.0}
	for i, e := range raw {
		if e != nil {
			v, err := r.eval(e)
			if err != nil {
				return nil, err
			}
			args[i] = v
		}
	}
	ticks, err := integerArg(args[0], "ticks_per_row", 1, maxArrayNewSize)
	if err != nil {
		return nil, err
	}
	va, ok := toFloat(args[1])
	if !ok || math.IsNaN(va) || math.IsInf(va, 0) || va < 0 || va > 100 {
		return nil, fmt.Errorf("va_percent must be between 0 and 100")
	}
	imbalance, ok := toFloat(args[2])
	if !ok || math.IsNaN(imbalance) || math.IsInf(imbalance, 0) || imbalance < 0 {
		return nil, fmt.Errorf("imbalance_percent must be nonnegative")
	}
	if r.footprintLoader == nil {
		return math.NaN(), nil
	}
	idx := r.effectiveBarIndex()
	if idx < 0 {
		return math.NaN(), nil
	}
	key := fmt.Sprintf("footprint:levels:%s:%s:%d", r.activeSymbol, r.timeframe, idx)
	levels, ok := r.indicatorState[key].(footprintLevels)
	if !ok {
		open, close, trading := r.barTimes()
		values, err := r.footprintLoader(r.activeSymbol, r.timeframe, BarTime{Open: open, Close: close, TradingDay: trading})
		if err != nil {
			return nil, fmt.Errorf("request.footprint provider: %w", err)
		}
		if len(values) > maxArrayNewSize {
			return nil, fmt.Errorf("footprint level limit exceeded")
		}
		levels = footprintLevels{append([]FootprintLevel(nil), values...)}
		r.indicatorState[key] = levels
	}
	if len(levels.values) == 0 {
		return math.NaN(), nil
	}
	tick, ok := toFloat(r.rootNamespaces["syminfo"].(map[string]interface{})["mintick"])
	if !ok || math.IsNaN(tick) || math.IsInf(tick, 0) || tick <= 0 {
		return nil, fmt.Errorf("request.footprint requires syminfo.mintick")
	}
	cache := fmt.Sprintf("footprint:built:%s:%s:%d:%d:%g:%g", r.activeSymbol, r.timeframe, idx, ticks, va, imbalance)
	if value, ok := r.indicatorState[cache]; ok {
		return value, nil
	}
	value, err := buildFootprint(levels.values, tick, ticks, va, imbalance, r.exactSeriesValue("low", idx), r.exactSeriesValue("high", idx))
	if err != nil {
		return nil, err
	}
	if value == nil {
		return math.NaN(), nil
	}
	r.indicatorState[cache] = value
	return value, nil
}

func buildFootprint(levels []FootprintLevel, tick float64, ticks int, va, imbalance, low, high float64) (*footprintValue, error) {
	if tick <= 0 || math.IsInf(tick, 0) || math.IsNaN(tick) || ticks < 1 {
		return nil, fmt.Errorf("invalid footprint row size")
	}
	size := tick * float64(ticks)
	if math.IsInf(size, 0) {
		return nil, fmt.Errorf("footprint row size overflow")
	}
	index := func(price float64) (int, error) {
		n := math.Floor(math.Floor(price/tick+0.5) / float64(ticks))
		if math.IsNaN(n) || math.IsInf(n, 0) || math.Abs(n) > 1<<53 {
			return 0, fmt.Errorf("footprint price exceeds supported tick grid")
		}
		return int(n), nil
	}
	byRow := map[int][2]float64{}
	first, last := 0, -1
	for _, l := range levels {
		if math.IsNaN(l.Price) || math.IsInf(l.Price, 0) || math.IsNaN(l.BuyVolume) || math.IsInf(l.BuyVolume, 0) || math.IsNaN(l.SellVolume) || math.IsInf(l.SellVolume, 0) || l.BuyVolume < 0 || l.SellVolume < 0 {
			return nil, fmt.Errorf("invalid footprint level")
		}
		row, err := index(l.Price)
		if err != nil {
			return nil, err
		}
		if len(byRow) == 0 {
			first, last = row, row
		} else {
			if row < first {
				first = row
			}
			if row > last {
				last = row
			}
		}
		vol := byRow[row]
		vol[0] += l.BuyVolume
		vol[1] += l.SellVolume
		byRow[row] = vol
	}
	if len(byRow) == 0 {
		return nil, nil
	}
	if !math.IsNaN(low) && !math.IsNaN(high) && low <= high {
		lo, err := index(low)
		if err != nil {
			return nil, err
		}
		// A high exactly on an upper grid edge does not open another empty row.
		hiTick := math.Floor(high/tick+0.5) - 1
		hi := math.Floor(hiTick / float64(ticks))
		if math.IsInf(hi, 0) || math.Abs(hi) > 1<<53 {
			return nil, fmt.Errorf("footprint range exceeds grid")
		}
		if lo < first {
			first = lo
		}
		if int(hi) > last {
			last = int(hi)
		}
	}
	if last-first+1 > 2000 {
		return nil, nil
	}
	fp := &footprintValue{rows: make([]*volumeRow, last-first+1), first: first, rowSize: size}
	middle := float64(len(fp.rows)-1) / 2
	best := -1.0
	for i := range fp.rows {
		vol := byRow[first+i]
		fp.rows[i] = &volumeRow{down: float64(first+i) * size, up: float64(first+i+1) * size, buy: vol[0], sell: vol[1]}
		if math.IsInf(fp.rows[i].down, 0) || math.IsInf(fp.rows[i].up, 0) {
			return nil, fmt.Errorf("footprint row price overflow")
		}
		fp.buy += vol[0]
		fp.sell += vol[1]
		total := vol[0] + vol[1]
		if total > best+1e-10 || math.Abs(total-best) <= 1e-10 && math.Abs(float64(i)-middle) < math.Abs(float64(fp.poc)-middle)-1e-10 {
			best = total
			fp.poc = i
		}
	}
	if math.IsInf(fp.buy+fp.sell, 0) {
		return nil, fmt.Errorf("footprint volume overflow")
	}
	fp.val, fp.vah = fp.poc, fp.poc
	sum := (fp.rows[fp.poc].buy + fp.rows[fp.poc].sell)
	limit := (fp.buy+fp.sell)*va/100 + 0.01
	for fp.val > 0 || fp.vah+1 < len(fp.rows) {
		up, down := math.Inf(-1), math.Inf(-1)
		if fp.vah+1 < len(fp.rows) {
			r := fp.rows[fp.vah+1]
			up = r.buy + r.sell
		}
		if fp.val > 0 {
			r := fp.rows[fp.val-1]
			down = r.buy + r.sell
		}
		above := up > down
		if math.Abs(up-down) <= 1e-10 {
			above = fp.vah+1-fp.poc <= fp.poc-fp.val+1
		}
		next := down
		if above {
			next = up
		}
		if sum+next > limit {
			break
		}
		sum += next
		if above {
			fp.vah++
		} else {
			fp.val--
		}
	}
	ratio := math.Max(1, imbalance/100)
	for i, row := range fp.rows {
		row.buyImbalance = i > 0 && row.buy > 0 && row.buy+1e-10 >= ratio*fp.rows[i-1].sell
		row.sellImbalance = i+1 < len(fp.rows) && row.sell > 0 && row.sell+1e-10 >= ratio*fp.rows[i+1].buy
	}
	return fp, nil
}

func footprintBuiltin(name string, args []interface{}) (interface{}, error) {
	required := 1
	if name == "footprint.get_row_by_price" {
		required = 2
	}
	if len(args) != required {
		return nil, fmt.Errorf("%s expects %d args", name, required)
	}
	if name == "footprint" || name == "volume_row" {
		if isNA(args[0]) {
			return math.NaN(), nil
		}
		if name == "footprint" {
			if value, ok := args[0].(*footprintValue); ok {
				return value, nil
			}
		} else {
			if value, ok := args[0].(*volumeRow); ok {
				return value, nil
			}
		}
		return nil, fmt.Errorf("%s cast requires a matching value or na", name)
	}
	if strings.HasPrefix(name, "volume_row.") {
		row, ok := args[0].(*volumeRow)
		if !ok || row == nil {
			if strings.Contains(name, "has_") {
				return false, nil
			}
			return math.NaN(), nil
		}
		switch name {
		case "volume_row.up_price":
			return row.up, nil
		case "volume_row.down_price":
			return row.down, nil
		case "volume_row.buy_volume":
			return row.buy, nil
		case "volume_row.sell_volume":
			return row.sell, nil
		case "volume_row.total_volume":
			return row.buy + row.sell, nil
		case "volume_row.delta":
			return row.buy - row.sell, nil
		case "volume_row.has_buy_imbalance":
			return row.buyImbalance, nil
		case "volume_row.has_sell_imbalance":
			return row.sellImbalance, nil
		}
	}
	fp, ok := args[0].(*footprintValue)
	if !ok || fp == nil {
		return math.NaN(), nil
	}
	switch name {
	case "footprint.buy_volume":
		return fp.buy, nil
	case "footprint.sell_volume":
		return fp.sell, nil
	case "footprint.total_volume":
		return fp.buy + fp.sell, nil
	case "footprint.delta":
		return fp.buy - fp.sell, nil
	case "footprint.poc":
		return fp.rows[fp.poc], nil
	case "footprint.vah":
		return fp.rows[fp.vah], nil
	case "footprint.val":
		return fp.rows[fp.val], nil
	case "footprint.rows":
		out := &pineArray{}
		for _, row := range fp.rows {
			out.items = append(out.items, row)
		}
		return out, nil
	case "footprint.get_row_by_price":
		price, ok := toFloat(args[1])
		if !ok || math.IsNaN(price) || math.IsInf(price, 0) {
			return math.NaN(), nil
		}
		index := math.Floor(price/fp.rowSize+1e-6) - float64(fp.first)
		if index < 0 || index >= float64(len(fp.rows)) {
			return math.NaN(), nil
		}
		return fp.rows[int(index)], nil
	}
	return nil, fmt.Errorf("unknown footprint API %s", name)
}
