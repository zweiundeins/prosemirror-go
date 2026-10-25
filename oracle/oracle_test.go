package oracle_test

import (
	"testing"

	"github.com/zweiundeins/prosemirror-go/oracle"
)

func TestOracle(t *testing.T) {
	o, err := oracle.New([]byte(`{"nodes":[{"name":"doc","content":"block+"},{"name":"paragraph","group":"block","content":"inline*"},{"name":"text","group":"inline"}],"marks":[{"name":"strong"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"hi"}]}]}`
	if msg, err := o.Check(doc); msg != "" || err != nil {
		t.Errorf("check %q %v", msg, err)
	}
	if msg, _ := o.Check(`{"type":"doc"}`); msg == "" {
		t.Error("an empty doc checked")
	}
	id, err := o.Load(doc)
	if err != nil {
		t.Fatal(err)
	}
	defer o.Free(id)
	if failed, err := o.Apply(id, `{"stepType":"addMark","from":1,"to":3,"mark":{"type":"strong"}}`); failed != "" || err != nil {
		t.Fatalf("apply %q %v", failed, err)
	}
	if got, _ := o.JSON(id); got != `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","marks":[{"type":"strong"}],"text":"hi"}]}]}` {
		t.Errorf("json %s", got)
	}
	if _, err := o.Apply(id, `{"stepType":"nope"}`); err == nil {
		t.Error("an unknown step type applied")
	}
}
