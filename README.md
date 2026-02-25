# SafePC SIEM

CEP (Complex Event Processing) + UEBA (User Entity Behavior Analytics) 통합 시스템

## 아키텍처

- **Cobra CLI**: `go run main.go {cep|ueba}` 명령어 기반
- **Echo Framework**: HTTP API 서버
- **Viper**: 환경변수 설정 관리 (`.env` 파일 + 환경변수)
- **구조**: `controllers/` (HTTP 핸들러) + `services/` (비즈니스 로직)

## 프로젝트 구조

```
SafePC/
├── cmd/                     # Cobra CLI 커맨드
│   ├── root.go
│   ├── cep.go               # CEP echo 서버 + 라우팅
│   └── ueba.go              # UEBA echo 서버 + 라우팅
├── config/
│   └── config.go            # Viper 기반 설정 로더
├── internal/
│   ├── common/              # 공통 로직
│   │   ├── opensearch.go    # OpenSearch 클라이언트
│   │   └── fieldmeta.go     # field-meta API (CEP/UEBA 공통)
│   ├── cep/                 # CEP 로직
│   │   ├── controllers/
│   │   │   ├── rule.go      # 규칙 CRUD
│   │   │   └── job.go       # Flink Job 관리
│   │   └── services/
│   │       ├── flink.go     # FlinkService
│   │       ├── sql_builder.go # Rule → Flink SQL
│   │       └── util.go
│   └── ueba/                # UEBA 로직
│       ├── controllers/
│       │   ├── rule.go      # 규칙 CRUD
│       │   ├── status.go    # 상태/헬스체크
│       │   └── user.go      # 사용자 위험 점수
│       └── services/
│           └── processor.go # Kafka consumer + 점수 계산
├── siem/                    # 배포 디렉토리
│   ├── .env                 # 환경변수 (서버 이관 시 수정)
│   ├── docker-compose.yml
│   ├── cep/Dockerfile
│   └── ueba/Dockerfile
├── go.mod                   # Go 1.21, echo, cobra, viper, sarama
└── main.go                  # 엔트리포인트
```

## 로컬 실행

```bash
# CEP 서비스
go run main.go cep

# UEBA 서비스
go run main.go ueba
```

## Docker 빌드 & 배포

```bash
cd siem
docker-compose build
docker-compose up -d
```

## 환경변수 (siem/.env)

```env
KAFKA_BOOTSTRAP_SERVERS=43.202.241.121:44105
OPENSEARCH_URL=http://siem-opensearch:9200
DASHBOARD_URL=http://siem-dashboard:8501
TIMEZONE=Asia/Seoul
```

## 공통 API (CEP/UEBA 모두 제공)

### Field Metadata
- `GET /api/field-meta` — 최신 필드 메타데이터 조회
- `PUT /api/field-meta` — 필드 메타데이터 저장
- `POST /api/field-meta/analyze` — 이벤트별 필드 목록 분석
- `POST /api/field-meta/analyze-field` — 특정 필드 상세 분석

## CEP 전용 API
- `GET /api/rules` — 규칙 목록
- `POST /api/rules` — 규칙 생성
- `PUT /api/rules/{id}` — 규칙 수정
- `POST /api/submit` — Flink Job 제출
- `POST /api/reload` — 전체 규칙 재로드

## UEBA 전용 API
- `GET /api/rules` — 규칙 목록
- `POST /api/rules` — 규칙 생성
- `PUT /api/rules/{id}` — 규칙 수정
- `GET /api/health` — 헬스체크 (메모리, 데이터 상태)
- `GET /api/status` — 서비스 상태
- `GET /api/config` — UEBA 설정 조회
- `GET /api/users` — 사용자 위험 점수 목록
- `GET /api/users/{id}` — 사용자 상세

## 환경변수 (siem/.env)

```env
KAFKA_BOOTSTRAP_SERVERS=43.202.241.121:44105
OPENSEARCH_URL=http://siem-opensearch:9200
DASHBOARD_URL=http://siem-dashboard:8501
TIMEZONE=Asia/Seoul

# 선택적 오버라이드
CEP_PORT=:48084
UEBA_PORT=:48082
FLINK_SQL_GATEWAY=http://203.229.154.49:48083
FLINK_REST_API=http://203.229.154.49:48081
```
