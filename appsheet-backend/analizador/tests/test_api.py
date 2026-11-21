"""Pruebas de punta a punta: imágenes sintéticas → OCR real (RapidOCR) → JSON."""

import pytest
from fastapi.testclient import TestClient

from app.main import app
from tests.utilidades import curp_valida, imagen_con_texto, ine_frente, ine_reverso, mrz_ine, tarjeta

NIV = "1HGCM82633A004352"
cliente = TestClient(app)


def test_salud():
    r = cliente.get("/salud")
    assert r.status_code == 200
    assert r.json()["ok"] is True


def test_ine_completa():
    valor_curp = curp_valida()
    archivos = {
        "frente": ("frente.jpg", ine_frente(valor_curp), "image/jpeg"),
        "reverso": ("reverso.jpg", ine_reverso(mrz_ine()), "image/jpeg"),
    }
    r = cliente.post("/v1/ine", files=archivos)
    assert r.status_code == 200, r.text
    datos = r.json()
    assert datos["esINE"]
    assert datos["mrz"]["valida"], datos
    assert datos["mrz"]["apellidos"] == "PEREZ LOPEZ"
    assert datos["curp"]["valor"] == valor_curp
    assert datos["curp"]["digitoValido"]
    assert datos["nombre"]["completo"] == "PEREZ LOPEZ MARIA FERNANDA"
    assert datos["vigencia"]["anio"] == 2033
    assert datos["consistencia"]["nacimientoCurpVsMrz"] is True
    assert datos["calidad"]["frente"]["ancho"] > 0


def test_tarjeta_completa():
    r = cliente.post("/v1/tarjeta", files={"imagen": ("t.jpg", tarjeta(NIV), "image/jpeg")})
    assert r.status_code == 200, r.text
    datos = r.json()
    assert datos["esTarjeta"]
    assert datos["niv"]["valor"] == NIV
    assert datos["placa"]["valor"] == "XYZ123A"
    assert datos["marca"] == "NISSAN"
    assert datos["anioModelo"] == 2019


def test_tarjeta_girada():
    import cv2
    import numpy as np

    datos = np.frombuffer(tarjeta(NIV), np.uint8)
    img = cv2.rotate(cv2.imdecode(datos, cv2.IMREAD_COLOR), cv2.ROTATE_90_CLOCKWISE)
    ok, girada = cv2.imencode(".jpg", img)
    assert ok
    r = cliente.post("/v1/tarjeta", files={"imagen": ("t.jpg", girada.tobytes(), "image/jpeg")})
    assert r.status_code == 200
    assert r.json()["niv"]["valor"] == NIV


def test_imagen_invalida():
    r = cliente.post("/v1/tarjeta", files={"imagen": ("t.jpg", b"no es imagen", "image/jpeg")})
    assert r.status_code == 422
    assert r.json()["error"]["codigo"] == "IMAGEN_INVALIDA"


def test_token(monkeypatch):
    import app.main as principal

    monkeypatch.setattr(principal, "TOKEN", "secreto")
    img = imagen_con_texto([(10, 50, "HOLA")])
    r = cliente.post("/v1/tarjeta", files={"imagen": ("t.jpg", img, "image/jpeg")})
    assert r.status_code == 401
    r = cliente.post("/v1/tarjeta", files={"imagen": ("t.jpg", img, "image/jpeg")},
                     headers={"X-Token-Analizador": "secreto"})
    assert r.status_code == 200


@pytest.mark.parametrize("campo", ["frente"])
def test_falta_archivo(campo):
    r = cliente.post("/v1/ine", files={})
    assert r.status_code == 422
