// Package appsheet habla con la API v2 de AppSheet
// (POST /api/v2/apps/{appId}/tables/{tabla}/Action), que lee y escribe en la
// base de datos de la app, sea Google Sheets o AppSheet Database.
package appsheet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"
)

// Fila es un registro de AppSheet. La API lee y escribe todos los valores como
// texto, con el formato del Locale de la solicitud.
type Fila map[string]string

// ErrNoConfigurado: faltan APPSHEET_APP_ID, APPSHEET_ACCESS_KEY o APPSHEET_RUN_AS.
var ErrNoConfigurado = errors.New("appsheet: la integración no está configurada (APPSHEET_APP_ID, APPSHEET_ACCESS_KEY, APPSHEET_RUN_AS)")

// ErrAPI describe una respuesta de error de AppSheet.
type ErrAPI struct {
	Estado  int
	Mensaje string
}

func (e *ErrAPI) Error() string {
	return fmt.Sprintf("appsheet: HTTP %d: %s", e.Estado, e.Mensaje)
}

// Temporal indica si vale la pena reintentar.
func (e *ErrAPI) Temporal() bool {
	return e.Estado == http.StatusTooManyRequests || e.Estado >= 500
}

type Cliente struct {
	base       string
	appID      string
	accessKey  string
	runAs      string
	locale     string
	timezone   string
	http       *http.Client
	reintentos int
	espera     time.Duration
}

type Opciones struct {
	BaseURL   string
	AppID     string
	AccessKey string
	RunAs     string
	Locale    string
	Timezone  string
	HTTP      *http.Client
}

func NuevoCliente(o Opciones) *Cliente {
	h := o.HTTP
	if h == nil {
		h = &http.Client{Timeout: 45 * time.Second}
	}
	return &Cliente{
		base: o.BaseURL, appID: o.AppID, accessKey: o.AccessKey, runAs: o.RunAs,
		locale: o.Locale, timezone: o.Timezone, http: h, reintentos: 3, espera: time.Second,
	}
}

type propiedades struct {
	Locale         string `json:"Locale,omitempty"`
	Timezone       string `json:"Timezone,omitempty"`
	RunAsUserEmail string `json:"RunAsUserEmail,omitempty"`
	Selector       string `json:"Selector,omitempty"`
}

type cuerpo struct {
	Action     string      `json:"Action"`
	Properties propiedades `json:"Properties"`
	Rows       []Fila      `json:"Rows"`
}

// Buscar ejecuta la acción Find. Un selector vacío devuelve todas las filas
// visibles para RunAsUserEmail (respetando los filtros de seguridad).
func (c *Cliente) Buscar(ctx context.Context, tabla, selector string) ([]Fila, error) {
	return c.accion(ctx, tabla, "Find", selector, nil)
}

func (c *Cliente) Agregar(ctx context.Context, tabla string, filas ...Fila) ([]Fila, error) {
	return c.accion(ctx, tabla, "Add", "", filas)
}

func (c *Cliente) Editar(ctx context.Context, tabla string, filas ...Fila) ([]Fila, error) {
	return c.accion(ctx, tabla, "Edit", "", filas)
}

// Configurado indica si hay credenciales de AppSheet.
func (c *Cliente) Configurado() bool {
	return c.appID != "" && c.accessKey != "" && c.runAs != ""
}

func (c *Cliente) accion(ctx context.Context, tabla, accion, selector string, filas []Fila) ([]Fila, error) {
	if !c.Configurado() {
		return nil, ErrNoConfigurado
	}
	if filas == nil {
		filas = []Fila{}
	}
	carga, err := json.Marshal(cuerpo{
		Action: accion,
		Properties: propiedades{
			Locale: c.locale, Timezone: c.timezone, RunAsUserEmail: c.runAs, Selector: selector,
		},
		Rows: filas,
	})
	if err != nil {
		return nil, err
	}
	destino := fmt.Sprintf("%s/api/v2/apps/%s/tables/%s/Action",
		c.base, url.PathEscape(c.appID), url.PathEscape(tabla))

	var ultimo error
	for intento := 0; intento <= c.reintentos; intento++ {
		if intento > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.espera * time.Duration(1<<(intento-1))):
			}
		}
		res, err := c.enviar(ctx, destino, carga)
		if err == nil {
			return res, nil
		}
		ultimo = err
		var e *ErrAPI
		if errors.As(err, &e) && !e.Temporal() {
			break
		}
		if ctx.Err() != nil {
			break
		}
	}
	return nil, fmt.Errorf("%s en %s: %w", accion, tabla, ultimo)
}

func (c *Cliente) enviar(ctx context.Context, destino string, carga []byte) ([]Fila, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, destino, bytes.NewReader(carga))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("ApplicationAccessKey", c.accessKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	datos, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ErrAPI{Estado: resp.StatusCode, Mensaje: recortar(string(datos), 400)}
	}
	return decodificar(datos)
}

// decodificar acepta las dos formas de respuesta de la API: un arreglo de
// filas (Find) o un objeto {"Rows": [...]} (Add, Edit). Una respuesta vacía
// equivale a cero filas.
func decodificar(datos []byte) ([]Fila, error) {
	datos = bytes.TrimSpace(datos)
	if len(datos) == 0 {
		return nil, nil
	}
	var crudo []map[string]any
	switch datos[0] {
	case '[':
		if err := json.Unmarshal(datos, &crudo); err != nil {
			return nil, fmt.Errorf("appsheet: respuesta no reconocida: %w", err)
		}
	case '{':
		var envoltura struct {
			Rows []map[string]any `json:"Rows"`
		}
		if err := json.Unmarshal(datos, &envoltura); err != nil {
			return nil, fmt.Errorf("appsheet: respuesta no reconocida: %w", err)
		}
		crudo = envoltura.Rows
	default:
		return nil, fmt.Errorf("appsheet: respuesta no reconocida: %s", recortar(string(datos), 120))
	}
	filas := make([]Fila, 0, len(crudo))
	for _, m := range crudo {
		f := make(Fila, len(m))
		for k, v := range m {
			f[k] = aTexto(v)
		}
		filas = append(filas, f)
	}
	return filas, nil
}

func aTexto(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "TRUE"
		}
		return "FALSE"
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}

func recortar(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// Columnas devuelve los nombres de columna en orden estable (útil en pruebas y logs).
func (f Fila) Columnas() []string {
	cols := make([]string, 0, len(f))
	for k := range f {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	return cols
}
