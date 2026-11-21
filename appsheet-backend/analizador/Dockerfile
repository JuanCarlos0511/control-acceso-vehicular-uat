# Analizador: OCR (RapidOCR + OpenCV) y consulta a REPUVE (Playwright + Chromium).
# Bookworm porque es la versión de Debian que soporta `playwright install --with-deps`.
FROM python:3.13-slim-bookworm
ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    ZONA_HORARIA=America/Monterrey \
    PLAYWRIGHT_BROWSERS_PATH=/ms-playwright
WORKDIR /srv
COPY requirements.txt .
# RapidOCR descarga sus modelos la primera vez que se crea; se hace aquí porque
# en ejecución el usuario sin privilegios no puede escribir en site-packages.
RUN pip install --no-cache-dir -r requirements.txt \
 && python -c "from rapidocr import RapidOCR; RapidOCR()" \
 && python -m playwright install --with-deps --only-shell chromium \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --system --create-home --uid 10002 analizador
COPY app ./app
USER analizador
EXPOSE 8000
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000", "--workers", "1"]
