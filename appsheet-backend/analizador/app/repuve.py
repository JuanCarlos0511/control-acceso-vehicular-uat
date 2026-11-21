"""Consulta Ciudadana del Registro Público Vehicular (REPUVE) con Playwright.

El portal (https://www2.repuve.gob.mx:8443/ciudadania/) es una aplicación
Angular protegida con reCAPTCHA v3. Al pulsar «Buscar» llama a
/consulta/consulta/repuve (datos del vehículo) y, al abrir cada pestaña, a
/pgj (reportes de robo de las fiscalías), /ocra (robo y recuperación reportados
por aseguradoras) y /aviso (avisos judiciales). Las respuestas son JSON con la
forma {"<clave>": [ {CAMPO: valor, ...} ]}.

El navegador se usa como lo haría una persona: escribe el NIV, pulsa Buscar y
abre las pestañas. No resuelve ni evade el reCAPTCHA (Chromium se identifica
como navegador automatizado). Si el portal rechaza la sesión o no responde, el
resultado es NO_DISPONIBLE y la solicitud queda para revisión humana.

El resultado tiene la misma forma que repuve.Resultado en el backend de Go.
"""

from __future__ import annotations

import asyncio
import json
import logging
import os
import re
import time
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from urllib.parse import urlparse
from zoneinfo import ZoneInfo

log = logging.getLogger("analizador.repuve")

SIN_REPORTE = "SIN_REPORTE"
CON_REPORTE = "CON_REPORTE"
RECUPERADO = "RECUPERADO"
NO_ENCONTRADO = "NO_ENCONTRADO"
NO_DISPONIBLE = "NO_DISPONIBLE"

CRITERIO_PLACA = "1"
CRITERIO_NIV = "2"

SERVICIOS = ("repuve", "series", "pgj", "ocra", "aviso")

# Selectores del portal. Si REPUVE cambia su página, normalmente basta con
# ajustar estos valores (con REPUVE_DEPURACION_DIR se guarda una traza de
# Playwright de cada consulta fallida; ábrela con `playwright show-trace`).
SELECTOR_NIV = 'input[placeholder*="serie"]'
SELECTOR_PLACA = 'input[placeholder*="placa"]'
NOMBRE_BOTON = re.compile(r"buscar", re.IGNORECASE)
SELECTOR_ALERTA = ".swal2-container .swal2-html-container"
SELECTOR_PESTANA = 'a[href$="consulta/{servicio}"]'
JS_RECAPTCHA = '() => typeof grecaptcha !== "undefined" && typeof grecaptcha.execute === "function"'

_NIV = re.compile(r"^[A-HJ-NPR-Z0-9]{17}$")


# ---------------------------------------------------------------------------
# Interpretación de las respuestas (función pura, sin navegador)
# ---------------------------------------------------------------------------

@dataclass
class Captura:
    recibida: bool
    estado: int = 0
    cuerpo: bytes = b""
    error: str = ""


def servicio_de(url: str) -> str | None:
    ruta = urlparse(url).path
    if "/consulta/consulta/" not in ruta:
        return None
    nombre = ruta.rstrip("/").rsplit("/", 1)[-1]
    return nombre if nombre in SERVICIOS else None


def _texto(fila: dict, clave: str) -> str:
    v = fila.get(clave)
    if v is None:
        return ""
    if isinstance(v, float) and v.is_integer():
        v = int(v)
    return str(v).strip()


def _filas(cuerpo: bytes) -> list[dict]:
    """Acepta {"clave": [filas]} o directamente [filas]."""
    datos = json.loads(cuerpo or b"null")
    if datos is None:
        return []
    if isinstance(datos, list):
        return [f for f in datos if isinstance(f, dict)]
    if isinstance(datos, dict):
        if "statusCode" in datos:
            raise ValueError("respuesta de error")
        filas: list[dict] = []
        for clave in sorted(datos):
            if isinstance(datos[clave], list):
                filas.extend(f for f in datos[clave] if isinstance(f, dict))
        return filas
    raise ValueError("forma no reconocida")


def _mensaje(cuerpo: bytes) -> str:
    try:
        return str(json.loads(cuerpo).get("message", ""))
    except (ValueError, AttributeError):
        return ""


def _verdadero(v) -> bool:
    return v is True or (isinstance(v, str) and v.strip().lower() == "true")


def resultado_base(criterio: str, valor: str, ahora: datetime, estatus: str = NO_DISPONIBLE, detalle: str = "") -> dict:
    r = {
        "estatus": estatus,
        "criterio": criterio,
        "valor": valor,
        "consultadoEn": ahora.isoformat(timespec="seconds"),
        "vehiculo": None,
        "reportes": [],
    }
    if detalle:
        r["detalle"] = detalle
    return r


