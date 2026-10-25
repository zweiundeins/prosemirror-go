package prosemirror_test

import (
	"fmt"

	"github.com/zweiundeins/prosemirror-go"
)

// Names is the app's resolver: what its nodes show that the document does
// not hold.
type Names map[string]string

var schema = prosemirror.MustSchema([]byte(`{"nodes": [
	{"name": "doc", "content": "block+"},
	{"name": "paragraph", "group": "block", "content": "inline*"},
	{"name": "text", "group": "inline"},
	{"name": "mention", "group": "inline", "inline": true, "atom": true, "attrs": {"id": {"validate": "string"}}}
], "marks": [{"name": "strong"}]}`))

var html = &prosemirror.HTML[Names]{
	Nodes: map[string]func(w *prosemirror.HTMLWriter[Names], n *prosemirror.Node){
		"paragraph": func(w *prosemirror.HTMLWriter[Names], n *prosemirror.Node) { w.Open("p"); w.Content(n); w.Raw("</p>") },
		"mention": func(w *prosemirror.HTMLWriter[Names], n *prosemirror.Node) {
			w.Raw(`<a class="mention">@`)
			w.Text(w.R[n.AttrString("id")])
			w.Raw(`</a>`)
		},
	},
	Marks: map[string]prosemirror.MarkHTML[Names]{
		"strong": {Open: func(w *prosemirror.HTMLWriter[Names], m *prosemirror.Mark) string { w.Raw("<b>"); return "</b>" }},
	},
}

var rules = prosemirror.Rules{MaxBytes: 200 << 10, Nodes: map[string]func(n, parent *prosemirror.Node) error{
	"mention": func(n, _ *prosemirror.Node) error {
		if n.AttrString("id") == "" {
			return prosemirror.Refuse("A mention names a person.")
		}
		return nil
	},
}}

var markdown = &prosemirror.Markdown[Names]{
	Directives: map[string]prosemirror.Directive{"mention": {Attrs: []string{"id"}}},
}

func Example() {
	doc, _ := schema.ParseDoc([]byte(`{"type":"doc","content":[{"type":"paragraph","content":[
		{"type":"text","text":"Hi "},{"type":"mention","attrs":{"id":"u1"}}]}]}`))
	steps, _ := schema.ParseSteps([]byte(`[{"stepType":"addMark","from":1,"to":3,"mark":{"type":"strong"}}]`))
	next, _ := prosemirror.Apply(doc, steps)
	fmt.Println(rules.Check(doc, next))
	fmt.Println(html.Render(next, Names{"u1": "Ada"}))
	fmt.Print(markdown.Render(next, nil))
	// Output:
	// <nil>
	// <p><b>Hi</b> <a class="mention">@Ada</a></p>
	// Hi :mention{id="u1"}
}
