package specqr

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestGS1URLAuthority(t *testing.T) {
	for _, tc := range []struct{ raw, canonical string }{
		{"EXAMPLE.COM.", "example.com."},
		{"%65xample.com", "example.com"},
		{"0x7f000001", "127.0.0.1"},
		{"0177.1", "127.0.0.1"},
		{"127.1", "127.0.0.1"},
		{"4294967295", "255.255.255.255"},
		{"0X7F.1", "127.0.0.1"},
		{"0x", "0.0.0.0"},
		{"0", "0.0.0.0"},
		{"1.2.65535", "1.2.255.255"},
		{"[0:0:0:1:0:0:0:1]", "[::1:0:0:0:1]"},
		{"[::ffff:192.0.2.128]", "[::ffff:c000:280]"},
		{"[1:0:2:0:3:0:4:0]", "[1:0:2:0:3:0:4:0]"},
		{"[0:0:0:0:0:0:0:0]", "[::]"},
		{"[1:0:0:2:0:0:3:4]", "[1::2:0:0:3:4]"},
		{"[1:2:3:4:5:6:0:0]", "[1:2:3:4:5:6::]"},
		{"foo_bar.test", "foo_bar.test"},
		{"foo..bar", "foo..bar"},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			u, err := gs1ParseURL("https://"+tc.raw+"/01/04912345678904", false)
			if err != nil {
				t.Fatal(err)
			}
			if u.authority != tc.canonical {
				t.Fatalf("authority = %q, want %q", u.authority, tc.canonical)
			}
		})
	}
	for _, host := range []string{
		"4294967296", "1.2.3.256", "1.2.3.4.5", "09", "08", "a%2fb", "%ff", "[:::]", "[1:2]",
		"[::%25eth0]", "[::ffff:192.00.2.128]", "[127.0.0.1]", "1.2.65536", "256.0.0.1", "example.1", "example.0x",
		"example%25.com", "example%00.com", "example%20.com", "example.com:65536", "example.com:-1", "example.com:+80", "example.com:80:80",
		"[::]suffix", "[::]:65536", "@", "xn--a", "xn--", "xn--abc", "xn--0.pt", "xn--abc-",
	} {
		if _, err := gs1ParseURL("https://"+host+"/01/04912345678904", false); err == nil {
			t.Errorf("accepted host %q", host)
		}
	}
}

func TestGS1URLCanonicalForms(t *testing.T) {
	for _, tc := range []struct{ raw, canonical string }{
		{" \x00HTTPS:\\EXAMPLE.com:0443/01/04912345678904\r\n", "https://example.com/01/04912345678904"},
		{"http:example.com:00080", "http://example.com/"},
		{"https:///example.com:00000", "https://example.com:0/"},
		{"https://user:pa:ss@EXAMPLE.com:443/path", "https://user:pa%3Ass@example.com/path"},
		{"https://u@x:p@EXAMPLE.com:00444/path", "https://u%40x:p@example.com:444/path"},
		{"https://user:@example.com", "https://user@example.com/"},
		{"https://:@example.com", "https://example.com/"},
		{"https://:password@example.com", "https://:password@example.com/"},
		{"https://u;=|[]:p[]@example.com", "https://u%3B%3D%7C%5B%5D:p%5B%5D@example.com/"},
		{"https://ü:€@example.com", "https://%C3%BC:%E2%82%AC@example.com/"},
		{"https://example.com/a b/é?x='é' #", "https://example.com/a%20b/%C3%A9?x=%27%C3%A9%27%20#"},
		{"https://exa\tmple.com/a\rb\nc", "https://example.com/abc"},
		{"https://example.com\\a\\b", "https://example.com/a/b"},
		{"https://example.com/a/../b/./c", "https://example.com/b/c"},
		{"https://example.com/a/%2e%2E", "https://example.com/"},
		{"https://example.com/a/..", "https://example.com/"},
		{"https://example.com/../../", "https://example.com/"},
		{"https://example.com/path?#", "https://example.com/path?#"},
	} {
		u, err := gs1ParseURL(tc.raw, false)
		if err != nil {
			t.Errorf("%q: %v", tc.raw, err)
			continue
		}
		got, err := u.serialize()
		if err != nil || got != tc.canonical {
			t.Errorf("%q: got %q (%v), want %q", tc.raw, got, err, tc.canonical)
		}
	}
	for _, raw := range []string{"", "https", ":example.com", "1http://example.com", "ht^tp://example.com", "https://", "https:////"} {
		if _, err := gs1ParseURL(raw, false); err == nil {
			t.Errorf("accepted invalid URL %q", raw)
		}
	}
}

