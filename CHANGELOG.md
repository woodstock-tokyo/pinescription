# Changelog

All notable changes to this project will be documented in this file.

## Unreleased

### Changed

- Allow exact-name registered custom functions to handle namespaced Pine calls such as `strategy.order(...)` before unsupported-feature checks run.

### Tests

- Add runtime coverage confirming a registered namespaced function can be called from Pine Script with evaluated arguments.

### Documentation

- Clarify that unsupported APIs such as `strategy.*`, `request.*`, and plotting functions still raise runtime errors unless an exact-name custom function hook is registered.
- Document `RegisterFunctionWithParamNames` for registered functions that need Pine Script named-argument support.
