package logsink

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/IBM/sarama"
	"github.com/markany/safepc-siem/config"
	"github.com/markany/safepc-siem/internal/common"
)

func Start(cfg *config.Config) {
	common.InitTimezone(cfg.Timezone)
	osClient := common.NewOSClient(cfg.OpenSearch.URL)

	topics := strings.Split(cfg.Kafka.EventTopics, ",")
	for i := range topics {
		topics[i] = strings.TrimSpace(topics[i])
	}

	// Kafka producer (변환 토픽 발행용)
	prodCfg := sarama.NewConfig()
	prodCfg.Producer.Return.Successes = true
	producer, err := sarama.NewSyncProducer([]string{cfg.Kafka.Bootstrap}, prodCfg)
	if err != nil {
		log.Fatalf("[LogSink] Producer 생성 실패: %v", err)
	}
	defer producer.Close()

	outTopic := cfg.LogSink.TransformedTopic
	log.Printf("[LogSink] 시작: %d개 원본 토픽 → %s", len(topics), outTopic)

	// Kafka consumer
	consCfg := sarama.NewConfig()
	consCfg.Consumer.Offsets.Initial = sarama.OffsetNewest
	consumer, err := sarama.NewConsumer([]string{cfg.Kafka.Bootstrap}, consCfg)
	if err != nil {
		log.Fatalf("[LogSink] Consumer 생성 실패: %v", err)
	}
	defer consumer.Close()

	for _, topic := range topics {
		partitions, err := consumer.Partitions(topic)
		if err != nil {
			log.Printf("[LogSink] %s 파티션 조회 실패: %v", topic, err)
			continue
		}
		for _, p := range partitions {
			pc, err := consumer.ConsumePartition(topic, p, sarama.OffsetNewest)
			if err != nil {
				continue
			}
			go func(pc sarama.PartitionConsumer) {
				defer pc.Close()
				for msg := range pc.Messages() {
					processMessage(msg.Value, producer, outTopic, osClient, cfg.IndexPrefix)
				}
			}(pc)
		}
	}
	select {}
}

func processMessage(data []byte, producer sarama.SyncProducer, outTopic string, os *common.OSClient, prefix string) {
	var event map[string]interface{}
	if err := json.Unmarshal(data, &event); err != nil {
		return
	}

	now := common.Now()
	if event["@timestamp"] == nil {
		event["@timestamp"] = now.Format(time.RFC3339)
	}

	// CEF label 변환
	if ext, ok := event["cefExtensions"].(map[string]interface{}); ok {
		common.ExpandCEFLabels(ext)
	}

	out, _ := json.Marshal(event)

	// 변환 토픽 발행
	producer.SendMessage(&sarama.ProducerMessage{
		Topic: outTopic,
		Value: sarama.ByteEncoder(out),
	})

	// OpenSearch event-logs 저장
	if err := os.Index(common.DailyLogsIndex(prefix, now.Format("2006.01.02")), event); err != nil {
		log.Printf("[LogSink] 저장 실패: %v", err)
	}
}
