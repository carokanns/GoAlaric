# Vendored Fathom

This directory contains the Syzygy probing core from `jdart1/Fathom`, pinned
to commit `c9c6fef0dddc05d2e242c183acf5833149ab676d` (2025-12-23). Fathom is
distributed under the MIT license included in `LICENSE`.

`tbchess.c` is stored as `tbchess.inc` because `tbprobe.c` includes it as one
translation unit. The only source modification is that include name and the
Go cgo build constraint at the top of `tbprobe.c`.
