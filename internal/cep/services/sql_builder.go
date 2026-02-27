package services

import (
	"fmt"
	"strings"
)

// CEP 규칙 JSON → Flink SQL 변환기
// ─────────────────────────────────────────────────────────
// 지원 규칙 타입:
//   1. 단일 패턴   - 조건 매칭 즉시 탐지
//   2. 집계        - TUMBLE 윈도우 내 N회 이상
//   3. 순차        - MATCH_RECOGNIZE (A→B 순서)
//   4. 동시        - 윈도우 내 AND/OR 복합 조건

import (
	"regexp"
	"sort"
)

// ── 숫자 포맷: float64 정수면 소수점 없이 출력 ──
func FmtNum(v interface{}) string {
	if f, ok := v.(float64); ok {
		if f == float64(int64(f)) {
			return fmt.Sprintf("%d", int64(f))
		}
		return fmt.Sprintf("%g", f)
	}
	return fmt.Sprintf("%v", v)
}

// ── 단일 조건 → WHERE 절 조각 ──

func BuildConditionClause(c map[string]interface{}) string {
	field, _ := c["field"].(string)
	op, _ := c["op"].(string)
	val := c["value"]

	// 가상 필드
	if field == "hour" || field == "time" {
		field = "HOUR(proctime)"
	} else if field == "dayOfWeek" {
		field = "DAYOFWEEK(proctime)"
	} else if !isBaseField(field) {
		// CEF extension: label 이름이면 cefExtensions MAP에서 동적 매칭
		// csN/cnN 키는 직접 접근, 그 외는 label 역매핑 서브쿼리
		field = cefField(field)
	}

	switch op {
	case "eq":
		switch v := val.(type) {
		case bool:
			return fmt.Sprintf("%s = '%t'", field, v)
		case string:
			return fmt.Sprintf("%s = '%s'", field, escapeSQLValue(v))
		default:
			return fmt.Sprintf("%s = '%s'", field, FmtNum(v))
		}
	case "neq":
		if s, ok := val.(string); ok {
			return fmt.Sprintf("%s != '%s'", field, escapeSQLValue(s))
		}
		return fmt.Sprintf("%s != '%s'", field, FmtNum(val))
	case "gt":
		return fmt.Sprintf("CAST(%s AS DOUBLE) > %s", field, FmtNum(val))
	case "gte":
		return fmt.Sprintf("CAST(%s AS DOUBLE) >= %s", field, FmtNum(val))
	case "lt":
		return fmt.Sprintf("CAST(%s AS DOUBLE) < %s", field, FmtNum(val))
	case "lte":
		return fmt.Sprintf("CAST(%s AS DOUBLE) <= %s", field, FmtNum(val))
	case "in":
		if arr, ok := val.([]interface{}); ok {
			parts := make([]string, len(arr))
			for i, v := range arr {
				if s, ok := v.(string); ok {
					parts[i] = fmt.Sprintf("'%s'", escapeSQLValue(s))
				} else {
					parts[i] = fmt.Sprintf("'%s'", FmtNum(v))
				}
			}
			return fmt.Sprintf("%s IN (%s)", field, strings.Join(parts, ","))
		}
	case "like":
		return fmt.Sprintf("%s LIKE '%v'", field, escapeSQLValue(fmt.Sprintf("%v", val)))
	case "regex":
		return fmt.Sprintf("REGEXP(%s, '%v')", field, escapeSQLValue(fmt.Sprintf("%v", val)))
	case "time_range":
		start := toInt(c["start"])
		end := toInt(c["end"])
		if start > end {
			return fmt.Sprintf("(HOUR(proctime) >= %d OR HOUR(proctime) < %d)", start, end)
		}
		return fmt.Sprintf("(HOUR(proctime) >= %d AND HOUR(proctime) < %d)", start, end)
	}
	return ""
}

// 기본 필드 (테이블에 직접 정의된 필드)
func isBaseField(field string) bool {
	base := map[string]bool{
		"msgId": true, "hostname": true, "userId": true,
		"userName": true, "userIp": true,
	}
	return base[field]
}

