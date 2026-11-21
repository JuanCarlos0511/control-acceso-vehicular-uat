"""Extracción de datos de la tarjeta de circulación."""

from __future__ import annotations

import re
from datetime import date

from . import vin as vin_mod
from .ocr import Linea, agrupar_filas, texto_fila
from .texto import alfanumerico, contiene_aprox

FRASES_FUERTES = ("TARJETA DE CIRCULACION",)
PALABRAS = (
    "PLACA", "SERIE", "NIV", "MOTOR", "MODELO", "MARCA", "PROPIETARIO", "VIGENCIA", "VEHICULO",
    "CILINDROS", "COMBUSTIBLE", "SECRETARIA", "FINANZAS", "GOBIERNO", "LINEA", "CLASE", "TIPO", "COLOR",
)
ETIQUETAS_NIV = ("NUMERO DE SERIE", "NO. DE SERIE", "NO DE SERIE", "SERIE", "NIV", "VIN", "N.I.V")
ETIQUETAS_PLACA = ("NUMERO DE PLACA", "NO. DE PLACA", "NO DE PLACA", "PLACAS", "PLACA")
MARCAS = (
    "ACURA", "ALFA ROMEO", "AUDI", "BAIC", "BAJAJ", "BMW", "BUICK", "BYD", "CADILLAC", "CHANGAN", "CHEVROLET",
    "CHIREY", "CHRYSLER", "CUPRA", "DODGE", "FIAT", "FORD", "GAC", "GEELY", "GMC", "HONDA", "HYUNDAI", "INFINITI",
    "ISUZU", "ITALIKA", "JAC", "JAGUAR", "JEEP", "JETOUR", "KIA", "LAND ROVER", "LINCOLN", "MAZDA",
    "MERCEDES BENZ", "MG", "MINI", "MITSUBISHI", "NISSAN", "OMODA", "PEUGEOT", "PORSCHE", "RAM", "RENAULT",
    "SEAT", "SUBARU", "SUZUKI", "TESLA", "TOYOTA", "VENTO", "VOLKSWAGEN", "VOLVO", "YAMAHA",
)
_ALIAS_MARCA = {"VW": "VOLKSWAGEN", "MERCEDES": "MERCEDES BENZ", "MERCEDES-BENZ": "MERCEDES BENZ", "GM": "CHEVROLET"}
_PLACA = re.compile(r"^(?=.*\d)(?=.*[A-Z])[A-Z0-9]{5,8}$")
_ANIO = re.compile(r"(?<!\d)(19[5-9]\d|20\d{2})(?!\d)")


def confianza_documento(textos: list[str]) -> float:
    unido = " | ".join(textos)
    puntos = sum(0.35 for f in FRASES_FUERTES if contiene_aprox(unido, f))
    puntos += min(0.55, 0.06 * sum(1 for p in PALABRAS if p in unido))
    return round(min(1.0, puntos), 2)


def _despues_de(texto: str, etiquetas: tuple[str, ...]) -> str | None:
    for etiqueta in etiquetas:
        indice = texto.find(etiqueta)
        if indice >= 0:
            return texto[indice + len(etiqueta):]
    return None


def _fila_siguiente(filas: list[list[Linea]], i: int) -> str:
    return texto_fila(filas[i + 1]) if i + 1 < len(filas) else ""


def candidatos_niv(filas: list[list[Linea]], lineas: list[Linea]) -> list[dict]:
    vistos: dict[str, dict] = {}

    def agregar(valor: str, etiquetado: bool, confianza: float) -> None:
        if not vin_mod.formato_valido(valor):
            return
        previo = vistos.get(valor)
        if previo is None or (etiquetado and not previo["etiquetado"]):
            vistos[valor] = {
                "valor": valor,
                "etiquetado": etiquetado,
                "digitoValido": vin_mod.digito_valido(valor),
                "confianza": round(confianza, 3),
            }

    for i, fila in enumerate(filas):
        texto = texto_fila(fila)
        resto = _despues_de(texto, ETIQUETAS_NIV)
        if resto is None:
            continue
        confianza = min(l.confianza for l in fila)
        for fuente in (resto, _fila_siguiente(filas, i)):
            unido = vin_mod.normalizar(fuente)
            for v in vin_mod.ventanas(unido):
                agregar(v, True, confianza)
            for parte in fuente.split(" "):
                agregar(vin_mod.normalizar(parte), True, confianza)

    for linea in lineas:
        for parte in linea.texto.replace(":", " ").split(" "):
            if len(alfanumerico(parte)) == 17:
                agregar(vin_mod.normalizar(parte), False, linea.confianza)

    return sorted(vistos.values(), key=lambda c: (c["etiquetado"], c["digitoValido"], c["confianza"]), reverse=True)


