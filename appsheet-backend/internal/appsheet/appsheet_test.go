package appsheet

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"registrovehicular/internal/config"
	"registrovehicular/internal/modelo"
)

const folio = "6f1c2d3e-4a5b-4c6d-8e7f-001122334455"

type peticion struct {
	Tabla  string
	Cuerpo cuerpo
}

func servidorFalso(t *testing.T, responder func(p peticion) (int, string)) (*httptest.Server, *[]peticion) {
	t.Helper()
	var recibidas []peticion
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("ApplicationAccessKey") != "llave" {
			t.Errorf("falta ApplicationAccessKey")
		}
		partes := strings.Split(r.URL.Path, "/")
		// /api/v2/apps/{app}/tables/{tabla}/Action
		if len(partes) != 8 || partes[4] != "app-1" || partes[7] != "Action" {
			t.Errorf("ruta inesperada %s", r.URL.Path)
		}
		var c cuerpo
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			t.Errorf("cuerpo: %v", err)
		}
		p := peticion{Tabla: partes[6], Cuerpo: c}
		recibidas = append(recibidas, p)
		estado, respuesta := responder(p)
		w.WriteHeader(estado)
		_, _ = w.Write([]byte(respuesta))
	}))
	t.Cleanup(srv.Close)
	return srv, &recibidas
}

func repo(srv *httptest.Server) *Repositorio {
	c := NuevoCliente(Opciones{BaseURL: srv.URL, AppID: "app-1", AccessKey: "llave", RunAs: "backend@uat.edu.mx", Locale: "en-US", Timezone: "Central Standard Time (Mexico)"})
	c.espera = time.Millisecond
	zona, _ := time.LoadLocation("America/Monterrey")
	return NuevoRepositorio(c, config.Tablas{Solicitudes: "Solicitudes", Alumnos: "Alumnos", Revisiones: "Revisiones", Resultados: "Resultados"}, zona, "en-US")
}

func TestLeerSolicitud(t *testing.T) {
	srv, recibidas := servidorFalso(t, func(p peticion) (int, string) {
		return 200, `[{"SolicitudId":"` + folio + `","Correo":"Alumno@UAT.edu.mx","FechaEnvio":"11/20/2026 07:42:10","Placa":"xyz-123-a","NIV":"1HGCM82633A004352","FotoINE":"Solicitudes_Images/a.FotoINE.jpg","AvisoLeido":"Y","_RowNumber":2}]`
	})
	s, err := repo(srv).Solicitud(context.Background(), folio)
	if err != nil || s == nil {
		t.Fatalf("Solicitud: %v %v", s, err)
	}
	if s.Correo != "alumno@uat.edu.mx" || !s.AvisoLeido || s.FechaEnvio.Day() != 20 || s.FechaEnvio.Hour() != 7 {
		t.Fatalf("mapeo incorrecto: %+v", s)
	}
	p := (*recibidas)[0]
	if p.Cuerpo.Action != "Find" || p.Cuerpo.Properties.RunAsUserEmail != "backend@uat.edu.mx" ||
		p.Cuerpo.Properties.Selector != `Filter(Solicitudes, [SolicitudId] = "`+folio+`")` {
		t.Fatalf("petición: %+v", p.Cuerpo)
	}
}

func TestClavesDuplicadas(t *testing.T) {
	srv, _ := servidorFalso(t, func(p peticion) (int, string) {
		return 200, `[{"SolicitudId":"` + folio + `"},{"SolicitudId":"` + folio + `"}]`
	})
	if _, err := repo(srv).Solicitud(context.Background(), folio); err == nil {
		t.Fatal("se esperaba error por claves duplicadas")
	}
}

func TestFolioInvalidoNoLlegaALaExpresion(t *testing.T) {
	srv, recibidas := servidorFalso(t, func(p peticion) (int, string) { return 200, `[]` })
	if _, err := repo(srv).Solicitud(context.Background(), `x") OR TRUE OR ("`); err == nil {
		t.Fatal("se esperaba error")
	}
	if len(*recibidas) != 0 {
		t.Fatal("no debió llamar a AppSheet")
	}
}

