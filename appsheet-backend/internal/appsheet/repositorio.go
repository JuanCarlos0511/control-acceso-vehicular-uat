package appsheet

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"registrovehicular/internal/config"
	"registrovehicular/internal/modelo"
)

// Repositorio traduce entre las filas de AppSheet y los tipos de modelo.
type Repositorio struct {
	c          almacenFilas
	t          config.Tablas
	zona       *time.Location
	diaPrimero bool // true si el Locale escribe las fechas como DD/MM/AAAA
	sheets     bool
}

type almacenFilas interface {
	Buscar(context.Context, string, string) ([]Fila, error)
	Agregar(context.Context, string, ...Fila) ([]Fila, error)
	Editar(context.Context, string, ...Fila) ([]Fila, error)
}

func NuevoRepositorio(c *Cliente, t config.Tablas, zona *time.Location, locale string) *Repositorio {
	return &Repositorio{c: c, t: t, zona: zona, diaPrimero: !strings.HasPrefix(strings.ToLower(locale), "en-us")}
}

func NuevoRepositorioSheets(c *ClienteSheets, t config.Tablas, zona *time.Location, locale string) *Repositorio {
	return &Repositorio{c: c, t: t, zona: zona, sheets: true, diaPrimero: !strings.HasPrefix(strings.ToLower(locale), "en-us")}
}

func (r *Repositorio) buscar(ctx context.Context, tabla, selector string) ([]Fila, error) {
	if r.sheets {
		// Sheets devuelve las filas de la pestaña. El repositorio aplica las
		// mismas comprobaciones de clave, placa, NIV y estado en Go.
		selector = ""
	}
	return r.c.Buscar(ctx, tabla, selector)
}

// Tablas devuelve la configuración de tablas (la usan las pruebas y el servicio).
func (r *Repositorio) Tablas() config.Tablas { return r.t }

// literal arma un texto de expresión de AppSheet. Solo acepta valores que ya
// pasaron por una validación estricta (UUID, correo, placa o NIV normalizados),
// porque las expresiones no tienen forma de escapar comillas.
func literal(v string) (string, error) {
	if strings.ContainsAny(v, "\"\n\r\\") {
		return "", fmt.Errorf("appsheet: valor no permitido en una expresión")
	}
	return `"` + v + `"`, nil
}

func (r *Repositorio) buscarUno(ctx context.Context, tabla, columna, valor string) (Fila, error) {
	lit, err := literal(valor)
	if err != nil {
		return nil, err
	}
	filas, err := r.buscar(ctx, tabla, fmt.Sprintf("Filter(%s, [%s] = %s)", tabla, columna, lit))
	if err != nil {
		return nil, err
	}
	var encontradas []Fila
	for _, f := range filas {
		if strings.EqualFold(strings.TrimSpace(f[columna]), valor) {
			encontradas = append(encontradas, f)
		}
	}
	switch len(encontradas) {
	case 0:
		return nil, nil
	case 1:
		return encontradas[0], nil
	default:
		return nil, fmt.Errorf("appsheet: hay claves duplicadas en %s", tabla)
	}
}

func (r *Repositorio) Solicitud(ctx context.Context, id string) (*modelo.Solicitud, error) {
	if !modelo.FolioValido(id) {
		return nil, fmt.Errorf("folio de solicitud inválido")
	}
	f, err := r.buscarUno(ctx, r.t.Solicitudes, "SolicitudId", id)
	if err != nil || f == nil {
		return nil, err
	}
	s := r.aSolicitud(f)
	return &s, nil
}

func (r *Repositorio) Solicitudes(ctx context.Context) ([]modelo.Solicitud, error) {
	filas, err := r.c.Buscar(ctx, r.t.Solicitudes, "")
	if err != nil {
		return nil, err
	}
	salida := make([]modelo.Solicitud, 0, len(filas))
	for _, f := range filas {
		salida = append(salida, r.aSolicitud(f))
	}
	return salida, nil
}

func (r *Repositorio) Alumno(ctx context.Context, correo string) (*modelo.Alumno, error) {
	correo = strings.ToLower(strings.TrimSpace(correo))
	if !modelo.CorreoValido(correo) {
		return nil, nil
	}
	f, err := r.buscarUno(ctx, r.t.Alumnos, "Correo", correo)
	if err != nil || f == nil {
		return nil, err
	}
	return &modelo.Alumno{
		Correo:         strings.ToLower(strings.TrimSpace(f["Correo"])),
		Matricula:      strings.TrimSpace(f["Matricula"]),
		NombreCompleto: strings.TrimSpace(f["NombreCompleto"]),
		Activo:         booleano(f["Activo"]),
		Programa:       strings.TrimSpace(f["Programa"]),
		SituacionUAT:   strings.TrimSpace(f["SituacionUAT"]),
		VerificadoUAT:  r.leerFecha(f["VerificadoUAT"]),
	}, nil
}

