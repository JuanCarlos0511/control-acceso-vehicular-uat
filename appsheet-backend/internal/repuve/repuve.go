// Package repuve define el resultado de la consulta a REPUVE y el cliente que
// la pide al analizador. La navegación con Playwright y la interpretación de
// las respuestas del portal están en analizador/app/repuve.py.
package repuve

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"registrovehicular/internal/modelo"
)

// Criterios de búsqueda del portal.
const (
	CriterioPlaca = "1"
	CriterioNIV   = "2"
)

type Consulta struct {
	Criterio string // CriterioNIV o CriterioPlaca
	Valor    string
}

type Vehiculo struct {
	Placa      string `json:"placa"`
	NIV        string `json:"niv"`
	Marca      string `json:"marca"`
	Modelo     string `json:"modelo"`
	Linea      string `json:"linea"`
	AnioModelo string `json:"anioModelo"`
	Clase      string `json:"clase"`
	Tipo       string `json:"tipo"`
	Entidad    string `json:"entidad"`
}

// Resumen es una descripción corta del vehículo, sin datos personales.
func (v Vehiculo) Resumen() string {
	partes := []string{}
	for _, p := range []string{v.Marca, v.Modelo, v.Linea, v.AnioModelo} {
		if p = strings.TrimSpace(p); p != "" {
			partes = append(partes, p)
		}
	}
	return strings.Join(partes, " ")
}

type Reporte struct {
	Fuente     string `json:"fuente"` // PGJ, OCRA, AVISO_JUDICIAL
	Estatus    string `json:"estatus"`
	Entidad    string `json:"entidad,omitempty"`
	Fecha      string `json:"fecha,omitempty"`
	Recuperado bool   `json:"recuperado"`
}

type Resultado struct {
	Estatus      string    `json:"estatus"` // valores modelo.REPUVE*
	Criterio     string    `json:"criterio"`
	Valor        string    `json:"valor"`
	ConsultadoEn time.Time `json:"consultadoEn"`
	Vehiculo     *Vehiculo `json:"vehiculo"`
	Reportes     []Reporte `json:"reportes"`
	Detalle      string    `json:"detalle,omitempty"`
	Advertencias []string  `json:"advertencias,omitempty"`
}

// Consultor consulta REPUVE. Un resultado NO_DISPONIBLE no es un error: el
// portal falló o rechazó la sesión, y la revisión queda para una persona.
type Consultor interface {
	Consultar(ctx context.Context, q Consulta) (Resultado, error)
}

// Cliente pide la consulta al analizador (POST /v1/repuve), que maneja el
// navegador con Playwright, hace una consulta a la vez y guarda caché.
type Cliente struct {
	base  string
	token string
	http  *http.Client
}

// NuevoCliente: espera es el tiempo máximo para la respuesta, incluida la fila
// de consultas pendientes en el analizador.
func NuevoCliente(base, token string, espera time.Duration) *Cliente {
	return &Cliente{base: strings.TrimRight(base, "/"), token: token, http: &http.Client{Timeout: espera}}
}

// ErrConsulta indica que el analizador rechazó los datos de la consulta.
type ErrConsulta struct{ Mensaje string }

func (e *ErrConsulta) Error() string { return "consulta REPUVE inválida: " + e.Mensaje }

func (c *Cliente) Consultar(ctx context.Context, q Consulta) (Resultado, error) {
	cuerpo := map[string]string{}
	switch q.Criterio {
	case CriterioNIV:
		cuerpo["niv"] = modelo.NormalizarNIV(q.Valor)
	case CriterioPlaca:
		cuerpo["placa"] = modelo.NormalizarPlaca(q.Valor)
	default:
		return Resultado{}, &ErrConsulta{Mensaje: "criterio no soportado"}
	}
	carga, _ := json.Marshal(cuerpo)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/v1/repuve", bytes.NewReader(carga))
	if err != nil {
		return Resultado{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("X-Token-Analizador", c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return Resultado{}, fmt.Errorf("analizador (REPUVE): %w", err)
	}
	defer resp.Body.Close()
	datos, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Resultado{}, err
	}
	switch {
	case resp.StatusCode == http.StatusUnprocessableEntity:
		var e struct {
			Error struct {
				Mensaje string `json:"mensaje"`
			} `json:"error"`
		}
		_ = json.Unmarshal(datos, &e)
		return Resultado{}, &ErrConsulta{Mensaje: e.Error.Mensaje}
	case resp.StatusCode != http.StatusOK:
		return Resultado{}, fmt.Errorf("analizador (REPUVE): HTTP %d", resp.StatusCode)
	}
	var r Resultado
	if err := json.Unmarshal(datos, &r); err != nil {
		return Resultado{}, fmt.Errorf("analizador (REPUVE): respuesta no reconocida: %w", err)
	}
	return r, nil
}
