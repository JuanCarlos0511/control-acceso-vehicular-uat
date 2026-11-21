from datetime import date

from app import curp, mrz, vin
from app.ocr import Linea
from app import ine, tarjeta
from tests.utilidades import curp_valida, mrz_ine

HOY = date(2026, 11, 21)


def linea(texto: str, x: float, y: float, conf: float = 0.98) -> Linea:
    return Linea(texto, conf, x, y, 12 * len(texto), 30)


# ---------- MRZ ----------

def test_mrz_valida_y_campos():
    l1, l2, l3 = mrz_ine()
    r = mrz.buscar([l1, l2, l3], HOY)
    assert r is not None and r.valida
    assert r.controles["compuesto"]
    assert r.nacimiento == date(2002, 3, 15)
    assert r.expiracion == date(2033, 12, 31)
    assert r.apellidos == "PEREZ LOPEZ"
    assert r.nombres == "MARIA FERNANDA"


def test_mrz_corrige_confusiones_de_ocr():
    l1, l2, l3 = mrz_ine()
    # O por 0 e I por 1 en campos numéricos, espacios y comillas angulares raras.
    l1_ocr = l1.replace("0", "O").replace("1", "I")
    l2_ocr = l2[:3] + " " + l2[3:].replace("<", "«")
    r = mrz.buscar(["texto de relleno", l1_ocr, l2_ocr, l3], HOY)
    assert r is not None and r.valida


def test_mrz_detecta_digito_alterado():
    l1, l2, l3 = mrz_ine()
    alterada = l2[:8] + "34" + l2[10:]  # cambia la fecha de expiración sin recalcular el dígito
    r = mrz.buscar([l1, alterada, l3], HOY)
    assert r is not None
    assert not r.controles["expiracion"]
    assert not r.valida


def test_mrz_ausente():
    assert mrz.buscar(["INSTITUTO NACIONAL ELECTORAL", "NOMBRE"], HOY) is None


# ---------- CURP ----------

def test_curp_valida():
    valor = curp_valida()
    r = curp.analizar_curp(valor)
    assert r.formato_valido and r.digito_valido
    assert r.nacimiento == date(2002, 3, 15)
    assert r.sexo == "M"


def test_curp_corrige_ocr_y_rechaza_digito():
    valor = curp_valida()
    con_ruido = valor[:4] + valor[4:10].replace("0", "O") + valor[10:]
    assert curp.analizar_curp(con_ruido).digito_valido
    malo = valor[:17] + str((int(valor[17]) + 1) % 10)
    r = curp.analizar_curp(malo)
    assert r.formato_valido and not r.digito_valido


def test_clave_elector():
    r = curp.buscar_clave_elector(["PRLPMR02031528M400"])
    assert r == {"valor": "PRLPMR02031528M400", "formatoValido": True}
    assert curp.buscar_clave_elector(["NOESUNACLAVE"]) is None


# ---------- NIV ----------

def test_niv_digito_verificador():
    assert vin.digito_valido("1M8GDM9AXKP042788")
    assert vin.digito_valido("1HGCM82633A004352")
    assert not vin.digito_valido("1HGCM82643A004352")
    assert vin.normalizar("1hgcm8263 3a0o4352") == "1HGCM82633A004352"


# ---------- INE (a partir de líneas de OCR) ----------

def test_ine_extrae_nombre_y_valida_consistencia():
    valor_curp = curp_valida()
    frente = [
        linea("INSTITUTO NACIONAL ELECTORAL", 40, 20),
        linea("CREDENCIAL PARA VOTAR", 40, 60),
        linea("NOMBRE", 330, 120),
        linea("PEREZ", 330, 165),
        linea("SEXO M", 850, 165),
        linea("LOPEZ", 330, 210),
        linea("MARIA FERNANDA", 330, 255),
        linea("DOMICILIO", 330, 310),
        linea(f"CURP {valor_curp}", 330, 420),
        linea("FECHA DE NACIMIENTO 15/03/2002", 330, 470),
        linea("VIGENCIA 2023 - 2033", 330, 520),
    ]
    reverso = [linea(t, 40, 500 + 50 * i) for i, t in enumerate(mrz_ine())]
    r = ine.analizar(frente, 1100, reverso, [], HOY)
    assert r["esINE"]
    assert r["nombre"]["apellidoPaterno"] == "PEREZ"
    assert r["nombre"]["apellidoMaterno"] == "LOPEZ"
    assert r["nombre"]["nombres"] == "MARIA FERNANDA"
    assert r["curp"]["digitoValido"]
    assert r["sexo"] == "M"
    assert r["vigencia"]["anio"] == 2033
    assert r["mrz"]["valida"]
    assert r["consistencia"] == {
        "nacimientoCurpVsMrz": True,
        "nacimientoCurpVsFrente": True,
        "nombreFrenteVsMrz": 1.0,
        "vigenciaFrenteVsMrz": True,
    }
    assert "SIN_REVERSO" not in r["advertencias"]


def test_ine_sin_reverso_lo_advierte():
    frente = [linea("INSTITUTO NACIONAL ELECTORAL", 40, 20), linea("NOMBRE", 330, 120), linea("PEREZ", 330, 165)]
    r = ine.analizar(frente, 1100, None, [], HOY)
    assert "SIN_REVERSO" in r["advertencias"]
    assert r["mrz"] is None


# ---------- Tarjeta (a partir de líneas de OCR) ----------

def test_tarjeta_extrae_niv_placa_marca():
    lineas = [
        linea("TARJETA DE CIRCULACION", 40, 20),
        linea("PLACA", 40, 100), linea("XYZ-123-A", 200, 100),
        linea("NO. DE SERIE", 40, 150), linea("1HGCM82633A004352", 260, 150),
        linea("MARCA NISSAN", 40, 200),
        linea("MODELO 2019", 40, 250),
        linea("VIGENCIA 2027", 40, 300),
    ]
    r = tarjeta.analizar(lineas, HOY)
    assert r["esTarjeta"]
    assert r["niv"]["valor"] == "1HGCM82633A004352"
    assert r["niv"]["candidatos"][0]["etiquetado"]
    assert r["niv"]["candidatos"][0]["digitoValido"]
    assert r["placa"]["valor"] == "XYZ123A"
    assert r["marca"] == "NISSAN"
    assert r["anioModelo"] == 2019
    assert r["vigencia"] == {"anio": 2027, "indefinida": False}


def test_tarjeta_placa_con_espacios():
    lineas = [linea("PLACAS", 40, 100), linea("ABC 1234", 200, 100)]
    r = tarjeta.analizar(lineas, HOY)
    assert "ABC1234" in [c["valor"] for c in r["placa"]["candidatos"]]
