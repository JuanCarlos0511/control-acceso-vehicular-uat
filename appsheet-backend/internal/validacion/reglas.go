// Package validacion decide el resultado de una solicitud a partir del padrón,
// el análisis de las fotos y la consulta a REPUVE. Aplica las mismas reglas que
// resolverRevisionVehicular_ en appsheet/RevisionVehicular.gs: solo se aprueba
// con identidad, INE y tarjeta validadas y una consulta REPUVE sin reporte para
// el mismo NIV. Ante cualquier duda la solicitud queda EN_REVISION para una
// persona; solo se devuelve al alumno cuando el problema es claro.
package validacion

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"registrovehicular/internal/analizador"
	"registrovehicular/internal/modelo"
	"registrovehicular/internal/repuve"
)

// Motivo explica una parte del dictamen. No contiene datos personales.
type Motivo struct {
	Codigo  string `json:"codigo"`
	Detalle string `json:"detalle"`
	// Bloquea impide la aprobación automática aunque las cuatro verificaciones pasen.
	Bloquea bool `json:"bloquea"`
}

type Entrada struct {
	Solicitud  modelo.Solicitud
	Alumno     *modelo.Alumno
	INE        *analizador.INE
	ErrINE     error
	Tarjeta    *analizador.Tarjeta
	ErrTarjeta error
	REPUVE     *repuve.Resultado
	Duplicadas []modelo.Solicitud
	Ahora      time.Time
}

// toleranciaReloj absorbe diferencias de reloj entre el backend y el analizador,
// que fecha la consulta a REPUVE.
const toleranciaReloj = time.Minute

type Politica struct {
	AutoAprobar bool
}

type Dictamen struct {
	Identidad     string    `json:"identidad"`
	INE           string    `json:"ine"`
	Tarjeta       string    `json:"tarjeta"`
	REPUVE        string    `json:"repuve"`
	NIVConsultado string    `json:"nivConsultado"`
	FechaREPUVE   time.Time `json:"fechaRepuve,omitzero"`
	Decision      string    `json:"decision"`
	Estado        string    `json:"estado"`
	Indicacion    string    `json:"indicacion"`
	PuntajeNombre float64   `json:"puntajeNombre"`
	Motivos       []Motivo  `json:"motivos"`
}

type motivos []Motivo

func (m *motivos) agregar(codigo, detalle string, bloquea bool) {
	*m = append(*m, Motivo{Codigo: codigo, Detalle: detalle, Bloquea: bloquea})
}

