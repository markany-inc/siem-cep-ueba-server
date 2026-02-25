package com.markany.siem;

import org.apache.flink.api.common.eventtime.WatermarkStrategy;
import org.apache.flink.api.common.state.*;
import org.apache.flink.api.common.typeinfo.Types;
import org.apache.flink.connector.kafka.source.KafkaSource;
import org.apache.flink.connector.kafka.source.enumerator.initializer.OffsetsInitializer;
import org.apache.flink.streaming.api.datastream.*;
import org.apache.flink.streaming.api.environment.StreamExecutionEnvironment;
import org.apache.flink.streaming.api.functions.co.BroadcastProcessFunction;
import org.apache.flink.streaming.api.functions.sink.RichSinkFunction;
import org.apache.flink.util.Collector;
import org.apache.flink.api.common.serialization.SimpleStringSchema;

import com.google.gson.*;
import net.sf.jsqlparser.parser.CCJSqlParserUtil;
import net.sf.jsqlparser.statement.select.*;
import net.sf.jsqlparser.expression.*;
import net.sf.jsqlparser.expression.operators.conditional.*;
import net.sf.jsqlparser.expression.operators.relational.*;
import net.sf.jsqlparser.schema.Column;

import java.net.URI;
import java.net.http.*;
import java.time.*;
import java.util.*;

public class SafePCCepJob {
    
    private static final String KAFKA_BOOTSTRAP = System.getenv().getOrDefault("KAFKA_BOOTSTRAP", "43.202.241.121:44105");
    private static final String OPENSEARCH_URL = System.getenv().getOrDefault("OPENSEARCH_URL", "http://203.229.154.49:49200");
    
    public static void main(String[] args) throws Exception {
        StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();
        
        // 이벤트 스트림
        KafkaSource<String> eventSource = KafkaSource.<String>builder()
            .setBootstrapServers(KAFKA_BOOTSTRAP)
            .setTopics("safepc-logs")
            .setGroupId("siem-cep-engine")
            .setStartingOffsets(OffsetsInitializer.latest())
            .setValueOnlyDeserializer(new SimpleStringSchema())
            .build();
        
        // 규칙 업데이트 스트림
        KafkaSource<String> ruleSource = KafkaSource.<String>builder()
            .setBootstrapServers(KAFKA_BOOTSTRAP)
            .setTopics("cep-rule-updates")
            .setGroupId("siem-cep-rule-loader")
            .setStartingOffsets(OffsetsInitializer.latest())
            .setValueOnlyDeserializer(new SimpleStringSchema())
            .build();
        
        DataStream<CefEvent> events = env.fromSource(eventSource, WatermarkStrategy.noWatermarks(), "events")
            .map(SafePCCepJob::parseCef).filter(e -> e != null);
        
        DataStream<RuleUpdate> ruleUpdates = env.fromSource(ruleSource, WatermarkStrategy.noWatermarks(), "rules")
            .map(SafePCCepJob::parseRuleUpdate).filter(r -> r != null);
        
        // Broadcast State
        MapStateDescriptor<String, CompiledRule> ruleStateDesc = new MapStateDescriptor<>(
            "cep-rules", Types.STRING, Types.GENERIC(CompiledRule.class));
        
        BroadcastStream<RuleUpdate> ruleBroadcast = ruleUpdates.broadcast(ruleStateDesc);
        
        events.connect(ruleBroadcast).process(new CepProcessor(ruleStateDesc)).addSink(new AlertSink());
        events.addSink(new LogSink());
        
        env.execute("SafePC CEP Engine");
    }
    
    // ========== CEP Processor ==========
    public static class CepProcessor extends BroadcastProcessFunction<CefEvent, RuleUpdate, Alert> {
        private final MapStateDescriptor<String, CompiledRule> stateDesc;
        private transient Map<String, CompiledRule> localRules;
        private transient boolean initialized = false;
        
        public CepProcessor(MapStateDescriptor<String, CompiledRule> stateDesc) {
            this.stateDesc = stateDesc;
        }
        
        @Override
        public void processElement(CefEvent event, ReadOnlyContext ctx, Collector<Alert> out) throws Exception {
            if (!initialized) {
                localRules = loadAllRules();
                initialized = true;
                System.out.println("[INIT] Loaded " + localRules.size() + " rules");
            }
            
            // Broadcast State 반영
            for (Map.Entry<String, CompiledRule> entry : ctx.getBroadcastState(stateDesc).immutableEntries()) {
                localRules.put(entry.getKey(), entry.getValue());
            }
            
            // 규칙 평가
            for (CompiledRule rule : localRules.values()) {
                if (rule != null && rule.enabled && rule.evaluate(event)) {
                    out.collect(new Alert(rule, event));
                    System.out.println("[ALERT] " + rule.name + " - " + event.userId);
                }
            }
        }
        