func TestGS1URLDotValuesAndPolicy(t *testing.T) {
	for _, token := range []string{".", "..", "%2e", "%2E.", ".%2E", "%2e%2e"} {
		for _, primary := range []string{"00", "01", "414"} {
			u, err := gs1ParseURL("https://example.com/a/../"+primary+"/123/10/"+token, false)
			if err != nil {
				t.Fatal(err)
			}
			if want := "/" + primary + "/123/10/" + token; u.path != want {
				t.Fatalf("dot value lost: got %q, want %q", u.path, want)
			}
		}
	}
	u, err := gs1ParseURL("https://example.com/01/123/..", true)
	if err != nil || u.path != "/01/" {
		t.Fatalf("base dot normalization: %q, %v", u.path, err)
	}
	for _, tc := range []struct {
		raw         string
		base, valid bool
	}{
		{"https://example.com", true, true}, {"https://example.com?#", true, true},
		{"https://example.com?x", true, false}, {"https://example.com#x", true, false},
		{"https://example.com?x", false, true}, {"https://example.com#", false, true}, {"https://example.com#x", false, false},
		{"http://example.com", false, true}, {"ftp://example.com:21", false, false}, {"ws://example.com", false, false},
		{"file:///x", false, false}, {"mailto:x@y.test", false, false},
	} {
		u, err := gs1ParseURL(tc.raw, tc.base)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.raw, err)
		}
		if got := gs1CheckURI(u, tc.base) == nil; got != tc.valid {
			t.Errorf("policy %q base %v: valid %v, want %v", tc.raw, tc.base, got, tc.valid)
		}
	}
}

func TestGS1URLUnicodeHostProfile(t *testing.T) {
	for _, tc := range []struct{ raw, ascii string }{
		{"例え.テスト", "xn--r8jz45g.xn--zckzah"}, {"faß.de", "xn--fa-hia.de"}, {"😀.test", "xn--e28h.test"},
		{"é.test", "xn--9ca.test"}, {"ｅｘａｍｐｌｅ.com", "example.com"}, {"a。b", "a.b"}, {"a．b｡c", "a.b.c"}, {"ẞ.de", "xn--zca.de"},
		{"bücher.example", "xn--bcher-kva.example"}, {"βόλος.example", "xn--nxasmm1c.example"}, {"пример.example", "xn--e1afmkfd.example"},
		{"אב.example", "xn--4dbc.example"}, {"ا١.example", "xn--mgb0j.example"}, {"한글.example", "xn--bj0bj06e.example"},
		{"☃.example", "xn--n3h.example"}, {"a\u00adb.example", "ab.example"}, {"%C3%89.example", "xn--9ca.example"},
	} {
		got, err := gs1Host(tc.raw)
		if err != nil || got != tc.ascii {
			t.Errorf("host %q: got %q (%v), want %q", tc.raw, got, err, tc.ascii)
		}
		again, err := gs1Host(tc.ascii)
		if err != nil || again != tc.ascii {
			t.Errorf("ACE roundtrip %q: %q, %v", tc.ascii, again, err)
		}
	}
	for _, host := range []string{
		"a\u200cb.test", "a\u200db.test", "\u0600.test", "e\u0301.com", "\u034f.example", "xn--a-ecp.example", "xn--e-xbb.com",
		"ﬁ.example", "①.example", "塚.example", "\ufe0f.example", "xn--jm6c.example", "xn--orh.example", "xn--nf6c.example", "xn--v86c.example",
		"אa.example", "aא.example", "xn--a-zhc.example", "א-.example", "ا1١.example", "가.example", "Ꭰ.example", "ᏸ.example", "\u1c80.example",
		"\u1f80.example", "\u2ff0.example", "\ufffc.example", "\ufffd.example", "\U000f0000.example", "\U0010ffff.example", "\u0378.example",
	} {
		if got, err := gs1Host(host); err == nil {
			t.Errorf("accepted unsupported host %q as %q", host, got)
		}
	}
}

