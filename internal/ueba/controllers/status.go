package controllers

import (
	"io"
	"encoding/json"
	"math"
	"runtime"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/markany/safepc-siem/internal/ueba/services"
)

type StatusController struct {
	StartTime time.Time
}

func NewStatusController() *StatusController {
	return &StatusController{StartTime: time.Now()}
}

func (c *StatusController) Health(ctx echo.Context) error {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	uc, bc, rc, totalEvents := services.GetStats()

	allocMB := float64(m.Alloc) / 1024 / 1024
	sysMB := float64(m.Sys) / 1024 / 1024
	uptime := time.Since(c.StartTime)

	level := "healthy"
	var warnings []string
	warnMB, critMB := services.GetHealthThresholds()
	if allocMB > critMB {
		level = "critical"
		warnings = append(warnings, "메모리 임계")
	} else if allocMB > warnMB {
		level = "warning"
		warnings = append(warnings, "메모리 경고")
	}
	if rc == 0 {
		level = "warning"
		warnings = append(warnings, "룰 없음")
	}

	return ctx.JSON(200, map[string]interface{}{
		"status":   level,
		"warnings": warnings,
		"uptime":   uptime.String(),
		"memory": map[string]interface{}{
			"alloc_mb":   math.Round(allocMB*100) / 100,
			"sys_mb":     math.Round(sysMB*100) / 100,
			"gc_count":   m.NumGC,
			"goroutines": runtime.NumGoroutine(),
		},
		"data": map[string]interface{}{
			"users":        uc,
			"baselines":    bc,
			"rules":        rc,
			"today_events": totalEvents,
		},
	})
}

func (c *StatusController) Status(ctx echo.Context) error {
	uc, bc, rc, _ := services.GetStats()
	return ctx.JSON(200, map[string]interface{}{
		"service":     "ueba-scoring",
		"mode":        "in-memory",
		"users":       uc,
		"baselines":   bc,
		"rules":       rc,
		"currentDate": services.GetCurrentDate(),
	})
}

func (c *StatusController) Config(ctx echo.Context) error {
	return ctx.JSON(200, services.GetConfig())
}

func (c *StatusController) Settings(ctx echo.Context) error {
	if ctx.Request().Method == "POST" {
		body, err := io.ReadAll(ctx.Request().Body)
		if err != nil {
			return ctx.JSON(400, map[string]string{"error": err.Error()})
		}
		var data map[string]interface{}
		if err := json.Unmarshal(body, &data); err != nil {
			return ctx.JSON(400, map[string]string{"error": err.Error()})
		}
		result, err := services.SaveSettings(data)
		if err != nil {
			return ctx.JSON(500, map[string]string{"error": err.Error()})
		}
		return ctx.JSON(200, map[string]interface{}{"status": "ok", "result": result})
	}
	return ctx.JSON(200, services.GetSettings())
}

func (c *StatusController) Reload(ctx echo.Context) error {
	services.ReloadCache()
	return ctx.String(200, "ok")
}

func (c *StatusController) Baseline(ctx echo.Context) error {
	services.TriggerBaseline()
	return ctx.String(200, "started")
}

func (c *StatusController) Save(ctx echo.Context) error {
	services.TriggerSave()
	return ctx.String(200, "ok")
}
