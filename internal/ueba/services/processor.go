package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/markany/safepc-siem/config"
	"github.com/markany/safepc-siem/internal/common"
)

const (
	maxRulesSize      = 100
	compositePageSize = 1000
)

// compositeAgg: composite aggregation 페이징으로 전체 유저 집계 (bucket 제한 없음)
func compositeAgg(index, userField string, query interface{}, subAggs map[string]interface{}, callback func(string, map[string]interface{})) {
	var afterKey map[string]interface{}
	for {
		composite := map[string]interface{}{
			"size":    compositePageSize,
			"sources": []interface{}{map[string]interface{}{"user": map[string]interface{}{"terms": map[string]interface{}{"field": userField}}}},
		}
		if afterKey != nil {
			composite["after"] = afterKey
		}
		aggs := map[string]interface{}{"users": map[string]interface{}{"composite": composite}}
		if subAggs != nil {
			aggs["users"].(map[string]interface{})["aggs"] = subAggs
		}
		body, _ := json.Marshal(map[string]interface{}{"size": 0, "query": query, "aggs": aggs})
		resp, err := httpClient.Post(fmt.Sprintf("%s/%s/_search", opensearchURL, index), "application/json", bytes.NewReader(body))
		if err != nil {
			break
		}
		var result struct {
			Aggregations struct {
				Users struct {
					AfterKey map[string]interface{} `json:"after_key"`
					Buckets  []map[string]interface{} `json:"buckets"`
				} `json:"users"`
			} `json:"aggregations"`
		}
		json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()

		for _, bucket := range result.Aggregations.Users.Buckets {
			key, _ := bucket["key"].(map[string]interface{})
			uid, _ := key["user"].(string)
			if uid != "" {
				callback(uid, bucket)
			}
		}
		if len(result.Aggregations.Users.Buckets) < compositePageSize {
			break
		}
		afterKey = result.Aggregations.Users.AfterKey
	}
}

var (
	opensearchURL  string
	kafkaBootstrap string
	kafkaEventTopics string
	dashboardURL   string
	timezone       string
	indexPrefix    string
	httpClient     = &http.Client{Timeout: 10 * time.Second}
	loc            *time.Location
	healthWarnMB, healthCritMB float64
	
	osClient  *common.OSClient

	configCache *Config
	configMu    sync.RWMutex
	rulesCache  []Rule
	rulesMu     sync.RWMutex

	userStates   = make(map[string]*UserState)
	userStatesMu sync.RWMutex

	baselines   = make(map[string]*Baseline)
	baselinesMu sync.RWMutex

	currentDate   string
	currentDateMu sync.RWMutex
)

// ===== 구조체 =====

type UserState struct {
	RiskScore      float64            `json:"riskScore"`
	RuleScore      float64            `json:"ruleScore"`
	RuleScores     map[string]float64 `json:"ruleScores"`
	AnomalyScore   float64            `json:"anomalyScore"`
	EventCounts    map[string]int     `json:"eventCounts"`
	EventValues    map[string]float64 `json:"eventValues"`
	PrevScore      float64            `json:"prevScore"`
	DaysSinceLast  int                `json:"daysSinceLast"`
	ColdStart      bool               `json:"coldStart"`
	LastUpdated    time.Time          `json:"lastUpdated"`
	Dirty          bool               `json:"-"`
}

type Rule struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Category  string        `json:"category"`
	Weight    float64       `json:"weight"`
	Enabled   bool          `json:"enabled"`
	Match     RuleMatch     `json:"match"`
	Aggregate RuleAggregate `json:"aggregate"`
	UEBA      RuleUEBA      `json:"ueba"`
}

type RuleMatch struct {
	MsgID      string          `json:"msgId"`
	Logic      string          `json:"logic"`
	Conditions []RuleCondition `json:"conditions"`
}

type RuleCondition struct {
	Field string      `json:"field"`
	Op    string      `json:"op"`
	Value interface{} `json:"value"`
}

// RuleAggregate: 룰이 매칭된 이벤트에서 무엇을 집계할지 정의
// Type: "count" (기본, 매칭 횟수) / "sum" (필드값 합산) / "cardinality" (고유값 수)
// Field: sum/cardinality 시 대상 CEF 필드명 (예: "fsize", "shost")
type RuleAggregate struct {
	Type  string `json:"type"`
	Field string `json:"field"`
}

type RuleUEBA struct {
	Enabled bool `json:"enabled"`
}

type Config struct {
	Anomaly AnomalyConfig `json:"anomaly"`
	Decay   DecayConfig   `json:"decay"`
	Tiers   TierConfig    `json:"tiers"`
}

type AnomalyConfig struct {
	ZThreshold        float64 `json:"z_threshold"`
	Beta              float64 `json:"beta"`
	SigmaFloor        float64 `json:"sigma_floor"`
	ColdStartMinDays  int     `json:"cold_start_min_days"`
	BaselineWindow    int     `json:"baseline_window_days"`
	FrequencyFunction string  `json:"frequency_function"`
}

type DecayConfig struct {
	Lambda      float64 `json:"lambda"`
	WeekendMode string  `json:"weekend_mode"`
}

type TierConfig struct {
	GreenMax  float64 `json:"green_max"`
	YellowMax float64 `json:"yellow_max"`
}

type Baseline struct {
	Mean       float64 `json:"mean"`
	Stddev     float64 `json:"stddev"`
	SampleDays int     `json:"sampleDays"`
}

type Score struct {
	UserID       string             `json:"userId"`
	RiskScore    float64            `json:"riskScore"`
	RiskLevel    string             `json:"riskLevel"`
	Status       string             `json:"status"`
	RuleScore    float64            `json:"ruleScore"`
	RuleScores   map[string]float64 `json:"ruleScores"`
	AnomalyScore float64            `json:"anomalyScore"`
	DailyScore   float64            `json:"dailyScore"`
	DecayedPrev  float64            `json:"decayedPrev"`
	PrevScore    float64            `json:"prevScore"`
	EventCounts  map[string]int     `json:"eventCounts"`
	EventValues  map[string]float64 `json:"eventValues"`
	Timestamp    string             `json:"@timestamp"`
}

func classifyRisk(score float64, cfg *Config) string {
	if score > cfg.Tiers.YellowMax {
		return "HIGH"
	} else if score > cfg.Tiers.GreenMax {
		return "MEDIUM"
	}
	return "LOW"
}

// ===== 초기화 =====

func initialize() {
	log.Println("[INIT] UEBA 시스템 초기화 시작")
	loadConfig()
	loadRules()
	loadAllBaselines()
	ensureBaselinesFresh()
	currentDate = time.Now().In(loc).Format("2006-01-02")
	recoverTodayState()
	log.Println("[INIT] 초기화 완료")
}

// ensureBaselinesFresh: baseline이 오늘 자정 기준(어제까지 데이터)으로 최신인지 확인
// settings 인덱스에 baseline_updated_at을 기록하여 판단
func ensureBaselinesFresh() {
	today := time.Now().In(loc).Format("2006-01-02")

	resp, err := httpClient.Get(fmt.Sprintf("%s/%s/_doc/baseline_meta", opensearchURL, common.SettingsIndex(indexPrefix)))
	if err == nil && resp.StatusCode == 200 {
		var result struct {
			Source struct {
				UpdatedAt string `json:"updated_at"`
			} `json:"_source"`
		}
		json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		if result.Source.UpdatedAt == today {
			log.Println("[INIT] Baseline 최신 (오늘 자정 갱신 완료)")
			return
		}
		log.Printf("[INIT] Baseline 갱신 필요 (마지막: %s, 오늘: %s)", result.Source.UpdatedAt, today)
	} else {
		if resp != nil {
			resp.Body.Close()
		}
		log.Println("[INIT] Baseline 메타 없음, 갱신 실행")
	}

	updateBaselines()
	// 갱신 완료 시점 기록
	meta, _ := json.Marshal(map[string]string{"updated_at": today})
	req, _ := http.NewRequest("PUT",
		fmt.Sprintf("%s/%s/_doc/baseline_meta", opensearchURL, common.SettingsIndex(indexPrefix)),
		bytes.NewReader(meta))
	req.Header.Set("Content-Type", "application/json")
	if r, err := httpClient.Do(req); err == nil {
		r.Body.Close()
	}
	log.Println("[INIT] Baseline 갱신 완료")
}

