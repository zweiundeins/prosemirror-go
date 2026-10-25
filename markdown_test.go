package prosemirror

import (
	"strconv"
	"strings"
	"testing"
)

func mdLine(prefix string) func(w *MarkdownWriter[testRes], n *Node) {
	return func(w *MarkdownWriter[testRes], n *Node) { w.Line(prefix + w.Inline(n)) }
}

// testMarkdown writes the test schema as an app would: Markdown's own
// syntax where it has one, directives for the app's nodes.
var testMarkdown = &Markdown[testRes]{
	Nodes: map[string]func(w *MarkdownWriter[testRes], n *Node){
		"heading": func(w *MarkdownWriter[testRes], n *Node) {
			l, _ := n.AttrInt("level")
			w.Line(strings.Repeat("#", l) + " " + w.Inline(n))
		},
		"blockquote":      func(w *MarkdownWriter[testRes], n *Node) { w.Blocks(n.Content().Nodes(), "> ", "> ") },
		"horizontal_rule": func(w *MarkdownWriter[testRes], n *Node) { w.Line("---") },
		"code_block": func(w *MarkdownWriter[testRes], n *Node) {
			w.Line("```" + n.AttrString("language"))
			for _, l := range strings.Split(n.TextBetween(0, n.Content().Size(), "", nil), "\n") {
				w.Line(l)
			}
			w.Line("```")
		},
		"bullet_list": mdList, "ordered_list": mdList, "task_list": mdList,
		"hard_break": func(w *MarkdownWriter[testRes], n *Node) { w.Write("\\\n") },
		"image": func(w *MarkdownWriter[testRes], n *Node) {
			w.Line("![" + MarkdownEscape(n.AttrString("alt")) + "](/img/" + n.AttrString("imageId") + ")")
		},
	},
	Marks: map[string]func(w *MarkdownWriter[testRes], m *Mark, n *Node, s string) string{
		"strong": func(w *MarkdownWriter[testRes], m *Mark, n *Node, s string) string { return "**" + s + "**" },
		"em":     func(w *MarkdownWriter[testRes], m *Mark, n *Node, s string) string { return "*" + s + "*" },
		"link": func(w *MarkdownWriter[testRes], m *Mark, n *Node, s string) string {
			if !n.IsText() {
				return s
			}
			return "[" + s + "](<" + m.AttrString("href") + ">)"
		},
	},
	Directives: map[string]Directive{
		"tag":     {Label: "name"},
		"mention": {Label: "label", Attrs: []string{"id"}},
		"xref":    {Name: "ref", Attrs: []string{"target"}},
		"embed":   {Label: "title", Attrs: []string{"src", "height"}},
	},
}

func mdList(w *MarkdownWriter[testRes], n *Node) {
	order, _ := n.AttrInt("order")
	for i, item := range n.Content().Nodes() {
		if i > 0 && item.ChildCount() > 1 {
			w.BlankLine()
		}
		marker := "- "
		switch {
		case n.Type().Name() == "ordered_list":
			marker = strconv.Itoa(order+i) + ". "
		case item.AttrBool("checked"):
			marker = "- [x] "
		case n.Type().Name() == "task_list":
			marker = "- [ ] "
		}
		w.Blocks(item.Content().Nodes(), marker, strings.Repeat(" ", len(marker)))
	}
}

func TestMarkdown(t *testing.T) {
	d := mustDoc(t, docOf(
		`{"type":"heading","attrs":{"level":2},"content":[`+text("Plan")+`]}`,
		`{"type":"paragraph","content":[`+text("a ")+`,`+text("bold", strong)+`,`+text(" *star* ")+`,`+text("x", link("https://x.y"))+`,{"type":"hard_break"},`+text("next")+`]}`,
		`{"type":"task_list","content":[{"type":"task_item","attrs":{"checked":true},"content":[{"type":"paragraph","content":[`+text("done")+`]}]},{"type":"task_item","content":[{"type":"paragraph","content":[`+text("open")+`]},{"type":"bullet_list","content":[{"type":"list_item","content":[`+para+`]}]}]}]}`,
		`{"type":"ordered_list","attrs":{"order":3},"content":[{"type":"list_item","content":[{"type":"paragraph","content":[`+text("three")+`]}]}]}`,
		`{"type":"blockquote","content":[`+para+`,{"type":"code_block","attrs":{"language":"go"},"content":[`+text("x := 1\ny")+`]}]}`,
		`{"type":"horizontal_rule"}`,
		`{"type":"image","attrs":{"imageId":"img-1","alt":"a [diagram]"}}`,
	))
	want := "## Plan\n\na **bold** \\*star\\* [x](<https://x.y>)\\\nnext\n\n- [x] done\n\n- [ ] open\n\n      - hi\n\n3. three\n\n> hi\n>\n> ```go\n> x := 1\n> y\n> ```\n\n---\n\n![a \\[diagram\\]](/img/img-1)\n"
	if got := testMarkdown.Render(d, testResolver); got != want {
		t.Errorf("markdown:\n%s\nwant\n%s", got, want)
	}
}

