# PaddleOCR 依赖较重,首次构建需要较长时间;模型文件在首次识别时下载,
# 通过 volume 挂载 ~/.paddleocr 可以在容器重建后复用。
FROM python:3.10-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
        libgomp1 libglib2.0-0 libgl1 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY app.py .

EXPOSE 8000
CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8000"]
