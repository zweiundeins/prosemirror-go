package prosemirror

import (
	"slices"
	"strconv"
	"strings"
	"sync"
)

// HTML renders documents as HTML, as prosemirror-model's DOMSerializer
// does in the browser: per node type and per mark type a function writes
// the element. R is what the app hands every render, its resolver: what
// nodes show that the document does not hold (a person's name, an issue's
// status now, a number computed from other data). A document holds ids;
// the renderer looks up what they stand for at render time.
//
// A node type without a function renders its content (an inline leaf:
// nothing), a mark type without one a <span>. Text is always escaped.
// Every function escapes what it writes from the document (Text, Open).
type HTML[R any] struct {
	// Nodes write a node's HTML by its type name. A block writes its
	// element with Open and its content with Content, so that a
	// comparison of versions can mark the element and replace the
	// content; a leaf writes what it shows.
	Nodes map[string]func(w *HTMLWriter[R], n *Node)
	// Marks open a mark's element by its type name.
	Marks map[string]MarkHTML[R]
	// Dynamic names the node types whose HTML can change while the node
	// does not (a list of query results rendered for each reader):
	// BlockCache renders the blocks that are or hold one every time.
	Dynamic []string
	// HighlightClass and HighlightAttr make the element of highlighted
	// text (RenderHighlighted): <mark class="highlight"
	// data-highlight="<ids>"> by default; a highlighted node's element
	// gets the same class and attribute.
	HighlightClass, HighlightAttr string
	// IDs are the node types with a stable id, for highlights of nodes.
	IDs IDs
	// DynamicKey names what a Dynamic node shows for r now: BlockCache
	// keeps a block whose Dynamic nodes' keys stay the same. Without it,
	// or with "", the block renders every time.
	DynamicKey func(r R, n *Node) string
}

// MarkHTML renders a mark type.
type MarkHTML[R any] struct {
	// Open writes the mark's start tag and returns its end tag.
	Open func(w *HTMLWriter[R], m *Mark) (close string)
	// TextOnly marks wrap text alone: an inline node that is not text
	// renders outside them, as a link mark leaves out a chip that is a
	// link of its own (anchors do not nest).
	TextOnly bool
}

// HTMLWriter is what a node or mark function writes with. R is the
// render's resolver.
type HTMLWriter[R any] struct {
	R R

	h      *HTML[R]
	b      *strings.Builder
	hl     *hlState          // nil: no highlights of text
	nodeHL map[string]string // highlight ids by node id
	marked string            // the highlight ids of the current block's element

	// What the current block's element and content get instead of the
	// usual: attributes Open adds, and a body Content renders.
	extra   string
	body    func()
	pending *Node
}

// Raw writes trusted HTML.
func (w *HTMLWriter[R]) Raw(s string) { w.b.WriteString(s) }

// Text writes s escaped for text and attribute values: &, <, >, " and '.
func (w *HTMLWriter[R]) Text(s string) { escapeHTML(w.b, s) }

// Open writes a start tag with attributes given as name and value pairs,
// the values escaped, and closes it after what the renderer adds to the
// current block's element (data-diff in a comparison). A block function
// calls it once, for its outermost element.
func (w *HTMLWriter[R]) Open(tag string, attrs ...string) {
	marked := w.marked
	w.marked = ""
	class, attr := w.h.highlight()
	w.b.WriteString("<" + tag)
	for i := 0; i+1 < len(attrs); i += 2 {
		v := attrs[i+1]
		if marked != "" && attrs[i] == "class" {
			v, class = v+" "+class, ""
		}
		w.b.WriteString(" " + attrs[i] + `="`)
		escapeHTML(w.b, v)
		w.b.WriteByte('"')
	}
	if marked != "" {
		if class != "" {
			w.b.WriteString(` class="` + class + `"`)
		}
		w.b.WriteString(" " + attr + `="`)
		escapeHTML(w.b, marked)
		w.b.WriteByte('"')
	}
	w.b.WriteString(w.extra)
	w.extra = ""
	w.b.WriteByte('>')
}

