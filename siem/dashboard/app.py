#!/usr/bin/env python3
"""SafePC SIEM Dashboard with WebSocket"""
from fastapi import FastAPI, HTTPException, Request, WebSocket, WebSocketDisconnect
from fastapi.templating import Jinja2Templates
from fastapi.staticfiles import StaticFiles
from fastapi.responses import HTMLResponse
from pydantic import BaseModel
from typing import List, Optional
import httpx, asyncio, json
from datetime import datetime, timedelta

app = FastAPI(title="SafePC SIEM Dashboard")
templates = Jinja2Templates(directory="templates")

# UTC → KST 변환 필터
def utc_to_kst(utc_str):
    if not utc_str:
        return "-"
    try:
        dt = datetime.fromisoformat(utc_str.replace('Z', '+00:00'))
        # 이미 KST(+09:00)면 변환 안 함
        if '+09:00' not in utc_str and 'Z' in utc_str or '+00:00' in utc_str:
            dt = dt + timedelta(hours=9)
        return dt.strftime('%m-%d %H:%M:%S')
    except:
        return utc_str[5:19] if len(utc_str) > 19 else utc_str

templates.env.filters['kst'] = utc_to_kst
app.mount("/static", StaticFiles(directory="static"), name="static")

import os
OPENSEARCH_URL = os.getenv("OPENSEARCH_URL", "http://203.229.154.49:49200")
FLINK_URL = os.getenv("FLINK_URL", "http://203.229.154.49:48081")
CEP_URL = os.getenv("CEP_URL", "http://203.229.154.49:48084")
UEBA_URL = os.getenv("UEBA_URL", "http://203.229.154.49:48082")

# 인덱스명 (INDEX_PREFIX 기반)
_P = os.getenv("INDEX_PREFIX", "safepc")
IDX_ALERTS = f"{_P}-siem-cep-alerts-*"
IDX_SCORES = f"{_P}-siem-ueba-scores-*"
IDX_LOGS = f"{_P}-siem-event-logs-*"

# UEBA 설정 캐시
_ueba_config_cache = None
_ueba_config_time = None

async def get_ueba_config():
    global _ueba_config_cache, _ueba_config_time
    now = datetime.now()
    if _ueba_config_cache and _ueba_config_time and (now - _ueba_config_time).seconds < 60:
        return _ueba_config_cache
    try:
        async with httpx.AsyncClient() as client:
            r = await client.get(f"{UEBA_URL}/api/config", timeout=5)
            _ueba_config_cache = r.json()
            _ueba_config_time = now
            return _ueba_config_cache
    except:
        pass
    return {"tiers": {"green_max": 40, "yellow_max": 99, "red_max": 150}}

# WebSocket 연결 관리
class ConnectionManager:
    def __init__(self):
        self.active: List[WebSocket] = []
    
    async def connect(self, ws: WebSocket):
        await ws.accept()
        self.active.append(ws)
    
    def disconnect(self, ws: WebSocket):
        self.active.remove(ws)
    
    async def broadcast(self, data: dict):
        for ws in self.active:
            try:
                await ws.send_json(data)
            except:
                pass

manager = ConnectionManager()

# Push API 모델
class AlertPush(BaseModel):
    ruleId: str
    ruleName: str
    severity: str
    description: str
    userId: str
    hostname: Optional[str] = None
    userIp: Optional[str] = None

class UebaPush(BaseModel):
    userId: str
    riskScore: int
    riskLevel: str
    prevScore: Optional[int] = None

# WebSocket 엔드포인트
@app.websocket("/ws")
async def websocket_endpoint(ws: WebSocket):
    await manager.connect(ws)
    try:
        while True:
            await ws.receive_text()
    except WebSocketDisconnect:
        manager.disconnect(ws)

# Flink/UEBA에서 호출하는 Push API
@app.post("/api/alert/push")
async def push_alert(alert: AlertPush):
    data = {"type": "alert", "data": alert.dict(), "timestamp": datetime.now().isoformat()}
    await manager.broadcast(data)
    return {"status": "ok", "clients": len(manager.active)}

@app.post("/api/ueba/push")
async def push_ueba(ueba: UebaPush):
    data = {"type": "ueba", "data": ueba.dict(), "timestamp": datetime.now().isoformat()}
    await manager.broadcast(data)
    return {"status": "ok", "clients": len(manager.active)}

