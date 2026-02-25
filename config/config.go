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

func LoadFromEnv(service string) *Config {
	viper.AutomaticEnv()
	
	// 기본값 설정
	viper.SetDefault("KAFKA_BOOTSTRAP_SERVERS", "43.202.241.121:44105")
	viper.SetDefault("KAFKA_CONSUMER_GROUP_PREFIX", "siem")
		viper.SetDefault("OPENSEARCH_URL", "http://203.229.154.49:49200")
		viper.SetDefault("TIMEZONE", "Asia/Seoul")
		viper.SetDefault("INDEX_PREFIX", "safepc")
	
		cfg := &Config{
			OpenSearch: OpenSearchConfig{
				URL: viper.GetString("OPENSEARCH_URL"),
			},
			Kafka: KafkaConfig{
				Bootstrap: viper.GetString("KAFKA_BOOTSTRAP_SERVERS"),
			},
			Timezone:    viper.GetString("TIMEZONE"),
			IndexPrefix: viper.GetString("INDEX_PREFIX"),
		}
	
		prefix := viper.GetString("KAFKA_CONSUMER_GROUP_PREFIX")
		viper.SetDefault("KAFKA_EVENT_TOPICS", "MESSAGE_AGENT,MESSAGE_DEVICE,MESSAGE_NETWORK,MESSAGE_PROCESS,MESSAGE_PRINT,MESSAGE_DRM,MESSAGE_CLIPBOARD,MESSAGE_CAPTURE,MESSAGE_PC,MESSAGE_SCREENBLOCKER,MESSAGE_ASSETS")
		cfg.Kafka.EventTopics = viper.GetString("KAFKA_EVENT_TOPICS")

	if service == "cep" {
		viper.SetDefault("CEP_PORT", ":48084")
		viper.SetDefault("FLINK_SQL_GATEWAY", "http://203.229.154.49:48083")
		viper.SetDefault("FLINK_REST_API", "http://203.229.154.49:48081")
		viper.SetDefault("KAFKA_ALERT_TOPIC", "cep-alerts")

		cfg.Server.Port = viper.GetString("CEP_PORT")
		cfg.Kafka.GroupID = prefix + "-cep-sql"
		cfg.Flink = FlinkConfig{
			SQLGateway: viper.GetString("FLINK_SQL_GATEWAY"),
			RestAPI:    viper.GetString("FLINK_REST_API"),
			AlertTopic: viper.GetString("KAFKA_ALERT_TOPIC"),
		}
	} else if service == "ueba" {
		viper.SetDefault("UEBA_PORT", ":48082")
		viper.SetDefault("DASHBOARD_URL", "http://203.229.154.49:48501")
		viper.SetDefault("HEALTH_WARN_MB", 256.0)
		viper.SetDefault("HEALTH_CRIT_MB", 512.0)

		cfg.Server.Port = viper.GetString("UEBA_PORT")
		cfg.Kafka.GroupID = prefix + "-ueba"
		cfg.UEBA = UEBAConfig{
			DashboardURL: viper.GetString("DASHBOARD_URL"),
			HealthWarnMB: viper.GetFloat64("HEALTH_WARN_MB"),
			HealthCritMB: viper.GetFloat64("HEALTH_CRIT_MB"),
		}
	}

	return cfg
}