        @Override
        public void processBroadcastElement(RuleUpdate update, Context ctx, Collector<Alert> out) throws Exception {
            BroadcastState<String, CompiledRule> state = ctx.getBroadcastState(stateDesc);
            
            if ("delete".equals(update.action)) {
                state.remove(update.ruleId);
                if (localRules != null) localRules.remove(update.ruleId);
                System.out.println("[RULE DELETE] " + update.ruleId);
            } else {
                CompiledRule rule = fetchRule(update.ruleId);
                if (rule != null) {
                    state.put(update.ruleId, rule);
                    if (localRules != null) localRules.put(update.ruleId, rule);
                    System.out.println("[RULE UPSERT] " + update.ruleId);
                }
            }
        }
        
        private Map<String, CompiledRule> loadAllRules() {
            Map<String, CompiledRule> rules = new HashMap<>();
            try {
                HttpClient client = HttpClient.newHttpClient();
                HttpRequest req = HttpRequest.newBuilder()
                    .uri(URI.create(OPENSEARCH_URL + "/safepc-cep-rules/_search?size=100")).GET().build();
                HttpResponse<String> res = client.send(req, HttpResponse.BodyHandlers.ofString());
                
                JsonObject json = JsonParser.parseString(res.body()).getAsJsonObject();
                for (var hit : json.getAsJsonObject("hits").getAsJsonArray("hits")) {
                    JsonObject src = hit.getAsJsonObject().getAsJsonObject("_source");
                    String id = hit.getAsJsonObject().get("_id").getAsString();
                    CompiledRule rule = CompiledRule.fromJson(src, id);
                    if (rule != null) rules.put(id, rule);
                }
            } catch (Exception e) {
                System.err.println("[LOAD ERROR] " + e.getMessage());
            }
            return rules;
        }
        
        private CompiledRule fetchRule(String ruleId) {
            try {
                HttpClient client = HttpClient.newHttpClient();
                HttpRequest req = HttpRequest.newBuilder()
                    .uri(URI.create(OPENSEARCH_URL + "/safepc-cep-rules/_doc/" + ruleId)).GET().build();
                HttpResponse<String> res = client.send(req, HttpResponse.BodyHandlers.ofString());
                
                JsonObject json = JsonParser.parseString(res.body()).getAsJsonObject();
                if (json.has("_source")) {
                    return CompiledRule.fromJson(json.getAsJsonObject("_source"), ruleId);
                }
            } catch (Exception e) {
                System.err.println("[FETCH ERROR] " + e.getMessage());
            }
            return null;
        }
    }
    
    // ========== CompiledRule (JSQLParser) ==========
    public static class CompiledRule implements java.io.Serializable {
        public String ruleId, name, severity, description, sql;
        public boolean enabled;
        public transient Expression whereExpr;
        
        public static CompiledRule fromJson(JsonObject src, String id) {
            try {
                CompiledRule r = new CompiledRule();
                r.ruleId = id;
                r.name = src.has("name") ? src.get("name").getAsString() : id;
                r.severity = src.has("severity") ? src.get("severity").getAsString() : "MEDIUM";
                r.description = src.has("description") ? src.get("description").getAsString() : "";
                r.enabled = !src.has("enabled") || src.get("enabled").getAsBoolean();
                r.sql = src.has("sql") ? src.get("sql").getAsString() : null;
                
                if (r.sql != null) r.parseWhere();
                return r.whereExpr != null ? r : null;
            } catch (Exception e) {
                return null;
            }
        }
        
        private void parseWhere() {
            try {
                var stmt = CCJSqlParserUtil.parse(sql);
                if (stmt instanceof Select) {
                    PlainSelect ps = (PlainSelect) ((Select) stmt).getSelectBody();
                    whereExpr = ps.getWhere();
                }
            } catch (Exception e) {
                System.err.println("[PARSE ERROR] " + sql + " - " + e.getMessage());
            }
        }
        
        public boolean evaluate(CefEvent event) {
            if (whereExpr == null) {
                parseWhere();
                if (whereExpr == null) return false;
            }
            try {
                return evalExpr(whereExpr, event);
            } catch (Exception e) {
                return false;
            }
        }
        
