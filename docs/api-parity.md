<!--
SPDX-FileCopyrightText: 2026 Woodstock K.K.

SPDX-License-Identifier: AGPL-3.0-only
-->

# PineTS API work order

Reference: local PineTS commit `496eb1f`. Pine Script v6, not every PineTS
fallback or extension, defines the intended behavior.

## 1. Complete partial APIs

| Original ID | API | Status |
| --- | --- | --- |
| 27 | Occurrence-specific `str.replace` | Implemented |
| 28 | Numeric `str.tostring` formats | Implemented; mintick needs symbol metadata |
| 29 | Numeric and date/time `str.format` | Implemented for the patterns documented in `features.md` |
| 30 | Stateful `fixnan` | Implemented, with separate call-site state |
| 31 | Bare `ta.tr` | Implemented |
| 34 | Time/session/timezone and timeframe helpers | Implemented with engine-clock fallback or supplied bar times; calendar limits documented |

## 2. Document completed partial APIs

See [features.md](features.md), including compatibility extensions, accepted
patterns, limits, and the distinction between engine-clock time and exchange data.

## 3. Missing APIs, by priority

| Original ID | Priority | Area | Status |
| --- | --- | --- | --- |
| 01 | P1 | Eight volume variables | Implemented |
| 02 | P1 | RCI | Implemented; warmup/NA limitations documented |
| 04 | P1 | Array fill/reverse/search/sort/indices/some | Implemented |
| 05 | P1 | Array variance/stdev/standardize | Implemented |
| 06 | P1 | Color/line/linefill/table array constructors | Implemented, including generic spellings |
| 07 | P1 | Map merge | Implemented |
| 08 | P1 | Additional string helpers | Implemented; regex uses the documented RE2 subset |
| 18 | P1 | Runtime errors | Implemented |
| 10 | P1/P2 | Input parameter interfaces | Implemented; discovered/resolved before providers and runtime creation |
| 03 | P2 | VWAP | Implemented, with daily/anchor resets and tuple bands |
| 09 | P2 | Time formatting | Implemented for documented calendar patterns |
| 11 | P2 | Tick rounding | Implemented; explicit tick metadata required |
| 12 | P2 | Calendar functions | Implemented |
| 13 | P2 | Last-bar time, bid/ask | Implemented; bid/ask require supplied 1T data |
| 14 | P2 | Bar state | Implemented for the existing historical engine |
| 15 | P2 | Session flags | Implemented; explicit weekly schedules required |
| 16 | P2 | Symbol information | Implemented through optional provider/engine metadata |
| 21 | P2/P3 | Ticker helpers | Implemented, including parameter-preserving alternative-chart IDs; providers supply transformed bars |
| 26 | P2/P3 | Libraries/import/export | Implemented: explicit resolver, versioned imports, exported functions/methods/types/enums, private helpers, frozen linked bytecode |
| 22 | P3 | Cross-context requests | Historical `security` / `security_lower_tf` implemented with isolated contexts, confirmed alignment, gaps/lookahead and tuple arrays; limits below |
| 23 | P3 | Footprint data | Implemented with genuine provider levels, immutable rows, POC/value area/imbalance calculations |
| 25 | P3 | Strategies | **Partial**: native historical broker, basic orders, trade ledger, fees/slippage, sizing, account/trade getters and two risk rules; advanced behavior below remains pending |

**This is not full PineTS or TradingView parity.** Exact-name host hooks retain
precedence over native strategy/request implementations. No data-less request
or alternative-chart no-op is counted as a completed implementation.

### Remaining compatibility work

- **Strategy broker:** margin liquidation (native mode requires explicit zero
  margins), trailing stops, fill/tick recalculation, bar magnifier, limit-fill
  verification, cross-currency accounting, combined relative/absolute exit-leg
  precedence, FIFO reassignment across distinct entry IDs, full exit reservation
  semantics, and order-fill alerts. Native code rejects unsupported options.
- **Strategy risk:** `max_cons_loss_days`, `max_drawdown`,
  `max_intraday_filled_orders`, and `max_intraday_loss` are still pending.
  `allow_entry_in` and `max_position_size` are implemented.
- **Requests:** cross-currency conversion, arbitrary mutable function-local
  captures, and complete TradingView parity for all expression/data-gap cases
  remain unverified/unsupported. Supported global dependencies and collection
  mutations are replayed; unrelated callbacks are not.
- **Other request sources:** financial/economic/corporate-action/seed APIs still
  require host hooks. PineTS's data-less `na` implementations were not copied.
- **Independent platform validation:** the historical broker is tested against
  deterministic, hand-calculated fixtures, not certified against every
  TradingView strategy field or fill scenario.

## 4. Document missing-API additions

Updated [api.md](api.md), [features.md](features.md),
[architecture.md](architecture.md), and the README for this batch. Inputs,
market-context requirements, historical-only semantics, compatibility limits,
and remaining gaps are explicit.

## Explicitly excluded

Original IDs **17, 19, 20, 24, 32, 33, 35, 36**: color and drawing/chart work,
plotting, a blanket argument-handling redesign, and a separate broad
tests/examples/coverage project. Focused regression tests accompany code
changes; this does not introduce a new argument-handling architecture.
