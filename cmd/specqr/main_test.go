package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLI(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "q.json")
	if c := run([]string{"-text", "CLI 日本語", "-format", "json", "-output", out}); c != 0 {
		t.Fatal(c)
	}
	b, e := os.ReadFile(out)
	if e != nil || !json.Valid(b) {
		t.Fatalf("invalid output %v", e)
	}
	input := filepath.Join(dir, "raw")
	os.WriteFile(input, []byte{0, 255, 128}, 0600)
	if c := run([]string{"-input", input, "-binary", "-format", "png", "-output", filepath.Join(dir, "q.png")}); c != 0 {
		t.Fatal(c)
	}
	for _, args := range [][]string{{"-text", "FIRST", "-format", "json", "SECOND"}, {"-scale", "0"}, {"-mask", "-2"}, {"-input", input, "-text", "extra"}, {"-eci", "9999999"}, {"-format", "wat"}, {"-binary", "-segments", input}} {
		if c := run(args); c == 0 {
			t.Fatalf("invalid CLI accepted %v", args)
		}
	}
}
func TestStrictJSONUnicode(t *testing.T) {
	for _, s := range []string{`[{"mode":"byte","text":"\ud800"}]`, `[{"mode":"byte","text":"\udc00"}]`, "\xff"} {
		if _, e := parseSegments([]byte(s)); e == nil {
			t.Fatal("malformed unicode accepted")
		}
	}
	for _, s := range []string{`[{"mode":"byte","text":"\ud83d\ude00"}]`, `[{"mode":"byte","text":"\\ud800"}]`} {
		if _, e := parseSegments([]byte(s)); e != nil {
			t.Fatal(e)
		}
	}
	for _, s := range []string{`[{"mode":"byte","text":"a","text":"b"}]`, `[{"mode":"byte","Text":"a"}]`, `null`, `[{"mode":"byte","bytes":[null,65]}]`, `[{"mode":"numeric","bytes":[49,50,51]}]`, `[{"mode":"byte","text":"a","assignmentNumber":0}]`, `[{"mode":"eci","assignmentNumber":26,"text":"abc"}]`, `[{"mode":"byte","bytes":[-1]}]`, `[{"mode":"byte","unknown":1}]`, `[] {}`} {
		if _, e := parseSegments([]byte(s)); e == nil {
			t.Fatal("bad JSON accepted")
		}
	}
}

func TestRejectDeepJSON(t *testing.T) {
	if _, e := parseSegments([]byte(strings.Repeat("[", 100000) + "0" + strings.Repeat("]", 100000))); e == nil {
		t.Fatal("deep nesting accepted")
	}
}
