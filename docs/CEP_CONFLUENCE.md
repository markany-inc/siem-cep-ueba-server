# CEP (Complex Event Processing) 시스템

## 1. 개요

### 1.1 CEP란?

CEP(Complex Event Processing)는 실시간으로 발생하는 이벤트 스트림에서 의미 있는 패턴을 탐지하는 기술입니다.

| 항목 | 설명 |
|------|------|
| 목적 | 보안 위협 패턴을 실시간으로 탐지하여 즉각 대응 |
| 방식 | 규칙 기반 패턴 매칭 (단일 이벤트, 집계, 순차 패턴) |
| 출력 | 탐지 알림 (Alert) |
| 예시 | "10분 내 로그인 실패 5회 이상", "USB 차단 후 클라우드 업로드 시도" |

기존 로그 분석이 사후 조사 중심이라면, CEP는 이벤트 발생 즉시 패턴을 감지하여 선제적 대응이 가능합니다.

### 1.2 시스템 개요

SafePC SIEM의 CEP 모듈은 Apache Flink SQL 기반 실시간 이벤트 탐지 시스템입니다.

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
│                      ┌───────────────────────────┼───────────────┐          │
│                      │                           │               │          │
│                      ▼                           ▼               │          │
│              ┌──────────────┐           ┌──────────────┐        │          │
│              │   LogSink    │           │  Flink SQL   │        │          │
│              │ (원본 저장)  │           │ (패턴 매칭)  │        │          │
│              └──────┬───────┘           └──────┬───────┘        │          │
│                     │                          │                │          │
│                     ▼                          ▼                │          │
│              ┌──────────────┐           ┌──────────────┐        │          │
│              │  OpenSearch  │           │ Kafka Alert  │        │          │
│              │ (event-logs) │           │   Topic      │        │          │
│              └──────────────┘           └──────┬───────┘        │          │
│                                                │                │          │
│                                                ▼                │          │
│                                         ┌──────────────┐        │          │
│                                         │Alert Consumer│        │          │
│                                         │ (알림 저장)  │        │          │
│                                         └──────┬───────┘        │          │
│                                                │                │          │
│                                                ▼                │          │
│                                         ┌──────────────┐        │          │
│                                         │  OpenSearch  │        │          │
│                                         │ (cep-alerts) │        │          │
│                                         └──────────────┘        │          │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 2. 서버 구성

### 2.1 Docker 컨테이너 구성

| 컨테이너 | 포트 | 역할 |
|----------|------|------|
| siem-cep | 48084 | CEP 메인 서버 (API + LogSink + Alert Consumer) |
| siem-flink-jobmanager | 48081 | Flink Job 관리 |
| siem-flink-sql-gateway | 48083 | Flink SQL 실행 |
| siem-opensearch | 49200 | 데이터 저장소 |
| siem-dashboard | 48501 | 웹 대시보드 |

### 2.2 CEP 메인 서버 (siem-cep) 역할

CEP 컨테이너는 3가지 역할을 동시에 수행합니다:

```
┌─────────────────────────────────────────────────────────┐
│                    siem-cep 컨테이너                     │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  1. Echo API 서버 (:48084)                              │
│     - 규칙 CRUD API                                     │
│     - Field Meta API (스키마 관리)                      │
│     - Flink Job 관리 API                                │
│                                                         │
│  2. LogSink (Kafka Consumer)                            │
│     - Kafka 토픽 구독 (MESSAGE_*)                       │
│     - 원본 이벤트 → OpenSearch 저장                     │
│     - 인덱스: {prefix}-event-logs-YYYY.MM.DD            │
│                                                         │
│  3. Alert Consumer (Kafka Consumer)                     │
│     - cep-alerts 토픽 구독                              │
│     - Flink 탐지 결과 → OpenSearch 저장                 │
│     - 인덱스: {prefix}-cep-alerts-YYYY.MM.DD            │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

### 2.3 OpenSearch 인덱스

| 인덱스 패턴 | 용도 |
|-------------|------|
| {prefix}-event-logs-YYYY.MM.DD | 원본 이벤트 로그 |
| {prefix}-cep-alerts-YYYY.MM.DD | CEP 탐지 알림 |
| {prefix}-cep-rules | CEP 규칙 저장 |
| {prefix}-field-meta | 필드 메타데이터 |

---

## 3. 데이터 흐름 상세

### 3.1 이벤트 수집 (Log Adapter)

```
소스 시스템 → Syslog CEF → Log Adapter → Kafka

