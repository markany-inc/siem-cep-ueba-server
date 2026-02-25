package controllers

import (
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/markany/safepc-siem/internal/common"
)

type AlertController struct {
	OS          *common.OSClient
	IndexPrefix string
}

func NewAlertController(os *common.OSClient, indexPrefix string) *AlertController {
	return &AlertController{OS: os, IndexPrefix: indexPrefix}
}

func (c *AlertController) List(ctx echo.Context) error {
	draw, _ := strconv.Atoi(ctx.QueryParam("draw"))
	start, _ := strconv.Atoi(ctx.QueryParam("start"))
	length, _ := strconv.Atoi(ctx.QueryParam("length"))
	if length == 0 {
		length = 10
	}

	search := ctx.QueryParam("search")
	rule := ctx.QueryParam("rule")
	severity := ctx.QueryParam("severity")
	orderCol, _ := strconv.Atoi(ctx.QueryParam("order_col"))
	orderDir := ctx.QueryParam("order_dir")
	if orderDir == "" {
		orderDir = "desc"
	}

	// 정렬 필드
	sortField := "@timestamp"
	if orderCol == 1 {
		sortField = "ruleName.keyword"
	} else if orderCol == 2 {
		sortField = "userId.keyword"
	}

	// 쿼리 빌드
	must := []map[string]interface{}{}
	if search != "" {
		must = append(must, map[string]interface{}{
			"multi_match": map[string]interface{}{
				"query":  search,
				"fields": []string{"userId", "ruleName", "hostname"},
			},
		})
	}
	if rule != "" {
		must = append(must, map[string]interface{}{"term": map[string]interface{}{"ruleId.keyword": rule}})
	}
	if severity != "" {
		must = append(must, map[string]interface{}{"term": map[string]interface{}{"severity.keyword": severity}})
	}

	query := map[string]interface{}{"match_all": map[string]interface{}{}}
	if len(must) > 0 {
		query = map[string]interface{}{"bool": map[string]interface{}{"must": must}}
	}

	// 인덱스 패턴
	index := common.DailyAlertsIndex(c.IndexPrefix, time.Now().Format("2006.01.02"))

	// 총 개수
	total := 0
	if countRes, err := c.OS.Count(index, query); err == nil {
		total = countRes
	}

	// 검색
	docs, _ := c.OS.Search(index, map[string]interface{}{
		"query": query,
		"from":  start,
		"size":  length,
		"sort":  []map[string]interface{}{{sortField: orderDir}},
	})

	// DataTables 형식 (배열 인덱스: 0=timestamp, 1=ruleName, 2=ruleId, 3=severity, 4=userId, 5=hostname)
	data := make([][]interface{}, len(docs))
	for i, doc := range docs {
		ts, _ := doc["@timestamp"].(string)
		ruleName := doc["RuleName"]
		if ruleName == nil {
			ruleName = doc["ruleName"]
		}
		ruleId := doc["RuleId"]
		if ruleId == nil {
			ruleId = doc["ruleId"]
		}
		severity := doc["Severity"]
		if severity == nil {
			severity = doc["severity"]
		}
		userId := doc["UserId"]
		if userId == nil {
			userId = doc["userId"]
		}
		hostname := doc["Hostname"]
		if hostname == nil {
			hostname = doc["hostname"]
		}
		data[i] = []interface{}{ts, ruleName, ruleId, severity, userId, hostname}
	}

	return ctx.JSON(200, map[string]interface{}{
		"draw":            draw,
		"recordsTotal":    total,
		"recordsFiltered": total,
		"data":            data,
	})
}
