package decoder

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"unicode"
)

const (
	Array = 'l'
	Int   = 'i'
	Dict  = 'd'
	End   = 'e'
)

type Decoder struct {
	r *bufio.Reader
}

func New(bencoded []byte) *Decoder {
	return &Decoder{
		r: bufio.NewReader(bytes.NewReader(bencoded)),
	}
}

func (d *Decoder) decodeInt() (int, error) {
	intBytes, err := d.r.ReadBytes(End)

	if err != nil {
		return 0, fmt.Errorf("invalid int format")
	}

	intBytes = intBytes[:len(intBytes)-1]

	numStr := string(intBytes)

	if len(numStr) > 1 && numStr[0] == '0' {
		return 0, fmt.Errorf("invalid int format")
	}

	n, err := strconv.Atoi(numStr)

	if err != nil {
		return 0, fmt.Errorf("invalid int format")
	}

	return n, nil
}

func (d *Decoder) decodeString(asBytes bool) (any, error) {
	lenBytes, err := d.r.ReadBytes(':')

	if err != nil {
		return nil, err
	}

	lenBytes = lenBytes[:len(lenBytes)-1]

	length, err := strconv.Atoi(string(lenBytes))

	if err != nil {
		return nil, fmt.Errorf("invalid string format")
	}

	str := make([]byte, length)

	_, err = d.r.Read(str)

	if err != nil {
		return nil, fmt.Errorf("failed to read string: %v", err)
	}

	if asBytes {
		return str, nil
	}

	return string(str), nil
}

func (d *Decoder) decodeArray() ([]any, error) {
	list := []any{}

	for {
		b, err := d.r.ReadByte()

		if err != nil {
			return nil, fmt.Errorf("failed to read byte: %v", err)
		}

		if b == End {
			break
		}

		err = d.r.UnreadByte()

		if err != nil {
			return nil, fmt.Errorf("failed to unread byte: %v", err)
		}

		v, err := d.Decode()

		if err != nil {
			return nil, fmt.Errorf("failed to decode: %v", err)
		}

		list = append(list, v)
	}

	return list, nil
}

func (d *Decoder) decodeDict() (map[string]any, error) {
	m := make(map[string]any)

	for {
		b, err := d.r.ReadByte()

		if err != nil {
			return nil, fmt.Errorf("failed to read byte: %v", err)
		}

		if b == End {
			break
		}

		err = d.r.UnreadByte()

		if err != nil {
			return nil, fmt.Errorf("failed to unread byte: %v", err)
		}

		k, err := d.Decode()

		if err != nil {
			return nil, fmt.Errorf("failed to decode dict key: %v", err)
		}

		key, ok := k.(string)

		if !ok {
			return nil, fmt.Errorf("dict key must be a string (got %T instead)", key)
		}

		v, err := d.Decode()

		if err != nil {
			return nil, fmt.Errorf("failed to decode dict value: %v", err)
		}

		m[key] = v
	}

	return m, nil
}

func (d *Decoder) Decode() (any, error) {
	b, err := d.r.ReadByte()

	if err != nil {
		return nil, fmt.Errorf("failed to read byte: %v", err)
	}

	switch {
	case unicode.IsDigit(rune(b)):
		err := d.r.UnreadByte()

		if err != nil {
			return nil, fmt.Errorf("failed to unread byte: %v", err)
		}

		return d.decodeString(false)
	case b == Int:
		return d.decodeInt()
	case b == Array:
		return d.decodeArray()
	case b == Dict:
		return d.decodeDict()
	default:
		return nil, fmt.Errorf("unknown format")
	}
}