func TestGS1URLPercentAndReplacementDecoding(t *testing.T) {
	for _, value := range []string{"%", "%0", "%GG", "%aG", "%FF", "%ED%A0%80", "%E2%82", "%C0%AF", "%F4%90%80%80", "\xff"} {
		if _, err := gs1StrictDecode(value, "test"); err == nil {
			t.Errorf("strict decoder accepted %q", value)
		}
	}
	for _, tc := range []struct{ raw, decoded string }{
		{"A%2FB", "A/B"}, {"A+B", "A+B"}, {"%00%1D", "\x00\x1d"}, {"%F0%9F%98%80", "😀"}, {"%252e", "%2e"},
	} {
		got, err := gs1StrictDecode(tc.raw, "test")
		if err != nil || got != tc.decoded {
			t.Errorf("strict %q: %q, %v", tc.raw, got, err)
		}
	}
	for _, tc := range []struct{ raw, decoded string }{
		{"a+b", "a b"}, {"%ED%A0%80", "���"}, {"%E2%82", "�"}, {"%E2%82A", "�A"}, {"%F0%9F%98%80", "😀"}, {"%00", "\x00"},
		{"%GG", "%GG"}, {"%", "%"}, {"%C0%AF", "��"}, {"%F4%90%80%80", "����"}, {"%F0%90%80", "�"},
		{"%F0%90%80A", "�A"}, {"%EF%BF%BD", "�"}, {"%E0%80%80", "���"}, {"%80%80", "��"}, {"%E1%80%E1%80", "��"},
	} {
		if got := gs1FormDecode(tc.raw); got != tc.decoded {
			t.Errorf("form %q: got %q, want %q", tc.raw, got, tc.decoded)
		}
	}
	for _, tc := range []struct {
		text    string
		mode    byte
		encoded string
	}{
		{"A B/é~!*'()-._", 0, "A%20B%2F%C3%A9~!*'()-._"},
		{"A B/é~!*'()-._", 1, "A+B%2F%C3%A9%7E%21*%27%28%29-._"},
		{"/a b^{}?\"`#<>é", 2, "/a%20b%5E%7B%7D%3F%22%60%23%3C%3E%C3%A9"},
		{"a b'\"#<>é/?", 3, "a%20b%27%22%23%3C%3E%C3%A9/?"},
	} {
		if got := gs1Encode(tc.text, tc.mode); got != tc.encoded {
			t.Errorf("encode mode %d: got %q, want %q", tc.mode, got, tc.encoded)
		}
	}
}

func TestGS1URLResourceBounds(t *testing.T) {
	for _, value := range []string{
		"https://example.com/" + strings.Repeat("a", GS1MaxInputCharacters),
		"https://example.com/" + strings.Repeat("/", GS1MaxElements),
		"https://example.com:" + strings.Repeat("9", 1000),
		"https://" + strings.Repeat("é", 1025) + ".example/",
		"https://xn--" + strings.Repeat("a", 4097) + ".example/",
		"https://xn--" + strings.Repeat("9", 1000) + ".example/",
		"https://example.com/\xff",
	} {
		if _, err := gs1ParseURL(value, false); err == nil {
			t.Errorf("accepted hostile input of %d bytes", len(value))
		}
	}
	label := strings.Repeat("é", 1024) + ".example"
	host, err := gs1Host(label)
	if err != nil {
		t.Fatalf("bounded label rejected: %v", err)
	}
	if again, err := gs1Host(host); err != nil || again != host {
		t.Fatalf("bounded ACE roundtrip: %q, %v", again, err)
	}
	if _, err := (gs1URL{scheme: "https", authority: "example.com", path: strings.Repeat("a", GS1MaxInputCharacters)}).serialize(); err == nil {
		t.Fatal("accepted oversized serialized output")
	}
}

