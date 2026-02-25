package cmd

import (
	"log"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/markany/safepc-siem/config"
	"github.com/markany/safepc-siem/internal/common"
	"github.com/markany/safepc-siem/internal/ueba/controllers"
	"github.com/markany/safepc-siem/internal/ueba/services"
	"github.com/spf13/cobra"
)

var uebaCmd = &cobra.Command{
	Use:   "ueba",
	Short: "UEBA 서비스 시작",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.LoadFromEnv("ueba")

		log.Println("UEBA 서비스 시작...")
		log.Printf("  Port: %s", cfg.Server.Port)
		log.Printf("  OpenSearch: %s", cfg.OpenSearch.URL)
		log.Printf("  Kafka: %s", cfg.Kafka.Bootstrap)

		osClient := common.NewOSClient(cfg.OpenSearch.URL)
		common.InitTimezone(cfg.Timezone)

		fieldMetaCtrl := common.NewFieldMetaController(osClient, cfg.IndexPrefix)
		ruleCtrl := controllers.NewRuleController()
		statusCtrl := controllers.NewStatusController()
		userCtrl := controllers.NewUserController()

		e := echo.New()
		e.HideBanner = true
		e.Use(middleware.Logger())
		e.Use(middleware.Recover())

		// field-meta 공통 API
		e.GET("/api/field-meta", fieldMetaCtrl.Get)
		e.PUT("/api/field-meta", fieldMetaCtrl.Put)
		e.POST("/api/field-meta/analyze", fieldMetaCtrl.Analyze)
		e.POST("/api/field-meta/analyze-field", fieldMetaCtrl.AnalyzeField)

		// UEBA 규칙 API
		e.GET("/api/rules", ruleCtrl.List)
		e.POST("/api/rules", ruleCtrl.Create)
		e.POST("/api/rules/validate", ruleCtrl.Validate)
		e.PUT("/api/rules/:id", ruleCtrl.Update)
		e.DELETE("/api/rules/:id", ruleCtrl.Delete)

		// UEBA 상태/설정 API
		e.GET("/api/health", statusCtrl.Health)
		e.GET("/api/status", statusCtrl.Status)
		e.GET("/api/config", statusCtrl.Config)
		e.GET("/api/settings", statusCtrl.Settings)
		e.POST("/api/settings", statusCtrl.Settings)
		e.GET("/reload", statusCtrl.Reload)
		e.GET("/baseline", statusCtrl.Baseline)
		e.GET("/save", statusCtrl.Save)

		// UEBA 사용자 API
		e.GET("/api/users", userCtrl.List)
		e.GET("/api/users/scores", userCtrl.Scores)
		e.GET("/api/users/:id", userCtrl.Get)
		e.GET("/api/users/:id/history", userCtrl.History)
		e.GET("/api/users/:id/hourly", userCtrl.Hourly)

		// UEBA 전체 로직 시작
		go services.StartProcessor(cfg)

		e.Logger.Fatal(e.Start(cfg.Server.Port))
	},
}
