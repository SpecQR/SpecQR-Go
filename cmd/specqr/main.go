// Command specqr encodes QR Model 2 text, binary files, or manual segments.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	qr "github.com/SpecQR/SpecQR-Go"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

const inputLimit = 4_000_000

func fail(err error) int {
	code := qr.InvalidInput
	var typed *qr.Error
	if errors.As(err, &typed) {
		code = typed.Code
	}
	b, _ := json.Marshal(map[string]any{"error": code, "message": err.Error()})
	fmt.Fprintln(os.Stderr, string(b))
	return 2
}
func readFile(path string) ([]byte, error) {
	var r io.Reader
	var f *os.File
	if path == "-" {
		r = os.Stdin
	} else {
		var e error
		f, e = os.Open(path)
		if e != nil {
			return nil, e
		}
		defer f.Close()
		r = f
	}
	b, e := io.ReadAll(io.LimitReader(r, inputLimit+1))
	if e != nil {
		return nil, e
	}
	if len(b) > inputLimit {
		return nil, &qr.Error{Code: qr.ResourceLimit, Message: "input exceeds 4 MB CLI budget"}
	}
	return b, nil
}
func rows(q *qr.QRCode) []string {
	m := q.Matrix()
	r := make([]string, len(m))
	for y, row := range m {
		b := make([]byte, len(row))
		for x, v := range row {
			b[x] = '0'
			if v {
				b[x] = '1'
			}
		}
		r[y] = string(b)
	}
	return r
}

type segmentJSON struct {
	Mode                 string  `json:"mode"`
	Text                 *string `json:"text,omitempty"`
	Data                 *string `json:"data,omitempty"`
	Bytes                *[]*int `json:"bytes,omitempty"`
	Assignment           *int    `json:"assignmentNumber,omitempty"`
	ApplicationIndicator *string `json:"applicationIndicator,omitempty"`
	Index                *int    `json:"index,omitempty"`
	Total                *int    `json:"total,omitempty"`
	Parity               *int    `json:"parity,omitempty"`
}

// Reject malformed JSON Unicode before encoding/json's documented replacement.
func strictJSONUnicode(b []byte) bool {
	if !utf8.Valid(b) {
		return false
	}
	for i := 0; i < len(b); i++ {
		if b[i] != '\\' {
			continue
		}
		i++
		if i >= len(b) {
			return false
		}
		if b[i] != 'u' {
			continue
		}
		if i+4 >= len(b) {
			return false
		}
		n, e := strconv.ParseUint(string(b[i+1:i+5]), 16, 16)
		if e != nil {
			return false
		}
		i += 4
		if n >= 0xd800 && n <= 0xdbff {
			if i+6 >= len(b) || b[i+1] != '\\' || b[i+2] != 'u' {
				return false
			}
			lo, e := strconv.ParseUint(string(b[i+3:i+7]), 16, 16)
			if e != nil || lo < 0xdc00 || lo > 0xdfff {
				return false
			}
			i += 6
		} else if n >= 0xdc00 && n <= 0xdfff {
			return false
		}
	}
	return true
}