// initUsersFromPrevScores: 어제까지 점수가 있는 모든 유저를 decay 적용하여 인메모리에 초기화
// 오늘 이벤트가 없어도 대시보드에 decay된 점수가 표시됨
func initUsersFromPrevScores() {
	today := time.Now().In(loc).Format("2006-01-02")
	now := time.Now().In(loc)
	count := 0

	compositeAgg(
		common.ScoresIndexPattern(indexPrefix),
		"userId.keyword",
		map[string]interface{}{"range": map[string]interface{}{
			"@timestamp": map[string]interface{}{"lt": today, "time_zone": "Asia/Seoul"},
		}},
		map[string]interface{}{
			"latest": map[string]interface{}{
				"top_hits": map[string]interface{}{
					"size": 1, "sort": []map[string]interface{}{{"@timestamp": "desc"}},
					"_source": []string{"riskScore", "@timestamp"},
				},
			},
		},
		func(uid string, bucket map[string]interface{}) {
			// top_hits 결과 파싱
			latest, _ := bucket["latest"].(map[string]interface{})
			hits, _ := latest["hits"].(map[string]interface{})
			hitArr, _ := hits["hits"].([]interface{})
			if len(hitArr) == 0 {
				return
			}
			hit, _ := hitArr[0].(map[string]interface{})
			src, _ := hit["_source"].(map[string]interface{})
			score := toFloat64(src["riskScore"])
			if score <= 0 {
				return
			}
			days := 1
			if ts, ok := src["@timestamp"].(string); ok {
				if t, err := time.Parse(time.RFC3339, ts); err == nil {
					days = int(now.Sub(t).Hours()/24) + 1
					if days < 1 {
						days = 1
					}
				}
			}
			userStatesMu.Lock()
			if _, exists := userStates[uid]; !exists {
				userStates[uid] = &UserState{
					PrevScore: score, DaysSinceLast: days,
					EventCounts: make(map[string]int), EventValues: make(map[string]float64),
					RuleScores: make(map[string]float64), LastUpdated: now, Dirty: true,
				}
				count++
			}
			userStatesMu.Unlock()
		},
	)
	log.Printf("[INIT] %d명 이전 점수 유저 초기화", count)
}

func recoverTodayState() {
	today := time.Now().In(loc).Format("2006-01-02")
	log.Printf("[INIT] 오늘(%s) aggregation 기반 상태 복구 중...", today)

	// 1) 어제까지 점수가 있는 유저를 decay 적용하여 초기화
	initUsersFromPrevScores()

	rules := loadRules()

	// 2) 룰별 OpenSearch aggregation으로 유저별 집계값 조회
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		recoverRuleAgg(rule, today)
	}

	// 3) anomaly용: msgId별 유저별 이벤트 카운트 (baseline 비교용)
	recoverEventCounts(today)

	// 4) 전체 유저 점수 계산
	userStatesMu.Lock()
	for userID, state := range userStates {
		state.Dirty = true
		calculateStateScore(userID, state)
	}
	userStatesMu.Unlock()

	log.Printf("[INIT] %d명 유저 복구 완료 (aggregation)", len(userStates))
	saveScoresBatch()
}

// recoverRuleAgg: 단일 룰에 대해 유저별 집계값을 OpenSearch aggregation으로 조회
func recoverRuleAgg(rule Rule, today string) {
	esQuery := buildRuleESQuery(rule, today)
	if esQuery == nil {
		return
	}
	queryPart := esQuery["query"]

	var subAggs map[string]interface{}
	switch rule.Aggregate.Type {
	case "sum":
		if rule.Aggregate.Field == "" {
			return
		}
		subAggs = map[string]interface{}{"val": map[string]interface{}{"sum": map[string]interface{}{"field": resolveAggField(rule.Aggregate.Field)}}}
	case "cardinality":
		if rule.Aggregate.Field == "" {
			return
		}
		subAggs = map[string]interface{}{"val": map[string]interface{}{"cardinality": map[string]interface{}{"field": resolveAggField(rule.Aggregate.Field)}}}
	}

	count := 0
	compositeAgg(
		common.LogsIndexPattern(indexPrefix),
		"cefExtensions.suid.keyword",
		queryPart,
		subAggs,
		func(uid string, bucket map[string]interface{}) {
			userStatesMu.Lock()
			state := getOrCreateState(uid)
			switch rule.Aggregate.Type {
			case "sum", "cardinality":
				if val, ok := bucket["val"].(map[string]interface{}); ok {
					state.EventValues[rule.Name] = toFloat64(val["value"])
				}
			default:
				state.EventValues[rule.Name] = toFloat64(bucket["doc_count"])
			}
			userStatesMu.Unlock()
			count++
		},
	)
	log.Printf("[INIT] 룰 '%s': %d명 집계", rule.Name, count)
}

// recoverEventCounts: anomaly 계산용 — msgId별 유저별 이벤트 카운트
func recoverEventCounts(today string) {
	queryPart := map[string]interface{}{
		"range": map[string]interface{}{
			"@timestamp": map[string]interface{}{"gte": today, "lt": today + "||+1d"},
		},
	}
	var afterKey map[string]interface{}
	for {
		composite := map[string]interface{}{
			"size": compositePageSize,
			"sources": []interface{}{
				map[string]interface{}{"user": map[string]interface{}{"terms": map[string]interface{}{"field": "cefExtensions.suid.keyword"}}},
				map[string]interface{}{"msg": map[string]interface{}{"terms": map[string]interface{}{"field": "msgId.keyword"}}},
			},
		}
		if afterKey != nil {
			composite["after"] = afterKey
		}
		body, _ := json.Marshal(map[string]interface{}{
			"size": 0, "query": queryPart,
			"aggs": map[string]interface{}{"pairs": map[string]interface{}{"composite": composite}},
		})
		resp, err := httpClient.Post(fmt.Sprintf("%s/%s/_search", opensearchURL, common.LogsIndexPattern(indexPrefix)), "application/json", bytes.NewReader(body))
		if err != nil {
			break
		}
		var result struct {
			Aggregations struct {
				Pairs struct {
					AfterKey map[string]interface{} `json:"after_key"`
					Buckets  []struct {
						Key      map[string]interface{} `json:"key"`
						DocCount int                    `json:"doc_count"`
					} `json:"buckets"`
				} `json:"pairs"`
			} `json:"aggregations"`
		}
		json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()

		userStatesMu.Lock()
		for _, b := range result.Aggregations.Pairs.Buckets {
			uid, _ := b.Key["user"].(string)
			msgId, _ := b.Key["msg"].(string)
			if uid == "" || msgId == "" {
				continue
			}
			state := getOrCreateState(uid)
			state.EventCounts[msgId] = b.DocCount
		}
		userStatesMu.Unlock()

		if len(result.Aggregations.Pairs.Buckets) < compositePageSize {
			break
		}
		afterKey = result.Aggregations.Pairs.AfterKey
	}
}

// getOrCreateState: 유저 상태 가져오기 (없으면 생성). Lock 보유 상태에서 호출
func getOrCreateState(uid string) *UserState {
	state, exists := userStates[uid]
	if !exists {
		prevScore, days := getPrevScore(uid)
		state = &UserState{
			EventCounts:   make(map[string]int),
			EventValues:   make(map[string]float64),
			RuleScores:    make(map[string]float64),
			PrevScore:     prevScore,
			DaysSinceLast: days,
			LastUpdated:   time.Now(),
		}
		userStates[uid] = state
	}
	return state
}

