package controllers

import (
	"log"
	"strings"
	"sync"

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

	// CEP 규칙만 조회 (cep.enabled=true)
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

	// 현재 Flink에서 실행 중인 CEP Job 목록 (이름 → jobID)
	runningJobs := c.Flink.GetRunningCEPJobs()

	// 규칙별 SQL 생성 + 변경 감지
	type ruleJob struct {
		ruleID, name, severity, sql string
	}
	var toSubmit []ruleJob
	wantedJobNames := make(map[string]bool)

	for _, doc := range docs {
		ruleID, _ := doc["_id"].(string)
		name, _ := doc["name"].(string)
		severity, _ := doc["severity"].(string)
		sql := services.BuildSQLFromRule(doc)
		if sql == "" {
			continue
		}

		jobName := "CEP: " + strings.ReplaceAll(name, "'", "''")
		wantedJobNames[jobName] = true

		// 이미 동일 이름으로 실행 중이면 스킵
		if _, running := runningJobs[jobName]; running {
			log.Printf("[CEP] 이미 실행 중 (스킵): %s", name)
			// ruleJobs 맵에 등록
			c.Flink.TrackJob(ruleID, runningJobs[jobName])
			continue
		}

		toSubmit = append(toSubmit, ruleJob{ruleID, name, severity, sql})
	}

	// 더 이상 필요 없는 Job 취소
	for jobName, jobID := range runningJobs {
		if !wantedJobNames[jobName] {
			log.Printf("[CEP] 불필요 Job 취소: %s (%s)", jobName, jobID)
			c.Flink.CancelJobByID(jobID)
		}
	}

	if len(toSubmit) == 0 {
		log.Printf("[CEP] 새로 제출할 규칙 없음 (기존 %d개 유지)", len(wantedJobNames))
		return 0, nil
	}

	// 병렬 제출 (최대 5개 동시)
	sem := make(chan struct{}, 5)
	var mu sync.Mutex
	submitted := 0

	var wg sync.WaitGroup
	for _, r := range toSubmit {
		wg.Add(1)
		go func(r ruleJob) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if _, err := c.Flink.SubmitRule(r.ruleID, r.name, r.severity, r.sql); err != nil {
				log.Printf("[CEP] 제출 실패: %s - %v", r.name, err)
			} else {
				mu.Lock()
				submitted++
				mu.Unlock()
			}
		}(r)
	}
	wg.Wait()

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
