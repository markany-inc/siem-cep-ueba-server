package common

import (
	"fmt"
	"time"

	"github.com/labstack/echo/v4"
)

type LogController struct {
	OS          *OSClient
	IndexPrefix string
}

func NewLogController(os *OSClient, indexPrefix string) *LogController {
	return &LogController{OS: os, IndexPrefix: indexPrefix}
}

// Search - 로그 검색 (날짜 범위, 사용자, msgId 필터 + 페이지네이션)
// GET /api/logs?from=2026-03-01&to=2026-03-25&userId=hslee&msgId=MESSAGE_DEVICE_USAGE&size=100&offset=0
func (c *LogController) Search(ctx echo.Context) error {
	from := ctx.QueryParam("from")
	to := ctx.QueryParam("to")
	userId := ctx.QueryParam("userId")
	msgId := ctx.QueryParam("msgId")
	hostname := ctx.QueryParam("hostname")
	size := 100
	offset := 0
	if s := ctx.QueryParam("size"); s != "" {
		fmt.Sscanf(s, "%d", &size)
	}
	if o := ctx.QueryParam("offset"); o != "" {
		fmt.Sscanf(o, "%d", &offset)
	}
	if size > 1000 {
		size = 1000
	}
	if offset > 10000 {
		offset = 10000 // OpenSearch 기본 제한
	}

	// 기본값: 최근 7일
	now := time.Now()
	if to == "" {
		to = now.Format("2006-01-02")
	}
	if from == "" {
		from = now.AddDate(0, 0, -7).Format("2006-01-02")
	}

	indexPattern := c.buildIndexPattern(from, to)

	// 쿼리 빌드
	must := []map[string]interface{}{
		{"range": map[string]interface{}{"@timestamp": map[string]interface{}{"gte": from, "lte": to + "T23:59:59"}}},
	}
	if userId != "" {
		must = append(must, map[string]interface{}{"term": map[string]interface{}{"userId.keyword": userId}})
	}
	if msgId != "" {
		must = append(must, map[string]interface{}{"term": map[string]interface{}{"msgId.keyword": msgId}})
	}
	if hostname != "" {
		must = append(must, map[string]interface{}{"term": map[string]interface{}{"hostname.keyword": hostname}})
	}

	query := map[string]interface{}{
		"query":          map[string]interface{}{"bool": map[string]interface{}{"must": must}},
		"sort":           []map[string]interface{}{{"@timestamp": "desc"}},
		"size":           size,
		"from":           offset,
		"track_total_hits": true,
	}

	result, err := c.OS.SearchRaw(indexPattern, query)
	if err != nil {
		return ctx.JSON(500, map[string]string{"error": err.Error()})
	}

	// 결과 파싱
	hitsObj, _ := result["hits"].(map[string]interface{})
	totalObj, _ := hitsObj["total"].(map[string]interface{})
	total := 0
	if v, ok := totalObj["value"].(float64); ok {
		total = int(v)
	}
	hitArr, _ := hitsObj["hits"].([]interface{})

	logs := make([]map[string]interface{}, 0, len(hitArr))
	for _, h := range hitArr {
		hit, _ := h.(map[string]interface{})
		if doc, ok := hit["_source"].(map[string]interface{}); ok {
			logs = append(logs, doc)
		}
	}

	return ctx.JSON(200, map[string]interface{}{
		"total":  total,
		"size":   size,
		"offset": offset,
		"logs":   logs,
	})
}

