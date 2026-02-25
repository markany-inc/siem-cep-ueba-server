# SafePC SIEM

운영 서버에 영향 없이 SIEM(이상행위 탐지) 기능을 확장하는 Extension 모듈

## 디렉토리 구조
```
siem/
├── .env                    # 환경 설정 (Kafka, OpenSearch)
├── opensearch/             # OpenSearch + Dashboards
├── flink/                  # Flink CEP (실시간 룰 기반 탐지)
├── ueba/                   # UEBA (ML 기반 이상탐지)
└── scripts/                # 마이그레이션 스크립트
```

## 서버 배포 순서
```bash
# 1. 이미지 로드
docker load < opensearch/images/opensearch-2.11.1.tar
docker load < opensearch/images/opensearch-dashboards-2.11.1.tar
docker load < flink/images/flink-1.18-java11.tar
docker load < ueba/images/python-3.11-slim.tar

# 2. OpenSearch 실행
cd opensearch && docker-compose up -d

# 3. Flink 실행
cd ../flink && docker-compose up -d

# 4. Flink Job 배포
curl -X POST http://localhost:48081/jars/upload -F "jarfile=@jobs/safepc-cep-1.0.0.jar"
# ... (flink/README.md 참고)

# 5. UEBA 설정
pip3 install opensearch-py scikit-learn numpy
crontab -e  # ueba/README.md 참고
```

## 접속 정보
- OpenSearch Dashboards: http://{서버IP}:45601
- Flink Web UI: http://{서버IP}:48081

## 환경 설정 (.env)
서버 이관 시 `.env` 파일의 Kafka 주소만 변경
```
KAFKA_BOOTSTRAP_SERVERS=43.202.241.121:44105
```
