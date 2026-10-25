package prosemirror

import (
	"testing"
)

// Each node and mark renders through its function; a node type without
// one renders its content, a mark type without one a span.
func TestHTMLHooks(t *testing.T) {
	cases := []struct{ doc, html string }{
		{docOf(`{"type":"heading","attrs":{"level":2},"content":[` + text("A <b>") + `]}`), `<h2>A &lt;b&gt;</h2>`},
		{docOf(`{"type":"code_block","attrs":{"language":"go\""},"content":[` + text("x := 1") + `]}`), `<pre data-language="go&#34;"><code>x := 1</code></pre>`},
		{inPara(text("a", strong, em) + `,` + text("b", strong) + `,` + text("c", `{"type":"strike"}`)), `<p><strong><em>a</em>b</strong><span>c</span></p>`},
		// A text-only mark leaves the inline node out.
		{inPara(text("go ", link("https://x.y/?a=1&b")) + `,{"type":"mention","attrs":{"id":"ada"},"marks":[` + link("https://x.y/") + `]},` + text(" on", link("https://x.y/"))),
			`<p><a href="https://x.y/?a=1&amp;b">go </a><a class="mention">@Ada &#34;Countess&#34;</a><a href="https://x.y/"> on</a></p>`},
		// The resolver is the render's; the node's label when it knows nothing.
		{inPara(`{"type":"mention","attrs":{"id":"bob","label":"Bob"}}`), `<p><a class="mention">@Bob</a></p>`},
		// Types without a function: a task list renders its content, an
		// inline leaf nothing.
		{docOf(`{"type":"task_list","content":[{"type":"task_item","content":[` + para + `]}]}`), `<p>hi</p>`},
		{inPara(text("a") + `,{"type":"xref","attrs":{"target":"x"}}`), `<p>a</p>`},
		{docOf(`{"type":"image","attrs":{"imageId":"a b","alt":"\"x\""}}`), `<img src="/img/a%20b" alt="&#34;x&#34;">`},
	}
	for _, c := range cases {
		if got := testHTML.Render(mustDoc(t, c.doc), testResolver); got != c.html {
			t.Errorf("%s:\n got %s\nwant %s", c.doc, got, c.html)
		}
	}
	d := mustDoc(t, docOf(para))
	if got := testHTML.RenderBlock(d.Child(0), testResolver); got != `<p>hi</p>` {
		t.Errorf("block %s", got)
	}
}

// The block cache keeps unchanged blocks and renders Dynamic ones, and the
// blocks holding them, every time.
func TestBlockCacheDynamic(t *testing.T) {
	d := mustDoc(t, docOf(para, `{"type":"blockquote","content":[{"type":"embed","attrs":{"src":"/v/1"}}]}`))
	bc := NewBlockCache(testHTML)
	r := testRes{Embeds: map[string]string{"/v/1": "one"}}
	if got := bc.Render(d, r); got != `<p>hi</p><blockquote><div class="embed" data-src="/v/1">one</div></blockquote>` {
		t.Fatalf("first %s", got)
	}
	r.Embeds = map[string]string{"/v/1": "two"}
	r.People = map[string]string{"x": "y"}
	if got := bc.Render(d, r); got != `<p>hi</p><blockquote><div class="embed" data-src="/v/1">two</div></blockquote>` {
		t.Fatalf("second %s", got)
	}
	if bc.m[d.Child(0)].live || !bc.m[d.Child(1)].live {
		t.Error("live flags")
	}
}