// Aggregate - 로그 집계 (이벤트별 카운트, 사용자별 카운트 등)
// POST /api/logs/aggregate
func (c *LogController) Aggregate(ctx echo.Context) error {
	var req struct {
		From    string `json:"from"`
		To      string `json:"to"`
		GroupBy string `json:"groupBy"` // msgId, userId, hostname
		UserId  string `json:"userId"`
		MsgId   string `json:"msgId"`
	}
	if err := ctx.Bind(&req); err != nil {
		return ctx.JSON(400, map[string]string{"error": "invalid JSON"})
	}

	now := time.Now()
	if req.To == "" {
		req.To = now.Format("2006-01-02")
	}
	if req.From == "" {
		req.From = now.AddDate(0, 0, -7).Format("2006-01-02")
	}
	if req.GroupBy == "" {
		req.GroupBy = "msgId"
	}

	indexPattern := c.buildIndexPattern(req.From, req.To)

	must := []map[string]interface{}{
		{"range": map[string]interface{}{"@timestamp": map[string]interface{}{"gte": req.From, "lte": req.To + "T23:59:59"}}},
	}
	if req.UserId != "" {
		must = append(must, map[string]interface{}{"term": map[string]interface{}{"userId.keyword": req.UserId}})
	}
	if req.MsgId != "" {
		must = append(must, map[string]interface{}{"term": map[string]interface{}{"msgId.keyword": req.MsgId}})
	}

	query := map[string]interface{}{
		"query": map[string]interface{}{"bool": map[string]interface{}{"must": must}},
		"size":  0,
		"aggs": map[string]interface{}{
			"group": map[string]interface{}{
				"terms": map[string]interface{}{"field": req.GroupBy + ".keyword", "size": 100},
			},
		},
	}

	result, err := c.OS.SearchRaw(indexPattern, query)
	if err != nil {
		return ctx.JSON(500, map[string]string{"error": err.Error()})
	}

	// aggregation 결과 추출
	aggs, _ := result["aggregations"].(map[string]interface{})
	group, _ := aggs["group"].(map[string]interface{})
	buckets, _ := group["buckets"].([]interface{})

	items := make([]map[string]interface{}, 0)
	for _, b := range buckets {
		bucket := b.(map[string]interface{})
		items = append(items, map[string]interface{}{
			"key":   bucket["key"],
			"count": bucket["doc_count"],
		})
	}

	return ctx.JSON(200, map[string]interface{}{
		"groupBy": req.GroupBy,
		"items":   items,
	})
}

// buildIndexPattern - 날짜 범위에 해당하는 인덱스 패턴 생성
func (c *LogController) buildIndexPattern(from, to string) string {
	// 간단히 와일드카드 사용 (OpenSearch가 없는 인덱스는 무시)
	return fmt.Sprintf("%s-siem-event-logs-*", c.IndexPrefix)
}

// MigrationMeta - 로그 마이그레이션 메타 정보 (프론트용)
// GET /api/logs/migration-meta
func (c *LogController) MigrationMeta(ctx echo.Context) error {
	// field-meta에서 이벤트별 필드 정보 조회
	doc, err := c.OS.Get(FieldMetaIndex(c.IndexPrefix), "meta-latest")
	if err != nil {
		// field-meta 없으면 기본 템플릿만 반환
		doc = map[string]interface{}{"events": map[string]interface{}{}}
	}

	events, _ := doc["events"].(map[string]interface{})

	// 규칙 생성에 필요한 메타 정보 구조화
	meta := map[string]interface{}{
		"events":    events,
		"operators": []string{"eq", "neq", "gt", "gte", "lt", "lte", "in", "like", "regex", "exists"},
		"logicOps":  []string{"and", "or"},
		"aggregates": map[string]interface{}{
			"functions": []string{"count", "sum", "avg", "min", "max", "count_distinct"},
			"windows":   []string{"1m", "5m", "10m", "30m", "1h", "6h", "1d"},
		},
		"ruleTemplate": map[string]interface{}{
			"name":    "",
			"enabled": true,
			"match": map[string]interface{}{
				"msgId":      "",
				"logic":      "and",
				"conditions": []interface{}{},
			},
			"aggregate": nil,
			"within":    "5m",
			"cep":       map[string]interface{}{"enabled": false, "severity": "MEDIUM"},
			"ueba":      map[string]interface{}{"enabled": false, "weight": 5},
		},
	}

	return ctx.JSON(200, meta)
}
