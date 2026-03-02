package controllers

import (
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/markany/safepc-siem/internal/ueba/services"
)

type UserController struct{}

func NewUserController() *UserController {
	return &UserController{}
}

func (c *UserController) List(ctx echo.Context) error {
	users := services.GetAllUsers()
	return ctx.JSON(200, map[string]interface{}{"users": users})
}

func (c *UserController) Get(ctx echo.Context) error {
	userID := ctx.Param("id")
	user := services.GetUser(userID)
	if user == nil {
		return ctx.JSON(404, map[string]string{"error": "User not found"})
	}
	return ctx.JSON(200, user)
}

func (c *UserController) History(ctx echo.Context) error {
	return ctx.JSON(200, services.GetUserHistory(ctx.Param("id")))
}

func (c *UserController) Hourly(ctx echo.Context) error {
	data := services.GetUserHourly(ctx.Param("id"))
	if data == nil {
		data = []map[string]interface{}{}
	}
	return ctx.JSON(200, data)
}

func (c *UserController) Scores(ctx echo.Context) error {
	draw, _ := strconv.Atoi(ctx.QueryParam("draw"))
	start, _ := strconv.Atoi(ctx.QueryParam("start"))
	length, _ := strconv.Atoi(ctx.QueryParam("length"))
	search := ctx.QueryParam("search")
	orderCol, _ := strconv.Atoi(ctx.QueryParam("order_col"))
	orderDir := ctx.QueryParam("order_dir")

	cols := []string{"userId", "riskScore", "riskLevel"}
	sortField := "riskScore"
	if orderCol < len(cols) {
		sortField = cols[orderCol]
	}

	return ctx.JSON(200, services.GetUserScores(draw, start, length, search, sortField, orderDir))
}

// SetContext: 유저 상황가중치 설정
func (c *UserController) SetContext(ctx echo.Context) error {
	userID := ctx.Param("id")
	var req struct {
		Context string `json:"context"`
	}
	if err := ctx.Bind(&req); err != nil {
		return ctx.JSON(400, map[string]string{"error": "invalid request"})
	}
	if err := services.SetUserContext(userID, req.Context); err != nil {
		return ctx.JSON(500, map[string]string{"error": err.Error()})
	}
	return ctx.JSON(200, map[string]string{"status": "ok", "userId": userID, "context": req.Context})
}

// GetContext: 유저 상황가중치 조회
func (c *UserController) GetContext(ctx echo.Context) error {
	userID := ctx.Param("id")
	context := services.GetUserContext(userID)
	return ctx.JSON(200, map[string]string{"userId": userID, "context": context})
}

// ListProfiles: 전체 유저 프로필 목록
func (c *UserController) ListProfiles(ctx echo.Context) error {
	return ctx.JSON(200, services.GetAllUserProfiles())
}