# OpenSearch 쿼리 헬퍼
async def get_yesterday_scores():
    """어제 최종 점수를 사용자별로 조회 → {userId: riskScore}"""
    res = await es_query(IDX_SCORES, {
        "size": 0,
        "query": {"range": {"@timestamp": {"gte": "now-1d/d", "lt": "now/d", "time_zone": "Asia/Seoul"}}},
        "aggs": {"byUser": {"terms": {"field": "userId.keyword", "size": 500}, "aggs": {
            "last": {"top_hits": {"size": 1, "sort": [{"@timestamp": "desc"}], "_source": ["riskScore"]}}
        }}}
    })
    result = {}
    for b in res.get("aggregations", {}).get("byUser", {}).get("buckets", []):
        hits = b.get("last", {}).get("hits", {}).get("hits", [])
        if hits:
            result[b["key"]] = hits[0]["_source"].get("riskScore", 0) or 0
    return result

async def es_query(index, body):
    async with httpx.AsyncClient() as client:
        r = await client.post(f"{OPENSEARCH_URL}/{index}/_search", json=body, timeout=10)
        return r.json()

@app.get("/", response_class=HTMLResponse)
async def dashboard(request: Request):
    alerts = await es_query(IDX_ALERTS, {
        "size": 0,
        "query": {"range": {"@timestamp": {"gte": "now/d", "time_zone": "Asia/Seoul"}}},
        "aggs": {
            "bySeverity": {"terms": {"field": "severity.keyword"}},
            "byRule": {"terms": {"field": "ruleName.keyword", "size": 10}}
        }
    })
    # 사용자별 오늘 기록으로 레벨별 카운트
    ueba = await es_query(IDX_SCORES, {
        "size": 0,
        "query": {"range": {"@timestamp": {"gte": "now/d", "time_zone": "Asia/Seoul"}}},
        "aggs": {
            "byUser": {
                "terms": {"field": "userId.keyword", "size": 500, "order": {"maxScore": "desc"}},
                "aggs": {
                    "maxScore": {"max": {"field": "riskScore"}},
                    "recent": {"top_hits": {"size": 1, "sort": [{"@timestamp": "desc"}], "_source": ["userId", "riskScore", "riskLevel", "prevScore", "status", "@timestamp"]}}
                }
            }
        }
    })
    # 사용자별 최신 riskLevel로 카운트
    level_counts = {"CRITICAL": 0, "HIGH": 0, "MEDIUM": 0, "LOW": 0}
    for bucket in ueba.get("aggregations", {}).get("byUser", {}).get("buckets", []):
        hits = bucket.get("recent", {}).get("hits", {}).get("hits", [])
        if hits:
            level = hits[0]["_source"].get("riskLevel", "LOW")
            level_counts[level] = level_counts.get(level, 0) + 1
    recent_alerts = await es_query(IDX_ALERTS, {
        "size": 10, "sort": [{"@timestamp": "desc"}],
        "_source": ["ruleId", "ruleName", "severity", "userId", "userName", "hostname", "@timestamp", "description"]
    })
    logs = await es_query(IDX_LOGS, {
        "size": 0,
        "track_total_hits": True,
        "query": {"range": {"@timestamp": {"gte": "now/d", "time_zone": "Asia/Seoul"}}},
        "aggs": {"byType": {"terms": {"field": "msgId.keyword", "size": 20}}}
    })
    # top 10 사용자 + 점수 변화 계산
    yesterday = await get_yesterday_scores()
    top_users = []
    ueba_last_update = ""
    for bucket in ueba.get("aggregations", {}).get("byUser", {}).get("buckets", []):
        hits = bucket.get("recent", {}).get("hits", {}).get("hits", [])
        if hits:
            current = hits[0]["_source"]
            if not ueba_last_update and current.get("@timestamp"):
                ueba_last_update = utc_to_kst(current["@timestamp"])
            # 어제 최종 점수와 비교
            prev_score = yesterday.get(current.get("userId", ""), current["riskScore"])
            current["scoreDiff"] = current["riskScore"] - prev_score
            top_users.append(current)
    # 점수 내림차순 정렬 후 상위 10명
    top_users = sorted(top_users, key=lambda x: x.get("riskScore", 0), reverse=True)[:10]
    config = await get_ueba_config()
    return templates.TemplateResponse("index.html", {
        "request": request, "alerts": alerts, "ueba": ueba,
        "recent_alerts": recent_alerts.get("hits", {}).get("hits", []),
        "top_users": top_users, "level_counts": level_counts,
        "logs": logs, "now": datetime.now().strftime("%Y-%m-%d %H:%M:%S"),
        "ueba_last_update": f"갱신: {ueba_last_update}" if ueba_last_update else "",
        "tiers": config.get("tiers", {})
    })

