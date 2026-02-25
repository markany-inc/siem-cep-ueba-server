package cmd

import (
	"log"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/markany/safepc-siem/config"
	"github.com/markany/safepc-siem/internal/cep/controllers"
	"github.com/markany/safepc-siem/internal/cep/services"
	"github.com/markany/safepc-siem/internal/common"
	"github.com/spf13/cobra"
)

var cepCmd = &cobra.Command{
	Use:   "cep",
	Short: "CEP 서비스 시작",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.LoadFromEnv("cep")

		log.Println("CEP 서비스 시작...")
		log.Printf("  Port: %s", cfg.Server.Port)
		log.Printf("  OpenSearch: %s", cfg.OpenSearch.URL)
		log.Printf("  Flink: %s", cfg.Flink.RestAPI)
		log.Printf("  Kafka: %s", cfg.Kafka.Bootstrap)

		os := common.NewOSClient(cfg.OpenSearch.URL)
		common.InitTimezone(cfg.Timezone)
		flink := services.NewFlinkService(
			cfg.Flink.SQLGateway,
			cfg.Flink.RestAPI,
			cfg.Kafka.Bootstrap,
			cfg.Flink.AlertTopic,
			cfg.Kafka.GroupID,
			cfg.Kafka.EventTopics,
		)

		fieldMetaCtrl := common.NewFieldMetaController(os, cfg.IndexPrefix)
		ruleCtrl := controllers.NewRuleController(os, flink, cfg.IndexPrefix)
		jobCtrl := controllers.NewJobController(flink, os, cfg.IndexPrefix)
		alertCtrl := controllers.NewAlertController(os, cfg.IndexPrefix)

		e := echo.New()
		e.HideBanner = true
		e.Use(middleware.Logger())
		e.Use(middleware.Recover())

		// field-meta 공통 API
		e.GET("/api/field-meta", fieldMetaCtrl.Get)
		e.PUT("/api/field-meta", fieldMetaCtrl.Put)
		e.POST("/api/field-meta/analyze", fieldMetaCtrl.Analyze)
		e.POST("/api/field-meta/analyze-field", fieldMetaCtrl.AnalyzeField)

		// CEP 규칙 API
		e.GET("/api/rules", ruleCtrl.List)
		e.POST("/api/rules", ruleCtrl.Create)
		e.PUT("/api/rules/:id", ruleCtrl.Update)
		e.DELETE("/api/rules/:id", ruleCtrl.Delete)
		e.POST("/api/build-sql", ruleCtrl.BuildSQL)

		// CEP Job API
		e.POST("/api/submit", jobCtrl.Submit)
		e.POST("/api/reload", jobCtrl.Reload)
		e.GET("/api/status", jobCtrl.Status)

		// CEP Alert API
		e.GET("/api/alerts", alertCtrl.List)

		// 초기 로드 (백그라운드)
		go func() {
			time.Sleep(1 * time.Second) // echo 서버 시작 대기
			if err := flink.EnsureSession(); err != nil {
				log.Printf("[CEP] 초기화 실패: %v", err)
				return
			}
			if submitted, err := jobCtrl.ReloadAll(); err != nil {
				log.Printf("[CEP] 규칙 로드 실패: %v", err)
			} else {
				log.Printf("[CEP] %d개 규칙 로드 완료", submitted)
			}
		}()

		// Kafka alert consumer 시작
		log.Println("[CEP] Alert consumer 시작 중...")
		go services.StartAlertConsumer(
			cfg.Kafka.Bootstrap,
			cfg.Kafka.GroupID+"-alert-consumer",
			cfg.Flink.AlertTopic,
			cfg.IndexPrefix,
			os,
		)

		// Kafka log sink 시작 — 모든 이벤트를 event-logs 인덱스에 저장
		log.Println("[CEP] Log sink 시작 중...")
		go services.StartLogSink(
			cfg.Kafka.Bootstrap,
			cfg.Kafka.GroupID+"-log-sink",
			cfg.Kafka.EventTopics,
			cfg.IndexPrefix,
			os,
		)

		e.Logger.Fatal(e.Start(cfg.Server.Port))
	},
}
