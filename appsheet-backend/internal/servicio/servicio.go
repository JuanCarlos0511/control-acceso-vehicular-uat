// Package servicio orquesta la validación de una solicitud: lee AppSheet,
// descarga las fotos, las analiza, consulta REPUVE, decide y guarda el
// resultado de vuelta en AppSheet.
package servicio

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"registrovehicular/internal/analizador"
	"registrovehicular/internal/modelo"
	"registrovehicular/internal/repuve"
	"registrovehicular/internal/validacion"
)

const Version = "1.0.0"

// Almacen es lo que el servicio necesita de AppSheet.
type Almacen interface {
	Solicitud(ctx context.Context, id string) (*modelo.Solicitud, error)
	Solicitudes(ctx context.Context) ([]modelo.Solicitud, error)
	Alumno(ctx context.Context, correo string) (*modelo.Alumno, error)
	Revision(ctx context.Context, id string) (*modelo.Revision, error)
	Revisiones(ctx context.Context) ([]modelo.Revision, error)
	Resultado(ctx context.Context, id string) (*modelo.Resultado, error)
	Resultados(ctx context.Context) ([]modelo.Resultado, error)
	AprobadasConVehiculo(ctx context.Context, placa, niv, excluirID string) ([]modelo.Solicitud, error)
	GuardarRevision(ctx context.Context, v modelo.Revision, nueva bool) error
	GuardarResultado(ctx context.Context, v modelo.Resultado, nuevo bool) error
	RegistrarValidacion(ctx context.Context, v modelo.Validacion) error
}

type Imagenes interface {
	Obtener(ctx context.Context, tabla, valor string) ([]byte, error)
}

type Analizador interface {
	AnalizarINE(ctx context.Context, frente, reverso []byte) (*analizador.INE, error)
	AnalizarTarjeta(ctx context.Context, imagen []byte) (*analizador.Tarjeta, error)
}

type Opciones struct {
	TablaSolicitudes string
	CorreoSistema    string
	Politica         validacion.Politica
	Zona             *time.Location
	Log              *slog.Logger
	Ahora            func() time.Time
}

// Máximo de revalidaciones automáticas por solicitud cuando REPUVE no responde.
// Es bajo a propósito: si el portal rechaza la sesión (reCAPTCHA), reintentar
// no cambia el resultado y solo agrega consultas al sitio.
const maxReintentosREPUVE = 3

type Servicio struct {
	almacen    Almacen
	imagenes   Imagenes
	analizador Analizador
	repuve     repuve.Consultor // nil = REPUVE deshabilitado
	o          Opciones
	log        *slog.Logger
	historial  *historial

	enCurso sync.Map     // solicitudId → struct{}
	pases   *emisorPases // nil = no se emiten pases

	muReintentos sync.Mutex
	reintentos   map[string]int
}

func Nuevo(a Almacen, img Imagenes, an Analizador, rp repuve.Consultor, o Opciones) *Servicio {
	if o.Log == nil {
		o.Log = slog.Default()
	}
	if o.Zona == nil {
		o.Zona = time.UTC
	}
	if o.Ahora == nil {
		o.Ahora = time.Now
	}
	if o.TablaSolicitudes == "" {
		o.TablaSolicitudes = "Solicitudes"
	}
	return &Servicio{
		almacen: a, imagenes: img, analizador: an, repuve: rp, o: o, log: o.Log,
		historial: nuevoHistorial(500), reintentos: map[string]int{},
	}
}

// ErrEnCurso indica que la solicitud ya se está validando.
var ErrEnCurso = errors.New("la solicitud ya se está validando")

// Informe es lo que devuelve la API sobre una validación. No incluye datos personales.
type Informe struct {
	SolicitudID string               `json:"solicitudId"`
	Origen      string               `json:"origen"`
	Omitida     string               `json:"omitida,omitempty"` // motivo por el que no se escribió nada
	Dictamen    *validacion.Dictamen `json:"dictamen,omitempty"`
	Vehiculo    *repuve.Vehiculo     `json:"vehiculoRepuve,omitempty"`
	Guardado    bool                 `json:"guardado"`
	Pase        *InfoPase            `json:"pase,omitempty"`
	Error       string               `json:"error,omitempty"`
	Inicio      time.Time            `json:"inicio"`
	DuracionSeg float64              `json:"duracionSeg"`
}

