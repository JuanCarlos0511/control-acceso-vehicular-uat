"""Servicio HTTP de análisis de documentos (INE y tarjeta de circulación).

No guarda imágenes ni resultados y no registra datos personales en los logs.
"""

from __future__ import annotations

import hmac
import logging
import os
from contextlib import asynccontextmanager
from datetime import date, datetime
from functools import lru_cache
from zoneinfo import ZoneInfo

import numpy as np
from fastapi import FastAPI, File, Header, HTTPException, Request, UploadFile
from fastapi.responses import JSONResponse
from pydantic import BaseModel

from . import ine, repuve, tarjeta
from .imagen import DetectorRostros, ImagenInvalida, decodificar, leer_qr, medir_calidad, preparar, rotar
from .ocr import Linea, MotorOCR, MotorRapidOCR

VERSION = "1.0.0"
MAX_BYTES = int(os.environ.get("ANALIZADOR_MAX_BYTES", str(12 * 1024 * 1024)))
ZONA = ZoneInfo(os.environ.get("ZONA_HORARIA", "America/Monterrey"))
TOKEN = os.environ.get("ANALIZADOR_TOKEN", "")

log = logging.getLogger("analizador")


@lru_cache(maxsize=1)
def motor() -> MotorOCR:
    return MotorRapidOCR()


@lru_cache(maxsize=1)
def consultor_repuve() -> repuve.ConsultorREPUVE:
    return repuve.ConsultorREPUVE(repuve.Opciones.desde_entorno())


@asynccontextmanager
async def ciclo_de_vida(_: FastAPI):
    # Carga los modelos de OCR al arrancar para que la primera solicitud no espere.
    motor().leer(np.full((64, 256, 3), 255, np.uint8))
    yield
    await consultor_repuve().cerrar()


app = FastAPI(title="Analizador de documentos · Registro vehicular", version=VERSION, lifespan=ciclo_de_vida)


@lru_cache(maxsize=1)
def detector_rostros() -> DetectorRostros | None:
    return DetectorRostros.desde_entorno()


def hoy() -> date:
    return datetime.now(ZONA).date()


class ErrorAnalisis(Exception):
    def __init__(self, estado: int, codigo: str, mensaje: str) -> None:
        self.estado, self.codigo, self.mensaje = estado, codigo, mensaje


@app.exception_handler(ErrorAnalisis)
async def _error_analisis(_: Request, e: ErrorAnalisis) -> JSONResponse:
    return JSONResponse({"error": {"codigo": e.codigo, "mensaje": e.mensaje}}, status_code=e.estado)


@app.exception_handler(HTTPException)
async def _error_http(_: Request, e: HTTPException) -> JSONResponse:
    return JSONResponse({"error": {"codigo": "SOLICITUD_INVALIDA", "mensaje": str(e.detail)}}, status_code=e.status_code)


def _autorizar(token: str | None) -> None:
    if TOKEN and not hmac.compare_digest((token or "").encode(), TOKEN.encode()):
        raise ErrorAnalisis(401, "NO_AUTORIZADO", "Token del analizador inválido.")


def _leer(archivo: UploadFile, campo: str) -> np.ndarray:
    datos = archivo.file.read(MAX_BYTES + 1)
    if len(datos) > MAX_BYTES:
        raise ErrorAnalisis(413, "IMAGEN_DEMASIADO_GRANDE", f"La imagen de {campo} supera el tamaño máximo.")
    try:
        return decodificar(datos)
    except ImagenInvalida as e:
        raise ErrorAnalisis(422, "IMAGEN_INVALIDA", f"{campo}: {e}") from e