// ── match 객체 → WHERE 절 전체 ──
func BuildMatchWhere(match map[string]interface{}) string {
	var clauses []string

	// msgId 필터
	if msgId, ok := match["msgId"].(string); ok && msgId != "" {
		clauses = append(clauses, fmt.Sprintf("msgId = '%s'", escapeSQLValue(msgId)))
	}

	// 추가 조건들 (AND/OR 결합)
	condLogic := "AND"
	if l, ok := match["logic"].(string); ok {
		condLogic = strings.ToUpper(l)
	}

	var condClauses []string
	if conditions, ok := match["conditions"].([]interface{}); ok {
		for _, raw := range conditions {
			if c, ok := raw.(map[string]interface{}); ok {
				if clause := BuildConditionClause(c); clause != "" {
					condClauses = append(condClauses, clause)
				}
			}
		}
	}

	if len(condClauses) == 1 {
		clauses = append(clauses, condClauses[0])
	} else if len(condClauses) > 1 {
		clauses = append(clauses, "("+strings.Join(condClauses, " "+condLogic+" ")+")")
	}

	if len(clauses) == 0 {
		return "1=1"
	}
	return strings.Join(clauses, " AND ")
}

// ── 시간 윈도우 → Flink INTERVAL ('5m' → INTERVAL '5' MINUTE) ──
func ParseWindow(window interface{}) string {
	s := fmt.Sprintf("%v", window)
	if s == "" || s == "<nil>" {
		return "INTERVAL '5' MINUTE"
	}
	re := regexp.MustCompile(`^(\d+)([smh])$`)
	m := re.FindStringSubmatch(s)
	if m != nil {
		unitMap := map[string]string{"s": "SECOND", "m": "MINUTE", "h": "HOUR"}
		return fmt.Sprintf("INTERVAL '%s' %s", m[1], unitMap[m[2]])
	}
	return fmt.Sprintf("INTERVAL '%s' MINUTE", s)
}

