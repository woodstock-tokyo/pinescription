<!--
SPDX-FileCopyrightText: 2026 Woodstock K.K.

SPDX-License-Identifier: AGPL-3.0-only
-->

# Documentation Index

This folder contains the maintained reference material for Pinescription beyond the package-level GoDoc on `pkg.go.dev`.

## Start Here

- [`api.md`](api.md) — public Go API reference for `Engine`, `Runtime`, `Provider`, alerts, and the recommended execution workflow
- [`architecture.md`](architecture.md) — compilation pipeline, runtime lifecycle, and bar-by-bar execution model
- [`features.md`](features.md) — supported and unsupported Pine Script language/runtime features

## Runnable Examples

Pinescription now includes executable example tests that render on `pkg.go.dev` and run under `go test`:

- Root package examples in `example_engine_test.go`
  - `ExampleNewEngine`
  - `ExampleEngine_ExecuteWithRuntime`
- Series package examples in `series/examples_test.go`
  - `ExampleNewQueue`
  - `ExampleSeriesExtend_Mean`
  - `ExampleCrossOver`

Use those examples for copy-pasteable consumer workflows, and use the documents in this folder for broader API and design context.

## Recommended Reading Order

1. Package GoDoc / pkg.go.dev examples for a quick first run
2. [`api.md`](api.md) to understand the public Go API surface
3. [`features.md`](features.md) to confirm Pine Script compatibility
4. [`architecture.md`](architecture.md) for internals and execution flow