CEF 원본:
<134>1 2026-02-25T10:00:00Z HOST SafePC - MESSAGE_DEVICE_USAGE - 
CEF:0|MarkAny|SafePC|8.0|10101001|Device Usage|5|
suid=kim suser=김철수 src=192.168.1.100 act=write outcome=blocked fname=secret.docx

Log Adapter 변환 후 (Kafka JSON):
{
  "msgId": "MESSAGE_DEVICE_USAGE",
  "hostname": "HOST",
  "appName": "SafePC",
  "cefExtensions": {
    "suid": "kim",
    "suser": "김철수",
    "src": "192.168.1.100",
    "act": "write",
    "outcome": "blocked",
    "fname": "secret.docx"
  }
}
```

### 3.2 로그 저장 (LogSink)

```
Kafka (MESSAGE_*) → LogSink → OpenSearch

- JSON 그대로 저장 (파싱/변환 없음)
- 인덱스: safepc-siem-event-logs-2026.02.25
- @timestamp 자동 추가
```

### 3.3 실시간 탐지 (Flink SQL)

```
Kafka (MESSAGE_*) → Flink SQL → Kafka (cep-alerts)

Flink 테이블 스키마:
CREATE TABLE events (
  msgId STRING,
  hostname STRING,
  cefExtensions MAP<STRING, STRING>,  -- 동적 필드
  userId AS cefExtensions['suid'],    -- 편의 alias
  proctime AS PROCTIME()
)
```

### 3.4 알림 후처리

Flink에서 탐지된 알림은 Kafka cep-alerts 토픽으로 전송됩니다.
외부 시스템에서 이 토픽을 구독하여 추가 후처리(알림 발송, 티켓 생성 등)가 가능합니다.

```
Flink 탐지 → Kafka (cep-alerts) → Alert Consumer → OpenSearch
                    ↓
              외부 시스템 구독 가능
              (이메일, Slack, 티켓 시스템 등)
