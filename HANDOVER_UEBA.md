# UEBA (User Entity Behavior Analytics) 인수인계서

> 작성일: 2026-02-26 | 최종 갱신: 2026-02-26

## 개요

사용자 행동 기반 위험도 점수 계산 시스템. Kafka에서 변환된 이벤트를 실시간 구독하여 룰 매칭 + 이상치 탐지를 수행하고, 누적 위험 점수를 관리한다.

## 아키텍처

```
Kafka (safepc-siem-events)  ← LogSink가 변환한 단일 토픽
  └→ UEBA Kafka Consumer
       ├→ 룰 매칭 (msgId + conditions)
       ├→ 집계 (count/sum/cardinality)
       ├→ 점수 계산 (ruleScore + anomalyScore + decayedPrev)
       ├→ OpenSearch 저장 (scores-YYYY.MM.DD, 10분 배치)
       └→ Dashboard Push (WebSocket)
```

## 서비스 정보

| 항목 | 값 |
|------|-----|
| 컨테이너 | siem-ueba |
| 포트 | 48082 |
| 엔트리포인트 | `./siem ueba` |
| Kafka GroupID | siem-ueba |
| 구독 토픽 | safepc-siem-events (변환 토픽 1개) |

## 코드 구조

```
cmd/ueba.go                         # Echo 서버 + 라우팅
internal/ueba/
  controllers/
    rule.go                         # 규칙 CRUD API
    status.go                       # 상태/헬스체크
    user.go                         # 사용자 위험 점수 API
  services/
    processor.go                    # 전체 로직 (1파일, ~1780줄)
```

## 점수 계산 공식

```
RiskScore = DecayedPrev + RuleScore + AnomalyScore

DecayedPrev = PrevScore × λ^days
  - λ = 0.9 (decay lambda)
  - days = 경과일수 (weekend_mode=skip 시 주말 제외)

RuleScore = Σ( Weight_i × f(Value_i) )
  - f = frequencyFunction (기본 log = ln(1+x))
  - Value = 룰별 집계값 (count/sum/cardinality)

AnomalyScore = Σ( β × max(0, Z - Z_threshold) )
  - Z = (count - mean) / max(stddev, sigma_floor)
  - β = 10, Z_threshold = 2.0, sigma_floor = 0.5
  - baseline sampleDays < cold_start_min_days면 스킵
  - 룰에 등록된 msgId만 anomaly 점수 반영
```

## 유저 상태 (status)

| status | 조건 | riskScore |
|--------|------|-----------|
| `active` | baseline 있음, cold start 아님 | 정상 계산 |
| `cold_start` | prevScore=0 + baseline sampleDays < 3일 | **0 (강제)** |
| `no_baseline` | baseline 자체가 없음 (신규 유저) | cold start면 0, prevScore 있으면 decay만 |

대시보드에서 등급 옆에 `Cold` / `신규` 배지로 표시된다.

## 초기화 흐름 (`initialize`)

```
1. loadConfig()          — OpenSearch settings에서 anomaly/decay/tier 설정
2. loadRules()           — ueba.enabled=true 규칙만 로드
3. loadAllBaselines()    — 899개 baseline (유저별×msgId별)
4. ensureBaselinesFresh() — baseline_meta.updated_at이 오늘이 아니면 재계산
5. recoverTodayState()
   5-1. initUsersFromPrevScores() — 어제까지 scores 있는 유저를 decay 적용하여 로드
   5-2. 오늘 event-logs에서 이벤트 재처리 (룰 매칭 + 집계)
   5-3. 전체 유저 점수 계산
   5-4. saveScoresBatch() — 즉시 scores 저장 (대시보드 표시용)
```

## Baseline

### 생성 시점
- **자정 롤오버** 시 `updateBaselines()` 실행
- 시작 시 `ensureBaselinesFresh()`로 오늘 자정 갱신 여부 확인

### 계산 방식
- event-logs에서 `baseline_window_days`(기본 7일) 범위 조회
- userId × msgId × 일별 doc_count 집계
- mean/stddev/sampleDays 계산 → baselines 인덱스에 저장
- key: `userId_msgId`

### Cold Start
- `cold_start_min_days` = **3** (현재 설정)
- prevScore > 0이면 cold start 아님 (이전 점수가 있으면 우회)
- baseline sampleDays < 3이면 anomaly 계산 스킵
- 신규 유저: 최소 3일 이벤트 축적 후 점수 시작

### Peer Group Baseline (미구현)
- 레퍼런스에는 같은 부서+역할의 peer baseline fallback 있음
- 현재 SafePC CEF 이벤트에 부서 정보 없어서 미구현
- 향후 Agent에서 부서 필드 추가 시 구현 가능

## 자정 롤오버 (`checkDateRollover`)

```
1. 현재 점수를 어제 날짜 인덱스에 저장 (saveScoresBatchForDate)
2. 모든 유저: prevScore = 현재 riskScore, 카운터 리셋
3. baseline 갱신 (비동기)
4. baseline_meta.updated_at 갱신
```

