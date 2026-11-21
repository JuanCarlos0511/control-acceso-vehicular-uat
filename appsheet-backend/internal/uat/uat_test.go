package uat

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNormalizarUsuario(t *testing.T) {
	casos := []struct {
		entrada, esperado string
		err               bool
	}{
		{"a2100000001", "a2100000001@alumnos.uat.edu.mx", false},
		{"  A2100000001@Alumnos.UAT.edu.mx ", "a2100000001@alumnos.uat.edu.mx", false},
		{"profesor@uat.edu.mx", "", true},
		{"x@alumnos.uat.edu.mx.evil.com", "", true},
		{"", "", true},
		{"a\"b@alumnos.uat.edu.mx", "", true},
	}
	for _, c := range casos {
		got, err := NormalizarUsuario(c.entrada, "alumnos.uat.edu.mx")
		if (err != nil) != c.err || got != c.esperado {
			t.Errorf("NormalizarUsuario(%q) = %q, %v", c.entrada, got, err)
		}
	}
}

const respuestaPortal = `{
  "source": "UAT", "authenticated": true,
  "login": {"exito": true},
  "selectedCareer": {"exito": true, "parametros": {"Id_Plan_Estudio_AlumnosUAT": 3314}},
  "programs": [
    {"planId": 1200, "program": "BACHILLERATO", "situation": "EGRESADO", "situationFields": [],
     "raw": {"Num_Matricula": "a2100000001", "Txt_Nombre": "ANA", "Txt_Paterno": "PÉREZ", "Txt_Materno": "LÓPEZ"}},
    {"planId": 3314, "program": "INGENIERÍA EN SISTEMAS  COMPUTACIONALES", "situation": "Activo", "situationFields": [],
     "raw": {"Num_Matricula": 2100000001, "Txt_Nombre": "ANA MARÍA", "Txt_Paterno": "PÉREZ", "Txt_Materno": "LÓPEZ"}}
  ],
  "situations": ["EGRESADO", "Activo"]
}`

func TestVerificar(t *testing.T) {
	var recibido map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/alumnos/scrape" || r.Header.Get("X-API-Key") != "clave" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("X-Forwarded-For") != "203.0.113.7" {
			t.Errorf("X-Forwarded-For = %q", r.Header.Get("X-Forwarded-For"))
		}
		_ = json.NewDecoder(r.Body).Decode(&recibido)
		_, _ = w.Write([]byte(respuestaPortal))
	}))
	defer srv.Close()

	a, err := NuevoCliente(srv.URL, "clave", 5*time.Second).Verificar(context.Background(), "a2100000001@alumnos.uat.edu.mx", "secreta", "203.0.113.7")
	if err != nil {
		t.Fatal(err)
	}
	if a.PlanID != 3314 || a.Situacion != "ACTIVO" || a.Programa != "INGENIERÍA EN SISTEMAS COMPUTACIONALES" {
		t.Errorf("plan elegido: %+v", a)
	}
	if a.Nombre != "ANA MARÍA PÉREZ LÓPEZ" || a.Matricula != "2100000001" {
		t.Errorf("identidad: %+v", a)
	}
	inc, _ := recibido["include"].(map[string]any)
	if recibido["password"] != "secreta" || inc["schedule"] != false || inc["finalGrades"] != false {
		t.Errorf("cuerpo enviado: %v", recibido)
	}
}

func TestVerificarErrores(t *testing.T) {
	casos := []struct {
		estado int
		cuerpo string
		quiere func(error) bool
	}{
		{401, `{"error":"UAT_LOGIN_FAILED","message":"x"}`, func(e error) bool { return errors.Is(e, ErrCredenciales) }},
		{429, `{"error":"RATE_LIMIT"}`, func(e error) bool { return errors.Is(e, ErrLimite) }},
		{400, `{"error":"VALIDATION_ERROR"}`, func(e error) bool { return errors.Is(e, ErrUsuario) }},
		{401, `{"error":"GATEWAY_UNAUTHORIZED"}`, func(e error) bool { var s *ErrServicio; return errors.As(e, &s) }},
		{502, `{"error":"UAT_PORTAL_ERROR"}`, func(e error) bool { var s *ErrServicio; return errors.As(e, &s) && s.Codigo == "UAT_PORTAL_ERROR" }},
		{200, `{"authenticated": false}`, func(e error) bool { var s *ErrServicio; return errors.As(e, &s) }},
	}
	for _, c := range casos {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(c.estado)
			_, _ = w.Write([]byte(c.cuerpo))
		}))
		_, err := NuevoCliente(srv.URL, "k", time.Second).Verificar(context.Background(), "a1@alumnos.uat.edu.mx", "p", "")
		if !c.quiere(err) {
			t.Errorf("HTTP %d %s → %v", c.estado, c.cuerpo, err)
		}
		srv.Close()
	}
}

func TestInterpretarSinNombreNiPlan(t *testing.T) {
	var r respuesta
	_ = json.Unmarshal([]byte(`{"authenticated":true,"programs":[{"planId":5,"program":"LIC","situation":null,"raw":{}}],"situations":["INSCRITO"],
	  "student":{"firstName":"Luis","paternalSurname":"Gómez","maternalSurname":""}}`), &r)
	a := interpretar("a9@alumnos.uat.edu.mx", r)
	if a.Situacion != "INSCRITO" || a.Matricula != "a9" || a.Nombre != "LUIS GÓMEZ" {
		t.Errorf("%+v", a)
	}
}
