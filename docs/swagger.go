package docs

import "github.com/swaggo/swag"

const docTemplate = `{
    "swagger": "2.0",
    "info": {
        "title": "SafePC SIEM API",
        "description": "CEP (Complex Event Processing) + UEBA (User Entity Behavior Analytics) API",
        "version": "1.0.0"
    },
    "basePath": "/api",
    "paths": {
        "/rules": {
            "get": {
                "tags": ["Rules"],
                "summary": "규칙 목록 조회",
                "responses": {"200": {"description": "규칙 목록"}}
            },
            "post": {
                "tags": ["Rules"],
                "summary": "규칙 생성",
                "parameters": [{"in": "body", "name": "body", "schema": {"$ref": "#/definitions/Rule"}}],
                "responses": {"200": {"description": "생성된 규칙"}}
            }
        },
        "/rules/{id}": {
            "put": {
                "tags": ["Rules"],
                "summary": "규칙 수정",
                "parameters": [{"in": "path", "name": "id", "type": "string", "required": true}],
                "responses": {"200": {"description": "수정된 규칙"}}
            },
            "delete": {
                "tags": ["Rules"],
                "summary": "규칙 삭제",
                "parameters": [{"in": "path", "name": "id", "type": "string", "required": true}],
                "responses": {"200": {"description": "삭제 완료"}}
            }
        },
        "/users": {
            "get": {
                "tags": ["UEBA Users"],
                "summary": "사용자 위험 점수 목록",
                "responses": {"200": {"description": "사용자 목록"}}
            }
        },
        "/users/{id}": {
            "get": {
                "tags": ["UEBA Users"],
                "summary": "사용자 상세 정보",
                "parameters": [{"in": "path", "name": "id", "type": "string", "required": true}],
                "responses": {"200": {"description": "사용자 상세"}}
            }
        },
        "/users/{id}/history": {
            "get": {
                "tags": ["UEBA Users"],
                "summary": "사용자 점수 이력",
                "parameters": [{"in": "path", "name": "id", "type": "string", "required": true}],
                "responses": {"200": {"description": "점수 이력"}}
            }
        },
        "/users/{id}/context": {
            "get": {
                "tags": ["UEBA Users"],
                "summary": "사용자 컨텍스트 조회",
                "parameters": [{"in": "path", "name": "id", "type": "string", "required": true}],
                "responses": {"200": {"description": "컨텍스트 정보"}}
            },
            "put": {
                "tags": ["UEBA Users"],
                "summary": "사용자 컨텍스트 설정",
                "parameters": [{"in": "path", "name": "id", "type": "string", "required": true}],
                "responses": {"200": {"description": "설정 완료"}}
            }
        },
        "/alerts": {
            "get": {
                "tags": ["CEP Alerts"],
                "summary": "알림 목록 조회",
                "responses": {"200": {"description": "알림 목록"}}
            }
        },
        "/field-meta": {
            "get": {
                "tags": ["Field Meta"],
                "summary": "필드 메타데이터 조회",
                "responses": {"200": {"description": "필드 메타"}}
            },
            "put": {
                "tags": ["Field Meta"],
                "summary": "필드 메타데이터 저장",
                "responses": {"200": {"description": "저장 완료"}}
            }
        },
        "/field-meta/analyze": {
            "post": {
                "tags": ["Field Meta"],
                "summary": "이벤트별 필드 분석",
                "responses": {"200": {"description": "분석 결과"}}
            }
        },
        "/health": {
            "get": {
                "tags": ["Status"],
                "summary": "헬스체크",
                "responses": {"200": {"description": "서비스 상태"}}
            }
        },
        "/status": {
            "get": {
                "tags": ["Status"],
                "summary": "서비스 상태",
                "responses": {"200": {"description": "상태 정보"}}
            }
        },
        "/config": {
            "get": {
                "tags": ["Status"],
                "summary": "설정 조회",
                "responses": {"200": {"description": "설정 정보"}}
            }
        },
        "/settings": {
            "get": {
                "tags": ["Settings"],
                "summary": "UEBA 설정 조회",
                "responses": {"200": {"description": "설정"}}
            },
            "post": {
                "tags": ["Settings"],
                "summary": "UEBA 설정 저장",
                "responses": {"200": {"description": "저장 완료"}}
            }
        },
        "/submit": {
            "post": {
                "tags": ["CEP Jobs"],
                "summary": "Flink Job 제출",
                "responses": {"200": {"description": "제출 결과"}}
            }
        },
        "/reload": {
            "post": {
                "tags": ["CEP Jobs"],
                "summary": "전체 규칙 재로드",
                "responses": {"200": {"description": "재로드 완료"}}
            }
        }
    },
    "definitions": {
        "Rule": {
            "type": "object",
            "properties": {
                "id": {"type": "string"},
                "name": {"type": "string"},
                "enabled": {"type": "boolean"},
                "weight": {"type": "number"},
                "match": {"type": "object"},
                "aggregate": {"type": "object"}
            }
        }
    }
}`

var SwaggerInfo = &swag.Spec{
	Version:          "1.0.0",
	Title:            "SafePC SIEM API",
	Description:      "CEP + UEBA API",
	BasePath:         "/api",
	InfoInstanceName: "swagger",
	SwaggerTemplate:  docTemplate,
}

func init() {
	swag.Register(SwaggerInfo.InfoInstanceName, SwaggerInfo)
}
