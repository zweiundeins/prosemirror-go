package prosemirror

import (
	"strings"
	"testing"
)

// FR-88: added text is marked as added, removed text as removed.
func TestDiff(t *testing.T) {
	a := mustDoc(t, docOf(
		`{"type":"heading","attrs":{"level":1},"content":[`+text("Release plan")+`]}`,
		`{"type":"paragraph","content":[`+text("We ship the editor on Monday.")+`]}`,
		`{"type":"bullet_list","content":[{"type":"list_item","content":[{"type":"paragraph","content":[`+text("one")+`]}]},{"type":"list_item","content":[{"type":"paragraph","content":[`+text("two")+`]}]}]}`,
		`{"type":"horizontal_rule"}`,
	))
	b := mustDoc(t, docOf(
		`{"type":"heading","attrs":{"level":1},"content":[`+text("Release plan")+`]}`,
		`{"type":"paragraph","content":[`+text("We ship the ")+`,`+text("editor", strong)+`,`+text(" on Tuesday.")+`]}`,
		`{"type":"bullet_list","content":[{"type":"list_item","content":[{"type":"paragraph","content":[`+text("one")+`]}]},{"type":"list_item","content":[{"type":"paragraph","content":[`+text("2")+`]}]},{"type":"list_item","content":[{"type":"paragraph","content":[`+text("three")+`]}]}]}`,
		`{"type":"paragraph","content":[`+text("<new>")+`]}`,
	))
	d := Diff(a, b)
	want := `<h1>Release plan</h1>` +
		`<p>We ship the <del class="diff-del">editor</del><ins class="diff-ins"><strong>editor</strong></ins> on <del class="diff-del">Monday</del><ins class="diff-ins">Tuesday</ins>.</p>` +
		`<ul><li><p>one</p></li><li><p><del class="diff-del">two</del><ins class="diff-ins">2</ins></p></li><li data-diff="ins"><p><ins class="diff-ins">three</ins></p></li></ul>` +
		`<del class="diff-del"><hr data-diff="del"></del><p data-diff="ins"><ins class="diff-ins">&lt;new&gt;</ins></p>`
	if got := testHTML.Diff(d, testResolver); got != want {
		t.Errorf("diff:\n got %s\nwant %s", got, want)
	}
	if d.Equal() || d.Inserted == 0 || d.Deleted == 0 {
		t.Errorf("counts %d %d", d.Inserted, d.Deleted)
	}
	if !Diff(a, a).Equal() {
		t.Error("a document differs from itself")
	}
	// Large documents compare in reasonable time and render.
	f := mustDoc(t, string(readFixture(t, "f200k-doc.json")))
	g := mustDoc(t, string(readFixture(t, "f200k-final.json")))
	if h := testHTML.Diff(Diff(f, g), testResolver); !strings.Contains(h, "diff-ins") {
		t.Error("no insertions in the fixture diff")
	}
}
