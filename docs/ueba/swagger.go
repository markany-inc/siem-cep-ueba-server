package ueba

import "github.com/swaggo/swag"

const uebaDocTemplate = `{
    "swagger": "2.0",
    "info": {
        "title": "SafePC UEBA API",
        "description": "User Entity Behavior Analytics - 사용자 행동 기반 위험 점수 분석\n\n## 점수 계산\nRiskScore = DecayedPrev + RuleScore + AnomalyScore\n- RuleScore: weight × ln(1 + count)\n- AnomalyScore: β × (Z-Score - threshold)",
        "version": "1.0.0"
    },
    "host": "203.229.154.49:48082",
    "basePath": "/api",
    "tags": [
        {"name": "Users", "description": "사용자 위험 점수"},
        {"name": "Rules", "description": "UEBA 규칙 관리"},
        {"name": "Logs", "description": "원본 로그 조회"},
        {"name": "Settings", "description": "시스템 설정"},
        {"name": "Status", "description": "서비스 상태"},
        {"name": "Field Meta", "description": "로그 필드 메타데이터"}
    ],
    "paths": {
        "/users": {
            "get": {
                "tags": ["Users"],
                "summary": "사용자 위험 점수 목록",
                "responses": {"200": {"description": "사용자 목록", "schema": {"$ref": "#/definitions/UserListResponse"}}}
            }
        },
        "/users/scores": {
            "get": {
                "tags": ["Users"],
                "summary": "점수 테이블 (DataTables 형식)",
                "responses": {"200": {"description": "점수 목록", "schema": {"$ref": "#/definitions/ScoresDataTable"}}}
            }
        },
        "/users/{id}": {
            "get": {
                "tags": ["Users"],
                "summary": "사용자 상세 정보",
                "parameters": [{"in": "path", "name": "id", "type": "string", "required": true}],
                "responses": {"200": {"description": "사용자 상세", "schema": {"$ref": "#/definitions/UserDetail"}}}
            }
        },
        "/users/{id}/history": {
            "get": {
                "tags": ["Users"],
                "summary": "일별 점수 이력",
                "parameters": [
                    {"in": "path", "name": "id", "type": "string", "required": true},
                    {"in": "query", "name": "days", "type": "integer", "default": 7}
                ],
                "responses": {"200": {"description": "점수 이력"}}
            }
        },
        "/users/{id}/hourly": {
            "get": {
                "tags": ["Users"],
                "summary": "시간별 이벤트 분포",
                "parameters": [{"in": "path", "name": "id", "type": "string", "required": true}],
                "responses": {"200": {"description": "시간별 분포"}}
            }
        },
        "/users/{id}/context": {
            "get": {
                "tags": ["Users"],
                "summary": "사용자 컨텍스트 조회",
                "parameters": [{"in": "path", "name": "id", "type": "string", "required": true}],
                "responses": {"200": {"description": "컨텍스트", "schema": {"$ref": "#/definitions/UserContext"}}}
            },
            "put": {
                "tags": ["Users"],
                "summary": "사용자 컨텍스트 설정",
                "description": "퇴사예정, 휴가 등 상황 설정 (점수 가중치 적용)",
                "parameters": [
                    {"in": "path", "name": "id", "type": "string", "required": true},
                    {"in": "body", "name": "body", "schema": {"$ref": "#/definitions/UserContextInput"}}
                ],
                "responses": {"200": {"description": "설정 완료"}}
            }
        },
        "/rules": {
            "get": {
                "tags": ["Rules"],
                "summary": "UEBA 규칙 목록",
                "responses": {"200": {"description": "규칙 목록"}}
            },
            "post": {
                "tags": ["Rules"],
                "summary": "UEBA 규칙 생성",
                "parameters": [{"in": "body", "name": "body", "required": true, "schema": {"$ref": "#/definitions/UEBARuleInput"}}],
                "responses": {"200": {"description": "생성 완료"}}
            }
        },
        "/rules/validate": {
            "post": {
                "tags": ["Rules"],
                "summary": "규칙 유효성 검증",
                "parameters": [{"in": "body", "name": "body", "required": true, "schema": {"$ref": "#/definitions/UEBARuleInput"}}],
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
                "parameters": [{"in": "path", "name": "id", "type": "string", "required": true}],
                "responses": {"200": {"description": "수정 완료"}}
            },
            "delete": {
                "tags": ["Rules"],
                "summary": "규칙 삭제",
                "parameters": [{"in": "path", "name": "id", "type": "string", "required": true}],
                "responses": {"200": {"description": "삭제 완료"}}
            }
        },
        "/logs": {
            "get": {
                "tags": ["Logs"],
                "summary": "로그 검색 (페이지네이션)",
                "parameters": [
                    {"in": "query", "name": "from", "type": "string"},
                    {"in": "query", "name": "to", "type": "string"},
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
                "summary": "규칙 생성용 메타",
                "responses": {"200": {"description": "메타 정보"}}
            }
        },
        "/health": {
            "get": {
                "tags": ["Status"],
                "summary": "헬스체크",
                "responses": {"200": {"description": "상태", "schema": {"$ref": "#/definitions/HealthResponse"}}}
            }
        },
        "/status": {
            "get": {
                "tags": ["Status"],
                "summary": "서비스 상태",
                "responses": {"200": {"description": "상태"}}
            }
        },
        "/config": {
            "get": {
                "tags": ["Settings"],
                "summary": "UEBA 설정 조회",
                "responses": {"200": {"description": "설정", "schema": {"$ref": "#/definitions/UEBAConfig"}}}
            }
        },
        "/settings": {
            "get": {"tags": ["Settings"], "summary": "전체 설정 조회", "responses": {"200": {"description": "설정"}}},
            "put": {"tags": ["Settings"], "summary": "설정 저장", "responses": {"200": {"description": "저장 완료"}}}
        },
        "/field-meta": {
            "get": {"tags": ["Field Meta"], "summary": "필드 메타 조회", "responses": {"200": {"description": "필드 메타"}}},
            "put": {"tags": ["Field Meta"], "summary": "필드 메타 저장", "responses": {"200": {"description": "저장 완료"}}}
        },
        "/field-meta/analyze": {
            "post": {"tags": ["Field Meta"], "summary": "이벤트별 필드 분석", "responses": {"200": {"description": "분석 결과"}}}
        }
    },
    "definitions": {
        "UserListResponse": {
            "type": "object",
            "example": {"users": [{"userId": "hslee", "riskScore": 315.73, "riskLevel": "HIGH", "ruleScore": 280.0, "anomalyScore": 35.73}]}
        },
        "ScoresDataTable": {
            "type": "object",
            "description": "DataTables 호환 형식",
            "example": {"data": [["hslee", 315.73, "HIGH", null], ["bklim", 7.25, "LOW", null]], "recordsTotal": 34, "recordsFiltered": 34, "draw": 0}
        },
        "UserDetail": {
            "type": "object",
            "example": {
                "userId": "hslee",
                "riskScore": 315.73,
                "riskLevel": "HIGH",
                "ruleScore": 280.0,
                "ruleScores": {"USB_BLOCK": 180.0, "CAPTURE_BLOCK": 100.0},
                "anomalyScore": 35.73,
                "eventCounts": {"MESSAGE_DEVICE_USAGE": 45, "MESSAGE_CAPTURE_BLOCK": 30},
                "coldStart": false,
                "daysSinceLast": 0
            }
        },
        "UserContext": {
            "type": "object",
            "example": {"userId": "hslee", "context": "normal", "effectiveContext": "normal", "startDate": "", "endDate": "", "note": "", "whitelisted": false}
        },
        "UserContextInput": {
            "type": "object",
            "properties": {
                "context": {"type": "string", "enum": ["normal", "departing", "vacation", "blacklist", "probation", "contractor", "vip"]},
                "startDate": {"type": "string", "format": "date"},
                "endDate": {"type": "string", "format": "date"},
                "note": {"type": "string"}
            },
            "example": {"context": "departing", "startDate": "2026-03-20", "endDate": "2026-04-30", "note": "4월말 퇴사 예정"}
        },
        "UEBARuleInput": {
            "type": "object",
            "description": "UEBA 규칙 생성/수정 요청",
            "required": ["name", "weight", "match"],
            "properties": {
                "name": {"type": "string"},
                "category": {"type": "string", "description": "카테고리 (매체제어, 정보유출 등)"},
                "weight": {"type": "number", "description": "점수 가중치 (1-100)"},
                "enabled": {"type": "boolean", "default": true},
                "match": {"$ref": "#/definitions/MatchCondition"},
                "aggregate": {"$ref": "#/definitions/UEBAAggregateCondition"},
                "ueba": {"type": "object", "properties": {"enabled": {"type": "boolean", "default": true}}}
            },
            "example": {
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
                "aggregate": {"type": "count"},
                "ueba": {"enabled": true}
            }
        },
        "MatchCondition": {
            "type": "object",
            "properties": {
                "msgId": {"type": "string"},
                "logic": {"type": "string", "enum": ["and", "or"], "default": "and"},
                "conditions": {"type": "array", "items": {"$ref": "#/definitions/Condition"}}
            }
        },
        "Condition": {
            "type": "object",
            "properties": {
                "field": {"type": "string"},
                "op": {"type": "string", "enum": ["eq", "neq", "gt", "gte", "lt", "lte", "in", "contains", "time_range"]},
                "value": {},
                "start": {"type": "integer"},
                "end": {"type": "integer"}
            },
            "example": {"field": "outcome", "op": "eq", "value": "blocked"}
        },
        "UEBAAggregateCondition": {
            "type": "object",
            "description": "집계 타입",
            "properties": {
                "type": {"type": "string", "enum": ["count", "sum", "cardinality"], "description": "count: 횟수, sum: 필드합산, cardinality: 고유값수"},
                "field": {"type": "string", "description": "sum/cardinality 시 대상 필드"}
            },
            "example": {"type": "count"}
        },
        "ValidateResponse": {
            "type": "object",
            "example": {"valid": true, "errors": []}
        },
        "LogSearchResponse": {
            "type": "object",
            "example": {"total": 15000, "size": 100, "offset": 0, "logs": []}
        },
        "LogAggRequest": {
            "type": "object",
            "example": {"from": "2026-03-18", "to": "2026-03-25", "groupBy": "msgId"}
        },
        "HealthResponse": {
            "type": "object",
            "example": {
                "status": "healthy",
                "uptime": "1h30m",
                "memory": {"alloc_mb": 45.2, "sys_mb": 120.5, "goroutines": 18},
                "data": {"users": 31, "baselines": 395, "rules": 10, "today_events": 11718}
            }
        },
        "UEBAConfig": {
            "type": "object",
            "description": "UEBA 설정",
            "example": {
                "anomaly": {
                    "z_threshold": 2.0,
                    "beta": 10,
                    "sigma_floor": 0.5,
                    "cold_start_min_days": 7,
                    "baseline_window_days": 30,
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
        }
    }
}`

var UEBASwaggerInfo = &swag.Spec{
	Version:          "1.0.0",
	Title:            "SafePC UEBA API",
	Description:      "User Entity Behavior Analytics API",
	BasePath:         "/api",
	InfoInstanceName: "ueba",
	SwaggerTemplate:  uebaDocTemplate,
}

func init() {
	swag.Register(UEBASwaggerInfo.InfoInstanceName, UEBASwaggerInfo)
}
