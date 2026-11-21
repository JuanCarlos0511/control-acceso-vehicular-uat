package servicio

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"registrovehicular/internal/analizador"
	"registrovehicular/internal/modelo"
	"registrovehicular/internal/repuve"
	"registrovehicular/internal/validacion"
)

const (
	folio   = "6f1c2d3e-4a5b-4c6d-8e7f-001122334455"
	niv     = "1HGCM82633A004352"
	sistema = "backend@uat.edu.mx"
)

var ahora = time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)

func ptr[T any](v T) *T { return &v }

// ---- Dobles ----

type almacenFalso struct {
	mu           sync.Mutex
	solicitudes  map[string]modelo.Solicitud
	alumnos      map[string]modelo.Alumno
	revisiones   map[string]modelo.Revision
	resultados   map[string]modelo.Resultado
	validaciones []modelo.Validacion
	errLeer      error
}

func nuevoAlmacen() *almacenFalso {
	return &almacenFalso{
		solicitudes: map[string]modelo.Solicitud{folio: {
			SolicitudID: folio, Correo: "alumna@uat.edu.mx", FechaEnvio: ahora.Add(-time.Hour), Placa: "XYZ-123-A", NIV: niv,
			FotoINE: "Solicitudes_Images/a.FotoINE.jpg", FotoINEReverso: "Solicitudes_Images/a.FotoINEReverso.jpg",
			FotoTarjeta: "Solicitudes_Images/a.FotoTarjeta.jpg", AvisoLeido: true,
		}},
		alumnos: map[string]modelo.Alumno{"alumna@uat.edu.mx": {
			Correo: "alumna@uat.edu.mx", Matricula: "2210123456", NombreCompleto: "María Fernanda Pérez López", Activo: true,
		}},
		revisiones: map[string]modelo.Revision{},
		resultados: map[string]modelo.Resultado{},
	}
}

func (a *almacenFalso) Solicitud(_ context.Context, id string) (*modelo.Solicitud, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.errLeer != nil {
		return nil, a.errLeer
	}
	if s, ok := a.solicitudes[id]; ok {
		return &s, nil
	}
	return nil, nil
}
func (a *almacenFalso) Solicitudes(context.Context) ([]modelo.Solicitud, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var l []modelo.Solicitud
	for _, s := range a.solicitudes {
		l = append(l, s)
	}
	return l, nil
}
func (a *almacenFalso) Alumno(_ context.Context, c string) (*modelo.Alumno, error) {
	if al, ok := a.alumnos[c]; ok {
		return &al, nil
	}
	return nil, nil
}
func (a *almacenFalso) Revision(_ context.Context, id string) (*modelo.Revision, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if r, ok := a.revisiones[id]; ok {
		return &r, nil
	}
	return nil, nil
}
func (a *almacenFalso) Revisiones(context.Context) ([]modelo.Revision, error) {
	var l []modelo.Revision
	for _, r := range a.revisiones {
		l = append(l, r)
	}
	return l, nil
}
func (a *almacenFalso) Resultado(_ context.Context, id string) (*modelo.Resultado, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if r, ok := a.resultados[id]; ok {
		return &r, nil
	}
	return nil, nil
}
func (a *almacenFalso) Resultados(context.Context) ([]modelo.Resultado, error) {
	var l []modelo.Resultado
	for _, r := range a.resultados {
		l = append(l, r)
	}
	return l, nil
}
func (a *almacenFalso) AprobadasConVehiculo(context.Context, string, string, string) ([]modelo.Solicitud, error) {
	return nil, nil
}
func (a *almacenFalso) GuardarRevision(_ context.Context, v modelo.Revision, nueva bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, existe := a.revisiones[v.SolicitudID]; existe == nueva {
		return errors.New("Add sobre una fila existente o Edit sobre una inexistente")
	}
	a.revisiones[v.SolicitudID] = v
	return nil
}
func (a *almacenFalso) GuardarResultado(_ context.Context, v modelo.Resultado, nuevo bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, existe := a.resultados[v.SolicitudID]; existe == nuevo {
		return errors.New("Add sobre una fila existente o Edit sobre una inexistente")
	}
	a.resultados[v.SolicitudID] = v
	return nil
}
func (a *almacenFalso) RegistrarValidacion(_ context.Context, v modelo.Validacion) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.validaciones = append(a.validaciones, v)
	return nil
}

type imagenesFalsas struct{ err error }

func (i imagenesFalsas) Obtener(_ context.Context, _, valor string) ([]byte, error) {
	if i.err != nil {
		return nil, i.err
	}
	return []byte("jpeg:" + valor), nil
}

type analizadorFalso struct {
	ine     *analizador.INE
	tarjeta *analizador.Tarjeta
	err     error
	reverso []byte
}