// buildRuleESQuery: UEBA 룰의 match 조건을 OpenSearch bool 쿼리로 변환
func buildRuleESQuery(rule Rule, today string) map[string]interface{} {
	must := []interface{}{
		map[string]interface{}{"term": map[string]interface{}{"msgId.keyword": rule.Match.MsgID}},
		map[string]interface{}{"range": map[string]interface{}{
			"@timestamp": map[string]interface{}{"gte": today, "lt": today + "||+1d"},
		}},
	}

	for _, cond := range rule.Match.Conditions {
		clause := conditionToESClause(cond)
		if clause != nil {
			must = append(must, clause)
		}
	}

	logic := strings.ToLower(rule.Match.Logic)
	if logic == "or" && len(rule.Match.Conditions) > 1 {
		// OR: should 절로
		should := []interface{}{}
		for _, cond := range rule.Match.Conditions {
			if c := conditionToESClause(cond); c != nil {
				should = append(should, c)
			}
		}
		return map[string]interface{}{
			"query": map[string]interface{}{
				"bool": map[string]interface{}{
					"must": []interface{}{
						map[string]interface{}{"term": map[string]interface{}{"msgId.keyword": rule.Match.MsgID}},
						map[string]interface{}{"range": map[string]interface{}{
							"@timestamp": map[string]interface{}{"gte": today, "lt": today + "||+1d"},
						}},
					},
					"should":               should,
					"minimum_should_match": 1,
				},
			},
		}
	}

	return map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{"must": must},
		},
	}
}

// conditionToESClause: 단일 condition → OpenSearch 쿼리 절
func conditionToESClause(cond RuleCondition) interface{} {
	// hour: 가상 필드 → script query
	if cond.Field == "hour" {
		return hourToESClause(cond)
	}
	field := resolveESField(cond.Field)
	switch cond.Op {
	case "eq":
		return map[string]interface{}{"term": map[string]interface{}{field: cond.Value}}
	case "neq":
		return map[string]interface{}{"bool": map[string]interface{}{
			"must_not": []interface{}{map[string]interface{}{"term": map[string]interface{}{field: cond.Value}}},
		}}
	case "gt":
		return map[string]interface{}{"range": map[string]interface{}{field: map[string]interface{}{"gt": cond.Value}}}
	case "gte":
		return map[string]interface{}{"range": map[string]interface{}{field: map[string]interface{}{"gte": cond.Value}}}
	case "lt":
		return map[string]interface{}{"range": map[string]interface{}{field: map[string]interface{}{"lt": cond.Value}}}
	case "lte":
		return map[string]interface{}{"range": map[string]interface{}{field: map[string]interface{}{"lte": cond.Value}}}
	case "in":
		return map[string]interface{}{"terms": map[string]interface{}{field: cond.Value}}
	case "contains":
		return map[string]interface{}{"wildcard": map[string]interface{}{field: fmt.Sprintf("*%v*", cond.Value)}}
	}
	return nil
}

// hourToESClause: hour 가상 필드 → OpenSearch script query
func hourToESClause(cond RuleCondition) interface{} {
	h := "doc['@timestamp'].value.withZoneSameInstant(ZoneId.of('Asia/Seoul')).getHour()"
	switch cond.Op {
	case "eq":
		return scriptQ(fmt.Sprintf("%s == %v", h, cond.Value))
	case "neq":
		return scriptQ(fmt.Sprintf("%s != %v", h, cond.Value))
	case "gt":
		return scriptQ(fmt.Sprintf("%s > %v", h, cond.Value))
	case "gte":
		return scriptQ(fmt.Sprintf("%s >= %v", h, cond.Value))
	case "lt":
		return scriptQ(fmt.Sprintf("%s < %v", h, cond.Value))
	case "lte":
		return scriptQ(fmt.Sprintf("%s <= %v", h, cond.Value))
	case "in":
		vals, _ := json.Marshal(cond.Value)
		return scriptQ(fmt.Sprintf("%s.contains(%s)", string(vals), h))
	case "time_range":
		start, end := toInt(cond.Value, "start"), toInt(cond.Value, "end")
		if start > end {
			return scriptQ(fmt.Sprintf("%s >= %d || %s < %d", h, start, h, end))
		}
		return scriptQ(fmt.Sprintf("%s >= %d && %s < %d", h, start, h, end))
	}
	return nil
}

func toInt(v interface{}, key string) int {
	if m, ok := v.(map[string]interface{}); ok {
		if n, ok := m[key].(float64); ok {
			return int(n)
		}
	}
	return 0
}

func scriptQ(script string) interface{} {
	return map[string]interface{}{"script": map[string]interface{}{"script": script}}
}

// resolveESField: UEBA 필드명 → OpenSearch 필드 경로
func resolveESField(field string) string {
	if field == "@timestamp" {
		return field
	}
	top := map[string]bool{"msgId": true, "hostname": true, "eventType": true, "severity": true}
	if top[field] {
		return field + ".keyword"
	}
	return "cefExtensions." + field + ".keyword"
}

// resolveAggField: sum/cardinality 대상 필드 → OpenSearch 경로
func resolveAggField(field string) string {
	// 숫자 필드는 .keyword 불필요
	return "cefExtensions." + field
}