// Evaluar produce el dictamen. Es determinista y no hace E/S.
func Evaluar(e Entrada, p Politica) Dictamen {
	var m motivos
	placa := modelo.NormalizarPlaca(e.Solicitud.Placa)
	niv := modelo.NormalizarNIV(e.Solicitud.NIV)
	placaOK, nivOK := modelo.PlacaValida(placa), modelo.NIVValido(niv)

	alumnoOK := e.Alumno != nil && e.Alumno.Activo && e.Alumno.Matricula != "" && e.Alumno.NombreCompleto != ""
	if !alumnoOK {
		m.agregar("ALUMNO_NO_ACTIVO", "La cuenta no corresponde a un alumno activo con datos completos en el padrón.", true)
	}
	if !e.Solicitud.AvisoLeido {
		m.agregar("AVISO_NO_LEIDO", "La solicitud no confirma la lectura del aviso de privacidad.", true)
	}
	if !placaOK {
		m.agregar("PLACA_FORMATO", "La placa debe tener de 4 a 10 letras o números.", false)
	}
	if !nivOK {
		m.agregar("NIV_FORMATO", "El NIV debe tener 17 caracteres sin I, O ni Q.", false)
	}

	var vehiculo *repuve.Vehiculo
	if e.REPUVE != nil {
		vehiculo = e.REPUVE.Vehiculo
	}

	d := Dictamen{}
	d.Identidad, d.PuntajeNombre = evaluarIdentidad(e.Alumno, e.INE, &m)
	d.INE = evaluarINE(e.INE, e.ErrINE, e.Ahora, &m)
	d.Tarjeta = evaluarTarjeta(placa, niv, e.Tarjeta, e.ErrTarjeta, vehiculo, e.Ahora, &m)
	d.REPUVE = evaluarREPUVE(placa, niv, e.REPUVE, e.Tarjeta, &m)
	if e.REPUVE != nil && e.REPUVE.Estatus != modelo.REPUVENoDisponible && e.REPUVE.Criterio == repuve.CriterioNIV {
		d.NIVConsultado = e.REPUVE.Valor
		d.FechaREPUVE = e.REPUVE.ConsultadoEn
	}
	if len(e.Duplicadas) > 0 {
		m.agregar("VEHICULO_YA_REGISTRADO", "Ya hay una solicitud aprobada con esa placa o NIV.", true)
	}

	// Devolver al alumno solo cuando el problema es claro y lo puede corregir.
	var indicaciones []string
	if d.Identidad == modelo.IdentidadNoCoincide || d.INE == modelo.INENoVigente {
		indicaciones = append(indicaciones, modelo.IndicacionINE)
	}
	if !placaOK || !nivOK || d.Tarjeta == modelo.TarjetaInconsistente || d.REPUVE == modelo.REPUVEDatosNoCoinciden {
		indicaciones = append(indicaciones, modelo.IndicacionTarjeta)
	}
	if fotoIlegible(e.ErrINE, ineCalidad(e.INE), e.INE != nil && e.INE.EsINE) ||
		fotoIlegible(e.ErrTarjeta, tarjetaCalidad(e.Tarjeta), e.Tarjeta != nil && e.Tarjeta.EsTarjeta) {
		indicaciones = append(indicaciones, modelo.IndicacionFotos)
	}

	bloqueado := false
	for _, mo := range m {
		bloqueado = bloqueado || mo.Bloquea
	}
	aprobable := alumnoOK && e.Solicitud.AvisoLeido && placaOK && nivOK && !bloqueado &&
		d.Identidad == modelo.IdentidadCoincide && d.INE == modelo.INEValidada &&
		d.Tarjeta == modelo.TarjetaValidada && d.REPUVE == modelo.REPUVESinReporte &&
		d.NIVConsultado == niv && !d.FechaREPUVE.IsZero() && !d.FechaREPUVE.After(e.Ahora.Add(toleranciaReloj))

	switch {
	case len(indicaciones) > 0:
		d.Decision, d.Estado, d.Indicacion = modelo.DecisionDevolver, modelo.EstadoRequiereAtencion, strings.Join(indicaciones, " ")
	case aprobable && p.AutoAprobar:
		d.Decision, d.Estado, d.Indicacion = modelo.DecisionAprobar, modelo.EstadoAprobada, modelo.IndicacionAprobada
	default:
		if aprobable {
			m.agregar("APROBACION_MANUAL", "Todas las verificaciones pasaron; la aprobación automática está desactivada.", false)
		}
		d.Decision, d.Estado, d.Indicacion = modelo.DecisionEnRevision, modelo.EstadoEnRevision, modelo.IndicacionEnRevision
	}
	d.Motivos = m
	return d
}

func evaluarIdentidad(al *modelo.Alumno, a *analizador.INE, m *motivos) (string, float64) {
	if al == nil || strings.TrimSpace(al.NombreCompleto) == "" || a == nil {
		return modelo.IdentidadSinConfirmar, 0
	}
	type candidato struct {
		texto     string
		confiable bool
	}
	var candidatos []candidato
	if a.MRZ != nil && a.MRZ.Apellidos != nil {
		texto := *a.MRZ.Apellidos
		if a.MRZ.Nombres != nil {
			texto += " " + *a.MRZ.Nombres
		}
		candidatos = append(candidatos, candidato{texto, a.MRZ.Valida})
	}
	if a.Nombre != nil && a.Nombre.Completo != "" {
		candidatos = append(candidatos, candidato{a.Nombre.Completo, a.Nombre.Confianza >= 0.85 && len(TokensNombre(a.Nombre.Completo)) >= 2})
	}
	if len(candidatos) == 0 {
		m.agregar("NOMBRE_NO_LEIDO", "No se pudo leer el nombre en la INE.", false)
		return modelo.IdentidadSinConfirmar, 0
	}

	mejorRef, mejorDoc, hayConfiable := 0.0, 0.0, false
	for _, c := range candidatos {
		ref, doc := Cobertura(al.NombreCompleto, c.texto)
		if ref > mejorRef || (ref == mejorRef && doc > mejorDoc) {
			mejorRef, mejorDoc = ref, doc
		}
		hayConfiable = hayConfiable || c.confiable
	}
	switch {
	case mejorRef >= 0.9 && mejorDoc >= 0.66:
		return modelo.IdentidadCoincide, mejorRef
	case mejorRef < 0.5 && hayConfiable:
		m.agregar("NOMBRE_NO_COINCIDE", fmt.Sprintf("El nombre de la INE no coincide con el padrón (%.0f%% de coincidencia).", mejorRef*100), false)
		return modelo.IdentidadNoCoincide, mejorRef
	default:
		m.agregar("NOMBRE_DUDOSO", fmt.Sprintf("El nombre de la INE coincide parcialmente con el padrón (%.0f%%).", mejorRef*100), false)
		return modelo.IdentidadSinConfirmar, mejorRef
	}
}

