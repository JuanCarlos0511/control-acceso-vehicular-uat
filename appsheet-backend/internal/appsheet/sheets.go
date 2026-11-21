package appsheet

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2/google"
	"registrovehicular/internal/config"
)

const alcanceSheets = "https://www.googleapis.com/auth/spreadsheets"

// ClienteSheets trabaja sobre la hoja privada que sirve de origen a la app.
// La cuenta de servicio necesita acceso de edición solo a ese archivo.
type ClienteSheets struct {
	base   string
	http   *http.Client
	zona   *time.Location
	claves map[string]string

	mu    sync.Mutex // serializa las escrituras de la instancia del backend
	hojas map[string]int
}

func NuevoClienteSheets(ctx context.Context, id string, t config.Tablas, zona *time.Location) (*ClienteSheets, error) {
	http, err := google.DefaultClient(ctx, alcanceSheets)
	if err != nil {
		return nil, fmt.Errorf("credenciales para Google Sheets: %w", err)
	}
	http.Timeout = 30 * time.Second
	return nuevoClienteSheets("https://sheets.googleapis.com/v4/spreadsheets/"+url.PathEscape(id), http, t, zona), nil
}

func nuevoClienteSheets(base string, http *http.Client, t config.Tablas, zona *time.Location) *ClienteSheets {
	claves := map[string]string{
		t.Solicitudes: "SolicitudId", t.Alumnos: "Correo", t.Revisiones: "SolicitudId",
		t.Resultados: "SolicitudId", t.Pases: "PaseId",
	}
	if t.Validaciones != "" {
		claves[t.Validaciones] = "ValidacionId"
	}
	return &ClienteSheets{base: base, http: http, zona: zona, claves: claves}
}

type filaSheets struct {
	indice int // índice de fila de la API de Sheets, desde cero
	datos  Fila
}

func (c *ClienteSheets) Salud(ctx context.Context, tabla string) error {
	cabeceras, err := c.leerValores(ctx, tabla, "!1:1")
	if err != nil {
		return err
	}
	_, err = columnasSheets(cabeceras, c.claves[tabla], tabla)
	return err
}

func (c *ClienteSheets) Buscar(ctx context.Context, tabla, selector string) ([]Fila, error) {
	if selector != "" {
		return nil, fmt.Errorf("sheets: la API de Sheets no interpreta selectores de AppSheet")
	}
	_, filas, err := c.leerTabla(ctx, tabla)
	if err != nil {
		return nil, err
	}
	salida := make([]Fila, 0, len(filas))
	for _, fila := range filas {
		salida = append(salida, fila.datos)
	}
	return salida, nil
}

func (c *ClienteSheets) Agregar(ctx context.Context, tabla string, filas ...Fila) ([]Fila, error) {
	return c.escribir(ctx, tabla, true, filas)
}

func (c *ClienteSheets) Editar(ctx context.Context, tabla string, filas ...Fila) ([]Fila, error) {
	return c.escribir(ctx, tabla, false, filas)
}

func (c *ClienteSheets) leerValores(ctx context.Context, tabla, rango string) ([][]any, error) {
	if c.claves[tabla] == "" {
		return nil, fmt.Errorf("sheets: pestaña no configurada")
	}
	a1 := "'" + strings.ReplaceAll(tabla, "'", "''") + "'" + rango
	var respuesta struct{ Values [][]any }
	err := c.llamar(ctx, http.MethodGet, "/values/"+url.PathEscape(a1)+"?valueRenderOption=UNFORMATTED_VALUE", nil, &respuesta)
	return respuesta.Values, err
}

