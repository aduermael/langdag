package langgraph

import (
	"encoding/binary"
	"fmt"
	"math"
)

// decodeMsgpack decodes a msgpack-encoded byte slice into a Go value.
// Supports: nil, bool, int types, float32/float64, str, bin, array, map.
// Returns interface{} which may be:
//   - nil
//   - bool
//   - int64
//   - uint64
//   - float64
//   - string
//   - []byte
//   - []interface{}
//   - map[string]interface{}  (when all keys are strings)
//   - map[interface{}]interface{} (when keys are mixed types)
func decodeMsgpack(data []byte) (interface{}, error) {
	val, _, err := decodeMsgpackValue(data, 0)
	return val, err
}

// decodeMsgpackValue decodes a single msgpack value starting at offset.
// Returns the decoded value and the next offset.
func decodeMsgpackValue(data []byte, offset int) (interface{}, int, error) {
	if offset >= len(data) {
		return nil, offset, fmt.Errorf("msgpack: unexpected end of data at offset %d", offset)
	}

	b := data[offset]
	offset++

	switch {
	// Positive fixint: 0xxxxxxx (0x00–0x7f)
	case b <= 0x7f:
		return uint64(b), offset, nil

	// Fixmap: 1000xxxx (0x80–0x8f)
	case b >= 0x80 && b <= 0x8f:
		n := int(b & 0x0f)
		return decodeMsgpackMap(data, offset, n)

	// Fixarray: 1001xxxx (0x90–0x9f)
	case b >= 0x90 && b <= 0x9f:
		n := int(b & 0x0f)
		return decodeMsgpackArray(data, offset, n)

	// Fixstr: 101xxxxx (0xa0–0xbf)
	case b >= 0xa0 && b <= 0xbf:
		n := int(b & 0x1f)
		return decodeMsgpackStr(data, offset, n)

	// Negative fixint: 111xxxxx (0xe0–0xff)
	case b >= 0xe0:
		return int64(int8(b)), offset, nil

	// nil
	case b == 0xc0:
		return nil, offset, nil

	// false
	case b == 0xc2:
		return false, offset, nil

	// true
	case b == 0xc3:
		return true, offset, nil

	// bin 8
	case b == 0xc4:
		if offset+1 > len(data) {
			return nil, offset, fmt.Errorf("msgpack: bin8 length truncated")
		}
		n := int(data[offset])
		offset++
		return decodeMsgpackBin(data, offset, n)

	// bin 16
	case b == 0xc5:
		if offset+2 > len(data) {
			return nil, offset, fmt.Errorf("msgpack: bin16 length truncated")
		}
		n := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		offset += 2
		return decodeMsgpackBin(data, offset, n)

	// bin 32
	case b == 0xc6:
		if offset+4 > len(data) {
			return nil, offset, fmt.Errorf("msgpack: bin32 length truncated")
		}
		n := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		offset += 4
		return decodeMsgpackBin(data, offset, n)

	// ext 8
	case b == 0xc7:
		if offset+1 > len(data) {
			return nil, offset, fmt.Errorf("msgpack: ext8 length truncated")
		}
		n := int(data[offset])
		offset++
		// skip type byte + data
		if offset+1+n > len(data) {
			return nil, offset, fmt.Errorf("msgpack: ext8 data truncated")
		}
		offset += 1 + n
		return nil, offset, nil

	// ext 16
	case b == 0xc8:
		if offset+2 > len(data) {
			return nil, offset, fmt.Errorf("msgpack: ext16 length truncated")
		}
		n := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		offset += 2
		if offset+1+n > len(data) {
			return nil, offset, fmt.Errorf("msgpack: ext16 data truncated")
		}
		offset += 1 + n
		return nil, offset, nil

	// ext 32
	case b == 0xc9:
		if offset+4 > len(data) {
			return nil, offset, fmt.Errorf("msgpack: ext32 length truncated")
		}
		n := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		offset += 4
		if offset+1+n > len(data) {
			return nil, offset, fmt.Errorf("msgpack: ext32 data truncated")
		}
		offset += 1 + n
		return nil, offset, nil

	// float 32
	case b == 0xca:
		if offset+4 > len(data) {
			return nil, offset, fmt.Errorf("msgpack: float32 truncated")
		}
		bits := binary.BigEndian.Uint32(data[offset : offset+4])
		offset += 4
		return float64(math.Float32frombits(bits)), offset, nil

	// float 64
	case b == 0xcb:
		if offset+8 > len(data) {
			return nil, offset, fmt.Errorf("msgpack: float64 truncated")
		}
		bits := binary.BigEndian.Uint64(data[offset : offset+8])
		offset += 8
		return math.Float64frombits(bits), offset, nil

	// uint 8
	case b == 0xcc:
		if offset+1 > len(data) {
			return nil, offset, fmt.Errorf("msgpack: uint8 truncated")
		}
		v := uint64(data[offset])
		offset++
		return v, offset, nil

	// uint 16
	case b == 0xcd:
		if offset+2 > len(data) {
			return nil, offset, fmt.Errorf("msgpack: uint16 truncated")
		}
		v := uint64(binary.BigEndian.Uint16(data[offset : offset+2]))
		offset += 2
		return v, offset, nil

	// uint 32
	case b == 0xce:
		if offset+4 > len(data) {
			return nil, offset, fmt.Errorf("msgpack: uint32 truncated")
		}
		v := uint64(binary.BigEndian.Uint32(data[offset : offset+4]))
		offset += 4
		return v, offset, nil

	// uint 64
	case b == 0xcf:
		if offset+8 > len(data) {
			return nil, offset, fmt.Errorf("msgpack: uint64 truncated")
		}
		v := binary.BigEndian.Uint64(data[offset : offset+8])
		offset += 8
		return v, offset, nil

	// int 8
	case b == 0xd0:
		if offset+1 > len(data) {
			return nil, offset, fmt.Errorf("msgpack: int8 truncated")
		}
		v := int64(int8(data[offset]))
		offset++
		return v, offset, nil

	// int 16
	case b == 0xd1:
		if offset+2 > len(data) {
			return nil, offset, fmt.Errorf("msgpack: int16 truncated")
		}
		v := int64(int16(binary.BigEndian.Uint16(data[offset : offset+2])))
		offset += 2
		return v, offset, nil

	// int 32
	case b == 0xd2:
		if offset+4 > len(data) {
			return nil, offset, fmt.Errorf("msgpack: int32 truncated")
		}
		v := int64(int32(binary.BigEndian.Uint32(data[offset : offset+4])))
		offset += 4
		return v, offset, nil

	// int 64
	case b == 0xd3:
		if offset+8 > len(data) {
			return nil, offset, fmt.Errorf("msgpack: int64 truncated")
		}
		v := int64(binary.BigEndian.Uint64(data[offset : offset+8]))
		offset += 8
		return v, offset, nil

	// fixext 1
	case b == 0xd4:
		if offset+2 > len(data) {
			return nil, offset, fmt.Errorf("msgpack: fixext1 truncated")
		}
		offset += 2
		return nil, offset, nil

	// fixext 2
	case b == 0xd5:
		if offset+3 > len(data) {
			return nil, offset, fmt.Errorf("msgpack: fixext2 truncated")
		}
		offset += 3
		return nil, offset, nil

	// fixext 4
	case b == 0xd6:
		if offset+5 > len(data) {
			return nil, offset, fmt.Errorf("msgpack: fixext4 truncated")
		}
		offset += 5
		return nil, offset, nil

	// fixext 8
	case b == 0xd7:
		if offset+9 > len(data) {
			return nil, offset, fmt.Errorf("msgpack: fixext8 truncated")
		}
		offset += 9
		return nil, offset, nil

	// fixext 16
	case b == 0xd8:
		if offset+17 > len(data) {
			return nil, offset, fmt.Errorf("msgpack: fixext16 truncated")
		}
		offset += 17
		return nil, offset, nil

	// str 8
	case b == 0xd9:
		if offset+1 > len(data) {
			return nil, offset, fmt.Errorf("msgpack: str8 length truncated")
		}
		n := int(data[offset])
		offset++
		return decodeMsgpackStr(data, offset, n)

	// str 16
	case b == 0xda:
		if offset+2 > len(data) {
			return nil, offset, fmt.Errorf("msgpack: str16 length truncated")
		}
		n := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		offset += 2
		return decodeMsgpackStr(data, offset, n)

	// str 32
	case b == 0xdb:
		if offset+4 > len(data) {
			return nil, offset, fmt.Errorf("msgpack: str32 length truncated")
		}
		n := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		offset += 4
		return decodeMsgpackStr(data, offset, n)

	// array 16
	case b == 0xdc:
		if offset+2 > len(data) {
			return nil, offset, fmt.Errorf("msgpack: array16 length truncated")
		}
		n := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		offset += 2
		return decodeMsgpackArray(data, offset, n)

	// array 32
	case b == 0xdd:
		if offset+4 > len(data) {
			return nil, offset, fmt.Errorf("msgpack: array32 length truncated")
		}
		n := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		offset += 4
		return decodeMsgpackArray(data, offset, n)

	// map 16
	case b == 0xde:
		if offset+2 > len(data) {
			return nil, offset, fmt.Errorf("msgpack: map16 length truncated")
		}
		n := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		offset += 2
		return decodeMsgpackMap(data, offset, n)

	// map 32
	case b == 0xdf:
		if offset+4 > len(data) {
			return nil, offset, fmt.Errorf("msgpack: map32 length truncated")
		}
		n := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		offset += 4
		return decodeMsgpackMap(data, offset, n)

	default:
		return nil, offset, fmt.Errorf("msgpack: unknown type byte 0x%02x at offset %d", b, offset-1)
	}
}

