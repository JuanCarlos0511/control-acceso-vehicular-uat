package servidor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"registrovehicular/internal/registro"
	"registrovehicular/internal/uat"
)

type registradorFalso struct{ ip, usuario string }

func (r *registradorFalso) Registrar(_ context.Context, usuario, contrasena, ip string) (*registro.Resultado, error) {
	r.ip, r.usuario = ip, usuario
	switch contrasena {
	case "mala":
		return nil, uat.ErrCredenciales
	case "caida":
		return nil, &uat.ErrServicio{Estado: 502, Codigo: "UAT_PORTAL_ERROR"}
	}
	return &registro.Resultado{Correo: "a1@alumnos.uat.edu.mx", Activo: true, Guardado: true}, nil
}

const frontend = "https://hackathonfrontend.452111.xyz"

func prepararAlumnos() (http.Handler, *registradorFalso) {
	r := &registradorFalso{}
	h := Nuevo(&validadorFalso{}, &colaFalsa{}, Opciones{
		WebhookSecreto: secreto, APIToken: token, Registro: r, CORSOrigenes: []string{frontend},
	}).Rutas()
	return h, r
}

func pedirAlumno(cuerpo, origen string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/alumnos/verificar", strings.NewReader(cuerpo))
	req.Header.Set("Content-Type", "application/json")
	if origen != "" {
		req.Header.Set("Origin", origen)
	}
	req.Header.Set("CF-Connecting-IP", "203.0.113.9")
	return req
}

func TestVerificarAlumno(t *testing.T) {
	h, r := prepararAlumnos()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, pedirAlumno(`{"usuario":"a1","contrasena":"buena"}`, frontend))
	if rec.Code != http.StatusOK || rec.Header().Get("Access-Control-Allow-Origin") != frontend {
		t.Fatalf("estado %d, CORS %q: %s", rec.Code, rec.Header().Get("Access-Control-Allow-Origin"), rec.Body)
	}
	if r.ip != "203.0.113.9" || r.usuario != "a1" {
		t.Errorf("ip %q usuario %q", r.ip, r.usuario)
	}
}

func TestVerificarAlumnoErrores(t *testing.T) {
	h, _ := prepararAlumnos()
	for contrasena, quiere := range map[string]struct {
		estado int
		codigo string
	}{"mala": {401, "CREDENCIALES"}, "caida": {502, "UAT_NO_DISPONIBLE"}} {
		estado, s := llamar(t, h, pedirAlumno(`{"usuario":"a1","contrasena":"`+contrasena+`"}`, frontend))
		if estado != quiere.estado || s.Error == nil || s.Error.Codigo != quiere.codigo {
			t.Errorf("%s: %d %+v", contrasena, estado, s.Error)
		}
	}
	if estado, _ := llamar(t, h, pedirAlumno(`no es json`, frontend)); estado != http.StatusBadRequest {
		t.Errorf("cuerpo inválido: %d", estado)
	}
}

func TestCORSOrigenNoPermitido(t *testing.T) {
	h, _ := prepararAlumnos()
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/alumnos/verificar", nil)
	req.Header.Set("Origin", "https://otro.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("preflight de otro origen: %d %q", rec.Code, rec.Header().Get("Access-Control-Allow-Origin"))
	}
	req.Header.Set("Origin", frontend)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Fatalf("preflight del frontend: %d", rec.Code)
	}
}

func TestRegistroDeshabilitado(t *testing.T) {
	h, _, _ := preparar()
	if estado, s := llamar(t, h, pedirAlumno(`{"usuario":"a1","contrasena":"x"}`, "")); estado != http.StatusServiceUnavailable || s.Error.Codigo != "NO_DISPONIBLE" {
		t.Fatalf("%d %+v", estado, s.Error)
	}
}
