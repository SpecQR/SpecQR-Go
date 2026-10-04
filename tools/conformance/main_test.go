package main

import (
	"strings"
	"testing"
)

func TestJSONUnicodeBoundary(t *testing.T) {
	for _, line := range []string{`{"text":"\ud800"}`, `{"text":"\udfff"}`, `{"text":"x\ud800z"}`, `{"text":"\ud800\u0020"}`, "{\"text\":\"\xff\"}"} {
		if _, err := parse([]byte(line)); err == nil {
			t.Errorf("accepted invalid Unicode %q", line)
		}
	}
	for _, line := range []string{`{"text":"\ud83d\ude00"}`, `{"text":"\\ud800"}`, `{"text":"é漢字🙂"}`} {
		if _, err := parse([]byte(line)); err != nil {
			t.Errorf("rejected valid Unicode %q: %v", line, err)
		}
	}
	for _, line := range []string{`null`, `[]`, `{} {}`, `{"text":`} {
		if _, err := parse([]byte(line)); err == nil {
			t.Errorf("accepted malformed object %q", line)
		}
	}
}
func TestSHAAndMatrixRows(t *testing.T) {
	if digest(nil) != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Fatal("SHA256 vector mismatch")
	}
	if strings.Join(rows([][]bool{{true, false}, {false, true}}), "") != "1001" {
		t.Fatal("matrix serialization mismatch")
	}
}
