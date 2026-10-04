// Development-only JSON-lines adapter to the actual dependency-free Go package.
// It contains no reference encoder or expected candidate outputs.
package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	qr "github.com/SpecQR/SpecQR-Go"
)

type object = map[string]any

func bad(s string) { panic(&qr.Error{Code: qr.InvalidInput, Message: s}) }
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
func str(v any) string {
	s, ok := v.(string)
	if !ok {
		bad("Expected string")
	}
	return s
}
func obj(v any) object {
	m, ok := v.(map[string]any)
	if !ok {
		bad("Expected object")
	}
	return m
}
func arr(v any) []any {
	a, ok := v.([]any)
	if !ok {
		bad("Expected array")
	}
	return a
}
func num(v any) int {
	n, ok := v.(json.Number)
	if !ok {
		bad("Expected integer")
	}
	x, e := strconv.ParseInt(string(n), 10, 32)
	if e != nil {
		bad("Expected integer")
	}
	return int(x)
}
func boolean(v any) bool {
	b, ok := v.(bool)
	if !ok {
		bad("Expected boolean")
	}
	return b
}
func dataBytes(v any) []byte {
	a := arr(v)
	b := make([]byte, len(a))
	for i, x := range a {
		n := num(x)
		if n < 0 || n > 255 {
			bad("Expected integer in 0..255")
		}
		b[i] = byte(n)
	}
	return b
}
func ints(b []byte) []int {
	out := make([]int, len(b))
	for i, v := range b {
		out[i] = int(v)
	}
	return out
}
func digest(b []byte) string { d := sha256.Sum256(b); return hex.EncodeToString(d[:]) }
func rows(matrix [][]bool) []string {
	out := make([]string, len(matrix))
	for i, row := range matrix {
		b := make([]byte, len(row))
		for j, v := range row {
			b[j] = '0'
			if v {
				b[j] = '1'
			}
		}
		out[i] = string(b)
	}
	return out
}
func matrixHash(matrix [][]bool) string { return digest([]byte(strings.Join(rows(matrix), ""))) }
func typed(err error) object {
	var e *qr.Error
	if errors.As(err, &e) {
		return object{"error": "SpecQrError", "message": e.Message, "isSpecQRError": true, "code": string(e.Code)}
	}
	return object{"error": fmt.Sprintf("%T", err), "message": err.Error(), "isSpecQRError": false, "code": nil}
}
func outcome(v string, e error) object {
	if e != nil {
		var q *qr.Error
		if errors.As(e, &q) {
			return object{"ok": false, "code": string(q.Code)}
		}
		panic(e)
	}
	return object{"ok": true, "value": v}
}
func asMap(v any) object {
	b := must(json.Marshal(v))
	var out object
	if e := json.Unmarshal(b, &out); e != nil {
		panic(e)
	}
	return out
}

