"""Extracción y verificación de datos de la credencial para votar (INE)."""

from __future__ import annotations

import re
from datetime import date

from . import curp as curp_mod
from . import mrz as mrz_mod
from .imagen import es_url_ine
from .ocr import Linea, agrupar_filas, texto_fila
from .texto import alfanumerico, contiene_aprox, similitud, solo_letras

FRASES_FUERTES = ("INSTITUTO NACIONAL ELECTORAL", "CREDENCIAL PARA VOTAR")
PALABRAS = (
    "NOMBRE", "DOMICILIO", "CLAVE DE ELECTOR", "CURP", "VIGENCIA", "SECCION",
    "FECHA DE NACIMIENTO", "ANO DE REGISTRO", "SEXO", "MEXICO", "ESTADO", "MUNICIPIO", "LOCALIDAD", "EMISION",
)
ETIQUETAS = (
    "DOMICILIO", "SEXO", "FECHA", "CLAVE", "CURP", "ANO DE", "REGISTRO", "SECCION",
    "VIGENCIA", "EMISION", "ESTADO", "MUNICIPIO", "LOCALIDAD", "EDAD", "NACIMIENTO",
)
_FECHA = re.compile(r"\b(\d{2})/(\d{2})/(\d{4})\b")
_ANIO = re.compile(r"(?<!\d)(19\d{2}|20\d{2})(?!\d)")


def confianza_documento(textos: list[str]) -> float:
    unido = " | ".join(textos)
    puntos = sum(0.3 for f in FRASES_FUERTES if contiene_aprox(unido, f))
    puntos += min(0.4, 0.06 * sum(1 for p in PALABRAS if p in unido))
    return round(min(1.0, puntos), 2)


def tokens(lineas: list[Linea]) -> list[str]:
    """Fragmentos alfanuméricos para buscar CURP y clave de elector."""
    salida: list[str] = []
    for linea in lineas:
        partes = [alfanumerico(p) for p in linea.texto.split(" ")]
        salida.extend(p for p in partes if p)
        salida.append(alfanumerico(linea.texto))
        for etiqueta in ("CURP", "CLAVEDEELECTOR", "CLAVEELECTOR"):
            unido = alfanumerico(linea.texto)
            if unido.startswith(etiqueta):
                salida.append(unido[len(etiqueta):])
    return salida


def _es_etiqueta(texto: str) -> bool:
    return any(texto.startswith(e) or f" {e}" in texto for e in ETIQUETAS)


def extraer_nombre(lineas: list[Linea], ancho_imagen: float) -> dict | None:
    """El nombre va debajo de la etiqueta NOMBRE: apellido paterno, materno y nombres."""
    etiqueta = next((l for l in lineas if l.texto == "NOMBRE" or l.texto.startswith("NOMBRE ")), None)
    if etiqueta is None:
        return None
    partes: list[tuple[str, float]] = []
    resto = solo_letras(etiqueta.texto[len("NOMBRE"):])
    if resto:
        partes.append((resto, etiqueta.confianza))

    tolerancia_x = max(0.12 * ancho_imagen, 2.5 * etiqueta.alto)
    limite_y = etiqueta.y + etiqueta.alto * 8
    debajo = sorted(
        (l for l in lineas
         if etiqueta.centro_y + etiqueta.alto * 0.4 < l.centro_y < limite_y and abs(l.x - etiqueta.x) <= tolerancia_x),
        key=lambda l: l.y,
    )
    for linea in debajo:
        if _es_etiqueta(linea.texto):
            break
        texto = solo_letras(linea.texto)
        if len(texto.replace(" ", "")) < 2:
            continue
        partes.append((texto, linea.confianza))
        if len(partes) == 3:
            break
    if not partes:
        return None

    textos = [p[0] for p in partes]
    if len(textos) == 3:
        paterno, materno, nombres = textos
    elif len(textos) == 2:
        paterno, materno, nombres = textos[0], None, textos[1]
    else:
        paterno, materno, nombres = None, None, textos[0]
    return {
        "apellidoPaterno": paterno,
        "apellidoMaterno": materno,
        "nombres": nombres,
        "completo": " ".join(textos),
        "confianza": round(min(p[1] for p in partes), 3),
    }


