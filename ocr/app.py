"""Phoenix OCR 服务 —— PaddleOCR 的 HTTP 封装。

PaddleOCR 是 Python 生态,与 Go 主服务通过本服务的 HTTP 边界解耦。
接口契约(internal/ocr/client.go 依赖):
    POST /ocr   multipart 字段 file  →  {"text": "识别出的全文"}
    GET  /healthz                     →  200
"""

import tempfile

from fastapi import FastAPI, File, HTTPException, UploadFile

app = FastAPI(title="Phoenix OCR Service")

_ocr = None


def get_ocr():
    """懒加载 PaddleOCR(首次调用会下载模型,启动即加载会拖慢容器就绪)。"""
    global _ocr
    if _ocr is None:
        from paddleocr import PaddleOCR

        _ocr = PaddleOCR(use_angle_cls=True, lang="ch", show_log=False)
    return _ocr


@app.get("/healthz")
def healthz():
    return {"status": "ok"}


@app.post("/ocr")
async def recognize(file: UploadFile = File(...)):
    data = await file.read()
    if not data:
        raise HTTPException(status_code=400, detail="empty file")

    suffix = "." + (file.filename or "img.png").rsplit(".", 1)[-1]
    with tempfile.NamedTemporaryFile(suffix=suffix) as tmp:
        tmp.write(data)
        tmp.flush()
        try:
            result = get_ocr().ocr(tmp.name, cls=True)
        except Exception as exc:  # PaddleOCR 对坏图会抛各种异常,统一转 422
            raise HTTPException(status_code=422, detail=f"ocr failed: {exc}") from exc

    lines = []
    for page in result or []:
        for item in page or []:
            # item: [box, (text, score)]
            lines.append(item[1][0])
    return {"text": "\n".join(lines)}
