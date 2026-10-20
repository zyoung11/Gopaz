# mkicon

Renders the Gopaz brand mark into `gopaz/gopaz.ico` (16 to 256 px,
PNG-compressed frames). Used together with
[rsrc](https://github.com/akavel/rsrc) to embed the icon into the executable:

```bash
cd gopaz
go run ./tools/mkicon -o gopaz.ico
rsrc -ico gopaz.ico -arch amd64 -o rsrc_windows_amd64.syso
```

The generated `gopaz.ico` and `rsrc_windows_amd64.syso` are checked in;
`go build` links the icon automatically. Re-run both commands only after
changing the brand mark.

# genoffsets

Regenerates `gopaz/offsets.go` from `onnxruntime_c_api.h`.

The header is not part of this repository. Copy it from the onnxruntime source tree or release matching the bundled `onnxruntime.dll` (API version 31 for onnxruntime 1.31).

```bash
cd gopaz
go run ./tools/genoffsets -header ort_c_api.h
go run ./tools/genoffsets -check -header ort_c_api.h
```

`-source` and `-o` default to `offsets.go`. `-check` exits nonzero when the checked-in offsets no longer match the header.
