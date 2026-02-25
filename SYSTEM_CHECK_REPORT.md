# SafePC SIEM 시스템 점검 보고서
**점검일시**: 2026-02-25 11:30

---

## 1. 시스템 상태 요약

| 항목 | 상태 | 비고 |
|------|------|------|
| CEP 서비스 | ✅ 정상 | 48084 포트 |
| UEBA 서비스 | ✅ 정상 | 48082 포트 |
| OpenSearch | ✅ 정상 | 49200 포트 |
| Flink | ✅ 정상 | 50개 Job 실행 중 |
| Kafka 연동 | ✅ 정상 | 로그 수신 중 |

---

## 2. 데이터 현황

| 항목 | 수치 |
|------|------|
| 총 로그 | 272,082건 |
| 발견된 이벤트 타입 | 13개 |
| UEBA 규칙 | 10개 |
| UEBA 사용자 | 36명 |
| UEBA Baseline | 899개 |

---

## 3. 기능별 테스트 결과

### 3.1 Kafka → 로그 저장 (LogSink)
```
✅ 정상 동작
- 최근 인덱스: safepc-siem-event-logs-2026.02.25 (3,744건)
- 일일 로그: 약 30,000건/일
```

### 3.2 Field Meta API (마이그레이션)
```
✅ 정상 동작
- POST /api/field-meta/analyze {"days": 3}
- 결과: 13개 이벤트, 필드 자동 추출 성공
```

### 3.3 CEP SQL 빌드
```
✅ 정상 동작

단일 패턴:
  입력: {"msgId": "MESSAGE_DEVICE_USAGE", "conditions": [{"field": "outcome", "op": "eq", "value": "blocked"}]}
  출력: SELECT userId, 1 as cnt FROM events WHERE msgId = 'MESSAGE_DEVICE_USAGE' AND cefExtensions['outcome'] = 'blocked'

집계 패턴:
  입력: {"aggregate": {"count": {"min": 5}, "within": "10m"}}
  출력: ... GROUP BY TUMBLE(proctime, INTERVAL '10' MINUTE), userId HAVING COUNT(*) >= 5
```

### 3.4 CEP 규칙 → Flink Job
```
✅ 정상 동작
- POST /api/rules → 규칙 생성 성공 (rule-1771986686)
- Flink Job 자동 제출 확인
```

### 3.5 UEBA 규칙 생성/삭제
```
✅ 정상 동작
- POST /api/rules → 생성 성공
- DELETE /api/rules/:id → 삭제 성공
```

### 3.6 UEBA 점수 계산
```
✅ 정상 동작
- 36명 사용자 점수 계산 중
- 예: whlee (8.32점), jyk (5.62점)
```

---

## 4. 문서 vs 코드 불일치 사항

### 4.1 연산자 차이

| 연산자 | CEP 코드 | CEP 문서 | UEBA 코드 | UEBA 문서 |
|--------|----------|----------|-----------|-----------|
| eq | ✅ | ✅ | ✅ | ✅ |
| neq | ✅ | ✅ | ✅ | ✅ |
| gt/gte/lt/lte | ✅ | ✅ | ✅ | ✅ |
| in | ✅ | ✅ | ✅ | ✅ |
| like | ✅ | ✅ | ❌ | ❌ |
| regex | ✅ | ✅ | ❌ | ❌ |
| contains | ❌ | ❌ | ✅ | ✅ |
| time_range | ✅ | ✅ | ✅ | ✅ |

**결론**: CEP는 like/regex 지원, UEBA는 contains 지원 - 문서와 코드 일치함

### 4.2 기타 확인 사항

| 항목 | 문서 | 코드 | 일치 |
|------|------|------|------|
| CEP 필드 접근 | cefExtensions['field'] | cefExtensions['field'] | ✅ |
| UEBA 점수 저장 | 자정 롤오버 | checkDateRollover() | ✅ |
| UEBA Baseline | 자정 갱신 | updateBaselines() | ✅ |
| Field Meta | msgId.keyword | msgId.keyword | ✅ |

---

## 5. End-to-End 흐름 검증

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         전체 데이터 흐름 검증                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  [소스 시스템] → [Log Adapter] → [Kafka]                                    │
│       ✅              ✅            ✅                                       │
│       │               │             │                                       │
│       │  Syslog CEF   │  JSON 변환  │  MESSAGE_* 토픽                       │
│       └───────────────┴─────────────┘                                       │
│                                     │                                       │
│                    ┌────────────────┼────────────────┐                      │
│                    │                │                │                      │
│                    ▼                ▼                ▼                      │
│              [CEP LogSink]    [Flink SQL]      [UEBA Consumer]              │
│                   ✅              ✅               ✅                        │
│                   │               │                │                        │
│                   ▼               ▼                ▼                        │
│              [OpenSearch]   [Kafka Alert]    [메모리 점수]                  │
│            (event-logs)         ✅               ✅                         │
│                 ✅               │                │                         │
│                                  ▼                ▼                         │
│                           [Alert Consumer]  [자정 저장]                     │
│                                 ✅              ✅                          │
│                                  │               │                          │
│                                  ▼               ▼                          │
│                            [OpenSearch]    [OpenSearch]                     │
│                           (cep-alerts)   (ueba-scores)                      │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 6. 신규 사이트 구축 가능 여부

### 6.1 필수 조건 충족 여부

| 조건 | 상태 | 비고 |
|------|------|------|
| Kafka 구독 | ✅ | MESSAGE_* 토픽 구독 |
| 로그 저장 | ✅ | LogSink → OpenSearch |
| 스키마 발견 | ✅ | field-meta/analyze API |
| CEP 규칙 → Flink SQL | ✅ | sql_builder.go |
| UEBA 규칙 → 점수 계산 | ✅ | processor.go |
| 동적 필드 접근 | ✅ | cefExtensions MAP |

### 6.2 구축 시나리오 실행 가능성

```
Phase 1: 인프라 구성        ✅ 가능 (docker-compose)
Phase 2: 로그 수집          ✅ 가능 (LogSink 자동 저장)
Phase 3: 마이그레이션       ✅ 가능 (field-meta/analyze)
Phase 4: 규칙 생성          ✅ 가능 (POST /api/rules)
Phase 5: 운영               ✅ 가능 (알림/점수 모니터링)
```

---

## 7. 결론

### 7.1 운영 준비 상태
**✅ 신규 사이트 CEP/UEBA 구축 및 운영 가능**

- Kafka 구독을 통한 로그 수집 정상
- 마이그레이션(스키마 발견) API 정상
- 프론트 UI에서 정규화된 JSON 수신 → CEP Flink SQL / UEBA 점수 계산 가능
- 문서와 코드 일치 확인됨

### 7.2 발견된 문제

1. **CEP 규칙 DELETE API 누락**: `cmd/cep.go`에 DELETE 라우트 없음
   - UEBA는 있음: `e.DELETE("/api/rules/:id", ruleCtrl.Delete)`
   - CEP는 없음: PUT만 존재
   - **수정 필요**: `e.DELETE("/api/rules/:id", ruleCtrl.Delete)` 추가

2. **UEBA 미구현 기능**: 상황 가중치, 화이트리스트 (사용자 정보 연동 시 구현 가능)

3. **Flink Job 추적**: tracked=0으로 표시되나 running=50 확인됨 (추적 로직 점검 필요)

### 7.3 권장 조치

1. **CEP DELETE 라우트 추가** (cmd/cep.go):
   ```go
   e.DELETE("/api/rules/:id", ruleCtrl.Delete)
   ```

2. Flink Job 추적 로직 점검

3. 프론트엔드 규칙 빌더 UI 연동 테스트
