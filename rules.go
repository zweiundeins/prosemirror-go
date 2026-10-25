package prosemirror

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// RuleError is a refusal by a rule; its message is a sentence a person
// can read.
type RuleError struct{ Msg string }

func (e *RuleError) Error() string { return e.Msg }

// Refuse returns a *RuleError with a formatted message, for rules.
func Refuse(format string, args ...any) error {
	return &RuleError{Msg: fmt.Sprintf(format, args...)}
}

// Rules are what a server checks of a document beyond its schema, after
// each commit: the size of the document, and per node and mark type what
// the app's attributes must look like (an id's format, a number's range).
// A rule returns nil, or a *RuleError (Refuse) whose message says what to
// change. The zero value checks the schema alone.
type Rules struct {
	// Noun names the document in the messages of the checks below:
	// "page" gives "The page is at its limit of 200 KB." Empty is
	// "document".
	Noun string
	// MaxBytes bounds the document's JSON as MarshalDoc writes it, and
	// MaxBlocks the number of block nodes in it, the document's own node
	// left out. A commit is refused only when it grows a document past
	// one; 0 means no bound.
	MaxBytes, MaxBlocks int
	// MaxAttrLength bounds every string attribute of every node and mark,
	// in UTF-16 units as the browser counts them; 0 means no bound.
	MaxAttrLength int
	// Nodes are the rules per node type name ("text" included). parent is
	// the node it sits in, nil for a top-level block.
	Nodes map[string]func(n, parent *Node) error
	// Marks are the rules per mark type name; n is the node the mark is
	// on.
	Marks map[string]func(m *Mark, n *Node) error
}

func (r Rules) noun() string {
	if r.Noun == "" {
		return "document"
	}
	return r.Noun
}

// Check validates a commit's result: the size bounds, the top node's
// content, and the schema and the rules on the top-level blocks of after
// that are not blocks of before (unchanged blocks keep their node, so a
// pointer comparison finds the changed ones, and the check costs the
// changed blocks only). before is nil for a new document, which checks
// every block.
func (r Rules) Check(before, after Doc) error {
	s := after.typ.schema
	if after.typ != s.top {
		return Refuse("This is not a %s.", r.noun())
	}
	if r.MaxBytes > 0 {
		if n := after.JSONLen(); n > r.MaxBytes && (before == nil || n > before.JSONLen()) {
			return Refuse("The %s is at its limit of %s KB.", r.noun(), thousands(r.MaxBytes/1024))
		}
	}
	if r.MaxBlocks > 0 {
		if n := after.Blocks(); n > r.MaxBlocks && (before == nil || n > before.Blocks()) {
			return Refuse("The %s is at its limit of %s blocks.", r.noun(), thousands(r.MaxBlocks))
		}
	}
	if !after.typ.validContent(after.content) {
		return Refuse("The %s holds something the editor cannot make.", r.noun())
	}
	for _, b := range changedBlocks(before, after) {
		if err := b.Check(); err != nil {
			return Refuse("The %s holds something the editor cannot make.", r.noun())
		}
		if err := r.checkNode(b, nil); err != nil {
			return err
		}
	}
	return nil
}

// changedBlocks returns the top-level blocks of after that are not, by
// pointer, at the same place from the start or the end of before.
func changedBlocks(before, after Doc) []*Node {
	var old []*Node
	if before != nil {
		old = before.content.nodes
	}
	cur := after.content.nodes
	lo := 0
	for lo < len(old) && lo < len(cur) && old[lo] == cur[lo] {
		lo++
	}
	hi := 0
	for hi < len(old)-lo && hi < len(cur)-lo && old[len(old)-1-hi] == cur[len(cur)-1-hi] {
		hi++
	}
	return cur[lo : len(cur)-hi]
}

func (r Rules) checkNode(n, parent *Node) error {
	if err := r.checkStrings(n.typ.attrs, n.attrs); err != nil {
		return err
	}
	for _, m := range n.marks {
		if err := r.checkStrings(m.typ.attrs, m.attrs); err != nil {
			return err
		}
		if rule := r.Marks[m.typ.name]; rule != nil {
			if err := rule(m, n); err != nil {
				return err
			}
		}
	}
	if rule := r.Nodes[n.typ.name]; rule != nil {
		if err := rule(n, parent); err != nil {
			return err
		}
	}
	for _, c := range n.content.nodes {
		if err := r.checkNode(c, n); err != nil {
			return err
		}
	}
	return nil
}

func (r Rules) checkStrings(specs []*attribute, values []any) error {
	if r.MaxAttrLength <= 0 {
		return nil
	}
	for i := range specs {
		if s, ok := values[i].(string); ok && len(s) > r.MaxAttrLength && utf16Len(s) > r.MaxAttrLength {
			return Refuse("An attribute holds at most %s characters.", thousands(r.MaxAttrLength))
		}
	}
	return nil
}

func thousands(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// SafeHref reports whether a link target may be rendered as a link: it
// parses with net/url, its scheme is http, https or mailto, http and https
// name a host, and it is at most 2,048 characters without spaces or
// control characters. It returns what the link's title shows: the host,
// or the address of a mailto link. Apps use it in the rule and the
// renderer of their link mark.
func SafeHref(href string) (title string, ok bool) {
	if href == "" || len(href) > 2048 {
		return "", false
	}
	for i := 0; i < len(href); i++ {
		if c := href[i]; c <= ' ' || c == 0x7f {
			return "", false
		}
	}
	u, err := url.Parse(href)
	if err != nil {
		return "", false
	}
	switch u.Scheme {
	case "http", "https":
		if u.Opaque != "" || u.Hostname() == "" {
			return "", false
		}
		return strings.ToLower(u.Host), true
	case "mailto":
		addr := u.Opaque
		if i := strings.IndexByte(addr, '?'); i >= 0 {
			addr = addr[:i]
		}
		if a, err := url.PathUnescape(addr); err == nil {
			addr = a
		}
		return addr, addr != ""
	}
	return "", false
}
