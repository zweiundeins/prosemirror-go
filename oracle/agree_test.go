package oracle_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/zweiundeins/prosemirror-go"
	"github.com/zweiundeins/prosemirror-go/oracle"
)

// prosemirror-go against the real prosemirror-model and
// prosemirror-transform: the fixtures and the test schema of the root
// module's testdata (written in V8 by gen-fixtures.mjs), and the
// generator's fuzz cases. In a copy of the module without the root's
// testdata the tests skip.

var testSchema = prosemirror.MustSchema(readFixture(nil, "schema.json"))

func parseDoc(data []byte) (prosemirror.Doc, error)      { return testSchema.ParseDoc(data) }
func parseSteps(data []byte) ([]prosemirror.Step, error) { return testSchema.ParseSteps(data) }

func mustDoc(t testing.TB, s string) prosemirror.Doc {
	t.Helper()
	d, err := parseDoc([]byte(s))
	if err != nil {
		t.Fatalf("%s: %v", s, err)
	}
	return d
}

func docOf(blocks ...string) string {
	return `{"type":"doc","content":[` + strings.Join(blocks, ",") + `]}`
}

const para = `{"type":"paragraph","content":[{"type":"text","text":"hi"}]}`

func inPara(inline string) string { return docOf(`{"type":"paragraph","content":[` + inline + `]}`) }

func text(s string, marks ...string) string {
	if len(marks) == 0 {
		return `{"type":"text","text":` + string(prosemirror.AppendValue(nil, s)) + `}`
	}
	return `{"type":"text","text":` + string(prosemirror.AppendValue(nil, s)) + `,"marks":[` + strings.Join(marks, ",") + `]}`
}

// readFixture reads a file of the root module's testdata; with t nil it
// panics where the file is missing.
func readFixture(t testing.TB, name string) []byte {
	b, err := os.ReadFile("../testdata/" + name)
	if err != nil {
		if t == nil {
			return []byte(`{"nodes":[{"name":"doc","content":"text*"},{"name":"text"}]}`)
		}
		t.Skip("no ../testdata (a copy of the oracle module alone): ", err)
	}
	return b
}

