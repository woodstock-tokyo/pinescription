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

// SymbolInfo holds provider-supplied syminfo fields. Missing numeric fields are
// na; no default mintick, point value, currency or exchange hours are invented.
type SymbolInfo map[string]interface{}

type SymbolInfoProvider interface {
	GetSymbolInfo(symbol string) (SymbolInfo, error)
}

// BarTime optionally supplies actual timestamps, including gaps and overnight
// trading-day labels. Times are absolute; exchange timezone lives in SymbolInfo.
type BarTime struct{ Open, Close, TradingDay time.Time }
type BarTimeProvider interface {
	GetBarTimes(symbol, timeframe string) ([]BarTime, error)
}

// MarketSessions contains explicit Pine session strings. Holidays and special
// closes should be represented in the provider's bars; strings are weekly rules.
type MarketSessions struct{ Regular, Premarket, Postmarket string }
type MarketSessionsProvider interface {
	GetMarketSessions(symbol string) (MarketSessions, error)
}

func (e *Engine) SetSymbolInfo(symbol string, info SymbolInfo) {
	if e.symbolInfo == nil {
		e.symbolInfo = map[string]SymbolInfo{}
	}
	copy := SymbolInfo{}
	for k, v := range info {
		copy[k] = v
	}
	e.symbolInfo[symbol] = copy
}
func (e *Engine) SetBarTimes(bars []BarTime) { e.barTimes = append([]BarTime(nil), bars...) }
func (e *Engine) SetMarketSessions(symbol string, sessions MarketSessions) {
	if e.marketSessions == nil {
		e.marketSessions = map[string]MarketSessions{}
	}
	e.marketSessions[symbol] = sessions
}

type marketContext struct {
	info     SymbolInfo
	bars     []BarTime
	sessions MarketSessions
	location *time.Location
}

func (e *Engine) marketContext(provider Provider, symbol, tf string, count int) (marketContext, error) {
	c := marketContext{info: SymbolInfo{}, location: time.UTC}
	if p, ok := provider.(SymbolInfoProvider); ok {
		info, err := p.GetSymbolInfo(symbol)
		if err != nil {
			return c, fmt.Errorf("symbol info: %w", err)
		}
		for k, v := range info {
			c.info[k] = v
		}
	}
	for k, v := range e.symbolInfo[symbol] {
		c.info[k] = v
	}
	if zone, ok := c.info["timezone"].(string); ok {
		loc, err := checkedTimezone(zone)
		if err != nil {
			return c, err
		}
		c.location = loc
	}
	if e.barTimes != nil {
		c.bars = append([]BarTime(nil), e.barTimes...)
	} else if p, ok := provider.(BarTimeProvider); ok {
		bars, err := p.GetBarTimes(symbol, tf)
		if err != nil {
			return c, fmt.Errorf("bar timestamps: %w", err)
		}
		c.bars = append([]BarTime(nil), bars...)
	}
	if c.bars != nil {
		if len(c.bars) != count {
			return c, fmt.Errorf("bar timestamp count %d differs from series length %d", len(c.bars), count)
		}
		for i, bar := range c.bars {
			if bar.Open.IsZero() || !bar.Close.After(bar.Open) || i > 0 && !bar.Open.After(c.bars[i-1].Open) {
				return c, fmt.Errorf("invalid bar timestamps at index %d", i)
			}
		}
	}
	if p, ok := provider.(MarketSessionsProvider); ok {
		s, err := p.GetMarketSessions(symbol)
		if err != nil {
			return c, err
		}
		c.sessions = s
	}
	if s, ok := e.marketSessions[symbol]; ok {
		c.sessions = s
	}
	for _, s := range []string{c.sessions.Regular, c.sessions.Premarket, c.sessions.Postmarket} {
		if s != "" {
			if _, _, err := sessionAt(time.Now().In(c.location), s); err != nil {
				return c, err
			}
		}
	}
	if tick, exists := c.info["mintick"]; exists {
		n, ok := toFloat(tick)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n <= 0 {
			return c, fmt.Errorf("syminfo.mintick must be positive")
		}
		c.info["mintick"] = n
	}
	return c, nil
}

