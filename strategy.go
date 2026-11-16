// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"fmt"
	"math"
	"strings"
	"time"
)

type strategyTrade struct {
	uid                                                          int
	entryID, entryComment, exitID, exitComment                   string
	dir                                                          int
	qty, entry, exit, entryFee, exitFee, profit, runup, drawdown float64
	entryBar, exitBar                                            int
	entryTime, exitTime                                          time.Time
}
type strategyOrder struct {
	defaultQty                            bool
	id, kind, from, comment, oca, ocaType string
	dir, bar                              int
	qty, percent                          float64
	limit, stop, profit, loss             *float64
	triggered                             bool
	intended                              map[int]bool
}
type strategyState struct {
	closeRule                                                                string
	minContract                                                              float64
	initial, cash, point, tick, mark, slippage, commission                   float64
	qtyType, commissionType, currency, allow                                 string
	defaultQty, maxPosition                                                  float64
	pyramiding                                                               int
	processClose                                                             bool
	orders                                                                   []*strategyOrder
	open, closed                                                             []*strategyTrade
	nextTrade                                                                int
	net, grossProfit, grossLoss, winningPercent, losingPercent, totalPercent float64
	wins, losses, evens                                                      int
	peak, trough, maxDD, maxRunup, maxAll, maxLong, maxShort                 float64
}

func strategyConstants() map[string]interface{} {
	return map[string]interface{}{"long": float64(1), "short": float64(-1), "fixed": "fixed", "cash": "cash", "percent_of_equity": "percent_of_equity",
		"commission": map[string]interface{}{"percent": "percent", "cash_per_order": "cash_per_order", "cash_per_contract": "cash_per_contract"},
		"direction":  map[string]interface{}{"all": "all", "long": "long", "short": "short"}, "oca": map[string]interface{}{"none": "none", "cancel": "cancel", "reduce": "reduce"}}
}

var strategyDeclarationParams = []string{"title", "shorttitle", "overlay", "format", "precision", "scale", "pyramiding", "calc_on_order_fills", "calc_on_every_tick", "max_bars_back", "backtest_fill_limits_assumption", "default_qty_type", "default_qty_value", "initial_capital", "currency", "slippage", "commission_type", "commission_value", "process_orders_on_close", "close_entries_rule", "margin_long", "margin_short", "explicit_plot_zorder", "max_lines_count", "max_labels_count", "max_boxes_count", "max_polylines_count", "calc_bars_count", "risk_free_rate", "use_bar_magnifier", "fill_orders_on_standard_ohlc", "dynamic_requests", "behind_chart"}