// GuardarAlumno agrega o actualiza la fila del alumno después de verificar
// su cuenta institucional.
func (r *Repositorio) GuardarAlumno(ctx context.Context, a modelo.Alumno, nuevo bool) error {
	if !modelo.CorreoValido(a.Correo) {
		return fmt.Errorf("appsheet: correo de alumno inválido")
	}
	return r.guardar(ctx, r.t.Alumnos, Fila{
		"Correo":         a.Correo,
		"Matricula":      a.Matricula,
		"NombreCompleto": a.NombreCompleto,
		"Activo":         strings.ToUpper(strconv.FormatBool(a.Activo)),
		"Programa":       a.Programa,
		"SituacionUAT":   a.SituacionUAT,
		"VerificadoUAT":  r.fechaHora(a.VerificadoUAT),
	}, nuevo)
}

func (r *Repositorio) Revision(ctx context.Context, id string) (*modelo.Revision, error) {
	f, err := r.buscarUno(ctx, r.t.Revisiones, "SolicitudId", id)
	if err != nil || f == nil {
		return nil, err
	}
	v := r.aRevision(f)
	return &v, nil
}

func (r *Repositorio) Revisiones(ctx context.Context) ([]modelo.Revision, error) {
	filas, err := r.c.Buscar(ctx, r.t.Revisiones, "")
	if err != nil {
		return nil, err
	}
	salida := make([]modelo.Revision, 0, len(filas))
	for _, f := range filas {
		salida = append(salida, r.aRevision(f))
	}
	return salida, nil
}

func (r *Repositorio) Resultado(ctx context.Context, id string) (*modelo.Resultado, error) {
	f, err := r.buscarUno(ctx, r.t.Resultados, "SolicitudId", id)
	if err != nil || f == nil {
		return nil, err
	}
	v := r.aResultado(f)
	return &v, nil
}

func (r *Repositorio) Resultados(ctx context.Context) ([]modelo.Resultado, error) {
	filas, err := r.c.Buscar(ctx, r.t.Resultados, "")
	if err != nil {
		return nil, err
	}
	salida := make([]modelo.Resultado, 0, len(filas))
	for _, f := range filas {
		salida = append(salida, r.aResultado(f))
	}
	return salida, nil
}

