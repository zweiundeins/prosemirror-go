package prosemirror

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// Schema holds the node and mark types of a schema, compiled as
// prosemirror-model's Schema does.
type Schema struct {
	nodes    []*NodeType
	marks    []*MarkType
	nodeByID map[string]*NodeType
	markByID map[string]*MarkType
	top      *NodeType
	text     *NodeType
}

// NodeType is a node type of the schema.
type NodeType struct {
	name          string
	schema        *Schema
	groups        []string
	attrs         []*attribute
	defaultAttrs  []any // nil when an attribute has no default
	contentMatch  *ContentMatch
	inlineContent bool
	isBlock       bool
	isText        bool
	markSet       []*MarkType // nil with allMarks: every mark
	allMarks      bool
	atom          bool
	code          bool
	defining      bool
	isolating     bool
	tableRole     string
}

// MarkType is a mark type of the schema.
type MarkType struct {
	name      string
	rank      int
	schema    *Schema
	attrs     []*attribute
	instance  *Mark // the mark without attributes, when all have defaults
	excluded  []*MarkType
	inclusive bool
	spanning  bool
	groups    []string
}

type attribute struct {
	name       string
	hasDefault bool
	def        any
	validate   []string // the allowed typeof names; nil: anything
}

// SchemaSpec is a schema as Go values: prosemirror-model's SchemaSpec with
// the nodes and marks in order, as the order of a content expression's
// groups and the nesting of marks depend on it. Only what the Go side uses
// is here; toDOM, parseDOM, draggable and the like are the browser's.
type SchemaSpec struct {
	TopNode string // "doc" when empty
	Nodes   []NodeSpec
	Marks   []MarkSpec
}

// NodeSpec is a node type's spec.
type NodeSpec struct {
	Name    string
	Content string // a content expression; empty for a leaf
	// Marks names the marks allowed inside: nil for the default (all in
	// inline content, none elsewhere), "" for none, "_" for all.
	Marks     *string
	Group     string // groups separated by spaces
	Inline    bool
	Atom      bool
	Code      bool
	Defining  bool
	Isolating bool
	TableRole string
	Attrs     []AttrSpec
}

// MarkSpec is a mark type's spec.
type MarkSpec struct {
	Name      string
	Inclusive *bool   // nil: true
	Excludes  *string // nil: the mark itself; "" none; "_" all
	Group     string
	Spanning  *bool // nil: true
	Attrs     []AttrSpec
}

// AttrSpec is an attribute's spec. The zero value has the default null.
type AttrSpec struct {
	Name string
	// Default is the value of an attribute a node or mark is made without;
	// Required means it has none and must be given.
	Default  any
	Required bool
	// Validate names the allowed types as prosemirror-model's validate
	// string does: "string", "number|null".
	Validate string
}

