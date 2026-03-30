package cep

import "github.com/swaggo/swag"

const cepDocTemplate = `{
    "swagger": "2.0",
    "info": {
        "title": "SafePC CEP API",
        "description": "Complex Event Processing - Flink 기반 실시간 규칙 탐지 시스템\n\n## 주요 기능\n- 실시간 이벤트 스트림 처리 (Kafka → Flink)\n- 규칙 기반 패턴 매칭 (단일, 집계, 순차)\n- Flink SQL 자동 생성 및 Job 관리",
        "version": "1.0.0"
    },
    "host": "203.229.154.49:48084",
    "basePath": "/api",
    "tags": [
        {"name": "Logs", "description": "원본 로그 조회"},
        {"name": "Rules", "description": "CEP 탐지 규칙 관리"},
        {"name": "Jobs", "description": "Flink Job 관리"},
        {"name": "Alerts", "description": "탐지된 알림 조회"},
        {"name": "Field Meta", "description": "로그 필드 메타데이터"}
    ],
    "paths": {
        "/logs": {
            "get": {
                "tags": ["Logs"],
                "summary": "로그 검색 (페이지네이션)",
                "parameters": [
                    {"in": "query", "name": "from", "type": "string", "description": "시작일 (YYYY-MM-DD)"},
                    {"in": "query", "name": "to", "type": "string", "description": "종료일"},
                    {"in": "query", "name": "userId", "type": "string"},
                    {"in": "query", "name": "msgId", "type": "string"},
                    {"in": "query", "name": "hostname", "type": "string"},
                    {"in": "query", "name": "size", "type": "integer", "default": 100},
                    {"in": "query", "name": "offset", "type": "integer", "default": 0}
                ],
                "responses": {"200": {"description": "로그 목록", "schema": {"$ref": "#/definitions/LogSearchResponse"}}}
            }
        },
        "/logs/aggregate": {
            "post": {
                "tags": ["Logs"],
                "summary": "로그 집계",
                "parameters": [{"in": "body", "name": "body", "schema": {"$ref": "#/definitions/LogAggRequest"}}],
                "responses": {"200": {"description": "집계 결과"}}
            }
        },
        "/logs/migration-meta": {
            "get": {
                "tags": ["Logs"],
                "summary": "규칙 생성용 메타 정보",
                "description": "프론트엔드 규칙 빌더 UI용 필드/연산자/템플릿",
                "responses": {"200": {"description": "메타 정보", "schema": {"$ref": "#/definitions/MigrationMeta"}}}
            }
        },
        "/rules": {
            "get": {
                "tags": ["Rules"],
                "summary": "CEP 규칙 목록",
                "responses": {"200": {"description": "규칙 목록"}}
            },
            "post": {
                "tags": ["Rules"],
                "summary": "CEP 규칙 생성",
                "description": "규칙 생성 후 자동으로 Flink Job 제출 (enabled=true인 경우)",
                "parameters": [{"in": "body", "name": "body", "required": true, "schema": {"$ref": "#/definitions/CEPRuleInput"}}],
                "responses": {
                    "200": {"description": "생성 완료", "schema": {"$ref": "#/definitions/RuleCreateResponse"}},
                    "400": {"description": "유효하지 않은 규칙"}
                }
            }
        },
        "/rules/validate": {
            "post": {
                "tags": ["Rules"],
                "summary": "규칙 유효성 검증",
                "description": "규칙 JSON 검증 및 생성될 Flink SQL 미리보기",
                "parameters": [{"in": "body", "name": "body", "required": true, "schema": {"$ref": "#/definitions/CEPRuleInput"}}],
                "responses": {
                    "200": {"description": "유효", "schema": {"$ref": "#/definitions/ValidateResponse"}},
                    "400": {"description": "유효하지 않음"}
                }
            }
        },
        "/rules/{id}": {
            "put": {
                "tags": ["Rules"],
                "summary": "규칙 수정",
                "parameters": [
                    {"in": "path", "name": "id", "type": "string", "required": true},
                    {"in": "body", "name": "body", "schema": {"$ref": "#/definitions/CEPRuleInput"}}
                ],
                "responses": {"200": {"description": "수정 완료"}}
            },
            "delete": {
                "tags": ["Rules"],
                "summary": "규칙 삭제",
                "parameters": [{"in": "path", "name": "id", "type": "string", "required": true}],
                "responses": {"200": {"description": "삭제 완료"}}
            }
        },
        "/build-sql": {
            "post": {
                "tags": ["Rules"],
                "summary": "규칙 → Flink SQL 변환",
                "parameters": [{"in": "body", "name": "body", "schema": {"$ref": "#/definitions/CEPRuleInput"}}],
                "responses": {"200": {"description": "SQL", "schema": {"type": "object", "properties": {"sql": {"type": "string"}}}}}
            }
        },
        "/submit": {
            "post": {
                "tags": ["Jobs"],
                "summary": "Flink Job 제출",
                "parameters": [{"in": "body", "name": "body", "schema": {"type": "object", "properties": {"ruleId": {"type": "string"}}}}],
                "responses": {"200": {"description": "제출 성공"}}
            }
        },
        "/reload": {
            "post": {
                "tags": ["Jobs"],
                "summary": "전체 규칙 재로드",
                "responses": {"200": {"description": "재로드 완료"}}
            }
        },
        "/status": {
            "get": {
                "tags": ["Jobs"],
                "summary": "Flink Job 상태",
                "responses": {"200": {"description": "Job 상태"}}
            }
        },
        "/alerts": {
            "get": {
                "tags": ["Alerts"],
                "summary": "CEP 알림 목록",
                "parameters": [
                    {"in": "query", "name": "from", "type": "string"},
                    {"in": "query", "name": "to", "type": "string"},
                    {"in": "query", "name": "ruleId", "type": "string"},
                    {"in": "query", "name": "severity", "type": "string"},
                    {"in": "query", "name": "size", "type": "integer", "default": 100}
                ],
                "responses": {"200": {"description": "알림 목록"}}
            }
        },
        "/field-meta": {
            "get": {"tags": ["Field Meta"], "summary": "필드 메타 조회", "responses": {"200": {"description": "필드 메타", "schema": {"$ref": "#/definitions/FieldMeta"}}}},
            "put": {"tags": ["Field Meta"], "summary": "필드 메타 저장", "responses": {"200": {"description": "저장 완료"}}}
        },
        "/field-meta/analyze": {
            "post": {
                "tags": ["Field Meta"],
                "summary": "이벤트별 필드 분석",
                "description": "N일치 로그에서 msgId별 필드 자동 추출",
                "parameters": [{"in": "body", "name": "body", "schema": {"type": "object", "properties": {"days": {"type": "integer", "default": 7}}}}],
                "responses": {"200": {"description": "분석 결과"}}
            }
        },
        "/field-meta/analyze-field": {
            "post": {
                "tags": ["Field Meta"],
                "summary": "특정 필드 값 목록 수집",
                "parameters": [{"in": "body", "name": "body", "schema": {"type": "object", "properties": {"event": {"type": "string"}, "field": {"type": "string"}}}}],
                "responses": {"200": {"description": "필드 값 목록"}}
            }
        }
    },
    "definitions": {
        "LogSearchResponse": {
            "type": "object",
            "example": {"total": 15000, "size": 100, "offset": 0, "logs": [{"@timestamp": "2026-03-25T10:00:00", "msgId": "MESSAGE_DEVICE_USAGE", "hostname": "PC-001", "cefExtensions": {"suid": "kim", "outcome": "blocked"}}]}
        },
        "LogAggRequest": {
            "type": "object",
            "example": {"from": "2026-03-18", "to": "2026-03-25", "groupBy": "msgId"}
        },
        "MigrationMeta": {
            "type": "object",
            "description": "프론트엔드 규칙 빌더용 메타 정보",
            "example": {
                "events": {"MESSAGE_DEVICE_USAGE": {"fields": ["outcome", "act", "fname", "fsize"]}},
                "operators": ["eq", "neq", "gt", "gte", "lt", "lte", "in", "like", "regex", "time_range"],
                "logicOps": ["and", "or"],
                "aggregates": {"functions": ["count", "sum", "avg"], "windows": ["30s", "1m", "5m", "10m", "30m", "1h"]},
                "ruleTemplate": {"name": "", "severity": "medium", "enabled": true, "match": {"msgId": "", "conditions": []}}
            }
        },
        "CEPRuleInput": {
            "type": "object",
            "description": "CEP 규칙 생성/수정 요청"
        },
        "CEPRule_단일패턴": {
            "type": "object",
            "description": "단일 패턴: 조건 매칭 시 즉시 탐지",
            "example": {"name": "USB_차단_탐지", "description": "USB 매체 차단 발생 시 즉시 알림", "severity": "medium", "enabled": true, "match": {"msgId": "MESSAGE_DEVICE_USAGE", "conditions": [{"field": "outcome", "op": "eq", "value": "blocked"}]}}
        },
        "CEPRule_집계패턴": {
            "type": "object",
            "description": "집계 패턴: 시간 윈도우 내 N회 이상 발생 시 탐지",
            "example": {"name": "로그인_실패_반복", "description": "10분 내 로그인 실패 5회 이상", "severity": "high", "enabled": true, "match": {"msgId": "MESSAGE_AGENT_AUTHENTICATION", "conditions": [{"field": "outcome", "op": "eq", "value": "failure"}]}, "aggregate": {"function": "count", "min": 5, "within": "10m"}, "group_by": ["suser"]}
        },
        "CEPRule_순차패턴": {
            "type": "object",
            "description": "순차 패턴: A → B 순서로 발생 시 탐지",
            "example": {"name": "로그인_실패후_성공", "description": "로그인 실패 후 5분 내 성공", "severity": "medium", "enabled": true, "patterns": [{"id": "fail", "order": 1, "match": {"msgId": "MESSAGE_AGENT_AUTHENTICATION", "conditions": [{"field": "outcome", "op": "eq", "value": "failure"}]}}, {"id": "success", "order": 2, "match": {"msgId": "MESSAGE_AGENT_AUTHENTICATION", "conditions": [{"field": "outcome", "op": "eq", "value": "success"}]}}], "aggregate": {"within": "5m"}, "group_by": ["suser"]}
        },
        "CEPRule_분기패턴": {
            "type": "object",
            "description": "분기 패턴: A → (B or C) → D 순서로 발생 시 탐지",
            "example": {"name": "로그인후_우회시도", "description": "로그인 실패 후 USB/프린터 차단, 이후 파일 반출", "severity": "high", "enabled": true, "patterns": [{"id": "login_fail", "order": 1, "match": {"msgId": "MESSAGE_LOGIN_FAIL"}}, {"id": "usb_block", "order": 2, "match": {"msgId": "MESSAGE_USB_BLOCK"}}, {"id": "print_block", "order": 2, "match": {"msgId": "MESSAGE_PRINT_BLOCK"}}, {"id": "file_export", "order": 3, "match": {"msgId": "MESSAGE_FILE_EXPORT"}}], "paths": [["login_fail", "usb_block", "file_export"], ["login_fail", "print_block", "file_export"]], "aggregate": {"within": "10m"}, "group_by": ["suser"]}
        },
        "MatchCondition": {
            "type": "object",
            "properties": {
                "msgId": {"type": "string", "description": "이벤트 타입"},
                "logic": {"type": "string", "enum": ["and", "or"], "default": "and"},
                "conditions": {"type": "array", "items": {"$ref": "#/definitions/Condition"}}
            }
        },
        "Condition": {
            "type": "object",
            "description": "필드 조건",
            "properties": {
                "field": {"type": "string", "description": "cefExtensions 내 필드명"},
                "op": {"type": "string", "enum": ["eq", "neq", "gt", "gte", "lt", "lte", "in", "like", "regex", "time_range"]},
                "value": {"description": "비교값 (문자열, 숫자, 배열)"},
                "start": {"type": "integer", "description": "time_range 시작 시간"},
                "end": {"type": "integer", "description": "time_range 종료 시간"}
            },
            "example": {"field": "outcome", "op": "eq", "value": "blocked"}
        },
        "AggregateCondition": {
            "type": "object",
            "description": "집계 조건",
            "properties": {
                "function": {"type": "string", "enum": ["count", "sum", "count_distinct"], "description": "집계 함수"},
                "field": {"type": "string", "description": "집계 대상 필드 (sum, count_distinct 시)"},
                "min": {"type": "integer", "description": "최소 임계값 (이상)"},
                "within": {"type": "string", "description": "시간 윈도우 (예: 10m, 1h)"}
            },
            "example": {"function": "count", "min": 5, "within": "10m"}
        },
        "PatternItem": {
            "type": "object",
            "description": "순차/분기 패턴 항목",
            "properties": {
                "id": {"type": "string", "description": "패턴 ID (paths에서 참조용)"},
                "order": {"type": "integer", "description": "순서 (같은 order면 병렬 분기)"},
                "match": {"$ref": "#/definitions/MatchCondition"},
                "quantifier": {"type": "object", "properties": {"min": {"type": "integer"}, "max": {"type": "integer"}}, "description": "반복 횟수"}
            },
            "example": {"id": "P1", "order": 1, "match": {"msgId": "MESSAGE_LOGIN_FAIL", "conditions": [{"field": "outcome", "op": "eq", "value": "failure"}]}}
        },
        "BranchPatternExample": {
            "type": "object",
            "description": "분기 패턴 예제: 1→(2 or 3)→4",
            "example": {
                "name": "분기 패턴",
                "within": "10m",
                "patterns": [
                    {"id": "P1", "order": 1, "match": {"msgId": "MESSAGE_LOGIN_FAIL"}},
                    {"id": "P2", "order": 2, "match": {"msgId": "MESSAGE_USB_BLOCK"}},
                    {"id": "P3", "order": 2, "match": {"msgId": "MESSAGE_PRINT_BLOCK"}},
                    {"id": "P4", "order": 3, "match": {"msgId": "MESSAGE_FILE_EXPORT"}}
                ],
                "paths": [["P1", "P2", "P4"], ["P1", "P3", "P4"]]
            }
        },
        "RuleCreateResponse": {
            "type": "object",
            "example": {"status": "ok", "ruleId": "rule-1711234567"}
        },
        "ValidateResponse": {
            "type": "object",
            "example": {"valid": true, "sql": "SELECT userId, 1 as cnt FROM events WHERE msgId = 'MESSAGE_DEVICE_USAGE' AND cefExtensions['outcome'] = 'blocked'"}
        },
        "FieldMeta": {
            "type": "object",
            "description": "필드 메타데이터 (프론트엔드 규칙 빌더 UI용)",
            "example": {
                "migratedAt": "2026-02-25T10:00:00Z",
                "events": {
                    "MESSAGE_DEVICE_USAGE": {
                        "enabled": true,
                        "label": "매체 사용",
                        "fields": {
                            "outcome": {"inputType": "select", "label": "결과", "options": [{"value": "allowed", "label": "허용"}, {"value": "blocked", "label": "차단"}]},
                            "fname": {"inputType": "input", "label": "파일명"},
                            "fsize": {"inputType": "number", "label": "파일크기(bytes)"}
                        }
                    }
                }
            }
        }
    }
}`

var CEPSwaggerInfo = &swag.Spec{
	Version:          "1.0.0",
	Title:            "SafePC CEP API",
	Description:      "Complex Event Processing API",
	BasePath:         "/api",
	InfoInstanceName: "cep",
	SwaggerTemplate:  cepDocTemplate,
}

func init() {
	swag.Register(CEPSwaggerInfo.InfoInstanceName, CEPSwaggerInfo)
}