// encoding/json replaces unpaired surrogate escapes. The adapter rejects them
// first so an invalid UTF-16 test cannot silently become valid replacement text.
func validateJSONUnicode(line []byte) error {
	if !utf8.Valid(line) {
		return &qr.Error{Code: qr.InvalidInput, Message: "JSON must be valid UTF-8"}
	}
	inside := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if !inside {
			if c == '"' {
				inside = true
			}
			continue
		}
		if c == '"' {
			inside = false
			continue
		}
		if c != '\\' {
			continue
		}
		i++
		if i >= len(line) {
			break
		}
		if line[i] != 'u' {
			continue
		}
		if i+4 >= len(line) {
			break
		}
		n, e := strconv.ParseUint(string(line[i+1:i+5]), 16, 16)
		if e != nil {
			continue
		}
		i += 4
		if n >= 0xd800 && n <= 0xdbff {
			if i+6 >= len(line) || line[i+1] != '\\' || line[i+2] != 'u' {
				return &qr.Error{Code: qr.InvalidInput, Message: "Unpaired UTF-16 surrogate"}
			}
			m, e := strconv.ParseUint(string(line[i+3:i+7]), 16, 16)
			if e != nil || m < 0xdc00 || m > 0xdfff {
				return &qr.Error{Code: qr.InvalidInput, Message: "Unpaired UTF-16 surrogate"}
			}
			i += 6
		} else if n >= 0xdc00 && n <= 0xdfff {
			return &qr.Error{Code: qr.InvalidInput, Message: "Unpaired UTF-16 surrogate"}
		}
	}
	return nil
}
func parse(line []byte) (object, error) {
	if e := validateJSONUnicode(line); e != nil {
		return nil, e
	}
	d := json.NewDecoder(bytes.NewReader(line))
	d.UseNumber()
	var r object
	if e := d.Decode(&r); e != nil {
		return nil, &qr.Error{Code: qr.InvalidInput, Message: e.Error()}
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		return nil, &qr.Error{Code: qr.InvalidInput, Message: "Trailing JSON content"}
	}
	if r == nil {
		return nil, &qr.Error{Code: qr.InvalidInput, Message: "Expected object"}
	}
	return r, nil
}

