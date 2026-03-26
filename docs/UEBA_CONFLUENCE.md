# UEBA (User Entity Behavior Analytics) 시스템

## 1. 개요

### 1.1 UEBA란?

UEBA(User Entity Behavior Analytics)는 사용자의 행동 패턴을 분석하여 내부 위협을 탐지하는 기술입니다.

| 항목 | 설명 |
|------|------|
| 목적 | 사용자별 위험 점수를 산출하여 내부 위협 조기 탐지 |
| 방식 | 규칙 기반 점수 + 통계적 이상치(Z-Score) 결합 |
| 출력 | 사용자별 위험 점수 (Risk Score) |
| 예시 | "평소 대비 파일 복사량 급증", "퇴사 예정자의 대량 다운로드" |

CEP가 "이벤트 패턴"을 탐지한다면, UEBA는 "사용자 행동의 이상 징후"를 탐지합니다.
개별 이벤트는 정상이어도 누적된 행동 패턴이 비정상일 수 있으며, UEBA는 이를 점수화하여 가시화합니다.

### 1.2 시스템 개요

SafePC SIEM의 UEBA 모듈은 사용자 행동 기반 이상 탐지 시스템입니다. 규칙 기반 점수와 통계적 이상치(Baseline 대비 Z-Score)를 결합하여 사용자별 위험 점수를 산출합니다.

### 1.3 독립 배포 아키텍처

본 시스템은 기존 운영 시스템(AWS 등)과 분리된 별도 서버(203.229.154.49)에 구축되어 있습니다.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         독립 배포 구조                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────────────┐         ┌─────────────────────────────────────┐   │
│  │   기존 운영 시스템    │         │        SIEM 전용 서버 (49번)        │   │
│  │      (AWS 등)        │         │                                     │   │
│  │                      │         │  ┌─────────┐  ┌─────────┐          │   │
│  │  ┌──────────────┐   │  Kafka  │  │   CEP   │  │  UEBA   │          │   │
│  │  │   SafePC     │───┼────────▶│  │ :48084  │  │ :48082  │          │   │
│  │  │   Agent      │   │         │  └─────────┘  └─────────┘          │   │
│  │  └──────────────┘   │         │       │            │               │   │
│  │                      │         │       ▼            ▼               │   │
│  │  ┌──────────────┐   │         │  ┌─────────────────────────────┐   │   │
│  │  │ Log Adapter  │───┼────────▶│  │       OpenSearch            │   │   │
│  │  └──────────────┘   │         │  │         :49200              │   │   │
│  │                      │         │  └─────────────────────────────┘   │   │
│  └─────────────────────┘         └─────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

이 구조의 장점:
- **기존 시스템 영향 없음**: SIEM 부하가 운영 서비스에 영향을 주지 않음
- **독립적 확장**: SIEM 리소스만 별도로 스케일 가능
- **유연한 적용**: 어떤 운영 환경에도 Kafka 연동만으로 SIEM 구축 가능
- **장애 격리**: SIEM 장애가 운영 시스템에 전파되지 않음

### 1.4 전체 아키텍처

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              데이터 흐름                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐                  │
│  │ 소스 시스템   │    │ Log Adapter  │    │    Kafka     │                  │
│  │ (SafePC 등)  │ → │ (CEF 파싱)   │ → │  (이벤트)    │                  │
│  │              │    │              │    │              │                  │
│  │ Syslog CEF   │    │ JSON 변환    │    │ MESSAGE_*    │                  │
│  └──────────────┘    └──────────────┘    └──────┬───────┘                  │
│                                                  │                          │
│                                                  ▼                          │
│                                         ┌──────────────┐                    │
│                                         │ UEBA Server  │                    │
│                                         │ (점수 계산)  │                    │
│                                         └──────┬───────┘                    │
│                                                │                            │
│                      ┌─────────────────────────┼─────────────────────┐      │
│                      │                         │                     │      │
│                      ▼                         ▼                     ▼      │
│              ┌──────────────┐         ┌──────────────┐      ┌──────────────┐│
│              │  OpenSearch  │         │  OpenSearch  │      │  Dashboard   ││
│              │  (scores)    │         │ (baselines)  │      │  (실시간)    ││
│              └──────────────┘         └──────────────┘      └──────────────┘│
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 2. 서버 구성