func evaluarINE(a *analizador.INE, err error, ahora time.Time, m *motivos) string {
	if err != nil {
		var ei *analizador.ErrImagen
		if errors.As(err, &ei) {
			m.agregar("INE_IMAGEN_INVALIDA", ei.Mensaje, false)
		} else {
			m.agregar("INE_ANALISIS_FALLIDO", "No se pudo analizar la foto de la INE.", false)
		}
		return modelo.INESinConfirmar
	}
	if a == nil {
		return modelo.INESinConfirmar
	}
	if !a.EsINE {
		m.agregar("NO_ES_INE", "La foto no se reconoce como credencial para votar.", false)
		return modelo.INESinConfirmar
	}

	estado := modelo.INEValidada
	degradar := func(nuevo string) {
		if estado == modelo.INENoVigente {
			return
		}
		if nuevo == modelo.INENoVigente || estado == modelo.INEValidada {
			estado = nuevo
		}
	}

	var fin time.Time
	confiable := false
	if a.MRZ != nil && a.MRZ.Controles.Expiracion && a.MRZ.FechaExpiracion != nil {
		if t, err := time.ParseInLocation("2006-01-02", *a.MRZ.FechaExpiracion, ahora.Location()); err == nil {
			fin, confiable = t.Add(24*time.Hour-time.Second), true
		}
	}
	if fin.IsZero() && a.Vigencia != nil && a.Vigencia.Anio > 0 {
		fin = time.Date(a.Vigencia.Anio, 12, 31, 23, 59, 59, 0, ahora.Location())
		confiable = a.Vigencia.Confianza >= 0.8
	}
	switch {
	case fin.IsZero():
		m.agregar("VIGENCIA_NO_LEIDA", "No se pudo leer la vigencia de la INE.", false)
		degradar(modelo.INESinConfirmar)
	case ahora.After(fin) && confiable:
		m.agregar("INE_VENCIDA", fmt.Sprintf("La INE perdió vigencia en %d.", fin.Year()), false)
		degradar(modelo.INENoVigente)
	case ahora.After(fin):
		m.agregar("VIGENCIA_DUDOSA", "La vigencia leída indica que la INE venció, pero la lectura es poco confiable.", false)
		degradar(modelo.INESinConfirmar)
	}

	switch {
	case a.MRZ == nil:
		m.agregar("SIN_MRZ", "No se leyó la zona MRZ del reverso; se necesita el reverso para verificar la credencial.", false)
		degradar(modelo.INESinConfirmar)
	case !a.MRZ.Valida:
		m.agregar("MRZ_INVALIDA", "Los dígitos de control de la MRZ no coinciden.", false)
		degradar(modelo.INESinConfirmar)
	}

	c := a.Consistencia
	if c.NacimientoCurpVsMrz != nil && !*c.NacimientoCurpVsMrz {
		m.agregar("INCONSISTENCIA_NACIMIENTO", "La fecha de nacimiento de la CURP no coincide con la del reverso.", false)
		degradar(modelo.INESinConfirmar)
	}
	if c.VigenciaFrenteVsMrz != nil && !*c.VigenciaFrenteVsMrz {
		m.agregar("INCONSISTENCIA_VIGENCIA", "La vigencia del frente no coincide con la del reverso.", false)
		degradar(modelo.INESinConfirmar)
	}
	if c.NombreFrenteVsMrz != nil && *c.NombreFrenteVsMrz < 0.6 {
		m.agregar("INCONSISTENCIA_NOMBRE", "El nombre del frente no coincide con el del reverso.", false)
		degradar(modelo.INESinConfirmar)
	}
	if a.CURP != nil && a.CURP.FormatoValido && !a.CURP.DigitoValido {
		m.agregar("CURP_DIGITO", "El dígito verificador de la CURP leída no coincide (posible error de lectura).", false)
	}
	if f := a.Calidad.Frente; f != nil {
		if !f.Aceptable {
			m.agregar("CALIDAD_INE", "Foto de la INE con problemas: "+strings.Join(f.Problemas, ", ")+".", false)
		}
		if f.RostroDetectado != nil && !*f.RostroDetectado {
			m.agregar("SIN_ROSTRO", "No se detectó la fotografía del titular en la INE.", false)
			degradar(modelo.INESinConfirmar)
		}
	}
	return estado
}

