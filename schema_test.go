package prosemirror

import (
	"testing"
)

// A schema given as Go values compiles to the schema of the same JSON:
// documents parse, write and check alike.
func TestBuildSchema(t *testing.T) {
	none := ""
	s, err := BuildSchema(SchemaSpec{
		Nodes: []NodeSpec{
			{Name: "doc", Content: "block+"},
			{Name: "paragraph", Group: "block", Content: "inline*"},
			{Name: "figure", Group: "block", Atom: true, Attrs: []AttrSpec{
				{Name: "from", Required: true, Validate: "string"},
				{Name: "measure", Default: "km", Validate: "string"},
				{Name: "digits", Default: 1, Validate: "number"},
				{Name: "extra"},
			}},
			{Name: "text", Group: "inline"},
			{Name: "stop", Group: "inline", Inline: true, Atom: true, Marks: &none, Attrs: []AttrSpec{{Name: "photo", Required: true}}},
		},
		Marks: []MarkSpec{{Name: "strong"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	j := MustSchema([]byte(`{"nodes":[{"name":"doc","content":"block+"},{"name":"paragraph","group":"block","content":"inline*"},
		{"name":"figure","group":"block","atom":true,"attrs":{"from":{"validate":"string"},"measure":{"default":"km","validate":"string"},"digits":{"default":1,"validate":"number"},"extra":{"default":null}}},
		{"name":"text","group":"inline"},{"name":"stop","group":"inline","inline":true,"atom":true,"marks":"","attrs":{"photo":{}}}],
		"marks":[{"name":"strong"}],"limits":{"ignored":true}}`))
	for _, doc := range []string{
		`{"type":"doc","content":[{"type":"figure","attrs":{"from":"2026-07-14T07:30Z"}},{"type":"paragraph","content":[{"type":"stop","attrs":{"photo":"p1"}}]}]}`,
		`{"type":"doc","content":[{"type":"figure","attrs":{"from":"x","digits":2,"extra":[1]}}]}`,
	} {
		a, errA := s.ParseDoc([]byte(doc))
		b, errB := j.ParseDoc([]byte(doc))
		if errA != nil || errB != nil {
			t.Fatalf("%s: %v %v", doc, errA, errB)
		}
		ja, _ := MarshalDoc(a)
		jb, _ := MarshalDoc(b)
		if string(ja) != string(jb) {
			t.Errorf("Go %s\nJSON %s", ja, jb)
		}
	}
	for _, bad := range []string{
		`{"type":"doc","content":[{"type":"figure"}]}`,
		`{"type":"doc","content":[{"type":"figure","attrs":{"from":"x","digits":"2"}}]}`,
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"figure","attrs":{"from":"x"}}]}]}`,
	} {
		_, errA := s.ParseDoc([]byte(bad))
		_, errB := j.ParseDoc([]byte(bad))
		if errA == nil || errB == nil || errA.Error() != errB.Error() {
			t.Errorf("%s: %v / %v", bad, errA, errB)
		}
	}
	// Create takes Go values.
	n, err := s.Node("figure").Create(map[string]any{"from": "x", "digits": 3}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if d, ok := n.AttrInt("digits"); !ok || d != 3 {
		t.Errorf("digits %v", n.Attr("digits"))
	}
	if _, err := BuildSchema(SchemaSpec{Nodes: []NodeSpec{{Name: "doc", Content: "text*"}}}); err == nil {
		t.Error("a schema without a text type")
	}
}
