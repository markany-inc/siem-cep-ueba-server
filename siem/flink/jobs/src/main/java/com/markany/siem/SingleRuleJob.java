package com.markany.siem;

import org.apache.flink.api.common.eventtime.WatermarkStrategy;
import org.apache.flink.api.common.restartstrategy.RestartStrategies;
import org.apache.flink.api.common.serialization.SimpleStringSchema;
import org.apache.flink.api.java.utils.ParameterTool;
import org.apache.flink.connector.kafka.source.KafkaSource;
import org.apache.flink.connector.kafka.source.enumerator.initializer.OffsetsInitializer;
import org.apache.flink.streaming.api.datastream.DataStream;
import org.apache.flink.streaming.api.environment.StreamExecutionEnvironment;
import org.apache.flink.table.api.Schema;
import org.apache.flink.table.api.Table;
import org.apache.flink.table.api.bridge.java.StreamTableEnvironment;
import org.apache.flink.types.Row;
import com.google.gson.*;
import java.net.http.*;
import java.net.URI;
import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.time.*;
import java.util.UUID;

/**
 * 단일 규칙 실행 Job
 * 사용법: --rule-name "USB_대량_복사"
 */
public class SingleRuleJob {
    
    private static final String KAFKA_BOOTSTRAP = System.getenv().getOrDefault("KAFKA_BOOTSTRAP_SERVERS", "43.202.241.121:44105");
    private static final String OPENSEARCH_URL = System.getenv().getOrDefault("OPENSEARCH_URL", "http://siem-opensearch:9200");
    private static final String DASHBOARD_URL = System.getenv().getOrDefault("DASHBOARD_URL", "http://siem-dashboard:8501");
    
    private static final String[] TOPICS = {
        "MESSAGE_AGENT", "MESSAGE_ASSETS", "MESSAGE_DEVICE", "MESSAGE_NETWORK",
        "MESSAGE_PROCESS", "MESSAGE_SCREENBLOCKER", "MESSAGE_PRINT", "MESSAGE_DRM",
        "MESSAGE_CLIPBOARD", "MESSAGE_CAPTURE", "MESSAGE_PC"
    };
    
    public static void main(String[] args) throws Exception {
        // 공백이 포함된 규칙명 처리: --rule-name 뒤의 모든 인자를 합침
        String ruleName = null;
        for (int i = 0; i < args.length; i++) {
            if ("--rule-name".equals(args[i]) && i + 1 < args.length) {
                StringBuilder sb = new StringBuilder();
                for (int j = i + 1; j < args.length; j++) {
                    if (args[j].startsWith("--")) break;
                    if (sb.length() > 0) sb.append(" ");
                    sb.append(args[j]);
                }
                ruleName = sb.toString();
                break;
            }
        }
        
        if (ruleName == null || ruleName.isEmpty()) {
            System.err.println("Usage: --rule-name <규칙명>");
            System.exit(1);
        }
        
        Rule rule = loadRule(ruleName);
        if (rule == null) {
            System.err.println("Rule not found: " + ruleName);
            System.exit(1);
        }
        
        if (rule.sql == null || rule.sql.isEmpty()) {
            System.err.println("Rule has no SQL: " + ruleName);
            System.exit(1);
        }
        
        System.out.println("[INIT] Rule: " + rule.name + ", SQL: " + rule.sql);
        
        StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();
        env.setRestartStrategy(RestartStrategies.fixedDelayRestart(Integer.MAX_VALUE,
            org.apache.flink.api.common.time.Time.seconds(10)));
        
        StreamTableEnvironment tableEnv = StreamTableEnvironment.create(env);
        
        String groupId = "cep-" + ruleName.replaceAll("[^a-zA-Z0-9가-힣_-]", "_");
        KafkaSource<String> source = KafkaSource.<String>builder()
            .setBootstrapServers(KAFKA_BOOTSTRAP)
            .setTopics(TOPICS)
            .setGroupId(groupId)
            .setStartingOffsets(OffsetsInitializer.latest())
            .setValueOnlyDeserializer(new SimpleStringSchema())
            .build();
        
        DataStream<CefEvent> events = env.fromSource(source, WatermarkStrategy.noWatermarks(), "Kafka Source")
            .map(SingleRuleJob::parseCef).filter(e -> e != null);
        
        tableEnv.createTemporaryView("events", events, 
            Schema.newBuilder()
                .columnByExpression("proctime", "PROCTIME()")
                .build());
        
        Table result = tableEnv.sqlQuery(rule.sql);
        
        final String rId = rule.ruleId;
        final String rName = rule.name;
        final String severity = rule.severity;
        final String description = rule.description;
        
        tableEnv.toDataStream(result, Row.class)
            .map(row -> {
                CefEvent e = new CefEvent();
                e.userId = row.getField(0) != null ? row.getField(0).toString() : "unknown";
                e.hostname = row.getField(1) != null ? row.getField(1).toString() : null;
                e.userIp = row.getField(2) != null ? row.getField(2).toString() : null;
                long cnt = 1;
                if (row.getArity() > 3 && row.getField(3) != null) {
                    cnt = ((Number)row.getField(3)).longValue();
                }
                String desc = cnt > 1 ? description + " (" + cnt + "회)" : description;
                return new Alert(rId, rName, severity, desc, e);
            })
            .addSink(new AlertSink());
        
        env.execute("CEP-" + ruleName);
    }
    
