package servicio

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"registrovehicular/internal/modelo"
	"registrovehicular/internal/pase"
)

type pasesFalsos struct {
	mu    sync.Mutex
	filas map[string]modelo.Pase
}

func (p *pasesFalsos) PasesDeSolicitud(_ context.Context, id string) ([]modelo.Pase, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var l []modelo.Pase
	for _, f := range p.filas {
		if f.SolicitudID == id {
			l = append(l, f)
		}
	}
	return l, nil
}
func (p *pasesFalsos) Pase(_ context.Context, id string) (*modelo.Pase, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if f, ok := p.filas[id]; ok {
		return &f, nil
	}
	return nil, nil
}
func (p *pasesFalsos) Pases(context.Context) ([]modelo.Pase, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var l []modelo.Pase
	for _, f := range p.filas {
		l = append(l, f)
	}
	return l, nil
}
func (p *pasesFalsos) GuardarPase(_ context.Context, f modelo.Pase, nuevo bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, existe := p.filas[f.PaseID]; existe == nuevo {
		return errors.New("Add sobre una fila existente o Edit sobre una inexistente")
	}
	p.filas[f.PaseID] = f
	return nil
}

func conPases(s *Servicio) *pasesFalsos {
	p := &pasesFalsos{filas: map[string]modelo.Pase{}}
	s.HabilitarPases(p, OpcionesPases{
		Firmador:   pase.NuevoFirmador("secreto-de-prueba-con-32-caracteres!!", time.UTC),
		URLPublica: "https://registro.example.mx",
		Dias:       365,
	})
	return p
}

func unico(t *testing.T, p *pasesFalsos) modelo.Pase {
	t.Helper()
	if len(p.filas) != 1 {
		t.Fatalf("se esperaba un pase, hay %d", len(p.filas))
	}
	for _, f := range p.filas {
		return f
	}
	return modelo.Pase{}
}

func TestAprobarEmitePase(t *testing.T) {
	s, _, _ := preparar(nil)
	p := conPases(s)
	inf, err := s.Validar(context.Background(), folio, "webhook")
	if err != nil {
		t.Fatal(err)
	}
	if inf.Pase == nil || !inf.Pase.Nuevo {
		t.Fatalf("no se emitió el pase: %+v", inf)
	}
	f := unico(t, p)
	if f.Estado != modelo.PaseActivo || f.Placa != "XYZ123A" || f.Correo != "alumna@uat.edu.mx" ||
		!strings.HasPrefix(f.QR, "https://registro.example.mx/pases/qr/RV1.") || !strings.HasSuffix(f.QR, ".png") {
		t.Fatalf("pase: %+v", f)
	}
	if f.VenceEn.Year() != 2027 || f.VenceEn.Hour() != 23 {
		t.Fatalf("vencimiento: %v", f.VenceEn)
	}

	// Revalidar no emite otro pase.
	if inf, err = s.Validar(context.Background(), folio, "api"); err != nil {
		t.Fatal(err)
	}
	if len(p.filas) != 1 {
		t.Fatalf("se duplicó el pase: %d", len(p.filas))
	}
}

func TestSinAprobarNoHayPase(t *testing.T) {
	s, _, an := preparar(nil)
	p := conPases(s)
	an.ine.MRZ = nil // INE sin confirmar → EN_REVISION
	if _, err := s.Validar(context.Background(), folio, "webhook"); err != nil {
		t.Fatal(err)
	}
	if len(p.filas) != 0 {
		t.Fatal("no debe haber pase sin aprobación")
	}
	if _, err := s.EmitirPase(context.Background(), folio, true); !errors.Is(err, ErrNoAprobada) {
		t.Fatalf("err = %v", err)
	}
}