        private boolean evalExpr(Expression expr, CefEvent e) {
            if (expr instanceof AndExpression) {
                AndExpression and = (AndExpression) expr;
                return evalExpr(and.getLeftExpression(), e) && evalExpr(and.getRightExpression(), e);
            }
            if (expr instanceof OrExpression) {
                OrExpression or = (OrExpression) expr;
                return evalExpr(or.getLeftExpression(), e) || evalExpr(or.getRightExpression(), e);
            }
            if (expr instanceof Parenthesis) {
                return evalExpr(((Parenthesis) expr).getExpression(), e);
            }
            if (expr instanceof EqualsTo) {
                EqualsTo eq = (EqualsTo) expr;
                String field = getColumnName(eq.getLeftExpression());
                String expected = getValue(eq.getRightExpression());
                String actual = e.getField(field);
                return expected != null && expected.equals(actual);
            }
            if (expr instanceof NotEqualsTo) {
                NotEqualsTo neq = (NotEqualsTo) expr;
                String field = getColumnName(neq.getLeftExpression());
                String expected = getValue(neq.getRightExpression());
                String actual = e.getField(field);
                return actual == null || !expected.equals(actual);
            }
            if (expr instanceof GreaterThanEquals) {
                GreaterThanEquals gte = (GreaterThanEquals) expr;
                String field = getColumnName(gte.getLeftExpression());
                double expected = getNumericValue(gte.getRightExpression());
                return e.getNumericField(field) >= expected;
            }
            if (expr instanceof GreaterThan) {
                GreaterThan gt = (GreaterThan) expr;
                String field = getColumnName(gt.getLeftExpression());
                double expected = getNumericValue(gt.getRightExpression());
                return e.getNumericField(field) > expected;
            }
            if (expr instanceof MinorThanEquals) {
                MinorThanEquals lte = (MinorThanEquals) expr;
                String field = getColumnName(lte.getLeftExpression());
                double expected = getNumericValue(lte.getRightExpression());
                return e.getNumericField(field) <= expected;
            }
            if (expr instanceof MinorThan) {
                MinorThan lt = (MinorThan) expr;
                String field = getColumnName(lt.getLeftExpression());
                double expected = getNumericValue(lt.getRightExpression());
                return e.getNumericField(field) < expected;
            }
            if (expr instanceof InExpression) {
                InExpression in = (InExpression) expr;
                String field = getColumnName(in.getLeftExpression());
                String actual = e.getField(field);
                if (actual == null) return in.isNot();
                
                ExpressionList list = (ExpressionList) in.getRightItemsList();
                for (Expression item : list.getExpressions()) {
                    if (actual.equals(getValue(item))) return !in.isNot();
                }
                return in.isNot();
            }
            if (expr instanceof LikeExpression) {
                LikeExpression like = (LikeExpression) expr;
                String field = getColumnName(like.getLeftExpression());
                String pattern = getValue(like.getRightExpression());
                String actual = e.getField(field);
                if (actual == null || pattern == null) return false;
                
                String regex = pattern.replace("%", ".*").replace("_", ".");
                return actual.matches(regex) != like.isNot();
            }
            return false;
        }
        
        private String getColumnName(Expression expr) {
            if (expr instanceof Column) return ((Column) expr).getColumnName();
            return expr.toString();
        }
        
        private String getValue(Expression expr) {
            if (expr instanceof StringValue) return ((StringValue) expr).getValue();
            if (expr instanceof LongValue) return String.valueOf(((LongValue) expr).getValue());
            if (expr instanceof DoubleValue) return String.valueOf(((DoubleValue) expr).getValue());
            return expr.toString().replace("'", "").replace("\"", "");
        }
        
        private double getNumericValue(Expression expr) {
            if (expr instanceof LongValue) return ((LongValue) expr).getValue();
            if (expr instanceof DoubleValue) return ((DoubleValue) expr).getValue();
            try { return Double.parseDouble(expr.toString()); } catch (Exception e) { return 0; }
        }
    }
    
    // ========== Data Classes ==========
    public static class CefEvent implements java.io.Serializable {
        public String msgId, userId, userName, hostname, userIp, action, outcome, eventType, timestamp;
        public long fileSize;
        public int pageCount, hour, dayOfWeek;
        public Map<String, String> ext = new HashMap<>();
        
        public String getField(String field) {
            switch (field) {
                case "msgId": return msgId;
                case "suid": case "userId": return userId;
                case "suser": case "userName": return userName;
                case "hostname": case "shost": return hostname;
                case "src": case "userIp": return userIp;
                case "act": case "action": return action;
                case "outcome": return outcome;
                case "eventType": return eventType;
                case "hour": return String.valueOf(hour);
                case "dayOfWeek": return String.valueOf(dayOfWeek);
                default: return ext.get(field);
            }
        }
        
        public double getNumericField(String field) {
            switch (field) {
                case "fsize": case "fileSize": return fileSize;
                case "pageCount": case "PageCount": return pageCount;
                case "hour": return hour;
                case "dayOfWeek": return dayOfWeek;
                default:
                    String v = ext.get(field);
                    if (v != null) try { return Double.parseDouble(v); } catch (Exception e) {}
                    return 0;
            }
        }
    }
    
