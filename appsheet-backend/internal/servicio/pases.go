package servicio

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"registrovehicular/internal/modelo"
	"registrovehicular/internal/pase"
)

// AlmacenPases es lo que el servicio necesita de la tabla Pases.
type AlmacenPases interface {
	PasesDeSolicitud(ctx context.Context, solicitudID string) ([]modelo.Pase, error)
	Pase(ctx context.Context, paseID string) (*modelo.Pase, error)
	Pases(ctx context.Context) ([]modelo.Pase, error)
	GuardarPase(ctx context.Context, p modelo.Pase, nuevo bool) error
}

type OpcionesPases struct {
	Firmador   *pase.Firmador
	URLPublica string    // base del backend; la imagen queda en <URLPublica>/pases/qr/<codigo>.png
	Dias       int       // vigencia desde la emisión
	VenceFijo  time.Time // si es futura, todos los pases vencen ese día
}

type emisorPases struct {
	a AlmacenPases
	o OpcionesPases
}

var (
	ErrPasesDeshabilitados = errors.New("la emisión de pases está deshabilitada (falta PASE_SECRETO)")
	ErrNoAprobada          = errors.New("la solicitud no está aprobada")
)

// HabilitarPases activa la emisión del código QR al aprobar una solicitud.
func (s *Servicio) HabilitarPases(a AlmacenPases, o OpcionesPases) {
	s.pases = &emisorPases{a: a, o: o}
}

// InfoPase es lo que la API devuelve de un pase.
type InfoPase struct {
	PaseID  string    `json:"paseId"`
	Codigo  string    `json:"codigo"`
	QR      string    `json:"qr"`
	VenceEn time.Time `json:"venceEn"`
	Estado  string    `json:"estado"`
	Nuevo   bool      `json:"nuevo"`
}

func infoDe(p modelo.Pase, nuevo bool) *InfoPase {
	return &InfoPase{PaseID: p.PaseID, Codigo: p.Codigo, QR: p.QR, VenceEn: p.VenceEn, Estado: p.Estado, Nuevo: nuevo}
}

// EmitirPase devuelve el pase activo de una solicitud aprobada o emite uno.
// Con forzar=false no vuelve a emitir si ya existe cualquier pase (también uno
// revocado: revocar es una decisión del personal). Con forzar=true emite uno
// nuevo cuando no hay ninguno activo.
func (s *Servicio) EmitirPase(ctx context.Context, solicitudID string, forzar bool) (*InfoPase, error) {
	if s.pases == nil {
		return nil, ErrPasesDeshabilitados
	}
	if !modelo.FolioValido(solicitudID) {
		return nil, ErrFolio
	}
	sol, err := s.almacen.Solicitud(ctx, solicitudID)
	if err != nil {
		return nil, fmt.Errorf("leer solicitud: %w", err)
	}
	if sol == nil {
		return nil, ErrNoEncontrada
	}
	res, err := s.almacen.Resultado(ctx, solicitudID)
	if err != nil {
		return nil, fmt.Errorf("leer resultado: %w", err)
	}
	if res == nil || res.Estado != modelo.EstadoAprobada {
		return nil, ErrNoAprobada
	}
	return s.emitir(ctx, *sol, forzar)
}

func (s *Servicio) emitir(ctx context.Context, sol modelo.Solicitud, forzar bool) (*InfoPase, error) {
	existentes, err := s.pases.a.PasesDeSolicitud(ctx, sol.SolicitudID)
	if err != nil {
		return nil, fmt.Errorf("leer pases: %w", err)
	}
	for _, p := range existentes {
		if p.Estado == modelo.PaseActivo {
			return infoDe(p, false), nil
		}
	}
	if len(existentes) > 0 && !forzar {
		return infoDe(existentes[len(existentes)-1], false), nil
	}

	placa := modelo.NormalizarPlaca(sol.Placa)
	if !modelo.PlacaValida(placa) {
		return nil, fmt.Errorf("la placa de la solicitud no es válida para emitir el pase")
	}
	id := nuevoUUID()
	ahora := s.o.Ahora()
	vence := s.vencimiento(ahora)
	codigo := s.pases.o.Firmador.Codigo(id, placa, vence)
	p := modelo.Pase{
		PaseID:      id,
		SolicitudID: sol.SolicitudID,
		Correo:      sol.Correo,
		Placa:       placa,
		Codigo:      codigo,
		QR:          s.pases.o.URLPublica + "/pases/qr/" + url.PathEscape(codigo) + ".png",
		EmitidoEn:   ahora,
		VenceEn:     vence,
		Estado:      modelo.PaseActivo,
	}
	if err := s.pases.a.GuardarPase(ctx, p, true); err != nil {
		return nil, fmt.Errorf("guardar pase: %w", err)
	}
	s.log.Info("pase emitido", "solicitud", sol.SolicitudID, "pase", id, "vence", vence.Format("2006-01-02"))
	return infoDe(p, true), nil
}