def candidatos_placa(filas: list[list[Linea]]) -> list[dict]:
    vistos: dict[str, dict] = {}
    for i, fila in enumerate(filas):
        texto = texto_fila(fila)
        resto = _despues_de(texto, ETIQUETAS_PLACA)
        if resto is None:
            continue
        confianza = min(l.confianza for l in fila)
        for fuente in (resto, _fila_siguiente(filas, i)):
            fuente = fuente.strip(" :.")
            # Se prueban el primer fragmento, los dos primeros unidos (ABC 123 D) y todo el texto.
            partes = [p for p in re.split(r"\s+", fuente) if p]
            opciones = [alfanumerico(p) for p in partes[:3]]
            if len(partes) >= 2:
                opciones.append(alfanumerico("".join(partes[:2])))
            if len(partes) >= 3:
                opciones.append(alfanumerico("".join(partes[:3])))
            for opcion in opciones:
                if _PLACA.match(opcion) and opcion not in vistos:
                    vistos[opcion] = {"valor": opcion, "etiquetado": True, "confianza": round(confianza, 3)}
            if vistos:
                break
    return list(vistos.values())


def extraer_marca(textos: list[str]) -> str | None:
    unido = " " + " | ".join(textos) + " "
    for marca in MARCAS:
        if re.search(rf"(?<![A-Z]){re.escape(marca)}(?![A-Z])", unido):
            return marca
    for alias, marca in _ALIAS_MARCA.items():
        if re.search(rf"(?<![A-Z]){re.escape(alias)}(?![A-Z])", unido):
            return marca
    return None


def extraer_anio_modelo(filas: list[list[Linea]], hoy: date) -> int | None:
    for i, fila in enumerate(filas):
        texto = texto_fila(fila)
        resto = _despues_de(texto, ("ANO MODELO", "MODELO"))
        if resto is None:
            continue
        for fuente in (resto, _fila_siguiente(filas, i)):
            anios = [int(a) for a in _ANIO.findall(fuente) if 1950 <= int(a) <= hoy.year + 1]
            if anios:
                return anios[0]
    return None


def extraer_vigencia(filas: list[list[Linea]], hoy: date) -> dict:
    for i, fila in enumerate(filas):
        texto = texto_fila(fila)
        resto = _despues_de(texto, ("VIGENCIA", "VIGENTE HASTA", "VALIDA HASTA"))
        if resto is None:
            continue
        fuente = resto + " " + _fila_siguiente(filas, i)
        if re.search(r"INDEFINID|PERMANENTE", fuente):
            return {"anio": None, "indefinida": True}
        anios = [int(a) for a in _ANIO.findall(fuente) if 2000 <= int(a) <= hoy.year + 15]
        if anios:
            return {"anio": max(anios), "indefinida": False}
    return {"anio": None, "indefinida": False}


def analizar(lineas: list[Linea], hoy: date) -> dict:
    filas = agrupar_filas(lineas)
    textos = [l.texto for l in lineas] + [texto_fila(f) for f in filas]
    nivs = candidatos_niv(filas, lineas)
    placas = candidatos_placa(filas)
    confianza = confianza_documento(textos)

    advertencias = []
    if not nivs:
        advertencias.append("NIV_NO_LEIDO")
    elif not nivs[0]["digitoValido"]:
        advertencias.append("NIV_DIGITO_VERIFICADOR_NO_COINCIDE")
    if not placas:
        advertencias.append("PLACA_NO_LEIDA")

    return {
        "esTarjeta": confianza >= 0.3,
        "confianzaDocumento": confianza,
        "niv": {"valor": nivs[0]["valor"] if nivs else None, "candidatos": nivs[:5]},
        "placa": {"valor": placas[0]["valor"] if placas else None, "candidatos": placas[:5]},
        "marca": extraer_marca(textos),
        "anioModelo": extraer_anio_modelo(filas, hoy),
        "vigencia": extraer_vigencia(filas, hoy),
        "advertencias": advertencias,
    }
