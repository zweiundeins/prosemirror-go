# Changelog

Until 1.0, a minor release may change the API; its entry says what to change in an app.

## v0.1.0 (2026-10-25)

The first release: tickit's `internal/doc`, extracted as a module of its own, with no node type of tickit's left in it. To move an app from `internal/doc`:

- Import `github.com/zweiundeins/prosemirror-go` (package `prosemirror`) for the model, the steps, the rebase and the JSON. Load the schema with `NewSchema(json)` or `BuildSchema(spec)`; package-level `ParseDoc` and `ParseSteps` are methods of the schema now.
- Move the renderers of the app's node types into an `HTML[R]`, a `Markdown[R]` and a `LeafText` of its own, and its checks into `Rules.Nodes` and `Rules.Marks`, built per commit with what they need to look up. `Rules.Noun` names the document in the size refusals.
- `RenderHighlighted`'s element is `<mark class="highlight" data-highlight="…">` unless `HTML.HighlightClass` and `HighlightAttr` say otherwise.

- `prosemirror`: a Go port of prosemirror-model 1.25.12 and the five steps of prosemirror-transform 1.12.2, with step maps, mappings with mirrors, `Rebase` (collab-commit's `applyCommit`), `FindDiffStart`, `FindDiffEnd`, `RestoreStep`, `Diff`, and JSON byte for byte as the browser writes it. Standard library only; the module requires nothing.
- Hooks for the app's node types: `HTML[R]` with a resolver per render, `BlockCache` with `Dynamic` and `DynamicKey`, `Markdown[R]` with generic directives (`Directive`, `Formats`, `SpaceList`, `ParseDirective`, `DirectiveNode`, `SplitInline`) and white space moved outside the marks' delimiters (`Verbatim` keeps it for code), `LeafText`, `Rules`.
- Stable node ids: `IDs`, `Rules.IDs`, `FindNode`, `NewID`, and highlights of a node's element (`Highlight.Node`).
- `oracle` (tag `oracle/vX.Y.Z`): a module of its own with ProseMirror in goja, embedded, for the differential tests and the fuzzer, and for an app's tests of its own schema (`oracle.New`).
