"""NIV / VIN de 17 caracteres (ISO 3779) con dígito verificador (posición 9)."""

from __future__ import annotations

import re

PATRON = re.compile(r"^[A-HJ-NPR-Z0-9]{17}$")
_VENTANA = re.compile(r"(?=([A-HJ-NPR-Z0-9]{17}))")
_VALORES = {
    **{str(d): d for d in range(10)},
    "A": 1, "B": 2, "C": 3, "D": 4, "E": 5, "F": 6, "G": 7, "H": 8,
    "J": 1, "K": 2, "L": 3, "M": 4, "N": 5, "P": 7, "R": 9,
    "S": 2, "T": 3, "U": 4, "V": 5, "W": 6, "X": 7, "Y": 8, "Z": 9,
}
_PESOS = (8, 7, 6, 5, 4, 3, 2, 10, 0, 9, 8, 7, 6, 5, 4, 3, 2)


def normalizar(texto: str) -> str:
    """El NIV no usa I, O ni Q: el OCR casi siempre las confunde con 1 y 0."""
    t = re.sub(r"[^A-Z0-9]", "", texto.upper())
    return t.replace("I", "1").replace("O", "0").replace("Q", "0")


def formato_valido(vin: str) -> bool:
    return bool(PATRON.match(vin))


def digito_verificador(vin: str) -> str:
    suma = sum(_VALORES[c] * p for c, p in zip(vin, _PESOS))
    resto = suma % 11
    return "X" if resto == 10 else str(resto)


def digito_valido(vin: str) -> bool:
    return formato_valido(vin) and digito_verificador(vin) == vin[8]


def ventanas(texto: str) -> list[str]:
    """Todas las subcadenas de 17 caracteres válidos dentro de un texto alfanumérico."""
    return [m.group(1) for m in _VENTANA.finditer(texto)]
