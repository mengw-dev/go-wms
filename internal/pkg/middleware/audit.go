package middleware

import (
	"encoding/json"
	"net/url"
	"strings"
	"unicode/utf8"
)

const auditParamLimit = 2048

var sensitiveKeys = []string{
	"password",
	"passwd",
	"pwd",
	"token",
	"secret",
	"authorization",
	"cookie",
	"apikey",
	"api_key",
}

func sanitizeOperLogParams(contentType, body string) string {
	contentType = strings.ToLower(contentType)
	if strings.Contains(contentType, "application/json") || strings.HasPrefix(strings.TrimSpace(body), "{") {
		var value any
		if err := json.Unmarshal([]byte(body), &value); err == nil {
			value = redactSensitive(value)
			if encoded, err := json.Marshal(value); err == nil {
				return truncateUTF8(string(encoded), auditParamLimit)
			}
		}
	}
	return truncateUTF8(redactRawParams(body), auditParamLimit)
}

func redactSensitive(value any) any {
	switch current := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(current))
		for key, item := range current {
			if isSensitiveKey(key) {
				out[key] = "[REDACTED]"
				continue
			}
			out[key] = redactSensitive(item)
		}
		return out
	case []any:
		out := make([]any, len(current))
		for i, item := range current {
			out[i] = redactSensitive(item)
		}
		return out
	default:
		return value
	}
}

func redactRawParams(raw string) string {
	values, err := url.ParseQuery(raw)
	if err != nil {
		return raw
	}
	for key := range values {
		if isSensitiveKey(key) {
			values[key] = []string{"[REDACTED]"}
		}
	}
	return values.Encode()
}

func isSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.NewReplacer("-", "", "_", "", ".", "").Replace(key))
	for _, sensitive := range sensitiveKeys {
		if strings.Contains(normalized, sensitive) {
			return true
		}
	}
	return false
}

func truncateUTF8(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for !utf8.ValidString(value) && len(value) > 0 {
		value = value[:len(value)-1]
	}
	return value
}