func evaluarTarjeta(placa, niv string, t *analizador.Tarjeta, err error, v *repuve.Vehiculo, ahora time.Time, m *motivos) string {
	if err != nil {
		var ei *analizador.ErrImagen
		if errors.As(err, &ei) {
			m.agregar("TARJETA_IMAGEN_INVALIDA", ei.Mensaje, false)
		} else {
			m.agregar("TARJETA_ANALISIS_FALLIDO", "No se pudo analizar la foto de la tarjeta de circulación.", false)
		}
		return modelo.TarjetaSinConfirmar
	}
	if t == nil {
		return modelo.TarjetaSinConfirmar
	}

	nivCoincide, nivCercano := false, false
	for _, c := range t.NIV.Candidatos {
		switch d := Distancia(c.Valor, niv); {
		case d == 0:
			nivCoincide = true
		case d <= 2:
			nivCercano = true
		}
	}
	nivContradice := false
	if !nivCoincide && !nivCercano && len(t.NIV.Candidatos) > 0 {
		p := t.NIV.Candidatos[0]
		nivContradice = p.Etiquetado || p.DigitoValido
	}

	placaCoincide := false
	distanciaEtiquetada := -1
	for _, c := range t.Placa.Candidatos {
		if c.Valor == placa {
			placaCoincide = true
		}
		if c.Etiquetado {
			if d := Distancia(c.Valor, placa); distanciaEtiquetada < 0 || d < distanciaEtiquetada {
				distanciaEtiquetada = d
			}
		}
	}
	placaREPUVE := v != nil && modelo.NormalizarPlaca(v.Placa) == placa

	if !t.EsTarjeta && !nivCoincide {
		m.agregar("NO_ES_TARJETA", "La foto no se reconoce como tarjeta de circulación.", false)
		return modelo.TarjetaSinConfirmar
	}
	if nivContradice {
		m.agregar("NIV_NO_COINCIDE", "El NIV de la tarjeta no coincide con el de la solicitud.", false)
		return modelo.TarjetaInconsistente
	}
	if !placaCoincide && distanciaEtiquetada >= 2 && !placaREPUVE {
		m.agregar("PLACA_NO_COINCIDE", "La placa de la tarjeta no coincide con la de la solicitud.", false)
		return modelo.TarjetaInconsistente
	}
	if t.Vigencia.Anio != nil && *t.Vigencia.Anio < ahora.Year() {
		m.agregar("TARJETA_VENCIDA", fmt.Sprintf("La vigencia leída en la tarjeta es %d; confirmar si está vigente.", *t.Vigencia.Anio), true)
	}
	if nivCoincide && (placaCoincide || placaREPUVE) {
		return modelo.TarjetaValidada
	}
	if !nivCoincide {
		if nivCercano {
			m.agregar("NIV_DUDOSO", "El NIV de la tarjeta se leyó con diferencias de 1 o 2 caracteres.", false)
		} else {
			m.agregar("NIV_NO_LEIDO", "No se pudo leer el NIV en la tarjeta.", false)
		}
	}
	if !placaCoincide && !placaREPUVE {
		m.agregar("PLACA_NO_CONFIRMADA", "No se pudo confirmar la placa en la tarjeta ni en REPUVE.", false)
	}
	return modelo.TarjetaSinConfirmar
}