@app.get("/users", response_class=HTMLResponse)
async def users(request: Request, level: str = None):
    # 사용자별 최근 2개 기록 조회
    body = {
        "size": 0,
        "aggs": {
            "byUser": {
                "terms": {"field": "userId.keyword", "size": 200},
                "aggs": {
                    "recent": {
                        "top_hits": {
                            "size": 1, "sort": [{"@timestamp": "desc"}],
                            "_source": ["userId", "userName", "riskScore", "riskLevel", "prevScore", "status", "eventValues", "ruleScores", "@timestamp"]
                        }
                    }
                }
            }
        }
    }
    result = await es_query(IDX_SCORES, body)
    yesterday = await get_yesterday_scores()
    users_list = []
    for bucket in result.get("aggregations", {}).get("byUser", {}).get("buckets", []):
        hits = bucket["recent"]["hits"]["hits"]
        if hits:
            user = hits[0]
            curr = user["_source"]["riskScore"]
            prev = yesterday.get(user["_source"].get("userId", ""), curr)
            user["_source"]["scoreDiff"] = curr - prev
            if level:
                if level == "HIGH" and user["_source"]["riskLevel"] not in ["HIGH"]:
                    continue
                elif level != "HIGH" and user["_source"]["riskLevel"] != level:
                    continue
            users_list.append(user)
    users_list.sort(key=lambda x: x["_source"]["riskScore"], reverse=True)
    config = await get_ueba_config()
    return templates.TemplateResponse("users.html", {"request": request, "users": users_list, "level": level, "tiers": config.get("tiers", {})})

@app.get("/user/{user_id}", response_class=HTMLResponse)
async def user_detail(request: Request, user_id: str):
    today_filter = {"range": {"@timestamp": {"gte": "now/d", "time_zone": "Asia/Seoul"}}}
    ueba = await es_query(IDX_SCORES, {
        "size": 10, "sort": [{"@timestamp": "desc"}],
        "query": {"bool": {"must": [{"term": {"userId": user_id}}, today_filter]}}
    })
    alerts = await es_query(IDX_ALERTS, {
        "size": 50, "sort": [{"@timestamp": "desc"}],
        "query": {"bool": {"must": [{"term": {"userId": user_id}}, today_filter]}}
    })
    logs = await es_query(IDX_LOGS, {
        "size": 50, "sort": [{"@timestamp": "desc"}],
        "query": {"bool": {"should": [{"term": {"userId": user_id}}, {"term": {"cefExtensions.suid": user_id}}], "minimum_should_match": 1, "filter": [today_filter]}}
    })
    # 오늘 전체 규칙 위반 집계
    today_agg = await es_query(IDX_SCORES, {
        "size": 0,
        "query": {"bool": {"must": [{"term": {"userId": user_id}}, {"range": {"@timestamp": {"gte": "now/d", "time_zone": "Asia/Seoul"}}}]}},
        "aggs": {
            "max_rule": {"max": {"field": "ruleScore"}},
            "max_anomaly": {"max": {"field": "anomalyScore"}},
            "max_decay": {"max": {"field": "decayedPrev"}}
        }
    })
    today_violations = await es_query(IDX_SCORES, {
        "size": 100, "_source": ["ruleScores"],
        "query": {"bool": {"must": [{"term": {"userId": user_id}}, {"range": {"@timestamp": {"gte": "now/d", "time_zone": "Asia/Seoul"}}}, {"exists": {"field": "ruleScores"}}]}}
    })
    # 오늘 발생한 모든 규칙 위반 합산
    today_rule_scores = {}
    for h in today_violations.get("hits", {}).get("hits", []):
        for rule, score in h["_source"].get("ruleScores", {}).items():
            if rule not in today_rule_scores or score > today_rule_scores[rule]:
                today_rule_scores[rule] = score
    aggs = today_agg.get("aggregations", {})
    today_summary = {
        "ruleScore": aggs.get("max_rule", {}).get("value", 0) or 0,
        "anomalyScore": aggs.get("max_anomaly", {}).get("value", 0) or 0,
        "decayedPrev": aggs.get("max_decay", {}).get("value", 0) or 0,
        "ruleViolations": list(today_rule_scores.keys()),
        "ruleScores": today_rule_scores
    }
    # 이전 대비 행동지표 변화 계산 (eventValues 기반)
    feature_changes = []
    hits = ueba.get("hits", {}).get("hits", [])
    if len(hits) >= 2:
        curr = hits[0]["_source"].get("eventValues", {}) or {}
        prev = hits[1]["_source"].get("eventValues", {}) or {}
        all_keys = set(list(curr.keys()) + list(prev.keys()))
        for k in sorted(all_keys):
            cv = curr.get(k, 0) or 0
            pv = prev.get(k, 0) or 0
            diff = cv - pv
            if diff != 0:
                feature_changes.append({"name": k, "diff": diff})
    config = await get_ueba_config()
    # 어제 최종 점수 조회 (전일대비 정확한 계산용)
    yesterday_score = await es_query(IDX_SCORES, {
        "size": 1, "sort": [{"@timestamp": "desc"}],
        "query": {"bool": {"must": [
            {"term": {"userId": user_id}},
            {"range": {"@timestamp": {"gte": "now-1d/d", "lt": "now/d", "time_zone": "Asia/Seoul"}}}
        ]}},
        "_source": ["riskScore"]
    })
    prev_day_score = 0
    yh = yesterday_score.get("hits", {}).get("hits", [])
    if yh:
        prev_day_score = yh[0]["_source"].get("riskScore", 0) or 0
    return templates.TemplateResponse("user_detail.html", {
        "request": request, "user_id": user_id,
        "ueba": hits, "alerts": alerts.get("hits", {}).get("hits", []),
        "logs": logs.get("hits", {}).get("hits", []),
        "feature_changes": feature_changes,
        "tiers": config.get("tiers", {}),
        "today": today_summary,
        "prev_day_score": prev_day_score
    })

