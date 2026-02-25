package services

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/IBM/sarama"
	"github.com/markany/safepc-siem/internal/common"
)

func StartAlertConsumer(kafka, groupID, topic, indexPrefix string, os *common.OSClient) {
	config := sarama.NewConfig()
	config.Consumer.Offsets.Initial = sarama.OffsetNewest

	consumer, err := sarama.NewConsumer([]string{kafka}, config)
	if err != nil {
		log.Fatalf("[CEP Alert] Kafka 연결 실패: %v", err)
	}
	defer consumer.Close()

	log.Printf("[CEP Alert] 시작: %s (topic: %s)", kafka, topic)

	partitions, err := consumer.Partitions(topic)
	if err != nil {
		log.Printf("[CEP Alert] 토픽 파티션 조회 실패: %v", err)
		return
	}

	for _, partition := range partitions {
		pc, err := consumer.ConsumePartition(topic, partition, sarama.OffsetNewest)
		if err != nil {
			log.Printf("[CEP Alert] 파티션 %d 구독 실패: %v", partition, err)
			continue
		}
		go func(pc sarama.PartitionConsumer) {
			defer pc.Close()
			for msg := range pc.Messages() {
				processAlert(msg.Value, os, indexPrefix)
			}
		}(pc)
	}
	select {}
}

func processAlert(data []byte, os *common.OSClient, indexPrefix string) {
	var alert map[string]interface{}
	if err := json.Unmarshal(data, &alert); err != nil {
		return
	}
	if alert["@timestamp"] == nil {
		alert["@timestamp"] = time.Now().Format(time.RFC3339)
	}
	indexName := common.DailyAlertsIndex(indexPrefix, time.Now().Format("2006.01.02"))
	docID := fmt.Sprintf("%d", time.Now().UnixNano())
	os.Put(indexName, docID, alert)
}

// StartLogSink subscribes to all event topics and stores raw events into event-logs index.
func StartLogSink(kafka, groupID, topicsCsv, indexPrefix string, os *common.OSClient) {
	topics := strings.Split(topicsCsv, ",")
	for i := range topics {
		topics[i] = strings.TrimSpace(topics[i])
	}

	config := sarama.NewConfig()
	config.Consumer.Offsets.Initial = sarama.OffsetNewest

	consumer, err := sarama.NewConsumer([]string{kafka}, config)
	if err != nil {
		log.Fatalf("[LogSink] Kafka 연결 실패: %v", err)
	}
	defer consumer.Close()

	log.Printf("[LogSink] 시작: %d개 토픽 구독", len(topics))

	for _, topic := range topics {
		partitions, err := consumer.Partitions(topic)
		if err != nil {
			log.Printf("[LogSink] 토픽 %s 파티션 조회 실패: %v", topic, err)
			continue
		}
		log.Printf("[LogSink] 토픽 %s: %d개 파티션", topic, len(partitions))
		for _, partition := range partitions {
			pc, err := consumer.ConsumePartition(topic, partition, sarama.OffsetNewest)
			if err != nil {
				log.Printf("[LogSink] %s/%d 구독 실패: %v", topic, partition, err)
				continue
			}
			go func(t string, pc sarama.PartitionConsumer) {
				defer pc.Close()
				for msg := range pc.Messages() {
					sinkEvent(msg.Value, os, indexPrefix)
				}
			}(topic, pc)
		}
	}
	select {}
}

func sinkEvent(data []byte, os *common.OSClient, indexPrefix string) {
	var event map[string]interface{}
	if err := json.Unmarshal(data, &event); err != nil {
		return
	}
	if event["@timestamp"] == nil {
		event["@timestamp"] = time.Now().Format(time.RFC3339)
	}
	indexName := common.DailyLogsIndex(indexPrefix, time.Now().Format("2006.01.02"))
	if err := os.Index(indexName, event); err != nil {
		log.Printf("[LogSink] 저장 실패: %v", err)
	}
}
