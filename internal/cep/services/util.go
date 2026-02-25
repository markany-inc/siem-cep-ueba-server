package services

import (
	"fmt"
	"strconv"
	"strings"
)

func toString(v interface{}, def string) string {
	if s, ok := v.(string); ok {
		return s
	}
	return def
}

func toInt(v interface{}) int {
	switch x := v.(type) {
	case int:
		return x
	case float64:
		return int(x)
	case string:
		if i, err := strconv.Atoi(x); err == nil {
			return i
		}
	}
	return 0
}

func toMap(v interface{}) map[string]interface{} {
	if m, ok := v.(map[string]interface{}); ok {
		return m
	}
	return nil
}

func toSlice(v interface{}) []map[string]interface{} {
	if arr, ok := v.([]interface{}); ok {
		result := make([]map[string]interface{}, 0, len(arr))
		for _, item := range arr {
			if m, ok := item.(map[string]interface{}); ok {
				result = append(result, m)
			}
		}
		return result
	}
	return nil
}

func toStringSlice(v interface{}, def []string) []string {
	if arr, ok := v.([]interface{}); ok {
		result := make([]string, 0, len(arr))
		for _, item := range arr {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return def
}

func getNestedVal(m map[string]interface{}, key string, def string) interface{} {
	if v, ok := m[key]; ok {
		return v
	}
	return def
}

func getNestedInt(m map[string]interface{}, key1, key2 string, def int) int {
	if sub, ok := m[key1].(map[string]interface{}); ok {
		if v, ok := sub[key2]; ok {
			return toInt(v)
		}
	}
	return def
}

func escapeSQLValue(s string) string {
	s = strings.ReplaceAll(s, "'", "''")
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, ";", "")
	s = strings.ReplaceAll(s, "--", "")
	return s
}

// cefField: 변환 토픽에서는 모든 필드가 cefExtensions MAP에 직접 존재
func cefField(field string) string {
	return fmt.Sprintf("cefExtensions['%s']", field)
}

func addSpaces(s string) string {
	var result []byte
	for i, c := range s {
		if i > 0 && c >= 'A' && c <= 'Z' {
			result = append(result, ' ')
		}
		result = append(result, byte(c))
	}
	return string(result)
}
