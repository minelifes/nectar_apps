package webauthn

import (
	"encoding/binary"
	"errors"
	"math"
)

// A minimal CBOR decoder (RFC 8949) for what WebAuthn needs: attestation objects and COSE keys —
// unsigned/negative integers, byte and text strings, arrays, maps, booleans, null. No tags, no floats,
// no indefinite lengths (authenticators never use them here).

var errCBOR = errors.New("webauthn: malformed CBOR")

type cborReader struct {
	b   []byte
	pos int
}

func (r *cborReader) byte() (byte, error) {
	if r.pos >= len(r.b) {
		return 0, errCBOR
	}
	c := r.b[r.pos]
	r.pos++
	return c, nil
}

func (r *cborReader) take(n uint64) ([]byte, error) {
	if n > uint64(len(r.b)-r.pos) {
		return nil, errCBOR
	}
	out := r.b[r.pos : r.pos+int(n)]
	r.pos += int(n)
	return out, nil
}

func (r *cborReader) head() (major byte, arg uint64, err error) {
	c, err := r.byte()
	if err != nil {
		return 0, 0, err
	}
	major, info := c>>5, c&0x1f
	switch {
	case info < 24:
		return major, uint64(info), nil
	case info == 24:
		b, err := r.take(1)
		if err != nil {
			return 0, 0, err
		}
		return major, uint64(b[0]), nil
	case info == 25:
		b, err := r.take(2)
		if err != nil {
			return 0, 0, err
		}
		return major, uint64(binary.BigEndian.Uint16(b)), nil
	case info == 26:
		b, err := r.take(4)
		if err != nil {
			return 0, 0, err
		}
		return major, uint64(binary.BigEndian.Uint32(b)), nil
	case info == 27:
		b, err := r.take(8)
		if err != nil {
			return 0, 0, err
		}
		return major, binary.BigEndian.Uint64(b), nil
	}
	return 0, 0, errCBOR // indefinite lengths and reserved values
}

// value decodes one item: uint64/int64 → int64, []byte, string, []any, map[any]any, bool, nil.
func (r *cborReader) value(depth int) (any, error) {
	if depth > 16 {
		return nil, errCBOR
	}
	major, arg, err := r.head()
	if err != nil {
		return nil, err
	}
	switch major {
	case 0:
		if arg > math.MaxInt64 {
			return nil, errCBOR
		}
		return int64(arg), nil
	case 1:
		if arg > math.MaxInt64 {
			return nil, errCBOR
		}
		return -1 - int64(arg), nil
	case 2:
		return r.take(arg)
	case 3:
		b, err := r.take(arg)
		return string(b), err
	case 4:
		if arg > 1024 {
			return nil, errCBOR
		}
		out := make([]any, 0, arg)
		for i := uint64(0); i < arg; i++ {
			v, err := r.value(depth + 1)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	case 5:
		if arg > 1024 {
			return nil, errCBOR
		}
		out := make(map[any]any, arg)
		for i := uint64(0); i < arg; i++ {
			k, err := r.value(depth + 1)
			if err != nil {
				return nil, err
			}
			if _, ok := k.([]byte); ok {
				return nil, errCBOR // unhashable keys never occur in WebAuthn
			}
			v, err := r.value(depth + 1)
			if err != nil {
				return nil, err
			}
			out[k] = v
		}
		return out, nil
	case 7:
		switch arg {
		case 20:
			return false, nil
		case 21:
			return true, nil
		case 22, 23:
			return nil, nil
		}
	}
	return nil, errCBOR
}

// decodeCBOR decodes one item and returns the number of bytes it used.
func decodeCBOR(b []byte) (any, int, error) {
	r := &cborReader{b: b}
	v, err := r.value(0)
	return v, r.pos, err
}
