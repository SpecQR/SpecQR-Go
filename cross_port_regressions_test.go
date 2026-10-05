package specqr

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
)

// These assert the established Go safety contract, not the TS encoder's exact
// segment choices. In particular forced alpha refuses literal FNC1 percent.
func TestCrossPortFNC1LiteralCorpus(t *testing.T) {
	payloads := []string{"10ABC%DEF", "10ABC%%DEF", "10%ABC", "10ABC%", "10ABC%\x1d21DEF%", "10" + strings.Repeat("A", 19) + "%", "ABC%DEF%%", "漢字A%AAAAAA"}
	for _, second := range []string{"", "37", "A"} {
		for _, text := range payloads {
			if second == "" && !strings.HasPrefix(text, "10") {
				continue
			}
			for _, disabled := range []bool{false, true} {
				for _, mode := range []Mode{Auto, Byte, Alphanumeric} {
					o := DefaultOptions()
					o.GS1 = second == ""
					if second != "" {
						v := second
						o.FNC1Second = &v
					}
					o.DisableOptimization = disabled
					o.Mode = mode
					q, e := Generate(text, o)
					p, pe := Estimate(text, o)
					if mode == Alphanumeric {
						for _, err := range []error{e, pe} {
							var typed *Error
							if !errors.As(err, &typed) || typed.Code != InvalidMode {
								t.Fatalf("%q: expected InvalidMode: %v", text, err)
							}
						}
						continue
					}
					if e != nil || pe != nil || !p.OK() {
						t.Fatalf("%q: %v / %v", text, e, pe)
					}
					if q.Segments()[1].Mode() != Byte || string(q.Segments()[1].LogicalBytes()) != text {
						t.Fatalf("literal payload changed: %q", text)
					}
					if q.Diagnostics()["dataBitLength"] != p.Diagnostics()["dataBitLength"] {
						t.Fatal("plan/generate disagreement")
					}
				}
			}
		}
	}
	for _, text := range []string{"ABC%DEF", "ABC%%DEF"} {
		alpha, e := AlphanumericSegment(text)
		if e != nil {
			t.Fatal(e)
		}
		f, _ := FNC1Segment()
		q, e := GenerateSegments([]Segment{f, alpha}, Options{})
		if e != nil || q.Segments()[1].Text() != text {
			t.Fatal("manual data altered", e)
		}
	}
	o := DefaultOptions()
	o.GS1 = true
	o.Version = 1
	o.ECC = L
	p, e := Estimate("10"+strings.Repeat("A", 19)+"%", o)
	if e != nil || p.OK() {
		t.Fatal("byte fallback capacity boundary", e)
	}
	if _, e = Generate("10"+strings.Repeat("A", 19)+"%", o); e == nil {
		t.Fatal("overflow generated")
	}
	if _, e = Generate(strings.Repeat("%", MaxPayloadUnits+1), o); e == nil {
		t.Fatal("resource budget bypassed")
	}
}

func TestCrossPortDigitalLinkPreservation(t *testing.T) {
	root := "https://example.com/01/04912345678904"
	for _, v := range []struct{ raw, value string }{{".", "."}, {"..", ".."}, {"%2e", "."}, {"%2E%2e", ".."}, {".%2E", ".."}, {"%2e.", ".."}} {
		uri := root + "/10/" + v.raw
		p, e := GS1ParseDigitalLink(uri)
		if e != nil || p.Elements[1].Value != v.value || !GS1ValidateDigitalLink(uri).OK {
			t.Fatal(uri, e)
		}
		want := root + "?10=" + v.value
		got, e := GS1NormalizeDigitalLink(uri)
		if e != nil || got != want {
			t.Fatal(got, e)
		}
		got, e = GS1CreateDigitalLink([]GS1Element{{"01", "04912345678904"}, {"10", v.value}}, GS1DigitalLinkOptions{BaseURL: "https://example.com", PathAIs: []string{"10"}})
		if e != nil || got != want {
			t.Fatal(got, e)
		}
	}
	lost := "https://example.com/01/./../01/04912345678904"
	if _, e := GS1ParseDigitalLink(lost); e == nil {
		t.Fatal("erased invalid primary accepted")
	}
	if GS1ValidateDigitalLink(lost).OK {
		t.Fatal("validated erased primary")
	}
	if _, e := GS1NormalizeDigitalLink(lost); e == nil {
		t.Fatal("normalized erased primary")
	}
	query := root + "?10=..&21=.&utm=a&utm=b"
	got, e := GS1NormalizeDigitalLink(query)
	if e != nil || got != query {
		t.Fatal(got, e)
	}
	got, e = GS1CreateDigitalLink([]GS1Element{{"01", "04912345678904"}, {"10", "%2e"}}, GS1DigitalLinkOptions{BaseURL: "https://example.com"})
	if e != nil || got != root+"/10/%252e" {
		t.Fatal(got, e)
	}
	got, e = GS1CreateDigitalLink([]GS1Element{{"01", "04912345678904"}}, GS1DigitalLinkOptions{BaseURL: "https://example.com/a/../b"})
	if e != nil || got != "https://example.com/b/01/04912345678904" {
		t.Fatal(got, e)
	}
}

func TestCrossPortPrintAndECCGuards(t *testing.T) {
	// Go validates against the maximum version extent, at options time, even when
	// a caller does not inspect diagnostics. This established stricter API remains.
	for _, dpi := range []float64{math.SmallestNonzeroFloat64, 1e-305, 1e-304} {
		for _, v := range []int{1, 40} {
			o := DefaultOptions()
			o.PrintDPI = dpi
			o.Version = v
			if e := o.Validate(); e == nil {
				t.Fatalf("unsafe dpi %g accepted", dpi)
			}
			if _, e := Generate("A", o); e == nil {
				t.Fatal("generated unsafe geometry")
			}
		}
	}
	for _, dpi := range []float64{1e-300, 300, math.MaxFloat64} {
		o := DefaultOptions()
		o.PrintDPI = dpi
		q, e := Generate("A", o)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = json.Marshal(q.Diagnostics()); e != nil {
			t.Fatal("nonfinite diagnostics", e)
		}
	}
	for _, ecc := range []ECC{"invalid", "constructor", "toString", "valueOf", "__proto__", "hasOwnProperty"} {
		o := DefaultOptions()
		o.ECC = ecc
		s, _ := UTF8Segment("A")
		_, e1 := Generate("A", o)
		_, e2 := Estimate("A", o)
		_, e3 := GenerateSegments([]Segment{s}, o)
		_, e4 := AnalyzeSegments([]Segment{s}, o)
		_, e5 := GetCapacity(1, ecc, Byte, 0)
		_, e6 := GenerateStructuredAppend(strings.Repeat("A", 100), StructuredAppendOptions{Options: o})
		_, e7 := GenerateSegmentsStructuredAppend([]Segment{s}, StructuredAppendOptions{Options: o})
		for _, e := range []error{e1, e2, e3, e4, e5, e6, e7} {
			var typed *Error
			if !errors.As(e, &typed) || typed.Code != InvalidECC {
				t.Fatal(ecc, e)
			}
		}
	}
	for _, ecc := range []ECC{L, M, Q, H} {
		o := DefaultOptions()
		o.ECC = ecc
		q, e := Generate("A", o)
		if e != nil || q.ECC() != ecc {
			t.Fatal(ecc, e)
		}
	}
	o := DefaultOptions()
	a, _ := Generate("A", o)
	o.PrintDPI = 300
	b, _ := Generate("A", o)
	if !reflect.DeepEqual(a.Matrix(), b.Matrix()) {
		t.Fatal("normal DPI altered matrix")
	}
}
