// Command genoffsets regenerates offsets.go from onnxruntime_c_api.h.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"os"
	"regexp"
	"strings"
)

var (
	memberRE = regexp.MustCompile(`ORT_API2_STATUS\s*\(\s*([A-Za-z_]\w*)|STABLE_API2_STATUS\s*\(\s*([A-Za-z_]\w*)|ORT_API_STATUS\s*\(\s*([A-Za-z_]\w*)|ORT_CLASS_RELEASE\s*\(\s*(\w+)\s*\)|\(\s*ORT_API_CALL\s*\*\s*([A-Za-z_]\w*)\s*\)`)
	offsetRE = regexp.MustCompile(`ortIdx([A-Za-z_]\w*)\s*=\s*(\d+)`)
	apiRE    = regexp.MustCompile(`(?m)^#define\s+ORT_API_VERSION\s+(\d+)`)
	structRE = regexp.MustCompile(`struct\s+OrtApi\s*\{`)
	blockRE  = regexp.MustCompile(`(?s)/\*.*?\*/`)
	lineRE   = regexp.MustCompile(`//[^\n]*`)
)

func main() {
	header := flag.String("header", "ort_c_api.h", "path to onnxruntime_c_api.h")
	source := flag.String("source", "offsets.go", "existing offsets file that lists the required names")
	output := flag.String("o", "offsets.go", "output path")
	check := flag.Bool("check", false, "verify that the existing offsets match the header")
	flag.Parse()

	headerBytes, err := os.ReadFile(*header)
	if err != nil {
		fatal(err)
	}
	sourceBytes, err := os.ReadFile(*source)
	if err != nil {
		fatal(err)
	}

	names := requiredNames(string(sourceBytes))
	index := apiIndex(string(headerBytes))
	version := "31"
	if m := apiRE.FindStringSubmatch(string(headerBytes)); m != nil {
		version = m[1]
	}

	var missing []string
	for _, name := range names {
		if _, ok := index[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		fatal(fmt.Errorf("header is missing members: %s", strings.Join(missing, ", ")))
	}

	generated := render(version, names, index)
	formatted, err := format.Source(generated)
	if err != nil {
		fatal(err)
	}
	if *check {
		existing := bytes.ReplaceAll(sourceBytes, []byte("\r\n"), []byte("\n"))
		if !bytes.Equal(formatted, existing) {
			fatal(fmt.Errorf("%s is out of date; run with -header and -o to regenerate", *source))
		}
		fmt.Println("offsets match")
		return
	}
	if err := os.WriteFile(*output, formatted, 0o644); err != nil {
		fatal(err)
	}
	fmt.Printf("wrote %s with %d offsets from %s\n", *output, len(names), *header)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

// requiredNames returns the member names referenced by an existing offsets file.
func requiredNames(source string) []string {
	matches := offsetRE.FindAllStringSubmatch(source, -1)
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, m[1])
	}
	return names
}

// apiIndex parses OrtApi member names and their function table indices.
func apiIndex(source string) map[string]int {
	source = blockRE.ReplaceAllString(source, " ")
	source = lineRE.ReplaceAllString(source, " ")
	loc := structRE.FindStringIndex(source)
	if loc == nil {
		fatal(fmt.Errorf("struct OrtApi not found"))
	}
	start := loc[0]
	open := strings.Index(source[start:], "{") + start
	depth := 0
	end := -1
	for i := open; i < len(source); i++ {
		switch source[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i
				i = len(source)
			}
		}
	}
	if end < 0 {
		fatal(fmt.Errorf("unterminated struct OrtApi"))
	}

	index := map[string]int{}
	idx := 0
	for _, m := range memberRE.FindAllStringSubmatch(source[open+1:end], -1) {
		name := ""
		for i, group := range m[1:] {
			if group != "" {
				name = group
				if i == 3 {
					name = "Release" + group
				}
				break
			}
		}
		if name != "" {
			index[name] = idx
			idx++
		}
	}
	return index
}

// render builds the offsets source file.
func render(version string, names []string, index map[string]int) []byte {
	var b strings.Builder
	b.WriteString("package main\n\n")
	fmt.Fprintf(&b, "// Code generated from onnxruntime_c_api.h (ORT_API_VERSION %s). DO NOT EDIT.\n\n", version)
	b.WriteString("const (\n")
	for _, name := range names {
		fmt.Fprintf(&b, "\tortIdx%s = %d\n", name, index[name])
	}
	b.WriteString(")\n")
	return []byte(b.String())
}
