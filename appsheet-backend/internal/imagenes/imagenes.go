// Package imagenes descarga las fotos que el alumno adjuntó en AppSheet.
//
// AppSheet guarda en la celda una ruta relativa a la carpeta de la app, por
// ejemplo "Solicitudes_Images/1a2b3c4d.FotoINE.103412.jpg", y el archivo vive
// en Google Drive. Con "Require Image and File URL Signing" activo, las URL
// públicas de AppSheet exigen una firma que solo genera AppSheet, por lo que la
// fuente recomendada es Drive con una cuenta de servicio de solo lectura.
package imagenes

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// MaxBytes limita el tamaño de cada foto.
const MaxBytes = 15 << 20

var ErrNoEncontrada = errors.New("imagen no encontrada")

// Fuente obtiene el contenido de una imagen a partir del valor de la celda.
type Fuente interface {
	Obtener(ctx context.Context, tabla, valor string) ([]byte, error)
}

// Local lee las fotos de un directorio (útil para pruebas y para montar la
// carpeta de Drive sincronizada en el servidor).
type Local struct{ Dir string }

func (l Local) Obtener(_ context.Context, _ string, valor string) ([]byte, error) {
	rel := filepath.FromSlash(strings.TrimPrefix(strings.TrimSpace(valor), "/"))
	if rel == "" || !filepath.IsLocal(rel) {
		return nil, fmt.Errorf("ruta de imagen no permitida")
	}
	f, err := os.Open(filepath.Join(l.Dir, rel))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoEncontrada
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return leerLimitado(f)
}

// URLAppSheet descarga con la URL de archivos de AppSheet. Solo funciona si la
// app tiene desactivado "Require Image and File URL Signing".
type URLAppSheet struct {
	Base    string // https://www.appsheet.com
	AppName string
	HTTP    *http.Client
}

func (u URLAppSheet) Obtener(ctx context.Context, tabla, valor string) ([]byte, error) {
	q := url.Values{"appName": {u.AppName}, "tableName": {tabla}, "fileName": {valor}}
	destino := strings.TrimRight(u.Base, "/") + "/template/gettablefileurl?" + q.Encode()
	return descargar(ctx, clienteODefecto(u.HTTP), destino)
}

// PorURL descarga valores que ya son URL absolutas, solo de dominios de Google
// (evita que una celda manipulada haga que el servidor pida URL arbitrarias).
type PorURL struct{ HTTP *http.Client }

var dominiosPermitidos = []string{"appsheet.com", "googleusercontent.com", "drive.google.com", "googleapis.com"}

func (p PorURL) Obtener(ctx context.Context, _ string, valor string) ([]byte, error) {
	u, err := url.Parse(strings.TrimSpace(valor))
	if err != nil || u.Scheme != "https" {
		return nil, fmt.Errorf("URL de imagen no permitida")
	}
	host := strings.ToLower(u.Hostname())
	permitido := false
	for _, d := range dominiosPermitidos {
		if host == d || strings.HasSuffix(host, "."+d) {
			permitido = true
			break
		}
	}
	if !permitido {
		return nil, fmt.Errorf("dominio de imagen no permitido: %s", host)
	}
	return descargar(ctx, clienteODefecto(p.HTTP), u.String())
}

// Enrutador usa URLs para valores http(s) y Archivos para rutas relativas.
type Enrutador struct {
	Archivos Fuente
	URLs     Fuente
}

func (e Enrutador) Obtener(ctx context.Context, tabla, valor string) ([]byte, error) {
	valor = strings.TrimSpace(valor)
	if valor == "" {
		return nil, ErrNoEncontrada
	}
	if strings.HasPrefix(valor, "http://") || strings.HasPrefix(valor, "https://") {
		if e.URLs == nil {
			return nil, fmt.Errorf("las URL de imagen no están habilitadas")
		}
		return e.URLs.Obtener(ctx, tabla, valor)
	}
	return e.Archivos.Obtener(ctx, tabla, valor)
}

// NombreArchivo devuelve el último segmento de la ruta de AppSheet.
func NombreArchivo(valor string) string {
	return path.Base(strings.ReplaceAll(strings.TrimSpace(valor), "\\", "/"))
}

func clienteODefecto(c *http.Client) *http.Client {
	if c != nil {
		return c
	}
	return &http.Client{Timeout: 60 * time.Second}
}

func descargar(ctx context.Context, c *http.Client, destino string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, destino, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNoEncontrada
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("descarga de imagen: HTTP %d", resp.StatusCode)
	}
	return leerLimitado(resp.Body)
}

func leerLimitado(r io.Reader) ([]byte, error) {
	datos, err := io.ReadAll(io.LimitReader(r, MaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(datos) > MaxBytes {
		return nil, fmt.Errorf("la imagen supera %d MB", MaxBytes>>20)
	}
	if len(datos) == 0 {
		return nil, ErrNoEncontrada
	}
	tipo := http.DetectContentType(datos)
	if !strings.HasPrefix(tipo, "image/") {
		return nil, fmt.Errorf("el archivo no es una imagen (%s)", tipo)
	}
	return datos, nil
}
