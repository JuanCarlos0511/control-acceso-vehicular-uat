"""Normalización de texto leído por OCR en documentos mexicanos."""

from __future__ import annotations

import re
import unicodedata
from difflib import SequenceMatcher

# Confusiones típicas del OCR, según el tipo de carácter que se espera en la posición.
A_DIGITO = str.maketrans({
    "O": "0", "Q": "0", "D": "0", "U": "0",
    "I": "1", "L": "1", "J": "1",
    "Z": "2", "S": "5", "B": "8", "G": "6", "T": "7", "A": "4",
})
A_LETRA = str.maketrans({
    "0": "O", "1": "I", "2": "Z", "4": "A", "5": "S", "6": "G", "7": "T", "8": "B",
})


def sin_acentos(texto: str) -> str:
    descompuesto = unicodedata.normalize("NFKD", texto)
    return "".join(c for c in descompuesto if not unicodedata.combining(c))


def normalizar(texto: str) -> str:
    """Mayúsculas, sin acentos (Ñ → N) y con espacios simples."""
    t = sin_acentos(texto or "").upper()
    t = re.sub(r"\s+", " ", t)
    return t.strip()


def alfanumerico(texto: str) -> str:
    return re.sub(r"[^A-Z0-9]", "", normalizar(texto))


def solo_letras(texto: str) -> str:
    t = re.sub(r"[^A-Z ]", " ", normalizar(texto))
    return re.sub(r"\s+", " ", t).strip()


def a_digitos(texto: str) -> str:
    return texto.translate(A_DIGITO)


def a_letras(texto: str) -> str:
    return texto.translate(A_LETRA)


def contiene_aprox(texto: str, frase: str, umbral: float = 0.84) -> bool:
    """True si `frase` aparece en `texto`, tolerando errores leves de OCR."""
    if frase in texto:
        return True
    n = len(frase)
    if n < 5 or len(texto) < n - 2:
        return False
    mejor = 0.0
    for inicio in range(0, max(1, len(texto) - n + 1)):
        ventana = texto[inicio:inicio + n]
        mejor = max(mejor, SequenceMatcher(None, ventana, frase).ratio())
        if mejor >= umbral:
            return True
    return False


def similitud(a: str, b: str) -> float:
    if not a or not b:
        return 0.0
    return SequenceMatcher(None, a, b).ratio()
