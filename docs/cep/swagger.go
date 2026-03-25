package cep

import "github.com/swaggo/swag"

const cepDocTemplate = `{
    "swagger": "2.0",
    "info": {
        "title": "SafePC CEP API",
        "description": "Complex Event Processing - Flink 기반 실시간 규칙 탐지 시스템\n\n## 주요 기능\n- 실시간 이벤트 스트림 처리 (Kafka → Flink)\n- 규칙 기반 탐지 및 알림 생성\n- Flink SQL 자동 생성 및 Job 관리",
        "version": "1.0.0",
        "contact": {"name": "SafePC SIEM Team"}
    },
    "host": "203.229.154.49:48084",
    "basePath": "/api",
    "tags": [
        {"name": "Logs", "description": "원본 로그 조회 및 분석"},
        {"name": "Rules", "description": "CEP 탐지 규칙 관리"},
        {"name": "Jobs", "description": "Flink Job 제출/관리"},
        {"name": "Alerts", "description": "탐지된 알림 조회"},
        {"name": "Field Meta", "description": "로그 필드 메타데이터"}
    ],
    "paths": {
        "/logs": {
            "get": {
                "tags": ["Logs"],
                "summary": "로그 검색 (페이지네이션)",
                "description": "날짜 범위, 사용자, 이벤트 타입으로 원본 로그 검색. 인덱스는 날짜별 샤딩됨.",
                "parameters": [
                    {"in": "query", "name": "from", "type": "string", "description": "시작일 (YYYY-MM-DD, 기본: 7일 전)"},
                    {"in": "query", "name": "to", "type": "string", "description": "종료일 (YYYY-MM-DD, 기본: 오늘)"},
                    {"in": "query", "name": "userId", "type": "string", "description": "사용자 ID 필터"},
                    {"in": "query", "name": "msgId", "type": "string", "description": "이벤트 타입 필터"},
                    {"in": "query", "name": "hostname", "type": "string", "description": "호스트명 필터"},
                    {"in": "query", "name": "size", "type": "integer", "description": "페이지 크기 (기본 100, 최대 1000)", "default": 100},
                    {"in": "query", "name": "offset", "type": "integer", "description": "시작 위치 (기본 0, 최대 10000)", "default": 0}
                ],
                "responses": {
                    "200": {
                        "description": "로그 목록",
                        "schema": {"$ref": "#/definitions/LogSearchResponse"}
                    }
                }
            }
        },
        "/logs/aggregate": {
            "post": {
                "tags": ["Logs"],
                "summary": "로그 집계",
                "description": "이벤트 타입별, 사용자별, 호스트별 로그 카운트 집계",
                "parameters": [{"in": "body", "name": "body", "required": true, "schema": {"$ref": "#/definitions/LogAggregateRequest"}}],
                "responses": {
                    "200": {
                        "description": "집계 결과",
                        "schema": {"$ref": "#/definitions/LogAggregateResponse"}
                    }
                }
            }
        },
        "/logs/migration-meta": {
            "get": {
                "tags": ["Logs"],
                "summary": "규칙 생성용 메타 정보",
                "description": "프론트엔드에서 규칙 JSON 생성 시 필요한 필드 목록, 연산자, 집계 함수, 규칙 템플릿 제공",
                "responses": {
                    "200": {
                        "description": "메타 정보",
                        "schema": {"$ref": "#/definitions/MigrationMeta"}
                    }
                }
            }
        },
        "/rules": {
            "get": {
                "tags": ["Rules"],
                "summary": "CEP 규칙 목록",
                "description": "CEP가 활성화된 모든 규칙 조회 (cep.enabled=true)",
                "responses": {
                    "200": {
                        "description": "규칙 목록",
                        "schema": {"$ref": "#/definitions/RuleListResponse"}
                    }
                }
            },
            "post": {
                "tags": ["Rules"],
                "summary": "CEP 규칙 생성",
                "description": "새 규칙 생성 후 자동으로 Flink Job 제출 (enabled=true인 경우)",
                "parameters": [{"in": "body", "name": "body", "required": true, "schema": {"$ref": "#/definitions/RuleInput"}}],
                "responses": {
                    "200": {
                        "description": "생성 완료",
                        "schema": {"$ref": "#/definitions/RuleCreateResponse"}
                    },
                    "400": {"description": "유효하지 않은 규칙", "schema": {"$ref": "#/definitions/ErrorResponse"}}
                }
            }
        },
        "/rules/validate": {
            "post": {
                "tags": ["Rules"],
                "summary": "규칙 유효성 검증",
                "description": "규칙 JSON 검증 및 생성될 Flink SQL 미리보기. 저장하지 않음.",
                "parameters": [{"in": "body", "name": "body", "required": true, "schema": {"$ref": "#/definitions/RuleInput"}}],
                "responses": {
                    "200": {
                        "description": "유효한 규칙",
                        "schema": {"$ref": "#/definitions/ValidateResponse"}
                    },
                    "400": {"description": "유효하지 않은 규칙", "schema": {"$ref": "#/definitions/ValidateErrorResponse"}}
                }
            }
        },
        "/rules/{id}": {
            "put": {
                "tags": ["Rules"],
                "summary": "규칙 수정",
                "description": "규칙 수정 후 Flink Job 재제출 (enabled=true) 또는 취소 (enabled=false)",
                "parameters": [
                    {"in": "path", "name": "id", "type": "string", "required": true, "description": "규칙 ID"},
                    {"in": "body", "name": "body", "required": true, "schema": {"$ref": "#/definitions/RuleInput"}}
                ],
                "responses": {"200": {"description": "수정 완료"}}
            },
            "delete": {
                "tags": ["Rules"],
                "summary": "규칙 삭제",
                "description": "규칙 삭제 및 관련 Flink Job 취소",
                "parameters": [{"in": "path", "name": "id", "type": "string", "required": true}],
                "responses": {"200": {"description": "삭제 완료"}}
            }
        },
        "/build-sql": {
            "post": {
                "tags": ["Rules"],
                "summary": "규칙 → Flink SQL 변환",
                "description": "규칙 JSON을 Flink SQL로 변환만 수행 (저장/제출 없음)",
                "parameters": [{"in": "body", "name": "body", "required": true, "schema": {"$ref": "#/definitions/RuleInput"}}],
                "responses": {
                    "200": {
                        "description": "생성된 SQL",
                        "schema": {"type": "object", "properties": {"sql": {"type": "string", "example": "INSERT INTO alerts SELECT ... FROM events WHERE msgId = 'MESSAGE_DEVICE_USAGE' AND fsize > 10485760"}}}
                    }
                }
            }
        },
        "/submit": {
            "post": {
                "tags": ["Jobs"],
                "summary": "Flink Job 제출",
                "description": "특정 규칙을 Flink Job으로 제출. 이미 실행 중이면 재시작.",
                "parameters": [{"in": "body", "name": "body", "schema": {"$ref": "#/definitions/SubmitRequest"}}],
                "responses": {
                    "200": {
                        "description": "제출 성공",
                        "schema": {"$ref": "#/definitions/SubmitResponse"}
                    }
                }
            }
        },
        "/reload": {
            "post": {
                "tags": ["Jobs"],
                "summary": "전체 규칙 재로드",
                "description": "모든 활성 규칙(enabled=true)을 Flink Job으로 재제출. 기존 Job은 취소 후 재시작.",
                "responses": {
                    "200": {
                        "description": "재로드 완료",
                        "schema": {"$ref": "#/definitions/ReloadResponse"}
                    }
                }
            }
        },
        "/status": {
            "get": {
                "tags": ["Jobs"],
                "summary": "Flink Job 상태",
                "description": "현재 실행 중인 모든 Flink Job 상태 조회",
                "responses": {
                    "200": {
                        "description": "Job 상태 목록",
                        "schema": {"$ref": "#/definitions/JobStatusResponse"}
                    }
                }
            }
        },
        "/alerts": {
            "get": {
                "tags": ["Alerts"],
                "summary": "CEP 알림 목록",
                "description": "CEP 규칙에 의해 탐지된 알림 조회",
                "parameters": [
                    {"in": "query", "name": "from", "type": "string", "description": "시작 시간 (ISO8601)", "example": "2026-03-25T00:00:00"},
                    {"in": "query", "name": "to", "type": "string", "description": "종료 시간"},
                    {"in": "query", "name": "ruleId", "type": "string", "description": "규칙 ID 필터"},
                    {"in": "query", "name": "userId", "type": "string", "description": "사용자 ID 필터"},
                    {"in": "query", "name": "severity", "type": "string", "description": "심각도 필터", "enum": ["LOW", "MEDIUM", "HIGH", "CRITICAL"]},
                    {"in": "query", "name": "size", "type": "integer", "description": "조회 개수 (기본 100)", "default": 100}
                ],
                "responses": {
                    "200": {
                        "description": "알림 목록",
                        "schema": {"$ref": "#/definitions/AlertListResponse"}
                    }
                }
            }
        },
        "/field-meta": {
            "get": {
                "tags": ["Field Meta"],
                "summary": "필드 메타데이터 조회",
                "description": "이벤트 타입별 사용 가능한 필드 목록 및 타입 정보",
                "responses": {"200": {"description": "필드 메타", "schema": {"$ref": "#/definitions/FieldMeta"}}}
            },
            "put": {
                "tags": ["Field Meta"],
                "summary": "필드 메타데이터 저장",
                "description": "분석된 필드 메타데이터 수동 저장",
                "parameters": [{"in": "body", "name": "body", "schema": {"$ref": "#/definitions/FieldMeta"}}],
                "responses": {"200": {"description": "저장 완료"}}
            }
        },
        "/field-meta/analyze": {
            "post": {
                "tags": ["Field Meta"],
                "summary": "이벤트별 필드 분석",
                "description": "실제 로그 데이터에서 이벤트 타입별 필드 목록 동적 추출",
                "parameters": [{"in": "body", "name": "body", "schema": {"$ref": "#/definitions/AnalyzeRequest"}}],
                "responses": {"200": {"description": "분석 결과"}}
            }
        },
        "/field-meta/analyze-field": {
            "post": {
                "tags": ["Field Meta"],
                "summary": "특정 필드 상세 분석",
                "description": "특정 필드의 값 분포, 통계 정보 조회",
                "parameters": [{"in": "body", "name": "body", "schema": {"$ref": "#/definitions/AnalyzeFieldRequest"}}],
                "responses": {"200": {"description": "필드 상세"}}
            }
        }
    },
    "definitions": {
        "LogSearchResponse": {
            "type": "object",
            "example": {"total": 15000, "size": 100, "offset": 0, "logs": [{"@timestamp": "2026-03-25T10:30:00+09:00", "msgId": "MESSAGE_DEVICE_USAGE", "userId": "hslee", "hostname": "PC-001", "fsize": 15728640}]},
            "properties": {
                "total": {"type": "integer", "description": "전체 결과 수"},
                "size": {"type": "integer", "description": "요청한 페이지 크기"},
                "offset": {"type": "integer", "description": "시작 위치"},
                "logs": {"type": "array", "items": {"type": "object"}}
            }
        },
        "LogAggregateRequest": {
            "type": "object",
            "example": {"from": "2026-03-18", "to": "2026-03-25", "groupBy": "msgId", "userId": "hslee"},
            "properties": {
                "from": {"type": "string", "description": "시작일"},
                "to": {"type": "string", "description": "종료일"},
                "groupBy": {"type": "string", "enum": ["msgId", "userId", "hostname"], "description": "그룹 기준"},
                "userId": {"type": "string", "description": "사용자 필터"},
                "msgId": {"type": "string", "description": "이벤트 타입 필터"}
            }
        },
        "LogAggregateResponse": {
            "type": "object",
            "example": {"groupBy": "msgId", "items": [{"key": "MESSAGE_DEVICE_USAGE", "count": 1523}, {"key": "MESSAGE_PRINT", "count": 892}]},
            "properties": {
                "groupBy": {"type": "string"},
                "items": {"type": "array", "items": {"type": "object", "properties": {"key": {"type": "string"}, "count": {"type": "integer"}}}}
            }
        },
        "MigrationMeta": {
            "type": "object",
            "example": {
                "events": {"MESSAGE_DEVICE_USAGE": ["userId", "hostname", "fsize", "fname", "deviceType"], "MESSAGE_PRINT": ["userId", "hostname", "printCount", "printerName"]},
                "operators": ["eq", "neq", "gt", "gte", "lt", "lte", "in", "like", "regex", "exists"],
                "logicOps": ["and", "or"],
                "aggregates": {"functions": ["count", "sum", "avg", "min", "max", "count_distinct"], "windows": ["1m", "5m", "10m", "30m", "1h", "6h", "1d"]},
                "ruleTemplate": {"name": "", "enabled": true, "match": {"msgId": "", "logic": "and", "conditions": []}, "within": "5m", "cep": {"enabled": true, "severity": "MEDIUM"}}
            }
        },
        "RuleListResponse": {
            "type": "object",
            "properties": {
                "rules": {"type": "array", "items": {"$ref": "#/definitions/Rule"}}
            }
        },
        "Rule": {
            "type": "object",
            "example": {"id": "rule-1711234567", "name": "대용량 USB 복사", "enabled": true, "match": {"msgId": "MESSAGE_DEVICE_USAGE", "logic": "and", "conditions": [{"field": "fsize", "op": "gt", "value": "10485760"}, {"field": "deviceType", "op": "eq", "value": "USB"}]}, "within": "5m", "cep": {"enabled": true, "severity": "HIGH"}, "sql": "INSERT INTO alerts ..."},
            "properties": {
                "id": {"type": "string"},
                "name": {"type": "string"},
                "enabled": {"type": "boolean"},
                "match": {"$ref": "#/definitions/RuleMatch"},
                "aggregate": {"$ref": "#/definitions/RuleAggregate"},
                "within": {"type": "string"},
                "cep": {"type": "object", "properties": {"enabled": {"type": "boolean"}, "severity": {"type": "string"}}},
                "sql": {"type": "string"}
            }
        },
        "RuleInput": {
            "type": "object",
            "required": ["name", "match"],
            "example": {"name": "대용량 USB 복사", "enabled": true, "match": {"msgId": "MESSAGE_DEVICE_USAGE", "logic": "and", "conditions": [{"field": "fsize", "op": "gt", "value": "10485760"}, {"field": "deviceType", "op": "eq", "value": "USB"}]}, "within": "5m", "cep": {"enabled": true, "severity": "HIGH"}},
            "properties": {
                "name": {"type": "string", "description": "규칙 이름"},
                "enabled": {"type": "boolean", "default": true, "description": "활성화 여부"},
                "match": {"$ref": "#/definitions/RuleMatch"},
                "aggregate": {"$ref": "#/definitions/RuleAggregate"},
                "within": {"type": "string", "description": "시간 윈도우 (예: 5m, 1h)"},
                "cep": {"type": "object", "properties": {"enabled": {"type": "boolean"}, "severity": {"type": "string", "enum": ["LOW", "MEDIUM", "HIGH", "CRITICAL"]}}}
            }
        },
        "RuleMatch": {
            "type": "object",
            "description": "매칭 조건",
            "properties": {
                "msgId": {"type": "string", "description": "이벤트 타입 (예: MESSAGE_DEVICE_USAGE)"},
                "logic": {"type": "string", "enum": ["and", "or"], "description": "조건 결합 방식"},
                "conditions": {"type": "array", "items": {"$ref": "#/definitions/Condition"}}
            }
        },
        "Condition": {
            "type": "object",
            "description": "개별 필터 조건",
            "example": {"field": "fsize", "op": "gt", "value": "10485760"},
            "properties": {
                "field": {"type": "string", "description": "필드명"},
                "op": {"type": "string", "enum": ["eq", "neq", "gt", "gte", "lt", "lte", "in", "like", "regex", "exists"], "description": "연산자"},
                "value": {"type": "string", "description": "비교값 (in의 경우 콤마 구분)"}
            }
        },
        "RuleAggregate": {
            "type": "object",
            "description": "집계 조건 (선택)",
            "example": {"function": "count", "field": "*", "groupBy": ["userId"], "having": {"op": "gte", "value": 5}},
            "properties": {
                "function": {"type": "string", "enum": ["count", "sum", "avg", "min", "max", "count_distinct"]},
                "field": {"type": "string", "description": "집계 대상 필드 (* = 전체)"},
                "groupBy": {"type": "array", "items": {"type": "string"}, "description": "그룹 기준 필드"},
                "having": {"type": "object", "properties": {"op": {"type": "string"}, "value": {"type": "number"}}}
            }
        },
        "RuleCreateResponse": {
            "type": "object",
            "example": {"status": "ok", "ruleId": "rule-1711234567"},
            "properties": {
                "status": {"type": "string"},
                "ruleId": {"type": "string"}
            }
        },
        "ValidateResponse": {
            "type": "object",
            "example": {"valid": true, "sql": "INSERT INTO alerts SELECT 'rule-123' as ruleId, '대용량 USB 복사' as ruleName, 'HIGH' as severity, userId, hostname, COUNT(*) as cnt FROM events WHERE msgId = 'MESSAGE_DEVICE_USAGE' AND fsize > 10485760 GROUP BY userId, hostname HAVING COUNT(*) >= 1"},
            "properties": {
                "valid": {"type": "boolean"},
                "sql": {"type": "string"}
            }
        },
        "ValidateErrorResponse": {
            "type": "object",
            "example": {"valid": false, "errors": ["name 필드 필수", "match.msgId 또는 match.conditions 필요"]},
            "properties": {
                "valid": {"type": "boolean"},
                "errors": {"type": "array", "items": {"type": "string"}}
            }
        },
        "SubmitRequest": {
            "type": "object",
            "example": {"ruleId": "rule-1711234567"},
            "properties": {
                "ruleId": {"type": "string", "description": "제출할 규칙 ID"}
            }
        },
        "SubmitResponse": {
            "type": "object",
            "example": {"status": "ok", "jobId": "abc123def456"},
            "properties": {
                "status": {"type": "string"},
                "jobId": {"type": "string", "description": "Flink Job ID"}
            }
        },
        "ReloadResponse": {
            "type": "object",
            "example": {"status": "ok", "submitted": 5, "failed": 0},
            "properties": {
                "status": {"type": "string"},
                "submitted": {"type": "integer", "description": "제출된 규칙 수"},
                "failed": {"type": "integer", "description": "실패한 규칙 수"}
            }
        },
        "JobStatusResponse": {
            "type": "object",
            "example": {"jobs": [{"ruleId": "rule-1711234567", "jobId": "abc123", "status": "RUNNING", "startTime": "2026-03-25T09:00:00Z"}]},
            "properties": {
                "jobs": {"type": "array", "items": {"type": "object", "properties": {"ruleId": {"type": "string"}, "jobId": {"type": "string"}, "status": {"type": "string"}, "startTime": {"type": "string"}}}}
            }
        },
        "AlertListResponse": {
            "type": "object",
            "example": {"total": 25, "alerts": [{"ruleId": "rule-1711234567", "ruleName": "대용량 USB 복사", "severity": "HIGH", "userId": "hslee", "hostname": "PC-001", "cnt": 3, "ts": "2026-03-25T10:30:00+09:00"}]},
            "properties": {
                "total": {"type": "integer"},
                "alerts": {"type": "array", "items": {"$ref": "#/definitions/Alert"}}
            }
        },
        "Alert": {
            "type": "object",
            "properties": {
                "ruleId": {"type": "string"},
                "ruleName": {"type": "string"},
                "severity": {"type": "string"},
                "userId": {"type": "string"},
                "hostname": {"type": "string"},
                "cnt": {"type": "integer", "description": "발생 횟수"},
                "ts": {"type": "string", "description": "탐지 시간"}
            }
        },
        "FieldMeta": {
            "type": "object",
            "example": {"events": {"MESSAGE_DEVICE_USAGE": {"fields": [{"name": "userId", "type": "keyword"}, {"name": "fsize", "type": "long"}, {"name": "fname", "type": "text"}]}}},
            "properties": {
                "events": {"type": "object"}
            }
        },
        "AnalyzeRequest": {
            "type": "object",
            "example": {"events": ["MESSAGE_DEVICE_USAGE", "MESSAGE_PRINT"], "days": 7},
            "properties": {
                "events": {"type": "array", "items": {"type": "string"}, "description": "분석할 이벤트 타입"},
                "days": {"type": "integer", "description": "분석 기간 (일)", "default": 7}
            }
        },
        "AnalyzeFieldRequest": {
            "type": "object",
            "example": {"event": "MESSAGE_DEVICE_USAGE", "field": "deviceType"},
            "properties": {
                "event": {"type": "string", "description": "이벤트 타입"},
                "field": {"type": "string", "description": "분석할 필드명"}
            }
        },
        "ErrorResponse": {
            "type": "object",
            "example": {"error": "SQL 생성 실패"},
            "properties": {
                "error": {"type": "string"}
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