var (
	ErrNoEncontrada = errors.New("la solicitud no existe o el backend no tiene permiso para verla")
	ErrFolio        = errors.New("folio de solicitud inválido")
)

// Validar ejecuta todo el flujo para una solicitud y guarda el resultado.
//
// Devuelve error solo ante fallas de infraestructura (AppSheet, Drive o el
// analizador caídos), para que el barrido lo reintente. Un problema con los
// documentos no es un error: se publica en el dictamen.
func (s *Servicio) Validar(ctx context.Context, id, origen string) (_ *Informe, err error) {
	if !modelo.FolioValido(id) {
		return nil, ErrFolio
	}
	if _, ocupado := s.enCurso.LoadOrStore(id, struct{}{}); ocupado {
		return nil, ErrEnCurso
	}
	defer s.enCurso.Delete(id)
	inicio := s.o.Ahora()
	inf := &Informe{SolicitudID: id, Origen: origen, Inicio: inicio}
	defer func() {
		inf.DuracionSeg = s.o.Ahora().Sub(inicio).Seconds()
		if err != nil {
			inf.Error = err.Error()
		}
		s.historial.guardar(*inf)
	}()

	sol, err := s.almacen.Solicitud(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("leer solicitud: %w", err)
	}
	if sol == nil {
		return nil, ErrNoEncontrada
	}
	resultado, err := s.almacen.Resultado(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("leer resultado: %w", err)
	}
	if resultado != nil && resultado.Estado == modelo.EstadoAprobada {
		inf.Omitida = "La solicitud ya está aprobada."
		return inf, nil
	}
	revision, err := s.almacen.Revision(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("leer revisión: %w", err)
	}
	if revision != nil && revision.Revisor != "" && !strings.EqualFold(revision.Revisor, s.o.CorreoSistema) {
		inf.Omitida = "Una persona del equipo revisor ya tomó la solicitud."
		return inf, nil
	}

	entrada, err := s.reunir(ctx, *sol)
	if err != nil {
		return nil, err
	}
	d := validacion.Evaluar(entrada, s.o.Politica)
	inf.Dictamen = &d
	if entrada.REPUVE != nil {
		inf.Vehiculo = entrada.REPUVE.Vehiculo
	}

	ahora := s.o.Ahora()
	err = s.almacen.GuardarRevision(ctx, modelo.Revision{
		SolicitudID:      id,
		Identidad:        d.Identidad,
		INE:              d.INE,
		Tarjeta:          d.Tarjeta,
		REPUVE:           d.REPUVE,
		NIVConsultado:    d.NIVConsultado,
		FechaREPUVE:      d.FechaREPUVE,
		Decision:         d.Decision,
		Revisor:          s.o.CorreoSistema,
		FechaPublicacion: ahora,
	}, revision == nil)
	if err != nil {
		return nil, fmt.Errorf("guardar revisión: %w", err)
	}
	err = s.almacen.GuardarResultado(ctx, modelo.Resultado{
		SolicitudID:   id,
		Correo:        sol.Correo,
		Estado:        d.Estado,
		Indicacion:    d.Indicacion,
		ActualizadoEn: ahora,
	}, resultado == nil)
	if err != nil {
		return nil, fmt.Errorf("guardar resultado: %w", err)
	}
	inf.Guardado = true

	if d.Estado == modelo.EstadoAprobada && s.pases != nil {
		// Si falla, la solicitud ya quedó aprobada y el barrido emite el pase después.
		if inf.Pase, err = s.emitir(ctx, *sol, false); err != nil {
			s.log.Error("no se pudo emitir el pase; se reintentará en el barrido", "solicitud", id, "error", err)
			err = nil
		}
	}

	if err := s.almacen.RegistrarValidacion(ctx, s.bitacora(id, origen, d, inf.Vehiculo, inicio)); err != nil {
		s.log.Warn("no se pudo registrar la bitácora de validación", "solicitud", id, "error", err)
	}
	s.log.Info("solicitud validada", "solicitud", id, "origen", origen, "estado", d.Estado,
		"identidad", d.Identidad, "ine", d.INE, "tarjeta", d.Tarjeta, "repuve", d.REPUVE)
	return inf, nil
}

