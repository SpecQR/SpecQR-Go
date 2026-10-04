package specqr

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

func TestGS1CatalogDetached(t *testing.T) {
	catalog := GS1GetSupportedAIs()
	if len(catalog) != 50 {
		t.Fatalf("catalog entries=%d", len(catalog))
	}
	seen := map[string]bool{}
	for _, v := range catalog {
		if seen[v.AI] {
			t.Fatalf("duplicate %s", v.AI)
		}
		seen[v.AI] = true
		if !reflect.DeepEqual(GS1GetAIInfo(v.AI), &v) {
			t.Fatalf("lookup %s differs", v.AI)
		}
	}
	catalog[0].AI = "XX"
	qualifier := GS1GetAIInfo("10")
	qualifier.DigitalLinkPathForPrimary[0] = "00"
	qualifier.AI = "XX"
	if GS1GetAIInfo("00").AI != "00" || GS1GetAIInfo("10").DigitalLinkPathForPrimary[0] != "01" {
		t.Fatal("catalog was mutated")
	}
	if GS1GetAIInfo("3106") != nil {
		t.Fatal("unknown family member accepted")
	}
}
func TestGS1CheckDigits(t *testing.T) {
	for _, body := range []string{"9638507", "01234567890", "400638133393", "0950600013435"} {
		full, e := GS1AppendGTINCheckDigit(body)
		if e != nil {
			t.Fatal(e)
		}
		ok, e := GS1ValidateGTINCheckDigit(full)
		if e != nil || !ok {
			t.Fatalf("%q %v %v", full, ok, e)
		}
		last := full[len(full)-1]
		wrong := full[:len(full)-1] + string(byte('0'+(last-'0'+1)%10))
		if ok, _ := GS1ValidateGTINCheckDigit(wrong); ok {
			t.Fatal("accepted changed check")
		}
	}
	sscc, e := GS1AppendSSCCCheckDigit("12345678901234567")
	if e != nil {
		t.Fatal(e)
	}
	if ok, e := GS1ValidateSSCCCheckDigit(sscc); e != nil || !ok {
		t.Fatalf("%q: %v %v", sscc, ok, e)
	}
	for _, v := range []string{"", "a", "１２", "١٢", "\xff"} {
		if _, e := GS1CalculateCheckDigit(v); e == nil {
			t.Fatalf("accepted %q", v)
		}
	}
	if v, e := GS1CalculateCheckDigit(strings.Repeat("9", GS1MaxInputCharacters)); e != nil || v != "0" {
		t.Fatalf("maximum numeric input %q %v", v, e)
	}
}
func TestGS1ElementRoundTrip(t *testing.T) {
	elements := []GS1Element{{"01", "09506000134352"}, {"10", "LOT%123"}, {"17", "260101"}, {"21", "ABC"}}
	raw, e := GS1ToElementString(elements)
	if e != nil {
		t.Fatal(e)
	}
	if raw != "010950600013435210LOT%123\x1d1726010121ABC" {
		t.Fatalf("raw=%q", raw)
	}
	parsed, e := GS1ParseElementString(raw)
	if e != nil || !reflect.DeepEqual(parsed.Elements, elements) || !parsed.HasSeparators {
		t.Fatalf("parsed=%+v %v", parsed, e)
	}
	human, e := GS1ToHumanReadable(elements)
	if e != nil {
		t.Fatal(e)
	}
	back, e := GS1FromHumanReadable(human)
	if e != nil || !reflect.DeepEqual(back, elements) {
		t.Fatalf("%q => %+v %v", human, back, e)
	}
	if converted, e := GS1ElementStringToHumanReadable(raw); e != nil || converted != human {
		t.Fatalf("human conversion=%q %v", converted, e)
	}
}
func TestGS1ElementDiagnostics(t *testing.T) {
	cases := []struct {
		input, code, ai string
		offset          int
	}{{"\x1d10ABC", "GS1_UNEXPECTED_SEPARATOR", "", 0}, {"88ABC", "GS1_UNSUPPORTED_AI", "88", 0}, {"10ABC17260101", "GS1_MISSING_SEPARATOR", "10", 2}, {"10ABC\x1d", "GS1_UNEXPECTED_SEPARATOR", "", -1}, {"0109506000134353", "GS1_INVALID_CHECK_DIGIT", "01", -1}, {"1732", "GS1_INVALID_LENGTH", "17", -1}, {"10é", "GS1_INVALID_CHARSET", "10", -1}}
	for _, c := range cases {
		r := GS1ValidateElementString(c.input)
		if r.OK || len(r.Errors) != 1 {
			t.Fatalf("%q => %+v", c.input, r)
		}
		v := r.Errors[0]
		if v.Code != c.code {
			t.Fatalf("%q code %s want %s", c.input, v.Code, c.code)
		}
		if c.ai != "" && (v.AI == nil || *v.AI != c.ai) {
			t.Fatalf("%q AI %+v", c.input, v.AI)
		}
		if c.offset >= 0 && (v.Offset == nil || *v.Offset != c.offset) {
			t.Fatalf("%q offset %+v", c.input, v.Offset)
		}
	}
	r := GS1ValidateElements([]GS1Element{{"17", "x"}, {"10", ""}})
	if len(r.Errors) != 2 || r.Errors[0].ElementIndex == nil || *r.Errors[0].ElementIndex != 0 || r.Errors[1].Value == nil || *r.Errors[1].Value != "" {
		t.Fatalf("issues=%+v", r)
	}
	no := false
	r = GS1ValidateElements([]GS1Element{{"17", "x"}, {"10", ""}}, GS1ValidationOptions{CollectAllErrors: &no})
	if len(r.Errors) != 1 {
		t.Fatal("collectAllErrors=false")
	}
	r = GS1ValidateElements([]GS1Element{{"10", "ABC"}}, GS1ValidationOptions{Context: "digital-link"})
	if r.OK || r.Errors[0].Code != "GS1_INVALID_DIGITAL_LINK_PLACEMENT" {
		t.Fatal("missing primary")
	}
	r = GS1ValidateElements([]GS1Element{{"10", "ABC"}}, GS1ValidationOptions{AllowUnsupportedAI: true})
	if r.OK || r.Errors[0].Expected != false {
		t.Fatal("unsupported option")
	}
	r = GS1ValidateElementString("10ABC")
	data, e := json.Marshal(r)
	if e != nil || string(data) != "{\"ok\":true,\"elements\":[{\"ai\":\"10\",\"value\":\"ABC\"}],\"hasSeparators\":false,\"warnings\":[]}" {
		t.Fatalf("validation JSON=%s %v", data, e)
	}
}
func TestGS1DigitalLinkRoundTrip(t *testing.T) {
	elements := []GS1Element{{"17", "260101"}, {"10", "LOT A%"}, {"01", "09506000134352"}, {"21", "SER/IAL"}, {"91", "A&B"}}
	uri, e := GS1CreateDigitalLink(elements, GS1DigitalLinkOptions{BaseURL: "HTTPS://ExAmPlE.com:443/stem/"})
	if e != nil {
		t.Fatal(e)
	}
	want := "https://example.com/stem/01/09506000134352/10/LOT%20A%25/21/SER%2FIAL?17=260101&91=A%26B"
	if uri != want {
		t.Fatalf("URI=%q want %q", uri, want)
	}
	parsed, e := GS1ParseDigitalLink(uri + "&other=%E2%82&other=a+b")
	if e != nil {
		t.Fatal(e)
	}
	if len(parsed.Elements) != 5 || len(parsed.PathElements) != 3 || len(parsed.QueryElements) != 2 || len(parsed.UnknownQuery) != 2 || parsed.UnknownQuery[0].Value != "�" {
		t.Fatalf("parsed=%+v", parsed)
	}
	normalized, e := GS1NormalizeDigitalLink(uri + "&other=%E2%82&other=a+b")
	if e != nil {
		t.Fatal(e)
	}
	if normalized != uri+"&other=%EF%BF%BD&other=a+b" {
		t.Fatalf("normalized %q", normalized)
	}
	queryOnly, e := GS1CreateDigitalLink(elements, GS1DigitalLinkOptions{BaseURL: "https://example.com", PathAIs: []string{}})
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(queryOnly, "?10=LOT+A%25&17=260101&21=SER%2FIAL&91=A%26B") {
		t.Fatalf("query placement %q", queryOnly)
	}
}
func TestGS1DigitalLinkDotPayload(t *testing.T) {
	for _, v := range []string{".", "..", "%2e", ".%2e", "..."} {
		elements := []GS1Element{{"01", "09506000134352"}, {"10", v}}
		uri, e := GS1CreateDigitalLink(elements, GS1DigitalLinkOptions{BaseURL: "https://example.com/base/../"})
		if e != nil {
			t.Fatal(e)
		}
		parsed, e := GS1ParseDigitalLink(uri)
		if e != nil || !reflect.DeepEqual(parsed.Elements, elements) {
			t.Fatalf("%q -> %q -> %+v %v", v, uri, parsed, e)
		}
		if (v == "." || v == "..") && !strings.Contains(uri, "?10=") {
			t.Fatal("dot-only value left in path")
		}
		direct := "https://example.com/01/09506000134352/10/" + gs1Encode(v, 0)
		p, e := GS1ParseDigitalLink(direct)
		if e != nil || p.Elements[1].Value != v {
			t.Fatalf("direct dot %q: %+v %v", direct, p, e)
		}
		a, e := GS1NormalizeDigitalLink(direct)
		if e != nil {
			t.Fatal(e)
		}
		b, e := GS1NormalizeDigitalLink(a)
		if e != nil || a != b {
			t.Fatalf("unstable %q %q %v", a, b, e)
		}
	}
}
func TestGS1DigitalLinkDiagnostics(t *testing.T) {
	cases := []struct{ uri, code, reason string }{{"not URL", "GS1_DIGITAL_LINK_INVALID_URI", "invalid-uri"}, {"ftp://example.com/01/09506000134352", "GS1_DIGITAL_LINK_INVALID_URI", "invalid-uri"}, {"https://example.com/01/09506000134352#x", "GS1_DIGITAL_LINK_FRAGMENT_NOT_ALLOWED", "fragment-not-allowed"}, {"https://example.com/01/09506000134352?x=%q", "GS1_INVALID_PERCENT_ENCODING", "invalid-percent-encoding"}, {"https://example.com/01/09506000134352?01=09506000134352", "GS1_DUPLICATE_AI", "duplicate-ai"}, {"https://example.com/01/09506000134352/17/260101", "GS1_INVALID_DIGITAL_LINK_PLACEMENT", "invalid-digital-link-placement"}, {"https://example.com/01/09506000134352/10/%ED%A0%80", "GS1_INVALID_PERCENT_ENCODING", "invalid-percent-encoding"}, {"https://example.com/01/09506000134352/10", "GS1_INVALID_INPUT", "malformed-path"}}
	for _, c := range cases {
		r := GS1ValidateDigitalLink(c.uri)
		if r.OK || len(r.Errors) != 1 || r.Errors[0].Code != c.code || r.Errors[0].Reason != c.reason {
			t.Fatalf("%q => %+v", c.uri, r)
		}
	}
	v := GS1ValidateDigitalLink("http://example.com/01/09506000134352?x=1")
	if !v.OK || len(v.Warnings) != 2 || v.Warnings[1].Count == nil || *v.Warnings[1].Count != 1 {
		t.Fatalf("warnings=%+v", v)
	}
	key := "<a>&\"\u2028\x00"
	v = GS1ValidateDigitalLink("https://example.com/01/09506000134352?"+gs1Encode(key, 1)+"=x", GS1DigitalLinkOptions{UnknownQuery: "reject"})
	if v.OK || v.Errors[0].Key == nil || *v.Errors[0].Key != key || !strings.Contains(v.Errors[0].Message, "<a>&") {
		t.Fatalf("unknown key issue=%+v", v)
	}
}
func TestGS1ResourceLimits(t *testing.T) {
	large := strings.Repeat("A", GS1MaxInputCharacters+1)
	for _, call := range []func() error{func() error { _, e := GS1FromHumanReadable(large); return e }, func() error { _, e := GS1ParseElementString(large); return e }, func() error { _, e := GS1ParseDigitalLink(large); return e }, func() error { _, e := GS1ToElementString(make([]GS1Element, GS1MaxElements+1)); return e }, func() error {
		_, e := GS1CreateDigitalLink([]GS1Element{{"01", "09506000134352"}}, GS1DigitalLinkOptions{BaseURL: "https://example.com", PathAIs: make([]string, GS1MaxElements+1)})
		return e
	}} {
		e := call()
		var typed *Error
		if e == nil || !errors.As(e, &typed) || typed.Code != InvalidGS1 {
			t.Fatalf("limit=%v", e)
		}
	}
	// A million-character final variable field must not perform quadratic scans.
	if _, e := GS1ParseElementString("10" + strings.Repeat("A", GS1MaxInputCharacters-2)); e == nil {
		t.Fatal("large variable accepted")
	}
	aggregate := make([]GS1Element, 12000)
	for i := range aggregate {
		aggregate[i] = GS1Element{"91", strings.Repeat("A", 90)}
	}
	if _, e := GS1ToElementString(aggregate); e == nil {
		t.Fatal("aggregate work limit")
	}
	if _, e := GS1ParseElementString("10\xff"); e == nil {
		t.Fatal("invalid UTF8 accepted")
	}
}
func TestGS1DeterministicMutationRoundTrips(t *testing.T) {
	rng := rand.New(rand.NewSource(732849))
	for i := 0; i < 2000; i++ {
		value := make([]byte, 1+rng.Intn(20))
		for j := range value {
			value[j] = 'A' + byte(rng.Intn(26))
		}
		v := string(value)
		e := []GS1Element{{"01", "09506000134352"}, {"10", v}, {"17", fmt.Sprintf("%06d", rng.Intn(1000000))}}
		raw, err := GS1ToElementString(e)
		if err != nil {
			t.Fatal(err)
		}
		p, err := GS1ParseElementString(raw)
		if err != nil || !reflect.DeepEqual(p.Elements, e) {
			t.Fatalf("raw mutation %+v %v", p, err)
		}
		uri, err := GS1CreateDigitalLink(e, GS1DigitalLinkOptions{BaseURL: "https://example.com"})
		if err != nil {
			t.Fatal(err)
		}
		puri, err := GS1ParseDigitalLink(uri)
		if err != nil || !reflect.DeepEqual(puri.Elements, e) {
			t.Fatalf("URI mutation %+v %v", puri, err)
		}
	}
}
func FuzzGS1Helpers(f *testing.F) {
	for _, s := range []string{"010950600013435210ABC\x1d17260101", "10%", "(10)ABC", "https://example.com/01/09506000134352/10/.."} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if p, e := GS1ParseElementString(input); e == nil {
			v, e := GS1ToElementString(p.Elements)
			if e != nil || v != input {
				t.Fatalf("raw unstable %q => %q %v", input, v, e)
			}
		}
		if v, e := GS1NormalizeDigitalLink(input); e == nil {
			w, e := GS1NormalizeDigitalLink(v)
			if e != nil || v != w {
				t.Fatalf("URI unstable %q => %q %v", v, w, e)
			}
		}
	})
}

