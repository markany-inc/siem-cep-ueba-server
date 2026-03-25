package ueba

import "github.com/swaggo/swag"

const uebaDocTemplate = `{
    "swagger": "2.0",
    "info": {
        "title": "SafePC UEBA API",
        "description": "User Entity Behavior Analytics - 사용자 행동 기반 위험 점수 분석",
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
                "responses": {
                    "200": {"description": "사용자 목록", "schema": {"$ref": "#/definitions/UserListResponse"}}
                }
            }
        },
        "/users/scores": {
            "get": {
                "tags": ["Users"],
                "summary": "점수 테이블 (DataTables 형식)",
                "description": "DataTables 호환 형식으로 사용자 점수 반환",
                "responses": {
                    "200": {"description": "점수 목록", "schema": {"$ref": "#/definitions/ScoresDataTable"}}
                }
            }
        },
        "/users/{id}": {
            "get": {
                "tags": ["Users"],
                "summary": "사용자 상세 정보",
                "parameters": [{"in": "path", "name": "id", "type": "string", "required": true}],
                "responses": {
                    "200": {"description": "사용자 상세", "schema": {"$ref": "#/definitions/UserDetail"}}
                }
            }
        },
        "/users/{id}/history": {
            "get": {
                "tags": ["Users"],
                "summary": "점수 이력",
                "parameters": [
                    {"in": "path", "name": "id", "type": "string", "required": true},
                    {"in": "query", "name": "days", "type": "integer", "default": 7}
                ],
                "responses": {"200": {"description": "점수 이력"}}
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
                    {"in": "query", "name": "from", "type": "string", "description": "시작일 YYYY-MM-DD"},
                    {"in": "query", "name": "to", "type": "string", "description": "종료일"},
                    {"in": "query", "name": "userId", "type": "string"},
                    {"in": "query", "name": "msgId", "type": "string"},
                    {"in": "query", "name": "hostname", "type": "string"},
                    {"in": "query", "name": "size", "type": "integer", "default": 100, "description": "페이지 크기 (최대 1000)"},
                    {"in": "query", "name": "offset", "type": "integer", "default": 0, "description": "시작 위치 (최대 10000)"}
                ],
                "responses": {"200": {"description": "로그 목록", "schema": {"$ref": "#/definitions/LogSearchResponse"}}}
            }
        },
        "/logs/aggregate": {
            "post": {
                "tags": ["Logs"],
                "summary": "로그 집계",
                "parameters": [{"in": "body", "name": "body", "schema": {"$ref": "#/definitions/LogAggRequest"}}],
                "responses": {"200": {"description": "집계 결과", "schema": {"$ref": "#/definitions/LogAggResponse"}}}
            }
        },
        "/logs/migration-meta": {
            "get": {
                "tags": ["Logs"],
                "summary": "규칙 생성용 메타",
                "responses": {"200": {"description": "메타 정보", "schema": {"$ref": "#/definitions/MigrationMeta"}}}
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
        "/field-meta": {
            "get": {"tags": ["Field Meta"], "summary": "필드 메타 조회", "responses": {"200": {"description": "필드 메타"}}},
            "put": {"tags": ["Field Meta"], "summary": "필드 메타 저장", "responses": {"200": {"description": "저장 완료"}}}
        }
    },
    "definitions": {
        "UserListResponse": {
            "type": "object",
            "example": {"users": [{"userId": "hslee", "riskScore": 12.5, "riskLevel": "MEDIUM", "ruleScore": 8.0, "anomalyScore": 4.5}]}
        },
        "ScoresDataTable": {
            "type": "object",
            "description": "DataTables 호환 형식",
            "example": {"data": [["hslee", 315.73, "HIGH", null], ["bklim", 7.25, "LOW", null]], "recordsTotal": 34, "recordsFiltered": 34, "draw": 0}
        },
        "UserDetail": {
            "type": "object",
            "example": {"userId": "hslee", "riskScore": 315.73, "riskLevel": "HIGH", "ruleScore": 280.0, "ruleScores": {"CAPTURE_BLOCK": 180.0}, "anomalyScore": 35.73, "eventCounts": {"MESSAGE_CAPTURE_BLOCK": 30}}
        },
        "UserContext": {
            "type": "object",
            "example": {"userId": "hslee", "context": "normal", "effectiveContext": "normal", "startDate": "", "endDate": "", "note": "", "whitelisted": false}
        },
        "UserContextInput": {
            "type": "object",
            "example": {"context": "departing", "startDate": "2026-03-20", "endDate": "2026-04-30", "note": "4월말 퇴사"},
            "properties": {
                "context": {"type": "string", "enum": ["normal", "departing", "vacation", "blacklist", "probation", "contractor", "vip"]},
                "startDate": {"type": "string"},
                "endDate": {"type": "string"},
                "note": {"type": "string"}
            }
        },
        "UEBARuleInput": {
            "type": "object",
            "required": ["name", "weight", "match"],
            "example": {"name": "화면 캡처 차단", "weight": 6, "enabled": true, "match": {"msgId": "MESSAGE_CAPTURE_BLOCK"}, "ueba": {"enabled": true}}
        },
        "ValidateResponse": {
            "type": "object",
            "example": {"valid": true, "errors": []}
        },
        "LogSearchResponse": {
            "type": "object",
            "example": {"total": 15000, "size": 100, "offset": 0, "logs": [{"@timestamp": "2026-03-25T10:00:00", "msgId": "MESSAGE_DEVICE_USAGE", "userId": "hslee"}]}
        },
        "LogAggRequest": {
            "type": "object",
            "example": {"from": "2026-03-18", "to": "2026-03-25", "groupBy": "msgId"}
        },
        "LogAggResponse": {
            "type": "object",
            "example": {"groupBy": "msgId", "items": [{"key": "MESSAGE_DEVICE_USAGE", "count": 1523}]}
        },
        "MigrationMeta": {
            "type": "object",
            "example": {"events": {}, "operators": ["eq", "neq", "gt", "gte", "lt", "lte", "in", "like", "regex", "exists"], "logicOps": ["and", "or"], "aggregates": {"functions": ["count", "sum", "avg"], "windows": ["1m", "5m", "1h"]}, "ruleTemplate": {"name": "", "match": {"msgId": "", "logic": "and", "conditions": []}}}
        },
        "HealthResponse": {
            "type": "object",
            "example": {"status": "healthy", "uptime": "1h30m", "memory": {"alloc_mb": 45.2, "sys_mb": 120.5}, "data": {"users": 31, "baselines": 395, "rules": 10, "today_events": 11718}}
        },
        "UEBAConfig": {
            "type": "object",
            "example": {"anomaly": {"z_threshold": 2.0, "beta": 0.3, "cold_start_min_days": 6, "baseline_window_days": 30}, "decay": {"lambda": 0.1}, "tiers": {"green_max": 10, "yellow_max": 30}, "multipliers": {"departing": 1.5, "blacklist": 2.0}}
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
