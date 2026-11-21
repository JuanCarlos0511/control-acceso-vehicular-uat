"""Portal local que imita la Consulta Ciudadana de REPUVE para probar el
navegador sin tocar el sitio real: mismo formulario, mismas rutas de API,
pestañas y alerta de error de SweetAlert. Los datos son ficticios."""

from __future__ import annotations

import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

NIV_SIN_REPORTE = "1HGCM82633A004352"
NIV_ROBADO = "3N1CN7AD8KL812345"
NIV_NO_EXISTE = "WVWZZZ3CZWE123456"
NIV_RECHAZADO = "1M8GDM9AXKP042788"

PAGINA = """<!doctype html><html lang="es-MX"><head><meta charset="utf-8"><title>REPUVE falso</title></head><body>
<script>window.grecaptcha = { execute: () => Promise.resolve("token-falso") };</script>
<input placeholder="Ingresa tu placa" maxlength="7">
<input placeholder="Ingresa tu número de serie" maxlength="17" id="niv">
<button type="button" id="buscar"><span class="glyphicon"></span> Buscar </button>
<nav id="pestanas" hidden>
  <a href="/ciudadania/consulta/pgj">Robo</a>
  <a href="/ciudadania/consulta/ocra">OCRA</a>
  <a href="/ciudadania/consulta/aviso">Avisos</a>
</nav>
<script>
let token = null;
async function llamar(servicio, datos) {
  const r = await fetch('/consulta/consulta/' + servicio, {
    method: 'POST', headers: {'content-type': 'application/json'}, body: JSON.stringify(datos)});
  return r;
}
document.getElementById('buscar').addEventListener('click', async () => {
  token = await grecaptcha.execute('x', {action: 'login'});
  const r = await llamar('repuve', {llave: document.getElementById('niv').value, criterio: '2', token});
  if (r.status === 401) {
    const c = document.createElement('div');
    c.className = 'swal2-container';
    c.innerHTML = '<div class="swal2-html-container">El reCaptcha no es válido. Intente nuevamente</div>';
    document.body.appendChild(c);
    return;
  }
  document.getElementById('pestanas').hidden = false;
});
document.querySelectorAll('#pestanas a').forEach(a => a.addEventListener('click', (e) => {
  e.preventDefault();
  llamar(a.getAttribute('href').split('/').pop(), {token});
}));
</script></body></html>"""

VEHICULO = {"PLACA": "XYZ123A", "VIN": "", "MARCA": "NISSAN", "MODELO": "VERSA", "ANIO_MODELO": "2019",
            "CLASE": "AUTOMOVIL", "TIPO": "SEDAN", "NOMBRE": "NO DEBE COPIARSE"}


class _Manejador(BaseHTTPRequestHandler):
    ultimo_niv = ""

    def log_message(self, *args):
        pass

    def _responder(self, estado: int, cuerpo, tipo="application/json"):
        datos = cuerpo.encode() if isinstance(cuerpo, str) else json.dumps(cuerpo).encode()
        self.send_response(estado)
        self.send_header("Content-Type", tipo)
        self.send_header("Content-Length", str(len(datos)))
        self.end_headers()
        self.wfile.write(datos)

    def do_GET(self):
        if self.path.startswith("/ciudadania"):
            self._responder(200, PAGINA, "text/html; charset=utf-8")
        else:
            self._responder(404, {"error": "no"})

    def do_POST(self):
        largo = int(self.headers.get("Content-Length", 0))
        datos = json.loads(self.rfile.read(largo) or b"{}")
        servicio = self.path.rsplit("/", 1)[-1]
        if servicio == "repuve":
            type(self).ultimo_niv = niv = datos.get("llave", "")
            if niv == NIV_RECHAZADO:
                return self._responder(401, {"statusCode": 401, "message": "Token inválido"})
            if niv == NIV_NO_EXISTE:
                return self._responder(200, {"datos": []})
            return self._responder(200, {"datos": [{**VEHICULO, "VIN": niv}]})
        robado = type(self).ultimo_niv == NIV_ROBADO
        if servicio == "pgj":
            filas = [{"ESTATUS_VHI_ROBO": "ROBADO", "FTE_VHI_ROBO": "TAMAULIPAS", "FECHA_ROBO": "01/02/2026"}] if robado else []
            return self._responder(200, {"datos": filas})
        if servicio == "ocra":
            return self._responder(200, {"conReporteRoboRecuperacion": "false"})
        if servicio == "aviso":
            return self._responder(200, {"datos": []})
        self._responder(404, {"error": "no"})


class PortalFalso:
    def __enter__(self) -> "PortalFalso":
        self._servidor = ThreadingHTTPServer(("127.0.0.1", 0), _Manejador)
        self.url = f"http://127.0.0.1:{self._servidor.server_port}/ciudadania/"
        self._hilo = threading.Thread(target=self._servidor.serve_forever, daemon=True)
        self._hilo.start()
        return self

    def __exit__(self, *exc) -> None:
        self._servidor.shutdown()
        self._servidor.server_close()