// rawList splits a JSON array into its elements.
func rawList(t testing.TB, data []byte) []json.RawMessage {
	t.Helper()
	var out []json.RawMessage
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

var docFixtures = []string{
	"f20k-doc", "f20k-final", "f200k-doc", "f200k-final",
	"f20k-k0-final", "f20k-k1-final", "f20k-k5-final", "f20k-k20-final", "f200k-k5-final",
}

// The oracle: package oracle runs prosemirror-model 1.25.12 and
// prosemirror-transform 1.12.2 in goja, here with the test schema.

type jsOracle struct{ *oracle.Oracle }

func loadOracle(t testing.TB) *jsOracle {
	t.Helper()
	o, err := oracle.New(readFixture(t, "schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	return &jsOracle{o}
}

type errString string

func (e errString) Error() string { return string(e) }

var (
	sharedOracleOnce sync.Once
	sharedOracle     *jsOracle
)

// theOracle returns one oracle shared by the tests of the process.
func theOracle(t testing.TB) *jsOracle {
	sharedOracleOnce.Do(func() { sharedOracle = loadOracle(t) })
	if sharedOracle == nil {
		t.Fatal("oracle failed to load")
	}
	return sharedOracle
}

var (
	fuzzOracleMu   sync.Mutex
	fuzzOracle     *jsOracle
	fuzzOracleUses int
)

// fuzzRuntime returns an oracle that is replaced every 100 uses: goja keeps
// what prosemirror-model's WeakMap caches hold, and a fuzz worker would grow
// past a gigabyte in a few thousand cases.
func fuzzRuntime(t testing.TB) *jsOracle {
	fuzzOracleMu.Lock()
	defer fuzzOracleMu.Unlock()
	if fuzzOracle == nil || fuzzOracleUses >= 100 {
		fuzzOracle, fuzzOracleUses = loadOracle(t), 0
	}
	fuzzOracleUses++
	return fuzzOracle
}

func (o *jsOracle) call(name string, args ...any) (string, error) { return o.Call(name, args...) }

func (o *jsOracle) must(t testing.TB, name string, args ...any) string {
	t.Helper()
	s, err := o.Call(name, args...)
	if err != nil {
		t.Fatalf("oracle.%s: %v", name, err)
	}
	return s
}

var diffPairs = [][2]string{
	{"f20k-doc", "f20k-final"}, {"f20k-doc", "f20k-k5-final"}, {"f20k-k0-final", "f20k-k20-final"},
	{"f200k-doc", "f200k-final"}, {"f20k-final", "f20k-doc"},
}

// check() agrees with the oracle on hand-made documents, valid and not,
// with prosemirror-model's messages.
func TestCheckAgreesWithOracle(t *testing.T) {
	o := theOracle(t)
	cases := []string{
		docOf(para),
		`{"type":"doc"}`,
		docOf(`{"type":"paragraph","content":[{"type":"heading","attrs":{"level":1}}]}`),
		docOf(`{"type":"paragraph","content":[{"type":"tag"}]}`),
		docOf(`{"type":"paragraph","content":[{"type":"tag","attrs":{}}]}`),
		docOf(`{"type":"paragraph","content":[{"type":"tag","attrs":""}]}`),
		docOf(`{"type":"paragraph","content":[{"type":"tag","attrs":{"name":7}}]}`),
		docOf(`{"type":"paragraph","content":[{"type":"tag","attrs":{"name":"WEB-1","extra":1}}]}`),
		docOf(`{"type":"video"}`),
		docOf(`{"type":"heading","attrs":{"level":"2"},"content":[{"type":"text","text":"x"}]}`),
		docOf(`{"type":"heading","attrs":{"level":null}}`),
		docOf(`{"type":"heading","attrs":null}`),
		docOf(`{"type":"heading","attrs":0}`),
		docOf(`{"type":"heading","attrs":{"level":2.5}}`),
		docOf(`{"type":"task_list","content":[{"type":"task_item","attrs":{"checked":"yes"},"content":[` + para + `]}]}`),
		docOf(`{"type":"task_list","content":[{"type":"task_item","content":[{"type":"heading"}]}]}`),
		docOf(`{"type":"image","attrs":{"imageId":5}}`),
		docOf(`{"type":"image"}`),
		docOf(`{"type":"image","attrs":{"imageId":"img-1"}}`),
		docOf(`{"type":"code_block","content":[{"type":"text","text":"x","marks":[{"type":"strong"}]}]}`),
		docOf(`{"type":"code_block","content":[{"type":"hard_break"}]}`),
		docOf(`{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"strong"},{"type":"strong"}]}]}`),
		docOf(`{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"em"},{"type":"strong"}]}]}`),
		docOf(`{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"blink"}]}]}`),
		docOf(`{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"link"}]}]}`),
		docOf(`{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":1}}]}]}`),
		docOf(`{"type":"paragraph","content":[{"type":"text","text":"x","marks":{"type":"em"}}]}`),
		docOf(`{"type":"paragraph","content":[{"type":"text","text":""}]}`),
		docOf(`{"type":"paragraph","content":[{"type":"text"}]}`),
		docOf(`{"type":"paragraph","content":[{"type":"text","text":"a"},{"type":"text","text":"b"}]}`),
		docOf(`{"type":"paragraph","content":[null]}`),
		docOf(`{"type":"paragraph","content":{"type":"text","text":"a"}}`),
		docOf(`{"type":"paragraph","marks":[{"type":"strong"}]}`),
		docOf(`{"type":"table","content":[{"type":"table_row","content":[` + para + `]}]}`),
		docOf(`{"type":"table","content":[{"type":"table_row","content":[{"type":"table_cell","attrs":{"colspan":"2"},"content":[` + para + `]}]}]}`),
		docOf(`{"type":"table","content":[{"type":"table_row","content":[{"type":"table_cell","attrs":{"colwidth":[100,{"b":1,"2":0,"a":[]}]},"content":[` + para + `]}]}]}`),
		docOf(`{"type":"bullet_list","content":[{"type":"list_item","content":[{"type":"bullet_list","content":[{"type":"list_item","content":[` + para + `]}]}]}]}`),
		docOf(`{"type":"paragraph","content":[{"type":"text","text":"tab\tquote\" back\\ ctl\u0001 é 😀 \u2028"}]}`),
		docOf(`{"type":"paragraph","content":[{"type":"mention","attrs":{"id":"s-priya"},"marks":[{"type":"link","attrs":{"href":"https://x.y"}}]}]}`),
		`{"type":"paragraph"}`,
	}
	for i, c := range cases {
		want := o.must(t, "check", c)
		_, err := parseDoc([]byte(c))
		got := ""
		if err != nil {
			got = err.Error()
		}
		if strings.HasPrefix(got, "A document is") {
			// The oracle checks the node it gets; ParseDoc wants a doc.
			continue
		}
		if got != want {
			t.Errorf("case %d %s:\n go: %q\n js: %q", i, c, got, want)
		}
		if err == nil {
			rt := o.must(t, "roundtrip", c)
			d, _ := parseDoc([]byte(c))
			if out, _ := prosemirror.MarshalDoc(d); string(out) != rt {
				t.Errorf("case %d: writes %s, oracle %s", i, out, rt)
			}
		}
	}
}

func randomMapping(r *rand.Rand) (string, *prosemirror.Mapping) {
	n := 1 + r.IntN(5)
	m := prosemirror.NewMapping(nil)
	var js struct {
		Maps   [][2]any `json:"maps"`
		Mirror []int    `json:"mirror"`
	}
	for i := 0; i < n; i++ {
		var ranges []int
		pos := r.IntN(5)
		for k := r.IntN(3); k >= 0; k-- {
			ranges = append(ranges, pos, r.IntN(4), r.IntN(4))
			pos += 1 + r.IntN(6)
		}
		inv := r.IntN(3) == 0
		m.AppendMap(prosemirror.NewStepMap(ranges, inv), -1)
		js.Maps = append(js.Maps, [2]any{ranges, inv})
	}
	if r.IntN(2) == 0 {
		// A mirror pairs a map with its inverse, as rebasing does.
		a := r.IntN(n)
		inv := m.Maps()[a].Invert()
		m.AppendMap(inv, a)
		js.Maps = append(js.Maps, [2]any{inv.Ranges(), inv.Inverted()})
		js.Mirror = []int{len(m.Maps()) - 1, a}
	}
	b, _ := json.Marshal(js)
	return string(b), m
}

// StepMap and Mapping with mirrors map positions and steps as
// prosemirror-transform does.
func TestMappingAgreesWithOracle(t *testing.T) {
	o := theOracle(t)
	r := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < 3000; i++ {
		js, m := randomMapping(r)
		pos, assoc := r.IntN(30), []int{-1, 1}[r.IntN(2)]
		res := m.MapResult(pos, assoc)
		got := fmt.Sprintf(`{"pos":%d,"deleted":%t,"before":%t,"after":%t,"across":%t}`, res.Pos, res.Deleted(), res.DeletedBefore(), res.DeletedAfter(), res.DeletedAcross())
		if want := o.must(t, "mapPos", js, pos, assoc); got != want {
			t.Fatalf("mapping %s pos %d assoc %d:\n go %s\n js %s", js, pos, assoc, got, want)
		}
		if p := m.Map(pos, assoc); p != res.Pos && strings.Contains(js, `"mirror":null`) {
			t.Fatalf("Map %d, MapResult %d", p, res.Pos)
		}
		var st string
		a, b := r.IntN(20), r.IntN(20)
		if a > b {
			a, b = b, a
		}
		switch r.IntN(4) {
		case 0:
			st = fmt.Sprintf(`{"stepType":"replace","from":%d,"to":%d}`, a, b)
		case 1:
			st = fmt.Sprintf(`{"stepType":"addMark","mark":{"type":"em"},"from":%d,"to":%d}`, a, b)
		case 2:
			st = fmt.Sprintf(`{"stepType":"attr","pos":%d,"attr":"level","value":2}`, a)
		default:
			g := a + r.IntN(b-a+1)
			st = fmt.Sprintf(`{"stepType":"replaceAround","from":%d,"to":%d,"gapFrom":%d,"gapTo":%d,"insert":0}`, a, b, g, g+r.IntN(b-g+1))
		}
		s, err := testSchema.ParseStep([]byte(st))
		if err != nil {
			t.Fatal(err)
		}
		gotStep := "null"
		if ms := s.Map(m); ms != nil {
			gotStep = string(prosemirror.MarshalStep(ms))
		}
		if want := o.must(t, "mapStep", st, js); gotStep != want {
			t.Fatalf("mapping %s step %s:\n go %s\n js %s", js, st, gotStep, want)
		}
		inv := m.Invert()
		_ = inv.Map(pos, assoc)
	}
}

// prosemirror.FindDiffStart and prosemirror.FindDiffEnd agree with prosemirror-model, and the
// restore step turns the current version into the old one (FR-89).
func TestRestoreStep(t *testing.T) {
	o := theOracle(t)
	for _, p := range diffPairs {
		ja, jb := readFixture(t, p[0]+".json"), readFixture(t, p[1]+".json")
		a, b := mustDoc(t, string(ja)), mustDoc(t, string(jb))
		start, ok := prosemirror.FindDiffStart(a.Content(), b.Content(), 0)
		if want := o.must(t, "diffStart", string(ja), string(jb)); (ok && strconv.Itoa(start) != want) || (!ok && want != "null") {
			t.Errorf("%v: diffStart %d, oracle %s", p, start, want)
		}
		ea, eb, _ := prosemirror.FindDiffEnd(a.Content(), b.Content(), a.Content().Size(), b.Content().Size())
		var end struct{ A, B int }
		json.Unmarshal([]byte(o.must(t, "diffEnd", string(ja), string(jb))), &end)
		if ea != end.A || eb != end.B {
			t.Errorf("%v: diffEnd %d %d, oracle %+v", p, ea, eb, end)
		}
		st := prosemirror.RestoreStep(a, b)
		got, err := st.Apply(a)
		if err != nil || !got.Eq(b) {
			t.Fatalf("%v: restore step %v", p, err)
		}
		if rs := st.(*prosemirror.ReplaceStep); rs.From == 0 && rs.To == a.Content().Size() {
			t.Logf("%v: the restore replaces the whole document", p)
		}
	}
	d := mustDoc(t, docOf(para))
	if prosemirror.RestoreStep(d, d) != nil {
		t.Fatal("restore of an equal document")
	}
	// Emoji: the step never cuts a pair.
	a := mustDoc(t, inPara(text("x😀y")))
	b := mustDoc(t, inPara(text("x😁y")))
	if s, _ := prosemirror.FindDiffStart(a.Content(), b.Content(), 0); s != 2 {
		t.Fatalf("diff start in an emoji: %d", s)
	}
	if got, err := prosemirror.RestoreStep(a, b).Apply(a); err != nil || !got.Eq(b) {
		t.Fatal(err)
	}
}

// Every fixture document parses and writes back byte-equal, as the oracle
// writes it (DM-05: documents are stored as prosemirror-model's JSON).
func TestFixtureDocsRoundTrip(t *testing.T) {
	o := theOracle(t)
	for _, name := range docFixtures {
		data := bytes.TrimSpace(readFixture(t, name+".json"))
		d, err := parseDoc(data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		out, _ := prosemirror.MarshalDoc(d)
		if !bytes.Equal(out, data) {
			t.Errorf("%s: Go writes it back differently (%d vs %d bytes)", name, len(out), len(data))
		}
		if want := o.must(t, "roundtrip", string(data)); want != string(out) {
			t.Errorf("%s: the oracle writes it differently", name)
		}
		if msg := o.must(t, "check", string(data)); msg != "" {
			t.Errorf("%s: the oracle refuses it: %s", name, msg)
		}
		if got := d.JSONLen(); got != len(out) {
			t.Errorf("%s: JSONLen %d, want %d", name, got, len(out))
		}
	}
}

// applyAndCompare applies each batch in Go and in the oracle and compares
// every step's outcome, and the documents every `every` batches.
func applyAndCompare(t *testing.T, prefix string, every int) {
	o := theOracle(t)
	data := readFixture(t, prefix+"-doc.json")
	d, err := parseDoc(data)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := strconv.Atoi(o.must(t, "load", string(data)))
	defer o.call("free", id)
	steps := 0
	for bi, raw := range rawList(t, readFixture(t, prefix+"-batches.json")) {
		for si, sraw := range rawList(t, raw) {
			st, err := testSchema.ParseStep(sraw)
			if err != nil {
				t.Fatalf("batch %d step %d: %v", bi, si, err)
			}
			if got := string(prosemirror.MarshalStep(st)); got != string(sraw) {
				t.Fatalf("batch %d step %d: writes back as %s, want %s", bi, si, got, sraw)
			}
			res, err := o.call("apply", id, string(sraw))
			next, gerr := st.Apply(d)
			switch {
			case err != nil:
				if gerr == nil || prosemirror.IsStepFailed(gerr) {
					t.Fatalf("batch %d step %d: oracle throws %v, Go: %v", bi, si, err, gerr)
				}
			case res != "":
				if !prosemirror.IsStepFailed(gerr) {
					t.Fatalf("batch %d step %d: oracle %s, Go: %v", bi, si, res, gerr)
				}
			default:
				if gerr != nil {
					t.Fatalf("batch %d step %d %s: oracle applies it, Go: %v", bi, si, sraw, gerr)
				}
				d = next
			}
			steps++
		}
		if (bi+1)%every == 0 {
			got, _ := prosemirror.MarshalDoc(d)
			if want := o.must(t, "json", id); want != string(got) {
				t.Fatalf("after batch %d: documents differ at byte %d", bi, firstDiff(want, string(got)))
			}
		}
	}
	got, _ := prosemirror.MarshalDoc(d)
	if want := bytes.TrimSpace(readFixture(t, prefix+"-final.json")); !bytes.Equal(got, want) {
		t.Fatalf("final document differs from V8's at byte %d", firstDiff(string(want), string(got)))
	}
	if err := d.Check(); err != nil {
		t.Fatal(err)
	}
	t.Logf("%s: %d steps agree", prefix, steps)
}

func firstDiff(a, b string) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return min(len(a), len(b))
}

func TestBatchesF20k(t *testing.T)  { applyAndCompare(t, "f20k", 1) }
func TestBatchesF200k(t *testing.T) { applyAndCompare(t, "f200k", 20) }

type commitJSON struct {
	Base    int             `json:"base"`
	Steps   json.RawMessage `json:"steps"`
	Applied json.RawMessage `json:"applied"`
	Error   *string         `json:"error"`
}

// replayCommits rebases every commit in Go as the writer does and compares
// the steps as applied with the oracle's, and the final document with V8's.
func replayCommits(t *testing.T, prefix string, k int) {
	o := theOracle(t)
	data := readFixture(t, prefix+"-doc.json")
	d, err := parseDoc(data)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := strconv.Atoi(o.must(t, "load", string(data)))
	defer o.call("free", id)
	var commits []commitJSON
	if err := json.Unmarshal(readFixture(t, fmt.Sprintf("%s-k%d-commits.json", prefix, k)), &commits); err != nil {
		t.Fatal(err)
	}
	var history [][]prosemirror.Step
	var historyJSON []string
	nsteps := 0
	for ci, c := range commits {
		steps, err := parseSteps(c.Steps)
		if err != nil {
			t.Fatalf("commit %d: %v", ci, err)
		}
		nsteps += len(steps)
		var since []prosemirror.Step
		var sinceJSON []string
		for v := c.Base; v < len(history); v++ {
			since = append(since, history[v]...)
			if historyJSON[v] != "[]" {
				sinceJSON = append(sinceJSON, strings.TrimSuffix(strings.TrimPrefix(historyJSON[v], "["), "]"))
			}
		}
		want := o.must(t, "rebase", id, "["+strings.Join(sinceJSON, ",")+"]", string(c.Steps))
		next, applied, err := prosemirror.Rebase(d, since, steps)
		if err != nil {
			t.Fatalf("commit %d: %v", ci, err)
		}
		got, _ := prosemirror.MarshalSteps(applied)
		if string(got) != want {
			t.Fatalf("commit %d (base %d): applied steps differ\n go: %s\njs: %s", ci, c.Base, got, want)
		}
		d = next
		history = append(history, applied)
		historyJSON = append(historyJSON, want)
	}
	got, _ := prosemirror.MarshalDoc(d)
	if want := bytes.TrimSpace(readFixture(t, fmt.Sprintf("%s-k%d-final.json", prefix, k))); !bytes.Equal(got, want) {
		t.Fatalf("final document differs from V8's at byte %d", firstDiff(string(want), string(got)))
	}
	t.Logf("%s k=%d: %d commits, %d steps agree", prefix, k, len(commits), nsteps)
}

func TestRebaseF20kK0(t *testing.T)  { replayCommits(t, "f20k", 0) }
func TestRebaseF20kK1(t *testing.T)  { replayCommits(t, "f20k", 1) }
func TestRebaseF20kK5(t *testing.T)  { replayCommits(t, "f20k", 5) }
func TestRebaseF20kK20(t *testing.T) { replayCommits(t, "f20k", 20) }
func TestRebaseF200kK5(t *testing.T) { replayCommits(t, "f200k", 5) }

type fuzzCaseJSON struct {
	Doc     json.RawMessage `json:"doc"`
	Commits []commitJSON    `json:"commits"`
	Final   json.RawMessage `json:"final"`
}

// runFuzzCase has the oracle's generator make a document and commits made
// on versions up to maxK behind the head, rebased by applyCommit in goja;
// Go rebases the same commits and must apply the same steps (FR-82.4 and
// MP-05: the browser and the writer drop the same steps).
func runFuzzCase(t *testing.T, seed uint32, kb, commits, maxK, edits int) (int, int) {
	o := fuzzRuntime(t)
	opts := fmt.Sprintf(`{"kb":%d,"commits":%d,"maxK":%d,"edits":%d}`, kb, commits, maxK, edits)
	raw := o.must(t, "fuzzCase", seed, opts)
	var c fuzzCaseJSON
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatal(err)
	}
	d, err := parseDoc(c.Doc)
	if err != nil {
		t.Fatalf("seed %d: %v", seed, err)
	}
	var history [][]prosemirror.Step
	steps, dropped := 0, 0
	for ci, cm := range c.Commits {
		st, err := parseSteps(cm.Steps)
		if err != nil {
			t.Fatalf("seed %d commit %d: %v", seed, ci, err)
		}
		var since []prosemirror.Step
		for v := cm.Base; v < len(history); v++ {
			since = append(since, history[v]...)
		}
		next, applied, err := prosemirror.Rebase(d, since, st)
		if cm.Error != nil {
			if err == nil {
				t.Fatalf("seed %d commit %d: the oracle throws %q, Go applies it", seed, ci, *cm.Error)
			}
			continue
		}
		if err != nil {
			t.Fatalf("seed %d commit %d (base %d of %d): Go: %v\nsteps %s", seed, ci, cm.Base, len(history), err, cm.Steps)
		}
		got, _ := prosemirror.MarshalSteps(applied)
		if string(got) != string(cm.Applied) {
			t.Fatalf("seed %d commit %d (base %d of %d): applied steps differ\nsteps %s\n go %s\n js %s", seed, ci, cm.Base, len(history), cm.Steps, got, cm.Applied)
		}
		steps += len(st)
		dropped += len(st) - len(applied)
		d = next
		history = append(history, applied)
	}
	got, _ := prosemirror.MarshalDoc(d)
	if string(got) != string(c.Final) {
		t.Fatalf("seed %d: final documents differ at byte %d", seed, firstDiff(string(c.Final), string(got)))
	}
	return steps, dropped
}

func TestRebaseGenerated(t *testing.T) {
	n := 60
	if testing.Short() {
		n = 10
	}
	steps, dropped := 0, 0
	for seed := uint32(1); seed <= uint32(n); seed++ {
		s, d := runFuzzCase(t, seed, 2, 12, int(seed%10), 4)
		steps += s
		dropped += d
	}
	t.Logf("%d cases, %d steps, %d dropped by both", n, steps, dropped)
}

func FuzzRebase(f *testing.F) {
	for _, s := range []uint32{1, 2, 3, 42} {
		f.Add(s, uint8(5), uint8(4))
	}
	f.Fuzz(func(t *testing.T, seed uint32, k, edits uint8) {
		runFuzzCase(t, seed, 1+int(seed%3), 10, int(k%16), 1+int(edits%6))
	})
}