// NewSchema compiles a schema written as JSON: an object with "topNode",
// and "nodes" and "marks" as arrays of specs, each with its "name" and the
// fields of prosemirror-model's NodeSpec and MarkSpec. Attributes keep the
// order they are written in, which is the order of their JSON. Other keys
// are ignored, so the file the browser builds its schema from can hold more.
func NewSchema(data []byte) (*Schema, error) {
	var raw struct {
		TopNode string            `json:"topNode"`
		Nodes   []json.RawMessage `json:"nodes"`
		Marks   []json.RawMessage `json:"marks"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("prosemirror: schema: %w", err)
	}
	spec := SchemaSpec{TopNode: raw.TopNode}
	for _, r := range raw.Nodes {
		var n struct {
			Name      string                     `json:"name"`
			Content   string                     `json:"content"`
			Marks     *string                    `json:"marks"`
			Group     string                     `json:"group"`
			Inline    bool                       `json:"inline"`
			Atom      bool                       `json:"atom"`
			Code      bool                       `json:"code"`
			Defining  bool                       `json:"defining"`
			Isolating bool                       `json:"isolating"`
			TableRole string                     `json:"tableRole"`
			Attrs     map[string]json.RawMessage `json:"attrs"`
		}
		if err := json.Unmarshal(r, &n); err != nil {
			return nil, fmt.Errorf("prosemirror: schema: %w", err)
		}
		attrs, err := attrSpecs(n.Name, r, n.Attrs)
		if err != nil {
			return nil, err
		}
		spec.Nodes = append(spec.Nodes, NodeSpec{Name: n.Name, Content: n.Content, Marks: n.Marks, Group: n.Group, Inline: n.Inline, Atom: n.Atom,
			Code: n.Code, Defining: n.Defining, Isolating: n.Isolating, TableRole: n.TableRole, Attrs: attrs})
	}
	for _, r := range raw.Marks {
		var m struct {
			Name      string                     `json:"name"`
			Inclusive *bool                      `json:"inclusive"`
			Excludes  *string                    `json:"excludes"`
			Group     string                     `json:"group"`
			Spanning  *bool                      `json:"spanning"`
			Attrs     map[string]json.RawMessage `json:"attrs"`
		}
		if err := json.Unmarshal(r, &m); err != nil {
			return nil, fmt.Errorf("prosemirror: schema: %w", err)
		}
		attrs, err := attrSpecs(m.Name, r, m.Attrs)
		if err != nil {
			return nil, err
		}
		spec.Marks = append(spec.Marks, MarkSpec{Name: m.Name, Inclusive: m.Inclusive, Excludes: m.Excludes, Group: m.Group, Spanning: m.Spanning, Attrs: attrs})
	}
	return BuildSchema(spec)
}

// attrSpecs reads the attribute specs of a node or mark spec in the order
// they are written.
func attrSpecs(typeName string, raw json.RawMessage, specs map[string]json.RawMessage) ([]AttrSpec, error) {
	order, err := attrOrder(raw)
	if err != nil {
		return nil, err
	}
	var out []AttrSpec
	for _, name := range order {
		var spec map[string]json.RawMessage
		if err := json.Unmarshal(specs[name], &spec); err != nil {
			return nil, fmt.Errorf("prosemirror: schema: attribute %s of %s: %w", name, typeName, err)
		}
		a := AttrSpec{Name: name, Required: true}
		if raw, ok := spec["default"]; ok {
			v, err := ParseValue(raw)
			if err != nil {
				return nil, err
			}
			a.Default, a.Required = v, false
		}
		if raw, ok := spec["validate"]; ok {
			if err := json.Unmarshal(raw, &a.Validate); err != nil {
				return nil, fmt.Errorf("prosemirror: schema: validate of %s.%s must be a string", typeName, name)
			}
		}
		out = append(out, a)
	}
	return out, nil
}

// MustSchema is NewSchema for a schema known to be valid, as an embedded
// file is: it panics on an error.
func MustSchema(data []byte) *Schema {
	s, err := NewSchema(data)
	if err != nil {
		panic(err)
	}
	return s
}

// BuildSchema compiles a schema given as Go values.
func BuildSchema(spec SchemaSpec) (*Schema, error) {
	s := &Schema{nodeByID: map[string]*NodeType{}, markByID: map[string]*MarkType{}}
	for _, ns := range spec.Nodes {
		attrs, err := compileAttrs(ns.Attrs)
		if err != nil {
			return nil, err
		}
		if _, dup := s.nodeByID[ns.Name]; dup {
			return nil, fmt.Errorf("prosemirror: schema: node type %s twice", ns.Name)
		}
		t := &NodeType{
			name: ns.Name, schema: s, attrs: attrs,
			isBlock: !(ns.Inline || ns.Name == "text"), isText: ns.Name == "text",
			atom: ns.Atom, code: ns.Code, defining: ns.Defining, isolating: ns.Isolating, tableRole: ns.TableRole,
		}
		if ns.Group != "" {
			t.groups = strings.Split(ns.Group, " ")
		}
		t.defaultAttrs = defaultAttrs(attrs)
		s.nodes = append(s.nodes, t)
		s.nodeByID[t.name] = t
	}
	for i, ms := range spec.Marks {
		attrs, err := compileAttrs(ms.Attrs)
		if err != nil {
			return nil, err
		}
		t := &MarkType{name: ms.Name, rank: i, schema: s, attrs: attrs, inclusive: ms.Inclusive == nil || *ms.Inclusive, spanning: ms.Spanning == nil || *ms.Spanning}
		if ms.Group != "" {
			t.groups = strings.Split(ms.Group, " ")
		}
		if d := defaultAttrs(attrs); d != nil {
			t.instance = &Mark{typ: t, attrs: d}
		}
		s.marks = append(s.marks, t)
		s.markByID[t.name] = t
	}
	top := spec.TopNode
	if top == "" {
		top = "doc"
	}
	if s.top = s.nodeByID[top]; s.top == nil {
		return nil, fmt.Errorf("prosemirror: schema is missing its top node type (%q)", top)
	}
	if s.text = s.nodeByID["text"]; s.text == nil {
		return nil, fmt.Errorf("prosemirror: every schema needs a text type")
	}
	if len(s.text.attrs) > 0 {
		return nil, fmt.Errorf("prosemirror: the text node type should not have attributes")
	}
	cache := map[string]*ContentMatch{}
	for i, t := range s.nodes {
		if _, both := s.markByID[t.name]; both {
			return nil, fmt.Errorf("prosemirror: %s can not be both a node and a mark", t.name)
		}
		expr := spec.Nodes[i].Content
		m, ok := cache[expr]
		if !ok {
			var err error
			if m, err = parseContentMatch(expr, s); err != nil {
				return nil, err
			}
			cache[expr] = m
		}
		t.contentMatch = m
		t.inlineContent = m.inlineContent()
		switch me := spec.Nodes[i].Marks; {
		case me != nil && *me == "_":
			t.allMarks = true
		case me != nil && *me != "":
			set, err := s.gatherMarks(strings.Split(*me, " "))
			if err != nil {
				return nil, err
			}
			t.markSet = set
		case me != nil || !t.inlineContent:
			t.markSet = []*MarkType{}
		default:
			t.allMarks = true
		}
	}
	for i, t := range s.marks {
		switch ex := spec.Marks[i].Excludes; {
		case ex == nil:
			t.excluded = []*MarkType{t}
		case *ex == "":
			t.excluded = nil
		default:
			set, err := s.gatherMarks(strings.Split(*ex, " "))
			if err != nil {
				return nil, err
			}
			t.excluded = set
		}
	}
	return s, nil
}

// attrOrder returns the attribute names of a node or mark spec in the order
// they are written, which is the order of the attributes in JSON.
func attrOrder(raw json.RawMessage) ([]string, error) {
	p := &parser{data: raw}
	var order []string
	err := p.object(func(key string) error {
		if key != "attrs" {
			_, err := p.value()
			return err
		}
		if p.peek() != '{' {
			_, err := p.value()
			return err
		}
		order = order[:0]
		return p.object(func(name string) error {
			order = append(order, name)
			_, err := p.value()
			return err
		})
	})
	return order, err
}

func compileAttrs(specs []AttrSpec) ([]*attribute, error) {
	var out []*attribute
	for _, s := range specs {
		a := &attribute{name: s.Name, hasDefault: !s.Required}
		if a.hasDefault {
			v, err := jsValue(s.Default)
			if err != nil {
				return nil, fmt.Errorf("prosemirror: schema: default of %s: %w", s.Name, err)
			}
			a.def = v
		}
		if s.Validate != "" {
			a.validate = strings.Split(s.Validate, "|")
		}
		out = append(out, a)
	}
	return out, nil
}

// jsValue turns a Go value into an attribute value as JavaScript holds it
// (nil, bool, float64, string, []any, *Object).
func jsValue(v any) (any, error) {
	switch x := v.(type) {
	case nil, bool, float64, string, *Object:
		return x, nil
	case int:
		return float64(x), nil
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			var err error
			if out[i], err = jsValue(e); err != nil {
				return nil, err
			}
		}
		return out, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return ParseValue(b)
}

func defaultAttrs(attrs []*attribute) []any {
	d := make([]any, len(attrs))
	for i, a := range attrs {
		if !a.hasDefault {
			return nil
		}
		d[i] = a.def
	}
	return d
}

func (s *Schema) gatherMarks(names []string) ([]*MarkType, error) {
	var found []*MarkType
	for _, name := range names {
		if m := s.markByID[name]; m != nil {
			found = append(found, m)
			continue
		}
		ok := false
		for _, m := range s.marks {
			if name == "_" || (m.groupHas(name)) {
				found = append(found, m)
				ok = true
			}
		}
		if !ok {
			return nil, fmt.Errorf("prosemirror: unknown mark type: '%s'", name)
		}
	}
	return found, nil
}

func (m *MarkType) groupHas(name string) bool { return slices.Contains(m.groups, name) }

// Node returns the node type called name, or nil.
func (s *Schema) Node(name string) *NodeType { return s.nodeByID[name] }

// Mark returns the mark type called name, or nil.
func (s *Schema) Mark(name string) *MarkType { return s.markByID[name] }

// Nodes returns the node types in schema order.
func (s *Schema) Nodes() []*NodeType { return slices.Clone(s.nodes) }

// Marks returns the mark types in rank order.
func (s *Schema) Marks() []*MarkType { return slices.Clone(s.marks) }

// Top returns the type of the document node.
func (s *Schema) Top() *NodeType { return s.top }

// Name returns the type's name.
func (t *NodeType) Name() string { return t.name }

// IsBlock reports whether nodes of the type are blocks.
func (t *NodeType) IsBlock() bool { return t.isBlock }

// IsInline reports whether nodes of the type are inline.
func (t *NodeType) IsInline() bool { return !t.isBlock }

// IsText reports whether this is the text type.
func (t *NodeType) IsText() bool { return t.isText }

// IsTextblock reports whether the type is a block with inline content.
func (t *NodeType) IsTextblock() bool { return t.isBlock && t.inlineContent }

// IsLeaf reports whether the type allows no content.
func (t *NodeType) IsLeaf() bool { return t.contentMatch == emptyMatch }

// IsAtom reports whether the type is a leaf or declared an atom.
func (t *NodeType) IsAtom() bool { return t.IsLeaf() || t.atom }

// InlineContent reports whether the type holds inline content.
func (t *NodeType) InlineContent() bool { return t.inlineContent }

// ContentMatch returns the start state of the type's content expression.
func (t *NodeType) ContentMatch() *ContentMatch { return t.contentMatch }

func (t *NodeType) isInGroup(g string) bool { return slices.Contains(t.groups, g) }

func (t *NodeType) hasRequiredAttrs() bool {
	for _, a := range t.attrs {
		if !a.hasDefault {
			return true
		}
	}
	return false
}

func (t *NodeType) attrIndex(name string) int {
	for i, a := range t.attrs {
		if a.name == name {
			return i
		}
	}
	return -1
}

// compatibleContent reports whether the type allows some of the content of
// other.
func (t *NodeType) compatibleContent(other *NodeType) bool {
	return t == other || t.contentMatch.compatible(other.contentMatch)
}

func (t *NodeType) allowsMarkType(m *MarkType) bool {
	return t.allMarks || slices.Contains(t.markSet, m)
}

func (t *NodeType) allowsMarks(marks []*Mark) bool {
	if t.allMarks {
		return true
	}
	for _, m := range marks {
		if !t.allowsMarkType(m.typ) {
			return false
		}
	}
	return true
}

// validContent reports whether the fragment is valid content of the type.
func (t *NodeType) validContent(f Fragment) bool {
	m := t.contentMatch.matchFragment(f, 0, len(f.nodes))
	if m == nil || !m.validEnd {
		return false
	}
	for _, c := range f.nodes {
		if !t.allowsMarks(c.marks) {
			return false
		}
	}
	return true
}

func (t *NodeType) checkContent(f Fragment) error {
	if !t.validContent(f) {
		return &ModelError{Msg: "Invalid content for node " + t.name + ": " + truncateUTF16(f.String(), 50)}
	}
	return nil
}

// Name returns the mark type's name.
func (m *MarkType) Name() string { return m.name }

// Rank is the mark's place in the nesting order.
func (m *MarkType) Rank() int { return m.rank }

func (m *MarkType) excludes(other *MarkType) bool { return slices.Contains(m.excluded, other) }

// ModelError is an error prosemirror-model throws as a RangeError or
// SyntaxError; its message is prosemirror-model's.
type ModelError struct{ Msg string }

func (e *ModelError) Error() string { return e.Msg }

func modelErrorf(format string, args ...any) error {
	return &ModelError{Msg: fmt.Sprintf(format, args...)}
}

// computeAttrs builds the attributes of a node or mark from a JSON value as
// prosemirror-model's computeAttrs does, JavaScript's truthiness included.
func computeAttrs(typeName string, attrs []*attribute, defaults []any, value any) ([]any, error) {
	if !truthy(value) && defaults != nil {
		return defaults, nil
	}
	built := make([]any, len(attrs))
	obj, _ := value.(*Object)
	for i, a := range attrs {
		var given any
		switch {
		case !truthy(value):
			given = value // `value && value[name]` is value itself
		case obj != nil:
			given = obj.Get(a.name)
		default:
			given = Undefined
		}
		if given == Undefined {
			if !a.hasDefault {
				return nil, modelErrorf("No value supplied for attribute %s", a.name)
			}
			given = a.def
		} else if err := a.check(typeName, given); err != nil {
			return nil, err
		}
		built[i] = given
	}
	return built, nil
}

func (a *attribute) check(typeName string, v any) error {
	if a.validate == nil {
		return nil
	}
	name := typeOf(v)
	if !slices.Contains(a.validate, name) {
		return modelErrorf("Expected value of type %s for attribute %s on type %s, got %s", strings.Join(a.validate, ","), a.name, typeName, name)
	}
	return nil
}
