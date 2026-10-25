package prosemirror

import (
	"errors"
	"testing"
)

// A lone surrogate is kept by ProseMirror but cannot be held in Go's
// UTF-8 strings: Go refuses the document.
func TestLoneSurrogateRefused(t *testing.T) {
	for _, s := range []string{`\ud83d`, `\ude00`, `a\ud83dx`, `\ud83d\u0041`} {
		_, err := parseDoc([]byte(docOf(`{"type":"paragraph","content":[{"type":"text","text":"` + s + `"}]}`)))
		if !errors.Is(err, ErrLoneSurrogate) {
			t.Errorf("%s: %v", s, err)
		}
	}
	d, err := parseDoc([]byte(docOf(`{"type":"paragraph","content":[{"type":"text","text":"\ud83d\ude00"}]}`)))
	if err != nil || d.Child(0).Child(0).Text() != "😀" || d.Child(0).Child(0).NodeSize() != 2 {
		t.Fatalf("pair: %v", err)
	}
	// A step that cuts the pair in half is refused.
	for _, st := range []string{
		`{"stepType":"replace","from":2,"to":2,"slice":{"content":[{"type":"text","text":"x"}]}}`,
		`{"stepType":"replace","from":1,"to":2}`,
		`{"stepType":"addMark","mark":{"type":"em"},"from":2,"to":3}`,
	} {
		s, err := testSchema.ParseStep([]byte(st))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Apply(d); !errors.Is(err, ErrSurrogateSplit) {
			t.Errorf("%s: %v", st, err)
		}
	}
}

func TestRefusedSteps(t *testing.T) {
	for _, st := range []string{
		`{"stepType":"docAttr","attr":"x","value":1}`,
		`{"stepType":"addNodeMark","pos":0,"mark":{"type":"em"}}`,
		`{"stepType":"replace","from":1.5,"to":2}`,
		`{"stepType":"replace","from":-1,"to":2}`,
		`{"stepType":"replace","from":3,"to":2}`,
		`{"stepType":"replace","from":1,"to":1,"slice":{"content":[{"type":"text","text":"x"}],"openStart":1}}`,
		`{"stepType":"replaceAround","from":1,"to":2,"gapFrom":3,"gapTo":2,"insert":0}`,
		`{"stepType":"bogus"}`,
		`{}`,
	} {
		if _, err := testSchema.ParseStep([]byte(st)); err == nil {
			t.Errorf("%s: accepted", st)
		}
	}
}

func TestJSONNumbersAndStrings(t *testing.T) {
	for _, c := range []struct {
		in  float64
		out string
	}{{0, "0"}, {1, "1"}, {-3, "-3"}, {2.5, "2.5"}, {0.1, "0.1"}, {1e21, "1e+21"}, {1e-7, "1e-7"}, {123456789012345680000, "123456789012345680000"}, {1 << 60, "1152921504606847000"}, {0.000001, "0.000001"}, {-1.5e-10, "-1.5e-10"}} {
		if got := string(appendNumber(nil, c.in)); got != c.out {
			t.Errorf("%v: %s, want %s", c.in, got, c.out)
		}
	}
	v, err := ParseValue([]byte(`{"b":1,"2":2,"a":3,"1":4,"b":5}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(appendValue(nil, v)); got != `{"1":4,"2":2,"b":5,"a":3}` {
		t.Errorf("object order: %s", got)
	}
	if got := string(appendString(nil, "a\"\\\n\x01<>&\u2028é")); got != `"a\"\\\n\u0001<>&`+"\u2028é\"" {
		t.Errorf("string: %s", got)
	}
}