def _vehiculo(f: dict) -> dict:
    # NOMBRE y demás datos que no describen al vehículo no se copian.
    return {
        "placa": _texto(f, "PLACA"),
        "niv": _texto(f, "VIN"),
        "marca": _texto(f, "MARCA"),
        "modelo": _texto(f, "MODELO"),
        "linea": _texto(f, "LINEA"),
        "anioModelo": _texto(f, "ANIO_MODELO"),
        "clase": _texto(f, "CLASE"),
        "tipo": _texto(f, "TIPO"),
        "entidad": _texto(f, "ENTIDAD_EMPLACO"),
    }


def _reporte_pgj(f: dict) -> dict | None:
    estatus = _texto(f, "ESTATUS_VHI_ROBO").upper()
    if not estatus or "NO ROBADO" in estatus or "SIN REPORTE" in estatus:
        return None
    recuperado = "RECUPERAD" in estatus or bool(_texto(f, "FTE_RECUPERA")) or bool(_texto(f, "FEC_ACT_REC"))
    return {"fuente": "PGJ", "estatus": estatus, "entidad": _texto(f, "FTE_VHI_ROBO"),
            "fecha": _texto(f, "FECHA_ROBO"), "recuperado": recuperado}


def _reporte_ocra(cuerpo: bytes) -> dict | None:
    try:
        o = json.loads(cuerpo)
    except ValueError:
        return None
    if not isinstance(o, dict) or not _verdadero(o.get("conReporteRoboRecuperacion")):
        return None
    reporte = o.get("reporte") or {}
    robo = o.get("reporteRobo") or {}
    return {"fuente": "OCRA", "estatus": str(reporte.get("estatus", "")).strip().upper(),
            "entidad": str(robo.get("estado", "")), "fecha": str(robo.get("fechaRobo", "")),
            "recuperado": str(reporte.get("roboORecuperacion")) == "2"}


def interpretar(capturas: dict[str, Captura], criterio: str, valor: str, ahora: datetime) -> dict:
    r = resultado_base(criterio, valor, ahora)

    veh = capturas.get("repuve")
    if veh is None or not veh.recibida:
        veh = capturas.get("series", veh)
    if veh is None or not veh.recibida:
        r["detalle"] = "El servicio de vehículos de REPUVE no respondió." + (f" {veh.error}" if veh and veh.error else "")
        return r
    if veh.estado in (401, 403):
        r["detalle"] = ("REPUVE rechazó la consulta (verificación reCAPTCHA). " + _mensaje(veh.cuerpo)).strip()
        return r
    if veh.estado >= 400:
        r["detalle"] = f"REPUVE respondió HTTP {veh.estado}."
        return r
    try:
        filas = _filas(veh.cuerpo)
    except ValueError:
        r["detalle"] = "Respuesta de REPUVE no reconocida."
        return r
    if not filas:
        r["estatus"] = NO_ENCONTRADO
        return r
    r["vehiculo"] = _vehiculo(filas[0])

    pgj = capturas.get("pgj")
    if pgj is None or not pgj.recibida or pgj.estado >= 400:
        r["detalle"] = "El servicio de reportes de robo (PGJ) no respondió; no se puede afirmar que el vehículo no tenga reporte."
        return r
    try:
        filas_pgj = _filas(pgj.cuerpo)
    except ValueError:
        r["detalle"] = "Respuesta de reportes de robo (PGJ) no reconocida."
        return r
    r["reportes"].extend(rep for rep in map(_reporte_pgj, filas_pgj) if rep)

    advertencias = []
    ocra = capturas.get("ocra")
    if ocra and ocra.recibida and ocra.estado < 400:
        if rep := _reporte_ocra(ocra.cuerpo):
            r["reportes"].append(rep)
    else:
        advertencias.append("OCRA no respondió")

    aviso = capturas.get("aviso")
    if aviso and aviso.recibida and aviso.estado < 400:
        try:
            for f in _filas(aviso.cuerpo):
                delito = f"{_texto(f, 'DELITO')} {_texto(f, 'TIPO_DELITO')}".strip()
                if delito:
                    r["reportes"].append({"fuente": "AVISO_JUDICIAL", "estatus": delito, "recuperado": False})
        except ValueError:
            advertencias.append("Respuesta de avisos judiciales no reconocida")
    else:
        advertencias.append("Avisos judiciales no respondió")
    if advertencias:
        r["advertencias"] = advertencias

    if any(not rep["recuperado"] for rep in r["reportes"]):
        r["estatus"] = CON_REPORTE
    elif r["reportes"]:
        r["estatus"] = RECUPERADO
    else:
        r["estatus"] = SIN_REPORTE
    return r


# ---------------------------------------------------------------------------
# Validación de la consulta
# ---------------------------------------------------------------------------

