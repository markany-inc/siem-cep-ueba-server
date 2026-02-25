package services

import (
	"encoding/json"
	"fmt"
	"log"
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
	now := common.Now()
	if alert["@timestamp"] == nil {
		alert["@timestamp"] = now.Format(time.RFC3339)
	}
	indexName := common.DailyAlertsIndex(indexPrefix, now.Format("2006.01.02"))
	docID := fmt.Sprintf("%d", now.UnixNano())
	os.Put(indexName, docID, alert)
}