func columnasSheets(valores [][]any, clave, tabla string) (map[string]int, error) {
	if len(valores) == 0 {
		return nil, fmt.Errorf("sheets: faltan los encabezados de %s", tabla)
	}
	columnas := make(map[string]int, len(valores[0]))
	for i, valor := range valores[0] {
		campo := strings.TrimSpace(textoSheets(valor))
		if campo == "" {
			continue
		}
		if _, repetido := columnas[campo]; repetido {
			return nil, fmt.Errorf("sheets: encabezado duplicado en %s", tabla)
		}
		columnas[campo] = i
	}
	if _, ok := columnas[clave]; !ok {
		return nil, fmt.Errorf("sheets: falta la columna %s en %s", clave, tabla)
	}
	return columnas, nil
}

func (c *ClienteSheets) leerTabla(ctx context.Context, tabla string) (map[string]int, []filaSheets, error) {
	valores, err := c.leerValores(ctx, tabla, "")
	if err != nil {
		return nil, nil, err
	}
	clave := c.claves[tabla]
	columnas, err := columnasSheets(valores, clave, tabla)
	if err != nil {
		return nil, nil, err
	}
	var filas []filaSheets
	for i, celdas := range valores[1:] {
		datos := Fila{}
		for campo, j := range columnas {
			if j >= len(celdas) {
				continue
			}
			if numero, ok := celdas[j].(json.Number); ok && fechaSheets(campo) {
				dias, err := numero.Float64()
				if err != nil || dias < 0 || dias > 150000 {
					return nil, nil, fmt.Errorf("sheets: fecha inválida en %s", tabla)
				}
				// El serial representa la hora de pared de la hoja, no UTC.
				fecha := inicioSheets.AddDate(0, 0, int(dias))
				t := fecha.Add(time.Duration(math.Round((dias-math.Floor(dias))*86400)) * time.Second)
				datos[campo] = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, c.zona).Format(time.RFC3339)
			} else {
				datos[campo] = textoSheets(celdas[j])
			}
		}
		if strings.TrimSpace(datos[clave]) != "" {
			filas = append(filas, filaSheets{indice: i + 1, datos: datos})
		}
	}
	return columnas, filas, nil
}

func (c *ClienteSheets) escribir(ctx context.Context, tabla string, agregar bool, nuevas []Fila) ([]Fila, error) {
	if len(nuevas) == 0 {
		return nil, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	columnas, existentes, err := c.leerTabla(ctx, tabla)
	if err != nil {
		return nil, err
	}
	id, err := c.idPestana(ctx, tabla)
	if err != nil {
		return nil, err
	}
	clave := c.claves[tabla]
	indices := map[string]int{}
	for _, fila := range existentes {
		valor := normalizarClaveSheets(fila.datos[clave])
		if _, duplicada := indices[valor]; duplicada {
			return nil, fmt.Errorf("sheets: claves duplicadas en %s", tabla)
		}
		indices[valor] = fila.indice
	}

	var peticiones []map[string]any
	for _, fila := range nuevas {
		valor := normalizarClaveSheets(fila[clave])
		indice, existe := indices[valor]
		if valor == "" || agregar == existe {
			return nil, fmt.Errorf("sheets: la clave falta o no admite esa operación en %s", tabla)
		}
		celdas := make([]map[string]any, 0)
		for campo, valor := range fila {
			posicion, ok := columnas[campo]
			if !ok {
				return nil, fmt.Errorf("sheets: falta la columna %s en %s", campo, tabla)
			}
			celda, campos, err := c.celda(campo, valor)
			if err != nil {
				return nil, err
			}
			if agregar {
				for len(celdas) <= posicion {
					celdas = append(celdas, map[string]any{})
				}
				celdas[posicion] = celda
			} else {
				peticiones = append(peticiones, map[string]any{"updateCells": map[string]any{
					"start":  map[string]int{"sheetId": id, "rowIndex": indice, "columnIndex": posicion},
					"rows":   []map[string]any{{"values": []map[string]any{celda}}},
					"fields": campos,
				}})
			}
		}
		if agregar {
			peticiones = append(peticiones, map[string]any{"appendCells": map[string]any{
				"sheetId": id, "rows": []map[string]any{{"values": celdas}},
				"fields": "userEnteredValue,userEnteredFormat.numberFormat",
			}})
			indices[valor] = -1
		}
	}
	if err := c.llamar(ctx, http.MethodPost, ":batchUpdate", map[string]any{"requests": peticiones}, nil); err != nil {
		return nil, err
	}
	return nuevas, nil
}

func (c *ClienteSheets) idPestana(ctx context.Context, tabla string) (int, error) {
	if id, ok := c.hojas[tabla]; ok {
		return id, nil
	}
	var archivo struct {
		Sheets []struct {
			Properties struct {
				SheetID int
				Title   string
			}
		}
	}
	if err := c.llamar(ctx, http.MethodGet, "?fields=sheets(properties(sheetId,title))", nil, &archivo); err != nil {
		return 0, err
	}
	c.hojas = make(map[string]int, len(archivo.Sheets))
	for _, hoja := range archivo.Sheets {
		c.hojas[hoja.Properties.Title] = hoja.Properties.SheetID
	}
	if id, ok := c.hojas[tabla]; ok {
		return id, nil
	}
	return 0, fmt.Errorf("sheets: no se encontró la pestaña %s", tabla)
}

func (c *ClienteSheets) llamar(ctx context.Context, metodo, ruta string, carga, salida any) error {
	var cuerpo io.Reader
	if carga != nil {
		b, err := json.Marshal(carga)
		if err != nil {
			return err
		}
		cuerpo = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, metodo, c.base+ruta, cuerpo)
	if err != nil {
		return err
	}
	if carga != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sheets: HTTP %d", resp.StatusCode)
	}
	if salida == nil {
		_, err = io.Copy(io.Discard, resp.Body)
		return err
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 32<<20))
	decoder.UseNumber()
	return decoder.Decode(salida)
}