func (r *Runtime) applyMarketContext(c marketContext) {
	r.marketContext = c
	info := r.rootNamespaces["syminfo"].(map[string]interface{})
	for _, field := range strings.Fields("tickerid main_tickerid prefix ticker root description type currency basecurrency country timezone session volumetype isin current_contract industry sector") {
		info[field] = ""
	}
	for _, field := range strings.Fields("mintick pricescale minmove pointvalue mincontract expiration_date employees shareholders shares_outstanding_float shares_outstanding_total recommendations_buy recommendations_buy_strong recommendations_date recommendations_hold recommendations_sell recommendations_sell_strong recommendations_total target_price_average target_price_date target_price_estimates target_price_high target_price_low target_price_median") {
		info[field] = math.NaN()
	}
	symbol := strings.SplitN(r.activeSymbol, ";", 2)[0]
	if id, err := decodeTicker(r.activeSymbol); err == nil {
		symbol = id.symbol
	}
	prefix, ticker, found := strings.Cut(symbol, ":")
	if !found {
		ticker, prefix = symbol, ""
	}
	info["ticker"], info["prefix"], info["tickerid"], info["main_tickerid"] = ticker, prefix, r.activeSymbol, r.activeSymbol
	info["timezone"], info["session"] = c.location.String(), r.session
	for k, v := range c.info {
		info[k] = v
	}
	r.lastTimeIndex = -1
}

func (r *Runtime) exchangeTimezone() *time.Location {
	if r.marketContext.location != nil {
		return r.marketContext.location
	}
	return time.UTC
}

func (r *Runtime) sessionFlag(name string) (interface{}, error) {
	s := r.marketContext.sessions
	if s.Regular == "" {
		return nil, fmt.Errorf("%s requires an explicit regular MarketSessions schedule", name)
	}
	open, close, _ := r.barTimes()
	open = open.In(r.exchangeTimezone())
	window, regular, err := sessionAt(open, s.Regular)
	if err != nil {
		return nil, err
	}
	contains := func(spec string) bool {
		if spec == "" {
			return false
		}
		_, ok, _ := sessionAt(open, spec)
		return ok
	}
	switch name {
	case "session.ismarket":
		return regular, nil
	case "session.ispremarket":
		return contains(s.Premarket), nil
	case "session.ispostmarket":
		return contains(s.Postmarket), nil
	case "session.isfirstbar_regular":
		return regular && !open.After(window.open), nil
	case "session.islastbar_regular":
		return regular && !close.Before(window.close), nil
	case "session.isfirstbar", "session.islastbar":
		// Extend through explicitly supplied adjacent pre/regular/post windows.
		first, last := window.open, window.close
		inSession := regular
		specs := []string{s.Premarket, s.Regular, s.Postmarket}
		if !regular {
			for _, spec := range specs {
				if spec == "" {
					continue
				}
				if w, ok, _ := sessionAt(open, spec); ok {
					first, last, inSession = w.open, w.close, true
					break
				}
			}
		}
		if !inSession {
			return false, nil
		}
		for pass := 0; pass < len(specs); pass++ {
			for _, spec := range specs {
				if spec == "" {
					continue
				}
				for _, point := range []time.Time{first.Add(-time.Nanosecond), last} {
					if w, ok, _ := sessionAt(point, spec); ok {
						if w.open.Before(first) {
							first = w.open
						}
						if w.close.After(last) {
							last = w.close
						}
					}
				}
			}
		}
		if name == "session.isfirstbar" {
			return inSession && !open.After(first), nil
		}
		return inSession && !close.Before(last), nil
	}
	return nil, fmt.Errorf("unknown session flag %s", name)
}