func loadAllBaselines() {
	log.Println("[INIT] Baseline 로드 중...")
	query := map[string]interface{}{"size": 10000, "query": map[string]interface{}{"match_all": map[string]interface{}{}}}
	body, _ := json.Marshal(query)
	resp, err := httpClient.Post(fmt.Sprintf("%s/%s/_search", opensearchURL, common.BaselinesIndex(indexPrefix)), "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("[WARN] Baseline 로드 실패: %v", err)
		return
	}
	defer resp.Body.Close()

	var result struct {
		Hits struct {
			Hits []struct {
				ID     string   `json:"_id"`
				Source Baseline `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	baselinesMu.Lock()
	for _, hit := range result.Hits.Hits {
		bl := hit.Source
		baselines[hit.ID] = &bl
	}
	baselinesMu.Unlock()
	log.Printf("[INIT] %d개 Baseline 로드 완료", len(baselines))
}

// getPrevScore는 최근 점수 인덱스를 탐색하여 (점수, 경과일수)를 반환한다.
// reference의 GetPreviousDayRiskScore와 동일한 역할.
func getPrevScore(userID string) (float64, int) {
	now := time.Now().In(loc)
	today := now.Format("2006-01-02")
	query := map[string]interface{}{
		"size": 1,
		"sort": []map[string]interface{}{{"@timestamp": "desc"}},
		"query": map[string]interface{}{"bool": map[string]interface{}{"must": []interface{}{
			map[string]interface{}{"term": map[string]interface{}{"userId": userID}},
			map[string]interface{}{"range": map[string]interface{}{"@timestamp": map[string]interface{}{"lt": today, "time_zone": "Asia/Seoul"}}},
		}}},
		"_source": []string{"riskScore", "@timestamp"},
	}
	body, _ := json.Marshal(query)
	resp, err := httpClient.Post(fmt.Sprintf("%s/%s/_search", opensearchURL, common.ScoresIndexPattern(indexPrefix)), "application/json", bytes.NewReader(body))
	if err != nil {
		return 0, 1
	}
	defer resp.Body.Close()
	var result struct {
		Hits struct {
			Hits []struct {
				Source struct {
					RiskScore float64 `json:"riskScore"`
					Timestamp string  `json:"@timestamp"`
				} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result.Hits.Hits) > 0 && result.Hits.Hits[0].Source.RiskScore > 0 {
		score := result.Hits.Hits[0].Source.RiskScore
		if t, err := time.Parse(time.RFC3339, result.Hits.Hits[0].Source.Timestamp); err == nil {
			days := int(now.Sub(t).Hours()/24) + 1
			if days < 1 {
				days = 1
			}
			return score, days
		}
		return score, 1
	}
	return 0, 1
}

// ===== 이벤트 처리 =====

func processEvent(data []byte) {
	var event map[string]interface{}
	if err := json.Unmarshal(data, &event); err != nil {
		return
	}

	userID, _ := event["userId"].(string)
	if userID == "" {
		if ext, ok := event["cefExtensions"].(map[string]interface{}); ok {
			userID, _ = ext["suid"].(string)
		}
	}
	if userID == "" {
		return
	}

	msgId, _ := event["msgId"].(string)
	if msgId == "" {
		return
	}

	checkDateRollover()

	// CEF Label 기반으로 attrs 파싱 → event["_attrs"]에 저장
	event["_attrs"] = parseCEFAttrs(event)

	rules := loadRules()

	userStatesMu.Lock()
	state, exists := userStates[userID]
	if !exists {
		prevScore, days := getPrevScore(userID)
		state = &UserState{
			EventCounts:   make(map[string]int),
			EventValues:   make(map[string]float64),
			PrevScore:     prevScore,
			DaysSinceLast: days,
		}
		userStates[userID] = state
	}

	// 기본 msgId 카운트 (baseline/anomaly용)
	state.EventCounts[msgId]++

	// 룰별 집계
	matched := false
	for _, rule := range rules {
		if !rule.Enabled || !matchEvent(event, rule) {
			continue
		}
		matched = true
		ruleKey := rule.Name

		switch rule.Aggregate.Type {
		case "sum":
			val := toFloat64(getFieldValue(event, rule.Aggregate.Field))
			state.EventValues[ruleKey] += val
		case "cardinality":
			val := fmt.Sprintf("%v", getFieldValue(event, rule.Aggregate.Field))
			cardKey := ruleKey + "::" + val
			if state.EventValues[cardKey] == 0 {
				state.EventValues[cardKey] = 1
				state.EventValues[ruleKey]++
			}
		default: // "count" 또는 빈값
			state.EventValues[ruleKey]++
		}
	}

	state.LastUpdated = time.Now()
	state.Dirty = true
	userStatesMu.Unlock()

	calculateStateScore(userID, state)
	pushToDashboard(userID, state)

	if matched {
		log.Printf("[EVENT] %s: %s (score=%.1f)", userID, msgId, state.RiskScore)
	}
}

// ===== 점수 계산 (인메모리, ES 쿼리 없음) =====

func effectiveDays(days int, mode string) int {
	if mode != "skip" || days <= 1 {
		return days
	}
	// 주말(토/일)을 경과 일수에서 제외
	today := time.Now().In(loc)
	count := 0
	for i := 1; i <= days; i++ {
		d := today.AddDate(0, 0, -i)
		wd := d.Weekday()
		if wd != time.Saturday && wd != time.Sunday {
			count++
		}
	}
	if count < 1 {
		count = 1
	}
	return count
}

func frequencyFunc(count float64, mode string) float64 {
	switch mode {
	case "log2":
		return math.Log2(1 + count)
	case "log10":
		return math.Log10(1 + count)
	case "linear":
		return count
	default: // "log" = ln
		return math.Log1p(count)
	}
}

func calculateStateScore(userID string, state *UserState) {
	cfg := loadConfig()
	rules := loadRules()

	var ruleScore float64
	ruleScores := make(map[string]float64)
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		val := state.EventValues[rule.Name]
		if val > 0 {
			s := rule.Weight * frequencyFunc(val, cfg.Anomaly.FrequencyFunction)
			ruleScore += s
			ruleScores[rule.Name] = math.Round(s*100) / 100
		}
	}

	// Anomaly: 룰에 등록된 msgId만 대상 (reference 방식)
	ruleMsgIDs := make(map[string]bool)
	for _, rule := range rules {
		if rule.Enabled {
			ruleMsgIDs[rule.Match.MsgID] = true
		}
	}

	var anomalyScore float64
	for msgID, count := range state.EventCounts {
		bl := getBaseline(userID, msgID)
		if bl == nil || bl.SampleDays < cfg.Anomaly.ColdStartMinDays {
			continue
		}
		stddev := math.Max(bl.Stddev, cfg.Anomaly.SigmaFloor)
		z := (float64(count) - bl.Mean) / stddev
		if z > cfg.Anomaly.ZThreshold {
			excess := cfg.Anomaly.Beta * (z - cfg.Anomaly.ZThreshold)
			if ruleMsgIDs[msgID] {
				anomalyScore += excess
			} else {
				// 룰 미등록 이벤트 급증 — 점수 반영 없이 로그만
				log.Printf("[ANOMALY-INFO] %s: %s 급증 (count=%d mean=%.1f z=%.1f)", userID, msgID, count, bl.Mean, z)
			}
		}
	}

	state.RuleScore = math.Round(ruleScore*100) / 100
	state.RuleScores = ruleScores
	state.AnomalyScore = math.Round(anomalyScore*100) / 100

	// Cold Start: baseline 데이터 부족 시 점수=0 (reference 동일)
	if isColdStart(userID, state, cfg) {
		state.ColdStart = true
		state.RiskScore = 0
		return
	}
	state.ColdStart = false

	// Decay: λ^days (weekend_mode=skip 시 주말 제외)
	days := state.DaysSinceLast
	if days < 1 {
		days = 1
	}
	days = effectiveDays(days, cfg.Decay.WeekendMode)
	decayed := state.PrevScore * math.Pow(cfg.Decay.Lambda, float64(days))
	state.RiskScore = math.Round((decayed+ruleScore+anomalyScore)*100) / 100
}

// isColdStart는 유저의 baseline 샘플일수가 cold_start_min_days 미만인지 확인한다.
func isColdStart(userID string, state *UserState, cfg *Config) bool {
	// 이전 점수가 있으면 cold start 아님
	if state.PrevScore > 0 {
		return false
	}
	baselinesMu.RLock()
	defer baselinesMu.RUnlock()
	maxDays := 0
	prefix := userID + "_"
	for k, bl := range baselines {
		if len(k) > len(prefix) && k[:len(prefix)] == prefix && bl.SampleDays > maxDays {
			maxDays = bl.SampleDays
		}
	}
	return maxDays < cfg.Anomaly.ColdStartMinDays
}

func getBaseline(userID, msgID string) *Baseline {
	baselinesMu.RLock()
	bl := baselines[userID+"_"+msgID]
	baselinesMu.RUnlock()
	return bl
}

// ===== 조건 평가 (이벤트 데이터로 직접 룰 매칭) =====

func matchEvent(event map[string]interface{}, rule Rule) bool {
	msgId, _ := event["msgId"].(string)
	if msgId != rule.Match.MsgID {
		return false
	}
	if len(rule.Match.Conditions) == 0 {
		return true
	}

	logic := rule.Match.Logic
	if logic == "" {
		logic = "and"
	}

	if logic == "and" {
		for _, cond := range rule.Match.Conditions {
			if !evaluateCondition(event, cond) {
				return false
			}
		}
		return true
	}
	// or
	for _, cond := range rule.Match.Conditions {
		if evaluateCondition(event, cond) {
			return true
		}
	}
	return false
}

func evaluateCondition(event map[string]interface{}, cond RuleCondition) bool {
	fieldValue := getFieldValue(event, cond.Field)
	if fieldValue == nil {
		return false
	}
	switch cond.Op {
	case "eq":
		return compareEqual(fieldValue, cond.Value)
	case "neq":
		return !compareEqual(fieldValue, cond.Value)
	case "gt":
		return compareNumeric(fieldValue, cond.Value) > 0
	case "gte":
		return compareNumeric(fieldValue, cond.Value) >= 0
	case "lt":
		return compareNumeric(fieldValue, cond.Value) < 0
	case "lte":
		return compareNumeric(fieldValue, cond.Value) <= 0
	case "contains":
		return containsString(fieldValue, cond.Value)
	case "in":
		return inArray(fieldValue, cond.Value)
	case "time_range":
		h := int(toFloat64(fieldValue))
		start, end := toInt(cond.Value, "start"), toInt(cond.Value, "end")
		if start > end {
			return h >= start || h < end
		}
		return h >= start && h < end
	}
	return false
}

func getFieldValue(event map[string]interface{}, field string) interface{} {
	// hour 가상 필드: @timestamp에서 추출
	if field == "hour" {
		if ts, ok := event["@timestamp"].(string); ok {
			if t, err := time.Parse(time.RFC3339, ts); err == nil {
				return float64(t.In(loc).Hour())
			}
		}
		return nil
	}
	// 1. attrs (Label 기반 파싱 결과) 에서 먼저 찾기
	if attrs, ok := event["_attrs"].(map[string]interface{}); ok {
		if val, ok := attrs[field]; ok {
			return val
		}
	}
	// 2. 이벤트 최상위
	if val, ok := event[field]; ok {
		return val
	}
	// 3. cefExtensions 내부
	if ext, ok := event["cefExtensions"].(map[string]interface{}); ok {
		if val, ok := ext[field]; ok {
			return val
		}
	}
	return nil
}

// parseCEFAttrs: 변환 토픽에서는 label 이름이 이미 cefExtensions에 존재
// 기본 CEF 필드도 attrs에 복사
func parseCEFAttrs(event map[string]interface{}) map[string]interface{} {
	attrs := make(map[string]interface{})
	ext, _ := event["cefExtensions"].(map[string]interface{})
	if ext != nil {
		for k, v := range ext {
			attrs[k] = v
		}
	}
	if v, ok := attrs["act"]; ok {
		attrs["action"] = v
	}
	return attrs
}

func compareEqual(a, b interface{}) bool {
	// 両方を数値変換できるなら数値比較
	fa, okA := tryFloat64(a)
	fb, okB := tryFloat64(b)
	if okA && okB {
		return fa == fb
	}
	switch va := a.(type) {
	case string:
		return va == fmt.Sprintf("%v", b)
	case bool:
		if vb, ok := b.(bool); ok {
			return va == vb
		}
		if vb, ok := b.(string); ok {
			return (va && vb == "true") || (!va && vb == "false")
		}
	}
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

// tryFloat64: 数値変換を試みる。文字列の場合もParseFloat
func tryFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case string:
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

func compareNumeric(a, b interface{}) int {
	va, vb := toFloat64(a), toFloat64(b)
	if va > vb {
		return 1
	} else if va < vb {
		return -1
	}
	return 0
}

func toFloat64(v interface{}) float64 {
	f, _ := tryFloat64(v)
	return f
}

func containsString(a, b interface{}) bool {
	return strings.Contains(strings.ToLower(fmt.Sprintf("%v", a)), strings.ToLower(fmt.Sprintf("%v", b)))
}

func inArray(val interface{}, arr interface{}) bool {
	if s, ok := arr.([]interface{}); ok {
		for _, item := range s {
			if compareEqual(val, item) {
				return true
			}
		}
	}
	return false
}

// ===== 자정 롤오버 =====

func checkDateRollover() {
	today := time.Now().In(loc).Format("2006-01-02")
	currentDateMu.Lock()
	defer currentDateMu.Unlock()
	if currentDate == today {
		return
	}
	log.Printf("[ROLLOVER] 날짜 변경: %s → %s", currentDate, today)

	// 롤오버 전 현재 점수 저장 (어제 날짜 인덱스에)
	saveScoresBatchForDate(strings.ReplaceAll(currentDate, "-", "."))

	currentDate = today
	userStatesMu.Lock()
	for _, state := range userStates {
		state.PrevScore = state.RiskScore
		state.DaysSinceLast = 1
		state.RuleScore = 0
		state.AnomalyScore = 0
		state.RuleScores = nil
		state.EventCounts = make(map[string]int)
		state.EventValues = make(map[string]float64)
		state.Dirty = true
	}
	userStatesMu.Unlock()

	go func() {
		updateBaselines()
		today := time.Now().In(loc).Format("2006-01-02")
		meta, _ := json.Marshal(map[string]string{"updated_at": today})
		req, _ := http.NewRequest("PUT",
			fmt.Sprintf("%s/%s/_doc/baseline_meta", opensearchURL, common.SettingsIndex(indexPrefix)),
			bytes.NewReader(meta))
		req.Header.Set("Content-Type", "application/json")
		if r, err := httpClient.Do(req); err == nil {
			r.Body.Close()
		}
	}()
}

// ===== 설정/규칙 로드 =====

func loadConfig() *Config {
	configMu.RLock()
	if configCache != nil {
		configMu.RUnlock()
		return configCache
	}
	configMu.RUnlock()

	configMu.Lock()
	defer configMu.Unlock()

	configCache = &Config{
		Anomaly: AnomalyConfig{ZThreshold: 2.0, Beta: 10, SigmaFloor: 0.5, ColdStartMinDays: 7, BaselineWindow: 7, FrequencyFunction: "log"},
		Decay:   DecayConfig{Lambda: 0.9},
		Tiers:   TierConfig{GreenMax: 40, YellowMax: 99},
	}

	resp, err := httpClient.Get(fmt.Sprintf("%s/%s/_doc/settings", opensearchURL, common.SettingsIndex(indexPrefix)))
	if err == nil && resp.StatusCode == 200 {
		var result struct {
			Source Config `json:"_source"`
		}
		json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		configCache = &result.Source
	}
	return configCache
}

func loadRules() []Rule {
	rulesMu.RLock()
	if rulesCache != nil {
		rulesMu.RUnlock()
		return rulesCache
	}
	rulesMu.RUnlock()

	rulesMu.Lock()
	defer rulesMu.Unlock()

	query := map[string]interface{}{"size": maxRulesSize, "query": map[string]interface{}{
		"bool": map[string]interface{}{
			"must": []map[string]interface{}{
				{"term": map[string]interface{}{"enabled": true}},
				{"term": map[string]interface{}{"ueba.enabled": true}},
			},
		},
	}}
	body, _ := json.Marshal(query)
	resp, err := httpClient.Post(fmt.Sprintf("%s/%s/_search", opensearchURL, common.RulesIndex(indexPrefix)), "application/json", bytes.NewReader(body))
	if err != nil {
		return rulesCache
	}
	defer resp.Body.Close()

	// raw JSON으로 디코딩 (struct 필드 누락 방지)
	var rawResult struct {
		Hits struct {
			Hits []struct {
				ID  string          `json:"_id"`
				Raw json.RawMessage `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	json.Unmarshal(bodyBytes, &rawResult)

	rulesCache = make([]Rule, 0, len(rawResult.Hits.Hits))
	for _, hit := range rawResult.Hits.Hits {
		var rule Rule
		json.Unmarshal(hit.Raw, &rule)
		if rule.Name == "" {
			rule.Name = hit.ID
		}
		rule.ID = hit.ID

		rulesCache = append(rulesCache, rule)
	}
	log.Printf("[CONFIG] %d개 UEBA 규칙 로드", len(rulesCache))
	return rulesCache
}

// ===== Baseline 업데이트 (자정) =====

func updateBaselines() {
	log.Println("[BASELINE] 업데이트 시작")
	cfg := loadConfig()
	window := cfg.Anomaly.BaselineWindow
	yesterday := time.Now().In(loc).AddDate(0, 0, -1).Format("2006-01-02")
	startDate := time.Now().In(loc).AddDate(0, 0, -window).Format("2006-01-02")

	query := map[string]interface{}{
		"size": 0,
		"query": map[string]interface{}{
			"range": map[string]interface{}{
				"@timestamp": map[string]interface{}{"gte": startDate, "lte": yesterday},
			},
		},
		"aggs": map[string]interface{}{
			"by_user": map[string]interface{}{
				"terms": map[string]interface{}{"field": "userId", "size": 100000},
				"aggs": map[string]interface{}{
					"by_msgId": map[string]interface{}{
						"terms": map[string]interface{}{"field": "msgId", "size": 50},
						"aggs": map[string]interface{}{
							"daily": map[string]interface{}{
								"date_histogram": map[string]interface{}{
									"field": "@timestamp", "calendar_interval": "day",
								},
							},
						},
					},
				},
			},
		},
	}

	body, _ := json.Marshal(query)
	resp, err := httpClient.Post(fmt.Sprintf("%s/%s/_search", opensearchURL, common.LogsIndexPattern(indexPrefix)), "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("[BASELINE] 조회 실패: %v", err)
		return
	}
	defer resp.Body.Close()

	var result struct {
		Aggregations struct {
			ByUser struct {
				Buckets []struct {
					Key     string `json:"key"`
					ByMsgId struct {
						Buckets []struct {
							Key   string `json:"key"`
							Daily struct {
								Buckets []struct {
									DocCount int `json:"doc_count"`
								} `json:"buckets"`
							} `json:"daily"`
						} `json:"buckets"`
					} `json:"by_msgId"`
				} `json:"buckets"`
			} `json:"by_user"`
		} `json:"aggregations"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	var bulkBody bytes.Buffer
	count := 0

	baselinesMu.Lock()
	for _, ub := range result.Aggregations.ByUser.Buckets {
		for _, mb := range ub.ByMsgId.Buckets {
			counts := make([]float64, 0)
			for _, day := range mb.Daily.Buckets {
				counts = append(counts, float64(day.DocCount))
			}
			if len(counts) == 0 {
				continue
			}
			mean, stddev := calcMeanStddev(counts)
			bl := &Baseline{Mean: mean, Stddev: stddev, SampleDays: len(counts)}
			key := ub.Key + "_" + mb.Key
			baselines[key] = bl

			bulkBody.WriteString(fmt.Sprintf(`{"index":{"_index":"%s","_id":"%s"}}`, common.BaselinesIndex(indexPrefix), key))
			bulkBody.WriteString("\n")
			blJSON, _ := json.Marshal(bl)
			bulkBody.Write(blJSON)
			bulkBody.WriteString("\n")
			count++
		}
	}
	baselinesMu.Unlock()

	if bulkBody.Len() > 0 {
		httpClient.Post(opensearchURL+"/_bulk", "application/x-ndjson", &bulkBody)
	}
	log.Printf("[BASELINE] %d개 업데이트 완료", count)
}

func calcMeanStddev(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(len(values))
	var variance float64
	for _, v := range values {
		variance += (v - mean) * (v - mean)
	}
	return mean, math.Sqrt(variance / float64(len(values)))
}

// ===== 점수 저장 (10분 배치) =====

func saveScoresBatch() {
	saveScoresBatchForDate(time.Now().In(loc).Format("2006.01.02"))
}

func saveScoresBatchForDate(day string) {
	userStatesMu.Lock()
	var bulkBody bytes.Buffer
	cfg := loadConfig()
	count := 0

	for userID, state := range userStates {
		if !state.Dirty {
			continue
		}
		days := state.DaysSinceLast
		if days < 1 {
			days = 1
		}
		days = effectiveDays(days, cfg.Decay.WeekendMode)
		decayed := state.PrevScore * math.Pow(cfg.Decay.Lambda, float64(days))
		score := Score{
			UserID:       userID,
			RiskScore:    state.RiskScore,
			RiskLevel:    classifyRisk(state.RiskScore, cfg),
			Status:       getUserStatus(userID, state, cfg),
			RuleScore:    state.RuleScore,
			RuleScores:   state.RuleScores,
			AnomalyScore: state.AnomalyScore,
			DailyScore:   state.RuleScore + state.AnomalyScore,
			DecayedPrev:  math.Round(decayed*100) / 100,
			PrevScore:    state.PrevScore,
			EventCounts:  state.EventCounts,
			EventValues:  state.EventValues,
			Timestamp:    time.Now().In(loc).Format(time.RFC3339),
		}
		bulkBody.WriteString(fmt.Sprintf(`{"index":{"_index":"%s","_id":"%s_%s"}}`, common.DailyScoresIndex(indexPrefix, day), userID, time.Now().In(loc).Format("15")))
		bulkBody.WriteString("\n")
		scoreJSON, _ := json.Marshal(score)
		bulkBody.Write(scoreJSON)
		bulkBody.WriteString("\n")
		state.Dirty = false
		count++
	}
	userStatesMu.Unlock()

	if bulkBody.Len() > 0 {
		httpClient.Post(opensearchURL+"/_bulk", "application/x-ndjson", &bulkBody)
		log.Printf("[SAVE] %d명 점수 저장", count)
	}
}

// ===== 대시보드 푸시 =====

func pushToDashboard(userID string, state *UserState) {
	cfg := loadConfig()
	data := map[string]interface{}{
		"userId": userID, "riskScore": state.RiskScore,
		"riskLevel": classifyRisk(state.RiskScore, cfg), "prevScore": state.PrevScore,
	}
	body, _ := json.Marshal(data)
	httpClient.Post(dashboardURL+"/api/ueba/push", "application/json", bytes.NewReader(body))
}

// ===== HTTP API =====

// esQuery: OpenSearch에 POST _search 요청
func esQuery(index string, body map[string]interface{}) (map[string]interface{}, error) {
	b, _ := json.Marshal(body)
	resp, err := httpClient.Post(opensearchURL+"/"+index+"/_search", "application/json", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	return result, nil
}

// esRequest: OpenSearch에 임의 HTTP 요청
func esRequest(method, path string, body interface{}) (map[string]interface{}, error) {
	var reqBody *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewReader(b)
	} else {
		reqBody = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, opensearchURL+path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	return result, nil
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request) (map[string]interface{}, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	var data map[string]interface{}
	err = json.Unmarshal(body, &data)
	return data, err
}

// ===== Export Functions =====

func GetStats() (users, baselineCount, rules, events int) {
	userStatesMu.RLock()
	users = len(userStates)
	for _, s := range userStates {
		for _, c := range s.EventCounts {
			events += c
		}
	}
	userStatesMu.RUnlock()
	baselinesMu.RLock()
	baselineCount = len(baselines)
	baselinesMu.RUnlock()
	rules = len(loadRules())
	return
}

func GetHealthThresholds() (warn, crit float64) {
	return healthWarnMB, healthCritMB
}

func GetCurrentDate() string {
	currentDateMu.RLock()
	defer currentDateMu.RUnlock()
	return currentDate
}

func GetConfig() *Config {
	configMu.RLock()
	defer configMu.RUnlock()
	return configCache
}

func getUserStatus(uid string, state *UserState, cfg *Config) string {
	if state.ColdStart {
		return "cold_start"
	}
	baselinesMu.RLock()
	hasBaseline := false
	prefix := uid + "_"
	for k := range baselines {
		if len(k) > len(prefix) && k[:len(prefix)] == prefix {
			hasBaseline = true
			break
		}
	}
	baselinesMu.RUnlock()
	if !hasBaseline {
		return "no_baseline"
	}
	return "active"
}

func GetAllUsers() []map[string]interface{} {
	userStatesMu.RLock()
	defer userStatesMu.RUnlock()
	cfg := loadConfig()

	users := make([]map[string]interface{}, 0, len(userStates))
	for uid, state := range userStates {
		users = append(users, map[string]interface{}{
			"userId":       uid,
			"riskScore":    state.RiskScore,
			"riskLevel":    classifyRisk(state.RiskScore, cfg),
			"ruleScore":    state.RuleScore,
			"anomalyScore": state.AnomalyScore,
			"prevScore":    state.PrevScore,
			"coldStart":    state.ColdStart,
			"status":       getUserStatus(uid, state, cfg),
			"lastUpdated":  state.LastUpdated,
		})
	}
	return users
}

func GetUser(userID string) map[string]interface{} {
	userStatesMu.RLock()
	state, exists := userStates[userID]
	userStatesMu.RUnlock()

	if !exists {
		return nil
	}

	return map[string]interface{}{
		"userId":        userID,
		"riskScore":     state.RiskScore,
		"riskLevel":     classifyRisk(state.RiskScore, loadConfig()),
		"ruleScore":     state.RuleScore,
		"ruleScores":    state.RuleScores,
		"anomalyScore":  state.AnomalyScore,
		"eventCounts":   state.EventCounts,
		"eventValues":   state.EventValues,
		"prevScore":     state.PrevScore,
		"daysSinceLast": state.DaysSinceLast,
		"coldStart":     state.ColdStart,
		"lastUpdated":   state.LastUpdated,
	}
}

func GetRules() []Rule {
	return loadRules()
}

// GetRulesRaw: 프론트엔드용 — OpenSearch 원본 JSON 반환 (struct 직렬화 누락 방지)
func GetRulesRaw() []map[string]interface{} {
	query := map[string]interface{}{"size": maxRulesSize, "query": map[string]interface{}{
		"bool": map[string]interface{}{
			"must": []map[string]interface{}{
				{"term": map[string]interface{}{"enabled": true}},
				{"term": map[string]interface{}{"ueba.enabled": true}},
			},
		},
	}}
	body, _ := json.Marshal(query)
	resp, err := httpClient.Post(fmt.Sprintf("%s/%s/_search", opensearchURL, common.RulesIndex(indexPrefix)), "application/json", bytes.NewReader(body))
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var result struct {
		Hits struct {
			Hits []struct {
				ID     string                 `json:"_id"`
				Source map[string]interface{} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	rules := make([]map[string]interface{}, 0, len(result.Hits.Hits))
	for _, hit := range result.Hits.Hits {
		r := hit.Source
		r["id"] = hit.ID
		delete(r, "_id")
		rules = append(rules, r)
	}
	return rules
}

func CreateRule(data map[string]interface{}) (string, error) {
	if errs := validateRule(data); len(errs) > 0 {
		return "", fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	delete(data, "_id")
	delete(data, "id")
	data["createdAt"] = time.Now().In(loc).Format(time.RFC3339)
	result, err := esRequest("POST", fmt.Sprintf("/%s/_doc", common.RulesIndex(indexPrefix)), data)
	if err != nil {
		return "", err
	}
	reloadAndReprocess()
	id, _ := result["_id"].(string)
	return id, nil
}

func UpdateRule(id string, data map[string]interface{}) error {
	delete(data, "_id")
	delete(data, "id")
	_, err := esRequest("PUT", fmt.Sprintf("/%s/_doc/%s", common.RulesIndex(indexPrefix), id), data)
	if err != nil {
		return err
	}
	reloadAndReprocess()
	return nil
}

func DeleteRule(id string) error {
	_, err := esRequest("DELETE", fmt.Sprintf("/%s/_doc/%s", common.RulesIndex(indexPrefix), id), nil)
	if err != nil {
		return err
	}
	reloadAndReprocess()
	return nil
}

// reloadAndReprocess: 룰 변경 후 캐시 갱신 + 해당 룰만 재집계
func reloadAndReprocess() {
	ReloadCache()
	go func() {
		today := time.Now().In(loc).Format("2006-01-02")
		rules := loadRules()
		log.Printf("[RULE] 룰 변경 → %d개 룰 재집계 시작", len(rules))

		// 룰별 EventValues 초기화 후 재집계
		userStatesMu.Lock()
		for _, state := range userStates {
			for _, rule := range rules {
				delete(state.EventValues, rule.Name)
			}
		}
		userStatesMu.Unlock()

		for _, rule := range rules {
			if !rule.Enabled {
				continue
			}
			recoverRuleAgg(rule, today)
		}

		// 전체 유저 점수 재계산
		userStatesMu.Lock()
		for userID, state := range userStates {
			state.Dirty = true
			calculateStateScore(userID, state)
		}
		userStatesMu.Unlock()

		saveScoresBatch()
		log.Printf("[RULE] 재집계 완료 (%d명)", len(userStates))
	}()
}

func validateRule(data map[string]interface{}) []string {
	var errs []string
	if _, ok := data["name"].(string); !ok || data["name"] == "" {
		errs = append(errs, "name 필수")
	}
	match, _ := data["match"].(map[string]interface{})
	if match == nil {
		errs = append(errs, "match 필수")
		return errs
	}
	if _, ok := match["msgId"].(string); !ok || match["msgId"] == "" {
		errs = append(errs, "match.msgId 필수")
	}
	validOps := map[string]bool{"eq": true, "neq": true, "gt": true, "gte": true, "lt": true, "lte": true, "contains": true, "in": true, "time_range": true}
	if conds, ok := match["conditions"].([]interface{}); ok {
		for i, raw := range conds {
			c, _ := raw.(map[string]interface{})
			if c == nil {
				errs = append(errs, fmt.Sprintf("conditions[%d]: 객체 아님", i))
				continue
			}
			if f, _ := c["field"].(string); f == "" {
				errs = append(errs, fmt.Sprintf("conditions[%d]: field 필수", i))
			}
			op, _ := c["op"].(string)
			if !validOps[op] {
				errs = append(errs, fmt.Sprintf("conditions[%d]: 잘못된 op '%s'", i, op))
			}
			if _, exists := c["value"]; !exists {
				errs = append(errs, fmt.Sprintf("conditions[%d]: value 필수", i))
			}
			if op == "gt" || op == "gte" || op == "lt" || op == "lte" {
				if _, ok := tryFloat64(c["value"]); !ok {
					errs = append(errs, fmt.Sprintf("conditions[%d]: %s 연산자에 숫자 변환 불가한 value", i, op))
				}
			}
		}
	}
	if agg, ok := data["aggregate"].(map[string]interface{}); ok {
		t, _ := agg["type"].(string)
		if t != "" && t != "count" && t != "sum" && t != "cardinality" {
			errs = append(errs, fmt.Sprintf("aggregate.type '%s' 잘못됨 (count/sum/cardinality)", t))
		}
		if t == "sum" || t == "cardinality" {
			if f, _ := agg["field"].(string); f == "" {
				errs = append(errs, fmt.Sprintf("aggregate.type=%s 시 field 필수", t))
			}
		}
	}
	return errs
}

func ValidateRule(data map[string]interface{}) []string {
	return validateRule(data)
}

func ReloadCache() {
	configMu.Lock()
	configCache = nil
	configMu.Unlock()
	rulesMu.Lock()
	rulesCache = nil
	rulesMu.Unlock()
	loadConfig()
	loadRules()
}

func TriggerBaseline() {
	go updateBaselines()
}

func TriggerSave() {
	saveScoresBatch()
}

func GetUserHistory(userID string) map[string]interface{} {
	now := time.Now().In(loc)
	today := now.Format("2006-01-02")

	daily, _ := esQuery(common.ScoresIndexPattern(indexPrefix), map[string]interface{}{
		"size": 0,
		"query": map[string]interface{}{"bool": map[string]interface{}{"must": []interface{}{
			map[string]interface{}{"term": map[string]interface{}{"userId": userID}},
			map[string]interface{}{"range": map[string]interface{}{"@timestamp": map[string]interface{}{"gte": "now-6d/d", "lte": "now", "time_zone": "Asia/Seoul"}}},
		}}},
		"aggs": map[string]interface{}{"daily": map[string]interface{}{
			"date_histogram": map[string]interface{}{"field": "@timestamp", "calendar_interval": "day", "format": "MM-dd", "time_zone": "Asia/Seoul", "min_doc_count": 0,
				"extended_bounds": map[string]interface{}{"min": "now-6d/d", "max": "now/d"}},
			"aggs": map[string]interface{}{"max_score": map[string]interface{}{"max": map[string]interface{}{"field": "riskScore"}}},
		}},
	})
	var dailyData []map[string]interface{}
	if aggs, ok := daily["aggregations"].(map[string]interface{}); ok {
		if d, ok := aggs["daily"].(map[string]interface{}); ok {
			if buckets, ok := d["buckets"].([]interface{}); ok {
				for _, bkt := range buckets {
					b, _ := bkt.(map[string]interface{})
					label, _ := b["key_as_string"].(string)
					ms, _ := b["max_score"].(map[string]interface{})
					score := 0.0
					if v, ok := ms["value"].(float64); ok {
						score = v
					}
					dailyData = append(dailyData, map[string]interface{}{"date": label, "label": label, "score": score})
				}
			}
		}
	}

	hourly, _ := esQuery(common.ScoresIndexPattern(indexPrefix), map[string]interface{}{
		"size": 0,
		"query": map[string]interface{}{"bool": map[string]interface{}{"must": []interface{}{
			map[string]interface{}{"term": map[string]interface{}{"userId": userID}},
			map[string]interface{}{"range": map[string]interface{}{"@timestamp": map[string]interface{}{"gte": today + "T00:00:00+09:00", "lte": today + "T23:59:59+09:00"}}},
			map[string]interface{}{"exists": map[string]interface{}{"field": "ruleScores"}},
		}}},
		"aggs": map[string]interface{}{"hourly": map[string]interface{}{
			"date_histogram": map[string]interface{}{"field": "@timestamp", "calendar_interval": "hour", "format": "HH", "time_zone": "Asia/Seoul"},
			"aggs": map[string]interface{}{
				"ruleScore":    map[string]interface{}{"max": map[string]interface{}{"field": "ruleScore"}},
				"anomalyScore": map[string]interface{}{"max": map[string]interface{}{"field": "anomalyScore"}},
				"decayScore":   map[string]interface{}{"max": map[string]interface{}{"field": "decayedPrev"}},
			},
		}},
	})
	var hourlyData []map[string]interface{}
	if aggs, ok := hourly["aggregations"].(map[string]interface{}); ok {
		if h, ok := aggs["hourly"].(map[string]interface{}); ok {
			if buckets, ok := h["buckets"].([]interface{}); ok {
				for _, bkt := range buckets {
					b, _ := bkt.(map[string]interface{})
					hourStr, _ := b["key_as_string"].(string)
					hr, _ := strconv.Atoi(hourStr)
					getVal := func(key string) float64 {
						if m, ok := b[key].(map[string]interface{}); ok {
							if v, ok := m["value"].(float64); ok {
								return v
							}
						}
						return 0
					}
					hourlyData = append(hourlyData, map[string]interface{}{
						"hour": hr, "ruleScore": getVal("ruleScore"), "anomalyScore": getVal("anomalyScore"), "decayScore": getVal("decayScore"),
					})
				}
			}
		}
	}

	return map[string]interface{}{"daily": dailyData, "hourly": hourlyData}
}

func GetUserHourly(userID string) []map[string]interface{} {
	result, err := esQuery(common.LogsIndexPattern(indexPrefix), map[string]interface{}{
		"size": 0,
		"query": map[string]interface{}{"bool": map[string]interface{}{"must": []interface{}{
			map[string]interface{}{"term": map[string]interface{}{"userId": userID}},
			map[string]interface{}{"range": map[string]interface{}{"@timestamp": map[string]interface{}{"gte": "now/d"}}},
		}}},
		"aggs": map[string]interface{}{"hourly": map[string]interface{}{
			"date_histogram": map[string]interface{}{"field": "@timestamp", "calendar_interval": "hour", "format": "HH"},
		}},
	})
	if err != nil {
		return nil
	}
	var data []map[string]interface{}
	if aggs, ok := result["aggregations"].(map[string]interface{}); ok {
		if h, ok := aggs["hourly"].(map[string]interface{}); ok {
			if buckets, ok := h["buckets"].([]interface{}); ok {
				for _, bkt := range buckets {
					b, _ := bkt.(map[string]interface{})
					hourStr, _ := b["key_as_string"].(string)
					hr, _ := strconv.Atoi(hourStr)
					cnt, _ := b["doc_count"].(float64)
					data = append(data, map[string]interface{}{"hour": hr, "count": int(cnt)})
				}
			}
		}
	}
	return data
}

func GetUserScores(draw, start, length int, search, sortField, orderDir string) map[string]interface{} {
	if length <= 0 {
		length = 10
	}
	if orderDir == "" {
		orderDir = "desc"
	}

	type userRow struct {
		UserID   string
		Score    float64
		Level    string
		Features map[string]interface{}
	}
	var users []userRow

	compositeAgg(
		common.ScoresIndexPattern(indexPrefix),
		"userId.keyword",
		map[string]interface{}{"match_all": map[string]interface{}{}},
		map[string]interface{}{
			"recent": map[string]interface{}{
				"top_hits": map[string]interface{}{"size": 1, "sort": []map[string]interface{}{{"@timestamp": "desc"}}},
			},
		},
		func(uid string, bucket map[string]interface{}) {
			recent, _ := bucket["recent"].(map[string]interface{})
			hits, _ := recent["hits"].(map[string]interface{})
			hitArr, _ := hits["hits"].([]interface{})
			if len(hitArr) == 0 {
				return
			}
			h, _ := hitArr[0].(map[string]interface{})
			src, _ := h["_source"].(map[string]interface{})
			realUID, _ := src["userId"].(string)
			if realUID == "" {
				realUID = uid
			}
			if search != "" && !strings.Contains(strings.ToLower(realUID), strings.ToLower(search)) {
				return
			}
			score, _ := src["riskScore"].(float64)
			level, _ := src["riskLevel"].(string)
			features, _ := src["features"].(map[string]interface{})
			users = append(users, userRow{realUID, score, level, features})
		},
	)

	// 정렬
	if sortField == "riskScore" || sortField == "" {
		for i := 0; i < len(users); i++ {
			for j := i + 1; j < len(users); j++ {
				if (orderDir == "desc" && users[j].Score > users[i].Score) || (orderDir == "asc" && users[j].Score < users[i].Score) {
					users[i], users[j] = users[j], users[i]
				}
			}
		}
	}

	total := len(users)
	end := start + length
	if end > total {
		end = total
	}
	if start > total {
		start = total
	}
	page := users[start:end]

	data := make([][]interface{}, len(page))
	for i, u := range page {
		data[i] = []interface{}{u.UserID, u.Score, u.Level, u.Features}
	}
	return map[string]interface{}{"draw": draw, "recordsTotal": total, "recordsFiltered": total, "data": data}
}

func GetSettings() map[string]interface{} {
	result, err := esRequest("GET", fmt.Sprintf("/%s/_doc/settings", common.SettingsIndex(indexPrefix)), nil)
	if err != nil {
		return map[string]interface{}{}
	}
	cfg, _ := result["_source"].(map[string]interface{})
	if cfg == nil {
		cfg = map[string]interface{}{}
	}

	rulesResult, _ := esQuery(common.RulesIndex(indexPrefix), map[string]interface{}{
		"size":  maxRulesSize,
		"query": map[string]interface{}{"term": map[string]interface{}{"ueba.enabled": true}},
	})
	weights := map[string]interface{}{}
	if hits, ok := rulesResult["hits"].(map[string]interface{}); ok {
		if hitArr, ok := hits["hits"].([]interface{}); ok {
			for _, h := range hitArr {
				hit, _ := h.(map[string]interface{})
				id, _ := hit["_id"].(string)
				src, _ := hit["_source"].(map[string]interface{})
				weight := 0.0
				if w, ok := src["weight"].(float64); ok {
					weight = w
				}
				msgId := ""
				if m, ok := src["match"].(map[string]interface{}); ok {
					msgId, _ = m["msgId"].(string)
				}
				ruleName, _ := src["name"].(string)
				if ruleName == "" {
					ruleName = id
				}
				weights[id] = map[string]interface{}{"name": ruleName, "weight": weight, "msgId": msgId}
			}
		}
	}
	cfg["weights"] = weights
	return cfg
}

func SaveSettings(data map[string]interface{}) (map[string]interface{}, error) {
	data["updated_at"] = time.Now().In(loc).Format(time.RFC3339)

	if weights, ok := data["weights"].(map[string]interface{}); ok {
		for name, wv := range weights {
			wm, _ := wv.(map[string]interface{})
			weight := 0.0
			if w, ok := wm["weight"].(float64); ok {
				weight = w
			}
			esRequest("POST", fmt.Sprintf("/%s/_update/%s", common.RulesIndex(indexPrefix), name), map[string]interface{}{
				"doc": map[string]interface{}{"weight": weight},
			})
		}
		delete(data, "weights")
	}

	result, err := esRequest("PUT", fmt.Sprintf("/%s/_doc/settings", common.SettingsIndex(indexPrefix)), data)
	if err != nil {
		return nil, err
	}
	ReloadCache()
	return result, nil
}

// ===== Kafka Consumer =====

func startKafkaConsumer() {
	topics := strings.Split(kafkaEventTopics, ",")
	if len(topics) == 1 && topics[0] == "" {
		log.Println("[KAFKA] 구독할 UEBA 토픽이 설정되지 않았습니다. (KAFKA_EVENT_TOPICS)")
		return
	}

	config := sarama.NewConfig()
	config.Consumer.Offsets.Initial = sarama.OffsetNewest

	consumer, err := sarama.NewConsumer([]string{kafkaBootstrap}, config)
	if err != nil {
		log.Fatalf("[KAFKA] 연결 실패: %v", err)
	}
	defer consumer.Close()

	log.Printf("[KAFKA] Consumer 시작: %v", topics)

	for _, topic := range topics {
		partitions, err := consumer.Partitions(topic)
		if err != nil {
			continue
		}
		for _, p := range partitions {
			pc, err := consumer.ConsumePartition(topic, p, sarama.OffsetNewest)
			if err != nil {
				continue
			}
			go func(pc sarama.PartitionConsumer) {
				for msg := range pc.Messages() {
					processEvent(msg.Value)
				}
			}(pc)
		}
	}
	select {}
}

// ===== Main =====

func StartProcessor(cfg *config.Config) {
	opensearchURL = cfg.OpenSearch.URL
	kafkaBootstrap = cfg.Kafka.Bootstrap
	kafkaEventTopics = cfg.Kafka.EventTopics
	dashboardURL = cfg.UEBA.DashboardURL
	timezone = cfg.Timezone
	indexPrefix = cfg.IndexPrefix
	healthWarnMB = cfg.UEBA.HealthWarnMB
	healthCritMB = cfg.UEBA.HealthCritMB

	osClient = common.NewOSClient(opensearchURL)

	log.Printf("[UEBA] 프로세서 시작")
	log.Printf("[UEBA] OpenSearch: %s", opensearchURL)
	log.Printf("[UEBA] Kafka: %s", kafkaBootstrap)

	var err error
	loc, err = time.LoadLocation(timezone)
	if err != nil {
		loc = time.FixedZone("KST", 9*60*60)
	}

	initialize()
	startKafkaConsumer()
}