### 2.1 Docker 컨테이너 구성

| 컨테이너 | 포트 | 역할 |
|----------|------|------|
| siem-ueba | 48082 | UEBA 메인 서버 (API + Kafka Consumer + 점수 계산) |
| siem-opensearch | 49200 | 데이터 저장소 |
| siem-dashboard | 48501 | 웹 대시보드 |

### 2.2 UEBA 메인 서버 (siem-ueba) 역할

```
┌─────────────────────────────────────────────────────────┐
│                    siem-ueba 컨테이너                    │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  1. Echo API 서버 (:48082)                              │
│     - 규칙 CRUD API                                     │
│     - Field Meta API (스키마 관리, CEP와 공통)          │
│     - 사용자 점수 조회 API                              │
│     - 설정/상태 API                                     │
│                                                         │
│  2. Kafka Consumer                                      │
│     - Kafka 토픽 구독 (MESSAGE_*)                       │
│     - 이벤트 수신 → 규칙 매칭 → 점수 계산              │
│                                                         │
│  3. 점수 계산 엔진 (인메모리)                           │
│     - 규칙 점수: weight × log(count)                    │
│     - 이상치 점수: Z-Score 기반                         │
│     - Decay: 전일 점수 × λ^days                         │
│                                                         │
│  4. 자정 롤오버                                         │
│     - 날짜 변경 시 점수 OpenSearch 저장                 │
│     - Baseline 갱신 (mean, stddev 재계산)               │
│     - 전일 점수 → PrevScore로 이월                      │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

### 2.3 OpenSearch 인덱스

| 인덱스 패턴 | 용도 |
|-------------|------|
| {prefix}-ueba-scores-YYYY.MM.DD | 사용자별 일일 점수 |
| {prefix}-ueba-baselines | 사용자별 이벤트 통계 (mean, stddev) |
| {prefix}-ueba-rules | UEBA 규칙 저장 |
| {prefix}-settings | UEBA 설정 (anomaly, decay, tiers) |
| {prefix}-field-meta | 필드 메타데이터 (CEP와 공유) |

---

## 3. 점수 계산 알고리즘

### 3.1 전체 점수 공식

```
RiskScore = DecayedPrev + RuleScore + AnomalyScore

DecayedPrev = PrevScore × λ^days    (전일 점수 감쇠)
RuleScore   = Σ(weight × f(count))  (규칙 기반 점수)
AnomalyScore = Σ(β × (z - threshold)) (이상치 점수, z > threshold인 경우만)
```

### 3.2 규칙 점수 (RuleScore)

```
RuleScore = Σ (rule.weight × frequencyFunc(matchCount))

frequencyFunc 옵션:
- log (기본): ln(1 + count)
- log2: log₂(1 + count)
- log10: log₁₀(1 + count)
- linear: count
```

### 3.3 이상치 점수 (AnomalyScore)

```
Z-Score = (오늘 발생 횟수 - 평균) / 표준편차

if Z > z_threshold:
    AnomalyScore += β × (Z - z_threshold)
```

### 3.4 감쇠 (Decay)

```
DecayedPrev = PrevScore × λ^days

