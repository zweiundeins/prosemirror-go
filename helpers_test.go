package prosemirror

import (
	"encoding/json"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
)

// testSchema is testdata/schema.json: the nodes of a typical editor
// (paragraphs, headings, lists, task lists, tables, images) and four app
// nodes (tag, mention and xref inline, embed as a block), which the oracle
// builds too.
var testSchema = func() *Schema {
	b, err := os.ReadFile("testdata/schema.json")
	if err != nil {
		panic(err)
	}
	return MustSchema(b)
}()

func parseDoc(data []byte) (Doc, error)      { return testSchema.ParseDoc(data) }
func parseSteps(data []byte) ([]Step, error) { return testSchema.ParseSteps(data) }

func mustDoc(t testing.TB, s string) Doc {
	t.Helper()
	d, err := parseDoc([]byte(s))
	if err != nil {
		t.Fatalf("%s: %v", s, err)
	}
	return d
}

func docOf(blocks ...string) string {
	return `{"type":"doc","content":[` + strings.Join(blocks, ",") + `]}`
}

const para = `{"type":"paragraph","content":[{"type":"text","text":"hi"}]}`

func inPara(inline string) string { return docOf(`{"type":"paragraph","content":[` + inline + `]}`) }

func text(s string, marks ...string) string {
	if len(marks) == 0 {
		return `{"type":"text","text":` + string(appendString(nil, s)) + `}`
	}
	return `{"type":"text","text":` + string(appendString(nil, s)) + `,"marks":[` + strings.Join(marks, ",") + `]}`
}

const (
	strong = `{"type":"strong"}`
	em     = `{"type":"em"}`
)

func link(href string) string {
	return `{"type":"link","attrs":{"href":` + string(appendString(nil, href)) + `}}`
}

// testRes is the resolver of the test renderers: people's names by id,
// and what each embed shows now.
type testRes struct {
	People map[string]string
	Embeds map[string]string
}

var testResolver = testRes{
	People: map[string]string{"ada": `Ada "Countess"`},
	Embeds: map[string]string{"/v/1": "<b>live</b>"},
}

// testHTML renders the test schema as an app would.
var testHTML = &HTML[testRes]{
	Nodes: map[string]func(w *HTMLWriter[testRes], n *Node){
		"paragraph":  func(w *HTMLWriter[testRes], n *Node) { w.Open("p"); w.Content(n); w.Raw("</p>") },
		"blockquote": func(w *HTMLWriter[testRes], n *Node) { w.Open("blockquote"); w.Content(n); w.Raw("</blockquote>") },
		"heading": func(w *HTMLWriter[testRes], n *Node) {
			l, _ := n.AttrInt("level")
			tag := "h" + strconv.Itoa(min(max(l, 1), 6))
			w.Open(tag)
			w.Content(n)
			w.Raw("</" + tag + ">")
		},
		"code_block": func(w *HTMLWriter[testRes], n *Node) {
			if l := n.AttrString("language"); l != "" {
				w.Open("pre", "data-language", l)
			} else {
				w.Open("pre")
			}
			w.Raw("<code>")
			w.Content(n)
			w.Raw("</code></pre>")
		},
		"horizontal_rule": func(w *HTMLWriter[testRes], n *Node) { w.Open("hr") },
		"bullet_list":     func(w *HTMLWriter[testRes], n *Node) { w.Open("ul"); w.Content(n); w.Raw("</ul>") },
		"list_item":       func(w *HTMLWriter[testRes], n *Node) { w.Open("li"); w.Content(n); w.Raw("</li>") },
		"hard_break":      func(w *HTMLWriter[testRes], n *Node) { w.Raw("<br>") },
		"image": func(w *HTMLWriter[testRes], n *Node) {
			w.Open("img", "src", "/img/"+url.PathEscape(n.AttrString("imageId")), "alt", n.AttrString("alt"))
		},
		"mention": func(w *HTMLWriter[testRes], n *Node) {
			name, ok := w.R.People[n.AttrString("id")]
			if !ok {
				name = n.AttrString("label")
			}
			w.Raw(`<a class="mention">@`)
			w.Text(name)
			w.Raw(`</a>`)
		},
		"tag": func(w *HTMLWriter[testRes], n *Node) { w.Raw("#"); w.Text(n.AttrString("name")) },
		"embed": func(w *HTMLWriter[testRes], n *Node) {
			w.Open("div", "class", "embed", "data-src", n.AttrString("src"))
			w.Raw(w.R.Embeds[n.AttrString("src")])
			w.Raw("</div>")
		},
	},
	Marks: map[string]MarkHTML[testRes]{
		"strong": {Open: func(w *HTMLWriter[testRes], m *Mark) string { w.Raw("<strong>"); return "</strong>" }},
		"em":     {Open: func(w *HTMLWriter[testRes], m *Mark) string { w.Raw("<em>"); return "</em>" }},
		"link": {TextOnly: true, Open: func(w *HTMLWriter[testRes], m *Mark) string {
			w.Raw(`<a href="`)
			w.Text(m.AttrString("href"))
			w.Raw(`">`)
			return "</a>"
		}},
	},
	Dynamic: []string{"embed"},
}

var testLeaves = LeafText{
	"hard_break": func(*Node) string { return "\n" },
	"mention":    func(n *Node) string { return "@" + n.AttrString("label") },
	"tag":        func(n *Node) string { return "#" + n.AttrString("name") },
	"image":      func(n *Node) string { return n.AttrString("alt") },
}

func readFixture(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// rawList splits a JSON array into its elements.
func rawList(t testing.TB, data []byte) []json.RawMessage {
	t.Helper()
	var out []json.RawMessage
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// The fixtures' documents, written in V8 by oracle/gen-fixtures.mjs; the
// oracle module's tests check them against ProseMirror.
var docFixtures = []string{
	"f20k-doc", "f20k-final", "f200k-doc", "f200k-final",
	"f20k-k0-final", "f20k-k1-final", "f20k-k5-final", "f20k-k20-final", "f200k-k5-final",
}

type commitJSON struct {
	Base    int             `json:"base"`
	Steps   json.RawMessage `json:"steps"`
	Applied json.RawMessage `json:"applied"`
	Error   *string         `json:"error"`
}