    private static Rule loadRule(String ruleName) {
        try {
            HttpClient client = HttpClient.newHttpClient();
            String encoded = URLEncoder.encode(ruleName, StandardCharsets.UTF_8).replace("+", "%20");
            String url = OPENSEARCH_URL + "/safepc-cep-rules/_doc/" + encoded;
            System.out.println("[DEBUG] Loading rule from: " + url);
            HttpRequest req = HttpRequest.newBuilder()
                .uri(URI.create(url))
                .GET()
                .build();
            HttpResponse<String> res = client.send(req, HttpResponse.BodyHandlers.ofString());
            System.out.println("[DEBUG] Response status: " + res.statusCode());
            
            if (res.statusCode() != 200) {
                System.err.println("[DEBUG] Response body: " + res.body());
                return null;
            }
            
            JsonObject json = JsonParser.parseString(res.body()).getAsJsonObject();
            if (!json.has("_source")) return null;
            
            JsonObject src = json.getAsJsonObject("_source");
            Rule r = new Rule();
            r.ruleId = src.has("ruleId") ? src.get("ruleId").getAsString() : ruleName;
            r.name = src.has("name") ? src.get("name").getAsString() : ruleName;
            r.severity = src.has("severity") ? src.get("severity").getAsString() : "MEDIUM";
            r.description = src.has("description") ? src.get("description").getAsString() : "";
            r.sql = src.has("sql") && !src.get("sql").isJsonNull() ? src.get("sql").getAsString() : null;
            return r;
        } catch (Exception e) {
            System.err.println("[ERROR] Failed to load rule: " + e.getMessage());
            return null;
        }
    }
    
    private static CefEvent parseCef(String raw) {
        try {
            CefEvent e = new CefEvent();
            JsonObject json = JsonParser.parseString(raw).getAsJsonObject();
            
            e.msgId = json.has("msgId") ? json.get("msgId").getAsString() : null;
            e.hostname = json.has("hostname") ? json.get("hostname").getAsString() : null;
            
            if (json.has("cefExtensions")) {
                JsonObject ext = json.getAsJsonObject("cefExtensions");
                e.userId = ext.has("suid") && !ext.get("suid").isJsonNull() ? ext.get("suid").getAsString() : null;
                e.userName = ext.has("suser") && !ext.get("suser").isJsonNull() ? ext.get("suser").getAsString() : null;
                e.userIp = ext.has("src") && !ext.get("src").isJsonNull() ? ext.get("src").getAsString() : null;
                e.action = ext.has("act") && !ext.get("act").isJsonNull() ? ext.get("act").getAsString() : null;
                e.outcome = ext.has("outcome") && !ext.get("outcome").isJsonNull() ? ext.get("outcome").getAsString() : null;
                e.deviceType = ext.has("cs1") && !ext.get("cs1").isJsonNull() ? ext.get("cs1").getAsString() : null;
                e.configType = ext.has("cs1") && !ext.get("cs1").isJsonNull() ? ext.get("cs1").getAsString() : null;
                e.eventType = ext.has("eventType") && !ext.get("eventType").isJsonNull() ? ext.get("eventType").getAsString() : null;
                e.isMalicious = ext.has("cn1") && !ext.get("cn1").isJsonNull() && "1".equals(ext.get("cn1").getAsString());
                if (ext.has("fsize") && !ext.get("fsize").isJsonNull()) e.fileSize = ext.get("fsize").getAsLong();
                if (ext.has("cn4") && !ext.get("cn4").isJsonNull()) e.pageCount = ext.get("cn4").getAsInt();
            }
            e.timestamp = Instant.now().toString();
            return e.msgId != null ? e : null;
        } catch (Exception ex) { return null; }
    }
    
    public static class CefEvent {
        public String msgId, userId, userName, hostname, userIp, action, outcome, deviceType, configType, eventType, timestamp;
        public long fileSize;
        public int pageCount;
        public boolean isMalicious;
    }
    
    public static class Rule {
        public String ruleId, name, severity, description, sql;
    }
    
    public static class Alert {
        public String ruleId, ruleName, severity, description, userId, userName, hostname, userIp, timestamp, alertId;
        public Alert(String ruleId, String ruleName, String severity, String desc, CefEvent e) {
            this.ruleId = ruleId; this.ruleName = ruleName; this.severity = severity; this.description = desc;
            this.userId = e.userId; this.userName = e.userName; this.hostname = e.hostname; this.userIp = e.userIp;
            this.timestamp = Instant.now().toString();
            this.alertId = "alert-" + UUID.randomUUID().toString().substring(0, 12);
        }
    }
    
    public static class AlertSink implements org.apache.flink.streaming.api.functions.sink.SinkFunction<Alert> {
        public void invoke(Alert a, Context ctx) {
            try {
                String idx = "safepc-alerts-" + LocalDate.now(ZoneId.of("Asia/Seoul")).toString().replace("-", ".");
                String json = new Gson().toJson(a).replace("\"timestamp\":", "\"@timestamp\":");
                
                HttpClient.newHttpClient().send(HttpRequest.newBuilder()
                    .uri(URI.create(OPENSEARCH_URL + "/" + idx + "/_doc"))
                    .header("Content-Type", "application/json")
                    .POST(HttpRequest.BodyPublishers.ofString(json)).build(),
                    HttpResponse.BodyHandlers.ofString());
                
                HttpClient.newHttpClient().send(HttpRequest.newBuilder()
                    .uri(URI.create(DASHBOARD_URL + "/api/alert/push"))
                    .header("Content-Type", "application/json")
                    .POST(HttpRequest.BodyPublishers.ofString(new Gson().toJson(a))).build(),
                    HttpResponse.BodyHandlers.ofString());
                
                System.out.println("[ALERT] " + a.ruleId + ": " + a.description + " (" + a.userId + ")");
            } catch (Exception e) { e.printStackTrace(); }
        }
    }
}
