"""Validación de CURP y de clave de elector."""

from __future__ import annotations

import re
from dataclasses import dataclass
from datetime import date

from .texto import a_digitos, a_letras

ESTADOS = (
    "AS BC BS CC CL CM CS CH DF DG GT GR HG JC MC MN MS NT NL OC PL QT QR SP SL SR TC TS TL VZ YN ZS NE"
).split()

PATRON_CURP = re.compile(
    r"^[A-Z][AEIOUX][A-Z]{2}\d{2}(0[1-9]|1[0-2])(0[1-9]|[12]\d|3[01])[HMX]"
    r"(" + "|".join(ESTADOS) + r")[B-DF-HJ-NP-TV-Z]{3}[0-9A-Z]\d$"
)
DICCIONARIO = "0123456789ABCDEFGHIJKLMNÑOPQRSTUVWXYZ"

# Tipo de carácter esperado en cada posición: L letra, D dígito, A alfanumérico.
_TIPOS_CURP = "LLLLDDDDDDLLLLLLAD"
PATRON_CLAVE_ELECTOR = re.compile(r"^[A-Z]{6}\d{8}[HM]\d{3}$")
_TIPOS_CLAVE = "LLLLLLDDDDDDDDLDDD"


def digito_curp(curp17: str) -> int:
    suma = sum(DICCIONARIO.index(c) * (18 - i) for i, c in enumerate(curp17))
    return (10 - suma % 10) % 10


def _corregir(texto: str, tipos: str) -> str:
    salida = []
    for c, t in zip(texto, tipos):
        if t == "L":
            salida.append(a_letras(c))
        elif t == "D":
            salida.append(a_digitos(c))
        else:
            salida.append(c)
    return "".join(salida)


@dataclass
class CURP:
    valor: str
    formato_valido: bool
    digito_valido: bool
    nacimiento: date | None
    sexo: str | None

    def a_dict(self) -> dict:
        return {
            "valor": self.valor,
            "formatoValido": self.formato_valido,
            "digitoValido": self.digito_valido,
            "fechaNacimiento": self.nacimiento.isoformat() if self.nacimiento else None,
            "sexo": self.sexo,
        }


def analizar_curp(texto: str) -> CURP:
    valor = _corregir(texto, _TIPOS_CURP) if len(texto) == 18 else texto
    formato = bool(PATRON_CURP.match(valor))
    digito = formato and digito_curp(valor[:17]) == int(valor[17])
    nacimiento = None
    if formato:
        siglo = 1900 if valor[16].isdigit() else 2000
        try:
            nacimiento = date(siglo + int(valor[4:6]), int(valor[6:8]), int(valor[8:10]))
        except ValueError:
            formato = digito = False
    return CURP(valor, formato, digito, nacimiento, valor[10] if formato else None)


def buscar_curp(tokens: list[str]) -> CURP | None:
    """Elige el mejor candidato de 18 caracteres: primero el que pasa el dígito verificador."""
    candidatos = [analizar_curp(t) for t in tokens if len(t) == 18]
    validos = [c for c in candidatos if c.formato_valido]
    for c in validos:
        if c.digito_valido:
            return c
    return validos[0] if validos else None


def buscar_clave_elector(tokens: list[str]) -> dict | None:
    for t in tokens:
        if len(t) != 18:
            continue
        valor = _corregir(t, _TIPOS_CLAVE)
        if PATRON_CLAVE_ELECTOR.match(valor):
            return {"valor": valor, "formatoValido": True}
    return None