func evaluarREPUVE(placa, niv string, r *repuve.Resultado, t *analizador.Tarjeta, m *motivos) string {
	if r == nil {
		m.agregar("REPUVE_NO_CONSULTADO", "No se consultó REPUVE.", true)
		return modelo.REPUVENoDisponible
	}
	switch r.Estatus {
	case modelo.REPUVENoDisponible:
		m.agregar("REPUVE_NO_DISPONIBLE", r.Detalle, true)
		return r.Estatus
	case modelo.REPUVENoEncontrado:
		m.agregar("REPUVE_NO_ENCONTRADO", "REPUVE no tiene registro para ese NIV.", true)
		return r.Estatus
	case modelo.REPUVEConReporte:
		m.agregar("REPUVE_CON_REPORTE", "REPUVE muestra un reporte vigente para el vehículo.", true)
		return r.Estatus
	case modelo.REPUVERecuperado:
		m.agregar("REPUVE_RECUPERADO", "El vehículo tiene un reporte de robo con recuperación.", true)
		return r.Estatus
	}

	v := r.Vehiculo
	if v == nil {
		return modelo.REPUVESinReporte
	}
	if v.NIV != "" && modelo.NormalizarNIV(v.NIV) != niv {
		m.agregar("REPUVE_NIV_DISTINTO", "El NIV registrado en REPUVE es distinto al de la solicitud.", false)
		return modelo.REPUVEDatosNoCoinciden
	}
	placaREPUVE := modelo.NormalizarPlaca(v.Placa)
	if modelo.PlacaValida(placaREPUVE) && !strings.Contains(placaREPUVE, "SIN") && placaREPUVE != placa {
		if tarjetaConfirmaPlaca(t, placa) {
			m.agregar("REPUVE_PLACA_DISTINTA", "La placa en REPUVE es distinta a la de la tarjeta; puede ser un cambio de placas reciente.", true)
		} else {
			m.agregar("REPUVE_PLACA_DISTINTA", "La placa de la solicitud no coincide con la registrada en REPUVE.", false)
			return modelo.REPUVEDatosNoCoinciden
		}
	}
	if t != nil && t.Marca != nil && v.Marca != "" && !mismaMarca(*t.Marca, v.Marca) {
		m.agregar("MARCA_DISTINTA", "La marca de la tarjeta no coincide con la registrada en REPUVE.", true)
	}
	if t != nil && t.AnioModelo != nil && v.AnioModelo != "" && strconv.Itoa(*t.AnioModelo) != strings.TrimSpace(v.AnioModelo) {
		m.agregar("ANIO_DISTINTO", "El año modelo de la tarjeta no coincide con el registrado en REPUVE.", true)
	}
	return modelo.REPUVESinReporte
}

func tarjetaConfirmaPlaca(t *analizador.Tarjeta, placa string) bool {
	if t == nil {
		return false
	}
	for _, c := range t.Placa.Candidatos {
		if c.Valor == placa {
			return true
		}
	}
	return false
}

var aliasMarca = map[string]string{
	"VW": "VOLKSWAGEN", "GM": "CHEVROLET", "GENERAL MOTORS": "CHEVROLET",
	"MERCEDES": "MERCEDES BENZ", "MERCEDESBENZ": "MERCEDES BENZ",
}

func mismaMarca(a, b string) bool {
	norm := func(s string) string {
		s = strings.Join(TokensNombre(s), " ")
		if alias, ok := aliasMarca[s]; ok {
			return alias
		}
		return s
	}
	x, y := norm(a), norm(b)
	return x != "" && y != "" && (x == y || strings.Contains(x, y) || strings.Contains(y, x))
}

func ineCalidad(a *analizador.INE) *analizador.Calidad {
	if a == nil {
		return nil
	}
	return a.Calidad.Frente
}

func tarjetaCalidad(t *analizador.Tarjeta) *analizador.Calidad {
	if t == nil {
		return nil
	}
	return t.Calidad
}

// fotoIlegible: la imagen no se pudo decodificar, o no se reconoce el
// documento y además la foto tiene problemas de calidad.
func fotoIlegible(err error, c *analizador.Calidad, reconocido bool) bool {
	var ei *analizador.ErrImagen
	if errors.As(err, &ei) {
		return true
	}
	return err == nil && c != nil && !reconocido && !c.Aceptable
}
