// Package oracle runs the real prosemirror-model 1.25.12 and
// prosemirror-transform 1.12.2 in goja, for tests that check Go code
// against ProseMirror: the bundle oracle.js (built from src/ by build.sh,
// committed) is embedded. It is for tests only; it brings goja, which the
// prosemirror package does not import.
//
// Every function takes and returns JSON strings, so no object crosses
// between goja and Go. An Oracle is safe for concurrent use; goja keeps
// what prosemirror-model's caches hold, so a long fuzz run makes a new
// Oracle every few hundred cases.
//
// An app checks its own schema against ProseMirror with it:
//
//	o, err := oracle.New(schemaJSON)
//	msg, err := o.Check(docJSON)     // "" when valid, else prosemirror-model's message
//	out, err := o.Roundtrip(docJSON) // the document as prosemirror-model writes it
//	id, err := o.Load(docJSON)
//	failed, err := o.Apply(id, stepJSON)
//	applied, err := o.Rebase(id, sinceJSON, stepsJSON)
package oracle

import (
	_ "embed"
	"errors"
	"strconv"
	"strings"
	"sync"

	"github.com/dop251/goja"
)

//go:embed oracle.js
var bundle string

// goja's String.prototype.split leaves empty strings between the tokens of
// prosemirror-model's content-expression tokenizer, which V8 does not; the
// patch drops them, which V8 never produces in the middle.
const (
	splitCall    = `.split(/\s*(?=\b|\W|$)/)`
	splitPatched = `.split(/\s*(?=\b|\W|$)/).filter(function(t){return t!==""})`
)

var program = sync.OnceValues(func() (*goja.Program, error) {
	if !strings.Contains(bundle, splitCall) {
		return nil, errors.New("oracle.js: the tokenizer's split call is gone; update the patch")
	}
	return goja.Compile("oracle.js", strings.Replace(bundle, splitCall, splitPatched, 1), false)
})

// Oracle is a goja runtime with ProseMirror and a schema.
type Oracle struct {
	mu sync.Mutex
	vm *goja.Runtime
	o  *goja.Object
}

// New starts an oracle with a schema in the JSON format prosemirror-go's
// NewSchema reads.
func New(schemaJSON []byte) (*Oracle, error) {
	prog, err := program()
	if err != nil {
		return nil, err
	}
	vm := goja.New()
	if _, err := vm.RunProgram(prog); err != nil {
		return nil, err
	}
	o := &Oracle{vm: vm, o: vm.Get("oracle").ToObject(vm)}
	if _, err := o.Call("init", string(schemaJSON)); err != nil {
		return nil, err
	}
	return o, nil
}

// Call runs oracle[name](args...) of oracle.js (src/entry.js lists them)
// and returns the result as a string; a JavaScript exception is the error.
func (o *Oracle) Call(name string, args ...any) (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	fn, ok := goja.AssertFunction(o.o.Get(name))
	if !ok {
		return "", errors.New("oracle has no function " + name)
	}
	vs := make([]goja.Value, len(args))
	for i, a := range args {
		vs[i] = o.vm.ToValue(a)
	}
	v, err := fn(goja.Undefined(), vs...)
	if err != nil {
		var ex *goja.Exception
		if errors.As(err, &ex) {
			return "", errors.New(ex.Value().String())
		}
		return "", err
	}
	return v.String(), nil
}

// Check reads a document with Node.fromJSON and checks it: "" when it is
// valid, else prosemirror-model's message.
func (o *Oracle) Check(doc string) (string, error) { return o.Call("check", doc) }

// Roundtrip reads a document and writes it back as prosemirror-model does.
func (o *Oracle) Roundtrip(doc string) (string, error) { return o.Call("roundtrip", doc) }

// Load keeps a document in the oracle for Apply and Rebase.
func (o *Oracle) Load(doc string) (int, error) {
	s, err := o.Call("load", doc)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(s)
}

// Free forgets a loaded document.
func (o *Oracle) Free(id int) { o.Call("free", id) }

// JSON is a loaded document as prosemirror-model writes it.
func (o *Oracle) JSON(id int) (string, error) { return o.Call("json", id) }

// Apply applies one step to a loaded document as Transform.maybeStep does:
// "" when it applied, "failed: <reason>" when it failed and the document
// stayed; an error when ProseMirror throws.
func (o *Oracle) Apply(id int, step string) (string, error) { return o.Call("apply", id, step) }

// Rebase maps a commit's steps over the steps since its base and applies
// them to a loaded document, as prosemirror-go's Rebase does, and returns
// the steps as applied.
func (o *Oracle) Rebase(id int, since, steps string) (string, error) {
	return o.Call("rebase", id, since, steps)
}
