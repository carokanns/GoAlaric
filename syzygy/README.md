# Syzygy probing

The C probing code in this directory is vendored from
[`jdart1/Fathom`](https://github.com/jdart1/Fathom) commit
`c9c6fef0dddc05d2e242c183acf5833149ab676d` (2025-12-23).

Fathom is MIT licensed; its license is preserved in `FATHOM_LICENSE`.
The upstream `tbchess.c` is stored as `tbchess.inc` because Fathom includes it
from `tbprobe.c`; leaving the `.c` suffix would make cgo compile it twice.
GoAlaric converts its file-major bitboards to Fathom's standard rank-major
layout before every probe. WDL probing is used inside the search tree and the
non-thread-safe DTZ root API is serialized and called only once per search.

Builds with `CGO_ENABLED=0` remain supported, but tablebase probing is disabled
in those builds.
