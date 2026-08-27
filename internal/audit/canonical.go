package audit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
)

var canonicalCache struct {
	key   string
	value []byte
}

func CanonicalJSON(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	key := string(raw)
	if canonicalCache.key == key {
		return append([]byte(nil), canonicalCache.value...), nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	var result bytes.Buffer
	if err := writeCanonical(&result, decoded); err != nil {
		return nil, err
	}
	canonical := append([]byte(nil), result.Bytes()...)
	canonicalCache.key = key
	canonicalCache.value = canonical
	return append([]byte(nil), canonical...), nil
}

func writeCanonical(buffer *bytes.Buffer, value any) error {
	switch typed := value.(type) {
	case nil:
		buffer.WriteString("null")
	case bool:
		buffer.WriteString(strconv.FormatBool(typed))
	case string:
		encoded, _ := json.Marshal(typed)
		buffer.Write(encoded)
	case json.Number:
		if _, err := strconv.ParseFloat(typed.String(), 64); err != nil {
			return fmt.Errorf("非法数字 %q", typed)
		}
		buffer.WriteString(typed.String())
	case []any:
		buffer.WriteByte('[')
		for index, item := range typed {
			if index > 0 {
				buffer.WriteByte(',')
			}
			if err := writeCanonical(buffer, item); err != nil {
				return err
			}
		}
		buffer.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		buffer.WriteByte('{')
		for index, key := range keys {
			if index > 0 {
				buffer.WriteByte(',')
			}
			encoded, _ := json.Marshal(key)
			buffer.Write(encoded)
			buffer.WriteByte(':')
			if err := writeCanonical(buffer, typed[key]); err != nil {
				return err
			}
		}
		buffer.WriteByte('}')
	default:
		return fmt.Errorf("不支持的规范 JSON 类型 %T", value)
	}
	return nil
}
