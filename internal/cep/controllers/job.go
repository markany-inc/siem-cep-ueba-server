package controllers

import (
	"log"
	"time"

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

	// CEP 규칙 조회 (jobId 포함)
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

	// 1. 룰에 저장된 jobId 수집
	toCancel := make(map[string]string) // jobId → name
	for _, doc := range docs {
		if jobId, ok := doc["jobId"].(string); ok && jobId != "" {
			name, _ := doc["name"].(string)
			toCancel[jobId] = name
		}
	}

	// 2. Flink에서 실행 중인 CEP Job도 추가
	for jid, name := range c.Flink.GetRunningCEPJobs() {
		toCancel[jid] = name
	}

	// 3. 일괄 취소 요청
	for jid, name := range toCancel {
		log.Printf("[CEP] Job 취소: %s", name)
		c.Flink.CancelJobByID(jid)
	}

	// 4. 취소 완료 대기 (RUNNING 상태 Job이 없을 때까지, 최대 10초)
	if len(toCancel) > 0 {
		for i := 0; i < 20; i++ {
			time.Sleep(500 * time.Millisecond)
			if len(c.Flink.GetRunningCEPJobs()) == 0 {
				break
			}
		}
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

	// 직렬 제출 + jobId 저장
	submitted := 0
	for _, r := range toSubmit {
		jobId, err := c.Flink.SubmitRule(r.ruleID, r.name, r.severity, r.sql)
		if err != nil {
			log.Printf("[CEP] 제출 실패: %s - %v", r.name, err)
			c.updateRuleJobStatus(r.ruleID, "", "FAILED")
		} else {
			submitted++
			c.updateRuleJobStatus(r.ruleID, jobId, "RUNNING")
		}
	}

	return submitted, nil
}

// updateRuleJobStatus 룰 인덱스에 Job 상태 업데이트
func (c *JobController) updateRuleJobStatus(ruleID, jobId, status string) {
	idx := common.RulesIndex(c.IndexPrefix)
	err := c.OS.Update(idx, ruleID, map[string]interface{}{
		"jobId":        jobId,
		"jobStatus":    status,
		"jobStartedAt": time.Now().Format(time.RFC3339),
	})
	if err != nil {
		log.Printf("[CEP] jobId 저장 실패: %s/%s - %v", idx, ruleID, err)
	}
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
