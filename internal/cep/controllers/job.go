package controllers

import (
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

	docs, err := c.OS.Search(common.RulesIndex(c.IndexPrefix), map[string]interface{}{
		"size": 100,
		"query": map[string]interface{}{"match_all": map[string]interface{}{}},
	})
	if err != nil {
		return 0, err
	}

	submitted := 0
	for _, doc := range docs {
		if enabled, _ := doc["enabled"].(bool); !enabled {
			continue
		}

		ruleID, _ := doc["_id"].(string)
		name, _ := doc["name"].(string)
		severity, _ := doc["severity"].(string)

		sql := services.BuildSQLFromRule(doc)
		if sql == "" {
			continue
		}

		if _, err := c.Flink.SubmitRule(ruleID, name, severity, sql); err == nil {
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
