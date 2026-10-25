package prosemirror

import (
	"crypto/rand"
	"strconv"
)

// IDs names the node types that carry a stable id and the attribute that
// holds it, {"image": "id", "route": "id"}: a node keeps its id through
// every edit, so a comment, a map or a link can name the node itself. The
// schema declares the attribute (a string, default ""); coedit's editor
// gives a node its id when it is made, and a new one to a copy. Rules.IDs
// refuses a node without one and two nodes with the same.
type IDs map[string]string

// Of returns n's id, and whether n's type carries one.
func (ids IDs) Of(n *Node) (string, bool) {
	attr, ok := ids[n.typ.name]
	if !ok {
		return "", false
	}
	return n.AttrString(attr), true
}

// Find returns the node with the id and its position.
func (ids IDs) Find(d Doc, id string) (n *Node, pos int, ok bool) {
	if id == "" {
		return nil, 0, false
	}
	d.Descendants(func(c *Node, p int, _ *Node, _ int) bool {
		if ok {
			return false
		}
		if v, has := ids.Of(c); has && v == id {
			n, pos, ok = c, p, true
		}
		return !ok
	})
	return n, pos, ok
}

// FindNode returns the first node, in document order, whose attribute
// attr is value, and its position.
func FindNode(d Doc, attr, value string) (n *Node, pos int, ok bool) {
	d.Descendants(func(c *Node, p int, _ *Node, _ int) bool {
		if ok {
			return false
		}
		if i := c.typ.attrIndex(attr); i >= 0 {
			if s, _ := c.attrs[i].(string); s == value {
				n, pos, ok = c, p, true
			}
		}
		return !ok
	})
	return n, pos, ok
}

// each calls fn for every node of n (n included) whose type carries an id.
func (ids IDs) each(n *Node, fn func(c *Node, id string)) {
	if id, ok := ids.Of(n); ok {
		fn(n, id)
	}
	for _, c := range n.content.nodes {
		if c.typ.isText {
			continue
		}
		ids.each(c, fn)
	}
}

// check refuses an id-carrying node of the changed blocks without an id,
// and an id that two nodes of the document hold.
func (ids IDs) check(after Doc, changed []*Node) error {
	if len(ids) == 0 {
		return nil
	}
	mine := map[string]bool{}
	var err error
	in := map[*Node]bool{}
	for _, b := range changed {
		in[b] = true
		ids.each(b, func(c *Node, id string) {
			switch {
			case err != nil:
			case id == "":
				err = Refuse("This %s has no id.", c.typ.name)
			case mine[id]:
				err = Refuse("Two nodes have the id %s.", id)
			default:
				mine[id] = true
			}
		})
	}
	if err != nil || len(mine) == 0 {
		return err
	}
	for _, b := range after.content.nodes {
		if in[b] {
			continue
		}
		ids.each(b, func(_ *Node, id string) {
			if err == nil && mine[id] {
				err = Refuse("Two nodes have the id %s.", id)
			}
		})
	}
	return err
}

// NewID returns a new id for a node an app makes on the server (an
// import, a whole document it commits), in the format of coedit's editor:
// 16 characters of 0-9 and a-z, from crypto/rand.
func NewID() string {
	var b [9]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	var s []byte
	for _, x := range b {
		if x < 36 {
			s = append(s, '0')
		}
		s = strconv.AppendUint(s, uint64(x), 36)
	}
	return string(s[:16])
}