func (r *Runtime) strategyArgs(call *Expr, names []string, required int) (map[string]interface{}, error) {
	raw, err := bindNamedCallArgs(call.Left.Name, call.Args, callParamSpec{Names: names, Required: required})
	if err != nil {
		return nil, err
	}
	values := map[string]interface{}{}
	for i, e := range raw {
		if e != nil {
			value, err := r.eval(e)
			if err != nil {
				return nil, err
			}
			values[names[i]] = value
		}
	}
	return values, nil
}
func strategyNumber(values map[string]interface{}, key string, def float64) (float64, error) {
	value, exists := values[key]
	if !exists || isNA(value) {
		return def, nil
	}
	if _, b := value.(bool); b {
		return 0, fmt.Errorf("strategy %s must be numeric", key)
	}
	n, ok := toFloat(value)
	if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, fmt.Errorf("strategy %s must be finite numeric", key)
	}
	return n, nil
}
func strategyText(values map[string]interface{}, key, def string) (string, error) {
	value, exists := values[key]
	if !exists {
		return def, nil
	}
	s, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("strategy %s must be a string", key)
	}
	return s, nil
}
func strategyBool(values map[string]interface{}, key string) (bool, error) {
	value, exists := values[key]
	if !exists {
		return false, nil
	}
	b, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("strategy %s must be bool", key)
	}
	return b, nil
}
func (r *Runtime) initStrategy(call *Expr) (interface{}, error) {
	if r.strategy != nil {
		return nil, nil
	}
	values, err := r.strategyArgs(call, strategyDeclarationParams, 1)
	if err != nil {
		return nil, err
	}
	if _, err := strategyText(values, "title", ""); err != nil {
		return nil, err
	}
	for _, name := range []string{"calc_on_order_fills", "calc_on_every_tick", "use_bar_magnifier", "fill_orders_on_standard_ohlc"} {
		enabled, err := strategyBool(values, name)
		if err != nil {
			return nil, err
		}
		if enabled {
			return nil, fmt.Errorf("native strategy does not yet support %s", name)
		}
	}
	for _, name := range []string{"margin_long", "margin_short"} {
		v, err := strategyNumber(values, name, 100)
		if err != nil {
			return nil, err
		}
		if v != 0 {
			return nil, fmt.Errorf("native strategy margin liquidation is not implemented; explicitly set %s = 0", name)
		}
	}
	for _, name := range []string{"backtest_fill_limits_assumption", "calc_bars_count"} {
		n, err := strategyNumber(values, name, 0)
		if err != nil {
			return nil, err
		}
		if n != 0 {
			return nil, fmt.Errorf("native strategy does not yet support %s", name)
		}
	}
	for _, vt := range []string{"open", "high", "low", "close"} {
		if !r.valueTypesBySymbol[r.activeSymbol][vt] {
			return nil, fmt.Errorf("native strategy requires provider OHLC data (%s missing)", vt)
		}
	}
	info := r.rootNamespaces["syminfo"].(map[string]interface{})
	tick, ok := toFloat(info["mintick"])
	if !ok || math.IsNaN(tick) || tick <= 0 {
		return nil, fmt.Errorf("native strategy requires syminfo.mintick")
	}
	point, ok := toFloat(info["pointvalue"])
	if !ok || math.IsNaN(point) || math.IsInf(point, 0) || point <= 0 {
		return nil, fmt.Errorf("native strategy requires syminfo.pointvalue")
	}
	capital, err := strategyNumber(values, "initial_capital", 1e6)
	if err != nil {
		return nil, err
	}
	if capital <= 0 {
		return nil, fmt.Errorf("initial_capital must be positive")
	}
	s := &strategyState{initial: capital, cash: capital, point: point, tick: tick, allow: "all", peak: capital, trough: capital}
	s.defaultQty, err = strategyNumber(values, "default_qty_value", 1)
	if err != nil || s.defaultQty < 0 {
		return nil, fmt.Errorf("invalid default_qty_value")
	}
	s.qtyType, err = strategyText(values, "default_qty_type", "fixed")
	if err != nil {
		return nil, err
	}
	if s.qtyType != "fixed" && s.qtyType != "cash" && s.qtyType != "percent_of_equity" {
		return nil, fmt.Errorf("invalid default_qty_type")
	}
	if step, ok := toFloat(info["mincontract"]); ok && !math.IsNaN(step) && !math.IsInf(step, 0) && step > 0 {
		s.minContract = step
	}
	if s.qtyType != "fixed" && s.minContract == 0 {
		return nil, fmt.Errorf("cash/percent strategy sizing requires syminfo.mincontract")
	}
	s.commissionType, err = strategyText(values, "commission_type", "percent")
	if err != nil {
		return nil, err
	}
	if s.commissionType != "percent" && s.commissionType != "cash_per_order" && s.commissionType != "cash_per_contract" {
		return nil, fmt.Errorf("invalid commission_type")
	}
	s.commission, err = strategyNumber(values, "commission_value", 0)
	if err != nil || s.commission < 0 {
		return nil, fmt.Errorf("invalid commission_value")
	}
	s.slippage, err = strategyNumber(values, "slippage", 0)
	if err != nil || s.slippage < 0 || s.slippage != math.Trunc(s.slippage) {
		return nil, fmt.Errorf("slippage must be a nonnegative integer")
	}
	pyramid, err := strategyNumber(values, "pyramiding", 0)
	if err != nil || pyramid < 0 || pyramid > 1000 || pyramid != math.Trunc(pyramid) {
		return nil, fmt.Errorf("invalid pyramiding")
	}
	s.pyramiding = int(math.Max(1, pyramid))
	s.processClose, err = strategyBool(values, "process_orders_on_close")
	if err != nil {
		return nil, err
	}
	s.currency, err = strategyText(values, "currency", "NONE")
	if err != nil {
		return nil, err
	}
	if s.currency == "NONE" {
		s.currency, _ = info["currency"].(string)
	}
	if c, _ := info["currency"].(string); c != "" && s.currency != "" && c != s.currency {
		return nil, fmt.Errorf("cross-currency broker accounting is not implemented; use the symbol currency")
	}
	s.mark = r.exactSeriesValue("close", r.effectiveBarIndex())
	s.closeRule, err = strategyText(values, "close_entries_rule", "FIFO")
	if err != nil {
		return nil, err
	}
	if s.closeRule != "FIFO" && s.closeRule != "ANY" {
		return nil, fmt.Errorf("close_entries_rule must be FIFO or ANY")
	}
	for _, vt := range []string{"open", "high", "low", "close"} {
		value := r.exactSeriesValue(vt, r.effectiveBarIndex())
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, fmt.Errorf("strategy has invalid initial %s", vt)
		}
	}
	r.strategy = s
	return nil, nil
}
func (r *Runtime) evalStrategyCall(call *Expr) (interface{}, error) {
	name := call.Left.Name
	if name == "strategy" {
		return r.initStrategy(call)
	}
	s := r.strategy
	if s == nil {
		return nil, fmt.Errorf("unsupported feature: %s requires strategy() declaration", name)
	}
	if strings.HasPrefix(name, "strategy.closedtrades.") || strings.HasPrefix(name, "strategy.opentrades.") {
		return r.strategyTradeGetter(call)
	}
	if name == "strategy.convert_to_account" || name == "strategy.convert_to_symbol" {
		values, err := r.strategyArgs(call, []string{"value"}, 1)
		if err != nil {
			return nil, err
		}
		n, err := strategyNumber(values, "value", math.NaN())
		if err != nil {
			return nil, err
		}
		symbolCurrency, _ := r.rootNamespaces["syminfo"].(map[string]interface{})["currency"].(string)
		from, to := symbolCurrency, s.currency
		if name == "strategy.convert_to_symbol" {
			from, to = to, from
		}
		if from == to {
			return n, nil
		}
		return nil, fmt.Errorf("native strategy cross-currency conversion is not implemented")
	}
	if name == "strategy.default_entry_qty" {
		values, err := r.strategyArgs(call, []string{"fill_price"}, 1)
		if err != nil {
			return nil, err
		}
		price, err := strategyNumber(values, "fill_price", 0)
		if err != nil || price <= 0 {
			return nil, fmt.Errorf("fill_price must be positive")
		}
		return s.orderQty(price), nil
	}
	if name == "strategy.risk.allow_entry_in" {
		values, err := r.strategyArgs(call, []string{"value"}, 1)
		if err != nil {
			return nil, err
		}
		dir, err := strategyText(values, "value", "all")
		if err != nil {
			return nil, err
		}
		if dir != "all" && dir != "long" && dir != "short" {
			return nil, fmt.Errorf("invalid allowed direction")
		}
		s.allow = dir
		return nil, nil
	}
	if name == "strategy.risk.max_position_size" {
		values, err := r.strategyArgs(call, []string{"contracts"}, 1)
		if err != nil {
			return nil, err
		}
		n, err := strategyNumber(values, "contracts", 0)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("contracts must be positive")
		}
		s.maxPosition = n
		return nil, nil
	}
	if name == "strategy.cancel_all" {
		if len(call.Args) > 0 {
			return nil, fmt.Errorf("strategy.cancel_all expects no args")
		}
		s.orders = nil
		return nil, nil
	}
	if name == "strategy.cancel" {
		values, err := r.strategyArgs(call, []string{"id"}, 1)
		if err != nil {
			return nil, err
		}
		id, err := strategyText(values, "id", "")
		if err != nil {
			return nil, err
		}
		out := s.orders[:0]
		for _, o := range s.orders {
			if o.id != id {
				out = append(out, o)
			}
		}
		s.orders = out
		return nil, nil
	}
	switch name {
	case "strategy.entry", "strategy.order", "strategy.exit", "strategy.close", "strategy.close_all":
		return r.placeStrategyOrder(call)
	}
	return nil, fmt.Errorf("native strategy API %s is not implemented", name)
}
func (s *strategyState) position() (float64, float64) {
	size, cost := 0.0, 0.0
	for _, t := range s.open {
		size += float64(t.dir) * t.qty
		cost += t.entry * t.qty
	}
	if size == 0 {
		return 0, math.NaN()
	}
	return size, cost / math.Abs(size)
}
func (s *strategyState) equity(price float64) float64 {
	total := s.cash
	for _, t := range s.open {
		total += (price - t.entry) * float64(t.dir) * t.qty * s.point
	}
	return total
}
func (s *strategyState) orderQty(price float64) float64 {
	qty := s.defaultQty
	switch s.qtyType {
	case "cash":
		qty = s.defaultQty / (price * s.point)
	case "percent_of_equity":
		qty = math.Max(0, s.equity(s.mark)) * s.defaultQty / 100 / (price * s.point)
	}
	if s.minContract > 0 {
		qty = math.Floor(qty/s.minContract+1e-10) * s.minContract
	}
	return qty
}
func (s *strategyState) fee(price, qty float64) float64 {
	switch s.commissionType {
	case "cash_per_order":
		return s.commission
	case "cash_per_contract":
		return qty * s.commission
	}
	return math.Abs(price*qty*s.point) * s.commission / 100
}
func (r *Runtime) placeStrategyOrder(call *Expr) (interface{}, error) {
	name := call.Left.Name
	s := r.strategy
	params := []string{"id", "direction", "qty", "limit", "stop", "oca_name", "oca_type", "comment", "alert_message", "disable_alert"}
	required := 2
	switch name {
	case "strategy.exit":
		params = []string{"id", "from_entry", "qty", "qty_percent", "profit", "limit", "loss", "stop", "trail_price", "trail_points", "trail_offset", "oca_name", "comment", "comment_profit", "comment_loss", "comment_trailing", "alert_message", "alert_profit", "alert_loss", "alert_trailing", "disable_alert"}
		required = 1
	case "strategy.close":
		params = []string{"id", "comment", "qty", "qty_percent", "alert_message", "immediately", "disable_alert"}
		required = 1
	case "strategy.close_all":
		params = []string{"comment", "alert_message", "immediately", "disable_alert"}
		required = 0
	}
	values, err := r.strategyArgs(call, params, required)
	if err != nil {
		return nil, err
	}
	for _, key := range []string{"trail_price", "trail_points", "trail_offset", "alert_message", "alert_profit", "alert_loss", "alert_trailing", "comment_profit", "comment_loss", "comment_trailing"} {
		if v, ok := values[key]; ok && !isNA(v) {
			if text, ok := v.(string); ok && text == "" {
				continue
			}
			return nil, fmt.Errorf("native strategy does not yet support %s", key)
		}
	}
	id, err := strategyText(values, "id", "Close all")
	if err != nil {
		return nil, err
	}
	o := &strategyOrder{id: id, kind: "entry", bar: r.effectiveBarIndex(), percent: 100}
	o.comment, err = strategyText(values, "comment", "")
	if err != nil {
		return nil, err
	}
	o.oca, err = strategyText(values, "oca_name", "")
	if err != nil {
		return nil, err
	}
	o.ocaType, err = strategyText(values, "oca_type", "none")
	if err != nil {
		return nil, err
	}
	if o.ocaType != "none" && o.ocaType != "cancel" && o.ocaType != "reduce" {
		return nil, fmt.Errorf("invalid oca_type")
	}
	o.qty, err = strategyNumber(values, "qty", math.NaN())
	if err != nil || o.qty < 0 {
		return nil, fmt.Errorf("qty must be nonnegative")
	}
	o.percent, err = strategyNumber(values, "qty_percent", 100)
	if err != nil || o.percent < 0 || o.percent > 100 {
		return nil, fmt.Errorf("qty_percent must be between 0 and 100")
	}
	for key, dst := range map[string]**float64{"limit": &o.limit, "stop": &o.stop, "profit": &o.profit, "loss": &o.loss} {
		if value, ok := values[key]; ok && !isNA(value) {
			n, err := strategyNumber(values, key, math.NaN())
			if err != nil {
				return nil, err
			}
			if key == "profit" || key == "loss" {
				if n < 0 {
					return nil, fmt.Errorf("%s must be nonnegative", key)
				}
				n = math.Ceil(n)
			} else {
				n = roundToTick(n, s.tick)
			}
			*dst = &n
		}
	}
	if name == "strategy.entry" || name == "strategy.order" {
		dir, err := strategyNumber(values, "direction", 0)
		if err != nil || (dir != 1 && dir != -1) {
			return nil, fmt.Errorf("direction must be strategy.long or strategy.short")
		}
		o.dir = int(dir)
		if name == "strategy.order" {
			o.kind = "order"
		}
		if math.IsNaN(o.qty) {
			o.defaultQty = true
			o.qty = s.orderQty(s.mark)
		}
		if s.minContract > 0 {
			o.qty = math.Floor(o.qty/s.minContract+1e-10) * s.minContract
		}
		if o.defaultQty && o.ocaType == "reduce" {
			return nil, fmt.Errorf("OCA reduce currently requires explicit qty")
		}
		if math.IsNaN(o.qty) || math.IsInf(o.qty, 0) || o.qty < 0 {
			return nil, fmt.Errorf("invalid calculated order quantity")
		}
		if o.qty == 0 && !o.defaultQty {
			return nil, nil
		}
	} else if name == "strategy.exit" {
		o.kind = "exit"
		o.from, err = strategyText(values, "from_entry", "")
		if err != nil {
			return nil, err
		}
		if o.profit != nil && o.limit != nil || o.loss != nil && o.stop != nil {
			return nil, fmt.Errorf("combined relative and absolute exit legs are not yet implemented")
		}
		if o.limit == nil && o.stop == nil && o.profit == nil && o.loss == nil {
			return nil, fmt.Errorf("strategy.exit requires a profit/limit or loss/stop leg")
		}
	} else {
		o.kind = "close"
		if name == "strategy.close" {
			o.from = id
		}
		o.intended = map[int]bool{}
		total := 0.0
		for _, t := range s.open {
			if o.from == "" || t.entryID == o.from {
				o.intended[t.uid] = true
				total += t.qty
			}
		}
		if len(o.intended) == 0 {
			return nil, nil
		}
		if math.IsNaN(o.qty) {
			o.qty = total * o.percent / 100
		}
		immediate, err := strategyBool(values, "immediately")
		if err != nil {
			return nil, err
		}
		if immediate {
			return nil, r.fillStrategyClose(o, s.mark, -1)
		}
	}
	for i, prior := range s.orders {
		if prior.id == o.id && prior.kind == o.kind && prior.from == o.from {
			s.orders[i] = o
			return nil, nil
		}
	}
	if len(s.orders) >= 10000 {
		return nil, fmt.Errorf("strategy pending order limit exceeded")
	}
	s.orders = append(s.orders, o)
	return nil, nil
}
func (r *Runtime) strategyBeginBar() error {
	s := r.strategy
	if s == nil {
		return nil
	}
	i := r.effectiveBarIndex()
	o, h, l, c := r.exactSeriesValue("open", i), r.exactSeriesValue("high", i), r.exactSeriesValue("low", i), r.exactSeriesValue("close", i)
	for _, price := range []float64{o, h, l, c} {
		if math.IsNaN(price) || math.IsInf(price, 0) {
			return fmt.Errorf("strategy bar %d has invalid OHLC", i)
		}
	}
	if h < math.Max(o, c) || l > math.Min(o, c) {
		return fmt.Errorf("strategy bar %d has inconsistent OHLC", i)
	}
	path := []float64{o, l, h, c}
	if math.Abs(o-h) < math.Abs(o-l) {
		path = []float64{o, h, l, c}
	}
	if err := r.strategySegment(o, o, true, false); err != nil {
		return err
	}
	for j := 1; j < len(path); j++ {
		if err := r.strategySegment(path[j-1], path[j], false, false); err != nil {
			return err
		}
	}
	s.mark = c
	r.strategyMark(c)
	return nil
}
func (r *Runtime) strategyEndBar() error {
	if r.strategy == nil {
		return nil
	}
	r.strategy.mark = r.exactSeriesValue("close", r.effectiveBarIndex())
	if r.strategy.processClose {
		return r.strategySegment(r.strategy.mark, r.strategy.mark, true, true)
	}
	return nil
}
func strategyCross(start, end, level float64, dir int, stop bool) (float64, bool) {
	marketable := start <= level
	if dir < 0 {
		marketable = start >= level
	}
	if stop {
		marketable = !marketable || start == level
	}
	if marketable {
		return start, true
	}
	if level >= math.Min(start, end) && level <= math.Max(start, end) {
		return level, true
	}
	return 0, false
}
func (r *Runtime) strategySegment(start, end float64, opening, closeProcessing bool) error {
	s := r.strategy
	current := start
	for events := 0; events < 100000; events++ {
		var chosen *strategyOrder
		var chosenTrade *strategyTrade
		price, best := 0.0, math.Inf(1)
		activate, slip := false, false
		for _, o := range s.orders {
			if o.bar >= r.effectiveBarIndex() && !closeProcessing {
				continue
			}
			if o.kind == "close" && opening {
				if len(s.matchingTrades(o)) > 0 && 0 < best {
					chosen, chosenTrade, price, best, activate, slip = o, nil, current, 0, false, true
				}
				continue
			}
			if o.kind == "exit" {
				for _, t := range s.matchingTrades(o) {
					limit, stop := o.limit, o.stop
					if o.profit != nil {
						p := t.entry + float64(t.dir)**o.profit*s.tick
						limit = &p
					}
					if o.loss != nil {
						p := t.entry - float64(t.dir)**o.loss*s.tick
						stop = &p
					}
					for _, leg := range []struct {
						level *float64
						stop  bool
					}{{limit, false}, {stop, true}} {
						if leg.level == nil {
							continue
						}
						p, hit := strategyCross(current, end, *leg.level, -t.dir, leg.stop)
						distance := math.Abs(p - current)
						if hit && distance < best {
							chosen, chosenTrade, price, best, activate, slip = o, t, p, distance, false, leg.stop
						}
					}
				}
				continue
			}
			if o.kind == "close" {
				continue
			}
			if o.limit == nil && o.stop == nil {
				if opening && 0 < best {
					chosen, chosenTrade, price, best, activate, slip = o, nil, current, 0, false, true
				}
				continue
			}
			level, isStop, isActivation := o.limit, false, false
			if o.stop != nil && !o.triggered {
				level, isStop, isActivation = o.stop, true, o.limit != nil
			}
			if level == nil {
				continue
			}
			p, hit := strategyCross(current, end, *level, o.dir, isStop)
			distance := math.Abs(p - current)
			if hit && distance < best {
				chosen, chosenTrade, price, best, activate, slip = o, nil, p, distance, isActivation, isStop
			}
		}
		if chosen == nil {
			r.strategyMark(end)
			return nil
		}
		r.strategyMark(price)
		current = price
		if activate {
			chosen.triggered = true
			continue
		}
		fill := price
		dir := chosen.dir
		if chosenTrade != nil {
			dir = -chosenTrade.dir
		} else if chosen.kind == "close" {
			matches := s.matchingTrades(chosen)
			if len(matches) > 0 {
				dir = -matches[0].dir
			}
		}
		if slip {
			fill += float64(dir) * s.slippage * s.tick
		}
		qty := chosen.qty
		var err error
		if chosen.kind == "close" || chosen.kind == "exit" {
			matches := s.matchingTrades(chosen)
			total := 0.0
			for _, t := range matches {
				total += t.qty
			}
			if math.IsNaN(chosen.qty) {
				chosen.qty = total * chosen.percent / 100
			}
			if s.minContract > 0 {
				chosen.qty = math.Floor(chosen.qty/s.minContract+1e-10) * s.minContract
			}
			qty = math.Min(chosen.qty, total)
			if chosenTrade != nil {
				qty = math.Min(qty, chosenTrade.qty)
			}
			err = r.fillStrategyClose(chosen, fill, tradeUID(chosenTrade))
		} else {
			qty, err = r.fillStrategyEntry(chosen, fill)
		}
		if err != nil {
			return err
		}
		if chosen.kind == "exit" {
			chosen.qty -= qty
		}
		if chosen.kind != "exit" || chosen.qty <= 1e-12 || len(s.matchingTrades(chosen)) == 0 {
			s.removeOrder(chosen)
		}
		if chosen.oca != "" && chosen.ocaType != "none" {
			remaining := s.orders[:0]
			for _, o := range s.orders {
				if o.oca == chosen.oca {
					if chosen.ocaType == "cancel" {
						continue
					}
					o.qty = math.Max(0, o.qty-qty)
					if o.qty == 0 {
						continue
					}
				}
				remaining = append(remaining, o)
			}
			s.orders = remaining
		}
	}
	return fmt.Errorf("strategy intrabar event limit exceeded")
}
func tradeUID(t *strategyTrade) int {
	if t == nil {
		return -1
	}
	return t.uid
}
func (s *strategyState) removeOrder(order *strategyOrder) {
	for i, o := range s.orders {
		if o == order {
			s.orders = append(s.orders[:i], s.orders[i+1:]...)
			return
		}
	}
}
func (s *strategyState) matchingTrades(order *strategyOrder) []*strategyTrade {
	var out []*strategyTrade
	for _, t := range s.open {
		if order.from != "" && order.from != t.entryID {
			continue
		}
		if order.intended != nil && !order.intended[t.uid] {
			continue
		}
		out = append(out, t)
	}
	return out
}
func (r *Runtime) fillStrategyEntry(o *strategyOrder, price float64) (float64, error) {
	s := r.strategy
	size, _ := s.position()
	qty := o.qty
	if o.defaultQty {
		qty = s.orderQty(price)
	}
	if o.kind == "entry" && size*float64(o.dir) > 0 && len(s.open) >= s.pyramiding {
		return 0, nil
	}
	openQty := qty
	closeQty := 0.0
	if size*float64(o.dir) < 0 {
		closeQty = math.Abs(size)
		if o.kind == "order" {
			closeQty = math.Min(closeQty, qty)
			openQty = math.Max(0, qty-closeQty)
		}
	}
	if o.kind == "entry" && (s.allow == "long" && o.dir < 0 || s.allow == "short" && o.dir > 0) {
		openQty = 0
	}
	if o.kind == "entry" && s.maxPosition > 0 {
		existing := 0.0
		if size*float64(o.dir) > 0 {
			existing = math.Abs(size)
		}
		openQty = math.Min(openQty, math.Max(0, s.maxPosition-existing))
	}
	if s.minContract > 0 {
		openQty = math.Floor(openQty/s.minContract+1e-10) * s.minContract
	}
	total := closeQty + openQty
	if total <= 0 {
		return 0, nil
	}
	if math.IsInf(total, 0) || math.IsInf(price*total*s.point, 0) || math.IsNaN(price) {
		return 0, fmt.Errorf("strategy order notional overflow")
	}
	perUnitFee := s.fee(price, total) / total
	if math.IsInf(perUnitFee, 0) || math.IsNaN(perUnitFee) {
		return 0, fmt.Errorf("strategy commission overflow")
	}
	left := closeQty
	for _, t := range append([]*strategyTrade(nil), s.open...) {
		if left <= 0 {
			break
		}
		n := math.Min(t.qty, left)
		if err := r.closeStrategyTrade(t, n, price, n*perUnitFee, o.id, o.comment); err != nil {
			return 0, err
		}
		left -= n
	}
	if openQty > 0 {
		if len(s.open) >= 1000 {
			return 0, fmt.Errorf("strategy open-trade limit exceeded")
		}
		s.nextTrade++
		open, _, _ := r.barTimes()
		t := &strategyTrade{uid: s.nextTrade, entryID: o.id, entryComment: o.comment, dir: o.dir, qty: openQty, entry: price, entryFee: openQty * perUnitFee, entryBar: r.effectiveBarIndex(), entryTime: open}
		s.cash -= t.entryFee
		s.open = append(s.open, t)
	}
	r.strategyMark(price)
	return total, nil
}
func (r *Runtime) fillStrategyClose(o *strategyOrder, price float64, uid int) error {
	s := r.strategy
	matches := s.matchingTrades(o)
	total := 0.0
	for _, t := range matches {
		if uid < 0 || uid == t.uid {
			total += t.qty
		}
	}
	qty := o.qty
	if math.IsNaN(qty) {
		qty = total * o.percent / 100
	}
	qty = math.Min(qty, total)
	if s.minContract > 0 {
		qty = math.Floor(qty/s.minContract+1e-10) * s.minContract
	}
	if qty <= 0 {
		return nil
	}
	perUnitFee := s.fee(price, qty) / qty
	for _, t := range matches {
		if uid >= 0 && uid != t.uid {
			continue
		}
		n := math.Min(t.qty, qty)
		if err := r.closeStrategyTrade(t, n, price, n*perUnitFee, o.id, o.comment); err != nil {
			return err
		}
		qty -= n
		if qty <= 1e-12 {
			break
		}
	}
	r.strategyMark(price)
	return nil
}
func (r *Runtime) closeStrategyTrade(t *strategyTrade, qty, price, fee float64, id, comment string) error {
	s := r.strategy
	if s.closeRule == "FIFO" && len(s.open) > 0 && s.open[0] != t && s.open[0].entryID != t.entryID {
		return fmt.Errorf("FIFO ledger reassignment across entry IDs is not implemented; use close_entries_rule = \"ANY\"")
	}
	if len(s.closed) >= maxArrayNewSize {
		return fmt.Errorf("strategy trade-history limit exceeded")
	}
	closed := *t
	fraction := qty / t.qty
	closed.qty = qty
	closed.entryFee = t.entryFee * fraction
	closed.exitFee = fee
	closed.exit = price
	closed.exitID = id
	closed.exitComment = comment
	closed.runup *= fraction
	closed.drawdown *= fraction
	closed.exitBar = r.effectiveBarIndex()
	closed.exitTime = r.barOpenTime()
	gross := (price - t.entry) * float64(t.dir) * qty * s.point
	if math.IsNaN(gross) || math.IsInf(gross, 0) || math.IsNaN(fee) || math.IsInf(fee, 0) {
		return fmt.Errorf("strategy trade accounting overflow")
	}
	closed.profit = gross - closed.entryFee - fee
	s.cash += gross - fee
	s.net += closed.profit
	s.closed = append(s.closed, &closed)
	percent := closed.profit / (qty * t.entry * s.point) * 100
	s.totalPercent += percent
	if closed.profit > 0 {
		s.wins++
		s.grossProfit += closed.profit
		s.winningPercent += percent
	} else if closed.profit < 0 {
		s.losses++
		s.grossLoss -= closed.profit
		s.losingPercent += percent
	} else {
		s.evens++
	}
	t.qty -= qty
	t.entryFee -= closed.entryFee
	t.runup *= 1 - fraction
	t.drawdown *= 1 - fraction
	if t.qty <= 1e-12 {
		for i, open := range s.open {
			if open == t {
				s.open = append(s.open[:i], s.open[i+1:]...)
				break
			}
		}
	}
	return nil
}
func (r *Runtime) strategyMark(price float64) {
	s := r.strategy
	if s == nil {
		return
	}
	s.mark = price
	for _, t := range s.open {
		profit := (price - t.entry) * float64(t.dir) * t.qty * s.point
		t.runup = math.Max(t.runup, profit)
		t.drawdown = math.Max(t.drawdown, -profit)
	}
	equity := s.equity(price)
	s.peak = math.Max(s.peak, equity)
	s.trough = math.Min(s.trough, equity)
	s.maxDD = math.Max(s.maxDD, s.peak-equity)
	s.maxRunup = math.Max(s.maxRunup, equity-s.trough)
	size, _ := s.position()
	s.maxAll = math.Max(s.maxAll, math.Abs(size))
	if size > 0 {
		s.maxLong = math.Max(s.maxLong, size)
	} else {
		s.maxShort = math.Max(s.maxShort, -size)
	}
}
func (r *Runtime) strategyIdentifier(name string) (interface{}, bool) {
	s := r.strategy
	if s == nil {
		return nil, false
	}
	size, average := s.position()
	openProfit := s.equity(s.mark) - s.initial - s.net
	div := func(value float64, n int) float64 {
		if n == 0 {
			return math.NaN()
		}
		return value / float64(n)
	}
	switch name {
	case "strategy.account_currency":
		return s.currency, true
	case "strategy.initial_capital":
		return s.initial, true
	case "strategy.equity":
		return s.equity(s.mark), true
	case "strategy.netprofit":
		return s.net, true
	case "strategy.netprofit_percent":
		return s.net / s.initial * 100, true
	case "strategy.openprofit":
		return openProfit, true
	case "strategy.openprofit_percent":
		return openProfit / s.initial * 100, true
	case "strategy.grossprofit":
		return s.grossProfit, true
	case "strategy.grossloss":
		return s.grossLoss, true
	case "strategy.grossprofit_percent":
		return s.grossProfit / s.initial * 100, true
	case "strategy.grossloss_percent":
		return s.grossLoss / s.initial * 100, true
	case "strategy.position_size":
		return size, true
	case "strategy.position_avg_price":
		return average, true
	case "strategy.position_entry_name":
		if len(s.open) == 0 {
			return "", true
		}
		return s.open[len(s.open)-1].entryID, true
	case "strategy.opentrades":
		return len(s.open), true
	case "strategy.closedtrades":
		return len(s.closed), true
	case "strategy.closedtrades.first_index":
		return 0, true
	case "strategy.opentrades.capital_held":
		return float64(0), true // Explicitly disabled margins.
	case "strategy.wintrades":
		return s.wins, true
	case "strategy.losstrades":
		return s.losses, true
	case "strategy.eventrades":
		return s.evens, true
	case "strategy.avg_trade":
		return div(s.net, len(s.closed)), true
	case "strategy.avg_trade_percent":
		return div(s.totalPercent, len(s.closed)), true
	case "strategy.avg_winning_trade":
		return div(s.grossProfit, s.wins), true
	case "strategy.avg_winning_trade_percent":
		return div(s.winningPercent, s.wins), true
	case "strategy.avg_losing_trade":
		return div(s.grossLoss, s.losses), true
	case "strategy.avg_losing_trade_percent":
		return div(-s.losingPercent, s.losses), true
	case "strategy.max_drawdown":
		return s.maxDD, true
	case "strategy.max_drawdown_percent":
		return s.maxDD / s.initial * 100, true
	case "strategy.max_runup":
		return s.maxRunup, true
	case "strategy.max_runup_percent":
		return s.maxRunup / s.initial * 100, true
	case "strategy.max_contracts_held_all":
		return s.maxAll, true
	case "strategy.max_contracts_held_long":
		return s.maxLong, true
	case "strategy.max_contracts_held_short":
		return s.maxShort, true
	case "strategy.margin_liquidation_price":
		return math.NaN(), true
	}
	return nil, false
}
func (r *Runtime) strategyTradeGetter(call *Expr) (interface{}, error) {
	closed := strings.HasPrefix(call.Left.Name, "strategy.closedtrades.")
	prefix := "strategy.opentrades."
	trades := r.strategy.open
	if closed {
		prefix = "strategy.closedtrades."
		trades = r.strategy.closed
	}
	field := strings.TrimPrefix(call.Left.Name, prefix)
	values, err := r.strategyArgs(call, []string{"trade_num"}, 1)
	if err != nil {
		return nil, err
	}
	n, err := integerArg(values["trade_num"], "trade_num", -maxArrayNewSize, maxArrayNewSize)
	if err != nil {
		return nil, err
	}
	if n < 0 || n >= len(trades) {
		return math.NaN(), nil
	}
	t := trades[n]
	profit := t.profit
	if !closed {
		profit = (r.strategy.mark-t.entry)*float64(t.dir)*t.qty*r.strategy.point - t.entryFee
	}
	switch field {
	case "entry_id":
		return t.entryID, nil
	case "entry_comment":
		return t.entryComment, nil
	case "entry_price":
		return t.entry, nil
	case "entry_bar_index":
		return t.entryBar, nil
	case "entry_time":
		return float64(t.entryTime.UnixMilli()), nil
	case "size":
		return float64(t.dir) * t.qty, nil
	case "commission":
		return t.entryFee + t.exitFee, nil
	case "profit":
		return profit, nil
	case "profit_percent":
		return profit / (t.entry * t.qty * r.strategy.point) * 100, nil
	case "max_runup":
		return t.runup, nil
	case "max_drawdown":
		return t.drawdown, nil
	case "max_runup_percent":
		return t.runup / (t.entry * t.qty * r.strategy.point) * 100, nil
	case "max_drawdown_percent":
		return t.drawdown / (t.entry * t.qty * r.strategy.point) * 100, nil
	}
	if closed {
		switch field {
		case "exit_id":
			return t.exitID, nil
		case "exit_comment":
			return t.exitComment, nil
		case "exit_price":
			return t.exit, nil
		case "exit_bar_index":
			return t.exitBar, nil
		case "exit_time":
			return float64(t.exitTime.UnixMilli()), nil
		}
	}
	return nil, fmt.Errorf("unknown strategy trade getter %s", field)
}