@app.get("/api/user/{user_id}/hourly")
async def user_hourly(user_id: str):
    """UEBA 서버에 위임"""
    async with httpx.AsyncClient() as client:
        r = await client.get(f"{UEBA_URL}/api/users/{user_id}/hourly", timeout=10)
        return r.json()

@app.get("/api/user/{user_id}/history")
async def user_history(user_id: str):
    """UEBA 서버에 위임"""
    async with httpx.AsyncClient() as client:
        r = await client.get(f"{UEBA_URL}/api/users/{user_id}/history", timeout=10)
        return r.json()

# 서버 사이드 페이징 API
@app.get("/api/alerts")
async def api_alerts(draw: int = 1, start: int = 0, length: int = 10, search: str = "", order_col: int = 0, order_dir: str = "desc", rule: str = "", severity: str = ""):
    """CEP 알림 조회 — Go CEP 서비스에 위임"""
    try:
        async with httpx.AsyncClient() as client:
            r = await client.get(f"{CEP_URL}/api/alerts", params={
                "draw": draw, "start": start, "length": length,
                "search": search, "order_col": order_col, "order_dir": order_dir,
                "rule": rule, "severity": severity
            }, timeout=10)
            return r.json()
    except Exception as e:
        return {"draw": draw, "recordsTotal": 0, "recordsFiltered": 0, "data": []}

@app.get("/api/logs")
async def api_logs(draw: int = 1, start: int = 0, length: int = 15, search: str = "", order_col: int = 0, order_dir: str = "desc", msgId: str = "", outcome: str = ""):
    cols = ["@timestamp", "cefExtensions.suid", "msgId", "cefExtensions.shost", "cefExtensions.src", "cefExtensions.outcome"]
    sort_field = cols[order_col] if order_col < len(cols) else "@timestamp"
    query = {"bool": {"must": [{"range": {"@timestamp": {"gte": "now-1d"}}}]}}
    if search:
        query["bool"]["must"].append({"multi_match": {"query": search, "fields": ["msgId", "cefExtensions.suid", "cefExtensions.shost", "userId", "hostname"]}})
    if msgId:
        query["bool"]["must"].append({"term": {"msgId": msgId}})
    if outcome:
        query["bool"]["should"] = [{"term": {"outcome": outcome}}, {"term": {"cefExtensions.outcome": outcome}}]
        query["bool"]["minimum_should_match"] = 1
    total = await es_query(IDX_LOGS, {"size": 0, "query": query, "track_total_hits": True})
    total_count = min(total.get("hits", {}).get("total", {}).get("value", 0), 50000)
    if start >= 50000:
        return {"draw": draw, "recordsTotal": total_count, "recordsFiltered": total_count, "data": []}
    result = await es_query(IDX_LOGS, {"from": start, "size": length, "sort": [{sort_field: order_dir}], "query": query})
    hits = result.get("hits", {}).get("hits", [])
    def ext(s, f):
        """Extract field from top-level or cefExtensions"""
        return s.get(f, "") or s.get("cefExtensions", {}).get(f, "")
    return {
        "draw": draw,
        "recordsTotal": total_count,
        "recordsFiltered": total_count,
        "data": [[
            h["_source"].get("@timestamp", ""),
            h["_source"].get("msgId", "").replace("MESSAGE_", ""),
            ext(h["_source"], "userId") or ext(h["_source"], "suid"),
            ext(h["_source"], "hostname") or ext(h["_source"], "shost"),
            ext(h["_source"], "userIp") or ext(h["_source"], "src"),
            ext(h["_source"], "action") or ext(h["_source"], "act"),
            ext(h["_source"], "outcome"),
        ] for h in hits]
    }

