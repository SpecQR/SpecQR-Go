package specqr

import (
	"errors"
	"strings"
	"testing"
)

func TestStructuredAppendRoundTripAndCopies(t *testing.T) {
	text := strings.Repeat("日本語ABC123", 20)
	o := StructuredAppendOptions{Options: Options{Version: 2}, MaxSymbols: 16, SymbolDiagnostics: true}
	r, e := GenerateStructuredAppend(text, o)
	if e != nil {
		t.Fatal(e)
	}
	if r.Total() < 2 {
		t.Fatal("not split")
	}
	parts := []SAPart{}
	for i, q := range r.Symbols() {
		s := ""
		for _, seg := range q.Segments() {
			if !seg.IsControl() {
				s += seg.Text()
			}
		}
		parts = append([]SAPart{{Index: i + 1, Total: r.Total(), Parity: r.Parity(), Text: &s}}, parts...)
	}
	m, e := MergeStructuredAppendParts(parts)
	if e != nil || m.Text() != text {
		t.Fatalf("merge: %v", e)
	}
	parts[0].Index = 99
	copy := m.Parts()
	*copy[0].Text = "changed"
	if m.Text() != text || *m.Parts()[0].Text == "changed" {
		t.Fatal("merge alias")
	}
	a := r.Symbols()
	a[0] = nil
	if r.Symbols()[0] == nil {
		t.Fatal("symbols alias")
	}
	d := r.Diagnostics()
	d["symbols"].([]any)[0].(map[string]any)["index"] = 99
	if r.Diagnostics()["symbols"].([]any)[0].(map[string]any)["index"] == 99 {
		t.Fatal("diagnostic alias")
	}
}
func TestStructuredAppendErrorsAndManual(t *testing.T) {
	for _, s := range []string{"", "a"} {
		if _, e := GenerateStructuredAppend(s, StructuredAppendOptions{}); e == nil {
			t.Fatal("one-symbol input accepted")
		}
	}
	for _, o := range []StructuredAppendOptions{{MaxSymbols: 1}, {Options: Options{GS1: true}}, {Options: Options{ECI: new(0)}}, {Options: Options{BoostECC: true}}} {
		if _, e := GenerateStructuredAppend("ABC", o); e == nil {
			t.Fatal("bad option accepted")
		}
	}
	s, _ := UTF8Segment(strings.Repeat("😀", 80))
	r, e := GenerateSegmentsStructuredAppend([]Segment{s}, StructuredAppendOptions{Options: Options{Version: 2}, FullSplitUnits: true})
	if e != nil {
		t.Fatal(e)
	}
	for _, q := range r.Symbols() {
		for _, v := range q.Segments() {
			if !v.IsControl() && !strings.Contains(v.Text(), "😀") {
				t.Fatal("split UTF8 broken")
			}
		}
	}
	if _, e := GenerateBytesStructuredAppend(make([]byte, 1_000_001), StructuredAppendOptions{}); e == nil {
		t.Fatal("oversized raw accepted")
	}
	a, b := "a", "b"
	parity := int('a' ^ 'b')
	good := []SAPart{{Index: 2, Total: 2, Parity: parity, Text: &b}, {Index: 1, Total: 2, Parity: parity, Text: &a}}
	if _, e := MergeStructuredAppendParts(good); e != nil {
		t.Fatal(e)
	}
	for _, bad := range [][]SAPart{nil, good[:1], {good[0], good[0]}, {{Index: 1, Total: 2, Parity: 0, Text: &a}, {Index: 2, Total: 2, Parity: 0, Text: &b}}, {{Index: 1, Total: 2, Parity: parity, Text: &a}, {Index: 2, Total: 2, Parity: parity, Bytes: []byte("b"), Binary: true}}} {
		_, e := MergeStructuredAppendParts(bad)
		var typed *Error
		if !errors.As(e, &typed) {
			t.Fatalf("bad merge did not reject typed: %v", e)
		}
	}
}
func FuzzMergeNoPanic(f *testing.F) {
	f.Add("abc", 1, 2, 12)
	f.Fuzz(func(t *testing.T, s string, i, n, p int) {
		if len(s) > 256 {
			t.Skip()
		}
		_, _ = MergeStructuredAppendParts([]SAPart{{Index: i, Total: n, Parity: p, Text: &s}})
	})
}
