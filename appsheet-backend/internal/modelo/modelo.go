// Package modelo define las filas de las tablas de AppSheet y los valores
// permitidos en cada columna. Los nombres y valores son los mismos que usa
// appsheet/RevisionVehicular.gs, para que la revisión humana y la automática
// escriban exactamente lo mismo.
package modelo

import (
	"regexp"
	"strings"
	"time"
)

// Valores de la columna Identidad de Revisiones.
const (
	IdentidadCoincide     = "COINCIDE"
	IdentidadNoCoincide   = "NO_COINCIDE"
	IdentidadSinConfirmar = "SIN_CONFIRMAR"
)

// Valores de la columna INE de Revisiones.
const (
	INEValidada     = "VALIDADA"
	INENoVigente    = "NO_VIGENTE"
	INESinConfirmar = "SIN_CONFIRMAR"
)

// Valores de la columna Tarjeta de Revisiones.
const (
	TarjetaValidada      = "VALIDADA"
	TarjetaInconsistente = "INCONSISTENTE"
	TarjetaSinConfirmar  = "SIN_CONFIRMAR"
)

// Valores de la columna REPUVE de Revisiones.
const (
	REPUVESinReporte       = "SIN_REPORTE"
	REPUVEConReporte       = "CON_REPORTE"
	REPUVERecuperado       = "RECUPERADO"
	REPUVENoEncontrado     = "NO_ENCONTRADO"
	REPUVENoDisponible     = "NO_DISPONIBLE"
	REPUVEDatosNoCoinciden = "DATOS_NO_COINCIDEN"
)

// Valores de la columna Decision de Revisiones.
const (
	DecisionEnRevision = "EN_REVISION"
	DecisionDevolver   = "DEVOLVER"
	DecisionAprobar    = "APROBAR"
)

// Valores de la columna Estado de Resultados.
const (
	EstadoEnRevision       = "EN_REVISION"
	EstadoRequiereAtencion = "REQUIERE_ATENCION"
	EstadoAprobada         = "APROBADA"
)

// Indicaciones para el alumno. Son las mismas que publica RevisionVehicular.gs.
const (
	IndicacionEnRevision = "El equipo está verificando tu solicitud."
	IndicacionAprobada   = "Tu registro vehicular fue aprobado."
	IndicacionINE        = "Verifica que tu INE vigente sea legible y que el nombre coincida con tu padrón escolar."
	IndicacionTarjeta    = "Revisa que la placa y el NIV coincidan con la tarjeta de circulación vigente."
	IndicacionFotos      = "No pudimos leer tus fotos. Envía una nueva solicitud con fotos nítidas, completas y sin reflejos."
	IndicacionContacto   = "Contacta al equipo de registro vehicular para revisar el expediente."
)

// Solicitud es una fila de la tabla Solicitudes (la llena el alumno en AppSheet).
type Solicitud struct {
	SolicitudID    string
	Correo         string
	FechaEnvio     time.Time
	Placa          string
	NIV            string
	FotoINE        string
	FotoINEReverso string
	FotoTarjeta    string
	AvisoLeido     bool
}

// Alumno es una fila del padrón (tabla Alumnos). Las últimas tres columnas
// las llena el registro con la cuenta institucional (portal de alumnos UAT).
type Alumno struct {
	Correo         string
	Matricula      string
	NombreCompleto string
	Activo         bool
	Programa       string
	SituacionUAT   string
	VerificadoUAT  time.Time
}

// Revision es una fila de la tabla Revisiones (expediente del equipo revisor).
type Revision struct {
	SolicitudID      string
	Identidad        string
	INE              string
	Tarjeta          string
	REPUVE           string
	NIVConsultado    string
	FechaREPUVE      time.Time
	Decision         string
	Revisor          string
	FechaPublicacion time.Time
}

// Resultado es una fila de la tabla Resultados (lo que ve el alumno).
type Resultado struct {
	SolicitudID   string
	Correo        string
	Estado        string
	Indicacion    string
	ActualizadoEn time.Time
}

// Valores de la columna Estado de Pases.
const (
	PaseActivo   = "ACTIVO"
	PaseRevocado = "REVOCADO"
)

// Pase es una fila de la tabla Pases: el código QR que el alumno muestra en la
// caseta. Se emite cuando la solicitud queda APROBADA. Para revocarlo, el
// personal cambia Estado a REVOCADO en la hoja.
type Pase struct {
	PaseID      string
	SolicitudID string
	Correo      string
	Placa       string
	Codigo      string // contenido del QR, firmado
	QR          string // URL de la imagen PNG (columna Image en AppSheet)
	EmitidoEn   time.Time
	VenceEn     time.Time
	Estado      string
}

// Validacion es una fila de la tabla opcional Validaciones: la bitácora de
// cada ejecución automática. No guarda CURP, CIC, OCR, nombres ni fotos.
type Validacion struct {
	ValidacionID   string
	SolicitudID    string
	FechaHora      time.Time
	Origen         string
	Identidad      string
	INE            string
	Tarjeta        string
	REPUVE         string
	Decision       string
	PuntajeNombre  float64
	Motivos        string
	VehiculoREPUVE string
	DuracionSeg    float64
	Version        string
}

var (
	reUUID   = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-4[a-fA-F0-9]{3}-[89abAB][a-fA-F0-9]{3}-[a-fA-F0-9]{12}$`)
	rePlaca  = regexp.MustCompile(`^[A-Z0-9]{4,10}$`)
	reNIV    = regexp.MustCompile(`^[A-HJ-NPR-Z0-9]{17}$`)
	reCorreo = regexp.MustCompile(`^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}$`)
)

// FolioValido indica si el SolicitudId tiene el formato UUID v4 que genera AppSheet.
func FolioValido(id string) bool { return reUUID.MatchString(id) }

// CorreoValido valida un correo ya normalizado en minúsculas.
func CorreoValido(correo string) bool { return reCorreo.MatchString(correo) }

// NormalizarPlaca deja la placa en mayúsculas sin espacios, guiones ni puntos
// (contrato §4: "xyz-123-a" → "XYZ123A").
func NormalizarPlaca(placa string) string {
	r := strings.NewReplacer(" ", "", "-", "", ".", "", "\t", "")
	return strings.ToUpper(r.Replace(strings.TrimSpace(placa)))
}

// PlacaValida aplica la regla del contrato: de 4 a 10 caracteres A-Z0-9.
func PlacaValida(placa string) bool { return rePlaca.MatchString(placa) }

// NormalizarNIV deja el NIV en mayúsculas y sin espacios ni guiones.
func NormalizarNIV(niv string) string {
	r := strings.NewReplacer(" ", "", "-", "", "\t", "")
	return strings.ToUpper(r.Replace(strings.TrimSpace(niv)))
}

// NIVValido: 17 caracteres, sin I, O ni Q (ISO 3779).
func NIVValido(niv string) bool { return reNIV.MatchString(niv) }
