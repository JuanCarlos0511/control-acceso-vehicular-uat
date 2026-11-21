"""Motor de OCR y utilidades para trabajar con las cajas de texto detectadas."""

from __future__ import annotations

import threading
from dataclasses import dataclass
from typing import Protocol

import numpy as np

from .texto import normalizar


@dataclass(frozen=True)
class Linea:
    """Un bloque de texto detectado, con su caja alineada a los ejes."""

    texto: str  # normalizado: mayúsculas, sin acentos
    confianza: float
    x: float
    y: float
    ancho: float
    alto: float

    @property
    def centro_y(self) -> float:
        return self.y + self.alto / 2

    @property
    def derecha(self) -> float:
        return self.x + self.ancho


class MotorOCR(Protocol):
    nombre: str

    def leer(self, imagen: np.ndarray) -> list[Linea]: ...


class MotorRapidOCR:
    """OCR con RapidOCR (modelos PP-OCR en ONNX). No requiere binarios del sistema."""

    nombre = "rapidocr"

    def __init__(self) -> None:
        from rapidocr import RapidOCR

        self._motor = RapidOCR(params={"Global.log_level": "warning"})
        # La sesión de RapidOCR no garantiza ser segura entre hilos.
        self._candado = threading.Lock()

    def leer(self, imagen: np.ndarray) -> list[Linea]:
        with self._candado:
            salida = self._motor(imagen)
        if salida is None or salida.txts is None or salida.boxes is None:
            return []
        lineas = []
        for texto, confianza, caja in zip(salida.txts, salida.scores, salida.boxes):
            puntos = np.asarray(caja, dtype=float)
            x0, y0 = puntos[:, 0].min(), puntos[:, 1].min()
            x1, y1 = puntos[:, 0].max(), puntos[:, 1].max()
            t = normalizar(str(texto))
            if t:
                lineas.append(Linea(t, float(confianza), float(x0), float(y0), float(x1 - x0), float(y1 - y0)))
        return ordenar(lineas)


def ordenar(lineas: list[Linea]) -> list[Linea]:
    return sorted(lineas, key=lambda l: (l.y, l.x))


def agrupar_filas(lineas: list[Linea]) -> list[list[Linea]]:
    """Agrupa las cajas que comparten renglón (traslape vertical), de arriba hacia abajo."""
    filas: list[list[Linea]] = []
    for linea in sorted(lineas, key=lambda l: l.centro_y):
        for fila in filas:
            ref = fila[0]
            tolerancia = max(ref.alto, linea.alto) * 0.5
            if abs(ref.centro_y - linea.centro_y) <= tolerancia:
                fila.append(linea)
                break
        else:
            filas.append([linea])
    return [sorted(f, key=lambda l: l.x) for f in filas]


def texto_fila(fila: list[Linea]) -> str:
    return " ".join(l.texto for l in fila)
