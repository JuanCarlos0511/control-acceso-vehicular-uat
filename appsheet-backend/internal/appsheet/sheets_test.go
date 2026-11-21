package appsheet

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"registrovehicular/internal/config"
	"registrovehicular/internal/modelo"
)

type transporteSheetsPrueba func(*http.Request) (*http.Response, error)

func (f transporteSheetsPrueba) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func respuestaSheetsPrueba(cuerpo string) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(cuerpo))}, nil
}

func TestSheetsLeeSolicitudDelFormulario(t *testing.T) {
	id := "a4a551df-8ca8-49e9-b113-08830def89b3"
	transport := transporteSheetsPrueba(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || r.URL.Path != "/values/'Solicitudes'" || r.URL.Query().Get("valueRenderOption") != "UNFORMATTED_VALUE" {
			t.Fatalf("lectura inesperada: %s %s", r.Method, r.URL)
		}
		return respuestaSheetsPrueba(`{"values":[["Correo","FechaEnvio","SolicitudId","Placa","AvisoLeido","FotoINE"],
          ["otro@example.invalid",36526.5,"otro-folio"],
          [],
          ["alumno@example.invalid",36526.25,"` + id + `","001234",true,"FotosRegistroVehicularUAT/ine.jpg"]]}`)
	})
	zona := time.FixedZone("Campus", -6*3600)
	tablas := config.Tablas{Solicitudes: "Solicitudes"}
	cliente := nuevoClienteSheets("https://sheets.example.invalid", &http.Client{Transport: transport}, tablas, zona)
	repo := NuevoRepositorioSheets(cliente, tablas, zona, "en-US")
	solicitud, err := repo.Solicitud(context.Background(), id)
	if err != nil || solicitud == nil {
		t.Fatalf("no recuperó la solicitud: %v", err)
	}
	if solicitud.Correo != "alumno@example.invalid" || solicitud.Placa != "001234" ||
		!solicitud.AvisoLeido || solicitud.FotoINE != "FotosRegistroVehicularUAT/ine.jpg" ||
		solicitud.FechaEnvio.Format(time.RFC3339) != "2000-01-01T06:00:00-06:00" {
		t.Fatalf("solicitud recuperada de forma incorrecta: %+v", solicitud)
	}
}

func TestSheetsPublicaResultadoSinInterpretarTextoComoFormula(t *testing.T) {
	id := "a4a551df-8ca8-49e9-b113-08830def89b3"
	type escritura struct {
		SheetID int
		Start   struct{ RowIndex, ColumnIndex int }
		Rows    []struct {
			Values []struct{ UserEnteredValue map[string]any }
		}
	}
	var peticiones []map[string]escritura
	existe := false
	transport := transporteSheetsPrueba(func(r *http.Request) (*http.Response, error) {
		switch r.Method + " " + r.URL.Path {
		case "POST /archivo:batchUpdate":
			var carga struct{ Requests []map[string]escritura }
			if err := json.NewDecoder(r.Body).Decode(&carga); err != nil {
				t.Fatal(err)
			}
			peticiones = append(peticiones, carga.Requests...)
			return respuestaSheetsPrueba(`{}`)
		case "GET /archivo/values/'Resultados'":
			const encabezados = `["Estado","Indicacion","SolicitudId","Correo","ActualizadoEn","NotaPrivada"]`
			if existe {
				return respuestaSheetsPrueba(`{"values":[` + encabezados + `,[],["EN_REVISION","","` + id + `","alumno@example.invalid",36526.25,"no tocar"]]}`)
			}
			return respuestaSheetsPrueba(`{"values":[` + encabezados + `]}`)
		case "GET /archivo":
			return respuestaSheetsPrueba(`{"sheets":[{"properties":{"sheetId":123,"title":"Resultados"}}]}`)
		default:
			t.Fatalf("petición inesperada: %s %s", r.Method, r.URL)
		}
		return nil, nil
	})
	zona := time.FixedZone("Campus", -6*3600)
	tablas := config.Tablas{Resultados: "Resultados"}
	cliente := nuevoClienteSheets("https://sheets.example.invalid/archivo", &http.Client{Transport: transport}, tablas, zona)
	repo := NuevoRepositorioSheets(cliente, tablas, zona, "en-US")
	resultado := modelo.Resultado{
		SolicitudID: id, Correo: "alumno@example.invalid", Estado: "EN_REVISION",
		Indicacion: "=2+2", ActualizadoEn: time.Date(2000, 1, 1, 6, 0, 0, 0, zona),
	}
	if err := repo.GuardarResultado(context.Background(), resultado, true); err != nil {
		t.Fatal(err)
	}
	alta := peticiones[0]["appendCells"]
	if alta.SheetID != 123 || alta.Rows[0].Values[1].UserEnteredValue["stringValue"] != "=2+2" ||
		alta.Rows[0].Values[4].UserEnteredValue["numberValue"] != 36526.25 {
		t.Fatalf("alta de resultado incorrecta: %+v", alta)
	}
	existe = true
	peticiones = nil
	if err := repo.GuardarResultado(context.Background(), resultado, false); err != nil {
		t.Fatal(err)
	}
	if len(peticiones) != 5 {
		t.Fatalf("se esperaban cinco celdas gestionadas; se obtuvieron %d", len(peticiones))
	}
	for _, req := range peticiones {
		edicion := req["updateCells"]
		if edicion.Start.RowIndex != 2 || edicion.Start.ColumnIndex >= 5 {
			t.Fatalf("edición fuera del expediente o sobre nota privada: %+v", edicion)
		}
	}
	if err := repo.GuardarResultado(context.Background(), resultado, true); err == nil {
		t.Fatal("se permitió agregar un folio duplicado")
	}
}

