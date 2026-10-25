package prosemirror

import (
	"strings"
)

// Markdown writes documents as Markdown, as prosemirror-markdown's
// MarkdownSerializer does: per node type and per mark type a function, and
// R the app's resolver for the render, as HTML has it.
//
// A node type without a function is written as its Directive when it has
// one, else a textblock as a paragraph, another block as its content and
// an inline leaf as nothing. Text is escaped.
type Markdown[R any] struct {
	// Nodes write a node by its type name. A block writes lines (Line)
	// and nested blocks (Blocks); an inline node writes its text (Write).
	Nodes map[string]func(w *MarkdownWriter[R], n *Node)
	// Marks wrap s, the Markdown of node n, in a mark by its type name.
	Marks map[string]func(w *MarkdownWriter[R], m *Mark, n *Node, s string) string
	// Directives write node types that Markdown has no syntax for as
	// generic directives (directive.go).
	Directives map[string]Directive
}

// MarkdownWriter is what a node or mark function writes with. R is the
// render's resolver.
type MarkdownWriter[R any] struct {
	R R

	md          *Markdown[R]
	b           strings.Builder
	first, rest string // the prefix of the block's first line and of the others
	used        bool   // a line was written with first
	out         *strings.Builder
}

// Render writes the document as Markdown, blocks separated by a blank
// line, ending with one newline.
func (m *Markdown[R]) Render(d Doc, r R) string {
	w := &MarkdownWriter[R]{R: r, md: m}
	w.blocks(d.content.nodes, "", "")
	return strings.TrimRight(w.b.String(), "\n") + "\n"
}

// Line writes a line of the current block, after its prefix: a quote's
// "> ", a list item's marker on the first line and its indent on the
// others.
func (w *MarkdownWriter[R]) Line(s string) {
	p := w.rest
	if !w.used {
		p, w.used = w.first, true
	}
	w.b.WriteString(p + s + "\n")
}

// BlankLine writes an empty line of the current block, as between two
// items of a loose list.
func (w *MarkdownWriter[R]) BlankLine() { w.b.WriteString(strings.TrimRight(w.rest, " ") + "\n") }

// Blocks writes blocks nested in the current one, separated by blank
// lines: the first line of the first after the current prefix and first,
// every other line after the current block's indent and rest. A quote
// passes "> " twice, a list item its marker and as many spaces.
func (w *MarkdownWriter[R]) Blocks(nodes []*Node, first, rest string) {
	f := w.rest + first
	if !w.used {
		f = w.first + first
	}
	of, or := w.first, w.rest
	w.blocks(nodes, f, w.rest+rest)
	w.first, w.rest, w.used = of, or, true
}

func (w *MarkdownWriter[R]) blocks(nodes []*Node, first, rest string) {
	for i, n := range nodes {
		if i > 0 {
			w.b.WriteString(strings.TrimRight(rest, " ") + "\n")
		}
		w.first, w.rest, w.used = first, rest, false
		w.block(n)
		first = rest
	}
}

func (w *MarkdownWriter[R]) block(n *Node) {
	if f := w.md.Nodes[n.typ.name]; f != nil {
		f(w, n)
		return
	}
	if d, ok := w.md.Directives[n.typ.name]; ok {
		w.Directive(n, d)
		return
	}
	if n.typ.inlineContent {
		for _, l := range strings.Split(w.Inline(n), "\n") {
			w.Line(l)
		}
		return
	}
	w.Blocks(n.content.nodes, "", "")
}

// Write writes the text of an inline node.
func (w *MarkdownWriter[R]) Write(s string) {
	if w.out != nil {
		w.out.WriteString(s)
	}
}

// Inline returns the Markdown of n's inline content: text escaped, each
// node wrapped in its marks, innermost first.
func (w *MarkdownWriter[R]) Inline(n *Node) string {
	var b strings.Builder
	for _, c := range n.content.nodes {
		var s string
		if c.typ.isText {
			s = MarkdownEscape(c.text)
		} else {
			var one strings.Builder
			prev := w.out
			w.out = &one
			if f := w.md.Nodes[c.typ.name]; f != nil {
				f(w, c)
			} else if d, ok := w.md.Directives[c.typ.name]; ok {
				w.Directive(c, d)
			}
			w.out = prev
			s = one.String()
		}
		for i := len(c.marks) - 1; i >= 0; i-- {
			if f := w.md.Marks[c.marks[i].typ.name]; f != nil {
				s = f(w, c.marks[i], c, s)
			}
		}
		b.WriteString(s)
	}
	return b.String()
}

var mdSpecial = strings.NewReplacer(`\`, `\\`, "`", "\\`", `*`, `\*`, `_`, `\_`, `[`, `\[`, `]`, `\]`, `<`, `\<`, `>`, `\>`, `#`, `\#`, `|`, `\|`, `~`, `\~`)

// MarkdownEscape escapes the characters of s that Markdown would read as
// syntax.
func MarkdownEscape(s string) string { return mdSpecial.Replace(s) }
