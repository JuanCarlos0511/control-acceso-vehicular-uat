"""Datos ficticios para las pruebas. Ninguno corresponde a una persona real."""

from __future__ import annotations

import cv2
import numpy as np

from app.curp import digito_curp
from app.mrz import digito_control


def curp_valida(base17: str = "PELM020315MTSRPRA") -> str:
    return base17 + str(digito_curp(base17))


def mrz_ine(
    cic: str = "123456789",
    ocr: str = "0123456789012",
    nacimiento: str = "020315",
    sexo: str = "F",
    expiracion: str = "331231",
    apellidos: str = "PEREZ<LOPEZ",
    nombres: str = "MARIA<FERNANDA",
) -> tuple[str, str, str]:
    l1 = f"IDMEX{cic}{digito_control(cic)}<<{ocr}"
    l2 = f"{nacimiento}{digito_control(nacimiento)}{sexo}{expiracion}{digito_control(expiracion)}MEX" + "<" * 11
    l2 += str(digito_control(l1[5:30] + l2[0:7] + l2[8:15] + l2[18:29]))
    l3 = f"{apellidos}<<{nombres}".ljust(30, "<")[:30]
    assert len(l1) == len(l2) == 30
    return l1, l2, l3


def imagen_con_texto(renglones: list[tuple[int, int, str]], ancho: int = 1100, alto: int = 700) -> bytes:
    """Dibuja texto negro sobre fondo claro y devuelve un JPEG."""
    img = np.full((alto, ancho, 3), 245, np.uint8)
    # Un poco de textura para que la nitidez no sea artificialmente nula.
    ruido = np.random.default_rng(7).integers(0, 12, (alto, ancho, 1), dtype=np.uint8)
    img = cv2.subtract(img, np.repeat(ruido, 3, axis=2))
    for x, y, texto in renglones:
        cv2.putText(img, texto, (x, y), cv2.FONT_HERSHEY_SIMPLEX, 0.9, (20, 20, 20), 2, cv2.LINE_AA)
    ok, datos = cv2.imencode(".jpg", img, [cv2.IMWRITE_JPEG_QUALITY, 92])
    assert ok
    return datos.tobytes()


def ine_frente(curp: str) -> bytes:
    return imagen_con_texto([
        (40, 50, "INSTITUTO NACIONAL ELECTORAL"),
        (40, 90, "MEXICO  CREDENCIAL PARA VOTAR"),
        (330, 150, "NOMBRE"),
        (330, 195, "PEREZ"),
        (330, 240, "LOPEZ"),
        (330, 285, "MARIA FERNANDA"),
        (330, 340, "DOMICILIO"),
        (330, 385, "C SOL 123 COL CENTRO 89000"),
        (330, 450, f"CURP {curp}"),
        (330, 500, "FECHA DE NACIMIENTO 15/03/2002"),
        (330, 550, "VIGENCIA 2023 - 2033"),
        (850, 195, "SEXO M"),
    ])


def ine_reverso(lineas_mrz: tuple[str, str, str]) -> bytes:
    l1, l2, l3 = lineas_mrz
    return imagen_con_texto([(40, 520, l1), (40, 575, l2), (40, 630, l3)], ancho=1000)


def tarjeta(niv: str, placa: str = "XYZ123A") -> bytes:
    return imagen_con_texto([
        (40, 50, "GOBIERNO DEL ESTADO DE TAMAULIPAS"),
        (40, 95, "TARJETA DE CIRCULACION"),
        (40, 160, f"PLACA {placa}"),
        (40, 210, f"NUMERO DE SERIE {niv}"),
        (40, 260, "MARCA NISSAN   LINEA VERSA"),
        (40, 310, "MODELO 2019   COLOR GRIS"),
        (40, 360, "PROPIETARIO PEREZ LOPEZ MARIA FERNANDA"),
        (40, 410, "VIGENCIA 2027"),
    ])
