package prosemirror

import (
	"fmt"
	"strconv"
	"strings"
)

// Directive writes a node type as a generic directive of the CommonMark
// directive proposal, the syntax remark-directive reads, and reads it
// back (ParseDirective, DirectiveNode):
//
//	:name[label]{a="1" b="x"}      an inline node
//	::name[label]{a="1"}           a block without content, or a textblock
//	                               whose inline content is the label
//	:::name{a="1"}                 a block with blocks inside, closed by a
//	content                        line of as many colons
//	:::
//
// It suits node types Markdown has no syntax for: their attributes
// survive an export and an import, and a reader of the file still sees
// the label.
type Directive struct {
	// Name is the directive's name; empty is the node type's name.
	Name string
	// Label is the attribute written as the [label] of an inline node or
	// a block without content; empty, or an empty value, writes none.
	Label string
	// Attrs are the attributes written in braces, in order; an attribute
	// equal to its default is left out. A value that is not a string is
	// written as JSON, unless Formats has its own form.
	Attrs []string
	// Formats write and read attributes in a form of their own, such as
	// a list of ids separated by spaces (SpaceList).
	Formats map[string]AttrFormat
}

// AttrFormat writes an attribute's value as the text of a directive and
// reads it back.
type AttrFormat struct {
	Format func(v any) string
	Parse  func(s string) (any, error)
}