// App nodes write as directives and read back into the same nodes.
func TestDirectives(t *testing.T) {
	d := mustDoc(t, docOf(
		`{"type":"paragraph","content":[`+text("see ")+`,{"type":"tag","attrs":{"name":"a]b"}},{"type":"mention","attrs":{"id":"ada \"x\"","label":"Ada"}},{"type":"xref","attrs":{"target":"p-1"}}]}`,
		`{"type":"embed","attrs":{"src":"/v/1","title":"Grimsel","height":12}}`,
		`{"type":"embed","attrs":{"src":"/v/2"}}`,
	))
	want := "see :tag[a\\]b]:mention[Ada]{id=\"ada &quot;x&quot;\"}:ref{target=\"p-1\"}\n\n::embed[Grimsel]{src=\"/v/1\" height=\"12\"}\n\n::embed{src=\"/v/2\"}\n"
	got := testMarkdown.Render(d, testResolver)
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	inline := d.Child(0).Content().Nodes()[1:]
	rest := strings.TrimPrefix(strings.SplitN(got, "\n", 2)[0], "see ")
	for i, n := range inline {
		p, k, ok := ParseDirective(rest)
		if !ok {
			t.Fatalf("parse %q", rest)
		}
		rest = rest[k:]
		back, err := testMarkdown.DirectiveNode(testSchema, p, nil)
		if err != nil || !back.Eq(n) {
			t.Errorf("inline %d: %v %v, want %v", i, back, err, n)
		}
	}
	for i, line := range []string{"::embed[Grimsel]{src=\"/v/1\" height=\"12\"}", "::embed{src=\"/v/2\"}"} {
		p, k, ok := ParseDirective(line)
		if !ok || k != len(line) || p.Colons != 2 {
			t.Fatalf("parse %q: %+v %d", line, p, k)
		}
		back, err := testMarkdown.DirectiveNode(testSchema, p, nil)
		if err != nil || !back.Eq(d.Child(1+i)) {
			t.Errorf("block %d: %v %v", i, back, err)
		}
	}
	// Containers: blocks inside, fences longer outside.
	notes := &Markdown[testRes]{Directives: map[string]Directive{"blockquote": {Name: "note"}}}
	q := mustDoc(t, docOf(`{"type":"blockquote","content":[`+para+`,{"type":"blockquote","content":[`+para+`]}]}`))
	if got := notes.Render(q, testRes{}); got != "::::note\nhi\n\n:::note\nhi\n:::\n::::\n" {
		t.Errorf("containers:\n%s", got)
	}
}

func TestParseDirective(t *testing.T) {
	cases := []struct {
		in    string
		n     int
		name  string
		label string
		attrs [][2]string
	}{
		{`:stop[Grimselpass]{photo="g1"} on`, 30, "stop", "Grimselpass", [][2]string{{"photo", "g1"}}},
		{`::route{from=2026-07-14T07:30Z to='x y' #main .wide hidden}`, 59, "route", "", [][2]string{{"from", "2026-07-14T07:30Z"}, {"to", "x y"}, {"id", "main"}, {"class", "wide"}, {"hidden", ""}}},
		{`:a[x [y] \] z]`, 14, "a", `x [y] \] z`, nil},
		{`:::note{kind="tip"}`, 19, "note", "", [][2]string{{"kind", "tip"}}},
	}
	for _, c := range cases {
		p, n, ok := ParseDirective(c.in)
		if !ok || n != c.n || p.Name != c.name || p.Label != c.label || len(p.Attrs) != len(c.attrs) {
			t.Errorf("%q: %+v %d %v", c.in, p, n, ok)
			continue
		}
		for i := range c.attrs {
			if p.Attrs[i] != c.attrs[i] {
				t.Errorf("%q: attr %d %v", c.in, i, p.Attrs[i])
			}
		}
	}
	for _, bad := range []string{"plain", ":", ":1a", ":a[open", `:a{x="open}`} {
		if _, _, ok := ParseDirective(bad); ok {
			t.Errorf("%q parsed", bad)
		}
	}
	if _, err := testMarkdown.DirectiveNode(testSchema, ParsedDirective{Name: "nope"}, nil); err == nil {
		t.Error("an unknown directive made a node")
	}
	if _, err := testMarkdown.DirectiveNode(testSchema, ParsedDirective{Name: "embed", Attrs: [][2]string{{"height", "tall"}}}, nil); err == nil {
		t.Error("a number attribute took a word")
	}
}
