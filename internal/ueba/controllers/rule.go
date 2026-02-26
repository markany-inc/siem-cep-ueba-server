package controllers

import (
	"io"
	"encoding/json"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/markany/safepc-siem/internal/ueba/services"
)

type RuleController struct{}

func NewRuleController() *RuleController {
	return &RuleController{}
}

func (c *RuleController) List(ctx echo.Context) error {
	return ctx.JSON(200, map[string]interface{}{"rules": services.GetRulesRaw()})
}

func (c *RuleController) Create(ctx echo.Context) error {
	data, err := readBody(ctx)
	if err != nil {
		return ctx.JSON(400, map[string]string{"error": err.Error()})
	}
	id, err := services.CreateRule(data)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "필수") || strings.Contains(errMsg, "잘못") {
			return ctx.JSON(400, map[string]string{"error": errMsg})
		}
		return ctx.JSON(500, map[string]string{"error": errMsg})
	}
	return ctx.JSON(200, map[string]string{"status": "ok", "id": id})
}

func (c *RuleController) Update(ctx echo.Context) error {
	data, err := readBody(ctx)
	if err != nil {
		return ctx.JSON(400, map[string]string{"error": err.Error()})
	}
	if err := services.UpdateRule(ctx.Param("id"), data); err != nil {
		return ctx.JSON(500, map[string]string{"error": err.Error()})
	}
	return ctx.JSON(200, map[string]string{"status": "ok"})
}

func (c *RuleController) Delete(ctx echo.Context) error {
	if err := services.DeleteRule(ctx.Param("id")); err != nil {
		return ctx.JSON(500, map[string]string{"error": err.Error()})
	}
	return ctx.JSON(200, map[string]string{"status": "ok"})
}

func (c *RuleController) Validate(ctx echo.Context) error {
	data, err := readBody(ctx)
	if err != nil {
		return ctx.JSON(400, map[string]string{"error": err.Error()})
	}
	if errs := services.ValidateRule(data); len(errs) > 0 {
		return ctx.JSON(400, map[string]interface{}{"valid": false, "errors": errs})
	}
	return ctx.JSON(200, map[string]interface{}{"valid": true})
}

func readBody(ctx echo.Context) (map[string]interface{}, error) {
	body, err := io.ReadAll(ctx.Request().Body)
	if err != nil {
		return nil, err
	}
	var data map[string]interface{}
	err = json.Unmarshal(body, &data)
	return data, err
}