func (s *Servicio) vencimiento(ahora time.Time) time.Time {
	local := ahora.In(s.o.Zona)
	finDeDia := func(t time.Time) time.Time {
		return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, s.o.Zona)
	}
	if f := s.pases.o.VenceFijo; !f.IsZero() && f.After(local) {
		return finDeDia(f)
	}
	return finDeDia(local.AddDate(0, 0, s.pases.o.Dias))
}

// EmitirPasesPendientes emite el pase de las solicitudes aprobadas que todavía
// no tienen uno, por ejemplo las que aprobó una persona desde la hoja.
func (s *Servicio) EmitirPasesPendientes(ctx context.Context) (int, error) {
	if s.pases == nil {
		return 0, nil
	}
	resultados, err := s.almacen.Resultados(ctx)
	if err != nil {
		return 0, err
	}
	pases, err := s.pases.a.Pases(ctx)
	if err != nil {
		return 0, err
	}
	conPase := map[string]bool{}
	for _, p := range pases {
		conPase[p.SolicitudID] = true
	}
	emitidos := 0
	for _, r := range resultados {
		if r.Estado != modelo.EstadoAprobada || conPase[r.SolicitudID] || !modelo.FolioValido(r.SolicitudID) {
			continue
		}
		info, err := s.EmitirPase(ctx, r.SolicitudID, false)
		if err != nil {
			s.log.Warn("no se pudo emitir un pase pendiente", "solicitud", r.SolicitudID, "error", err)
			continue
		}
		if info.Nuevo {
			emitidos++
		}
	}
	return emitidos, nil
}

// Verificacion es la respuesta para la caseta al escanear un QR.
type Verificacion struct {
	Valido  bool      `json:"valido"`
	Motivo  string    `json:"motivo,omitempty"` // CODIGO_INVALIDO, VENCIDO, NO_REGISTRADO, REVOCADO, NO_APROBADA
	Mensaje string    `json:"mensaje"`
	PaseID  string    `json:"paseId,omitempty"`
	Placa   string    `json:"placa,omitempty"`
	VenceEn time.Time `json:"venceEn,omitzero"`
}

// VerificarPase revisa firma, vigencia, revocación y que la solicitud siga aprobada.
// El guardia debe comparar Placa con la placa del vehículo.
func (s *Servicio) VerificarPase(ctx context.Context, codigo string) (*Verificacion, error) {
	if s.pases == nil {
		return nil, ErrPasesDeshabilitados
	}
	d, err := s.pases.o.Firmador.Verificar(codigo)
	if err != nil {
		return &Verificacion{Motivo: "CODIGO_INVALIDO", Mensaje: "El código no es un pase vehicular válido."}, nil
	}
	v := &Verificacion{PaseID: d.PaseID, Placa: d.Placa, VenceEn: d.Vence}
	if s.o.Ahora().After(d.Vence) {
		v.Motivo, v.Mensaje = "VENCIDO", "El pase venció."
		return v, nil
	}
	p, err := s.pases.a.Pase(ctx, d.PaseID)
	if err != nil {
		return nil, fmt.Errorf("leer pase: %w", err)
	}
	switch {
	case p == nil || p.Codigo != strings.TrimSpace(codigo):
		v.Motivo, v.Mensaje = "NO_REGISTRADO", "El pase no está registrado."
		return v, nil
	case p.Estado == modelo.PaseRevocado:
		v.Motivo, v.Mensaje = "REVOCADO", "El pase fue revocado."
		return v, nil
	}
	res, err := s.almacen.Resultado(ctx, p.SolicitudID)
	if err != nil {
		return nil, fmt.Errorf("leer resultado: %w", err)
	}
	if res == nil || res.Estado != modelo.EstadoAprobada {
		v.Motivo, v.Mensaje = "NO_APROBADA", "La solicitud del pase ya no está aprobada."
		return v, nil
	}
	v.Valido, v.Mensaje = true, "Pase vigente. Confirma que la placa coincida con la del vehículo."
	return v, nil
}

// ImagenPase dibuja el QR de un código con firma válida. No consulta AppSheet,
// porque AppSheet vuelve a pedir la imagen cada vez que la muestra.
func (s *Servicio) ImagenPase(codigo string) ([]byte, error) {
	if s.pases == nil {
		return nil, ErrPasesDeshabilitados
	}
	if _, err := s.pases.o.Firmador.Verificar(codigo); err != nil {
		return nil, err
	}
	return pase.PNG(codigo, 512)
}