// reunir obtiene todo lo necesario para decidir.
func (s *Servicio) reunir(ctx context.Context, sol modelo.Solicitud) (validacion.Entrada, error) {
	e := validacion.Entrada{Solicitud: sol}

	alumno, err := s.almacen.Alumno(ctx, sol.Correo)
	if err != nil {
		return e, fmt.Errorf("leer padrón: %w", err)
	}
	e.Alumno = alumno

	fotos, err := s.descargarFotos(ctx, sol)
	if err != nil {
		return e, err
	}

	placa := modelo.NormalizarPlaca(sol.Placa)
	niv := modelo.NormalizarNIV(sol.NIV)

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		if fotos.ine == nil {
			e.ErrINE = &analizador.ErrImagen{Codigo: "SIN_FOTO", Mensaje: "La solicitud no tiene foto de la INE."}
			return
		}
		e.INE, e.ErrINE = s.analizador.AnalizarINE(ctx, fotos.ine, fotos.ineReverso)
	}()
	go func() {
		defer wg.Done()
		if fotos.tarjeta == nil {
			e.ErrTarjeta = &analizador.ErrImagen{Codigo: "SIN_FOTO", Mensaje: "La solicitud no tiene foto de la tarjeta."}
			return
		}
		e.Tarjeta, e.ErrTarjeta = s.analizador.AnalizarTarjeta(ctx, fotos.tarjeta)
	}()
	var errRepuve error
	go func() {
		defer wg.Done()
		e.REPUVE, errRepuve = s.consultarREPUVE(ctx, niv)
	}()
	wg.Wait()
	// La hora de la decisión se toma al final: la consulta a REPUVE debe quedar antes.
	e.Ahora = s.o.Ahora().In(s.o.Zona)

	// Un analizador caído es una falla de infraestructura; una foto inválida, no.
	for _, err := range []error{e.ErrINE, e.ErrTarjeta} {
		var ei *analizador.ErrImagen
		if err != nil && !errors.As(err, &ei) {
			return e, fmt.Errorf("analizar documentos: %w", err)
		}
	}
	if errRepuve != nil && ctx.Err() != nil {
		return e, ctx.Err()
	}

	if modelo.PlacaValida(placa) && modelo.NIVValido(niv) {
		e.Duplicadas, err = s.almacen.AprobadasConVehiculo(ctx, placa, niv, sol.SolicitudID)
		if err != nil {
			return e, fmt.Errorf("buscar registros duplicados: %w", err)
		}
	}
	return e, nil
}

type fotos struct{ ine, ineReverso, tarjeta []byte }

func (s *Servicio) descargarFotos(ctx context.Context, sol modelo.Solicitud) (fotos, error) {
	var f fotos
	obtener := func(valor string, destino *[]byte, opcional bool) error {
		if strings.TrimSpace(valor) == "" {
			return nil
		}
		datos, err := s.imagenes.Obtener(ctx, s.o.TablaSolicitudes, valor)
		if err != nil {
			if opcional {
				s.log.Warn("no se pudo descargar una foto opcional", "solicitud", sol.SolicitudID, "error", err)
				return nil
			}
			return fmt.Errorf("descargar foto: %w", err)
		}
		*destino = datos
		return nil
	}
	if err := obtener(sol.FotoINE, &f.ine, false); err != nil {
		return f, err
	}
	if err := obtener(sol.FotoINEReverso, &f.ineReverso, true); err != nil {
		return f, err
	}
	if err := obtener(sol.FotoTarjeta, &f.tarjeta, false); err != nil {
		return f, err
	}
	return f, nil
}

