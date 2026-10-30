package mikrotik

import (
	"errors"
	"net"
)

// ErrResponseTooLarge: the router (or whatever answers on its address) sent a
// word or a total response beyond the caps. Router hosts are tenant-supplied,
// so the binary transport bounds what it will read; the message deliberately
// carries no router text.
var ErrResponseTooLarge = errors.New("mikrotik: response from router too large")

const (
	maxAPIWordBytes     = 1 << 20 // one word (go-routeros allocates the declared length up front)
	maxAPIResponseBytes = 8 << 20 // whole call, same as the REST cap
)

var errMalformedFrame = errors.New("mikrotik: unexpected response from router")

// limitedConn sits between the socket and go-routeros. It decodes the API
// word-length prefixes as bytes stream through, and fails BEFORE the library
// sees a prefix declaring more than maxAPIWordBytes, so the library never
// allocates the peer-declared size. It also caps the total bytes read.
type limitedConn struct {
	net.Conn
	total   int
	prefix  []byte // length-prefix bytes of the word being parsed
	need    int    // prefix bytes still missing (0 when not inside a prefix)
	skip    int    // word body bytes still to pass through
	failure error  // sticky; set when a cap trips
}

func (c *limitedConn) Read(p []byte) (int, error) {
	if c.failure != nil {
		return 0, c.failure
	}
	n, err := c.Conn.Read(p)
	if n > 0 {
		if ferr := c.scan(p[:n]); ferr != nil {
			c.failure = ferr
			return 0, ferr
		}
	}
	return n, err
}

func (c *limitedConn) scan(b []byte) error {
	c.total += len(b)
	if c.total > maxAPIResponseBytes {
		return ErrResponseTooLarge
	}
	for len(b) > 0 {
		if c.skip > 0 {
			k := min(c.skip, len(b))
			c.skip -= k
			b = b[k:]
			continue
		}
		if len(c.prefix) == 0 {
			first := b[0]
			switch {
			case first < 0x80:
				c.need = 1
			case first < 0xC0:
				c.need = 2
			case first < 0xE0:
				c.need = 3
			case first < 0xF0:
				c.need = 4
			case first == 0xF0:
				c.need = 5
			default:
				return errMalformedFrame // control bytes are not valid in replies
			}
		}
		k := min(c.need-len(c.prefix), len(b))
		c.prefix = append(c.prefix, b[:k]...)
		b = b[k:]
		if len(c.prefix) < c.need {
			return nil
		}
		l := decodeAPILength(c.prefix)
		c.prefix = c.prefix[:0]
		if l > maxAPIWordBytes {
			return ErrResponseTooLarge
		}
		c.skip = l
	}
	return nil
}

func decodeAPILength(p []byte) int {
	switch len(p) {
	case 1:
		return int(p[0])
	case 2:
		return int(p[0]&0x3F)<<8 | int(p[1])
	case 3:
		return int(p[0]&0x1F)<<16 | int(p[1])<<8 | int(p[2])
	case 4:
		return int(p[0]&0x0F)<<24 | int(p[1])<<16 | int(p[2])<<8 | int(p[3])
	default:
		return int(uint32(p[1])<<24 | uint32(p[2])<<16 | uint32(p[3])<<8 | uint32(p[4]))
	}
}