// Every Unicode scalar is checked against the frozen profile. Anything accepted
// must produce an ASCII host that the same adapter accepts without changing it.
// This guards against valid-looking but noncanonical or unsupported ACE output.
func TestGS1HostScalarStability(t *testing.T) {
	if testing.Short() {
		t.Skip("exhaustive Unicode scalar sweep")
	}
	accepted := 0
	for cp := rune(128); cp <= utf8.MaxRune; cp++ {
		if cp >= 0xd800 && cp <= 0xdfff {
			continue
		}
		host, err := gs1Host(string(cp) + ".example")
		if err != nil {
			continue
		}
		accepted++
		if !gs1ASCII(host) {
			t.Fatalf("U+%04X produced non-ASCII host %q", cp, host)
		}
		again, err := gs1Host(host)
		if err != nil || again != host {
			t.Fatalf("U+%04X: %q -> %q, %v", cp, host, again, err)
		}
	}
	if accepted < 100000 {
		t.Fatalf("sweep unexpectedly accepted only %d Unicode scalars", accepted)
	}
	t.Logf("%d Unicode scalars have stable ASCII host serialization", accepted)
}

func FuzzGS1URLAdapter(f *testing.F) {
	for _, value := range []string{"https://example.com/01/04912345678904/10/..", "https://例え.テスト/", "https://[::ffff:192.0.2.1]/", "https://u:p@127.1:443/a/..", "https://xn--9ca.test/?x=%E2%82"} {
		f.Add(value)
	}
	f.Fuzz(func(t *testing.T, input string) {
		u, err := gs1ParseURL(input, false)
		if err != nil || gs1CheckURI(u, false) != nil {
			return
		}
		out, err := u.serialize()
		if err != nil {
			return
		}
		v, err := gs1ParseURL(out, false)
		if err != nil {
			t.Fatalf("accepted URL failed roundtrip: %q: %v", out, err)
		}
		again, err := v.serialize()
		if err != nil || again != out {
			t.Fatalf("unstable URL %q -> %q: %v", out, again, err)
		}
	})
}

// TestGS1URLDifferential is an optional development lane. Its independently
// generated Node reference input is never needed by ordinary Go tests.
func TestGS1URLDifferential(t *testing.T) {
	path := os.Getenv("SPECQR_URL_REFERENCE")
	if path == "" {
		t.Skip("set SPECQR_URL_REFERENCE using tools/url-differential/run.py")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var reference struct {
		RuntimeVersions map[string]string `json:"runtimeVersions"`
		NodeVersion     string            `json:"nodeVersion"`
		Seed            uint32            `json:"seed"`
		CaseCount       int               `json:"caseCount"`
		UniqueInputs    int               `json:"uniqueInputs"`
		Cases           []struct {
			Input    string  `json:"input"`
			Expected *string `json:"expected"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &reference); err != nil {
		t.Fatal(err)
	}
	if reference.NodeVersion != "v24.19.0" && reference.NodeVersion != "v24.21.0" {
		t.Fatalf("unrecognized Node reference version %q", reference.NodeVersion)
	}
	expectedADA := "3.4.4"
	if reference.NodeVersion == "v24.21.0" {
		expectedADA = "4.0.0"
	}
	if reference.RuntimeVersions["node"] != strings.TrimPrefix(reference.NodeVersion, "v") || reference.RuntimeVersions["ada"] != expectedADA || reference.RuntimeVersions["icu"] != "78.3" || reference.RuntimeVersions["unicode"] != "17.0" {
		t.Fatal("reference Node/ADA/ICU/Unicode component mismatch")
	}
	if reference.Seed != 43807 || reference.CaseCount != 10000 || len(reference.Cases) != reference.CaseCount {
		t.Fatal("reference corpus metadata mismatch")
	}
	for i, tc := range reference.Cases {
		u, err := gs1ParseURL(tc.Input, false)
		got := ""
		if err == nil {
			got, err = u.serialize()
		}
		if tc.Expected == nil {
			if err == nil {
				t.Fatalf("case %d accepted %q as %q; Node rejected it", i, tc.Input, got)
			}
		} else if err != nil || got != *tc.Expected {
			t.Fatalf("case %d for %q: got %q (%v), want %q", i, tc.Input, got, err, *tc.Expected)
		}
	}
	t.Logf("%s: %d cases (%d distinct inputs), zero mismatches", reference.NodeVersion, reference.CaseCount, reference.UniqueInputs)
}
