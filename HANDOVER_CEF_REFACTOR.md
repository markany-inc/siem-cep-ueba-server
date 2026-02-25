# CEF Label 동적 처리 리팩토링 인수인계서

## 현재 문제

Flink CEP가 Kafka 원본 이벤트를 직접 구독하는데, CEF 커스텀 필드(cs1, cn1 등)의 label 매핑이 동적이라 SQL에서 label 이름으로 접근할 수 없음.

- `cs1Label=Config Type, cs1=ipchange` → SQL에서 `ConfigType`으로 접근 불가
- 현재 CASE WHEN 또는 OpenSearch 조회로 우회 중 → 범위 제한/하드코딩 문제
- CEF 표준상 csN/cnN 번호는 무한히 늘어날 수 있어 범위 고정 불가

## 결정된 아키텍처

### AS-IS
```
Kafka(원본) ──→ Flink(직접 구독, 원본 그대로) ──→ cep-alerts 토픽
             ──→ LogSink(변환 + OpenSearch 저장)
             ──→ UEBA(직접 구독, 원본 그대로)
```
- LogSink가 CEP 컨테이너 안에 있어서 CEP 내려도 Flink Job은 계속 실행됨
- label 변환은 OpenSearch 저장 시에만 적용, Flink/UEBA는 원본 사용

### TO-BE
```
Kafka(원본 이벤트)
  │
  ├─→ CEP Consumer
  │     ├─ 원본 구독
  │     ├─ CEF label 변환 (expandCEFLabels)
  │     ├─ 변환된 토픽 발행 (safepc-cep-events)
  │     └─ Flink가 변환 토픽 구독 → cefExtensions['ConfigType'] 직접 접근 가능
  │
  └─→ UEBA Consumer
        ├─ 원본 구독
        ├─ CEF label 변환 (expandCEFLabels)
        ├─ OpenSearch event-logs 저장 (baseline/검색용)
        └─ 변환된 데이터로 룰 매칭 + 점수 계산
```

### 핵심 원칙
- **CEP**: 로그 저장 안 함. alert만 저장. CEP 내리면 변환 토픽 발행 중단 → Flink 읽을 데이터 없음
- **UEBA**: event-logs 저장 담당. baseline/anomaly 계산에 전체 로그 필요
- **label 변환**: 각 서비스 입구에서 1회 수행. 하드코딩/범위 제한 완전 제거
- **LogSink 제거**: 독립 컴포넌트 대신 각 서비스가 자기 책임으로 변환

## 변환 토픽 자동 생성 규칙

토픽 네이밍: `{INDEX_PREFIX}-{service}-events`
- CEP용: `safepc-cep-events`
- UEBA용: `safepc-ueba-events` (필요 시)

토픽이 없으면 Consumer 시작 시 자동 생성 (Kafka auto.create.topics 또는 AdminClient).

## 구현 계획

### 1단계: CEP Consumer 변환 토픽 발행
- `internal/cep/services/consumer.go`
- 기존 LogSink의 `sinkEvent()` 대신 변환 후 Kafka producer로 `safepc-cep-events` 토픽에 발행
- `expandCEFLabels()` 함수는 그대로 사용 (이미 *Label 접미사 동적 처리)
- LogSink의 OpenSearch 저장 로직 제거 (UEBA가 담당)

### 2단계: Flink 테이블 변경
- `internal/cep/services/flink.go`의 `EnsureSession()`
- events 테이블의 Kafka 토픽을 원본 → `safepc-cep-events`로 변경
- consumer group도 CEP 전용으로 분리

### 3단계: SQL builder 단순화
- `internal/cep/services/util.go`의 `cefField()`
- CASE WHEN / resolveLabelKey 전부 제거
- 모든 필드를 `cefExtensions['필드명']`으로 직접 접근 (변환 토픽이니까)
- `isCEFRawKey`, `cefRawKeys` 등 불필요한 분기 제거

### 4단계: UEBA Consumer 변경
- `internal/ueba/services/processor.go`
- 이벤트 수신 시 `expandCEFLabels()` 적용 후 처리
- event-logs OpenSearch 저장 로직을 UEBA로 이관 (현재 CEP LogSink에서 수행)
- `parseCEFAttrs()` 함수도 변환된 데이터 기반으로 단순화

### 5단계: field-meta 정리
- `internal/common/fieldmeta.go`
- `analyzeEvent()`: 이미 label 이름으로 반환하도록 수정 완료
- `analyzeFieldDetail()`: 이미 동적 매핑 조회로 수정 완료
- UEBA가 event-logs를 저장하므로 field-meta는 UEBA 쪽 데이터 기반

## 현재 완료된 작업 (이번 세션)

### CEF label 동적 처리
- `expandCEFLabels()`: `*Label` 접미사 동적 탐색, 고정 prefix 없음 ✅
- `analyzeEvent()`: cs/cn raw 키 대신 label 이름 반환 ✅
- `analyzeFieldDetail()`: 실제 문서에서 매핑 키 동적 탐색 ✅

### CEP 규칙 마이그레이션
- `window.threshold/minutes` → `aggregate.count.min/within` 변환 완료 ✅
- 레거시 `sql`, `conditions` 필드 제거 완료 ✅
- 18개 CEP 규칙 정상 동작 확인 ✅

### ReloadAll 개선
- 전체 취소 후 재제출 방식으로 변경 (중복 Job 방지) ✅
- 직렬 제출 (Flink SQL Gateway 세션 공유 문제 해결) ✅

### 대시보드 수정
- 점수 변화: `prevScore` → 어제 최종 점수 직접 조회로 통일 ✅
- 점수 구성: "점수 변화 상세" → "오늘 점수 구성" (decay+rule+anomaly=total) ✅
- UEBA Settings: 하드코딩 weights 제거, OpenSearch 규칙에서 동적 로드 ✅

### 데이터 마이그레이션
- 2/19~2/20 테스트 데이터 삭제 ✅
- 2/21~2/25 scores 문서 prevScore/decayedPrev/riskScore 연쇄 재계산 ✅
- UEBA 전용 규칙의 잘못된 CEP alert 3건 삭제 ✅

## 서버 정보
- 배포 서버: 203.229.154.49
- SSH: `sshpass -p 'M@rkAny' ssh root@203.229.154.49`
- 소스: `/Safepc/SafePC/`
- Docker: `/Safepc/siem/` (cep, ueba), `/Safepc/siem/dashboard/`
- Git: `https://github.com/markany-inc/siem-cep-ueba-server` main

## 미커밋 파일
- `internal/cep/services/util.go` — cefField, resolveLabelKey, expandCEFLabels 관련
- `internal/cep/services/consumer.go` — expandCEFLabels 동적화
- `internal/cep/controllers/job.go` — ReloadAll 단순화
- `internal/cep/services/flink.go` — GetRunningCEPJobs 맵 변경
- `internal/common/fieldmeta.go` — analyzeEvent/analyzeFieldDetail label 변환
- `cmd/cep.go` — InitLabelResolver 호출 추가
- `siem/dashboard/app.py` — get_yesterday_scores, scoreDiff 통일
- `siem/dashboard/templates/user_detail.html` — 오늘 점수 구성
- `siem/dashboard/templates/settings.html` — 하드코딩 weights 제거
