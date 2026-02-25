package cmd

import (
	"log"

	"github.com/markany/safepc-siem/config"
	"github.com/markany/safepc-siem/internal/logsink"
	"github.com/spf13/cobra"
)

var logsinkCmd = &cobra.Command{
	Use:   "logsink",
	Short: "LogSink 서비스 시작 (원본 이벤트 → 변환 토픽 발행 + OpenSearch 저장)",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.LoadFromEnv("logsink")
		log.Println("LogSink 서비스 시작...")
		log.Printf("  Kafka: %s", cfg.Kafka.Bootstrap)
		log.Printf("  원본 토픽: %s", cfg.Kafka.EventTopics)
		log.Printf("  변환 토픽: %s", cfg.LogSink.TransformedTopic)
		log.Printf("  OpenSearch: %s", cfg.OpenSearch.URL)
		logsink.Start(cfg)
	},
}
