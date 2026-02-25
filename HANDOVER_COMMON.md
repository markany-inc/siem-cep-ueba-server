# SafePC SIEM 공통 인수인계서

> 작성일: 2026-02-26 | 최종 갱신: 2026-02-26

## 시스템 구성

```
Kafka (11개 원본 토픽: MESSAGE_AGENT, ..., MESSAGE_ASSETS)
  └→ siem-logsink
       ├→ CEF Label 변환 (ExpandCEFLabels)
       ├→ Kafka (safepc-siem-events) 발행
       └→ OpenSearch (event-logs-YYYY.MM.DD) 저장
            ├→ siem-cep (Flink SQL 구독 → Alert 생성)
            └→ siem-ueba (Kafka Consumer → 점수 계산)

siem-dashboard (FastAPI + WebSocket)
  ├→ OpenSearch 직접 조회 (scores, alerts, event-logs)
  ├→ UEBA API 호출 (/api/config, /api/users)
  └→ WebSocket Push 수신 (실시간 Alert/Score 갱신)
```

## 서버 정보

```
서버: 203.229.154.49
SSH: sshpass -p 'M@rkAny' ssh root@203.229.154.49
소스: /Safepc/SafePC/
배포: /Safepc/siem/
대시보드: /Safepc/siem/dashboard/
Git: https://github.com/markany-inc/siem-cep-ueba-server
```

## 서비스 포트

| 서비스 | 컨테이너 | 포트 |
|--------|----------|------|
| CEP | siem-cep | 48084 |
| UEBA | siem-ueba | 48082 |
| LogSink | siem-logsink | - (내부) |
| Dashboard | siem-dashboard | 48501 |
| Flink JobManager | siem-flink-jobmanager | 48081 |
| Flink SQL Gateway | - | 48083 |
| OpenSearch | siem-opensearch | 49200 |
| OS Dashboards | siem-opensearch-dashboards | 45601 |
| Kafka | 43.202.241.121 | 44105 |

## 프로젝트 구조

```
SafePC/
├── main.go                          # 엔트리포인트
├── cmd/
│   ├── root.go                      # Cobra CLI 루트 (cep, ueba, logsink 등록)
│   ├── cep.go                       # CEP Echo 서버
│   ├── ueba.go                      # UEBA Echo 서버
│   └── logsink.go                   # LogSink 진입점
├── config/config.go                 # Viper 설정 (CEP/UEBA/LogSink 분기)
├── internal/
│   ├── common/
│   │   ├── cef.go                   # ExpandCEFLabels() — *Label 동적 스캔
│   │   ├── indices.go               # 인덱스명 헬퍼
│   │   ├── opensearch.go            # OSClient
│   │   └── fieldmeta.go            # 필드 메타데이터 API
│   ├── logsink/sink.go             # LogSink 서비스
│   ├── cep/                         # → HANDOVER_CEP.md 참조
│   └── ueba/                        # → HANDOVER_UEBA.md 참조
├── siem/
│   ├── .env                         # 환경변수
│   ├── docker-compose.yml           # CEP, UEBA, LogSink, Flink, OpenSearch
│   ├── cep/Dockerfile
│   ├── ueba/Dockerfile
│   ├── logsink/Dockerfile
│   └── dashboard/
│       ├── docker-compose.yml       # 대시보드 (볼륨 마운트: templates, app.py)
│       ├── app.py
│       └── templates/
├── reference/EventScore/            # 레퍼런스 프로젝트 (점수 공식 원본)
└── credentials/confluence_token.txt # Confluence API 토큰
```

## LogSink (독립 서비스)

CEF Label 변환 + 이벤트 저장을 담당하는 독립 컨테이너.

```
원본 Kafka 11개 토픽 구독
  → ExpandCEFLabels(): *Label 접미사 동적 스캔, label→value 매핑
  → Kafka safepc-siem-events 발행 (CEP/UEBA가 구독)
  → OpenSearch event-logs-YYYY.MM.DD 저장
```

- `internal/common/cef.go`: `ExpandCEFLabels(ext map[string]interface{})` — 공개 함수
- `internal/logsink/sink.go`: Kafka consumer → 변환 → producer + OpenSearch
- 하드코딩된 Label 범위 없음 — `*Label` 접미사만으로 동적 감지

