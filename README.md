# prosemirror-go

A Go port of [ProseMirror](https://prosemirror.net)'s document model and steps, for servers that keep ProseMirror documents: it reads and writes their JSON byte for byte as the browser does, applies and rebases the steps editors send, checks documents against the schema and the app's rules, and renders them as HTML, Markdown and plain text. Standard library only; the tests run the real prosemirror-model and prosemirror-transform in [goja](https://github.com/dop251/goja) and must agree with them.

    go get github.com/zweiundeins/prosemirror-go

The package knows no node type. The app brings its schema, and per node and mark type its renderers and rules.

## What it ports

From prosemirror-model 1.25.12: schemas with content expressions, attributes and their validation, marks, `Fragment`, `Node`, `Slice`, `ResolvedPos`, the replace algorithm, `check`, `toJSON` and `fromJSON`, `findDiffStart` and `findDiffEnd`. From prosemirror-transform 1.12.2: the steps `replace`, `replaceAround`, `addMark`, `removeMark` and `attr`, `StepMap` and `Mapping` with mirrors. `Rebase` is `applyCommit` of [prosemirror-collab-commit](https://github.com/stepwisehq/prosemirror-collab-commit): a server that orders commits maps a stale one over the commits since its base and applies what still fits, as the browser does. The Transform helpers (lift, wrap, split) and the DOM are not ported: the browser makes steps, the server applies them.

Positions count UTF-16 units as in the browser; JSON keeps JavaScript's key order, number format and string escapes. The package documentation lists the details and what it refuses.

## One package

Everything is in package `prosemirror`. Steps reach into the model's internals on every apply (a fragment's nodes, a node's cached sizes), as prosemirror-transform reaches into objects JavaScript does not hide; split into `model` and `transform`, the model would have to export those internals or the transform would pay for accessors on the hot path. The renderers walk the same internals. An app imports one name.

## The app's node types

| Hook | What the app gives |
| --- | --- |
| `NewSchema(json)`, `BuildSchema(SchemaSpec)` | the schema: the JSON the browser builds its `Schema` from (nodes and marks as ordered arrays, other keys ignored), or the same as Go values |
| `HTML[R]` | per node type a function that writes its element (`Open`, `Content`, `Text`, `Raw`), per mark type one that opens it; `R` is the app's resolver, handed to each render |
| `Markdown[R]` | per node and mark type a function, and `Directives` for node types Markdown has no syntax for |
| `LeafText` | per leaf type what it stands for in plain text, for search and feeds |
| `Rules` | per node and mark type a check, plus size bounds; built per commit, so a rule can ask the app's data |
| `IDs` | the node types with a stable id and its attribute, `{"route": "id"}`: `Rules.IDs` refuses a missing or a repeated id, `IDs.Find` and `FindNode` find the node, `HTML.IDs` lets a highlight mark the node's element |

```go
type Names map[string]string // the app's resolver: id to name

html := &prosemirror.HTML[Names]{Nodes: map[string]func(w *prosemirror.HTMLWriter[Names], n *prosemirror.Node){
	"paragraph": func(w *prosemirror.HTMLWriter[Names], n *prosemirror.Node) { w.Open("p"); w.Content(n); w.Raw("</p>") },
	"mention": func(w *prosemirror.HTMLWriter[Names], n *prosemirror.Node) {
		w.Raw(`<a class="mention">@`); w.Text(w.R[n.AttrString("id")]); w.Raw(`</a>`)
	},
}}
rules := prosemirror.Rules{MaxBytes: 200 << 10, Nodes: map[string]func(n, parent *prosemirror.Node) error{
	"mention": func(n, _ *prosemirror.Node) error { return knownPerson(n.AttrString("id")) },
}}
doc, _ := schema.ParseDoc(raw)
next, applied, err := prosemirror.Rebase(head, stepsSinceBase, steps) // then rules.Check(head, next)
fmt.Println(html.Render(next, Names{"u1": "Ada"}))
```

`example_test.go` is this example in full, run by `go test`.

**Render-time values.** A document holds ids; what they show (a person's name now, an issue's status, a number computed from other data) comes from the resolver `R` the app passes to `Render`, `RenderHighlighted`, `Diff` and `Markdown.Render`. `BlockCache` keeps the HTML of unchanged top-level blocks; `HTML.Dynamic` names node types whose HTML changes without the node (rows of a query) so that their blocks render every time, and `Reset` drops the rest when the app's data changes.

**Rules with the app's context.** `Rules` is a value of plain functions. An app builds it for each commit, with closures over what the commit may see (its database transaction, the actor), so a rule can refuse an id that is not the app's or that the person may not name. `Check` runs the schema check, the size bounds and the rules on the top-level blocks the commit changed only. A refusal is a `*RuleError` whose message a person reads.

**Directives.** A node type Markdown has no syntax for writes as a generic directive of the [CommonMark directive proposal](https://talk.commonmark.org/t/generic-directives-plugins-syntax/444), the syntax [remark-directive](https://github.com/remarkjs/remark-directive) reads: `:stop[Grimselpass]{photo="p1"}` inline, `::route{from="2026-07-14T07:30Z" to="2026-07-14T15:10Z"}` for a block, `:::note{kind="tip"}` around blocks. `Directive` maps a node type to its name, the attribute that is its label and the attributes it writes; an attribute at its default is left out, and one that is not a string is JSON unless `Formats` gives it a form of its own (`SpaceList`: `photos="p1 p2 p3"`). An importer reads one back with `ParseDirective` and `Markdown.DirectiveNode`, which types each attribute by the schema; `Markdown.SplitInline` splits a line into its inline directives' nodes and the Markdown between them, which it hands to the importer's own function (plain text by default). The Markdown parser itself is the app's.

**Marks and spaces.** Before a mark's function wraps a text node, the writer moves the white space at its edges outside, as prosemirror-markdown does, so a strong " early" writes ` **early**`, which CommonMark reads as strong; `Markdown.Verbatim` names marks such as code whose text keeps its spaces.

**Highlights.** `RenderHighlighted` marks ranges kept outside the document, such as the text of inline comments, in `<mark>` elements (`HighlightClass`, `HighlightAttr`), and `MapRange` moves a range through steps. A highlight with `Node` set names a node by its stable id instead, and the class and attribute go on the element its function opens with `Open`: a comment on an image or a map, which has no text to select. Such an anchor needs no mapping, and is gone when `IDs.Find` no longer finds the node.

**Stable ids.** A node type opts in with an attribute in the schema (a string, default `""`) and an entry in `IDs`. coedit's editor gives each such node an id when it is made and a fresh one to a copy (a paste, a split), and `NewID()` makes one in the same format (16 characters of 0-9 and a-z) for nodes the server makes, in an import or a whole document it commits; the server's `Rules.IDs` refuses a commit that leaves one without an id or with an id another node holds, on the blocks the commit changed. Add ids to existing documents before turning the rule on: a restore of an old version changes its blocks, and their nodes would be refused.

**Dynamic blocks.** `HTML.Dynamic` names node types whose HTML changes while the node does not; `HTML.DynamicKey(r, n)` says what such a node shows now (a version, a hash), and `BlockCache` keeps a block whose keys stay the same. Without a key it renders the block every time.

## Tests

    go test ./...                                  # the package, its rules and renderers
    cd oracle && go test ./...                     # against ProseMirror in goja, about 5 s
    cd oracle && go test -run '^$' -fuzz FuzzRebase -fuzztime 1m

The oracle tests are a module of their own, `github.com/zweiundeins/prosemirror-go/oracle` in `oracle/`, so that this module requires nothing: importing `prosemirror` brings no goja into an app's module graph. Its `go.mod` replaces prosemirror-go with `../`, as lith's nested modules do, and it tags as `oracle/vX.Y.Z`. They run `oracle/oracle.js`, a committed bundle of prosemirror-model, prosemirror-transform, the rebase and a document generator, inside goja: fixture documents parse and write back byte for byte, `check` gives the same verdicts and messages, 200 batches of steps and 1,500 rebased commits end in V8's documents, and the fuzzer has the generator make documents and concurrent commits that Go must rebase to the same steps. The test schema (`testdata/schema.json`) holds a typical editor's nodes and four app nodes (`tag`, `mention`, `xref`, `embed`). The package documentation says how to change the ProseMirror versions and regenerate the fixtures in Node.

**Your own schema against ProseMirror.** Package `oracle` is the oracle the tests use, for an app's tests too: the app requires `github.com/zweiundeins/prosemirror-go/oracle` (in its tests only, so goja stays out of its binary), and `oracle.New(schemaJSON)` starts ProseMirror in goja with the app's schema; `Check`, `Roundtrip`, `Load`, `Apply`, `JSON` and `Rebase` answer as prosemirror-model and prosemirror-transform do, so the app compares what prosemirror-go does with its documents and steps. [BENCHMARKS.md](BENCHMARKS.md) has the measured times.

## Licence

MIT, © zwei und eins gmbh and contributors ([LICENSE](LICENSE)). The code is a port of ProseMirror (MIT, © Marijn Haverbeke and others) and of prosemirror-collab-commit's `applyCommit` (MIT, © Greg Zapp); the JavaScript the tests run keeps its licences in [oracle/LICENSES.txt](oracle/LICENSES.txt).

Extracted from [tickit](https://github.com/zweiundeins/tickit)'s `internal/doc`, whose history this repository carries.
