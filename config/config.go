package config

import (
	"github.com/spf13/viper"
)

type Config struct {
	Server      ServerConfig
	OpenSearch  OpenSearchConfig
	Kafka       KafkaConfig
	Flink       FlinkConfig
	UEBA        UEBAConfig
	LogSink     LogSinkConfig
	Timezone    string
	IndexPrefix string
}

type ServerConfig struct {
	Port string
}

type OpenSearchConfig struct {
	URL string
}

type KafkaConfig struct {
	Bootstrap   string
	GroupID     string
	EventTopics string
}

type FlinkConfig struct {
	SQLGateway string
	RestAPI    string
	AlertTopic string
}

type UEBAConfig struct {
	DashboardURL string
	HealthWarnMB float64
	HealthCritMB float64
}

type LogSinkConfig struct {
	TransformedTopic string
}

func LoadFromEnv(service string) *Config {
	viper.AutomaticEnv()

	viper.SetDefault("KAFKA_BOOTSTRAP_SERVERS", "43.202.241.121:44105")
	viper.SetDefault("KAFKA_CONSUMER_GROUP_PREFIX", "siem")
	viper.SetDefault("OPENSEARCH_URL", "http://203.229.154.49:49200")
	viper.SetDefault("TIMEZONE", "Asia/Seoul")
	viper.SetDefault("INDEX_PREFIX", "safepc")
	viper.SetDefault("KAFKA_TRANSFORMED_TOPIC", "safepc-siem-events")
	viper.SetDefault("KAFKA_EVENT_TOPICS", "MESSAGE_AGENT,MESSAGE_DEVICE,MESSAGE_NETWORK,MESSAGE_PROCESS,MESSAGE_PRINT,MESSAGE_DRM,MESSAGE_CLIPBOARD,MESSAGE_CAPTURE,MESSAGE_PC,MESSAGE_SCREENBLOCKER,MESSAGE_ASSETS")

	prefix := viper.GetString("KAFKA_CONSUMER_GROUP_PREFIX")
	transformedTopic := viper.GetString("KAFKA_TRANSFORMED_TOPIC")

	cfg := &Config{
		OpenSearch:  OpenSearchConfig{URL: viper.GetString("OPENSEARCH_URL")},
		Kafka:       KafkaConfig{Bootstrap: viper.GetString("KAFKA_BOOTSTRAP_SERVERS")},
		LogSink:     LogSinkConfig{TransformedTopic: transformedTopic},
		Timezone:    viper.GetString("TIMEZONE"),
		IndexPrefix: viper.GetString("INDEX_PREFIX"),
	}

	switch service {
	case "logsink":
		// LogSink: 원본 11개 토픽 구독
		cfg.Kafka.EventTopics = viper.GetString("KAFKA_EVENT_TOPICS")
		cfg.Kafka.GroupID = prefix + "-logsink"

	case "cep":
		// CEP: 변환 토픽 1개 구독
		cfg.Kafka.EventTopics = transformedTopic
		cfg.Kafka.GroupID = prefix + "-cep-sql"
		viper.SetDefault("CEP_PORT", ":48084")
		viper.SetDefault("FLINK_SQL_GATEWAY", "http://203.229.154.49:48083")
		viper.SetDefault("FLINK_REST_API", "http://203.229.154.49:48081")
		viper.SetDefault("KAFKA_ALERT_TOPIC", "cep-alerts")
		cfg.Server.Port = viper.GetString("CEP_PORT")
		cfg.Flink = FlinkConfig{
			SQLGateway: viper.GetString("FLINK_SQL_GATEWAY"),
			RestAPI:    viper.GetString("FLINK_REST_API"),
			AlertTopic: viper.GetString("KAFKA_ALERT_TOPIC"),
		}

	case "ueba":
		// UEBA: 변환 토픽 1개 구독
		cfg.Kafka.EventTopics = transformedTopic
		cfg.Kafka.GroupID = prefix + "-ueba"
		viper.SetDefault("UEBA_PORT", ":48082")
		viper.SetDefault("DASHBOARD_URL", "http://203.229.154.49:48501")
		viper.SetDefault("HEALTH_WARN_MB", 256.0)
		viper.SetDefault("HEALTH_CRIT_MB", 512.0)
		cfg.Server.Port = viper.GetString("UEBA_PORT")
		cfg.UEBA = UEBAConfig{
			DashboardURL: viper.GetString("DASHBOARD_URL"),
			HealthWarnMB: viper.GetFloat64("HEALTH_WARN_MB"),
			HealthCritMB: viper.GetFloat64("HEALTH_CRIT_MB"),
		}
	}

	return cfg
}
