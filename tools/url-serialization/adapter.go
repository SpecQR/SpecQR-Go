package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	q "github.com/SpecQR/SpecQR-Go"
	"os"
)

type Options struct {
	Context            string
	CollectAllErrors   *bool
	AllowUnsupportedAi bool
	BaseUrl            string
	PrimaryAi          string
	PathAis            []string
	UnknownQuery       string
	Normalize          bool
	Mode               string
}
type Request struct {
	Op       string
	Input    string
	Elements []q.GS1Element
	Options  Options
}

func call(r Request) (any, error) {
	o := r.Options
	v := q.GS1ValidationOptions{Context: o.Context, CollectAllErrors: o.CollectAllErrors, AllowUnsupportedAI: o.AllowUnsupportedAi}
	l := q.GS1DigitalLinkOptions{BaseURL: o.BaseUrl, PrimaryAI: o.PrimaryAi, PathAIs: o.PathAis, UnknownQuery: o.UnknownQuery, Normalize: o.Normalize, Mode: o.Mode}
	s := r.Input
	switch r.Op {
	case "dictionary":
		return q.GS1GetSupportedAIs(), nil
	case "info":
		return q.GS1GetAIInfo(s), nil
	case "checkDigit":
		return q.GS1CalculateCheckDigit(s)
	case "validateCheckDigit":
		return q.GS1ValidateCheckDigit(s)
	case "gtinDigit":
		return q.GS1CalculateGTINCheckDigit(s)
	case "gtinAppend":
		return q.GS1AppendGTINCheckDigit(s)
	case "gtinValidate":
		return q.GS1ValidateGTINCheckDigit(s)
	case "ssccDigit":
		return q.GS1CalculateSSCCCheckDigit(s)
	case "ssccAppend":
		return q.GS1AppendSSCCCheckDigit(s)
	case "ssccValidate":
		return q.GS1ValidateSSCCCheckDigit(s)
	case "human":
		return q.GS1FromHumanReadable(s)
	case "raw":
		return q.GS1ParseElementString(s)
	case "create":
		return q.GS1ToElementString(r.Elements)
	case "validateElements":
		return q.GS1ValidateElements(r.Elements, v), nil
	case "validateRaw":
		return q.GS1ValidateElementString(s, v), nil
	case "linkCreate":
		return q.GS1CreateDigitalLink(r.Elements, l)
	case "linkParse":
		return q.GS1ParseDigitalLink(s, l)
	case "linkValidate":
		return q.GS1ValidateDigitalLink(s, l), nil
	case "linkNormalize":
		return q.GS1NormalizeDigitalLink(s, l)
	default:
		panic("unknown op")
	}
}
func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 65536), 4000000)
	out := json.NewEncoder(os.Stdout)
	for in.Scan() {
		var r Request
		if err := json.Unmarshal(in.Bytes(), &r); err != nil {
			panic(err)
		}
		v, err := call(r)
		if err != nil {
			e, ok := err.(*q.Error)
			if !ok {
				panic(err)
			}
			v = map[string]any{"throws": map[string]any{"code": e.Code, "message": e.Message}}
		}
		if err = out.Encode(v); err != nil {
			panic(err)
		}
	}
	if err := in.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
