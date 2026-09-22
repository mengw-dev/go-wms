package typex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

// Int64List 在 JSON 中使用字符串数组表示 int64 ID，避免 JavaScript 精度丢失。
// 为兼容旧客户端，Unmarshal 同时接受字符串和数字数组。
type Int64List []int64

func (ids Int64List) MarshalJSON() ([]byte, error) {
	values := make([]string, len(ids))
	for i, id := range ids {
		values[i] = strconv.FormatInt(id, 10)
	}
	return json.Marshal(values)
}

func (ids *Int64List) UnmarshalJSON(raw []byte) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		*ids = nil
		return nil
	}

	var stringsValue []string
	if err := json.Unmarshal(raw, &stringsValue); err == nil {
		out := make([]int64, len(stringsValue))
		for i, value := range stringsValue {
			id, parseErr := strconv.ParseInt(value, 10, 64)
			if parseErr != nil || id <= 0 {
				return fmt.Errorf("invalid int64 id %q", value)
			}
			out[i] = id
		}
		*ids = out
		return nil
	}

	var numbersValue []int64
	if err := json.Unmarshal(raw, &numbersValue); err == nil {
		for _, id := range numbersValue {
			if id <= 0 {
				return fmt.Errorf("invalid int64 id %d", id)
			}
		}
		*ids = numbersValue
		return nil
	}

	return fmt.Errorf("ids must be an array of strings or numbers")
}
