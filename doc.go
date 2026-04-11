// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

// Package pinescription compiles Pine Script v6 source code into bytecode and executes
// it against market data providers, letting you run TradingView-style indicators
// and Pine-compatible calculations in Go applications without external dependencies.
//
// The typical compile-and-execute cycle looks like this:
//
//	engine := pinescription.NewEngine()
//	engine.RegisterMarketDataProvider(&myProvider{})
//	engine.SetDefaultSymbol("AAPL")
//
//	bytecode, err := engine.Compile(`
//	    ma := sma(close, 20)
//	    ema(close, 9)
//	`)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	result, err := engine.Execute(bytecode)
//	// result holds the final expression value, or nil if the script produces NaN
//
// A Provider supplies OHLCV series and other market data. See the Provider
// interface for the full contract. The engine is safe for concurrent use
// across multiple goroutines so long as each goroutine uses its own Engine
// instance or serializes calls to a shared instance.
package pinescription
