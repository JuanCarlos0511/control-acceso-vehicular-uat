import asyncio
import json
from datetime import datetime, timezone

import pytest
from fastapi.testclient import TestClient

from app import repuve
from app.repuve import Captura, interpretar
from tests.portal_falso import NIV_NO_EXISTE, NIV_RECHAZADO, NIV_ROBADO, NIV_SIN_REPORTE, PortalFalso

AHORA = datetime(2026, 11, 21, 10, 0, tzinfo=timezone.utc)
NIV = "1HGCM82633A004352"
VEHICULO = json.dumps({"datos": [{"PLACA": "XYZ123A", "VIN": NIV, "MARCA": "NISSAN", "MODELO": "VERSA",
                                  "ANIO_MODELO": 2019, "CLASE": "AUTOMOVIL", "NOMBRE": "NO SE GUARDA"}]}).encode()


def ok(cuerpo) -> Captura:
    return Captura(True, 200, cuerpo if isinstance(cuerpo, bytes) else json.dumps(cuerpo).encode())


# ---------- Interpretación ----------

@pytest.mark.parametrize("capturas, estatus, reportes", [
    ({"repuve": ok(VEHICULO), "pgj": ok({"d": []}), "ocra": ok({"conReporteRoboRecuperacion": "false"}), "aviso": ok({"d": []})},
     repuve.SIN_REPORTE, 0),
    ({"repuve": ok(VEHICULO), "pgj": ok({"d": [{"ESTATUS_VHI_ROBO": "ROBADO", "FTE_VHI_ROBO": "TAMAULIPAS"}]})},
     repuve.CON_REPORTE, 1),
    ({"repuve": ok(VEHICULO), "pgj": ok({"d": [{"ESTATUS_VHI_ROBO": "ROBADO", "FTE_RECUPERA": "NL"}]})},
     repuve.RECUPERADO, 1),
    ({"repuve": ok(VEHICULO), "pgj": ok({"d": []}),
      "ocra": ok({"conReporteRoboRecuperacion": "true", "reporte": {"estatus": "ROBADO", "roboORecuperacion": 1}})},
     repuve.CON_REPORTE, 1),
    ({"repuve": ok(VEHICULO), "pgj": ok({"d": []}), "aviso": ok({"d": [{"DELITO": "FRAUDE"}]})}, repuve.CON_REPORTE, 1),
    ({"repuve": ok(VEHICULO), "pgj": ok({"d": [{"ESTATUS_VHI_ROBO": "NO ROBADO"}]})}, repuve.SIN_REPORTE, 0),
    ({"repuve": ok({"datos": []})}, repuve.NO_ENCONTRADO, 0),
    ({"repuve": ok(VEHICULO)}, repuve.NO_DISPONIBLE, 0),
    ({"repuve": Captura(True, 401, b'{"statusCode":401,"message":"Token invalido"}')}, repuve.NO_DISPONIBLE, 0),
    ({}, repuve.NO_DISPONIBLE, 0),
    ({"series": ok(VEHICULO), "pgj": ok([])}, repuve.SIN_REPORTE, 0),
])
def test_interpretar(capturas, estatus, reportes):
    r = interpretar(capturas, repuve.CRITERIO_NIV, NIV, AHORA)
    assert r["estatus"] == estatus, r
    assert len(r["reportes"]) == reportes


def test_vehiculo_sin_datos_personales():
    r = interpretar({"repuve": ok(VEHICULO), "pgj": ok([])}, repuve.CRITERIO_NIV, NIV, AHORA)
    assert r["vehiculo"] == {"placa": "XYZ123A", "niv": NIV, "marca": "NISSAN", "modelo": "VERSA", "linea": "",
                             "anioModelo": "2019", "clase": "AUTOMOVIL", "tipo": "", "entidad": ""}
    assert "NO SE GUARDA" not in json.dumps(r)
    assert r["consultadoEn"] == "2026-11-21T10:00:00+00:00"


