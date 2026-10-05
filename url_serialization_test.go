package specqr

import "testing"

func TestURLSerializationEmptyFragment(t *testing.T) {
	root := "https://example.com/01/04912345678904"
	elements := []GS1Element{{AI: "01", Value: "04912345678904"}, {AI: "10", Value: "ABC123"}, {AI: "17", Value: "251231"}}
	paths := [][]string{nil, {}, {"21"}, {"01", "10"}}
	expected := []string{root + "/10/ABC123?17=251231#", root + "?10=ABC123&17=251231#", root + "?10=ABC123&17=251231#", root + "/10/ABC123?17=251231#"}
	for i, path := range paths {
		actual, err := GS1CreateDigitalLink(elements, GS1DigitalLinkOptions{BaseURL: "https://example.com#", PathAIs: path})
		if err != nil || actual != expected[i] {
			t.Fatalf("case %d: %q / %v", i, actual, err)
		}
		normalized, err := GS1NormalizeDigitalLink(actual)
		if err != nil || normalized != root+"/10/ABC123?17=251231" {
			t.Fatalf("normalize %d: %q / %v", i, normalized, err)
		}
	}
	actual, err := GS1NormalizeDigitalLink(root + "?x=%00")
	if err != nil || actual != root+"?x=%00" {
		t.Fatalf("query NUL: %q / %v", actual, err)
	}
}
