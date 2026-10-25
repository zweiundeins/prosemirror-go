package prosemirror

import (
	"strings"
	"testing"
)

// testRules are rules an app could have: a heading's level, no control
// characters in text but tabs and newlines in code, safe links, images
// from a library.
var testRules = Rules{
	Noun: "page", MaxBytes: 200 << 10, MaxBlocks: 2000, MaxAttrLength: 2048,
	Nodes: map[string]func(n, parent *Node) error{
		"heading": func(n, _ *Node) error {
			if l, ok := n.AttrInt("level"); !ok || l < 1 || l > 3 {
				return Refuse("A heading has level 1, 2 or 3.")
			}
			return nil
		},
		"text": func(n, parent *Node) error {
			code := parent.Type().code
			for _, c := range n.Text() {
				if c < 0x20 && !(code && (c == '\t' || c == '\n')) {
					return Refuse("The text holds a control character.")
				}
			}
			return nil
		},
		"image": func(n, _ *Node) error {
			if !strings.HasPrefix(n.AttrString("imageId"), "img-") {
				return Refuse("Images come from the image library.")
			}
			return nil
		},
	},
	Marks: map[string]func(m *Mark, n *Node) error{
		"link": func(m *Mark, n *Node) error {
			if _, ok := SafeHref(m.AttrString("href")); !ok {
				return Refuse("A link must start with http://, https:// or mailto:.")
			}
			return nil
		},
	},
}

// The fixture documents pass, their blocks at least where the 200 KB ones
// sit past the size bound.
func TestRulesAcceptFixtures(t *testing.T) {
	for _, name := range docFixtures {
		d := mustDoc(t, string(readFixture(t, name+".json")))
		r := testRules
		if strings.HasPrefix(name, "f200k") {
			r.MaxBytes = 0
		}
		if err := r.Check(nil, d); err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if err := (Rules{}).Check(nil, d); err != nil {
			t.Errorf("%s, zero rules: %v", name, err)
		}
	}
}

func TestRulesRefuse(t *testing.T) {
	cases := []struct{ doc, msg string }{
		{docOf(`{"type":"heading","attrs":{"level":4}}`), "A heading has level 1, 2 or 3."},
		{docOf(`{"type":"heading","attrs":{"level":1.5}}`), "A heading has level 1, 2 or 3."},
		{docOf(`{"type":"image","attrs":{"imageId":"evil"}}`), "Images come from the image library."},
		{inPara(text("x", link("javascript:alert(1)"))), "A link must start with http://, https:// or mailto:."},
		{inPara(text("x", link("https://"+strings.Repeat("a", 2050)))), "An attribute holds at most 2,048 characters."},
		{inPara(text("a\u0007b")), "The text holds a control character."},
		{docOf(`{"type":"code_block","content":[` + text("a\u0007b") + `]}`), "The text holds a control character."},
		{docOf(strings.Repeat(`{"type":"horizontal_rule"},`, 2000) + `{"type":"horizontal_rule"}`), "The page is at its limit of 2,000 blocks."},
		{docOf(strings.Repeat(`{"type":"code_block","content":[{"type":"text","text":"`+strings.Repeat("x", 1000)+`"}]},`, 205) + para), "The page is at its limit of 200 KB."},
	}
	for i, c := range cases {
		err := testRules.Check(nil, mustDoc(t, c.doc))
		if err == nil || err.Error() != c.msg {
			t.Errorf("case %d: %v, want %q", i, err, c.msg)
		}
	}
	ok := []string{
		docOf(`{"type":"code_block","attrs":{"language":"go"},"content":[` + text("a\tb\nc") + `]}`),
		inPara(text("x", link("mailto:ada@example.com"))),
		docOf(`{"type":"image","attrs":{"imageId":"img-1"}}`),
	}
	for i, c := range ok {
		if err := testRules.Check(nil, mustDoc(t, c)); err != nil {
			t.Errorf("ok %d: %v", i, err)
		}
	}
	if err := (Rules{}).Check(nil, mustDoc(t, docOf(para)).Child(0)); err == nil || err.Error() != "This is not a document." {
		t.Errorf("not a document: %v", err)
	}
}

// Only the blocks a commit changed are validated: an old block that breaks
// a rule (written before the rule) does not refuse an edit elsewhere.
func TestRulesCheckChangedBlocksOnly(t *testing.T) {
	before := mustDoc(t, docOf(`{"type":"heading","attrs":{"level":3}}`, para))
	// Swap in a level the rules refuse without a step, as an old document would hold.
	h, _ := testSchema.Node("heading").Create(map[string]any{"level": 9}, nil)
	before = before.copy(before.content.replaceChild(0, h))
	st, _ := testSchema.ParseStep([]byte(`{"stepType":"replace","from":3,"to":3,"slice":{"content":[{"type":"text","text":"!"}]}}`))
	after, err := st.Apply(before)
	if err != nil {
		t.Fatal(err)
	}
	if err := testRules.Check(before, after); err != nil {
		t.Fatalf("an edit in the paragraph: %v", err)
	}
	st, _ = testSchema.ParseStep([]byte(`{"stepType":"replace","from":1,"to":1,"slice":{"content":[{"type":"text","text":"!"}]}}`))
	after, _ = st.Apply(before)
	if err := testRules.Check(before, after); err == nil {
		t.Fatal("an edit in the heading passed")
	}
	// A slice that puts a heading inside a paragraph applies, and Check refuses it.
	st, _ = testSchema.ParseStep([]byte(`{"stepType":"replace","from":0,"to":0,"slice":{"content":[{"type":"paragraph","content":[{"type":"heading","attrs":{"level":1}}]}]}}`))
	after, err = st.Apply(mustDoc(t, docOf(para)))
	if err != nil {
		t.Fatal(err)
	}
	if err := testRules.Check(mustDoc(t, docOf(para)), after); err == nil || err.Error() != "The page holds something the editor cannot make." {
		t.Fatalf("heading in a paragraph: %v", err)
	}
}

func TestSafeHref(t *testing.T) {
	for href, want := range map[string]string{
		"https://example.com/a?b": "example.com",
		"HTTP://Example.COM:8080": "example.com:8080",
		"mailto:ada@example.com":  "ada@example.com",
	} {
		if got, ok := SafeHref(href); !ok || got != want {
			t.Errorf("%s: %q %v", href, got, ok)
		}
	}
	for _, href := range []string{"javascript:alert(1)", "JaVaScRiPt:alert(1)", "data:text/html,x", "https:javascript:alert(1)", "//evil.example", " https://x.y", "https://x.y/\n", "vbscript:x", "/relative", "", "http://"} {
		if _, ok := SafeHref(href); ok {
			t.Errorf("%q accepted", href)
		}
	}
}

// Documents are shared between goroutines: concurrent reads, caches
// included, are safe (run with -race).
func TestConcurrentReads(t *testing.T) {
	d := mustDoc(t, string(readFixture(t, "f20k-doc.json")))
	st, _ := testSchema.ParseStep([]byte(`{"stepType":"replace","from":3,"to":3,"slice":{"content":[{"type":"text","text":"x"}]}}`))
	c := NewBlockCache(testHTML)
	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			after, err := st.Apply(d)
			if err != nil {
				t.Error(err)
				return
			}
			_ = testRules.Check(d, after)
			_ = testHTML.Render(after, testResolver)
			_ = c.Render(after, testResolver)
			_ = testMarkdown.Render(after, testResolver)
			_ = testLeaves.Text(d, "\n")
			_ = d.JSONLen() + d.Blocks()
		}()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
}