    public static class RuleUpdate implements java.io.Serializable {
        public String ruleId, action;
    }
    
    public static class Alert implements java.io.Serializable {
        public String ruleId, ruleName, severity, description, userId, hostname, userIp, alertId, timestamp;
        
        public Alert(CompiledRule r, CefEvent e) {
            this.ruleId = r.ruleId; this.ruleName = r.name; this.severity = r.severity;
            this.description = r.description; this.userId = e.userId; this.hostname = e.hostname;
            this.userIp = e.userIp; this.timestamp = Instant.now().toString();
            this.alertId = "alert-" + UUID.randomUUID().toString().substring(0, 12);
        }
    }
    
    // ========== Parsers ==========
    private static CefEvent parseCef(String raw) {
        try {
            CefEvent e = new CefEvent();
            JsonObject json = JsonParser.parseString(raw).getAsJsonObject();
            
            e.msgId = json.has("msgId") ? json.get("msgId").getAsString() : null;
            e.hostname = json.has("hostname") ? json.get("hostname").getAsString() : null;
            
            if (json.has("cefExtensions")) {
                JsonObject ext = json.getAsJsonObject("cefExtensions");
                e.userId = getStr(ext, "suid");
                e.userName = getStr(ext, "suser");
                e.userIp = getStr(ext, "src");
                e.action = getStr(ext, "act");
                e.outcome = getStr(ext, "outcome");
                e.eventType = getStr(ext, "eventType");
                
                if (ext.has("fsize") && !ext.get("fsize").isJsonNull())
                    e.fileSize = ext.get("fsize").getAsLong();
                
                // Label 기반 매핑
                for (String key : ext.keySet()) {
                    if (key.endsWith("Label") && !ext.get(key).isJsonNull()) {
                        String labelName = ext.get(key).getAsString().replace(" ", "");
                        String valueKey = key.replace("Label", "");
                        if (ext.has(valueKey) && !ext.get(valueKey).isJsonNull())
                            e.ext.put(labelName, ext.get(valueKey).getAsString());
                    }
                }
                
                String pc = e.ext.get("PageCount");
                if (pc != null) try { e.pageCount = Integer.parseInt(pc); } catch (Exception ignored) {}
            }
            
            ZonedDateTime now = ZonedDateTime.now(ZoneId.of("Asia/Seoul"));
            e.hour = now.getHour();
            e.dayOfWeek = now.getDayOfWeek().getValue();
            e.timestamp = Instant.now().toString();
            
            return e.msgId != null ? e : null;
        } catch (Exception ex) { return null; }
    }
    
    private static RuleUpdate parseRuleUpdate(String raw) {
        try {
            JsonObject json = JsonParser.parseString(raw).getAsJsonObject();
            RuleUpdate u = new RuleUpdate();
            u.ruleId = json.get("ruleId").getAsString();
            u.action = json.has("action") ? json.get("action").getAsString() : "upsert";
            return u;
        } catch (Exception e) { return null; }
    }
    
    private static String getStr(JsonObject o, String k) {
        return o.has(k) && !o.get(k).isJsonNull() ? o.get(k).getAsString() : null;
    }
    
    // ========== Sinks ==========
    public static class AlertSink extends RichSinkFunction<Alert> {
        @Override
        public void invoke(Alert alert, Context ctx) {
            try {
                HttpClient client = HttpClient.newHttpClient();
                HttpRequest req = HttpRequest.newBuilder()
                    .uri(URI.create(OPENSEARCH_URL + "/safepc-alerts/_doc"))
                    .header("Content-Type", "application/json")
                    .POST(HttpRequest.BodyPublishers.ofString(new Gson().toJson(alert))).build();
                client.send(req, HttpResponse.BodyHandlers.ofString());
            } catch (Exception e) { System.err.println("[ALERT ERROR] " + e.getMessage()); }
        }
    }
    
    public static class LogSink extends RichSinkFunction<CefEvent> {
        @Override
        public void invoke(CefEvent event, Context ctx) {
            try {
                HttpClient client = HttpClient.newHttpClient();
                HttpRequest req = HttpRequest.newBuilder()
                    .uri(URI.create(OPENSEARCH_URL + "/safepc-logs-" + java.time.LocalDate.now() + "/_doc"))
                    .header("Content-Type", "application/json")
                    .POST(HttpRequest.BodyPublishers.ofString(new Gson().toJson(event))).build();
                client.send(req, HttpResponse.BodyHandlers.ofString());
            } catch (Exception e) { System.err.println("[LOG ERROR] " + e.getMessage()); }
        }
    }
}
