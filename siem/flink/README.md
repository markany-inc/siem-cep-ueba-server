# Flink CEP

## Docker 이미지
- `flink-1.18-java11.tar` - Apache Flink 1.18 (Java 11)

## 이미지 로드
```bash
docker load < images/flink-1.18-java11.tar
```

## 실행
```bash
docker-compose up -d
```

## 포트
- 48081: Flink Web UI

## CEP Job 빌드
```bash
cd jobs
mvn clean package -DskipTests
# 결과: target/safepc-cep-1.0.0.jar
```

## Job 배포
```bash
# 기존 Job 중지 (있으면)
JOB_ID=$(curl -s http://localhost:48081/jobs | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
curl -X PATCH "http://localhost:48081/jobs/$JOB_ID?mode=cancel"

# 새 Job 제출
curl -X POST http://localhost:48081/jars/upload -F "jarfile=@jobs/target/safepc-cep-1.0.0.jar"
JAR_ID=$(curl -s http://localhost:48081/jars | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
curl -X POST "http://localhost:48081/jars/$JAR_ID/run"
```

## 소스 코드
- `jobs/src/main/java/com/markany/siem/SafePCCepJob.java` - 메인 CEP Job
- `jobs/pom.xml` - Maven 빌드 설정
- `jobs/cep_rules.py` - CEP 룰 정의 (참고용)

## Kafka 토픽 구독
- Consumer Group: `siem-flink-cep-group`
- 토픽: MESSAGE_AGENT, MESSAGE_DEVICE, MESSAGE_NETWORK, MESSAGE_PROCESS, MESSAGE_SCREENBLOCKER
- 인증 로그: MESSAGE_AGENT 토픽 내 MESSAGE_AGENT_AUTHENTICATION 사용