func (s *Servicio) consultarREPUVE(ctx context.Context, niv string) (*repuve.Resultado, error) {
	if s.repuve == nil {
		return &repuve.Resultado{Estatus: modelo.REPUVENoDisponible, Criterio: repuve.CriterioNIV, Valor: niv,
			ConsultadoEn: s.o.Ahora(), Detalle: "La consulta automática a REPUVE está deshabilitada."}, nil
	}
	if !modelo.NIVValido(niv) {
		return nil, nil
	}
	r, err := s.repuve.Consultar(ctx, repuve.Consulta{Criterio: repuve.CriterioNIV, Valor: niv})
	if err != nil {
		return &repuve.Resultado{Estatus: modelo.REPUVENoDisponible, Criterio: repuve.CriterioNIV, Valor: niv,
			ConsultadoEn: s.o.Ahora(), Detalle: err.Error()}, err
	}
	return &r, nil
}

func (s *Servicio) bitacora(id, origen string, d validacion.Dictamen, v *repuve.Vehiculo, inicio time.Time) modelo.Validacion {
	partes := make([]string, 0, len(d.Motivos))
	for _, m := range d.Motivos {
		partes = append(partes, m.Codigo+": "+m.Detalle)
	}
	vehiculo := ""
	if v != nil {
		vehiculo = v.Resumen()
	}
	return modelo.Validacion{
		ValidacionID:   nuevoUUID(),
		SolicitudID:    id,
		FechaHora:      s.o.Ahora(),
		Origen:         origen,
		Identidad:      d.Identidad,
		INE:            d.INE,
		Tarjeta:        d.Tarjeta,
		REPUVE:         d.REPUVE,
		Decision:       d.Decision,
		PuntajeNombre:  d.PuntajeNombre,
		Motivos:        strings.Join(partes, "\n"),
		VehiculoREPUVE: vehiculo,
		DuracionSeg:    s.o.Ahora().Sub(inicio).Seconds(),
		Version:        Version,
	}
}

// Ultimo devuelve el último informe en memoria de una solicitud.
func (s *Servicio) Ultimo(id string) (Informe, bool) { return s.historial.obtener(id) }

// Pendientes lista las solicitudes recientes que el backend debe (re)validar:
// las que no tienen revisión ni resultado, y las que el propio backend dejó en
// revisión porque REPUVE no estaba disponible.
func (s *Servicio) Pendientes(ctx context.Context, dias int) ([]string, error) {
	solicitudes, err := s.almacen.Solicitudes(ctx)
	if err != nil {
		return nil, err
	}
	revisiones, err := s.almacen.Revisiones(ctx)
	if err != nil {
		return nil, err
	}
	resultados, err := s.almacen.Resultados(ctx)
	if err != nil {
		return nil, err
	}
	porRevision := map[string]modelo.Revision{}
	for _, r := range revisiones {
		porRevision[r.SolicitudID] = r
	}
	porResultado := map[string]modelo.Resultado{}
	for _, r := range resultados {
		porResultado[r.SolicitudID] = r
	}
	limite := s.o.Ahora().AddDate(0, 0, -dias)
	var ids []string
	for _, sol := range solicitudes {
		if !modelo.FolioValido(sol.SolicitudID) || (!sol.FechaEnvio.IsZero() && sol.FechaEnvio.Before(limite)) {
			continue
		}
		rev, hayRev := porRevision[sol.SolicitudID]
		res, hayRes := porResultado[sol.SolicitudID]
		switch {
		case hayRes && res.Estado == modelo.EstadoAprobada:
		case !hayRes && (!hayRev || strings.EqualFold(rev.Revisor, s.o.CorreoSistema)):
			ids = append(ids, sol.SolicitudID)
		case s.repuve != nil && hayRev && strings.EqualFold(rev.Revisor, s.o.CorreoSistema) &&
			rev.Decision == modelo.DecisionEnRevision && rev.REPUVE == modelo.REPUVENoDisponible &&
			s.contarReintento(sol.SolicitudID):
			ids = append(ids, sol.SolicitudID)
		}
	}
	return ids, nil
}

// contarReintento registra un reintento y dice si todavía se permite.
func (s *Servicio) contarReintento(id string) bool {
	s.muReintentos.Lock()
	defer s.muReintentos.Unlock()
	if s.reintentos[id] >= maxReintentosREPUVE {
		return false
	}
	s.reintentos[id]++
	return true
}

func nuevoUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