func (a *analizadorFalso) AnalizarINE(_ context.Context, _, reverso []byte) (*analizador.INE, error) {
	a.reverso = reverso
	return a.ine, a.err
}
func (a *analizadorFalso) AnalizarTarjeta(context.Context, []byte) (*analizador.Tarjeta, error) {
	return a.tarjeta, a.err
}

type repuveFalso struct {
	r     repuve.Resultado
	veces int
	fecha func() time.Time // si no es nil, fecha la consulta en el momento en que se hace
}

func (f *repuveFalso) Consultar(_ context.Context, q repuve.Consulta) (repuve.Resultado, error) {
	f.veces++
	r := f.r
	r.Criterio, r.Valor = q.Criterio, q.Valor
	if f.fecha != nil {
		r.ConsultadoEn = f.fecha()
	}
	return r, nil
}

func ineOK() *analizador.INE {
	a := &analizador.INE{
		EsINE: true, Nombre: &analizador.Nombre{Completo: "PEREZ LOPEZ MARIA FERNANDA", Confianza: 0.97},
		MRZ: &analizador.MRZ{Valida: true, Controles: analizador.ControlesMRZ{Documento: true, Nacimiento: true, Expiracion: true},
			FechaExpiracion: ptr("2033-12-31"), Apellidos: ptr("PEREZ LOPEZ"), Nombres: ptr("MARIA FERNANDA")},
	}
	a.Calidad.Frente = &analizador.Calidad{Aceptable: true}
	return a
}

func tarjetaOK() *analizador.Tarjeta {
	t := &analizador.Tarjeta{EsTarjeta: true}
	t.NIV.Candidatos = []analizador.CandidatoNIV{{Valor: niv, Etiquetado: true, DigitoValido: true}}
	t.Placa.Candidatos = []analizador.CandidatoPlaca{{Valor: "XYZ123A", Etiquetado: true}}
	return t
}

func preparar(rp *repuveFalso) (*Servicio, *almacenFalso, *analizadorFalso) {
	a := nuevoAlmacen()
	an := &analizadorFalso{ine: ineOK(), tarjeta: tarjetaOK()}
	if rp == nil {
		rp = &repuveFalso{r: repuve.Resultado{Estatus: modelo.REPUVESinReporte, ConsultadoEn: ahora.Add(-time.Minute),
			Vehiculo: &repuve.Vehiculo{Placa: "XYZ123A", NIV: niv, Marca: "NISSAN"}}}
	}
	s := Nuevo(a, imagenesFalsas{}, an, rp, Opciones{
		CorreoSistema: sistema, Politica: validacion.Politica{AutoAprobar: true}, Ahora: func() time.Time { return ahora },
	})
	return s, a, an
}

// ---- Pruebas ----

func TestValidarApruebaYGuarda(t *testing.T) {
	s, a, an := preparar(nil)
	inf, err := s.Validar(context.Background(), folio, "webhook")
	if err != nil {
		t.Fatal(err)
	}
	if !inf.Guardado || inf.Dictamen.Estado != modelo.EstadoAprobada {
		t.Fatalf("informe: %+v", inf)
	}
	rev := a.revisiones[folio]
	if rev.Decision != modelo.DecisionAprobar || rev.Revisor != sistema || rev.NIVConsultado != niv || rev.FechaREPUVE.IsZero() {
		t.Fatalf("revisión: %+v", rev)
	}
	res := a.resultados[folio]
	if res.Estado != modelo.EstadoAprobada || res.Correo != "alumna@uat.edu.mx" || res.Indicacion != modelo.IndicacionAprobada {
		t.Fatalf("resultado: %+v", res)
	}
	if len(a.validaciones) != 1 || a.validaciones[0].VehiculoREPUVE != "NISSAN" {
		t.Fatalf("bitácora: %+v", a.validaciones)
	}
	if string(an.reverso) == "" {
		t.Fatal("no se envió el reverso de la INE al analizador")
	}
	if _, ok := s.Ultimo(folio); !ok {
		t.Fatal("no quedó en el historial")
	}
}

// El analizador fecha la consulta cuando la hace, después de que empezó la validación.
func TestApruebaConConsultaPosteriorAlInicio(t *testing.T) {
	reloj := ahora
	rp := &repuveFalso{r: repuve.Resultado{Estatus: modelo.REPUVESinReporte,
		Vehiculo: &repuve.Vehiculo{Placa: "XYZ123A", NIV: niv, Marca: "NISSAN"}}}
	s, _, _ := preparar(rp)
	s.o.Ahora = func() time.Time { reloj = reloj.Add(10 * time.Second); return reloj }
	// La consulta tarda 2 minutos (más que la tolerancia de reloj) y se fecha al terminar.
	rp.fecha = func() time.Time { reloj = reloj.Add(2 * time.Minute); return reloj }
	inf, err := s.Validar(context.Background(), folio, "webhook")
	if err != nil || inf.Dictamen.Decision != modelo.DecisionAprobar {
		t.Fatalf("se esperaba APROBAR: %+v %v", inf.Dictamen, err)
	}
}

