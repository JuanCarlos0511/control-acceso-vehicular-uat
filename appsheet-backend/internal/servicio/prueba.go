package servicio

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"registrovehicular/internal/analizador"
	"registrovehicular/internal/modelo"
	"registrovehicular/internal/repuve"
	"registrovehicular/internal/validacion"
)

// EntradaPrueba permite probar el flujo con fotos subidas directamente, sin
// leer ni escribir en AppSheet.
type EntradaPrueba struct {
	Placa           string
	NIV             string
	NombreCompleto  string
	INEFrente       []byte
	INEReverso      []byte
	Tarjeta         []byte
	ConsultarREPUVE bool
}

// InformePrueba incluye lo que leyó el analizador. Solo se devuelve a quien
// subió las fotos con el token de la API, y no se guarda.
type InformePrueba struct {
	Dictamen validacion.Dictamen `json:"dictamen"`
	INE      *analizador.INE     `json:"ine"`
	Tarjeta  *analizador.Tarjeta `json:"tarjeta"`
	REPUVE   *repuve.Resultado   `json:"repuve"`
}

// ErrFaltanFotos indica que la prueba no trae la INE o la tarjeta.
var ErrFaltanFotos = errors.New("faltan la foto de la INE (ineFrente) o la de la tarjeta (tarjeta)")

func (s *Servicio) Probar(ctx context.Context, p EntradaPrueba) (*InformePrueba, error) {
	if len(p.INEFrente) == 0 || len(p.Tarjeta) == 0 {
		return nil, ErrFaltanFotos
	}
	e := validacion.Entrada{
		Solicitud: modelo.Solicitud{
			SolicitudID: "00000000-0000-4000-8000-000000000000",
			Placa:       p.Placa, NIV: p.NIV, AvisoLeido: true,
		},
		Alumno: &modelo.Alumno{NombreCompleto: p.NombreCompleto, Matricula: "PRUEBA", Activo: true},
	}
	niv := modelo.NormalizarNIV(p.NIV)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		e.INE, e.ErrINE = s.analizador.AnalizarINE(ctx, p.INEFrente, p.INEReverso)
	}()
	go func() {
		defer wg.Done()
		e.Tarjeta, e.ErrTarjeta = s.analizador.AnalizarTarjeta(ctx, p.Tarjeta)
	}()
	if p.ConsultarREPUVE {
		e.REPUVE, _ = s.consultarREPUVE(ctx, niv)
	}
	wg.Wait()
	e.Ahora = s.o.Ahora().In(s.o.Zona)
	for _, err := range []error{e.ErrINE, e.ErrTarjeta} {
		var ei *analizador.ErrImagen
		if err != nil && !errors.As(err, &ei) {
			return nil, fmt.Errorf("analizar documentos: %w", err)
		}
	}
	return &InformePrueba{
		Dictamen: validacion.Evaluar(e, s.o.Politica),
		INE:      e.INE,
		Tarjeta:  e.Tarjeta,
		REPUVE:   e.REPUVE,
	}, nil
}

// ErrREPUVEDeshabilitado indica que REPUVE_HABILITADO=false.
var ErrREPUVEDeshabilitado = errors.New("la consulta automática a REPUVE está deshabilitada")

// ConsultarREPUVE expone la consulta directa (para el equipo revisor).
func (s *Servicio) ConsultarREPUVE(ctx context.Context, q repuve.Consulta) (*repuve.Resultado, error) {
	if s.repuve == nil {
		return nil, ErrREPUVEDeshabilitado
	}
	r, err := s.repuve.Consultar(ctx, q)
	if err != nil {
		return nil, err
	}
	return &r, nil
}