def extraer_vigencia(filas: list[list[Linea]], hoy: date) -> dict | None:
    for i, fila in enumerate(filas):
        texto = texto_fila(fila)
        indice = texto.find("VIGENCIA")
        if indice < 0:
            continue
        anios = [int(a) for a in _ANIO.findall(texto[indice:])]
        confianza = min(l.confianza for l in fila)
        if not anios and i + 1 < len(filas):
            siguiente = filas[i + 1]
            anios = [int(a) for a in _ANIO.findall(texto_fila(siguiente))]
            confianza = min(confianza, *(l.confianza for l in siguiente))
        anios = [a for a in anios if 2000 <= a <= hoy.year + 15]
        if anios:
            return {"anio": max(anios), "confianza": round(confianza, 3)}
    return None


def extraer_fecha_nacimiento(textos: list[str]) -> date | None:
    for texto in textos:
        for dd, mm, aaaa in _FECHA.findall(texto):
            try:
                return date(int(aaaa), int(mm), int(dd))
            except ValueError:
                continue
    return None


def extraer_sexo(filas: list[list[Linea]]) -> str | None:
    for fila in filas:
        texto = texto_fila(fila)
        if m := re.search(r"SEXO\s*:?\s*([HM])\b", texto):
            return m.group(1)
    return None


def _similitud_nombres(a: str, b: str) -> float:
    return round(similitud(" ".join(sorted(a.split())), " ".join(sorted(b.split()))), 3)


def analizar(
    frente: list[Linea],
    ancho_frente: float,
    reverso: list[Linea] | None,
    qrs: list[str],
    hoy: date,
) -> dict:
    reverso = reverso or []
    todas = frente + reverso
    filas_frente = agrupar_filas(frente)
    filas_todas = filas_frente + agrupar_filas(reverso)
    textos = [l.texto for l in todas] + [texto_fila(f) for f in filas_todas]

    advertencias: list[str] = []
    nombre = extraer_nombre(frente, ancho_frente)
    if nombre is None and reverso:
        # La foto del frente pudo subirse en el campo del reverso.
        nombre = extraer_nombre(reverso, ancho_frente)

    curp = curp_mod.buscar_curp(tokens(todas))
    clave = curp_mod.buscar_clave_elector(tokens(todas))
    vigencia = extraer_vigencia(filas_todas, hoy)
    nacimiento = extraer_fecha_nacimiento(textos)
    sexo = extraer_sexo(filas_todas)
    mrz = mrz_mod.buscar([l.texto for l in reverso] + [texto_fila(f) for f in agrupar_filas(reverso)]
                         + [l.texto for l in frente], hoy)

    if nombre is None:
        advertencias.append("NOMBRE_NO_LEIDO")
    if curp is None:
        advertencias.append("CURP_NO_LEIDA")
    elif not curp.digito_valido:
        advertencias.append("CURP_DIGITO_INVALIDO")
    if not reverso:
        advertencias.append("SIN_REVERSO")
    if mrz is None:
        advertencias.append("MRZ_NO_LEIDA")
    elif not mrz.valida:
        advertencias.append("MRZ_DIGITOS_INVALIDOS")

    consistencia = {
        "nacimientoCurpVsMrz": None,
        "nacimientoCurpVsFrente": None,
        "nombreFrenteVsMrz": None,
        "vigenciaFrenteVsMrz": None,
    }
    if curp and curp.digito_valido and mrz and mrz.controles["nacimiento"] and mrz.nacimiento:
        consistencia["nacimientoCurpVsMrz"] = curp.nacimiento == mrz.nacimiento
    if curp and curp.formato_valido and nacimiento:
        consistencia["nacimientoCurpVsFrente"] = curp.nacimiento == nacimiento
    if nombre and mrz and mrz.apellidos:
        nombre_mrz = f"{mrz.apellidos} {mrz.nombres or ''}".strip()
        consistencia["nombreFrenteVsMrz"] = _similitud_nombres(nombre["completo"], nombre_mrz)
    if vigencia and mrz and mrz.controles["expiracion"] and mrz.expiracion:
        consistencia["vigenciaFrenteVsMrz"] = vigencia["anio"] == mrz.expiracion.year

    confianza = confianza_documento(textos)
    return {
        "esINE": confianza >= 0.35 or (mrz is not None and mrz.valida),
        "confianzaDocumento": confianza,
        "nombre": nombre,
        "curp": curp.a_dict() if curp else None,
        "claveElector": clave,
        "fechaNacimiento": nacimiento.isoformat() if nacimiento else None,
        "sexo": sexo,
        "vigencia": vigencia,
        "mrz": mrz.a_dict() if mrz else None,
        "qr": {"detectados": len(qrs), "urlsINE": sum(1 for q in qrs if es_url_ine(q))},
        "consistencia": consistencia,
        "advertencias": advertencias,
    }
