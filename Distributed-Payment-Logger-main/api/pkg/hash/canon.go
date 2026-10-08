package hash

import (
	"bytes"
	"encoding/json"
	"sort"
)

// CanonicalizeJSON returns a canonical JSON string with sorted keys for stable hashing.
// Input can be raw JSON bytes (preferred) or any Go value already marshaled.
func CanonicalizeJSON(raw []byte) (string, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", err
	}
	buf := &bytes.Buffer{}
	encodeCanonical(buf, v)
	return buf.String(), nil
}

func encodeCanonical(buf *bytes.Buffer, v any) {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			buf.WriteByte('"')
			buf.WriteString(escape(k))
			buf.WriteByte('"')
			buf.WriteByte(':')
			encodeCanonical(buf, x[k])
		}
		buf.WriteByte('}')
	case []any:
		buf.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				buf.WriteByte(',')
			}
			encodeCanonical(buf, e)
		}
		buf.WriteByte(']')
	case string:
		b, _ := json.Marshal(x)
		buf.Write(b)
	case float64, bool, nil:
		b, _ := json.Marshal(x)
		buf.Write(b)
	default:
		b, _ := json.Marshal(x)
		buf.Write(b)
	}
}

func escape(s string) string { b, _ := json.Marshal(s); return string(b[1 : len(b)-1]) }