// validateJSONTokens rejects null and duplicate object keys instead of allowing
// encoding/json to replace data or silently keep the final field value.
func validateJSONTokens(d *json.Decoder, depth int) error {
	if depth > 4 {
		return fmt.Errorf("segments JSON nesting exceeds schema limit")
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	if token == nil {
		return fmt.Errorf("null is not a segment value")
	}
	delimiter, composite := token.(json.Delim)
	if !composite {
		return nil
	}
	switch delimiter {
	case '[':
		for d.More() {
			if err := validateJSONTokens(d, depth+1); err != nil {
				return err
			}
		}
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return fmt.Errorf("duplicate or invalid segment field")
			}
			seen[name] = true
			switch name {
			case "mode", "text", "data", "bytes", "assignmentNumber", "applicationIndicator", "index", "total", "parity":
			default:
				return fmt.Errorf("unknown segment field %q", name)
			}
			if err := validateJSONTokens(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter")
	}
	_, err = d.Token()
	return err
}

func parseSegments(b []byte) ([]qr.Segment, error) {
	if len(strings.TrimSpace(string(b))) == 0 || strings.TrimSpace(string(b))[0] != '[' {
		return nil, fmt.Errorf("segments JSON must be an array")
	}
	if !strictJSONUnicode(b) {
		return nil, fmt.Errorf("segments JSON must contain valid Unicode scalar strings")
	}
	validator := json.NewDecoder(strings.NewReader(string(b)))
	validator.UseNumber()
	if e := validateJSONTokens(validator, 0); e != nil {
		return nil, e
	}
	var a []segmentJSON
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if e := d.Decode(&a); e != nil {
		return nil, e
	}
	var trailing any
	if e := d.Decode(&trailing); e != io.EOF {
		return nil, fmt.Errorf("segments JSON has trailing data")
	}
	if len(a) > qr.MaxManualSegments {
		return nil, fmt.Errorf("too many segments")
	}
	ss := make([]qr.Segment, 0, len(a))
	for _, v := range a {
		dataMode := v.Mode == "numeric" || v.Mode == "alphanumeric" || v.Mode == "kanji" || v.Mode == "byte"
		if v.Bytes != nil && v.Mode != "byte" {
			return nil, fmt.Errorf("bytes is valid only for byte mode")
		}
		if !dataMode && (v.Text != nil || v.Data != nil || v.Bytes != nil) {
			return nil, fmt.Errorf("control segment cannot contain payload")
		}
		if dataMode && v.Text == nil && v.Data == nil && v.Bytes == nil {
			return nil, fmt.Errorf("data segment requires explicit payload")
		}
		if v.Assignment != nil && v.Mode != "eci" {
			return nil, fmt.Errorf("assignmentNumber is valid only for ECI")
		}
		if v.ApplicationIndicator != nil && v.Mode != "fnc1-second" {
			return nil, fmt.Errorf("applicationIndicator is valid only for FNC1 second")
		}
		if (v.Index != nil || v.Total != nil || v.Parity != nil) && v.Mode != "structured-append" {
			return nil, fmt.Errorf("append metadata is valid only for Structured Append")
		}
		text := ""
		if v.Text != nil {
			text = *v.Text
		}
		if v.Data != nil {
			if v.Text != nil {
				return nil, fmt.Errorf("segment cannot specify both data and text")
			}
			text = *v.Data
		}
		var s qr.Segment
		var e error
		switch v.Mode {
		case "numeric":
			s, e = qr.NumericSegment(text)
		case "alphanumeric":
			s, e = qr.AlphanumericSegment(text)
		case "kanji":
			s, e = qr.KanjiSegment(text)
		case "byte":
			if v.Bytes != nil {
				if v.Text != nil || v.Data != nil {
					return nil, fmt.Errorf("byte segment cannot mix text and bytes")
				}
				b := make([]byte, len(*v.Bytes))
				for i, n := range *v.Bytes {
					if n == nil || *n < 0 || *n > 255 {
						return nil, fmt.Errorf("byte must be 0..255")
					}
					b[i] = byte(*n)
				}
				s, e = qr.BytesSegment(b)
			} else {
				s, e = qr.UTF8Segment(text)
			}
		case "eci":
			if v.Assignment == nil {
				return nil, fmt.Errorf("ECI assignmentNumber is required")
			}
			s, e = qr.ECISegment(*v.Assignment)
		case "fnc1":
			s, e = qr.FNC1Segment()
		case "fnc1-second":
			if v.ApplicationIndicator == nil {
				return nil, fmt.Errorf("FNC1 second requires applicationIndicator")
			}
			s, e = qr.FNC1SecondSegment(*v.ApplicationIndicator)
		case "structured-append":
			if v.Index == nil || v.Total == nil || v.Parity == nil {
				return nil, fmt.Errorf("Structured Append requires index, total and parity")
			}
			s, e = qr.StructuredAppendSegment(*v.Index, *v.Total, *v.Parity)
		default:
			return nil, fmt.Errorf("unknown segment mode %q", v.Mode)
		}
		if e != nil {
			return nil, e
		}
		ss = append(ss, s)
	}
	return ss, nil
}
func run(args []string) int {
	f := flag.NewFlagSet("specqr", flag.ContinueOnError)
	text := f.String("text", "", "UTF-8 payload (or one positional argument)")
	input := f.String("input", "", "input file; - reads standard input")
	binary := f.Bool("binary", false, "treat input as arbitrary bytes")
	segments := f.String("segments", "", "manual segment JSON file; - reads standard input")
	output := f.String("output", "-", "output file; - writes stdout")
	format := f.String("format", "svg", "svg, png, json, matrix, svg-data-url or png-data-url")
	ecc := f.String("ecc", "M", "L, M, Q or H")
	mode := f.String("mode", "auto", "auto, numeric, alphanumeric, byte or kanji")
	version := f.Int("version", 0, "fixed version 1..40; 0 automatic")
	minimum := f.Int("min-version", 1, "smallest automatic version")
	maximum := f.Int("max-version", 40, "largest automatic version")
	mask := f.Int("mask", -1, "mask 0..7; -1 automatic")
	eci := f.Int("eci", -1, "ECI assignment 0..999999; -1 disabled")
	gs1 := f.Bool("gs1", false, "validate GS1 element string and add FNC1")
	fnc := f.String("fnc1-second", "", "two digits or one ASCII Latin letter")
	boost := f.Bool("boost-ecc", false, "increase ECC within selected version")
	noopt := f.Bool("no-optimize", false, "disable mixed-mode optimization")
	margin := f.Int("margin", 4, "quiet-zone width in modules")
	scale := f.Int("scale", 8, "pixels per module")
	fg := f.String("foreground", "#000000", "dark color")
	bg := f.String("background", "#ffffff", "light color")
	dpi := f.Float64("dpi", 0, "optional print DPI")
	sa := f.Bool("structured-append", false, "generate a set (json output only)")
	maxSymbols := f.Int("max-symbols", 16, "Structured Append symbol limit 2..16")
	showVersion := f.Bool("version-info", false, "print edition identifier")
	if e := f.Parse(args); e != nil {
		if e == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *showVersion {
		fmt.Fprintln(os.Stdout, qr.Version)
		return 0
	}
	if len(f.Args()) > 1 {
		return fail(fmt.Errorf("at most one positional text payload is allowed"))
	}
	providedText := false
	f.Visit(func(v *flag.Flag) {
		if v.Name == "text" {
			providedText = true
		}
	})
	if providedText && len(f.Args()) == 1 {
		return fail(fmt.Errorf("select either -text or positional text"))
	}
	sources := 0
	if providedText || len(f.Args()) == 1 {
		sources++
	}
	if *input != "" {
		sources++
	}
	if *segments != "" {
		sources++
	}
	if sources > 1 {
		return fail(fmt.Errorf("select one text, input or segments source"))
	}
	if len(f.Args()) == 1 {
		*text = f.Args()[0]
	}
	if *binary && *segments != "" {
		return fail(fmt.Errorf("binary cannot combine with segments"))
	}
	if *mask < -1 || *eci < -1 {
		return fail(fmt.Errorf("mask/eci automatic sentinel is -1"))
	}
	o := qr.DefaultOptions()
	o.ECC = qr.ECC(*ecc)
	o.Mode = qr.Mode(*mode)
	o.Version = *version
	o.MinVersion = *minimum
	o.MaxVersion = *maximum
	o.DisableOptimization = *noopt
	o.BoostECC = *boost
	o.GS1 = *gs1
	o.PrintDPI = *dpi
	o.Render = qr.RenderOptions{Margin: *margin, Scale: *scale, Foreground: *fg, Background: *bg}
	if *mask >= 0 {
		o.Mask = mask
	}
	if *eci >= 0 {
		o.ECI = eci
	}
	if *fnc != "" {
		o.FNC1Second = fnc
	}
	if e := o.Validate(); e != nil {
		return fail(e)
	}
	data := []byte(*text)
	var e error
	if *input != "" {
		data, e = readFile(*input)
		if e != nil {
			return fail(e)
		}
	}
	var ss []qr.Segment
	if *segments != "" {
		b, e := readFile(*segments)
		if e != nil {
			return fail(e)
		}
		ss, e = parseSegments(b)
		if e != nil {
			return fail(e)
		}
	}
	var q *qr.QRCode
	var out []byte
	if *sa {
		if *format != "json" {
			return fail(fmt.Errorf("Structured Append requires -format json"))
		}
		opts := qr.StructuredAppendOptions{Options: o, MaxSymbols: *maxSymbols, SymbolDiagnostics: true}
		var set *qr.SAResult
		if *segments != "" {
			set, e = qr.GenerateSegmentsStructuredAppend(ss, opts)
		} else if *binary {
			set, e = qr.GenerateBytesStructuredAppend(data, opts)
		} else {
			set, e = qr.GenerateStructuredAppend(string(data), opts)
		}
		if e != nil {
			return fail(e)
		}
		symbols := []any{}
		for _, v := range set.Symbols() {
			symbols = append(symbols, map[string]any{"matrix": rows(v), "diagnostics": v.Diagnostics()})
		}
		out, e = json.MarshalIndent(map[string]any{"symbols": symbols, "diagnostics": set.Diagnostics()}, "", "  ")
	} else {
		if *segments != "" {
			q, e = qr.GenerateSegments(ss, o)
		} else if *binary {
			q, e = qr.GenerateBytes(data, o)
		} else {
			q, e = qr.Generate(string(data), o)
		}
		if e != nil {
			return fail(e)
		}
		var s string
		switch *format {
		case "svg":
			s, e = q.ToSVG()
			out = []byte(s)
		case "png":
			out, e = q.ToPNG()
		case "json":
			out, e = json.MarshalIndent(map[string]any{"matrix": rows(q), "diagnostics": q.Diagnostics()}, "", "  ")
		case "matrix":
			out = []byte(strings.Join(rows(q), "\n") + "\n")
		case "svg-data-url":
			s, e = q.ToSVGDataURL()
			out = []byte(s)
		case "png-data-url":
			s, e = q.ToPNGDataURL()
			out = []byte(s)
		default:
			return fail(fmt.Errorf("unsupported output format %q", *format))
		}
	}
	if e != nil {
		return fail(e)
	}
	if *output == "-" {
		_, e = os.Stdout.Write(out)
	} else {
		e = os.WriteFile(*output, out, 0644)
	}
	if e != nil {
		return fail(e)
	}
	return 0
}
func main() { os.Exit(run(os.Args[1:])) }
