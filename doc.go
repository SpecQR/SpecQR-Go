// Package specqr encodes QR Code Model 2 versions 1 through 40 without external
// Go modules or CGO. It supports numeric, alphanumeric, UTF-8, raw bytes and Kanji,
// explicit/mixed segments, ECI/FNC1, GS1 helpers and Structured Append.
//
// Generate and related functions return immutable symbols with detached slices
// and diagnostics. Public malformed data returns typed *Error values. Callers
// must not mutate their input slices or Options pointers concurrently with a call.
// Rendering uses standard-library PNG encoding and escaped SVG, with allocation
// budgets checked before raster allocation. PNG bytes can differ between Go
// versions; decoded RGBA pixels and QR matrices have deterministic semantics.
//
// High-level FNC1 text preserves literal percent by selecting byte mode; forcing
// alphanumeric mode for such input is rejected. Manual FNC1 alphanumeric segments
// retain low-level percent escaping: %% is a literal percent and % is a separator.
// GS1 uses a bounded catalog, not a complete GS1 validator or certification claim.
package specqr