## 설정 (OpenSearch `safepc-siem-common-settings/_doc/settings`)

```json
{
  "anomaly": {
    "z_threshold": 2.0,
    "beta": 10,
    "sigma_floor": 0.5,
    "cold_start_min_days": 3,
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

| 설정 | 설명 | 기본값 |
|------|------|--------|
| z_threshold | anomaly 감지 임계값 | 2.0 |
| beta | anomaly 점수 배율 | 10 |
| sigma_floor | 표준편차 최소값 (0 방지) | 0.5 |
| cold_start_min_days | cold start 판정 기준일 | 3 |
| baseline_window_days | baseline 계산 윈도우 | 7 |
| frequency_function | 빈도 스케일링 (log/log2/log10/linear) | log |
| lambda | decay 계수 (0~1, 작을수록 빠른 감쇠) | 0.9 |
| weekend_mode | 주말 처리 (skip=주말 제외, count=포함) | skip |
| green_max | LOW 최대 점수 | 40 |
| yellow_max | MEDIUM 최대 점수 (초과 시 HIGH) | 99 |

## 룰 매칭

### 구조
```json
{
  "name": "USB 대용량 파일 복사",
  "weight": 15,
  "enabled": true,
  "match": {
    "msgId": "MESSAGE_DEVICE_USAGE",
    "logic": "and",
    "conditions": [
      {"field": "action", "op": "eq", "value": "write"},
      {"field": "fsize", "op": "gt", "value": 10485760}
    ]
  },
  "aggregate": {"type": "sum", "field": "fsize"},
  "ueba": {"enabled": true}
}
```

### 조건 연산자
eq, neq, gt, gte, lt, lte, contains, in, time_range

### 집계 타입
- `count` (기본): 매칭 횟수
- `sum`: 필드값 합산 (예: fsize)
- `cardinality`: 고유값 수 (예: 접속 호스트 수)

### 필드 접근 순서
1. `_attrs` (CEF Label 기반 파싱 결과)
2. 이벤트 최상위 필드
3. `cefExtensions` 내부

## API

| Method | Path | 설명 |
|--------|------|------|
| GET | /api/rules | 규칙 목록 |
| POST | /api/rules | 규칙 생성 |
| PUT | /api/rules/:id | 규칙 수정 |
| DELETE | /api/rules/:id | 규칙 삭제 |
| GET | /api/users | 전체 유저 목록 (status 포함) |
| GET | /api/users/:id | 유저 상세 |
| GET | /api/users/:id/history | 유저 일별/시간별 점수 이력 |
| GET | /api/health | 헬스체크 |
| GET | /api/status | 서비스 상태 |
| GET | /api/config | UEBA 설정 조회 |
| GET | /api/settings | 설정 + 룰 가중치 조회 |
| PUT | /api/settings | 설정 저장 |
| POST | /api/reload | 캐시 리로드 |
| POST | /api/baseline | baseline 수동 갱신 |
| GET | /save | 수동 scores 저장 |

## OpenSearch 인덱스

| 인덱스 | 용도 |
|--------|------|
| `safepc-siem-common-rules` | 규칙 저장 (CEP/UEBA 공유) |
| `safepc-siem-common-settings` | UEBA 설정 + baseline_meta |
| `safepc-siem-ueba-scores-YYYY.MM.DD` | 시간별 점수 스냅샷 (_id: userId_HH) |
| `safepc-siem-ueba-baselines` | baseline (mean/stddev/sampleDays) |
| `safepc-siem-event-logs-YYYY.MM.DD` | 변환된 이벤트 (LogSink가 저장) |

## 배포

```bash
sshpass -p 'M@rkAny' scp -o StrictHostKeyChecking=no \
  internal/ueba/services/processor.go \
  root@203.229.154.49:/Safepc/SafePC/internal/ueba/services/processor.go
cd /Safepc/siem && docker-compose up -d --build ueba
```

## 미구현 기능 (레퍼런스 대비)

| 기능 | 설명 | 우선순위 |
|------|------|----------|
| Peer Group Baseline | 부서/역할별 baseline fallback | 중 (부서 정보 필요) |
| Context Multiplier | 퇴직예정/징계 등 상황별 가중치 | 중 (HR 연동 필요) |
| Whitelist | 역할별 이벤트 제외 | 낮음 |
| FloorScore | 유저별 최소 점수 | 낮음 |
| DailyScore 별도 저장 | cold start 기간에도 daily score 기록 | 낮음 |

## 주의사항

- `processor.go` 1파일에 전체 로직 (~1780줄) — 향후 분리 고려
- scores 저장은 10분 배치 + 시작 시 즉시 저장
- `_id = userId_HH` → 같은 시간대에 여러 번 저장하면 덮어씀 (의도된 동작)
- 자정 롤오버 시 `saveScoresBatchForDate(어제날짜)` → 어제 인덱스에 저장
- `loadConfig()`/`loadRules()`는 캐시 사용 — 변경 시 `/api/reload` 호출 필요
