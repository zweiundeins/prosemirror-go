package prosemirror

import (
	"encoding/json"
	"os"
	"sync"
	"testing"

	"github.com/zweiundeins/prosemirror-go/oracle"
)

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

func readFixture(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
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
