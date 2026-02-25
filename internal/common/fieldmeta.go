package common

import (
	"time"

	"github.com/labstack/echo/v4"
)



type FieldMetaController struct {
	OS          *OSClient
	IndexPrefix string
}

func NewFieldMetaController(os *OSClient, indexPrefix string) *FieldMetaController {
	return &FieldMetaController{OS: os, IndexPrefix: indexPrefix}
}

// Get - 저장된 field-meta 조회
func (c *FieldMetaController) Get(ctx echo.Context) error {
	docs, _ := c.OS.Search(FieldMetaIndex(c.IndexPrefix), map[string]interface{}{
		"size": 1,
		"sort": []map[string]string{{"migratedAt": "desc"}},
	})
	if len(docs) == 0 {
		return ctx.JSON(404, map[string]string{"error": "field-meta not found"})
	}
	return ctx.JSON(200, docs[0])
}

// Put - field-meta 저장
func (c *FieldMetaController) Put(ctx echo.Context) error {
	var meta map[string]interface{}
	if err := ctx.Bind(&meta); err != nil {
		return ctx.JSON(400, map[string]string{"error": "invalid JSON"})
	}
	meta["migratedAt"] = time.Now().Format(time.RFC3339)
	if err := c.OS.Put(FieldMetaIndex(c.IndexPrefix), "meta-latest", meta); err != nil {
		return ctx.JSON(500, map[string]string{"error": err.Error()})
	}
	c.OS.Refresh(FieldMetaIndex(c.IndexPrefix))
	return ctx.JSON(200, map[string]string{"status": "ok"})
}

// Analyze - 이벤트별 필드 목록 동적 추출
func (c *FieldMetaController) Analyze(ctx echo.Context) error {
	var req struct {
		Events []string `json:"events"`
		Days   int      `json:"days"`
	}
	ctx.Bind(&req)

	// days 파라미터가 있으면 n일치 로그에서 모든 이벤트 분석
	if req.Days > 0 || len(req.Events) == 0 {
		return c.analyzeByDays(ctx, req.Days)
	}

	// events 파라미터가 있으면 특정 이벤트만 분석
	result := make(map[string][]string)
	for _, evt := range req.Events {
		result[evt] = c.analyzeEvent(evt)
	}
	return ctx.JSON(200, result)
}

// analyzeByDays - n일치 로그에서 모든 이벤트 타입과 필드 추출
func (c *FieldMetaController) analyzeByDays(ctx echo.Context, days int) error {
	if days <= 0 {
		days = 7
	}

	// 날짜 범위 쿼리 + 이벤트별 문서 수
	query := map[string]interface{}{
		"size": 0,
		"query": map[string]interface{}{
			"range": map[string]interface{}{
				"@timestamp": map[string]string{
					"gte": "now-" + itoa(days) + "d/d",
				},
			},
		},
		"aggs": map[string]interface{}{
			"msgIds": map[string]interface{}{
				"terms": map[string]interface{}{"field": "msgId.keyword", "size": 100},
			},
		},
	}

	raw, _ := c.OS.SearchRaw(LogsIndexPattern(c.IndexPrefix), query)

	// 프론트엔드 기대 구조: { fields: [...], sampleCount: N }
	events := make(map[string]interface{})
	if aggs, ok := raw["aggregations"].(map[string]interface{}); ok {
		if msgIds, ok := aggs["msgIds"].(map[string]interface{}); ok {
			if buckets, ok := msgIds["buckets"].([]interface{}); ok {
				for _, b := range buckets {
					if bucket, ok := b.(map[string]interface{}); ok {
						key, _ := bucket["key"].(string)
						count, _ := bucket["doc_count"].(float64)
						if key != "" {
							events[key] = map[string]interface{}{
								"fields":      c.analyzeEvent(key),
								"sampleCount": int(count),
							}
						}
					}
				}
			}
		}
	}

	return ctx.JSON(200, map[string]interface{}{
		"events": events,
		"days":   days,
	})
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

// AnalyzeField - 특정 필드 상세 정보
func (c *FieldMetaController) AnalyzeField(ctx echo.Context) error {
	var req struct {
		Event string `json:"event"`
		Field string `json:"field"`
	}
	ctx.Bind(&req)
	return ctx.JSON(200, c.analyzeFieldDetail(req.Event, req.Field))
}

// analyzeEvent - 로그 인덱스에서 해당 이벤트의 CEF extension 필드만 추출
func (c *FieldMetaController) analyzeEvent(msgID string) []string {
	docs, _ := c.OS.Search(LogsIndexPattern(c.IndexPrefix), map[string]interface{}{
		"size":  50,
		"query": map[string]interface{}{"term": map[string]interface{}{"msgId.keyword": msgID}},
		"sort":  []map[string]string{{"@timestamp": "desc"}},
	})

	fieldSet := make(map[string]bool)
	for _, doc := range docs {
		// cefExtensions 내부 필드 추출
		if cef, ok := doc["cefExtensions"].(map[string]interface{}); ok {
			for k := range cef {
				fieldSet[k] = true
			}
		}
		// attrs도 확인 (하위 호환)
		if attrs, ok := doc["attrs"].(map[string]interface{}); ok {
			for k := range attrs {
				fieldSet[k] = true
			}
		}
	}

	fields := make([]string, 0, len(fieldSet))
	for f := range fieldSet {
		fields = append(fields, f)
	}
	return fields
}

// analyzeFieldDetail - 필드 값 목록 수집 (select/checkbox용)
func (c *FieldMetaController) analyzeFieldDetail(msgID, field string) map[string]interface{} {
	// aggregation으로 값별 카운트
	raw, _ := c.OS.SearchRaw(LogsIndexPattern(c.IndexPrefix), map[string]interface{}{
		"size": 0,
		"query": map[string]interface{}{"term": map[string]interface{}{"msgId.keyword": msgID}},
		"aggs": map[string]interface{}{
			"vals": map[string]interface{}{
				"terms": map[string]interface{}{
					"field": "cefExtensions." + field + ".keyword",
					"size":  100,
				},
			},
			"vals2": map[string]interface{}{
				"terms": map[string]interface{}{
					"field": "attrs." + field + ".keyword",
					"size":  100,
				},
			},
		},
	})

	// 값 + 카운트 수집
	valueMap := make(map[string]int)
	if aggs, ok := raw["aggregations"].(map[string]interface{}); ok {
		for _, aggName := range []string{"vals", "vals2"} {
			if agg, ok := aggs[aggName].(map[string]interface{}); ok {
				if buckets, ok := agg["buckets"].([]interface{}); ok {
					for _, b := range buckets {
						if bucket, ok := b.(map[string]interface{}); ok {
							key, _ := bucket["key"].(string)
							count, _ := bucket["doc_count"].(float64)
							if key != "" {
								valueMap[key] += int(count)
							}
						}
					}
				}
			}
		}
	}

	// {value, count} 형태로 변환
	values := make([]map[string]interface{}, 0, len(valueMap))
	for v, c := range valueMap {
		values = append(values, map[string]interface{}{"value": v, "count": c})
	}

	return map[string]interface{}{
		"field":  field,
		"values": values,
	}
}
