# genoffsets

Regenerates `gopaz/offsets.go` from `onnxruntime_c_api.h`.

The header is not part of this repository. Copy it from the onnxruntime source tree or release matching the bundled `onnxruntime.dll` (API version 31 for onnxruntime 1.31).

```bash
cd gopaz
go run ./tools/genoffsets -header ort_c_api.h
go run ./tools/genoffsets -check -header ort_c_api.h
```

`-source` and `-o` default to `offsets.go`. `-check` exits nonzero when the checked-in offsets no longer match the header.
