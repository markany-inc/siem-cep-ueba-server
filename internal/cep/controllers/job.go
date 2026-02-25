package controllers

import (
	"log"

	"github.com/labstack/echo/v4"
	"github.com/markany/safepc-siem/internal/cep/services"
	"github.com/markany/safepc-siem/internal/common"
)

type JobController struct {
	Flink       *services.FlinkService
	OS          *common.OSClient
	IndexPrefix string
}

func NewJobController(flink *services.FlinkService, os *common.OSClient, indexPrefix string) *JobController {
	return &JobController{Flink: flink, OS: os, IndexPrefix: indexPrefix}
}

func (c *JobController) Submit(ctx echo.Context) error {
	var req struct {
		RuleID   string                 `json:"ruleId"`
		Name     string                 `json:"name"`
		Severity string                 `json:"severity"`
		SQL      string                 `json:"sql"`
		Rule     map[string]interface{} `json:"rule"`
	}
	if err := ctx.Bind(&req); err != nil {
		return ctx.JSON(400, map[string]string{"error": "invalid JSON"})
	}

	sql := req.SQL
	if sql == "" && req.Rule != nil {
		sql = services.BuildSQLFromRule(req.Rule)
	}
	if sql == "" {
		return ctx.JSON(400, map[string]string{"error": "sql 또는 rule 필요"})
	}
	if req.Severity == "" {
		req.Severity = "MEDIUM"
	}

	jobID, err := c.Flink.SubmitRule(req.RuleID, req.Name, req.Severity, sql)
	if err != nil {
		return ctx.JSON(500, map[string]string{"error": err.Error()})
	}
	return ctx.JSON(200, map[string]string{"status": "ok", "jobId": jobID})
}

func (c *JobController) Reload(ctx echo.Context) error {
	submitted, err := c.ReloadAll()
	if err != nil {
		return ctx.JSON(500, map[string]string{"error": err.Error()})
	}
	return ctx.JSON(200, map[string]interface{}{"status": "ok", "submitted": submitted})
}

func (c *JobController) ReloadAll() (int, error) {
	if err := c.Flink.EnsureSession(); err != nil {
		return 0, err
	}

	// 기존 CEP Job 전부 취소
	for jid, name := range c.Flink.GetRunningCEPJobs() {
		log.Printf("[CEP] 기존 Job 취소: %s", name)
		c.Flink.CancelJobByID(jid)
	}

	// CEP 규칙 조회
	docs, err := c.OS.Search(common.RulesIndex(c.IndexPrefix), map[string]interface{}{
		"size": 100,
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{
					{"term": map[string]interface{}{"enabled": true}},
					{"term": map[string]interface{}{"cep.enabled": true}},
				},
			},
		},
	})
	if err != nil {
		return 0, err
	}

	type ruleJob struct {
		ruleID, name, severity, sql string
	}
	var toSubmit []ruleJob
	for _, doc := range docs {
		ruleID, _ := doc["_id"].(string)
		name, _ := doc["name"].(string)
		severity, _ := doc["severity"].(string)
		sql := services.BuildSQLFromRule(doc)
		if sql == "" {
			continue
		}
		toSubmit = append(toSubmit, ruleJob{ruleID, name, severity, sql})
	}

	if len(toSubmit) == 0 {
		return 0, nil
	}

	// 직렬 제출 (SET pipeline.name + INSERT가 세션 공유하므로 병렬 불가)
	submitted := 0
	for _, r := range toSubmit {
		if _, err := c.Flink.SubmitRule(r.ruleID, r.name, r.severity, r.sql); err != nil {
			log.Printf("[CEP] 제출 실패: %s - %v", r.name, err)
		} else {
			submitted++
		}
	}

	return submitted, nil
}

func (c *JobController) Status(ctx echo.Context) error {
	jobs, _ := c.Flink.GetRunningJobIDs()
	trackedCount, trackedJobs := c.Flink.GetTrackedJobs()
	return ctx.JSON(200, map[string]interface{}{
		"running":     len(jobs),
		"tracked":     trackedCount,
		"trackedJobs": trackedJobs,
	})
}
