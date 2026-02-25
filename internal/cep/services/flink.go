package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type FlinkService struct {
	SQLGatewayURL  string
	FlinkURL       string
	KafkaBootstrap string
	AlertTopic     string
	GroupID        string
	EventTopics    string

	client        *http.Client
	sessionID     string
	tablesCreated bool
	sessionMu     sync.Mutex
	ruleJobs      map[string]string
	ruleJobsMu    sync.RWMutex
}

func NewFlinkService(sqlGateway, flinkURL, kafka, alertTopic, groupID, eventTopics string) *FlinkService {
	return &FlinkService{
		SQLGatewayURL:  sqlGateway,
		FlinkURL:       flinkURL,
		KafkaBootstrap: kafka,
		AlertTopic:     alertTopic,
		GroupID:        groupID,
		EventTopics:    eventTopics,
		client:         &http.Client{Timeout: 30 * time.Second},
		ruleJobs:       make(map[string]string),
	}
}

func (s *FlinkService) GetRunningJobIDs() (map[string]bool, error) {
	resp, err := s.client.Get(s.FlinkURL + "/jobs/overview")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var body struct {
		Jobs []struct {
			JID   string `json:"jid"`
			State string `json:"state"`
		} `json:"jobs"`
	}
	json.NewDecoder(resp.Body).Decode(&body)
	ids := make(map[string]bool)
	for _, j := range body.Jobs {
		if j.State == "RUNNING" {
			ids[j.JID] = true
		}
	}
	return ids, nil
}

func (s *FlinkService) EnsureSession() error {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()

	if s.sessionID != "" && s.tablesCreated {
		return nil
	}

	if s.sessionID == "" {
		b, _ := json.Marshal(map[string]interface{}{})
		resp, err := s.client.Post(s.SQLGatewayURL+"/v1/sessions", "application/json", bytes.NewReader(b))
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		var result struct {
			SessionHandle string `json:"sessionHandle"`
		}
		json.NewDecoder(resp.Body).Decode(&result)
		s.sessionID = result.SessionHandle
		log.Printf("[Flink] 세션: %s", s.sessionID)
	}

	if !s.tablesCreated {
		// Flink는 여러 토픽을 세미콜론(;)으로 구분하여 지정
		flinkTopics := strings.ReplaceAll(s.EventTopics, ",", ";")

		eventsDDL := fmt.Sprintf(
			"CREATE TABLE IF NOT EXISTS events ("+
				"  msgId STRING, hostname STRING, eventName STRING, signatureId STRING,"+
				"  cefExtensions ROW<"+
				"    suid STRING, suser STRING, src STRING, smac STRING, shost STRING, rt STRING,"+
				"    aid STRING, customerExternalID STRING,"+
				"    act STRING, outcome STRING, reason STRING, msg STRING,"+
				"    fname STRING, filePath STRING, fsize STRING, fileHash STRING,"+
				"    proto STRING, dhost STRING, request STRING, app STRING,"+
				"    cs1 STRING, cs1Label STRING, cs2 STRING, cs2Label STRING,"+
				"    cs3 STRING, cs3Label STRING, cs4 STRING, cs4Label STRING,"+
				"    cs5 STRING, cs5Label STRING, cs6 STRING, cs6Label STRING,"+
				"    cn1 STRING, cn1Label STRING, cn2 STRING, cn2Label STRING,"+
				"    cn3 STRING, cn3Label STRING, cn4 STRING, cn4Label STRING,"+
				"    eventUuid STRING, eventType STRING, hasEventFile STRING,"+
				"    description STRING, deviceUuid STRING, userUuid STRING,"+
				"    adminUnblock STRING, blockType STRING, changeType STRING,"+
				"    deviceInfo STRING, deviceType STRING, systemUuid STRING,"+
				"    captureFileCount STRING, captureFiles STRING,"+
				"    unlockActorType STRING, releasedBlockType STRING"+
				"  >,"+
				"  userId AS cefExtensions.suid, userIp AS cefExtensions.src,"+
				"  action AS cefExtensions.act, outcome AS cefExtensions.outcome,"+
				"  cs1 AS cefExtensions.cs1, cs1Label AS cefExtensions.cs1Label,"+
				"  cs2 AS cefExtensions.cs2, cs2Label AS cefExtensions.cs2Label,"+
				"  cs3 AS cefExtensions.cs3, cs3Label AS cefExtensions.cs3Label,"+
				"  cs4 AS cefExtensions.cs4, cs4Label AS cefExtensions.cs4Label,"+
				"  cs5 AS cefExtensions.cs5, cs5Label AS cefExtensions.cs5Label,"+
				"  cs6 AS cefExtensions.cs6, cs6Label AS cefExtensions.cs6Label,"+
				"  cn1 AS cefExtensions.cn1, cn1Label AS cefExtensions.cn1Label,"+
				"  cn2 AS cefExtensions.cn2, cn2Label AS cefExtensions.cn2Label,"+
				"  cn3 AS cefExtensions.cn3, cn3Label AS cefExtensions.cn3Label,"+
				"  cn4 AS cefExtensions.cn4, cn4Label AS cefExtensions.cn4Label,"+
				"  fname AS cefExtensions.fname, filePath AS cefExtensions.filePath,"+
				"  fsize AS cefExtensions.fsize, fileHash AS cefExtensions.fileHash,"+
				"  proto AS cefExtensions.proto, dhost AS cefExtensions.dhost,"+
				"  request AS cefExtensions.request, app AS cefExtensions.app,"+
				"  proctime AS PROCTIME()"+
				") WITH ("+
				"  'connector' = 'kafka',"+
				"  'topic' = '%s',"+
				"  'properties.bootstrap.servers' = '%s',"+
				"  'properties.group.id' = '%s',"+
				"  'scan.startup.mode' = 'latest-offset',"+
				"  'format' = 'json',"+
				"  'json.fail-on-missing-field' = 'false',"+
				"  'json.ignore-parse-errors' = 'true'"+
				")", flinkTopics, s.KafkaBootstrap, s.GroupID)

		if err := s.ExecSQL(eventsDDL); err != nil {
			return err
		}

		alertsDDL := fmt.Sprintf(
			"CREATE TABLE IF NOT EXISTS alerts ("+
				"  ruleId STRING, ruleName STRING, severity STRING, userId STRING,"+
				"  hostname STRING, userIp STRING, cnt BIGINT, ts TIMESTAMP(3)"+
				") WITH ("+
				"  'connector' = 'kafka',"+
				"  'topic' = '%s',"+
				"  'properties.bootstrap.servers' = '%s',"+
				"  'format' = 'json'"+
				")", s.AlertTopic, s.KafkaBootstrap)

		if err := s.ExecSQL(alertsDDL); err != nil {
			return err
		}

		s.tablesCreated = true
		log.Println("[Flink] 테이블 생성 완료")
	}
	return nil
}