// ── 통합 규칙 JSON → Flink SQL SELECT 쿼리 ──
func BuildSQLFromRule(rule map[string]interface{}) string {
	patterns := toSlice(rule["patterns"])
	logic := strings.ToUpper(toString(rule["logic"], "AND"))
	within := rule["within"]
	byFields := toStringSlice(rule["by"], []string{"userId"})
	aggregate, _ := rule["aggregate"].(map[string]interface{})

	// 출력 필드: by 필드 + hostname, userIp (alerts 테이블에 필요)
	selectFields := strings.Join(byFields, ", ") + ", hostname, userIp"
	groupFields := strings.Join(byFields, ", ") + ", hostname, userIp"

	// ── 통합 JSON: events[] → patterns[].match 변환 ──
	if len(patterns) == 0 {
		if evts, ok := rule["events"].([]interface{}); ok && len(evts) > 0 {
			for i, raw := range evts {
				if ev, ok := raw.(map[string]interface{}); ok {
					match := map[string]interface{}{}
					if msgId, ok := ev["msgId"]; ok {
						match["msgId"] = msgId
					}
					if conds, ok := ev["conditions"]; ok {
						match["conditions"] = conds
					}
					p := map[string]interface{}{"match": match}
					if len(evts) > 1 {
						p["order"] = float64(i + 1)
					}
					patterns = append(patterns, p)
				}
			}
		}
	}

	// ── 통합 JSON: aggregate.minCount → aggregate.count.min 변환 ──
	if aggregate != nil {
		if mc, ok := aggregate["minCount"]; ok {
			if _, hasCount := aggregate["count"]; !hasCount {
				aggregate["count"] = map[string]interface{}{"min": mc}
			}
		}
	}

	// ── 하위 호환: match만 있으면 단일 패턴으로 변환 ──
	if len(patterns) == 0 {
		if m, ok := rule["match"].(map[string]interface{}); ok {
			patterns = []map[string]interface{}{{"match": m}}
		}
	}

	// ── 하위 호환: conditions 배열만 있으면 변환 ──
	if len(patterns) == 0 {
		if conds, ok := rule["conditions"].([]interface{}); ok {
			match := map[string]interface{}{}
			// msgId를 conditions에서 추출하고 나머지만 유지
			var filtered []interface{}
			for _, raw := range conds {
				if c, ok := raw.(map[string]interface{}); ok {
					if f, _ := c["field"].(string); f == "msgId" {
						match["msgId"] = c["value"]
						continue
					}
				}
				filtered = append(filtered, raw)
			}
			match["conditions"] = filtered
			patterns = []map[string]interface{}{{"match": match}}
		}
	}

	if len(patterns) == 0 {
		return "SELECT * FROM events WHERE 1=0"
	}

	// order 필드 존재 여부 확인
	hasOrder := false
	for _, p := range patterns {
		if _, ok := p["order"]; ok {
			hasOrder = true
			break
		}
	}

	// ═══════ 단일 패턴 ═══════
	if len(patterns) == 1 {
		p := patterns[0]
		match := toMap(p["match"])
		where := BuildMatchWhere(match)
		quant := toMap(p["quantifier"])

		// 집계 조건
		if aggregate != nil {
			interval := ParseWindow(getNestedVal(aggregate, "within", "1h"))
			minCount := getNestedInt(aggregate, "count", "min", 1)
			return fmt.Sprintf(
				"SELECT %s, COUNT(*) as cnt "+
					"FROM events WHERE %s "+
					"GROUP BY TUMBLE(proctime, %s), %s "+
					"HAVING COUNT(*) >= %d", selectFields, where, interval, groupFields, minCount)
		}

		// quantifier 반복 횟수
		if minQ := toInt(quant["min"]); minQ > 1 {
			interval := ParseWindow(within)
			if within == nil {
				interval = ParseWindow("1h")
			}
			return fmt.Sprintf(
				"SELECT %s, COUNT(*) as cnt "+
					"FROM events WHERE %s "+
					"GROUP BY TUMBLE(proctime, %s), %s "+
					"HAVING COUNT(*) >= %d", selectFields, where, interval, groupFields, minQ)
		}

		// 단순 필터
		return fmt.Sprintf("SELECT %s, 1 as cnt FROM events WHERE %s", selectFields, where)
	}

	// ═══════ 순차 패턴 (MATCH_RECOGNIZE) ═══════
	if hasOrder {
		// order 기준 정렬
		var ordered []map[string]interface{}
		for _, p := range patterns {
			if _, ok := p["order"]; ok {
				ordered = append(ordered, p)
			}
		}
		sort.Slice(ordered, func(i, j int) bool {
			return toInt(ordered[i]["order"]) < toInt(ordered[j]["order"])
		})

		var patternParts, defineClauses []string
		for _, p := range ordered {
			pid := fmt.Sprintf("P%d", toInt(p["order"]))
			match := toMap(p["match"])
			quant := toMap(p["quantifier"])
			where := BuildMatchWhere(match)
			defineClauses = append(defineClauses, fmt.Sprintf("%s AS %s", pid, where))

			minQ := toInt(quant["min"])
			if minQ <= 0 {
				minQ = 1
			}
			maxQ := toInt(quant["max"])
			if minQ > 1 {
				if maxQ > 0 {
					patternParts = append(patternParts, fmt.Sprintf("%s{%d,%d}", pid, minQ, maxQ))
				} else {
					patternParts = append(patternParts, fmt.Sprintf("%s{%d,}", pid, minQ))
				}
			} else {
				patternParts = append(patternParts, pid)
			}
		}

		partitionBy := strings.Join(byFields, ", ")
		interval := ParseWindow(within)
		if within == nil {
			interval = ParseWindow("5m")
		}

		return fmt.Sprintf(
			"SELECT * FROM events\nMATCH_RECOGNIZE (\n"+
				"  PARTITION BY %s\n"+
				"  ORDER BY proctime\n"+
				"  MEASURES\n"+
				"    COUNT(*) AS cnt\n"+
				"  ONE ROW PER MATCH\n"+
				"  AFTER MATCH SKIP PAST LAST ROW\n"+
				"  PATTERN (%s) WITHIN %s\n"+
				"  DEFINE\n"+
				"    %s\n)",
			partitionBy,
			strings.Join(patternParts, " "), interval,
			strings.Join(defineClauses, ", "))
	}

	// ═══════ 동시 패턴 (AND/OR) ═══════
	var whereParts []string
	for _, p := range patterns {
		match := toMap(p["match"])
		whereParts = append(whereParts, "("+BuildMatchWhere(match)+")")
	}

	// OR: 어느 하나라도 매칭
	if logic == "OR" {
		return fmt.Sprintf("SELECT %s, 1 as cnt FROM events WHERE %s",
			selectFields, strings.Join(whereParts, " OR "))
	}

	// AND: 윈도우 내 모든 패턴 존재
	interval := ParseWindow(within)
	if within == nil {
		interval = ParseWindow("30m")
	}

	var caseSelects, havingParts []string
	for i, p := range patterns {
		match := toMap(p["match"])
		where := BuildMatchWhere(match)
		caseSelects = append(caseSelects, fmt.Sprintf("MAX(CASE WHEN %s THEN 1 ELSE 0 END) AS p%d", where, i))
		havingParts = append(havingParts, fmt.Sprintf("p%d = 1", i))
	}

	return fmt.Sprintf(
		"SELECT %s, %s, COUNT(*) as cnt "+
			"FROM events WHERE %s "+
			"GROUP BY TUMBLE(proctime, %s), %s "+
			"HAVING %s",
		selectFields, strings.Join(caseSelects, ", "),
		strings.Join(whereParts, " OR "),
		interval, groupFields,
		strings.Join(havingParts, " AND "))
}

// ── 유틸리티 ──







