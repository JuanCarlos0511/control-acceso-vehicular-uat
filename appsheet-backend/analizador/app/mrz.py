"""Zona de lectura mecánica (MRZ) del reverso de la INE, formato ICAO 9303 TD1 (3 × 30)."""

from __future__ import annotations

import re
from dataclasses import dataclass
from datetime import date

from .texto import a_digitos, a_letras, normalizar

LARGO = 30
PESOS = (7, 3, 1)

_L1 = re.compile(r"[I1L][D0O][A-Z<]{0,2}MEX")
_L2 = re.compile(r"[0-9OQDIZSBGT]{7}[MFHX<][0-9OQDIZSBGT]{7}MEX")


def valor(c: str) -> int:
    if c.isdigit():
        return int(c)
    if "A" <= c <= "Z":
        return ord(c) - 55
    return 0  # '<'


def digito_control(texto: str) -> int:
    return sum(valor(c) * PESOS[i % 3] for i, c in enumerate(texto)) % 10


def _control_ok(campo: str, digito: str) -> bool:
    return digito.isdigit() and digito_control(campo) == int(digito)


def limpiar(texto: str) -> str:
    t = normalizar(texto).replace(" ", "")
    for c in "«‹([{":
        t = t.replace(c, "<")
    return re.sub(r"[^A-Z0-9<]", "<", t)


def _ajustar(texto: str) -> str:
    return texto[:LARGO].ljust(LARGO, "<")


def _fecha(aammdd: str, hoy: date, futura: bool) -> date | None:
    if not re.fullmatch(r"\d{6}", aammdd):
        return None
    aa, mm, dd = int(aammdd[:2]), int(aammdd[2:4]), int(aammdd[4:])
    siglo = 2000
    if futura:
        if 2000 + aa > hoy.year + 30:
            siglo = 1900
    elif 2000 + aa > hoy.year:
        siglo = 1900
    try:
        return date(siglo + aa, mm, dd)
    except ValueError:
        return None


def _corregir_digitos(texto: str, posiciones: range) -> str:
    chars = list(texto)
    for i in posiciones:
        if chars[i] != "<":
            chars[i] = a_digitos(chars[i])
    return "".join(chars)


@dataclass
class MRZ:
    linea1: str
    linea2: str
    linea3: str | None
    documento: str
    nacimiento: date | None
    sexo: str
    expiracion: date | None
    apellidos: str | None
    nombres: str | None
    controles: dict[str, bool]

    @property
    def valida(self) -> bool:
        return self.controles["documento"] and self.controles["nacimiento"] and self.controles["expiracion"]

    def a_dict(self) -> dict:
        return {
            "valida": self.valida,
            "controles": self.controles,
            "fechaNacimiento": self.nacimiento.isoformat() if self.nacimiento else None,
            "fechaExpiracion": self.expiracion.isoformat() if self.expiracion else None,
            "sexo": self.sexo if self.sexo in "MFHX" else None,
            "apellidos": self.apellidos,
            "nombres": self.nombres,
        }


def interpretar(l1: str, l2: str, l3: str | None, hoy: date) -> MRZ:
    l1 = _ajustar(l1)
    l2 = _ajustar(l2)
    l1 = "ID" + l1[2:]
    l1 = _corregir_digitos(l1, range(5, 15))
    l1 = _corregir_digitos(l1, range(17, 30))
    l2 = _corregir_digitos(l2, range(0, 7))
    l2 = _corregir_digitos(l2, range(8, 15))
    l2 = _corregir_digitos(l2, range(29, 30))

    controles = {
        "documento": _control_ok(l1[5:14], l1[14]),
        "nacimiento": _control_ok(l2[0:6], l2[6]),
        "expiracion": _control_ok(l2[8:14], l2[14]),
        "compuesto": _control_ok(l1[5:30] + l2[0:7] + l2[8:15] + l2[18:29], l2[29]),
    }

    apellidos = nombres = None
    if l3:
        l3 = _ajustar(l3)
        partes = l3.split("<<", 1)
        apellidos = partes[0].replace("<", " ").strip() or None
        if len(partes) > 1:
            nombres = re.sub(r"\s+", " ", partes[1].replace("<", " ")).strip() or None

    return MRZ(
        linea1=l1,
        linea2=l2,
        linea3=l3,
        documento=l1[5:14],
        nacimiento=_fecha(l2[0:6], hoy, futura=False),
        sexo=l2[7],
        expiracion=_fecha(l2[8:14], hoy, futura=True),
        apellidos=apellidos,
        nombres=nombres,
        controles=controles,
    )


def buscar(textos: list[str], hoy: date) -> MRZ | None:
    """Busca las tres líneas de la MRZ entre los textos leídos (en orden de lectura)."""
    limpios = [limpiar(t) for t in textos]
    l1 = l2 = l3 = None
    i2 = -1
    for t in limpios:
        if (m := _L1.search(t)) and len(t) - m.start() >= 20:
            # Se reconstruye el prefijo para que el número de documento quede en la posición 6.
            l1 = "IDMEX" + t[m.end():]
            break
    for i, t in enumerate(limpios):
        if (m := _L2.search(t)) and len(t) - m.start() >= 20:
            l2 = t[m.start():]
            i2 = i
            break
    if l2 is None:
        return None
    if l1 is None:
        return None

    # La línea 3 son solo letras y '<', con '<<' entre apellidos y nombres; va después de la línea 2.
    candidatos3 = []
    for t in limpios[i2 + 1:] + limpios[:i2]:
        t3 = a_letras(t)
        if "<<" in t3 and re.fullmatch(r"[A-Z<]+", t3) and sum(c.isalpha() for c in t3) >= 4 and not t3.startswith("ID"):
            candidatos3.append(t3)
    l3 = candidatos3[0] if candidatos3 else None
    return interpretar(l1, l2, l3, hoy)
