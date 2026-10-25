package prosemirror

import (
	"regexp"
	"testing"
)

var testIDs = IDs{"image": "imageId", "embed": "src"}

func TestIDs(t *testing.T) {
	d := mustDoc(t, docOf(para, `{"type":"blockquote","content":[{"type":"embed","attrs":{"src":"e1"}}]}`, `{"type":"image","attrs":{"imageId":"i1"}}`))
	if n, pos, ok := testIDs.Find(d, "e1"); !ok || pos != 5 || n.Type().Name() != "embed" {
		t.Errorf("find e1: %v %d %v", n, pos, ok)
	}
	if _, _, ok := testIDs.Find(d, "nope"); ok {
		t.Error("found an id that is not there")
	}
	if n, pos, ok := FindNode(d, "imageId", "i1"); !ok || pos != 7 || n.Type().Name() != "image" {
		t.Errorf("FindNode: %v %d %v", n, pos, ok)
	}
	if id, ok := testIDs.Of(d.Child(0)); ok || id != "" {
		t.Error("a paragraph has an id")
	}
}

// A commit may not add a node without its id or with one another node
// has; an old block it does not touch is not checked.
func TestRulesIDs(t *testing.T) {
	r := Rules{IDs: testIDs}
	base := mustDoc(t, docOf(para, `{"type":"embed","attrs":{"src":"e1"}}`))
	add := func(block string) error {
		st, err := testSchema.ParseStep([]byte(`{"stepType":"replace","from":0,"to":0,"slice":{"content":[` + block + `]}}`))
		if err != nil {
			t.Fatal(err)
		}
		next, err := st.Apply(base)
		if err != nil {
			t.Fatal(err)
		}
		return r.Check(base, next)
	}
	if err := add(`{"type":"embed","attrs":{"src":"e2"}}`); err != nil {
		t.Errorf("a new id: %v", err)
	}
	if err := add(`{"type":"embed"}`); err == nil || err.Error() != "This embed has no id." {
		t.Errorf("no id: %v", err)
	}
	if err := add(`{"type":"embed","attrs":{"src":"e1"}}`); err == nil || err.Error() != "Two nodes have the id e1." {
		t.Errorf("a copied id: %v", err)
	}
	if err := add(`{"type":"blockquote","content":[{"type":"embed","attrs":{"src":"e3"}},{"type":"embed","attrs":{"src":"e3"}}]}`); err == nil {
		t.Error("two equal ids in one block passed")
	}
	old := mustDoc(t, docOf(para, `{"type":"embed"}`))
	st, _ := testSchema.ParseStep([]byte(`{"stepType":"replace","from":1,"to":1,"slice":{"content":[{"type":"text","text":"x"}]}}`))
	next, _ := st.Apply(old)
	if err := r.Check(old, next); err != nil {
		t.Errorf("an edit beside an old block without an id: %v", err)
	}
}

// A highlight of a node marks its element: the class joins the element's
// own, the attribute names the highlights.
func TestNodeHighlights(t *testing.T) {
	h := *testHTML
	h.IDs = testIDs
	d := mustDoc(t, docOf(`{"type":"paragraph","content":[`+text("ab")+`]}`, `{"type":"embed","attrs":{"src":"e1"}}`, `{"type":"image","attrs":{"imageId":"i1","alt":"x"}}`))
	hs := []Highlight{{ID: "c2", Node: "e1"}, {ID: "c1", Node: "e1"}, {ID: "c3", Node: "i1"}, {ID: "c4", From: 1, To: 2}, {ID: "c5", Node: "gone"}}
	want := `<p><mark class="highlight" data-highlight="c4">a</mark>b</p>` +
		`<div class="embed highlight" data-src="e1" data-highlight="c1 c2"></div>` +
		`<img src="/img/i1" alt="x" class="highlight" data-highlight="c3">`
	if got := h.RenderHighlighted(d, testRes{}, hs); got != want {
		t.Errorf("\n got %s\nwant %s", got, want)
	}
	c := NewBlockCache(&h)
	if got := c.RenderHighlighted(d, testRes{}, hs); got != want {
		t.Errorf("cache:\n got %s", got)
	}
	if got := c.RenderHighlighted(d, testRes{}, hs[3:4]); got != `<p><mark class="highlight" data-highlight="c4">a</mark>b</p><div class="embed" data-src="e1"></div><img src="/img/i1" alt="x">` {
		t.Errorf("cache after the node highlights went: %s", got)
	}
	// A comparison marks the element of a whole block and keeps its class.
	if got := h.Diff(Diff(mustDoc(t, docOf(para)), mustDoc(t, docOf(para, `{"type":"embed","attrs":{"src":"e1"}}`))), testRes{}); got != `<p>hi</p><ins class="diff-ins"><div class="embed" data-src="e1" data-diff="ins"></div></ins>` {
		t.Errorf("diff: %s", got)
	}
}

// With DynamicKey the cache keeps a Dynamic block while its key stays.
func TestBlockCacheDynamicKey(t *testing.T) {
	h := *testHTML
	calls := 0
	h.DynamicKey = func(r testRes, n *Node) string { calls++; return r.Embeds[n.AttrString("src")] }
	d := mustDoc(t, docOf(`{"type":"embed","attrs":{"src":"/v/1"}}`))
	c := NewBlockCache(&h)
	r := testRes{Embeds: map[string]string{"/v/1": "one"}}
	first := c.Render(d, r)
	before := c.m[d.Child(0)]
	if got := c.Render(d, r); got != first || c.m[d.Child(0)].html != before.html || calls != 2 {
		t.Fatalf("same key: %s, %d calls", got, calls)
	}
	r.Embeds = map[string]string{"/v/1": "two"}
	if got := c.Render(d, r); got != `<div class="embed" data-src="/v/1">two</div>` {
		t.Errorf("a new key: %s", got)
	}
	r.Embeds = map[string]string{}
	if got := c.Render(d, r); got != `<div class="embed" data-src="/v/1"></div>` {
		t.Errorf("no key renders: %s", got)
	}
}

func TestDirectiveFormats(t *testing.T) {
	s := MustSchema([]byte(`{"nodes":[{"name":"doc","content":"block+"},{"name":"text","group":"inline"},
		{"name":"gallery","group":"block","atom":true,"attrs":{"photos":{"default":[]},"layout":{"default":"row","validate":"string"}}}]}`))
	md := &Markdown[testRes]{Directives: map[string]Directive{"gallery": {Attrs: []string{"photos", "layout"}, Formats: map[string]AttrFormat{"photos": SpaceList}}}}
	d, err := s.ParseDoc([]byte(`{"type":"doc","content":[{"type":"gallery","attrs":{"photos":["p1","p2"],"layout":"grid"}},{"type":"gallery"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	got := md.Render(d, testRes{})
	if got != "::gallery{photos=\"p1 p2\" layout=\"grid\"}\n\n::gallery\n" {
		t.Fatalf("got %q", got)
	}
	p, _, ok := ParseDirective(`::gallery{photos="p1 p2" layout="grid"}`)
	if !ok {
		t.Fatal("no parse")
	}
	n, err := md.DirectiveNode(s, p, nil)
	if err != nil || !n.Eq(d.Child(0)) {
		t.Errorf("back: %v %v", n, err)
	}
}

// NewID writes ids as coedit's randomId does: 9 random bytes, each in base
// 36 padded to two characters, cut to 16.
func TestNewID(t *testing.T) {
	format := regexp.MustCompile(`^[0-9a-z]{16}$`)
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := NewID()
		if !format.MatchString(id) || seen[id] {
			t.Fatalf("id %q", id)
		}
		seen[id] = true
	}
}