```

---

## 4. 마이그레이션 (스키마 정규화)

### 4.1 개념

새 시스템 연동 시 저장된 로그 데이터에서 이벤트 타입과 필드를 자동 발견하고, 관리자가 설정하여 프론트엔드 규칙 빌더 UI에서 사용할 수 있게 합니다.

### 4.2 전제 조건

- 최소 수일치 이벤트 로그가 OpenSearch에 저장되어 있어야 함
- 분석하려는 이벤트 타입(msgId)이 로그에 존재해야 함

### 4.3 마이그레이션 프로세스

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         마이그레이션 → 규칙 생성 흐름                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Step 1: 로그 분석 (서버)                                                   │
│  ────────────────────────                                                   │
│  POST /api/field-meta/analyze {"days": 7}                                   │
│  → OpenSearch에서 7일치 로그 조회                                           │
│  → msgId별 cefExtensions 필드 목록 자동 추출                                │
│                                                                             │
│  Step 2: 필드 설정 (관리자 UI)                                              │
│  ──────────────────────────────                                             │
│  - 필드별 inputType 지정 (input, select, checkbox, number)                  │
│  - select/checkbox는 값 목록 수집 또는 수동 입력                            │
│  - 한글 라벨 지정                                                           │
│                                                                             │
│  Step 3: 메타데이터 저장 (서버)                                             │
│  ──────────────────────────────                                             │
│  PUT /api/field-meta → OpenSearch에 저장                                    │
│                                                                             │
│  Step 4: 프론트엔드 규칙 빌더 UI 제공                                       │
│  ─────────────────────────────────────                                      │
│  GET /api/field-meta → 저장된 스키마 조회                                   │
│  → 이벤트 선택 드롭다운, 필드별 조건 입력 UI 동적 생성                      │
│                                                                             │
│  Step 5: 규칙 생성 요청 (프론트 → 서버)                                     │
│  ─────────────────────────────────────────                                  │
│  POST /api/rules (CEP 규칙 JSON)                                            │
│  → SQL Builder가 Flink SQL로 변환                                           │
│  → Flink SQL Gateway를 통해 Job 제출                                        │
│  → Flink Job으로 실시간 탐지 시작                                           │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 4.4 Field Meta 스키마 JSON 예제

GET /api/field-meta 응답 (프론트엔드에 전달되는 스키마):

```json
{
  "migratedAt": "2026-02-25T10:00:00Z",
  "events": {
    "MESSAGE_DEVICE_USAGE": {
      "enabled": true,
      "label": "매체 사용",
      "fields": {
        "outcome": {
          "inputType": "select",
          "label": "결과",
          "options": [
            {"value": "allowed", "label": "허용"},
            {"value": "blocked", "label": "차단"}
          ]
        },
        "act": {
          "inputType": "select",
          "label": "동작",
          "options": [
            {"value": "read", "label": "읽기"},
            {"value": "write", "label": "쓰기"}
          ]
        },
        "fname": {
          "inputType": "input",
          "label": "파일명"
        },
        "fsize": {
          "inputType": "number",
          "label": "파일크기(bytes)"
        }
      }
    },
    "MESSAGE_AGENT_AUTHENTICATION": {
      "enabled": true,
      "label": "에이전트 인증",
      "fields": {
        "outcome": {
          "inputType": "select",
          "label": "결과",
          "options": [
            {"value": "success", "label": "성공"},
            {"value": "failure", "label": "실패"}
          ]
        },
        "reason": {
          "inputType": "input",
          "label": "사유"
        }
      }
    }
  }
}
```

### 4.5 Field Meta API

| Method | Path | 설명 |
|--------|------|------|
| POST | /api/field-meta/analyze | N일치 로그에서 이벤트/필드 추출 |
| POST | /api/field-meta/analyze-field | 특정 필드의 값 목록 수집 |
| GET | /api/field-meta | 저장된 메타데이터 조회 |
| PUT | /api/field-meta | 메타데이터 저장 |

---

## 5. CEP 규칙 언어

### 5.1 지원 연산자

| 연산자 | 설명 | SQL 변환 | 예시 |
|--------|------|----------|------|
| eq | 같음 | = | {"field": "outcome", "op": "eq", "value": "blocked"} |
| neq | 다름 | != | {"field": "act", "op": "neq", "value": "read"} |
| gt | 초과 | CAST > | {"field": "fsize", "op": "gt", "value": 10485760} |
| gte | 이상 | CAST >= | {"field": "fsize", "op": "gte", "value": 1048576} |
| lt | 미만 | CAST < | {"field": "fsize", "op": "lt", "value": 1024} |
| lte | 이하 | CAST <= | {"field": "fsize", "op": "lte", "value": 1024} |
| in | 목록 포함 | IN () | {"field": "outcome", "op": "in", "value": ["blocked", "denied"]} |
| like | 패턴 매칭 | LIKE | {"field": "fname", "op": "like", "value": "%.exe"} |
| regex | 정규식 | REGEXP() | {"field": "fname", "op": "regex", "value": ".*\\.(exe|dll)"} |
| time_range | 시간대 | HOUR() | {"field": "hour", "op": "time_range", "start": 22, "end": 6} |

### 5.2 시간 윈도우 형식

| 형식 | 예시 | 설명 |
|------|------|------|
| Ns | 30s | 30초 |
| Nm | 5m | 5분 |
| Nh | 1h | 1시간 |

### 5.3 집계 조건 (aggregate)

시간 윈도우 내 조건 만족 시 탐지합니다.

#### 기본 구조

| 필드 | 설명 | 필수 |
|------|------|------|
| function | 집계 함수 (count/sum/count_distinct) | 예 |
| field | 집계 대상 필드 | sum, count_distinct 시 |
| min | 최소 임계값 (이상) | 예 |
| within | 시간 윈도우 (예: 10m, 1h) | 예 |

#### 예제 1: 횟수 기반 (count)

10분 내 로그인 실패 5회 이상:

```json
{
  "name": "로그인실패_반복",
  "match": {
    "msgId": "MESSAGE_AGENT_AUTHENTICATION",
    "conditions": [{"field": "outcome", "op": "eq", "value": "failure"}]
  },
  "aggregate": {"function": "count", "min": 5, "within": "10m"},
  "group_by": ["suser"]
}
```

#### 예제 2: 합계 기반 (sum)

1시간 내 파일 전송량 100MB 이상:

```json
{
  "name": "대용량_파일전송",
  "match": {
    "msgId": "MESSAGE_FILE_TRANSFER",
    "conditions": [{"field": "act", "op": "eq", "value": "upload"}]
  },
  "aggregate": {"function": "sum", "field": "fsize", "min": 104857600, "within": "1h"},
  "group_by": ["suser"]
}
```

#### 예제 3: 고유값 기반 (count_distinct)

30분 내 서로 다른 호스트 10개 이상 접근:

```json
{
  "name": "다중호스트_접근",
  "match": {"msgId": "MESSAGE_NETWORK_ACCESS"},
  "aggregate": {"function": "count_distinct", "field": "dhost", "min": 10, "within": "30m"},
  "group_by": ["suser"]
}
```

#### 예제 4: 순차 패턴 + 집계 (patterns + aggregate)

로그인 후 10분 내 프린트 3회 이상:

```json
{
  "name": "로그인후_프린트반복",
  "patterns": [
    {"id": "login", "order": 1, "match": {"msgId": "MESSAGE_LOGIN"}},
    {"id": "print", "order": 2, "match": {"msgId": "MESSAGE_PRINT"}, "quantifier": {"min": 3}}
  ],
  "aggregate": {"within": "10m"},
  "group_by": ["suser"]
}
```

#### 예제 5: 순차 패턴 + 합계 조건

로그인 후 30분 내 파일 복사 총 50MB 이상:

```json
{
  "name": "로그인후_대량복사",
  "patterns": [
    {"id": "login", "order": 1, "match": {"msgId": "MESSAGE_LOGIN"}},
    {"id": "copy", "order": 2, "match": {"msgId": "MESSAGE_FILE_COPY"}}
  ],
  "aggregate": {"function": "sum", "field": "fsize", "min": 52428800, "within": "30m"},
  "group_by": ["suser"]
}
```

### 5.4 순차/분기 패턴 (patterns, paths)

이벤트 발생 순서를 지정하여 탐지합니다.

#### 순차 패턴 예제

로그인 실패 후 5분 내 성공:

```json
{
  "name": "로그인_실패후_성공",
  "description": "로그인 실패 후 5분 내 성공 (무차별 대입 의심)",
  "severity": "medium",
  "enabled": true,
  "patterns": [
    {
      "id": "fail",
      "order": 1,
      "match": {
        "msgId": "MESSAGE_AGENT_AUTHENTICATION",
        "conditions": [{"field": "outcome", "op": "eq", "value": "failure"}]
      }
    },
    {
      "id": "success",
      "order": 2,
      "match": {
        "msgId": "MESSAGE_AGENT_AUTHENTICATION",
        "conditions": [{"field": "outcome", "op": "eq", "value": "success"}]
      }
    }
  ],
  "aggregate": {"within": "5m"},
  "group_by": ["suser"]
}
```

#### 분기 패턴 예제

로그인 실패 → (USB 또는 프린터 차단) → 파일 반출:

```json
{
  "name": "로그인후_우회시도",
  "description": "로그인 실패 후 매체 차단, 이후 파일 반출 시도",
  "severity": "high",
  "enabled": true,
  "patterns": [
    {"id": "login_fail", "order": 1, "match": {"msgId": "MESSAGE_LOGIN_FAIL"}},
    {"id": "usb_block", "order": 2, "match": {"msgId": "MESSAGE_USB_BLOCK"}},
    {"id": "print_block", "order": 2, "match": {"msgId": "MESSAGE_PRINT_BLOCK"}},
    {"id": "file_export", "order": 3, "match": {"msgId": "MESSAGE_FILE_EXPORT"}}
  ],
  "paths": [
    ["login_fail", "usb_block", "file_export"],
    ["login_fail", "print_block", "file_export"]
  ],
  "aggregate": {"within": "10m"},
  "group_by": ["suser"]
}
```

#### 필드 설명

| 필드 | 설명 |
|------|------|
| patterns[].id | 패턴 식별자 (paths에서 참조) |
| patterns[].order | 단계 순서 (같은 order면 병렬 분기) |
| patterns[].match | 매칭 조건 (msgId, conditions) |
| patterns[].quantifier | 반복 횟수 (예: {"min": 3} = 3회 이상) |
| paths | 유효한 경로 목록 (없으면 order 순서대로 선형) |
| aggregate.within | 전체 패턴 시간 윈도우 |

---

## 6. CEP 규칙 API

### 6.1 규칙 생성

POST /api/rules

### 6.2 단일 패턴 (즉시 탐지)

조건 매칭 시 즉시 알림:

```json
{
  "name": "USB_차단_탐지",
  "description": "USB 매체 차단 발생 시 즉시 알림",
  "severity": "medium",
  "enabled": true,
  "match": {
    "msgId": "MESSAGE_DEVICE_USAGE",
    "conditions": [
      {"field": "outcome", "op": "eq", "value": "blocked"}
    ]
  }
}
```

### 6.3 집계 패턴 (N회 이상)

지정 시간 내 N회 이상 발생 시 알림:

```json
{
  "name": "로그인_실패_반복",
  "description": "10분 내 로그인 실패 5회 이상",
  "severity": "high",
  "enabled": true,
  "match": {
    "msgId": "MESSAGE_AGENT_AUTHENTICATION",
    "conditions": [
      {"field": "outcome", "op": "eq", "value": "failure"}
    ]
  },
  "aggregate": {
    "function": "count",
    "min": 5,
    "within": "10m"
  },
  "group_by": ["suser"]
}
```

### 6.4 순차 패턴 (A → B 순서)

이벤트가 특정 순서로 발생 시 탐지:

```json
{
  "name": "로그인_실패후_성공",
  "description": "로그인 실패 후 5분 내 성공 시 탐지 (무차별 대입 의심)",
  "severity": "medium",
  "enabled": true,
  "patterns": [
    {
      "id": "fail",
      "order": 1,
      "match": {
        "msgId": "MESSAGE_AGENT_AUTHENTICATION",
        "conditions": [
          {"field": "outcome", "op": "eq", "value": "failure"}
        ]
      }
    },
    {
      "id": "success",
      "order": 2,
      "match": {
        "msgId": "MESSAGE_AGENT_AUTHENTICATION",
        "conditions": [
          {"field": "outcome", "op": "eq", "value": "success"}
        ]
      }
    }
  ],
  "aggregate": {
    "within": "5m"
  },
  "group_by": ["suser"]
}
```

### 6.5 분기 패턴 (A → B|C → D)

병렬 분기 구조 탐지. paths로 유효한 경로를 명시:

```
다이어그램:
    ┌─→ [usb_block] ─┐