- λ: 감쇠율 (기본 0.9)
- days: 마지막 활동 이후 경과일
- weekend_mode=skip: 주말 제외 계산
```

### 3.5 Cold Start

Baseline 데이터가 부족한 신규 사용자는 점수 0으로 처리:
- cold_start_min_days (기본 7일) 미만 시 점수 계산 안함

---

## 4. 마이그레이션 (스키마 정규화)

CEP와 동일한 Field Meta API를 사용합니다.

### 4.1 마이그레이션 프로세스

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         마이그레이션 → 규칙 생성 흐름                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Step 1: 로그 분석 (서버)                                                   │
│  POST /api/field-meta/analyze {"days": 7}                                   │
│  → OpenSearch에서 7일치 로그 조회                                           │
│  → msgId별 cefExtensions 필드 목록 자동 추출                                │
│                                                                             │
│  Step 2: 필드 설정 (관리자 UI)                                              │
│  - 필드별 inputType 지정 (input, select, checkbox, number)                  │
│  - select/checkbox는 값 목록 수집 또는 수동 입력                            │
│  - 한글 라벨 지정                                                           │
│                                                                             │
│  Step 3: 메타데이터 저장                                                    │
│  PUT /api/field-meta → OpenSearch에 저장                                    │
│                                                                             │
│  Step 4: 규칙 생성                                                          │
│  POST /api/rules (UEBA 규칙 JSON)                                           │
│  → 규칙 저장 → 캐시 갱신 → 실시간 점수 계산 시작                            │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 4.2 Field Meta API (CEP와 공통)

| Method | Path | 설명 |
|--------|------|------|
| POST | /api/field-meta/analyze | N일치 로그에서 이벤트/필드 추출 |
| POST | /api/field-meta/analyze-field | 특정 필드의 값 목록 수집 |
| GET | /api/field-meta | 저장된 메타데이터 조회 |
| PUT | /api/field-meta | 메타데이터 저장 |

---

## 5. UEBA 규칙 언어

### 5.1 지원 연산자

| 연산자 | 설명 | 예시 |
|--------|------|------|
| eq | 같음 | {"field": "outcome", "op": "eq", "value": "blocked"} |
| neq | 다름 | {"field": "act", "op": "neq", "value": "read"} |
| gt | 초과 | {"field": "fsize", "op": "gt", "value": 10485760} |
| gte | 이상 | {"field": "fsize", "op": "gte", "value": 1048576} |
| lt | 미만 | {"field": "fsize", "op": "lt", "value": 1024} |
| lte | 이하 | {"field": "fsize", "op": "lte", "value": 1024} |
| contains | 포함 | {"field": "fname", "op": "contains", "value": ".exe"} |
| in | 목록 포함 | {"field": "outcome", "op": "in", "value": ["blocked", "denied"]} |
| time_range | 시간대 | {"field": "hour", "op": "time_range", "start": 22, "end": 6} |

### 5.2 집계 타입 (aggregate)

UEBA 점수 계산에 사용되는 집계 방식:

```json
// 횟수 기반 (기본)
{"function": "count"}

// 필드값 합산
{"function": "sum", "field": "fsize"}