func (s *FlinkService) ExecSQL(sql string) error {
	flat := strings.ReplaceAll(sql, "\n", " ")
	body, _ := json.Marshal(map[string]string{"statement": flat})

	resp, err := s.client.Post(
		fmt.Sprintf("%s/v1/sessions/%s/statements", s.SQLGatewayURL, s.sessionID),
		"application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == 404 || strings.Contains(string(raw), "does not exist") {
		log.Println("[Flink] 세션 만료, 재생성")
		s.sessionMu.Lock()
		s.sessionID = ""
		s.tablesCreated = false
		s.sessionMu.Unlock()
		if err := s.EnsureSession(); err != nil {
			return err
		}
		body2, _ := json.Marshal(map[string]string{"statement": flat})
		resp2, err := s.client.Post(
			fmt.Sprintf("%s/v1/sessions/%s/statements", s.SQLGatewayURL, s.sessionID),
			"application/json", bytes.NewReader(body2))
		if err != nil {
			return err
		}
		defer resp2.Body.Close()
		raw, _ = io.ReadAll(resp2.Body)
	}

	var result map[string]interface{}
	json.Unmarshal(raw, &result)
	if errs, ok := result["errors"]; ok {
		return fmt.Errorf("SQL 에러: %v", errs)
	}
	return nil
}

func (s *FlinkService) SubmitRule(ruleID, ruleName, severity, sql string) (string, error) {
	if err := s.EnsureSession(); err != nil {
		return "", err
	}

	s.cancelRule(ruleID)

	before, _ := s.GetRunningJobIDs()

	safeName := strings.ReplaceAll(ruleName, "'", "''")
	flat := strings.ReplaceAll(sql, "\n", " ")

	var insertSQL string
	if strings.Contains(strings.ToUpper(sql), "MATCH_RECOGNIZE") {
		insertSQL = fmt.Sprintf(
			"INSERT INTO alerts SELECT '%s', '%s', '%s', userId, hostname, userIp, cnt, CURRENT_TIMESTAMP FROM (%s)",
			ruleID, safeName, severity, flat)
	} else {
		insertSQL = fmt.Sprintf(
			"INSERT INTO alerts SELECT '%s', '%s', '%s', userId, hostname, userIp, cnt, CURRENT_TIMESTAMP FROM (%s) AS t",
			ruleID, safeName, severity, flat)
	}

	s.ExecSQL(fmt.Sprintf("SET 'pipeline.name' = 'CEP: %s'", safeName))

	if err := s.ExecSQL(insertSQL); err != nil {
		return "", err
	}

	for i := 0; i < 6; i++ {
		time.Sleep(500 * time.Millisecond)
		after, _ := s.GetRunningJobIDs()
		for jid := range after {
			if !before[jid] {
				s.ruleJobsMu.Lock()
				s.ruleJobs[ruleID] = jid
				s.ruleJobsMu.Unlock()
				log.Printf("[CEP] 규칙 제출: %s → %s", ruleName, jid)
				return jid, nil
			}
		}
	}

	log.Printf("[CEP] 규칙 제출됨 (Job ID 미확인): %s", ruleName)
	return "", nil
}

func (s *FlinkService) cancelRule(ruleID string) bool {
	s.ruleJobsMu.Lock()
	jobID, ok := s.ruleJobs[ruleID]
	if ok {
		delete(s.ruleJobs, ruleID)
	}
	s.ruleJobsMu.Unlock()

	if !ok || jobID == "" {
		return false
	}

	req, _ := http.NewRequest("PATCH", fmt.Sprintf("%s/jobs/%s?mode=cancel", s.FlinkURL, jobID), nil)
	resp, err := s.client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	log.Printf("[CEP] Job 취소: %s → %s", ruleID, jobID)
	return true
}

// GetTrackedJobs 추적 중인 Job 목록
func (s *FlinkService) GetTrackedJobs() (int, map[string]string) {
	s.ruleJobsMu.RLock()
	defer s.ruleJobsMu.RUnlock()
	jobs := make(map[string]string, len(s.ruleJobs))
	for k, v := range s.ruleJobs {
		jobs[k] = v
	}
	return len(jobs), jobs
}

// CancelRule 규칙 Job 취소 (public)
func (s *FlinkService) CancelRule(ruleID string) bool {
	return s.cancelRule(ruleID)
}