[login_fail]─┤              ├─→ [file_export]
    └─→ [print_block] ─┘
```

```json
{
  "name": "로그인후_우회시도",
  "description": "로그인 실패 후 USB 또는 프린터 차단, 이후 파일 반출 시도",
  "severity": "high",
  "enabled": true,
  "patterns": [
    {
      "id": "login_fail",
      "order": 1,
      "match": {"msgId": "MESSAGE_LOGIN_FAIL"}
    },
    {
      "id": "usb_block",
      "order": 2,
      "match": {"msgId": "MESSAGE_USB_BLOCK"}
    },
    {
      "id": "print_block",
      "order": 2,
      "match": {"msgId": "MESSAGE_PRINT_BLOCK"}
    },
    {
      "id": "file_export",
      "order": 3,
      "match": {"msgId": "MESSAGE_FILE_EXPORT"}
    }
  ],
  "paths": [
    ["login_fail", "usb_block", "file_export"],
    ["login_fail", "print_block", "file_export"]
  ],
  "aggregate": {
    "within": "10m"
  },
  "group_by": ["suser"]
}
```

### 6.6 동시 패턴 (OR 조건)

여러 이벤트 중 하나라도 발생 시 탐지:

```json
{
  "name": "매체_차단_통합",
  "description": "USB 또는 프린터 차단 발생",
  "severity": "medium",
  "enabled": true,
  "match": {
    "any": [
      {
        "msgId": "MESSAGE_DEVICE_USAGE",
        "conditions": [{"field": "outcome", "op": "eq", "value": "blocked"}]
      },
      {
        "msgId": "MESSAGE_PRINT_USAGE",
        "conditions": [{"field": "outcome", "op": "eq", "value": "blocked"}]
      }
    ]
  }
}
```

---

## 7. Flink Job 생성 과정

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         규칙 → Flink Job 생성 과정                           │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. 규칙 저장                                                               │
│     POST /api/rules (CEP 규칙 JSON)                                         │
│     → OpenSearch cep-rules 인덱스에 저장                                    │
│                                                                             │
│  2. SQL 변환                                                                │
│     규칙 JSON → SQL Builder → Flink SQL                                     │
│     - match.conditions → WHERE 절                                           │
│     - aggregate → GROUP BY TUMBLE + HAVING                                  │
│     - patterns[].order → MATCH_RECOGNIZE PATTERN                            │
│                                                                             │
│  3. Flink 세션 확보                                                         │
│     Flink SQL Gateway에 세션 생성                                           │
│     events/alerts 테이블 DDL 실행 (최초 1회)                                │
│                                                                             │
│  4. Job 제출                                                                │
│     INSERT INTO alerts                                                      │
│       SELECT ruleId, ruleName, severity, userId, hostname, userIp, cnt, ts  │
│       FROM (규칙 SQL)                                                       │
│     → Flink SQL Gateway를 통해 실행                                         │
│     → Flink Job으로 등록되어 실시간 탐지 시작                               │
│                                                                             │
│  5. 탐지 결과                                                               │
│     Kafka 이벤트 → Flink 패턴 매칭 → Kafka cep-alerts 토픽                  │
│     → Alert Consumer → OpenSearch 저장                                      │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 8. API 명세

### 8.1 규칙 API

| Method | Path | 설명 |
|--------|------|------|
| GET | /api/rules | 규칙 목록 |
| POST | /api/rules | 규칙 생성 + Flink Job 제출 |
| PUT | /api/rules/:id | 규칙 수정 (기존 Job 종료 → 새 Job 시작) |
| DELETE | /api/rules/:id | 규칙 삭제 + Job 종료 |
| POST | /api/build-sql | 규칙 JSON → SQL 미리보기 |

규칙 수정/삭제 시 동작:
- PUT: 기존 Flink Job 취소 → 규칙 저장 → 새 Job 제출 (enabled=true인 경우)
- PUT (enabled=false): 기존 Job 취소만 수행
- DELETE: 기존 Job 취소 → 규칙 삭제

### 8.2 Job API

| Method | Path | 설명 |
|--------|------|------|
| POST | /api/submit | 수동 Job 제출 |
| POST | /api/reload | 전체 규칙 재로드 |
| GET | /api/status | 실행 중인 Job 상태 |

### 8.3 Alert API

| Method | Path | 설명 |
|--------|------|------|
| GET | /api/alerts | 탐지 알림 목록 |

---

## 9. 환경 설정

### 9.1 .env 파일

```
OPENSEARCH_URL=http://203.229.154.49:49200
FLINK_SQL_GATEWAY=http://203.229.154.49:48083
FLINK_REST_API=http://203.229.154.49:48081
KAFKA_BOOTSTRAP_SERVERS=43.202.241.121:44105
KAFKA_EVENT_TOPICS=MESSAGE_AGENT,MESSAGE_DEVICE,MESSAGE_NETWORK,...
KAFKA_ALERT_TOPIC=cep-alerts
INDEX_PREFIX=safepc-siem
```

### 9.2 새 이벤트 토픽 추가

1. .env의 KAFKA_EVENT_TOPICS에 토픽 추가
2. CEP 서비스 재시작: docker-compose restart cep
3. 마이그레이션 UI에서 "분석 실행"
4. 필드 설정 후 저장

---

## 10. 제약 사항

### 10.1 Syslog CEF 형식 요구사항

CEP 시스템이 동작하려면 소스 시스템에서 Syslog CEF 형식으로 로그를 전송하고, Log Adapter가 정규화된 JSON으로 변환하여 Kafka에 전달해야 합니다.

```
필수 구조:

