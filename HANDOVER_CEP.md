# CEP (Complex Event Processing) 인수인계서

> 작성일: 2026-02-26 | 최종 갱신: 2026-02-26 11:15

## 개요

Flink SQL 기반 실시간 이벤트 탐지 시스템. Kafka에서 변환된 이벤트를 구독하여 규칙 기반 Alert를 생성한다.

## 아키텍처

```
Kafka (safepc-siem-events)  ← LogSink가 변환한 단일 토픽
  └→ Flink SQL (TUMBLE 윈도우, MATCH_RECOGNIZE 등)
       └→ Kafka (cep-alerts)
            └→ CEP AlertConsumer → OpenSearch (cep-alerts-YYYY.MM.DD)
                                 → Dashboard Push (WebSocket)
```

## 서비스 정보

| 항목 | 값 |
|------|-----|
| 컨테이너 | siem-cep |
| 포트 | 48084 |
| 엔트리포인트 | `./siem cep` |
| Kafka GroupID | siem-cep-sql |
| 구독 토픽 | safepc-siem-events (변환 토픽 1개) |

## 코드 구조

```
cmd/cep.go                          # Echo 서버 + 라우팅
internal/cep/
  controllers/
    rule.go                         # 규칙 CRUD API
    job.go                          # Flink Job 관리 API
    alert.go                        # Alert 조회 (DataTables)
  services/
    flink.go                        # Flink SQL Gateway 통신, Job 제출/취소
    sql_builder.go                  # Rule JSON → Flink SQL 변환
    consumer.go                     # Kafka AlertConsumer (cep-alerts → OpenSearch)
    util.go                         # cefField() 헬퍼
```

## 핵심 로직

### 1. SQL Builder (`sql_builder.go`)

Rule JSON을 Flink SQL로 변환한다.

- 이벤트 테이블: `safepc-siem-events` 토픽을 Flink Kafka connector로 구독
- CEF 필드 접근: `cefExtensions['필드명']` (LogSink가 Label→이름 변환 완료)
- 지원 패턴: 단일 WHERE, TUMBLE 집계+HAVING, MATCH_RECOGNIZE 순차, 윈도우 AND/OR

### 2. Flink 연동 (`flink.go`)

- SQL Gateway (`48083`): SQL 실행 (CREATE TABLE, INSERT INTO)
- REST API (`48081`): Job 상태 조회, 취소
- 규칙 1개 = Flink Job 1개 (현재 18개 실행 중)

### 3. Alert Consumer (`consumer.go`)

- Kafka `cep-alerts` 토픽 구독
- 필드명 정규화 후 OpenSearch `safepc-siem-cep-alerts-YYYY.MM.DD` 저장
- Dashboard에 WebSocket Push

## API

| Method | Path | 설명 |
|--------|------|------|
| GET | /api/rules | 규칙 목록 |
| POST | /api/rules | 규칙 생성 + Flink Job 제출 |
| PUT | /api/rules/:id | 규칙 수정 |
| POST | /api/submit | 수동 Job 제출 |
| POST | /api/reload | 전체 규칙 재로드 (순차 제출) |
| GET | /api/status | 실행 중인 Job 상태 |
| GET | /api/alerts | Alert 목록 (DataTables) |
| GET/PUT | /api/field-meta | 필드 메타데이터 조회/저장 |
| POST | /api/field-meta/analyze | 이벤트별 필드 동적 추출 |

## OpenSearch 인덱스

| 인덱스 | 용도 |
|--------|------|
| `safepc-siem-common-rules` | 규칙 저장 (CEP/UEBA 공유, 29개) |
| `safepc-siem-cep-alerts-YYYY.MM.DD` | Alert 저장 |

## 배포

```bash
# 소스 업로드 + 빌드
sshpass -p 'M@rkAny' scp -o StrictHostKeyChecking=no \
  internal/cep/services/*.go root@203.229.154.49:/Safepc/SafePC/internal/cep/services/
cd /Safepc/siem && docker-compose up -d --build cep
```

## 주의사항

- Flink TaskManager JVM 타임존은 KST (Asia/Seoul)
- `HOUR(PROCTIME())`은 KST 시간 반환
- 규칙 reload 시 기존 Job 취소 → 새 Job 순차 제출 (동시 제출 시 Flink 부하)
- `cefField("필드명")`은 `cefExtensions['필드명']`으로 변환 (Label 변환은 LogSink가 처리)

---

## CEP/UEBA 룰 스키마 통일 (2026-02-26)

CEP와 UEBA가 동일한 룰 JSON 구조를 사용:

```json
{
  "name": "비업무시간 접속",
  "enabled": true,
  "match": {
    "msgId": "MESSAGE_AGENT_AUTHENTICATION",
    "logic": "and",
    "conditions": [
      {"field": "outcome", "op": "eq", "value": "success"},
      {"field": "hour", "op": "time_range", "start": 22, "end": 6}
    ]
  },
  "cep": {"enabled": true, "severity": "MEDIUM"},
  "ueba": {"enabled": true, "weight": 5}
}
```

- CEP: `sql_builder.go` → Flink SQL 변환
- UEBA: `buildRuleESQuery()` → OpenSearch Query DSL 변환

### 지원 연산자

| op | CEP (Flink SQL) | UEBA (OpenSearch) |
|----|-----------------|-------------------|
| eq | `= 'value'` | `{"term": {...}}` |
| neq | `!= 'value'` | `{"bool": {"must_not": [...]}}` |
| gt/gte/lt/lte | `CAST(...) > N` | `{"range": {...}}` |
| in | `IN ('a','b')` | `{"terms": [...]}` |
| like | `LIKE '%...'` | - |
| regex | `REGEXP(...)` | - |
| contains | - | `{"wildcard": {...}}` |
| time_range | `HOUR(proctime) >= X OR < Y` | script query (KST) |

### hour 가상 필드

- CEP: `HOUR(proctime)` 변환
- UEBA: `@timestamp`에서 KST hour 추출