func decodeMsgpackStr(data []byte, offset, n int) (interface{}, int, error) {
	if offset+n > len(data) {
		return nil, offset, fmt.Errorf("msgpack: string data truncated (need %d bytes, have %d)", n, len(data)-offset)
	}
	s := string(data[offset : offset+n])
	return s, offset + n, nil
}

func decodeMsgpackBin(data []byte, offset, n int) (interface{}, int, error) {
	if offset+n > len(data) {
		return nil, offset, fmt.Errorf("msgpack: binary data truncated (need %d bytes, have %d)", n, len(data)-offset)
	}
	b := make([]byte, n)
	copy(b, data[offset:offset+n])
	return b, offset + n, nil
}

func decodeMsgpackArray(data []byte, offset, n int) (interface{}, int, error) {
	arr := make([]interface{}, 0, n)
	for i := 0; i < n; i++ {
		val, newOffset, err := decodeMsgpackValue(data, offset)
		if err != nil {
			return nil, newOffset, fmt.Errorf("msgpack: array element %d: %w", i, err)
		}
		arr = append(arr, val)
		offset = newOffset
	}
	return arr, offset, nil
}

func decodeMsgpackMap(data []byte, offset, n int) (interface{}, int, error) {
	// First try to decode as map[string]interface{} (common case for LangChain objects)
	m := make(map[string]interface{}, n)
	for i := 0; i < n; i++ {
		keyVal, newOffset, err := decodeMsgpackValue(data, offset)
		if err != nil {
			return nil, newOffset, fmt.Errorf("msgpack: map key %d: %w", i, err)
		}
		offset = newOffset

		val, newOffset, err := decodeMsgpackValue(data, offset)
		if err != nil {
			return nil, newOffset, fmt.Errorf("msgpack: map value %d: %w", i, err)
		}
		offset = newOffset

		// Convert key to string
		switch k := keyVal.(type) {
		case string:
			m[k] = val
		default:
			// Non-string key: fall back to string representation
			m[fmt.Sprintf("%v", k)] = val
		}
	}
	return m, offset, nil
}

// msgpackGetString safely extracts a string value from a decoded msgpack map.
func msgpackGetString(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// msgpackGetInt safely extracts an integer value from a decoded msgpack map.
func msgpackGetInt(m map[string]interface{}, key string) int {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int64:
		return int(n)
	case uint64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

// msgpackGetMap safely extracts a nested map from a decoded msgpack map.
func msgpackGetMap(m map[string]interface{}, key string) map[string]interface{} {
	v, ok := m[key]
	if !ok {
		return nil
	}
	sub, _ := v.(map[string]interface{})
	return sub
}

// msgpackGetSlice safely extracts a slice from a decoded msgpack map.
func msgpackGetSlice(m map[string]interface{}, key string) []interface{} {
	v, ok := m[key]
	if !ok {
		return nil
	}
	s, _ := v.([]interface{})
	return s
}
