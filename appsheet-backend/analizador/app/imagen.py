"""Decodificación, preparación y métricas de calidad de las fotos."""

from __future__ import annotations

import os
from dataclasses import dataclass, field
from urllib.parse import urlparse

import cv2
import numpy as np

MAX_PIXELES = 40_000_000
LADO_MAX = 1800
LADO_MIN = 1000

UMBRAL_NITIDEZ = 60.0
UMBRAL_OSCURA = 50.0
UMBRAL_SOBREEXPUESTA = 240.0
LADO_MIN_ACEPTABLE = 480


class ImagenInvalida(ValueError):
    pass


@dataclass
class Calidad:
    ancho: int
    alto: int
    nitidez: float
    brillo: float
    rostro_detectado: bool | None = None
    problemas: list[str] = field(default_factory=list)

    @property
    def aceptable(self) -> bool:
        return not self.problemas

    def a_dict(self) -> dict:
        return {
            "ancho": self.ancho,
            "alto": self.alto,
            "nitidez": round(self.nitidez, 1),
            "brillo": round(self.brillo, 1),
            "rostroDetectado": self.rostro_detectado,
            "aceptable": self.aceptable,
            "problemas": self.problemas,
        }


def decodificar(datos: bytes) -> np.ndarray:
    """Decodifica JPEG/PNG/WebP y aplica la orientación EXIF."""
    if not datos:
        raise ImagenInvalida("El archivo está vacío.")
    arreglo = np.frombuffer(datos, dtype=np.uint8)
    imagen = cv2.imdecode(arreglo, cv2.IMREAD_COLOR)
    if imagen is None:
        raise ImagenInvalida("El archivo no es una imagen JPEG, PNG o WebP válida.")
    alto, ancho = imagen.shape[:2]
    if alto * ancho > MAX_PIXELES:
        raise ImagenInvalida("La imagen tiene demasiados píxeles.")
    return imagen


def preparar(imagen: np.ndarray) -> np.ndarray:
    """Escala la imagen para que el OCR trabaje en un tamaño razonable."""
    alto, ancho = imagen.shape[:2]
    lado = max(alto, ancho)
    if lado > LADO_MAX:
        factor = LADO_MAX / lado
        return cv2.resize(imagen, (int(ancho * factor), int(alto * factor)), interpolation=cv2.INTER_AREA)
    if lado < LADO_MIN:
        factor = LADO_MIN / lado
        return cv2.resize(imagen, (int(ancho * factor), int(alto * factor)), interpolation=cv2.INTER_CUBIC)
    return imagen


def rotar(imagen: np.ndarray, grados: int) -> np.ndarray:
    if grados == 90:
        return cv2.rotate(imagen, cv2.ROTATE_90_CLOCKWISE)
    if grados == 180:
        return cv2.rotate(imagen, cv2.ROTATE_180)
    if grados == 270:
        return cv2.rotate(imagen, cv2.ROTATE_90_COUNTERCLOCKWISE)
    return imagen


def medir_calidad(imagen: np.ndarray, detector_rostros: "DetectorRostros | None" = None) -> Calidad:
    alto, ancho = imagen.shape[:2]
    gris = cv2.cvtColor(imagen, cv2.COLOR_BGR2GRAY)
    # La nitidez se mide a un ancho fijo para que no dependa de la resolución de la cámara.
    escala = 1000 / max(ancho, 1)
    gris_norm = cv2.resize(gris, (1000, max(1, int(alto * escala))), interpolation=cv2.INTER_AREA)
    nitidez = float(cv2.Laplacian(gris_norm, cv2.CV_64F).var())
    brillo = float(gris.mean())

    problemas = []
    if min(alto, ancho) < LADO_MIN_ACEPTABLE:
        problemas.append("RESOLUCION_BAJA")
    if nitidez < UMBRAL_NITIDEZ:
        problemas.append("BORROSA")
    if brillo < UMBRAL_OSCURA:
        problemas.append("OSCURA")
    elif brillo > UMBRAL_SOBREEXPUESTA:
        problemas.append("SOBREEXPUESTA")

    rostro = detector_rostros.hay_rostro(imagen) if detector_rostros else None
    return Calidad(ancho, alto, nitidez, brillo, rostro, problemas)


def leer_qr(imagen: np.ndarray) -> list[str]:
    detector = cv2.QRCodeDetector()
    try:
        ok, textos, _, _ = detector.detectAndDecodeMulti(imagen)
    except cv2.error:
        return []
    if not ok:
        return []
    return [t for t in textos if t]


def es_url_ine(texto: str) -> bool:
    try:
        url = urlparse(texto.strip())
    except ValueError:
        return False
    host = (url.hostname or "").lower()
    return url.scheme in ("http", "https") and (host == "ine.mx" or host.endswith(".ine.mx"))


class DetectorRostros:
    """Detector YuNet opcional. Solo se activa si existe el modelo ONNX configurado."""

    def __init__(self, ruta_modelo: str) -> None:
        self._ruta = ruta_modelo

    @classmethod
    def desde_entorno(cls) -> "DetectorRostros | None":
        ruta = os.environ.get("ANALIZADOR_MODELO_ROSTRO", "")
        if ruta and os.path.isfile(ruta) and hasattr(cv2, "FaceDetectorYN"):
            return cls(ruta)
        return None

    def hay_rostro(self, imagen: np.ndarray) -> bool:
        alto, ancho = imagen.shape[:2]
        detector = cv2.FaceDetectorYN.create(self._ruta, "", (ancho, alto), 0.7)
        _, rostros = detector.detect(imagen)
        return rostros is not None and len(rostros) > 0