def test_servicio_de():
    assert repuve.servicio_de("https://www2.repuve.gob.mx:8443/consulta/consulta/pgj") == "pgj"
    assert repuve.servicio_de("https://www2.repuve.gob.mx:8443/consulta/consulta/pdf") is None
    assert repuve.servicio_de("https://www2.repuve.gob.mx:8443/ciudadania/main.js") is None


def test_normalizar_consulta():
    assert repuve.normalizar_consulta("1hgcm8263-3a004352", None) == (repuve.CRITERIO_NIV, NIV)
    assert repuve.normalizar_consulta(None, "xyz-123-a") == (repuve.CRITERIO_PLACA, "XYZ123A")
    for niv, placa in [("123", None), ("1HGCM82633A0O4352", None), (None, "AB1"), (None, None)]:
        with pytest.raises(ValueError):
            repuve.normalizar_consulta(niv, placa)


def test_duracion():
    assert repuve._duracion("20s", 0) == 20
    assert repuve._duracion("2m", 0) == 120
    assert repuve._duracion("6h", 0) == 21600
    assert repuve._duracion("", 7) == 7


# ---------- Endpoint ----------

def test_endpoint_valida_y_responde(monkeypatch):
    import app.main as principal

    class Falso:
        o = repuve.Opciones()

        async def consultar(self, criterio, valor):
            return repuve.resultado_base(criterio, valor, AHORA, estatus=repuve.SIN_REPORTE)

        async def cerrar(self):
            pass

    monkeypatch.setattr(principal, "consultor_repuve", lambda: Falso())
    cliente = TestClient(principal.app)
    r = cliente.post("/v1/repuve", json={"niv": NIV})
    assert r.status_code == 200 and r.json()["estatus"] == repuve.SIN_REPORTE and r.json()["valor"] == NIV
    r = cliente.post("/v1/repuve", json={"niv": "corto"})
    assert r.status_code == 422 and r.json()["error"]["codigo"] == "CONSULTA_INVALIDA"


def test_deshabilitado():
    c = repuve.ConsultorREPUVE(repuve.Opciones(habilitado=False))
    r = asyncio.run(c.consultar(repuve.CRITERIO_NIV, NIV))
    assert r["estatus"] == repuve.NO_DISPONIBLE and "deshabilitada" in r["detalle"]


# ---------- Navegador real contra un portal falso local ----------

@pytest.fixture(scope="module")
def chromium_disponible():
    pytest.importorskip("playwright")
    from playwright.sync_api import sync_playwright

    try:
        with sync_playwright() as p:
            p.chromium.launch().close()
    except Exception as e:  # sin navegador instalado (playwright install chromium)
        pytest.skip(f"Chromium de Playwright no disponible: {e}")


@pytest.mark.parametrize("niv, estatus", [
    (NIV_SIN_REPORTE, repuve.SIN_REPORTE),
    (NIV_ROBADO, repuve.CON_REPORTE),
    (NIV_NO_EXISTE, repuve.NO_ENCONTRADO),
    (NIV_RECHAZADO, repuve.NO_DISPONIBLE),
])
def test_navegador_contra_portal_falso(chromium_disponible, tmp_path, niv, estatus):
    async def consultar(url):
        c = repuve.ConsultorREPUVE(repuve.Opciones(url=url, timeout_s=60, intervalo_min_s=0, cache_ttl_s=0,
                                                   depuracion_dir=str(tmp_path)))
        try:
            return await c.consultar(repuve.CRITERIO_NIV, niv)
        finally:
            await c.cerrar()

    with PortalFalso() as portal:
        r = asyncio.run(consultar(portal.url))
    assert r["estatus"] == estatus, r
    if estatus in (repuve.SIN_REPORTE, repuve.CON_REPORTE):
        assert r["vehiculo"]["marca"] == "NISSAN" and r["vehiculo"]["niv"] == niv
        assert "advertencias" not in r  # se abrieron las tres pestañas
    if niv == NIV_RECHAZADO:
        assert "reCAPTCHA" in r["detalle"]
        assert list(tmp_path.glob("repuve-*.zip")), "debió guardar la traza de depuración"