@app.get("/api/users")
async def api_users(draw: int = 1, start: int = 0, length: int = 10, search: str = "", order_col: int = 0, order_dir: str = "desc"):
    """UEBA 서버에 위임"""
    async with httpx.AsyncClient() as client:
        r = await client.get(f"{UEBA_URL}/api/users/scores", params={
            "draw": draw, "start": start, "length": length,
            "search": search, "order_col": order_col, "order_dir": order_dir
        }, timeout=10)
        return r.json()

@app.get("/api/rules")
@app.get("/api/cep/rules")
async def api_rules():
    """CEP 규칙 목록 — Go CEP 서비스에 위임"""
    try:
        async with httpx.AsyncClient() as client:
            r = await client.get(f"{CEP_URL}/api/rules", timeout=10)
            return r.json()
    except Exception as e:
        print(f"CEP rules error: {e}")
        return {"rules": [], "cep_rules": [], "ueba_features": []}

@app.get("/rules", response_class=HTMLResponse)
async def rules_page(request: Request):
    return templates.TemplateResponse("rules.html", {"request": request})

@app.get("/alerts", response_class=HTMLResponse)
async def alerts(request: Request, severity: str = None):
    return templates.TemplateResponse("alerts.html", {"request": request, "severity": severity})

@app.get("/logs", response_class=HTMLResponse)
async def logs_page(request: Request, msgId: str = None):
    return templates.TemplateResponse("logs.html", {"request": request, "msgId": msgId})

@app.get("/settings", response_class=HTMLResponse)
async def settings_page(request: Request):
    return templates.TemplateResponse("settings.html", {"request": request})

# safepc-ueba-rules API (CEP/UEBA 공통 규칙)

@app.post("/api/flink/explain")
async def flink_explain(request: Request):
    """Flink SQL 검증 — Go CEP 서비스에 위임"""
    data = await request.json()
    try:
        async with httpx.AsyncClient() as client:
            r = await client.post(f"{CEP_URL}/api/explain", json=data, timeout=10)
            return r.json()
    except Exception as e:
        return {"valid": False, "error": str(e)}

@app.get("/api/safepc-ueba-rules")
async def get_siem_rules():
    """UEBA 룰은 UEBA 서버에서, CEP 룰은 CEP 서버에서"""
    try:
        async with httpx.AsyncClient() as client:
            # UEBA 규칙
            r1 = await client.get(f"{UEBA_URL}/api/rules", timeout=10)
            ueba_rules = r1.json().get("rules", [])
            
            # CEP 규칙 — Go CEP 서비스에서
            r2 = await client.get(f"{CEP_URL}/api/rules", timeout=10)
            cep_rules = r2.json().get("rules", []) or []
            
            # 합치기 (id 기준 중복 제거)
            merged = {r.get("id", r.get("name", "")): r for r in ueba_rules}
            for r in cep_rules:
                rid = r.get("id", "")
                if rid and rid not in merged:
                    merged[rid] = r
            return list(merged.values())
    except:
        return []

# ── 필드 메타데이터 (마이그레이션) — Go CEP 서비스에 위임 ──

@app.get("/api/cep/field-meta")
async def get_field_meta():
    async with httpx.AsyncClient() as client:
        r = await client.get(f"{CEP_URL}/api/field-meta", timeout=10)
        return r.json()

@app.put("/api/cep/field-meta")
async def put_field_meta(request: Request):
    data = await request.json()
    async with httpx.AsyncClient() as client:
        r = await client.put(f"{CEP_URL}/api/field-meta", json=data, timeout=10)
        return r.json()

