package m0adapter

import (
	"sort"
	"strconv"
)

// canonicalizer emits CCJ-1 (registrar CCJ-1 §2.1, as referenced by
// m0-launch.md §2) in one bounded traversal of the parsed entry:
//
//   - object member names sorted by Unicode scalar value, which for valid
//     UTF-8 is byte order;
//   - no insignificant whitespace and no final LF;
//   - strings as UTF-8 with only ", \, \b, \f, \n, \r, \t escaped and the
//     remaining U+0000..U+001F as lowercase \u00xx; "/" and all non-ASCII
//     (U+2028/U+2029 included) emitted raw; no Unicode normalization;
//   - integers in shortest base-10 form.
//
// The admitted grammar has no arrays, booleans or null, so none is emitted.
// While emitting the top-level "input" member it records that value's byte
// span, so CCJ-1(input) is a sub-slice of CCJ-1(entry) and needs no second
// traversal or buffer.
type canonicalizer struct {
	p          *parser
	out        []byte
	limit      int
	inputStart int
	inputEnd   int
}

func canonicalize(p *parser, root jvalue, limit int) (*canonicalizer, error) {
	c := &canonicalizer{p: p, out: make([]byte, 0, limit), limit: limit, inputStart: -1, inputEnd: -1}
	if err := c.value(root, true); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *canonicalizer) grow(n int) error {
	if len(c.out)+n > c.limit {
		return refuse(ReasonEntryCanonicalLimit)
	}
	return nil
}

func (c *canonicalizer) value(v jvalue, top bool) error {
	switch v.kind {
	case kindString:
		return c.str(v.str)
	case kindInt:
		var tmp [24]byte
		b := strconv.AppendInt(tmp[:0], v.num, 10)
		if err := c.grow(len(b)); err != nil {
			return err
		}
		c.out = append(c.out, b...)
		return nil
	case kindObject:
		members := append([]jmember(nil), c.p.objects[v.obj].members...)
		sort.Slice(members, func(i, j int) bool { return members[i].name < members[j].name })
		if err := c.grow(1); err != nil {
			return err
		}
		c.out = append(c.out, '{')
		for i, m := range members {
			if i > 0 {
				if err := c.grow(1); err != nil {
					return err
				}
				c.out = append(c.out, ',')
			}
			if err := c.str(m.name); err != nil {
				return err
			}
			if err := c.grow(1); err != nil {
				return err
			}
			c.out = append(c.out, ':')
			start := len(c.out)
			if err := c.value(m.val, false); err != nil {
				return err
			}
			if top && m.name == "input" {
				c.inputStart, c.inputEnd = start, len(c.out)
			}
		}
		if err := c.grow(1); err != nil {
			return err
		}
		c.out = append(c.out, '}')
		return nil
	}
	return refuse(ReasonInternal)
}

const lowerHex = "0123456789abcdef"

func (c *canonicalizer) str(s string) error {
	if err := c.grow(2); err != nil {
		return err
	}
	c.out = append(c.out, '"')
	for i := 0; i < len(s); i++ {
		b := s[i]
		var esc []byte
		switch b {
		case '"':
			esc = []byte{'\\', '"'}
		case '\\':
			esc = []byte{'\\', '\\'}
		case '\b':
			esc = []byte{'\\', 'b'}
		case '\f':
			esc = []byte{'\\', 'f'}
		case '\n':
			esc = []byte{'\\', 'n'}
		case '\r':
			esc = []byte{'\\', 'r'}
		case '\t':
			esc = []byte{'\\', 't'}
		default:
			if b < 0x20 {
				esc = []byte{'\\', 'u', '0', '0', lowerHex[b>>4], lowerHex[b&0xf]}
			}
		}
		if esc == nil {
			if err := c.grow(1); err != nil {
				return err
			}
			c.out = append(c.out, b)
			continue
		}
		if err := c.grow(len(esc)); err != nil {
			return err
		}
		c.out = append(c.out, esc...)
	}
	if err := c.grow(1); err != nil {
		return err
	}
	c.out = append(c.out, '"')
	return nil
}
