package common

import "strings"

// ExpandCEFLabels: *Label 접미사 키를 찾아 label 이름으로 값을 매핑
// 예: cs1Label="Config Type", cs1="ipchange" → ConfigType="ipchange"
func ExpandCEFLabels(ext map[string]interface{}) {
	for k, v := range ext {
		if !strings.HasSuffix(k, "Label") {
			continue
		}
		label, _ := v.(string)
		if label == "" {
			continue
		}
		valueKey := strings.TrimSuffix(k, "Label")
		val, _ := ext[valueKey].(string)
		if val != "" {
			ext[strings.ReplaceAll(label, " ", "")] = val
		}
	}
}