var inicioSheets = time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)

func fechaSheets(campo string) bool {
	switch campo {
	case "FechaEnvio", "VerificadoUAT", "FechaREPUVE", "FechaPublicacion", "ActualizadoEn", "FechaHora", "EmitidoEn", "VenceEn":
		return true
	}
	return false
}

func (c *ClienteSheets) celda(campo, valor string) (map[string]any, string, error) {
	celda := map[string]any{}
	campos := "userEnteredValue"
	if valor == "" {
		return celda, campos, nil
	}
	switch {
	case fechaSheets(campo):
		fecha, err := time.Parse(time.RFC3339, valor)
		if err != nil {
			return nil, "", fmt.Errorf("sheets: fecha inválida para %s", campo)
		}
		fecha = fecha.In(c.zona)
		pared := time.Date(fecha.Year(), fecha.Month(), fecha.Day(), fecha.Hour(), fecha.Minute(), fecha.Second(), 0, time.UTC)
		celda["userEnteredValue"] = map[string]float64{"numberValue": float64(pared.Unix()-inicioSheets.Unix()) / 86400}
		celda["userEnteredFormat"] = map[string]any{"numberFormat": map[string]string{"type": "DATE_TIME", "pattern": "yyyy-mm-dd hh:mm:ss"}}
		campos += ",userEnteredFormat.numberFormat"
	case campo == "Activo" || campo == "AvisoLeido":
		celda["userEnteredValue"] = map[string]bool{"boolValue": booleano(valor)}
	case campo == "PuntajeNombre" || campo == "DuracionSeg":
		numero, err := strconv.ParseFloat(valor, 64)
		if err != nil {
			return nil, "", fmt.Errorf("sheets: número inválido para %s", campo)
		}
		celda["userEnteredValue"] = map[string]float64{"numberValue": numero}
	default:
		// stringValue evita que texto de un expediente se interprete como fórmula.
		celda["userEnteredValue"] = map[string]string{"stringValue": valor}
	}
	return celda, campos, nil
}

func textoSheets(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case json.Number:
		return x.String()
	}
	return ""
}

func normalizarClaveSheets(v string) string { return strings.ToLower(strings.TrimSpace(v)) }
