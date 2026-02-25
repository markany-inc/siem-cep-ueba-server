# OpenSearch + Dashboards

## Docker 이미지
- `opensearch-2.11.1.tar` - OpenSearch 검색엔진
- `opensearch-dashboards-2.11.1.tar` - 웹 UI

## 이미지 로드
```bash
docker load < images/opensearch-2.11.1.tar
docker load < images/opensearch-dashboards-2.11.1.tar
```

## 실행
```bash
docker-compose up -d
```

## 포트
- 49200: OpenSearch API
- 45601: Dashboards UI

## 데이터 디렉토리
- `./data` - OpenSearch 인덱스 데이터 (볼륨 마운트)