def _ocr_orientado(imagen: np.ndarray, puntuar) -> tuple[np.ndarray, list[Linea]]:
    """Lee la imagen y, si no parece el documento esperado, prueba girándola."""
    mejor_img, mejor_lineas = imagen, motor().leer(imagen)
    mejor = puntuar(mejor_lineas)
    if mejor >= 0.5:
        return mejor_img, mejor_lineas
    for grados in (90, 270, 180):
        girada = rotar(imagen, grados)
        lineas = motor().leer(girada)
        puntos = puntuar(lineas)
        if puntos > mejor:
            mejor_img, mejor_lineas, mejor = girada, lineas, puntos
        if mejor >= 0.5:
            break
    return mejor_img, mejor_lineas


def _puntaje_ine(lineas: list[Linea]) -> float:
    textos = [l.texto for l in lineas]
    extra = 0.5 if any("<<" in t for t in textos) else 0.0
    return ine.confianza_documento(textos) + extra


def _puntaje_tarjeta(lineas: list[Linea]) -> float:
    return tarjeta.confianza_documento([l.texto for l in lineas])


@app.get("/salud")
def salud() -> dict:
    return {"ok": True, "version": VERSION, "motorOcr": MotorRapidOCR.nombre,
            "repuve": consultor_repuve().o.habilitado}


@app.post("/v1/ine")
def analizar_ine(
    frente: UploadFile = File(...),
    reverso: UploadFile | None = File(None),
    x_token_analizador: str | None = Header(None),
) -> dict:
    _autorizar(x_token_analizador)
    img_frente = preparar(_leer(frente, "frente"))
    img_frente, lineas_frente = _ocr_orientado(img_frente, _puntaje_ine)
    calidad_frente = medir_calidad(img_frente, detector_rostros())
    qrs = leer_qr(img_frente)

    lineas_reverso = None
    calidad_reverso = None
    if reverso is not None and reverso.filename:
        img_reverso = preparar(_leer(reverso, "reverso"))
        img_reverso, lineas_reverso = _ocr_orientado(img_reverso, _puntaje_ine)
        calidad_reverso = medir_calidad(img_reverso).a_dict()
        qrs += leer_qr(img_reverso)

    resultado = ine.analizar(lineas_frente, img_frente.shape[1], lineas_reverso, qrs, hoy())
    resultado["calidad"] = {"frente": calidad_frente.a_dict(), "reverso": calidad_reverso}
    resultado["motor"] = {"ocr": MotorRapidOCR.nombre, "version": VERSION}
    if os.environ.get("ANALIZADOR_DEPURACION") == "1":
        resultado["lineas"] = [l.texto for l in lineas_frente + (lineas_reverso or [])]
    return resultado


@app.post("/v1/tarjeta")
def analizar_tarjeta(
    imagen: UploadFile = File(...),
    x_token_analizador: str | None = Header(None),
) -> dict:
    _autorizar(x_token_analizador)
    img = preparar(_leer(imagen, "tarjeta"))
    img, lineas = _ocr_orientado(img, _puntaje_tarjeta)
    resultado = tarjeta.analizar(lineas, hoy())
    resultado["calidad"] = medir_calidad(img).a_dict()
    resultado["motor"] = {"ocr": MotorRapidOCR.nombre, "version": VERSION}
    if os.environ.get("ANALIZADOR_DEPURACION") == "1":
        resultado["lineas"] = [l.texto for l in lineas]
    return resultado


class ConsultaREPUVE(BaseModel):
    niv: str | None = None
    placa: str | None = None


@app.post("/v1/repuve")
async def consultar_repuve(consulta: ConsultaREPUVE, x_token_analizador: str | None = Header(None)) -> dict:
    """Consulta REPUVE por NIV (o por placa). Las consultas se atienden de una
    en una; si el portal no responde, el estatus es NO_DISPONIBLE."""
    _autorizar(x_token_analizador)
    try:
        criterio, valor = repuve.normalizar_consulta(consulta.niv, consulta.placa)
    except ValueError as e:
        raise ErrorAnalisis(422, "CONSULTA_INVALIDA", str(e)) from e
    return await consultor_repuve().consultar(criterio, valor)