## 환경변수 (siem/.env)

```env
KAFKA_BOOTSTRAP_SERVERS=43.202.241.121:44105
OPENSEARCH_URL=http://siem-opensearch:9200
DASHBOARD_URL=http://siem-dashboard:8501
TIMEZONE=Asia/Seoul
INDEX_PREFIX=safepc
KAFKA_TRANSFORMED_TOPIC=safepc-siem-events

CEP_PORT=:48084
UEBA_PORT=:48082
FLINK_SQL_GATEWAY=http://203.229.154.49:48083
FLINK_REST_API=http://203.229.154.49:48081
```

## Config 분기 (`config/config.go`)

| 서비스 | EventTopics | GroupID |
|--------|-------------|---------|
| logsink | 원본 11개 토픽 | siem-logsink |
| cep | safepc-siem-events | siem-cep-sql |
| ueba | safepc-siem-events | siem-ueba |

## OpenSearch 인덱스 전체

| 인덱스 패턴 | 용도 | 작성자 |
|-------------|------|--------|
| `safepc-siem-event-logs-YYYY.MM.DD` | 변환된 이벤트 | LogSink |
| `safepc-siem-cep-alerts-YYYY.MM.DD` | CEP Alert | CEP AlertConsumer |
| `safepc-siem-ueba-scores-YYYY.MM.DD` | UEBA 점수 스냅샷 | UEBA |
| `safepc-siem-ueba-baselines` | Baseline (mean/stddev) | UEBA |
| `safepc-siem-common-rules` | 규칙 (CEP/UEBA 공유) | Dashboard/API |
| `safepc-siem-common-settings` | UEBA 설정 + baseline_meta | UEBA/Dashboard |
| `safepc-siem-common-field-meta` | 필드 메타데이터 | CEP/UEBA API |

## 배포 절차

```bash
# 1. 소스 업로드
sshpass -p 'M@rkAny' scp -o StrictHostKeyChecking=no -r \
  cmd/ config/ internal/ main.go go.mod go.sum \
  root@203.229.154.49:/Safepc/SafePC/

# 2. 서비스 빌드 & 재시작
sshpass -p 'M@rkAny' ssh root@203.229.154.49 \
  'cd /Safepc/siem && docker-compose up -d --build cep ueba logsink'

# 3. 대시보드 (볼륨 마운트 — 빌드 불필요, 파일만 scp)
sshpass -p 'M@rkAny' scp -o StrictHostKeyChecking=no \
  siem/dashboard/app.py root@203.229.154.49:/Safepc/siem/dashboard/app.py
sshpass -p 'M@rkAny' scp -o StrictHostKeyChecking=no -r \
  siem/dashboard/templates/ root@203.229.154.49:/Safepc/siem/dashboard/templates/
# 대시보드는 볼륨 마운트라 파일 변경 시 자동 반영 (FastAPI reload)
```

## Confluence 문서

| 페이지 | ID | 버전 |
|--------|-----|------|
| CEP | 1691484182 | v29 |
| UEBA | 1691222054 | v19 |

```bash
# 토큰
cat credentials/confluence_token.txt
# API: https://markany.atlassian.net/wiki/api/v2/pages/{id}
# Auth: khkim1@markany.com + token
```

## 트러블슈팅

### 대시보드에 유저가 안 보임
- UEBA가 시작 시 `saveScoresBatch()`로 즉시 저장 → 대시보드는 scores 인덱스 조회
- scores가 없으면: `curl http://localhost:48082/save` 수동 호출

### Flink Job이 안 뜸
- SQL Gateway 상태: `curl http://203.229.154.49:48083/v1/info`
- Job 목록: `curl http://203.229.154.49:48081/jobs`
- 규칙 재로드: `curl -X POST http://203.229.154.49:48084/api/reload`

### UEBA 점수가 0인 유저
- `status` 확인: `curl http://localhost:48082/api/users`
- `cold_start`: baseline sampleDays < 3 → 3일 대기
- `no_baseline`: 신규 유저 → 자정 baseline 갱신 후 반영
- `active`인데 0점: 이벤트가 없거나 룰 매칭 안 됨