Syslog CEF → Log Adapter → Kafka JSON

CEF 필수 필드:
- signatureId (→ msgId): 이벤트 타입 구분자
- suid: 사용자 ID
- shost: 호스트명

Kafka JSON 필수 구조:
{
  "msgId": "이벤트타입",
  "hostname": "호스트명",
  "cefExtensions": {
    "suid": "사용자ID",
    "필드명": "값",
    ...
  }
}
```

### 10.2 필드 제약

| 항목 | 제약 |
|------|------|
| 기본 필드 | msgId, hostname (테이블에 직접 정의) |
| CEF 필드 | cefExtensions MAP 내부 필드만 조건 사용 가능 |
| 타입 | 모든 값은 문자열로 저장 (숫자 비교 시 CAST 사용) |

### 10.3 집계 제약

- TUMBLE 윈도우만 지원
- 집계 함수: COUNT만 지원
- by 필드 기본값: ["userId"]

---

## 11. 신규 사이트 구축 시나리오

### 11.1 전체 흐름

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        신규 사이트 CEP 구축 시나리오                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Phase 1: 인프라 구성                                                       │
│  ─────────────────────                                                      │
│  - Kafka, Flink, OpenSearch, CEP 컨테이너 배포                              │
│  - .env 설정 (Kafka 토픽, 인덱스 접두사 등)                                 │
│  - CEP 서비스 시작 시 연결 상태 로그 출력                                   │
│                                                                             │
│  Phase 2: 로그 수집 (며칠간)                                                │
│  ─────────────────────────                                                  │
│  - Log Adapter가 Syslog CEF → Kafka 전송                                    │
│  - CEP LogSink가 Kafka → OpenSearch 저장                                    │
│  - 이 시점에는 CEP 규칙 없음, 로그만 쌓임                                   │
│  - 최소 수일치 로그 축적 필요                                               │
│                                                                             │
│  Phase 3: 마이그레이션 (스키마 정규화)                                      │
│  ─────────────────────────────────────                                      │
│  - POST /api/field-meta/analyze {"days": 7}                                 │
│  - 쌓인 로그에서 이벤트 타입/필드 자동 발견                                 │
│  - 관리자가 UI에서 필드 설정 (inputType, label, options)                    │
│  - PUT /api/field-meta 저장                                                 │
│                                                                             │
│  Phase 4: 규칙 생성                                                         │
│  ─────────────────────                                                      │
│  - 프론트엔드 규칙 빌더 UI에서 조건 설정                                    │
│  - POST /api/rules → Flink Job 생성                                         │
│  - 실시간 탐지 시작                                                         │
│                                                                             │
│  Phase 5: 운영                                                              │
│  ─────────────                                                              │
│  - 탐지 알림 모니터링                                                       │
│  - 규칙 튜닝 (PUT /api/rules/:id)                                           │
│  - 새 이벤트 타입 추가 시 마이그레이션 재실행                               │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 11.2 CEP 서비스 시작 로그 예시

```
2026/02/25 10:52:39 CEP 서비스 시작...
2026/02/25 10:52:39   Port: :48084
2026/02/25 10:52:39   OpenSearch: http://203.229.154.49:49200
2026/02/25 10:52:39   Flink: http://203.229.154.49:48081
2026/02/25 10:52:39   Kafka: 43.202.241.121:44105
2026/02/25 10:52:39 [CEP] Alert consumer 시작 중...
2026/02/25 10:52:39 [CEP] Log sink 시작 중...
⇨ http server started on [::]:48084
2026/02/25 10:52:39 [LogSink] 시작: 11개 토픽 구독
2026/02/25 10:52:39 [LogSink] 토픽 MESSAGE_AGENT: 6개 파티션
2026/02/25 10:52:40 [Flink] 세션: f6060ad2-ca41-4eea-9167-f149998b6b48
2026/02/25 10:52:40 [Flink] 테이블 생성 완료
```

---

## 12. 운영 명령어

```bash
# CEP 서비스 재시작
docker-compose restart cep

# 로그 확인
docker logs -f siem-cep

# 규칙 전체 재로드
curl -X POST http://localhost:48084/api/reload

# Flink Job 상태
curl http://localhost:48084/api/status

# SQL 빌드 테스트
curl -X POST http://localhost:48084/api/build-sql \
  -H 'Content-Type: application/json' \
  -d '{"match":{"msgId":"MESSAGE_DEVICE_USAGE","conditions":[{"field":"outcome","op":"eq","value":"blocked"}]}}'
```