func (h *HTML[R]) highlight() (class, attr string) {
	class, attr = h.HighlightClass, h.HighlightAttr
	if class == "" {
		class = "highlight"
	}
	if attr == "" {
		attr = "data-highlight"
	}
	return class, attr
}

// Content writes n's content: its blocks, its inline content with marks
// (and highlights), or a code block's text.
func (w *HTMLWriter[R]) Content(n *Node) {
	if w.pending == n && w.body != nil {
		b := w.body
		w.body = nil
		b()
		return
	}
	w.defaultBody(n)
}

// EscapeHTML escapes s as Text does, for HTML an app writes outside a
// render.
func EscapeHTML(s string) string {
	var b strings.Builder
	escapeHTML(&b, s)
	return b.String()
}

func escapeHTML(b *strings.Builder, s string) {
	start := 0
	for i := 0; i < len(s); i++ {
		var rep string
		switch s[i] {
		case '&':
			rep = "&amp;"
		case '<':
			rep = "&lt;"
		case '>':
			rep = "&gt;"
		case '"':
			rep = "&#34;"
		case '\'':
			rep = "&#39;"
		default:
			continue
		}
		b.WriteString(s[start:i])
		b.WriteString(rep)
		start = i + 1
	}
	b.WriteString(s[start:])
}

func (h *HTML[R]) writer(b *strings.Builder, r R) *HTMLWriter[R] {
	return &HTMLWriter[R]{R: r, h: h, b: b}
}

// Render renders the document's blocks; the caller writes the container
// around them.
func (h *HTML[R]) Render(d Doc, r R) string {
	var b strings.Builder
	b.Grow(d.content.size * 2)
	w := h.writer(&b, r)
	for _, n := range d.content.nodes {
		w.block(n)
	}
	return b.String()
}

// RenderBlock renders one block.
func (h *HTML[R]) RenderBlock(n *Node, r R) string {
	var b strings.Builder
	h.writer(&b, r).block(n)
	return b.String()
}

func (w *HTMLWriter[R]) block(n *Node) {
	if w.hl == nil {
		w.wrap(n, "", nil)
		return
	}
	start := w.hl.pos
	w.hl.pos = start + 1
	w.wrap(n, "", nil)
	w.hl.pos = start + n.size
}

// wrap renders a block with extra attributes on its element and body in
// place of its content; body nil renders the content.
func (w *HTMLWriter[R]) wrap(n *Node, extra string, body func()) {
	pe, pb, pn, pm := w.extra, w.body, w.pending, w.marked
	w.extra, w.body, w.pending, w.marked = extra, body, n, ""
	if w.nodeHL != nil {
		if id, ok := w.h.IDs.Of(n); ok && id != "" {
			w.marked = w.nodeHL[id]
		}
	}
	if f := w.h.Nodes[n.typ.name]; f != nil {
		f(w, n)
	} else {
		w.Content(n)
	}
	w.extra, w.body, w.pending, w.marked = pe, pb, pn, pm
}

// defaultBody renders a node's content the usual way.
func (w *HTMLWriter[R]) defaultBody(n *Node) {
	switch {
	case n.typ.code:
		for _, t := range n.content.nodes {
			w.text(t)
			w.advance(t)
		}
	case n.typ.inlineContent:
		w.inline(n.content.nodes)
	default:
		for _, c := range n.content.nodes {
			w.block(c)
		}
	}
}

type openMark struct {
	m     *Mark
	close string
}

// inline renders inline content with DOMSerializer's mark nesting: marks
// shared with the previous node stay open.
func (w *HTMLWriter[R]) inline(nodes []*Node) {
	var active []openMark
	for _, n := range nodes {
		marks := n.marks
		if !n.typ.isText {
			marks = slices.DeleteFunc(slices.Clone(marks), func(m *Mark) bool { return w.h.Marks[m.typ.name].TextOnly })
		}
		keep := 0
		for keep < len(active) && keep < len(marks) && marks[keep].Eq(active[keep].m) {
			keep++
		}
		for len(active) > keep {
			w.b.WriteString(active[len(active)-1].close)
			active = active[:len(active)-1]
		}
		for _, m := range marks[keep:] {
			active = append(active, openMark{m, w.openMark(m)})
		}
		w.leaf(n)
		w.advance(n)
	}
	for i := len(active) - 1; i >= 0; i-- {
		w.b.WriteString(active[i].close)
	}
}