func TestBarridoEmitePaseDeAprobacionHumana(t *testing.T) {
	s, a, _ := preparar(nil)
	p := conPases(s)
	a.resultados[folio] = modelo.Resultado{SolicitudID: folio, Estado: modelo.EstadoAprobada}
	n, err := s.EmitirPasesPendientes(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	unico(t, p)
	if n, _ = s.EmitirPasesPendientes(context.Background()); n != 0 {
		t.Fatalf("segundo barrido emitió %d", n)
	}
}

func TestRevocadoNoSeReemiteSolo(t *testing.T) {
	s, a, _ := preparar(nil)
	p := conPases(s)
	a.resultados[folio] = modelo.Resultado{SolicitudID: folio, Estado: modelo.EstadoAprobada}
	info, err := s.EmitirPase(context.Background(), folio, false)
	if err != nil {
		t.Fatal(err)
	}
	f := p.filas[info.PaseID]
	f.Estado = modelo.PaseRevocado
	p.filas[f.PaseID] = f

	if n, _ := s.EmitirPasesPendientes(context.Background()); n != 0 || len(p.filas) != 1 {
		t.Fatal("el barrido no debe reemitir un pase revocado")
	}
	nuevo, err := s.EmitirPase(context.Background(), folio, true)
	if err != nil || !nuevo.Nuevo || len(p.filas) != 2 {
		t.Fatalf("reemisión manual: %+v %v", nuevo, err)
	}
}

func TestVerificarPase(t *testing.T) {
	s, a, _ := preparar(nil)
	p := conPases(s)
	a.resultados[folio] = modelo.Resultado{SolicitudID: folio, Estado: modelo.EstadoAprobada}
	info, err := s.EmitirPase(context.Background(), folio, false)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	v, err := s.VerificarPase(ctx, info.Codigo)
	if err != nil || !v.Valido || v.Placa != "XYZ123A" {
		t.Fatalf("válido: %+v %v", v, err)
	}
	if v, _ = s.VerificarPase(ctx, strings.Replace(info.Codigo, "XYZ123A", "ABC123A", 1)); v.Valido || v.Motivo != "CODIGO_INVALIDO" {
		t.Fatalf("alterado: %+v", v)
	}

	f := p.filas[info.PaseID]
	f.Estado = modelo.PaseRevocado
	p.filas[f.PaseID] = f
	if v, _ = s.VerificarPase(ctx, info.Codigo); v.Valido || v.Motivo != "REVOCADO" {
		t.Fatalf("revocado: %+v", v)
	}
	f.Estado = modelo.PaseActivo
	p.filas[f.PaseID] = f

	a.resultados[folio] = modelo.Resultado{SolicitudID: folio, Estado: modelo.EstadoRequiereAtencion}
	if v, _ = s.VerificarPase(ctx, info.Codigo); v.Valido || v.Motivo != "NO_APROBADA" {
		t.Fatalf("no aprobada: %+v", v)
	}

	s.o.Ahora = func() time.Time { return ahora.AddDate(2, 0, 0) }
	if v, _ = s.VerificarPase(ctx, info.Codigo); v.Valido || v.Motivo != "VENCIDO" {
		t.Fatalf("vencido: %+v", v)
	}
}

func TestVencimientoFijo(t *testing.T) {
	s, a, _ := preparar(nil)
	p := conPases(s)
	s.pases.o.VenceFijo = time.Date(2027, 7, 31, 0, 0, 0, 0, time.UTC)
	a.resultados[folio] = modelo.Resultado{SolicitudID: folio, Estado: modelo.EstadoAprobada}
	if _, err := s.EmitirPase(context.Background(), folio, false); err != nil {
		t.Fatal(err)
	}
	if v := unico(t, p).VenceEn.Format("2006-01-02"); v != "2027-07-31" {
		t.Fatalf("vence %s", v)
	}
}

func TestImagenPaseSoloConFirma(t *testing.T) {
	s, a, _ := preparar(nil)
	conPases(s)
	a.resultados[folio] = modelo.Resultado{SolicitudID: folio, Estado: modelo.EstadoAprobada}
	info, _ := s.EmitirPase(context.Background(), folio, false)
	if img, err := s.ImagenPase(info.Codigo); err != nil || len(img) == 0 {
		t.Fatalf("imagen: %v", err)
	}
	if _, err := s.ImagenPase("https://sitio-malicioso.example"); err == nil {
		t.Fatal("no debe dibujar códigos sin firma")
	}
}