func TestSheetsRegistraYActualizaAlumno(t *testing.T) {
	correo := "a2100000001@alumnos.uat.edu.mx"
	var celdas []map[string]any
	var ediciones int
	existe := false
	transport := transporteSheetsPrueba(func(r *http.Request) (*http.Response, error) {
		switch r.Method + " " + r.URL.Path {
		case "GET /archivo/values/'Alumnos'":
			const enc = `["Correo","Matricula","NombreCompleto","Activo","Programa","SituacionUAT","VerificadoUAT"]`
			if existe {
				return respuestaSheetsPrueba(`{"values":[` + enc + `,["` + correo + `","a2100000001","ANA PÉREZ",true,"ISC","ACTIVO",46347.4]]}`)
			}
			return respuestaSheetsPrueba(`{"values":[` + enc + `]}`)
		case "GET /archivo":
			return respuestaSheetsPrueba(`{"sheets":[{"properties":{"sheetId":7,"title":"Alumnos"}}]}`)
		case "POST /archivo:batchUpdate":
			var carga struct {
				Requests []struct {
					AppendCells *struct {
						Rows []struct {
							Values []struct{ UserEnteredValue map[string]any }
						}
					}
					UpdateCells *struct{}
				}
			}
			if err := json.NewDecoder(r.Body).Decode(&carga); err != nil {
				t.Fatal(err)
			}
			for _, req := range carga.Requests {
				if req.AppendCells != nil {
					for _, v := range req.AppendCells.Rows[0].Values {
						celdas = append(celdas, v.UserEnteredValue)
					}
				}
				if req.UpdateCells != nil {
					ediciones++
				}
			}
			return respuestaSheetsPrueba(`{}`)
		}
		t.Fatalf("petición inesperada: %s %s", r.Method, r.URL)
		return nil, nil
	})
	zona := time.FixedZone("Campus", -6*3600)
	tablas := config.Tablas{Alumnos: "Alumnos"}
	cliente := nuevoClienteSheets("https://sheets.example.invalid/archivo", &http.Client{Transport: transport}, tablas, zona)
	repo := NuevoRepositorioSheets(cliente, tablas, zona, "en-US")
	ctx := context.Background()

	if a, err := repo.Alumno(ctx, correo); err != nil || a != nil {
		t.Fatalf("alumno inexistente: %+v %v", a, err)
	}
	alumno := modelo.Alumno{Correo: correo, Matricula: "a2100000001", NombreCompleto: "ANA PÉREZ", Activo: true,
		Programa: "ISC", SituacionUAT: "ACTIVO", VerificadoUAT: time.Date(2026, 11, 21, 9, 36, 0, 0, zona)}
	if err := repo.GuardarAlumno(ctx, alumno, true); err != nil {
		t.Fatal(err)
	}
	if len(celdas) != 7 || celdas[0]["stringValue"] != correo || celdas[3]["boolValue"] != true || celdas[6]["numberValue"] == nil {
		t.Fatalf("alta en Alumnos: %v", celdas)
	}

	existe = true
	a, err := repo.Alumno(ctx, correo)
	if err != nil || a == nil || !a.Activo || a.NombreCompleto != "ANA PÉREZ" || a.VerificadoUAT.IsZero() {
		t.Fatalf("lectura de Alumnos: %+v %v", a, err)
	}
	alumno.Activo, alumno.SituacionUAT = false, "BAJA"
	if err := repo.GuardarAlumno(ctx, alumno, false); err != nil || ediciones != 7 {
		t.Fatalf("edición: %v, %d celdas", err, ediciones)
	}
}
