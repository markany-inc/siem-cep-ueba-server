FROM python:3.11-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY . .
ENV LANG=C.UTF-8
ENV LC_ALL=C.UTF-8
EXPOSE 8501
CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8501"]