func TestGS1SourceDiagnosticMessages(t *testing.T) {
	cases := []struct{ got, want string }{
		{GS1ValidateElements([]GS1Element{{"x", "A"}}).Errors[0].Message, `GS1 element 0 has invalid AI "x"; expected 2 to 4 digits`},
		{GS1ValidateElements([]GS1Element{{"17", "123"}}).Errors[0].Message, "GS1 AI 17 value must be exactly 6 characters"},
		{GS1ValidateElementString("(10)ABC").Errors[0].Message, "GS1 element string input must be raw data without human-readable parentheses; use parseGs1HumanReadable() and createGs1ElementString() before generate(..., { gs1: true })"},
		{GS1ValidateElements(nil, GS1ValidationOptions{Context: "x"}).Errors[0].Message, `GS1 validation options.context must be "element-string" or "digital-link"`},
		{GS1ValidateDigitalLink("https://example.com", GS1DigitalLinkOptions{Normalize: true}).Errors[0].Message, "GS1 Digital Link validation options.normalize is not implemented yet"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Fatalf("message %q want %q", c.got, c.want)
		}
	}
	_, e := GS1CreateDigitalLink([]GS1Element{{"01", "09506000134352"}}, GS1DigitalLinkOptions{})
	var typed *Error
	if !errors.As(e, &typed) || typed.Message != "GS1 Digital Link options.baseUrl is required" {
		t.Fatalf("base error %v", e)
	}
}
