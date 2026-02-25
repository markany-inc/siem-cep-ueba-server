package controllers

import (
	"fmt"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/markany/safepc-siem/internal/cep/services"
	"github.com/markany/safepc-siem/internal/common"
)

type RuleController struct {
	OS          *common.OSClient
	Flink       *services.FlinkService
	IndexPrefix string
}

func NewRuleController(os *common.OSClient, flink *services.FlinkService, indexPrefix string) *RuleController {
	return &RuleController{OS: os, Flink: flink, IndexPrefix: indexPrefix}
}

func (c *RuleController) List(ctx echo.Context) error {
	docs, err := c.OS.Search(common.RulesIndex(c.IndexPrefix), map[string]interface{}{
		"query": map[string]interface{}{"term": map[string]interface{}{"cep.enabled": true}},
		"size":  100,
	})
	if err != nil {
		return ctx.JSON(500, map[string]string{"error": err.Error()})
	}
	for i := range docs {
		docs[i]["id"] = docs[i]["_id"]
	}
	return ctx.JSON(200, map[string]interface{}{"rules": docs})
}

func (c *RuleController) Create(ctx echo.Context) error {
	var rule map[string]interface{}
	if err := ctx.Bind(&rule); err != nil {
		return ctx.JSON(400, map[string]string{"error": "invalid JSON"})
	}

	ruleID := fmt.Sprintf("rule-%d", time.Now().Unix())
	if id, ok := rule["ruleId"].(string); ok && id != "" {
		ruleID = id
	}
	rule["ruleId"] = ruleID
	rule["createdAt"] = time.Now().Format(time.RFC3339)

	sql := services.BuildSQLFromRule(rule)
	if sql == "" {
		return ctx.JSON(400, map[string]string{"error": "SQL 생성 실패"})
	}
	rule["sql"] = sql

	if err := c.OS.Put(common.RulesIndex(c.IndexPrefix), ruleID, rule); err != nil {
		return ctx.JSON(500, map[string]string{"error": err.Error()})
	}
	c.OS.Refresh(common.RulesIndex(c.IndexPrefix))

	if enabled, _ := rule["enabled"].(bool); enabled {
		name, _ := rule["name"].(string)
		severity, _ := rule["name"].(string)
		if severity == "" {
			severity = "MEDIUM"
		}
		c.Flink.SubmitRule(ruleID, name, severity, sql)
	}

	return ctx.JSON(200, map[string]string{"status": "ok", "ruleId": ruleID})
}

func (c *RuleController) Update(ctx echo.Context) error {
	ruleID := ctx.Param("id")
	var rule map[string]interface{}
	if err := ctx.Bind(&rule); err != nil {
		return ctx.JSON(400, map[string]string{"error": "invalid JSON"})
	}

	rule["ruleId"] = ruleID
	rule["updatedAt"] = time.Now().Format(time.RFC3339)

	sql := services.BuildSQLFromRule(rule)
	if sql == "" {
		return ctx.JSON(400, map[string]string{"error": "SQL 생성 실패"})
	}
	rule["sql"] = sql

	if err := c.OS.Put(common.RulesIndex(c.IndexPrefix), ruleID, rule); err != nil {
		return ctx.JSON(500, map[string]string{"error": err.Error()})
	}
	c.OS.Refresh(common.RulesIndex(c.IndexPrefix))

	if enabled, _ := rule["enabled"].(bool); enabled {
		name, _ := rule["name"].(string)
		severity, _ := rule["name"].(string)
		if severity == "" {
			severity = "MEDIUM"
		}
		c.Flink.SubmitRule(ruleID, name, severity, sql)
	} else {
		c.Flink.CancelRule(ruleID)
	}

	return ctx.JSON(200, map[string]string{"status": "ok"})
}

func (c *RuleController) BuildSQL(ctx echo.Context) error {
	var rule map[string]interface{}
	if err := ctx.Bind(&rule); err != nil {
		return ctx.JSON(400, map[string]string{"error": "invalid JSON"})
	}
	sql := services.BuildSQLFromRule(rule)
	return ctx.JSON(200, map[string]string{"sql": sql})
}

func (c *RuleController) Delete(ctx echo.Context) error {
	ruleID := ctx.Param("id")
	c.Flink.CancelRule(ruleID)
	if err := c.OS.Delete(common.RulesIndex(c.IndexPrefix), ruleID); err != nil {
		return ctx.JSON(500, map[string]string{"error": err.Error()})
	}
	return ctx.JSON(200, map[string]string{"status": "ok"})
}
