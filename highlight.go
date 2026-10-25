package prosemirror

import (
	"slices"
	"strconv"
	"strings"
)

// Highlight is a part of a document that a render shows marked, such as
// what an inline comment is on: a range of text, or with Node a node by
// its stable id (HTML.IDs), whose element is marked as a whole. Its
// positions are the document's; the highlights live outside the document,
// so the schema knows nothing of them.
type Highlight struct {
	ID       string
	From, To int
	Node     string // the id of a node of a type in HTML.IDs; From and To are then unused
}

// hlState is where the renderer is in the document, and the ranges it
// marks, sorted.
type hlState struct {
	pos int
	hs  []Highlight
}

// RenderHighlighted renders the document as Render does, with the text
// inside each highlight in <mark class="highlight" data-highlight="<ids>">
// (HighlightClass, HighlightAttr), the ids of every highlight over it
// separated by spaces. Inline nodes that are not text stay unmarked; a
// range outside the document marks what it covers of it. A highlight of
// a node adds the class and the attribute to the element its function
// opens with Open.
func (h *HTML[R]) RenderHighlighted(d Doc, r R, hs []Highlight) string {
	nodes := nodeHighlights(hs)
	hs = sortHighlights(hs)
	var b strings.Builder
	b.Grow(d.content.size * 2)
	pos := 0
	for _, n := range d.content.nodes {
		b.WriteString(h.renderBlockAt(n, r, pos, hs, nodes))
		pos += n.size
	}
	return b.String()
}

// renderBlockAt renders a block that starts at pos.
func (h *HTML[R]) renderBlockAt(n *Node, r R, pos int, hs []Highlight, nodes map[string]string) string {
	var b strings.Builder
	w := h.writer(&b, r)
	if highlightsIn(hs, pos, pos+n.size) != "" {
		w.hl = &hlState{pos: pos, hs: hs}
	}
	w.nodeHL = nodes
	w.block(n)
	return b.String()
}

// nodeHighlights are the highlights of nodes: by node id, the ids of the
// highlights on it, sorted and separated by spaces.
func nodeHighlights(hs []Highlight) map[string]string {
	var m map[string][]string
	for _, h := range hs {
		if h.Node != "" && h.ID != "" {
			if m == nil {
				m = map[string][]string{}
			}
			m[h.Node] = append(m[h.Node], h.ID)
		}
	}
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, ids := range m {
		slices.Sort(ids)
		out[k] = strings.Join(slices.Compact(ids), " ")
	}
	return out
}

// nodeKey names the node highlights inside a block: its part of the
// block's key in the BlockCache.
func (h *HTML[R]) nodeKey(n *Node, nodes map[string]string) string {
	if len(nodes) == 0 || len(h.IDs) == 0 {
		return ""
	}
	var b strings.Builder
	h.IDs.each(n, func(_ *Node, id string) {
		if hl, ok := nodes[id]; ok {
			b.WriteString("#" + id + "=" + hl + ";")
		}
	})
	return b.String()
}