@app.post("/api/cep/field-meta/analyze")
async def field_meta_analyze(request: Request):
    data = await request.json()
    async with httpx.AsyncClient() as client:
        r = await client.post(f"{CEP_URL}/api/field-meta/analyze", json=data, timeout=30)
        return r.json()

@app.post("/api/cep/field-meta/analyze-field")
async def field_meta_analyze_field(request: Request):
    data = await request.json()
    async with httpx.AsyncClient() as client:
        r = await client.post(f"{CEP_URL}/api/field-meta/analyze-field", json=data, timeout=30)
        return r.json()

# ── CEP SQL 생성/제출/규칙 CRUD는 Go CEP 서비스(siem-cep:8084)가 담당 ──

@app.post("/api/cep/reload")
async def cep_reload():
    """CEP 전체 재로드 — Go CEP 서비스에 위임"""
    try:
        async with httpx.AsyncClient(timeout=60) as client:
            r = await client.post(f"{CEP_URL}/api/reload")
            return r.json()
    except Exception as e:
        return {"status": "error", "message": str(e)}

@app.put("/api/safepc-ueba-rules/{rule_id}")
@app.put("/api/cep/rules/{rule_id}")
async def update_siem_rule(rule_id: str, request: Request):
    """규칙 수정 — UEBA/CEP 각각 위임"""
    data = await request.json()
    import logging
    logging.info(f"[RULE PUT] {rule_id}: cep={data.get('cep')}, keys={list(data.keys())}")
    result = {}
    try:
        async with httpx.AsyncClient() as client:
            # UEBA 규칙이면 → UEBA 서버에 저장 + reload
            if data.get("ueba", {}).get("enabled"):
                r = await client.put(f"{UEBA_URL}/api/rules/{rule_id}", json=data, timeout=10)
                result["ueba"] = r.json()
            # CEP 규칙이면 → CEP 서비스에 위임
            if data.get("cep", {}).get("enabled"):
                r = await client.put(f"{CEP_URL}/api/rules/{rule_id}", json=data, timeout=30)
                result["cep"] = r.json()
            return {"status": "ok", "result": result}
    except Exception as e:
        return {"status": "error", "message": str(e)}

@app.delete("/api/safepc-ueba-rules/{rule_id}")
@app.delete("/api/cep/rules/{rule_id}")
async def delete_siem_rule(rule_id: str):
    """규칙 삭제 — UEBA/CEP 양쪽에 위임"""
    result = {}
    try:
        async with httpx.AsyncClient() as client:
            for name, url in [("ueba", UEBA_URL), ("cep", CEP_URL)]:
                try:
                    r = await client.delete(f"{url}/api/rules/{rule_id}", timeout=10)
                    result[name] = r.json()
                except:
                    pass
            return {"status": "ok", "result": result}
    except Exception as e:
        return {"status": "error", "message": str(e)}

@app.post("/api/safepc-ueba-rules")
@app.post("/api/cep/rules")
async def create_siem_rule(request: Request):
    """규칙 생성 — UEBA/CEP 각각 위임"""
    data = await request.json()
    result = {}
    try:
        async with httpx.AsyncClient() as client:
            # UEBA 규칙이면 → UEBA 서버에 생성 + reload
            if data.get("ueba", {}).get("enabled"):
                r = await client.post(f"{UEBA_URL}/api/rules", json=data, timeout=10)
                result["ueba"] = r.json()
            # CEP 규칙이면 → CEP 서비스에 위임
            if data.get("cep", {}).get("enabled"):
                r = await client.post(f"{CEP_URL}/api/rules", json=data, timeout=30)
                result["cep"] = r.json()
            return {"status": "ok", "result": result}
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

@app.get("/api/ueba/settings")
async def get_ueba_settings():
    """UEBA 서버에 위임"""
    async with httpx.AsyncClient() as client:
        r = await client.get(f"{UEBA_URL}/api/settings", timeout=10)
        return r.json()

@app.post("/api/ueba/settings")
async def save_ueba_settings(request: Request):
    """UEBA 서버에 위임"""
    data = await request.json()
    async with httpx.AsyncClient() as client:
        r = await client.post(f"{UEBA_URL}/api/settings", json=data, timeout=10)
        return r.json()

@app.get("/api/cep/status")
async def cep_status():
    """CEP 상태 — Go CEP 서비스에 위임 (Flink Job 상세 포함)"""
    try:
        async with httpx.AsyncClient() as client:
            r = await client.get(f"{CEP_URL}/api/status", timeout=10)
            return r.json()
    except Exception as e:
        return {"error": str(e)}

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8501)