func segments(v any) []qr.Segment {
	a := arr(v)
	if len(a) > 16384 {
		bad("Too many segments")
	}
	out := make([]qr.Segment, 0, len(a))
	for _, item := range a {
		r := obj(item)
		mode := str(r["mode"])
		allowed := map[string]bool{"mode": true}
		switch mode {
		case "numeric", "alphanumeric", "byte", "kanji":
			allowed["text"] = true
			allowed["bytes"] = true
			_, text := r["text"]
			_, binary := r["bytes"]
			if text == binary {
				bad("Data segment needs exactly one text or bytes field")
			}
		case "eci":
			allowed["assignmentNumber"] = true
		case "fnc1":
		case "fnc1-second":
			allowed["applicationIndicator"] = true
		case "structured-append":
			allowed["index"] = true
			allowed["total"] = true
			allowed["parity"] = true
		default:
			bad("Unknown segment mode")
		}
		for k := range r {
			if !allowed[k] {
				bad("Unknown segment field")
			}
		}
		var s qr.Segment
		switch mode {
		case "numeric":
			s = must(qr.NumericSegment(str(r["text"])))
		case "alphanumeric":
			s = must(qr.AlphanumericSegment(str(r["text"])))
		case "kanji":
			s = must(qr.KanjiSegment(str(r["text"])))
		case "byte":
			if t, ok := r["text"]; ok {
				s = must(qr.UTF8Segment(str(t)))
			} else {
				s = must(qr.BytesSegment(dataBytes(r["bytes"])))
			}
		case "eci":
			s = must(qr.ECISegment(num(r["assignmentNumber"])))
		case "fnc1":
			s = must(qr.FNC1Segment())
		case "fnc1-second":
			s = must(qr.FNC1SecondSegment(str(r["applicationIndicator"])))
		case "structured-append":
			s = must(qr.StructuredAppendSegment(num(r["index"]), num(r["total"]), num(r["parity"])))
		}
		out = append(out, s)
	}
	return out
}
func options(r any) qr.Options {
	opts := qr.DefaultOptions()
	if r == nil {
		return opts
	}
	for k, v := range obj(r) {
		switch k {
		case "version":
			if v != "auto" {
				opts.Version = num(v)
				if opts.Version < 1 || opts.Version > 40 {
					panic(&qr.Error{Code: qr.InvalidVersion, Message: "Version must be between 1 and 40"})
				}
			}
		case "minVersion":
			opts.MinVersion = num(v)
		case "maxVersion":
			opts.MaxVersion = num(v)
		case "maskPattern":
			if v != "auto" {
				n := num(v)
				opts.Mask = &n
			}
		case "errorCorrectionLevel":
			opts.ECC = qr.ECC(str(v))
		case "mode":
			opts.Mode = qr.Mode(str(v))
		case "optimizeSegments":
			opts.DisableOptimization = !boolean(v)
		case "boostErrorCorrection":
			opts.BoostECC = boolean(v)
		case "eci":
			if v != false {
				n := num(v)
				opts.ECI = &n
			}
		case "gs1":
			opts.GS1 = boolean(v)
		case "fnc1Second":
			if v != false {
				s := str(v)
				opts.FNC1Second = &s
			}
		case "structuredAppend":
			m := obj(v)
			s := must(qr.StructuredAppendSegment(num(m["index"]), num(m["total"]), num(m["parity"])))
			opts.StructuredAppend = &s
		case "scale":
			opts.Render.Scale = num(v)
		case "margin":
			opts.Render.Margin = num(v)
		case "foreground":
			opts.Render.Foreground = str(v)
		case "background":
			opts.Render.Background = str(v)
		case "printDpi":
			n, ok := v.(json.Number)
			if !ok {
				bad("DPI must be number")
			}
			opts.PrintDPI = must(n.Float64())
		case "maxSymbols", "diagnostics":
		case "output":
			if str(v) != "matrix" {
				bad("Conformance output must be matrix")
			}
		default:
			bad("Unknown option: " + k)
		}
	}
	return opts
}
func symbol(code *qr.QRCode, r object) object {
	out := object{"version": code.Version(), "ecc": code.ECC(), "mask": code.Mask(), "matrixHash": matrixHash(code.Matrix()), "data": hex.EncodeToString(code.DataCodewords()), "codewords": hex.EncodeToString(code.Codewords())}
	if r["includeMatrix"] == true || r["pngScale"] != nil {
		out["matrix"] = rows(code.Matrix())
	}
	if r["pngScale"] != nil {
		out["png"] = hex.EncodeToString(must(code.ToPNG()))
	}
	if r["includeDiagnostics"] == true {
		out["diagnostics"] = code.Diagnostics()
	}
	return out
}
func identity(r object) object {
	path := must(os.Executable())
	path = must(filepath.EvalSymlinks(path))
	d := digest(must(os.ReadFile(path)))
	deps := []string{}
	moduleVersion := ""
	modules := []object{}
	if info, ok := debug.ReadBuildInfo(); ok {
		moduleVersion = info.Main.Version
		for _, dep := range info.Deps {
			m := object{"path": dep.Path, "version": dep.Version, "sum": dep.Sum}
			if dep.Replace != nil {
				m["replacePath"] = dep.Replace.Path
				m["replaceVersion"] = dep.Replace.Version
			}
			modules = append(modules, m)
			if dep.Path != "github.com/SpecQR/SpecQR-Go" {
				deps = append(deps, dep.Path)
			}
		}
	}
	return object{"language": "Go", "packageVersion": qr.Version, "moduleVersion": moduleVersion, "goVersion": runtime.Version(), "nonce": r["nonce"], "pid": os.Getpid(), "executable": path, "binarySha256": d, "packageFilesSha256": object{filepath.Base(path): d}, "runtimeDependencies": deps, "goModules": modules}
}
func raw(r object) object {
	version := num(r["version"])
	seed := num(r["seed"])
	mask := num(r["mask"])
	ecc := qr.ECC(str(r["ecc"]))
	n := must(qr.DataCodewordCount(version, ecc))
	ordinal := strings.Index("LMQH", string(ecc))
	data := make([]byte, n)
	for i := range data {
		if seed == 0 {
			data[i] = 0
		} else if seed == 1 {
			data[i] = 255
		} else {
			data[i] = byte((i*149 + version*43 + ordinal*89 + seed*67) ^ (i >> uint(seed+1)))
		}
	}
	enc := must(qr.InterleaveCodewords(data, version, ecc))
	var fixed *int
	if mask >= 0 {
		fixed = &mask
	}
	m := must(qr.BuildMatrix(enc.Codewords, version, ecc, fixed))
	penalties := make([]int, len(m.MaskPenalties))
	for i, p := range m.MaskPenalties {
		penalties[i] = p.Penalty
	}
	return object{"data": hex.EncodeToString(data), "codewords": hex.EncodeToString(enc.Codewords), "matrixHash": matrixHash(m.Matrix), "mask": m.MaskPattern, "penalty": m.Penalty, "penalties": penalties}
}
func payload(r object) (string, []byte, bool) {
	if v, ok := r["rawText"]; ok {
		b := dataBytes(v)
		if !utf8.Valid(b) {
			bad("Text must be valid UTF-8")
		}
		return string(b), nil, false
	}
	if v, ok := r["bytes"]; ok {
		return "", dataBytes(v), true
	}
	if v, ok := r["text"]; ok {
		return str(v), nil, false
	}
	return "", nil, false
}
func appendSymbols(r object, opts qr.Options, text string, data []byte, binary bool) object {
	max := 16
	if v, ok := r["options"]; ok {
		if n, ok := obj(v)["maxSymbols"]; ok {
			max = num(n)
		}
	}
	full := r["includeDiagnostics"] == true
	o := qr.StructuredAppendOptions{Options: opts, MaxSymbols: max, FullSplitUnits: full, SymbolDiagnostics: full}
	var result *qr.SAResult
	if v, ok := r["segments"]; ok {
		result = must(qr.GenerateSegmentsStructuredAppend(segments(v), o))
	} else if binary {
		result = must(qr.GenerateBytesStructuredAppend(data, o))
	} else {
		result = must(qr.GenerateStructuredAppend(text, o))
	}
	symbols := []object{}
	hashes := []any{}
	versions := []any{}
	masks := []any{}
	for _, q := range result.Symbols() {
		s := symbol(q, r)
		symbols = append(symbols, s)
		hashes = append(hashes, s["matrixHash"])
		versions = append(versions, s["version"])
		masks = append(masks, s["mask"])
	}
	out := object{"total": result.Total(), "parity": result.Parity(), "inputLength": result.InputLength(), "byteLength": result.ByteLength(), "matrixHashes": hashes, "versions": versions, "masks": masks, "symbols": symbols}
	if full {
		out["diagnostics"] = result.Diagnostics()
	}
	return out
}
func merge(r object) object {
	parts := []qr.SAPart{}
	for _, v := range arr(r["parts"]) {
		m := obj(v)
		_, hasText := m["text"]
		_, hasBytes := m["bytes"]
		if hasText == hasBytes {
			bad("Part needs exactly one text or bytes payload")
		}
		p := qr.SAPart{Index: num(m["index"]), Total: num(m["total"]), Parity: num(m["parity"]), Binary: hasBytes}
		if hasText {
			s := str(m["text"])
			p.Text = &s
		} else {
			p.Bytes = dataBytes(m["bytes"])
		}
		parts = append(parts, p)
	}
	result := must(qr.MergeStructuredAppendParts(parts))
	var data any = result.Text()
	if result.Binary() {
		data = ints(result.Bytes())
	}
	pout := result.PartMetadata()
	return object{"data": data, "total": result.Total(), "parity": result.Parity(), "parts": pout, "diagnostics": result.Diagnostics()}
}
func gs1Elements(v any) []qr.GS1Element {
	out := []qr.GS1Element{}
	for _, x := range arr(v) {
		m := obj(x)
		out = append(out, qr.GS1Element{AI: str(m["ai"]), Value: str(m["value"])})
	}
	return out
}
func pairs(v any) [][]string {
	b := must(json.Marshal(v))
	var a []map[string]any
	if e := json.Unmarshal(b, &a); e != nil {
		panic(e)
	}
	out := [][]string{}
	for _, m := range a {
		out = append(out, []string{str(m["ai"]), str(m["value"])})
	}
	return out
}
func gs1(r object, command string) object {
	switch command {
	case "catalog":
		return object{"catalog": object{"ok": true, "value": qr.GS1GetSupportedAIs()}}
	case "url":
		input := str(r["input"])
		opts := qr.GS1DigitalLinkOptions{}
		if v, ok := r["options"]; ok {
			m := obj(v)
			if v, ok := m["primaryAi"]; ok {
				opts.PrimaryAI = str(v)
			}
			if v, ok := m["unknownQuery"]; ok {
				opts.UnknownQuery = str(v)
			}
			if v, ok := m["normalize"]; ok {
				opts.Normalize = boolean(v)
			}
		}
		p, e := qr.GS1ParseDigitalLink(input, opts)
		var parsed object
		if e != nil {
			var q *qr.Error
			if !errors.As(e, &q) {
				panic(e)
			}
			parsed = object{"ok": false, "code": string(q.Code)}
		} else {
			m := asMap(p)
			unknown := [][]string{}
			if a, ok := m["unknownQuery"].([]any); ok {
				for _, v := range a {
					q := obj(v)
					unknown = append(unknown, []string{str(q["key"]), str(q["value"])})
				}
			}
			parsed = object{"ok": true, "elements": pairs(m["elements"]), "path": pairs(m["pathElements"]), "query": pairs(m["queryElements"]), "unknown": unknown}
		}
		v := asMap(qr.GS1ValidateDigitalLink(input, opts))
		return object{"parse": parsed, "normalize": outcome(qr.GS1NormalizeDigitalLink(input, opts)), "validate": object{"ok": v["ok"], "errors": diagnostics(v["errors"]), "warnings": diagnostics(v["warnings"])}}
	case "create":
		return object{"create": outcome(qr.GS1CreateDigitalLink(gs1Elements(r["elements"]), qr.GS1DigitalLinkOptions{BaseURL: str(r["baseUrl"])}))}
	case "elements":
		e := gs1Elements(r["elements"])
		v := asMap(qr.GS1ValidateElements(e))
		return object{"validate": object{"ok": v["ok"], "errors": diagnostics(v["errors"])}, "string": outcome(qr.GS1ToElementString(e))}
	}
	bad("Unknown GS1 command")
	return nil
}
func diagnostics(v any) []object {
	out := []object{}
	if v == nil {
		return out
	}
	for _, x := range arr(v) {
		m := obj(x)
		p := object{}
		for _, k := range []string{"code", "reason", "ai", "value", "key", "offset", "elementIndex", "expected", "count"} {
			p[k] = m[k]
		}
		out = append(out, p)
	}
	return out
}
func dispatch(r object) object {
	command := "generate"
	if v, ok := r["command"]; ok {
		command = str(v)
	}
	switch command {
	case "identity":
		return identity(r)
	case "raw":
		return raw(r)
	case "merge":
		return merge(r)
	case "catalog", "url", "create", "elements":
		return gs1(r, command)
	case "gf":
		b := make([]byte, 0, 65536)
		for a := 0; a < 256; a++ {
			for c := 0; c < 256; c++ {
				b = append(b, byte(must(qr.GFMultiply(a, c))))
			}
		}
		return object{"bytes": hex.EncodeToString(b)}
	case "rs":
		degree := num(r["degree"])
		data := make([]byte, 300)
		for i := range data {
			data[i] = byte(i*61 + degree)
		}
		return object{"generator": hex.EncodeToString(must(qr.ReedSolomonDivisor(degree))), "remainder": hex.EncodeToString(must(qr.ReedSolomonRemainder(data, degree)))}
	case "concurrency":
		tasks := arr(r["requests"])
		if len(tasks) > 512 {
			bad("Unbounded concurrency")
		}
		for _, t := range tasks {
			if obj(t)["command"] == "concurrency" {
				bad("Nested concurrency")
			}
		}
		results := make([]object, len(tasks))
		var wg sync.WaitGroup
		for i, t := range tasks {
			wg.Add(1)
			go func(i int, t any) { defer wg.Done(); results[i] = safe(obj(t)) }(i, t)
		}
		wg.Wait()
		return object{"results": results}
	}
	opts := options(r["options"])
	if v, ok := r["pngScale"]; ok {
		opts.Render.Scale = num(v)
		opts.Render.Margin = 4
	}
	if command == "capacity" {
		c := must(qr.GetCapacity(opts.Version, opts.ECC, opts.Mode, 0))
		return object{"maximum": c.Maximum(), "dataCodewords": c.DataCodewords(), "capacityBits": c.CapacityBits(), "countBits": c.CharacterCountBits()}
	}
	text, data, binary := payload(r)
	if command == "estimate" {
		var p *qr.Plan
		if v, ok := r["segments"]; ok {
			p = must(qr.AnalyzeSegments(segments(v), opts))
		} else if binary {
			p = must(qr.EstimateBytes(data, opts))
		} else {
			p = must(qr.Estimate(text, opts))
		}
		out := object{"fits": p.OK(), "version": p.Version(), "requiredBits": p.RequiredBits(), "capacityBits": p.CapacityBits()}
		if r["includeDiagnostics"] == true {
			out["diagnostics"] = p.Diagnostics()
		}
		return out
	}
	if command == "structured-append" {
		return appendSymbols(r, opts, text, data, binary)
	}
	if command != "generate" {
		bad("Unknown conformance command")
	}
	var code *qr.QRCode
	if v, ok := r["segments"]; ok {
		code = must(qr.GenerateSegments(segments(v), opts))
	} else if binary {
		code = must(qr.GenerateBytes(data, opts))
	} else {
		code = must(qr.Generate(text, opts))
	}
	return symbol(code, r)
}
func safe(r object) (out object) {
	defer func() {
		if e := recover(); e != nil {
			if err, ok := e.(error); ok {
				out = typed(err)
			} else {
				out = object{"error": "Panic", "message": fmt.Sprint(e), "isSpecQRError": false, "code": nil}
			}
		}
	}()
	return dispatch(r)
}
func main() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 65536), 64<<20)
	writer := bufio.NewWriter(os.Stdout)
	defer writer.Flush()
	enc := json.NewEncoder(writer)
	enc.SetEscapeHTML(false)
	for scanner.Scan() {
		r, err := parse(scanner.Bytes())
		var out object
		if err != nil {
			out = typed(err)
		} else {
			out = safe(r)
		}
		fault := os.Getenv("SPECQR_TEST_FAULT")
		if r["command"] == "identity" {
			fault = ""
		}
		switch fault {
		case "exit":
			os.Exit(73)
		case "drop":
			continue
		case "error":
			out = object{"error": "InjectedFailure", "message": "Test-only negative control"}
		case "gs1-catalog":
			if _, ok := out["catalog"]; ok {
				out["catalog"] = object{"ok": true, "value": []string{"deliberate corruption"}}
			}
		case "sa-diagnostics":
			if v, ok := out["diagnostics"].(map[string]any); ok {
				v["parity"] = 999
			}
		case "merge-data":
			if _, ok := out["data"]; ok {
				out["data"] = "deliberate corruption"
			}
		case "matrixHash", "data", "codewords":
			if s, ok := out[fault].(string); ok && len(s) > 0 {
				b := []byte(s)
				i := 0
				if fault == "codewords" {
					i = len(b) - 1
				}
				if b[i] == '1' {
					b[i] = '0'
				} else {
					b[i] = '1'
				}
				out[fault] = string(b)
			}
		default:
			if strings.HasPrefix(fault, "leak-") {
				out = object{"error": strings.TrimPrefix(fault, "leak-"), "message": "Test-only untyped error leak", "isSpecQRError": false, "code": nil}
			}
		}
		if err := enc.Encode(out); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := writer.Flush(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