func sortHighlights(hs []Highlight) []Highlight {
	out := slices.DeleteFunc(slices.Clone(hs), func(h Highlight) bool { return h.From >= h.To || h.ID == "" || h.Node != "" })
	slices.SortFunc(out, func(a, b Highlight) int {
		if a.From != b.From {
			return a.From - b.From
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out
}

// highlightsIn names the highlights over from to to, relative to from: a
// block's key in the BlockCache.
func highlightsIn(hs []Highlight, from, to int) string {
	var b strings.Builder
	for _, h := range hs {
		if h.To > from && h.From < to {
			b.WriteString(h.ID)
			b.WriteByte(':')
			b.WriteString(strconv.Itoa(max(h.From, from) - from))
			b.WriteByte('-')
			b.WriteString(strconv.Itoa(min(h.To, to) - from))
			b.WriteByte(';')
		}
	}
	return b.String()
}

// text writes a text node, its highlighted parts marked.
func (w *HTMLWriter[R]) text(n *Node) {
	if w.hl == nil {
		w.Text(n.text)
		return
	}
	start, end := w.hl.pos, w.hl.pos+n.size
	cuts := []int{0, n.size}
	for _, h := range w.hl.hs {
		if h.To > start && h.From < end {
			cuts = append(cuts, max(h.From, start)-start, min(h.To, end)-start)
		}
	}
	slices.Sort(cuts)
	cuts = slices.Compact(cuts)
	for i := 0; i+1 < len(cuts); i++ {
		a, b := safeByteOffset(n.text, n.size, cuts[i]), safeByteOffset(n.text, n.size, cuts[i+1])
		if a >= b {
			continue
		}
		var ids []string
		for _, h := range w.hl.hs {
			if h.From <= start+cuts[i] && h.To >= start+cuts[i+1] {
				ids = append(ids, h.ID)
			}
		}
		if len(ids) == 0 {
			w.Text(n.text[a:b])
			continue
		}
		class, attr := w.h.highlight()
		w.Raw(`<mark class="` + class + `" ` + attr + `="`)
		w.Text(strings.Join(ids, " "))
		w.Raw(`">`)
		w.Text(n.text[a:b])
		w.Raw(`</mark>`)
	}
}

// advance moves the renderer's position past an inline node.
func (w *HTMLWriter[R]) advance(n *Node) {
	if w.hl != nil {
		w.hl.pos += n.size
	}
}

// safeByteOffset is byteOffset that moves a cut inside a surrogate pair
// to before it.
func safeByteOffset(s string, units, off int) (b int) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(splitPanic); !ok {
				panic(r)
			}
			b = safeByteOffset(s, units, off-1)
		}
	}()
	return byteOffset(s, units, off)
}

// InlineText is the text between from and to without separators: the
// characters of its text nodes alone, to compare with what a person
// selected. ok is false for a range outside the document, a
// reversed one, and one that cuts a character in half.
func InlineText(d Doc, from, to int) (s string, ok bool) {
	if from < 0 || to > d.content.size || from > to {
		return "", false
	}
	defer func() {
		if r := recover(); r != nil {
			if _, split := r.(splitPanic); !split {
				panic(r)
			}
			s, ok = "", false
		}
	}()
	return d.TextBetween(from, to, "", func(*Node) string { return "" }), true
}

// Find is the range of the first text node that holds s.
func Find(d Doc, s string) (from, to int, ok bool) {
	if s == "" {
		return 0, 0, false
	}
	d.Descendants(func(n *Node, pos int, _ *Node, _ int) bool {
		if ok {
			return false
		}
		if i := strings.Index(n.text, s); n.typ.isText && i >= 0 {
			from = pos + utf16Len(n.text[:i])
			to, ok = from+utf16Len(s), true
		}
		return !ok
	})
	return from, to, ok
}

// Quote is the text between from and to as a list of comments shows it:
// blocks separated by a space, at most n characters with an ellipsis.
func Quote(d Doc, from, to, n int) string {
	if _, ok := InlineText(d, from, to); !ok {
		return ""
	}
	q := []rune(strings.Join(strings.Fields(d.TextBetween(from, to, " ", func(*Node) string { return "" })), " "))
	if len(q) > n {
		return string(q[:n-1]) + "…"
	}
	return string(q)
}

// MapRange maps a highlight's range through steps: its start moves with
// text inserted at it, its end does not, so typing at either edge stays
// outside. gone reports a range that holds no text any more.
func MapRange(next Doc, steps []Step, from, to int) (nf, nt int, gone bool) {
	nf, nt = from, to
	for _, s := range steps {
		m := s.StepMap()
		nf, nt = m.Map(nf, 1), m.Map(nt, -1)
	}
	if nf >= nt {
		return nf, nf, true
	}
	if nt-nf < to-from {
		if t, ok := InlineText(next, nf, nt); !ok || t == "" {
			return nf, nt, true
		}
	}
	return nf, nt, false
}

// LeafText says what leaf nodes other than text stand for in plain text,
// by type name: a mention its name, a hard break a newline, an image its
// alt text. Its Leaf
// method is the leafText argument of TextBetween.
type LeafText map[string]func(n *Node) string

// Leaf returns what n stands for; "" for a type without a function.
func (l LeafText) Leaf(n *Node) string {
	if f := l[n.typ.name]; f != nil {
		return f(n)
	}
	return ""
}

// Text returns the document's plain text, for search and feeds: blocks
// separated by sep, leaves as l says.
func (l LeafText) Text(d Doc, sep string) string {
	return d.TextBetween(0, d.content.size, sep, l.Leaf)
}
