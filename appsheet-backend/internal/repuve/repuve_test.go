package repuve

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"registrovehicular/internal/modelo"
)

func TestClienteConsulta(t *testing.T) {
	var recibido map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/repuve" || r.Header.Get("X-Token-Analizador") != "tok" {
			t.Errorf("petición inesperada: %s %v", r.URL.Path, r.Header)
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &recibido)
		_, _ = w.Write([]byte(`{"estatus":"SIN_REPORTE","criterio":"2","valor":"1HGCM82633A004352",
			"consultadoEn":"2026-11-21T10:00:00-06:00","vehiculo":{"placa":"XYZ123A","niv":"1HGCM82633A004352","marca":"NISSAN",
			"modelo":"VERSA","linea":"","anioModelo":"2019","clase":"","tipo":"","entidad":""},"reportes":[]}`))
	}))
	defer srv.Close()

	r, err := NuevoCliente(srv.URL, "tok", time.Minute).Consultar(context.Background(), Consulta{Criterio: CriterioNIV, Valor: "1hgcm8263 3a004352"})
	if err != nil {
		t.Fatal(err)
	}
	if recibido["niv"] != "1HGCM82633A004352" {
		t.Fatalf("NIV enviado: %v", recibido)
	}
	if r.Estatus != modelo.REPUVESinReporte || r.Vehiculo.Resumen() != "NISSAN VERSA 2019" || r.ConsultadoEn.IsZero() {
		t.Fatalf("resultado: %+v", r)
	}
}

func TestClienteErrores(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var c map[string]string
		_ = json.NewDecoder(r.Body).Decode(&c)
		if c["niv"] == "MAL" {
			w.WriteHeader(422)
			_, _ = w.Write([]byte(`{"error":{"codigo":"CONSULTA_INVALIDA","mensaje":"El NIV debe tener 17 caracteres"}}`))
			return
		}
		w.WriteHeader(500)
	}))
	defer srv.Close()
	c := NuevoCliente(srv.URL, "", time.Minute)

	_, err := c.Consultar(context.Background(), Consulta{Criterio: CriterioNIV, Valor: "MAL"})
	var ec *ErrConsulta
	if !errors.As(err, &ec) || ec.Mensaje == "" {
		t.Fatalf("err = %v", err)
	}
	if _, err := c.Consultar(context.Background(), Consulta{Criterio: CriterioNIV, Valor: "1HGCM82633A004352"}); err == nil || errors.As(err, &ec) {
		t.Fatalf("un 500 debe ser error de infraestructura: %v", err)
	}
}
