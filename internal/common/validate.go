package common

import "fmt"

// ValidateRule - 공통 규칙 검증 로직
func ValidateRule(rule map[string]interface{}) []string {
	var errors []string

	// name 필수
	name, _ := rule["name"].(string)
	if name == "" {
		errors = append(errors, "name 필드 필수")
	}

	// match 필수
	match, ok := rule["match"].(map[string]interface{})
	if !ok {
		errors = append(errors, "match 필드 필수")
		return errors
	}

	// match.msgId 또는 match.conditions 필요
	msgId, _ := match["msgId"].(string)
	conditions, _ := match["conditions"].([]interface{})
	if msgId == "" && len(conditions) == 0 {
		errors = append(errors, "match.msgId 또는 match.conditions 필요")
	}

	// conditions 검증
	for i, cond := range conditions {
		c, ok := cond.(map[string]interface{})
		if !ok {
			errors = append(errors, "conditions[%d]: 유효하지 않은 형식")
			continue
		}
		field, _ := c["field"].(string)
		op, _ := c["op"].(string)
		if field == "" {
			errors = append(errors, errorf("conditions[%d]: field 필수", i))
		}
		if op == "" {
			errors = append(errors, errorf("conditions[%d]: op 필수", i))
		} else if !isValidOp(op) {
			errors = append(errors, errorf("conditions[%d]: 유효하지 않은 op '%s'", i, op))
		}
	}

	// aggregate 검증 (있는 경우)
	if agg, ok := rule["aggregate"].(map[string]interface{}); ok {
		fn, _ := agg["function"].(string)
		if fn != "" && !isValidAggFunc(fn) {
			errors = append(errors, errorf("aggregate.function: 유효하지 않은 함수 '%s'", fn))
		}
	}

	return errors
}

func isValidOp(op string) bool {
	valid := map[string]bool{
		"eq": true, "neq": true, "gt": true, "gte": true, "lt": true, "lte": true,
		"in": true, "like": true, "regex": true, "exists": true, "not_exists": true,
	}
	return valid[op]
}

func isValidAggFunc(fn string) bool {
	valid := map[string]bool{
		"count": true, "sum": true, "avg": true, "min": true, "max": true, "count_distinct": true,
	}
	return valid[fn]
}

func errorf(format string, args ...interface{}) string {
	return fmt.Sprintf(format, args...)
}