def normalizar_consulta(niv: str | None, placa: str | None) -> tuple[str, str]:
    """Devuelve (criterio, valor) o lanza ValueError con un mensaje para el usuario."""
    if niv and niv.strip():
        valor = re.sub(r"[\s-]", "", niv).upper()
        if not _NIV.match(valor):
            raise ValueError("El NIV debe tener 17 caracteres sin I, O ni Q.")
        return CRITERIO_NIV, valor
    if placa and placa.strip():
        valor = re.sub(r"[\s.-]", "", placa).upper()
        if not re.fullmatch(r"[A-Z0-9]{5,7}", valor):
            raise ValueError("REPUVE acepta placas de 5 a 7 letras o números.")
        return CRITERIO_PLACA, valor
    raise ValueError("Envía niv o placa.")


# ---------------------------------------------------------------------------
# Navegador
# ---------------------------------------------------------------------------

def _duracion(texto: str, defecto: float) -> float:
    """Acepta el mismo formato que el backend de Go: 90, 20s, 2m, 6h."""
    texto = (texto or "").strip().lower()
    if not texto:
        return defecto
    m = re.fullmatch(r"(\d+(?:\.\d+)?)(ms|s|m|h)?", texto)
    if not m:
        raise ValueError(f"duración inválida: {texto!r}")
    factor = {"ms": 0.001, "s": 1, "m": 60, "h": 3600, None: 1}[m.group(2)]
    return float(m.group(1)) * factor


@dataclass
class Opciones:
    habilitado: bool = True
    url: str = "https://www2.repuve.gob.mx:8443/ciudadania/"
    headless: bool = True
    timeout_s: float = 120
    intervalo_min_s: float = 20
    cache_ttl_s: float = 6 * 3600
    depuracion_dir: str = ""
    zona: str = "America/Monterrey"

    @classmethod
    def desde_entorno(cls) -> "Opciones":
        e = os.environ.get
        return cls(
            habilitado=e("REPUVE_HABILITADO", "true").strip().lower() not in ("false", "0", "no"),
            url=e("REPUVE_URL", cls.url),
            headless=e("REPUVE_HEADLESS", "true").strip().lower() not in ("false", "0", "no"),
            timeout_s=_duracion(e("REPUVE_TIMEOUT", ""), cls.timeout_s),
            intervalo_min_s=_duracion(e("REPUVE_INTERVALO_MIN", ""), cls.intervalo_min_s),
            cache_ttl_s=_duracion(e("REPUVE_CACHE_TTL", ""), cls.cache_ttl_s),
            depuracion_dir=e("REPUVE_DEPURACION_DIR", ""),
            zona=e("ZONA_HORARIA", cls.zona),
        )