func TestGuardarResultadoFormatoFecha(t *testing.T) {
	srv, recibidas := servidorFalso(t, func(p peticion) (int, string) { return 200, `{"Rows":[]}` })
	zona, _ := time.LoadLocation("America/Monterrey")
	err := repo(srv).GuardarResultado(context.Background(), modelo.Resultado{
		SolicitudID: folio, Correo: "a@uat.edu.mx", Estado: modelo.EstadoAprobada, Indicacion: modelo.IndicacionAprobada,
		ActualizadoEn: time.Date(2026, 11, 21, 9, 5, 0, 0, zona),
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	p := (*recibidas)[0]
	if p.Tabla != "Resultados" || p.Cuerpo.Action != "Add" || p.Cuerpo.Rows[0]["ActualizadoEn"] != "11/21/2026 09:05:00" {
		t.Fatalf("petición: %+v", p)
	}
}

func TestReintentaErroresTemporales(t *testing.T) {
	var intentos atomic.Int32
	srv, _ := servidorFalso(t, func(p peticion) (int, string) {
		if intentos.Add(1) < 3 {
			return 503, "ocupado"
		}
		return 200, `[]`
	})
	if _, err := repo(srv).Solicitudes(context.Background()); err != nil {
		t.Fatal(err)
	}
	if intentos.Load() != 3 {
		t.Fatalf("intentos = %d", intentos.Load())
	}
}

func TestNoReintentaErroresPermanentes(t *testing.T) {
	var intentos atomic.Int32
	srv, _ := servidorFalso(t, func(p peticion) (int, string) {
		intentos.Add(1)
		return 403, "API not enabled"
	})
	_, err := repo(srv).Solicitudes(context.Background())
	if err == nil || intentos.Load() != 1 {
		t.Fatalf("err=%v intentos=%d", err, intentos.Load())
	}
}

func TestAprobadasConVehiculo(t *testing.T) {
	otro := "11111111-2222-4333-8444-555555555555"
	srv, recibidas := servidorFalso(t, func(p peticion) (int, string) {
		if p.Tabla == "Solicitudes" {
			return 200, `[{"SolicitudId":"` + otro + `","Placa":"XYZ 123 A","NIV":"OTRO"}]`
		}
		return 200, `[{"SolicitudId":"` + otro + `","Estado":"APROBADA"}]`
	})
	dup, err := repo(srv).AprobadasConVehiculo(context.Background(), "XYZ123A", "1HGCM82633A004352", folio)
	if err != nil || len(dup) != 1 || dup[0].SolicitudID != otro {
		t.Fatalf("dup=%+v err=%v", dup, err)
	}
	if sel := (*recibidas)[1].Cuerpo.Properties.Selector; !strings.Contains(sel, `IN([SolicitudId], LIST("`+otro+`"))`) {
		t.Fatalf("selector %s", sel)
	}
}

func TestGuardarAlumno(t *testing.T) {
	srv, recibidas := servidorFalso(t, func(p peticion) (int, string) { return 200, `{"Rows":[]}` })
	zona, _ := time.LoadLocation("America/Monterrey")
	err := repo(srv).GuardarAlumno(context.Background(), modelo.Alumno{
		Correo: "a1@alumnos.uat.edu.mx", Matricula: "a1", NombreCompleto: "ANA PÉREZ", Activo: true,
		Programa: "ISC", SituacionUAT: "ACTIVO", VerificadoUAT: time.Date(2026, 11, 21, 9, 5, 0, 0, zona),
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	p := (*recibidas)[0]
	f := p.Cuerpo.Rows[0]
	if p.Tabla != "Alumnos" || p.Cuerpo.Action != "Add" || f["Activo"] != "TRUE" || f["VerificadoUAT"] != "11/21/2026 09:05:00" || f["SituacionUAT"] != "ACTIVO" {
		t.Errorf("%s %s %v", p.Tabla, p.Cuerpo.Action, f)
	}
}

func TestSinCredencialesNoLlamaAAppSheet(t *testing.T) {
	c := NuevoCliente(Opciones{BaseURL: "http://127.0.0.1:1"})
	if _, err := c.Buscar(context.Background(), "Alumnos", ""); err != ErrNoConfigurado {
		t.Fatalf("%v", err)
	}
}