// SpaceList writes an array of strings as the strings separated by
// spaces, `photos="p1 p2 p3"`, and reads it back.
var SpaceList = AttrFormat{
	Format: func(v any) string {
		list, _ := v.([]any)
		parts := make([]string, 0, len(list))
		for _, x := range list {
			if s, ok := x.(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " ")
	},
	Parse: func(s string) (any, error) {
		out := []any{}
		for _, f := range strings.Fields(s) {
			out = append(out, f)
		}
		return out, nil
	},
}

func (d Directive) name(n *Node) string {
	if d.Name != "" {
		return d.Name
	}
	return n.typ.name
}

// Directive writes n as the directive d.
func (w *MarkdownWriter[R]) Directive(n *Node, d Directive) {
	attrs := directiveAttrs(n, d)
	label := ""
	if l := n.AttrString(d.Label); d.Label != "" && l != "" {
		label = "[" + MarkdownEscape(l) + "]"
	}
	switch {
	case n.typ.IsInline():
		w.Write(":" + d.name(n) + label + attrs)
	case n.typ.inlineContent:
		w.Line("::" + d.name(n) + "[" + strings.ReplaceAll(w.Inline(n), "\n", " ") + "]" + attrs)
	case n.content.size == 0 || n.typ.IsAtom():
		w.Line("::" + d.name(n) + label + attrs)
	default:
		fence := strings.Repeat(":", 3+w.md.containerDepth(n))
		w.Line(fence + d.name(n) + attrs)
		w.Blocks(n.content.nodes, "", "")
		w.Line(fence)
	}
}

// containerDepth is how deep directives with blocks inside nest in n.
func (m *Markdown[R]) containerDepth(n *Node) int {
	depth := 0
	for _, c := range n.content.nodes {
		d := m.containerDepth(c)
		if _, ok := m.Directives[c.typ.name]; ok && m.Nodes[c.typ.name] == nil && c.typ.isBlock && !c.typ.inlineContent && !c.typ.IsAtom() && c.content.size > 0 {
			d++
		}
		depth = max(depth, d)
	}
	return depth
}

func directiveAttrs(n *Node, d Directive) string {
	var b strings.Builder
	for _, name := range d.Attrs {
		i := n.typ.attrIndex(name)
		if i < 0 {
			continue
		}
		v := n.attrs[i]
		if a := n.typ.attrs[i]; a.hasDefault && valuesEqual(v, a.def) {
			continue
		}
		var s string
		switch x := v.(type) {
		case string:
			s = x
		default:
			s = string(appendValue(nil, v))
		}
		if f := d.Formats[name]; f.Format != nil {
			s = f.Format(v)
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(name + `="` + strings.NewReplacer("&", "&amp;", `"`, "&quot;").Replace(s) + `"`)
	}
	if b.Len() == 0 {
		return ""
	}
	return "{" + b.String() + "}"
}

func valuesEqual(a, b any) bool {
	return string(appendValue(nil, a)) == string(appendValue(nil, b))
}

// ParsedDirective is a directive as ParseDirective reads it from text.
type ParsedDirective struct {
	Colons int    // 1 inline, 2 a leaf block, 3 or more a container's fence
	Name   string // the directive's name
	Label  string // the label's Markdown, without the brackets
	// HasLabel reports a label, even an empty one.
	HasLabel bool
	Attrs    [][2]string // the attributes as written: key and value
}

// ParseDirective reads a directive at the start of s: colons, a name, an
// optional [label] and optional {attributes} (key="value", key='value',
// key=value, a bare key, #id as id and .class as class). It returns how
// many bytes it read. A Markdown importer calls it where a line or an
// inline run starts with a colon, and DirectiveNode for the node.
func ParseDirective(s string) (d ParsedDirective, n int, ok bool) {
	i := 0
	for i < len(s) && s[i] == ':' {
		i++
	}
	d.Colons = i
	start := i
	for i < len(s) && (isAlpha(s[i]) || (i > start && (isDigit(s[i]) || s[i] == '-' || s[i] == '_'))) {
		i++
	}
	if d.Colons == 0 || i == start {
		return d, 0, false
	}
	d.Name = s[start:i]
	if i < len(s) && s[i] == '[' {
		depth, j := 0, i
	label:
		for ; j < len(s); j++ {
			switch s[j] {
			case '\\':
				j++
			case '[':
				depth++
			case ']':
				if depth--; depth == 0 {
					break label
				}
			}
		}
		if j >= len(s) {
			return d, 0, false
		}
		d.Label, d.HasLabel = s[i+1:j], true
		i = j + 1
	}
	if i < len(s) && s[i] == '{' {
		attrs, end, ok := parseDirectiveAttrs(s, i+1)
		if !ok {
			return d, 0, false
		}
		d.Attrs, i = attrs, end
	}
	return d, i, true
}

func isAlpha(c byte) bool { return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' }
func isDigit(c byte) bool { return '0' <= c && c <= '9' }

func parseDirectiveAttrs(s string, i int) ([][2]string, int, bool) {
	var out [][2]string
	unref := strings.NewReplacer("&quot;", `"`, "&#34;", `"`, "&#39;", "'", "&lt;", "<", "&gt;", ">", "&amp;", "&")
	for {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n') {
			i++
		}
		if i >= len(s) {
			return nil, 0, false
		}
		if s[i] == '}' {
			return out, i + 1, true
		}
		var key string
		switch s[i] {
		case '#', '.':
			key = map[byte]string{'#': "id", '.': "class"}[s[i]]
			i++
		}
		start := i
		for i < len(s) && !strings.ContainsRune(" \t\n=}\"'", rune(s[i])) {
			i++
		}
		if key != "" {
			out = append(out, [2]string{key, s[start:i]})
			continue
		}
		key = s[start:i]
		if key == "" {
			return nil, 0, false
		}
		if i >= len(s) || s[i] != '=' {
			out = append(out, [2]string{key, ""})
			continue
		}
		i++
		var val string
		if i < len(s) && (s[i] == '"' || s[i] == '\'') {
			q := s[i]
			end := strings.IndexByte(s[i+1:], q)
			if end < 0 {
				return nil, 0, false
			}
			val = s[i+1 : i+1+end]
			i += end + 2
		} else {
			start := i
			for i < len(s) && !strings.ContainsRune(" \t\n}", rune(s[i])) {
				i++
			}
			val = s[start:i]
		}
		out = append(out, [2]string{key, unref.Replace(val)})
	}
}

// DirectiveNode makes the node of a parsed directive: the node type whose
// Directive has its name, with the attributes it names, each read as the
// attribute's type (by its validate string, else by its default). An
// inline node's or a leaf block's label sets the Label attribute;
// content is the inline content of a textblock (the label as the
// importer parsed it) or the blocks inside a container. It refuses a
// directive no node type has, and attributes the schema refuses.
func (m *Markdown[R]) DirectiveNode(s *Schema, d ParsedDirective, content []*Node) (*Node, error) {
	for typeName, spec := range m.Directives {
		t := s.Node(typeName)
		if t == nil || (spec.Name != d.Name && !(spec.Name == "" && typeName == d.Name)) {
			continue
		}
		attrs := map[string]any{}
		for _, kv := range d.Attrs {
			i := t.attrIndex(kv[0])
			if i < 0 || !contains(spec.Attrs, kv[0]) {
				continue
			}
			parse := t.attrs[i].fromText
			if f := spec.Formats[kv[0]]; f.Parse != nil {
				parse = f.Parse
			}
			v, err := parse(kv[1])
			if err != nil {
				return nil, fmt.Errorf("prosemirror: directive %s: attribute %s: %w", d.Name, kv[0], err)
			}
			attrs[kv[0]] = v
		}
		if spec.Label != "" && d.HasLabel && !t.inlineContent {
			attrs[spec.Label] = MarkdownUnescape(d.Label)
		}
		n, err := t.Create(attrs, content)
		if err != nil {
			return nil, err
		}
		return n, n.Check()
	}
	return nil, fmt.Errorf("prosemirror: no node type for the directive %s", d.Name)
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// fromText reads an attribute's value written as text in a directive.
func (a *attribute) fromText(s string) (any, error) {
	kind := ""
	if len(a.validate) > 0 {
		for _, k := range a.validate {
			if k != "null" && k != "undefined" {
				kind = k
				break
			}
		}
	} else if a.hasDefault {
		kind = typeOf(a.def)
	}
	switch kind {
	case "number":
		f, err := strconv.ParseFloat(s, 64)
		return f, err
	case "boolean":
		return strconv.ParseBool(s)
	case "string", "":
		return s, nil
	}
	return ParseValue([]byte(s))
}

// MarkdownUnescape takes the backslashes out of escaped punctuation, as
// MarkdownEscape writes it.
func MarkdownUnescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && strings.IndexByte("\\`*_[]<>#|~!\"$%&'()+,-./:;=?@^{}", s[i+1]) >= 0 {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