// 고유값 수
{"function": "cardinality", "field": "dhost"}
```

| 필드 | 설명 | 필수 |
|------|------|------|
| function | 집계 함수 (count/sum/cardinality) | 예 |
| field | 집계 대상 필드 | sum, cardinality 시 |

점수 계산: `weight × ln(1 + 집계값)`

| function | 집계값 | 예시 |
|----------|--------|------|
| count | 매칭 횟수 | USB 차단 3회 → ln(4) = 1.39 |
| sum | 필드 합계 | 파일 100MB → ln(104857601) = 18.47 |
| cardinality | 고유값 수 | 호스트 10개 → ln(11) = 2.40 |

---

## 6. UEBA 규칙 API

### 6.1 규칙 생성

POST /api/rules

### 6.2 기본 규칙 (count)

이벤트 발생 횟수 기반 점수:

요청:
```json
{
  "name": "USB 차단",
  "category": "매체제어",
  "weight": 10,
  "enabled": true,
  "match": {
    "msgId": "MESSAGE_DEVICE_USAGE",
    "logic": "and",
    "conditions": [
      {"field": "outcome", "op": "eq", "value": "blocked"}
    ]
  },
  "aggregate": {
    "type": "count"
  },
  "ueba": {
    "enabled": true
  }
}
```

점수 계산:
```
USB 차단 3회 발생 시:
RuleScore = 10 × ln(1 + 3) = 10 × 1.386 = 13.86
```

### 6.3 합산 규칙 (sum)

필드값 합산 기반 점수:

요청:
```json
{
  "name": "대용량 파일 반출",
  "category": "정보유출",
  "weight": 5,
  "enabled": true,
  "match": {
    "msgId": "MESSAGE_FILE_EXPORT",
    "conditions": [
      {"field": "outcome", "op": "eq", "value": "allowed"}
    ]
  },
  "aggregate": {
    "type": "sum",
    "field": "fsize"
  },
  "ueba": {
    "enabled": true
  }
}
```

점수 계산:
```
100MB 파일 반출 시:
RuleScore = 5 × ln(1 + 104857600) = 5 × 18.47 = 92.35
```

### 6.4 고유값 규칙 (cardinality)

고유값 수 기반 점수:

요청:
```json
{
  "name": "다수 호스트 접근",
  "category": "내부이동",
  "weight": 15,
  "enabled": true,
  "match": {
    "msgId": "MESSAGE_NETWORK_ACCESS",
    "conditions": []
  },
  "aggregate": {
    "type": "cardinality",
    "field": "dhost"
  },
  "ueba": {
    "enabled": true
  }
}
```

점수 계산:
```
10개 호스트 접근 시:
RuleScore = 15 × ln(1 + 10) = 15 × 2.398 = 35.97
```

### 6.5 시간대 조건

업무 외 시간 가중:

요청:
```json
{
  "name": "야간 파일 접근",
  "category": "이상행위",
  "weight": 20,
  "enabled": true,
  "match": {
    "msgId": "MESSAGE_FILE_ACCESS",
    "logic": "and",
    "conditions": [
      {"field": "hour", "op": "time_range", "start": 22, "end": 6}
    ]
  },
  "ueba": {
    "enabled": true
  }
}
```

---

## 7. 설정 (Config)

### 7.1 설정 구조

```json
{
  "anomaly": {
    "z_threshold": 2.0,
    "beta": 10,
    "sigma_floor": 0.5,
    "cold_start_min_days": 7,
    "baseline_window_days": 7,
    "frequency_function": "log"
  },
  "decay": {
    "lambda": 0.9,
    "weekend_mode": "skip"
  },
  "tiers": {
    "green_max": 40,
    "yellow_max": 99
  }
}
```

### 7.2 설정 항목 설명

| 항목 | 기본값 | 설명 |
|------|--------|------|
| z_threshold | 2.0 | 이상치 판정 Z-Score 임계값 |
| beta | 10 | 이상치 점수 가중치 |
| sigma_floor | 0.5 | 최소 표준편차 (0 방지) |
| cold_start_min_days | 7 | Cold Start 판정 최소 일수 |
| baseline_window_days | 7 | Baseline 계산 윈도우 |
| frequency_function | log | 빈도 함수 (log/log2/log10/linear) |
| lambda | 0.9 | 일일 감쇠율 |
| weekend_mode | skip | 주말 처리 (skip/count) |
| green_max | 40 | LOW 위험 최대 점수 |
| yellow_max | 99 | MEDIUM 위험 최대 점수 |

---

## 8. API 명세

### 8.1 규칙 API

| Method | Path | 설명 |
|--------|------|------|
| GET | /api/rules | 규칙 목록 |
| POST | /api/rules | 규칙 생성 |
| PUT | /api/rules/:id | 규칙 수정 |
| DELETE | /api/rules/:id | 규칙 삭제 |
| POST | /api/rules/validate | 규칙 검증 |

### 8.2 사용자 API

| Method | Path | 설명 |
|--------|------|------|
| GET | /api/users | 사용자 목록 (DataTables 형식) |
| GET | /api/users/:id | 사용자 상세 |
| GET | /api/users/:id/hourly | 시간별 점수 |
| GET | /api/users/:id/history | 일별 점수 이력 |

### 8.3 설정/상태 API

| Method | Path | 설명 |
|--------|------|------|
| GET | /api/config | UEBA 설정 조회 |
| GET | /api/settings | 전체 설정 조회 |
| PUT | /api/settings | 설정 저장 |
| GET | /api/status | 서비스 상태 |
| GET | /api/health | 헬스체크 |
| POST | /api/save | 점수 즉시 저장 |
| POST | /api/baseline | Baseline 즉시 갱신 |
| POST | /api/reload | 캐시 재로드 |

---

## 9. 신규 사이트 구축 시나리오

### 9.1 전체 흐름

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                       신규 사이트 UEBA 구축 시나리오                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Phase 1: 인프라 구성                                                       │
│  ─────────────────────                                                      │
│  - Kafka, OpenSearch, UEBA 컨테이너 배포                                    │
│  - .env 설정 (Kafka 토픽, 인덱스 접두사 등)                                 │
│  - UEBA 서비스 시작 시 연결 상태 로그 출력                                  │
│                                                                             │
│  Phase 2: 로그 수집 + Baseline 축적 (최소 7일)                              │
│  ─────────────────────────────────────────────                              │
│  - Log Adapter가 Syslog CEF → Kafka 전송                                    │
│  - UEBA가 이벤트 수신 → 사용자별 카운트 집계                                │
│  - 자정마다 Baseline 자동 갱신 (mean, stddev)                               │
│  - Cold Start 기간: 점수 0으로 처리                                         │
│                                                                             │
│  Phase 3: 마이그레이션 (스키마 정규화)                                      │
│  ─────────────────────────────────────                                      │
│  - POST /api/field-meta/analyze {"days": 7}                                 │
│  - 쌓인 로그에서 이벤트 타입/필드 자동 발견                                 │
│  - 관리자가 UI에서 필드 설정                                                │
│  - PUT /api/field-meta 저장                                                 │
│                                                                             │
│  Phase 4: 규칙 생성                                                         │
│  ─────────────────────                                                      │
│  - 프론트엔드 규칙 빌더 UI에서 조건/가중치 설정                             │
│  - POST /api/rules → 규칙 저장 → 캐시 갱신                                  │
│  - 실시간 점수 계산 시작                                                    │
│                                                                             │
│  Phase 5: 운영                                                              │
│  ─────────────                                                              │
│  - 대시보드에서 위험 사용자 모니터링                                        │
│  - 규칙 가중치 튜닝 (PUT /api/rules/:id)                                    │
│  - 설정 조정 (z_threshold, lambda 등)                                       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 9.2 UEBA 서비스 시작 로그 예시

```
2026/02/25 10:52:39 UEBA 서비스 시작...
2026/02/25 10:52:39   Port: :48082
2026/02/25 10:52:39   OpenSearch: http://203.229.154.49:49200
2026/02/25 10:52:39   Kafka: 43.202.241.121:44105
2026/02/25 10:52:39 [INIT] UEBA 시스템 초기화 시작
2026/02/25 10:52:39 [CONFIG] 설정 로드 완료
2026/02/25 10:52:39 [CONFIG] 5개 UEBA 규칙 로드
2026/02/25 10:52:39 [BASELINE] 1,234개 baseline 로드
2026/02/25 10:52:39 [RECOVER] 오늘 점수 복구 중...
2026/02/25 10:52:40 [RECOVER] 156명 사용자 상태 복구 완료
⇨ http server started on [::]:48082
```

### 9.3 메모리 상태 관리

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           UEBA 메모리 상태 관리                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  시작 시 로드:                                                              │
│  ─────────────                                                              │
│  - OpenSearch에서 모든 Baseline 로드 → baselines map                        │
│  - 오늘 날짜 점수 인덱스에서 사용자 상태 복구 → userStates map              │
│                                                                             │
│  실시간 처리:                                                               │
│  ─────────────                                                              │
│  - Kafka 이벤트 수신 → 규칙 매칭 → userStates 업데이트 (메모리)             │
│  - 점수 계산 완료 → Dashboard로 실시간 push                                 │
│                                                                             │
│  자정 롤오버:                                                               │
│  ─────────────                                                              │
│  - userStates → OpenSearch 저장 (ueba-scores-YYYY.MM.DD)                    │
│  - Baseline 재계산 (최근 N일 로그 집계)                                     │
│  - PrevScore = 오늘 RiskScore                                               │
│  - EventCounts, EventValues 초기화                                          │
│                                                                             │
│  수동 저장:                                                                 │
│  ─────────────                                                              │
│  - POST /api/save → 현재 메모리 상태 즉시 저장                              │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 10. CEP와의 차이점

| 항목 | CEP | UEBA |
|------|-----|------|
| 목적 | 실시간 패턴 탐지 | 사용자 위험 점수 |
| 엔진 | Flink SQL | 인메모리 계산 |
| 출력 | 알림 (Alert) | 점수 (Score) |
| 규칙 | 패턴 매칭 | 가중치 기반 |
| 추가 필드 | - | weight, aggregate.type, ueba.enabled |
| Baseline | 없음 | 사용자별 통계 |
| Cold Start | 없음 | 7일 미만 시 점수 0 |

---

## 11. 환경 설정

### 11.1 .env 파일

```
OPENSEARCH_URL=http://203.229.154.49:49200
KAFKA_BOOTSTRAP_SERVERS=43.202.241.121:44105
KAFKA_EVENT_TOPICS=MESSAGE_AGENT,MESSAGE_DEVICE,MESSAGE_NETWORK,...
DASHBOARD_URL=http://siem-dashboard:8501
INDEX_PREFIX=safepc-siem
TIMEZONE=Asia/Seoul
```

---

## 13. 미구현 기능 (Reference 대비)

현재 UEBA 시스템은 reference(EventScore)와 비교하여 다음 기능이 미구현 상태입니다.
이 기능들은 **사용자 목록 및 상태 정보를 외부에서 받아올 수 있는 구조가 갖춰지면** 구현 가능합니다.

### 13.1 상황 가중치 (Context Multipliers)

Reference 구현:
```go
// daily.go
contextMult := cfg.GetContextMultiplier(user.Status)
subtotal := weight * freq * contextMult
```

| 사용자 상태 | 가중치 | 설명 |
|------------|--------|------|
| active | 1.0 | 정상 근무 |
| normal | 1.0 | 일반 |
| departing | 2.0 | 퇴사 예정자 |
| vacation | 1.5 | 휴가 중 접속 |
| blacklist | 2.0 | 블랙리스트 |

**현재 상태**: 미구현 (사용자 상태 정보 없음)
**필요 조건**: 사용자 목록 API에서 status 필드 제공

### 13.2 직무별 화이트리스트 (Whitelist Rules)

Reference 구현:
```go
// daily.go
if cfg.IsWhitelisted(user.Role, et) {
    continue  // 점수 계산에서 제외
}
```

| 직무 | 예외 이벤트 |
|------|------------|
| developer | large_file_copy |
| sysadmin | off_hours_access, hacking_tool |

**현재 상태**: 미구현 (사용자 직무 정보 없음)
**필요 조건**: 사용자 목록 API에서 role 필드 제공

### 13.3 사용자별 최저 점수 (Floor Score)

Reference 구현:
```go
// decay.go
decayed := max(user.FloorScore, previous * λ^days)
```

특정 사용자(블랙리스트 등)는 점수가 일정 수준 이하로 내려가지 않도록 설정.

**현재 상태**: 미구현
**필요 조건**: 사용자별 floor_score 설정 기능

### 13.4 Peer Baseline (동료 그룹 기준선)

Reference 구현:
```sql
CREATE TABLE peer_baselines (
    department TEXT NOT NULL,
    role TEXT NOT NULL,
    event_type TEXT NOT NULL,
    mean DOUBLE PRECISION,
    std_dev DOUBLE PRECISION,
    PRIMARY KEY (department, role, event_type)
);
```

Cold Start 기간에 개인 baseline 대신 동일 부서/직무 그룹의 baseline 참조.

**현재 상태**: 미구현 (부서/직무 정보 없음)
**필요 조건**: 사용자 목록 API에서 department, role 필드 제공

### 13.5 구현 우선순위

| 우선순위 | 기능 | 필요 데이터 |
|---------|------|------------|
| 1 | 상황 가중치 | user.status |
| 1 | 화이트리스트 | user.role |
| 2 | Floor Score | user.floor_score |
| 3 | Peer Baseline | user.department, user.role |

### 13.6 사용자 동기화 API 예시 (향후 구현 시)

```json
POST /api/sync/users
{
  "users": [
    {
      "employee_id": "kim",
      "name": "김철수",
      "department": "개발팀",
      "role": "developer",
      "status": "active",
      "floor_score": 0
    }
  ]
}
```

이 API가 구현되면 위 기능들을 순차적으로 추가할 수 있습니다.

---

## 14. 운영 명령어

```bash
# UEBA 서비스 재시작
docker-compose restart ueba

# 로그 확인
docker logs -f siem-ueba

# 캐시 재로드
curl -X POST http://localhost:48082/api/reload

# 점수 즉시 저장
curl -X POST http://localhost:48082/api/save

# Baseline 즉시 갱신
curl -X POST http://localhost:48082/api/baseline

# 서비스 상태
curl http://localhost:48082/api/status

# 헬스체크
curl http://localhost:48082/api/health
```