class ConsultorREPUVE:
    """Hace una consulta a la vez, con pausa mínima entre consultas y caché de
    los resultados definitivos. Cada consulta usa un contexto de navegador
    nuevo (sin cookies de consultas anteriores)."""

    def __init__(self, o: Opciones) -> None:
        self.o = o
        self._zona = ZoneInfo(o.zona)
        self._candado = asyncio.Lock()
        self._ultima = 0.0
        self._cache: dict[str, tuple[float, dict]] = {}
        self._playwright = None
        self._navegador = None

    def _ahora(self) -> datetime:
        return datetime.now(self._zona)

    async def consultar(self, criterio: str, valor: str) -> dict:
        if not self.o.habilitado:
            return resultado_base(criterio, valor, self._ahora(),
                                  detalle="La consulta a REPUVE está deshabilitada en el analizador.")
        async with self._candado:
            clave = f"{criterio}:{valor}"
            if (en_cache := self._cache.get(clave)) and en_cache[0] > time.monotonic():
                return dict(en_cache[1])
            espera = self.o.intervalo_min_s - (time.monotonic() - self._ultima)
            if espera > 0:
                await asyncio.sleep(espera)

            inicio = time.monotonic()
            try:
                r = await asyncio.wait_for(self._consultar(criterio, valor), timeout=self.o.timeout_s)
            except asyncio.TimeoutError:
                r = resultado_base(criterio, valor, self._ahora(), detalle="REPUVE no respondió a tiempo.")
            except Exception as e:  # el navegador murió o el portal no cargó
                log.warning("falla del navegador al consultar REPUVE: %s", type(e).__name__)
                await self._reiniciar()
                r = resultado_base(criterio, valor, self._ahora(),
                                   detalle=f"No se pudo completar la consulta en el portal ({type(e).__name__}).")
            self._ultima = time.monotonic()
            if r["estatus"] != NO_DISPONIBLE and self.o.cache_ttl_s > 0:
                self._cache[clave] = (time.monotonic() + self.o.cache_ttl_s, dict(r))
            log.info("consulta REPUVE: %s en %.1f s", r["estatus"], self._ultima - inicio)
            return r

    async def _navegador_listo(self):
        if self._navegador is not None and self._navegador.is_connected():
            return self._navegador
        await self._reiniciar()
        from playwright.async_api import async_playwright

        self._playwright = await async_playwright().start()
        self._navegador = await self._playwright.chromium.launch(headless=self.o.headless)
        return self._navegador

    async def _reiniciar(self) -> None:
        for cerrar in (getattr(self._navegador, "close", None), getattr(self._playwright, "stop", None)):
            if cerrar:
                try:
                    await cerrar()
                except Exception:
                    pass
        self._navegador = self._playwright = None

    async def cerrar(self) -> None:
        async with self._candado:
            await self._reiniciar()

    async def _consultar(self, criterio: str, valor: str) -> dict:
        navegador = await self._navegador_listo()
        contexto = await navegador.new_context(locale="es-MX", timezone_id=self.o.zona)
        if self.o.depuracion_dir:
            await contexto.tracing.start(screenshots=True, snapshots=True)
        exito = False
        try:
            pagina = await contexto.new_page()
            capturas: dict[str, Captura] = {}
            cambio = asyncio.Event()

            async def al_responder(respuesta):
                servicio = servicio_de(respuesta.url)
                if not servicio or respuesta.request.method != "POST":
                    return
                try:
                    capturas[servicio] = Captura(True, respuesta.status, await respuesta.body())
                except Exception as e:
                    capturas[servicio] = Captura(False, respuesta.status, error=type(e).__name__)
                cambio.set()

            def al_fallar(solicitud):
                servicio = servicio_de(solicitud.url)
                if servicio and solicitud.method == "POST":
                    capturas[servicio] = Captura(False, error=str(solicitud.failure or "falló la solicitud"))
                    cambio.set()

            pagina.on("response", al_responder)
            pagina.on("requestfailed", al_fallar)

            await pagina.goto(self.o.url, wait_until="domcontentloaded")
            campo = pagina.locator(SELECTOR_NIV if criterio == CRITERIO_NIV else SELECTOR_PLACA).first
            await campo.wait_for(state="visible", timeout=30_000)
            await pagina.wait_for_function(JS_RECAPTCHA, timeout=30_000)
            await campo.fill(valor)
            await pagina.get_by_role("button", name=NOMBRE_BOTON).first.click()

            alerta = await _esperar(pagina, capturas, cambio, ("repuve", "series"), 60)
            if not any(s in capturas for s in ("repuve", "series")):
                detalle = f"REPUVE mostró: {alerta}" if alerta else "REPUVE no respondió a tiempo."
                return resultado_base(criterio, valor, self._ahora(), detalle=detalle)

            r = interpretar(capturas, criterio, valor, self._ahora())
            if r["vehiculo"] is not None:
                for servicio in ("pgj", "ocra", "aviso"):
                    if servicio in capturas:
                        continue
                    enlace = pagina.locator(SELECTOR_PESTANA.format(servicio=servicio)).first
                    if await enlace.count() == 0:
                        continue
                    await enlace.click()
                    await _esperar(pagina, capturas, cambio, (servicio,), 35)
                r = interpretar(capturas, criterio, valor, self._ahora())
            exito = r["estatus"] != NO_DISPONIBLE
            return r
        finally:
            if self.o.depuracion_dir:
                ruta = None
                if not exito:
                    Path(self.o.depuracion_dir).mkdir(parents=True, exist_ok=True)
                    ruta = Path(self.o.depuracion_dir) / f"repuve-{self._ahora():%Y%m%d-%H%M%S}.zip"
                    log.info("traza de depuración de REPUVE: %s", ruta)
                await contexto.tracing.stop(path=ruta)
            await contexto.close()


async def _leer_alerta(pagina) -> str:
    alerta = pagina.locator(SELECTOR_ALERTA)
    try:
        if await alerta.count():
            return (await alerta.first.inner_text()).strip()
    except Exception:
        pass
    return ""


async def _esperar(pagina, capturas: dict, cambio: asyncio.Event, servicios: tuple[str, ...], limite_s: float) -> str:
    """Espera la respuesta de alguno de los servicios. Si antes aparece una
    alerta de error del portal, devuelve su texto."""
    fin = time.monotonic() + limite_s
    alerta = ""
    while time.monotonic() < fin:
        if any(s in capturas for s in servicios):
            return ""
        cambio.clear()
        try:
            await asyncio.wait_for(cambio.wait(), timeout=1.0)
            continue
        except asyncio.TimeoutError:
            pass
        if texto := await _leer_alerta(pagina):
            alerta = texto
            try:
                await asyncio.wait_for(cambio.wait(), timeout=3.0)
            except asyncio.TimeoutError:
                pass
            if not any(s in capturas for s in servicios):
                return alerta
    return alerta