// AprobadasConVehiculo devuelve las otras solicitudes aprobadas con la misma
// placa o el mismo NIV (la misma regla que comprobarVehiculoRegistrado_).
func (r *Repositorio) AprobadasConVehiculo(ctx context.Context, placa, niv, excluirID string) ([]modelo.Solicitud, error) {
	if !modelo.PlacaValida(placa) || !modelo.NIVValido(niv) || !modelo.FolioValido(excluirID) {
		return nil, fmt.Errorf("placa, NIV o folio inválidos para buscar duplicados")
	}
	lp, _ := literal(placa)
	ln, _ := literal(niv)
	lid, _ := literal(excluirID)
	limpia := func(col string) string {
		return fmt.Sprintf(`SUBSTITUTE(SUBSTITUTE(SUBSTITUTE(UPPER(TRIM([%s])), " ", ""), "-", ""), ".", "")`, col)
	}
	selector := fmt.Sprintf("Filter(%s, AND([SolicitudId] <> %s, OR(%s = %s, %s = %s)))",
		r.t.Solicitudes, lid, limpia("Placa"), lp, limpia("NIV"), ln)
	filas, err := r.buscar(ctx, r.t.Solicitudes, selector)
	if err != nil {
		return nil, err
	}
	candidatas := map[string]modelo.Solicitud{}
	var ids []string
	for _, f := range filas {
		s := r.aSolicitud(f)
		if s.SolicitudID == excluirID || !modelo.FolioValido(s.SolicitudID) {
			continue
		}
		if modelo.NormalizarPlaca(s.Placa) == placa || modelo.NormalizarNIV(s.NIV) == niv {
			candidatas[s.SolicitudID] = s
			lit, _ := literal(s.SolicitudID)
			ids = append(ids, lit)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	selector = fmt.Sprintf(`Filter(%s, AND([Estado] = "%s", IN([SolicitudId], LIST(%s))))`,
		r.t.Resultados, modelo.EstadoAprobada, strings.Join(ids, ", "))
	resultados, err := r.buscar(ctx, r.t.Resultados, selector)
	if err != nil {
		return nil, err
	}
	var aprobadas []modelo.Solicitud
	for _, f := range resultados {
		if s, ok := candidatas[strings.TrimSpace(f["SolicitudId"])]; ok && strings.TrimSpace(f["Estado"]) == modelo.EstadoAprobada {
			aprobadas = append(aprobadas, s)
		}
	}
	return aprobadas, nil
}

// GuardarRevision agrega o actualiza la fila de Revisiones de la solicitud.
func (r *Repositorio) GuardarRevision(ctx context.Context, v modelo.Revision, nueva bool) error {
	f := Fila{
		"SolicitudId":      v.SolicitudID,
		"Identidad":        v.Identidad,
		"INE":              v.INE,
		"Tarjeta":          v.Tarjeta,
		"REPUVE":           v.REPUVE,
		"NIVConsultado":    v.NIVConsultado,
		"FechaREPUVE":      r.fechaHora(v.FechaREPUVE),
		"Decision":         v.Decision,
		"Revisor":          v.Revisor,
		"FechaPublicacion": r.fechaHora(v.FechaPublicacion),
	}
	return r.guardar(ctx, r.t.Revisiones, f, nueva)
}

// GuardarResultado agrega o actualiza lo que ve el alumno.
func (r *Repositorio) GuardarResultado(ctx context.Context, v modelo.Resultado, nuevo bool) error {
	f := Fila{
		"SolicitudId":   v.SolicitudID,
		"Correo":        v.Correo,
		"Estado":        v.Estado,
		"Indicacion":    v.Indicacion,
		"ActualizadoEn": r.fechaHora(v.ActualizadoEn),
	}
	return r.guardar(ctx, r.t.Resultados, f, nuevo)
}

// PasesDeSolicitud devuelve los pases emitidos para una solicitud (puede haber
// uno revocado y uno nuevo).
func (r *Repositorio) PasesDeSolicitud(ctx context.Context, solicitudID string) ([]modelo.Pase, error) {
	lit, err := literal(solicitudID)
	if err != nil || !modelo.FolioValido(solicitudID) {
		return nil, fmt.Errorf("folio de solicitud inválido")
	}
	filas, err := r.buscar(ctx, r.t.Pases, fmt.Sprintf("Filter(%s, [SolicitudId] = %s)", r.t.Pases, lit))
	if err != nil {
		return nil, err
	}
	var pases []modelo.Pase
	for _, f := range filas {
		if p := r.aPase(f); p.SolicitudID == solicitudID {
			pases = append(pases, p)
		}
	}
	return pases, nil
}

// Pase busca un pase por su PaseId (para verificar un QR en la caseta).
func (r *Repositorio) Pase(ctx context.Context, paseID string) (*modelo.Pase, error) {
	if !modelo.FolioValido(paseID) {
		return nil, nil
	}
	f, err := r.buscarUno(ctx, r.t.Pases, "PaseId", paseID)
	if err != nil || f == nil {
		return nil, err
	}
	p := r.aPase(f)
	return &p, nil
}

func (r *Repositorio) Pases(ctx context.Context) ([]modelo.Pase, error) {
	filas, err := r.c.Buscar(ctx, r.t.Pases, "")
	if err != nil {
		return nil, err
	}
	salida := make([]modelo.Pase, 0, len(filas))
	for _, f := range filas {
		salida = append(salida, r.aPase(f))
	}
	return salida, nil
}

func (r *Repositorio) GuardarPase(ctx context.Context, p modelo.Pase, nuevo bool) error {
	return r.guardar(ctx, r.t.Pases, Fila{
		"PaseId":      p.PaseID,
		"SolicitudId": p.SolicitudID,
		"Correo":      p.Correo,
		"Placa":       p.Placa,
		"Codigo":      p.Codigo,
		"QR":          p.QR,
		"EmitidoEn":   r.fechaHora(p.EmitidoEn),
		"VenceEn":     r.fechaHora(p.VenceEn),
		"Estado":      p.Estado,
	}, nuevo)
}

func (r *Repositorio) aPase(f Fila) modelo.Pase {
	return modelo.Pase{
		PaseID:      strings.TrimSpace(f["PaseId"]),
		SolicitudID: strings.TrimSpace(f["SolicitudId"]),
		Correo:      strings.ToLower(strings.TrimSpace(f["Correo"])),
		Placa:       strings.TrimSpace(f["Placa"]),
		Codigo:      strings.TrimSpace(f["Codigo"]),
		QR:          strings.TrimSpace(f["QR"]),
		EmitidoEn:   r.leerFecha(f["EmitidoEn"]),
		VenceEn:     r.leerFecha(f["VenceEn"]),
		Estado:      strings.ToUpper(strings.TrimSpace(f["Estado"])),
	}
}

// RegistrarValidacion agrega una fila a la bitácora opcional de validaciones.
func (r *Repositorio) RegistrarValidacion(ctx context.Context, v modelo.Validacion) error {
	if r.t.Validaciones == "" {
		return nil
	}
	_, err := r.c.Agregar(ctx, r.t.Validaciones, Fila{
		"ValidacionId":   v.ValidacionID,
		"SolicitudId":    v.SolicitudID,
		"FechaHora":      r.fechaHora(v.FechaHora),
		"Origen":         v.Origen,
		"Identidad":      v.Identidad,
		"INE":            v.INE,
		"Tarjeta":        v.Tarjeta,
		"REPUVE":         v.REPUVE,
		"Decision":       v.Decision,
		"PuntajeNombre":  strconv.FormatFloat(v.PuntajeNombre, 'f', 2, 64),
		"Motivos":        v.Motivos,
		"VehiculoREPUVE": v.VehiculoREPUVE,
		"DuracionSeg":    strconv.FormatFloat(v.DuracionSeg, 'f', 1, 64),
		"Version":        v.Version,
	})
	return err
}

func (r *Repositorio) guardar(ctx context.Context, tabla string, f Fila, nueva bool) error {
	var err error
	if nueva {
		_, err = r.c.Agregar(ctx, tabla, f)
	} else {
		_, err = r.c.Editar(ctx, tabla, f)
	}
	return err
}

func (r *Repositorio) aSolicitud(f Fila) modelo.Solicitud {
	return modelo.Solicitud{
		SolicitudID:    strings.TrimSpace(f["SolicitudId"]),
		Correo:         strings.ToLower(strings.TrimSpace(f["Correo"])),
		FechaEnvio:     r.leerFecha(f["FechaEnvio"]),
		Placa:          strings.TrimSpace(f["Placa"]),
		NIV:            strings.TrimSpace(f["NIV"]),
		FotoINE:        strings.TrimSpace(f["FotoINE"]),
		FotoINEReverso: strings.TrimSpace(f["FotoINEReverso"]),
		FotoTarjeta:    strings.TrimSpace(f["FotoTarjeta"]),
		AvisoLeido:     booleano(f["AvisoLeido"]),
	}
}

func (r *Repositorio) aRevision(f Fila) modelo.Revision {
	return modelo.Revision{
		SolicitudID:      strings.TrimSpace(f["SolicitudId"]),
		Identidad:        strings.TrimSpace(f["Identidad"]),
		INE:              strings.TrimSpace(f["INE"]),
		Tarjeta:          strings.TrimSpace(f["Tarjeta"]),
		REPUVE:           strings.TrimSpace(f["REPUVE"]),
		NIVConsultado:    strings.TrimSpace(f["NIVConsultado"]),
		FechaREPUVE:      r.leerFecha(f["FechaREPUVE"]),
		Decision:         strings.TrimSpace(f["Decision"]),
		Revisor:          strings.ToLower(strings.TrimSpace(f["Revisor"])),
		FechaPublicacion: r.leerFecha(f["FechaPublicacion"]),
	}
}

func (r *Repositorio) aResultado(f Fila) modelo.Resultado {
	return modelo.Resultado{
		SolicitudID:   strings.TrimSpace(f["SolicitudId"]),
		Correo:        strings.ToLower(strings.TrimSpace(f["Correo"])),
		Estado:        strings.TrimSpace(f["Estado"]),
		Indicacion:    strings.TrimSpace(f["Indicacion"]),
		ActualizadoEn: r.leerFecha(f["ActualizadoEn"]),
	}
}

func (r *Repositorio) fechaHora(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	if r.sheets {
		return t.In(r.zona).Format(time.RFC3339)
	}
	if r.diaPrimero {
		return t.In(r.zona).Format("02/01/2006 15:04:05")
	}
	return t.In(r.zona).Format("01/02/2006 15:04:05")
}

func (r *Repositorio) leerFecha(v string) time.Time {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}
	}
	formatos := []string{"1/2/2006 15:04:05", "1/2/2006 3:04:05 PM", "1/2/2006 15:04", "1/2/2006"}
	if r.diaPrimero {
		formatos = []string{"2/1/2006 15:04:05", "2/1/2006 3:04:05 PM", "2/1/2006 15:04", "2/1/2006"}
	}
	formatos = append(formatos, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02")
	for _, f := range formatos {
		if t, err := time.ParseInLocation(f, v, r.zona); err == nil {
			return t
		}
	}
	return time.Time{}
}

func booleano(v string) bool {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "TRUE", "Y", "YES", "1", "VERDADERO", "SI", "SÍ":
		return true
	}
	return false
}