func (w *HTMLWriter[R]) openMark(m *Mark) string {
	if f := w.h.Marks[m.typ.name].Open; f != nil {
		return f(w, m)
	}
	w.b.WriteString("<span>")
	return "</span>"
}

func (w *HTMLWriter[R]) leaf(n *Node) {
	if n.typ.isText {
		w.text(n)
		return
	}
	if f := w.h.Nodes[n.typ.name]; f != nil {
		f(w, n)
		return
	}
	if n.content.size > 0 {
		w.inline(n.content.nodes)
	}
}

// BlockCache renders documents block by block and keeps the HTML of each
// top-level block by node, so a commit re-renders only the blocks it
// changed. Reset it when what the resolver returns changes (a chip's
// status, a page's title). It is safe for concurrent use.
type BlockCache[R any] struct {
	h  *HTML[R]
	mu sync.Mutex
	m  map[*Node]cachedBlock
}

// NewBlockCache returns a cache of h's renders.
func NewBlockCache[R any](h *HTML[R]) *BlockCache[R] { return &BlockCache[R]{h: h} }

// cachedBlock is a block's HTML and the highlights inside it, relative to
// its start, that it was rendered with.
type cachedBlock struct {
	html, marks string
	live        bool   // is or holds a Dynamic node
	key         string // the Dynamic nodes' keys it was rendered with
}

// Render renders the document, reusing the HTML of unchanged blocks, and
// forgets blocks the document no longer holds.
func (c *BlockCache[R]) Render(d Doc, r R) string { return c.RenderHighlighted(d, r, nil) }

// RenderHighlighted renders as Render does, with highlights as
// HTML.RenderHighlighted renders them; a block renders again when the
// highlights inside it change.
func (c *BlockCache[R]) RenderHighlighted(d Doc, r R, hs []Highlight) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	nodes := nodeHighlights(hs)
	hs = sortHighlights(hs)
	next := make(map[*Node]cachedBlock, len(d.content.nodes))
	var b strings.Builder
	pos := 0
	for _, n := range d.content.nodes {
		marks := highlightsIn(hs, pos, pos+n.size) + c.h.nodeKey(n, nodes)
		e, ok := c.m[n]
		if !ok {
			e.live = c.h.dynamic(n)
		}
		key := ""
		if e.live {
			key = c.h.dynamicKey(r, n)
		}
		if !ok || e.live && (key == "" || key != e.key) || e.marks != marks {
			e.html, e.marks, e.key = c.h.renderBlockAt(n, r, pos, hs, nodes), marks, key
		}
		next[n] = e
		b.WriteString(e.html)
		pos += n.size
	}
	c.m = next
	return b.String()
}

// Reset forgets every block.
func (c *BlockCache[R]) Reset() {
	c.mu.Lock()
	c.m = nil
	c.mu.Unlock()
}

// dynamicKey is the keys of the Dynamic nodes n is or holds, "" when one
// has none.
func (h *HTML[R]) dynamicKey(r R, n *Node) string {
	if h.DynamicKey == nil {
		return ""
	}
	var b strings.Builder
	ok := true
	var walk func(n *Node)
	walk = func(n *Node) {
		if !ok {
			return
		}
		if slices.Contains(h.Dynamic, n.typ.name) {
			k := h.DynamicKey(r, n)
			if k == "" {
				ok = false
				return
			}
			b.WriteString(strconv.Itoa(len(k)) + ":" + k)
		}
		for _, c := range n.content.nodes {
			walk(c)
		}
	}
	walk(n)
	if !ok {
		return ""
	}
	return b.String()
}

// dynamic reports whether n is or holds a node of a Dynamic type.
func (h *HTML[R]) dynamic(n *Node) bool {
	if len(h.Dynamic) == 0 {
		return false
	}
	if slices.Contains(h.Dynamic, n.typ.name) {
		return true
	}
	for _, c := range n.content.nodes {
		if h.dynamic(c) {
			return true
		}
	}
	return false
}