func TestNoTocaSolicitudAprobada(t *testing.T) {
	s, a, _ := preparar(nil)
	a.resultados[folio] = modelo.Resultado{SolicitudID: folio, Estado: modelo.EstadoAprobada}
	inf, err := s.Validar(context.Background(), folio, "api")
	if err != nil || inf.Omitida == "" || inf.Guardado {
		t.Fatalf("inf=%+v err=%v", inf, err)
	}
}

func TestNoPisaRevisionHumana(t *testing.T) {
	s, a, _ := preparar(nil)
	a.revisiones[folio] = modelo.Revision{SolicitudID: folio, Revisor: "revisora@uat.edu.mx", Decision: modelo.DecisionEnRevision}
	inf, err := s.Validar(context.Background(), folio, "barrido")
	if err != nil || inf.Omitida == "" {
		t.Fatalf("inf=%+v err=%v", inf, err)
	}
	if a.revisiones[folio].Revisor != "revisora@uat.edu.mx" {
		t.Fatal("se sobrescribió la revisión humana")
	}
}

func TestActualizaSuPropiaRevision(t *testing.T) {
	s, a, _ := preparar(nil)
	a.revisiones[folio] = modelo.Revision{SolicitudID: folio, Revisor: sistema, Decision: modelo.DecisionEnRevision, REPUVE: modelo.REPUVENoDisponible}
	a.resultados[folio] = modelo.Resultado{SolicitudID: folio, Estado: modelo.EstadoEnRevision}
	if _, err := s.Validar(context.Background(), folio, "barrido"); err != nil {
		t.Fatal(err) // el doble falla si se usa Add sobre una fila existente
	}
	if a.resultados[folio].Estado != modelo.EstadoAprobada {
		t.Fatalf("resultado: %+v", a.resultados[folio])
	}
}

func TestFallaDeInfraestructuraNoEscribe(t *testing.T) {
	s, a, an := preparar(nil)
	an.err = errors.New("connection refused")
	if _, err := s.Validar(context.Background(), folio, "webhook"); err == nil {
		t.Fatal("se esperaba error")
	}
	if len(a.revisiones) != 0 || len(a.resultados) != 0 {
		t.Fatal("no debió escribir en AppSheet")
	}

	s2, a2, _ := preparar(nil)
	s2.imagenes = imagenesFalsas{err: errors.New("drive: HTTP 500")}
	if _, err := s2.Validar(context.Background(), folio, "webhook"); err == nil || len(a2.resultados) != 0 {
		t.Fatal("una falla de Drive no debe publicar resultado")
	}
}

func TestSolicitudInexistente(t *testing.T) {
	s, _, _ := preparar(nil)
	_, err := s.Validar(context.Background(), "11111111-2222-4333-8444-555555555555", "api")
	if !errors.Is(err, ErrNoEncontrada) {
		t.Fatalf("err = %v", err)
	}
	if _, err := s.Validar(context.Background(), "no-es-uuid", "api"); !errors.Is(err, ErrFolio) {
		t.Fatalf("err = %v", err)
	}
}

func TestPendientesReintentaREPUVE(t *testing.T) {
	s, a, _ := preparar(&repuveFalso{r: repuve.Resultado{Estatus: modelo.REPUVENoDisponible, Detalle: "timeout"}})
	ids, _ := s.Pendientes(context.Background(), 14)
	if len(ids) != 1 {
		t.Fatalf("pendientes iniciales: %v", ids)
	}
	if _, err := s.Validar(context.Background(), folio, "barrido"); err != nil {
		t.Fatal(err)
	}
	if a.resultados[folio].Estado != modelo.EstadoEnRevision {
		t.Fatalf("resultado: %+v", a.resultados[folio])
	}
	for i := range maxReintentosREPUVE {
		if ids, _ := s.Pendientes(context.Background(), 14); len(ids) != 1 {
			t.Fatalf("reintento %d: %v", i, ids)
		}
	}
	if ids, _ := s.Pendientes(context.Background(), 14); len(ids) != 0 {
		t.Fatalf("debió agotar los reintentos: %v", ids)
	}
}

func TestPendientesIgnoraAntiguasYDevueltas(t *testing.T) {
	s, a, _ := preparar(nil)
	viejo := a.solicitudes[folio]
	viejo.FechaEnvio = ahora.AddDate(0, 0, -30)
	a.solicitudes[folio] = viejo
	if ids, _ := s.Pendientes(context.Background(), 14); len(ids) != 0 {
		t.Fatalf("pendientes: %v", ids)
	}
}

func TestColaDeduplica(t *testing.T) {
	s, _, _ := preparar(nil)
	c := NuevaCola(s, s.log)
	if !c.Encolar(folio, "webhook") || c.Encolar(folio, "webhook") {
		t.Fatal("la cola debe ignorar el segundo envío del mismo folio")
	}
}
