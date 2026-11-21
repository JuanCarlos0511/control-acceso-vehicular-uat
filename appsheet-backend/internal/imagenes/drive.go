package imagenes

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"golang.org/x/oauth2/google"
)

const alcanceDrive = "https://www.googleapis.com/auth/drive.readonly"

// Drive busca las fotos en la carpeta de la app en Google Drive.
//
// Credenciales: Application Default Credentials (variable
// GOOGLE_APPLICATION_CREDENTIALS con la llave de una cuenta de servicio, o la
// cuenta del entorno en Cloud Run). La carpeta debe estar compartida con esa
// cuenta como lector.
type Drive struct {
	CarpetaID string
	http      *http.Client
	api       string

	mu       sync.Mutex
	carpetas map[string]string // "padre/nombre" → id
}

func NuevoDrive(ctx context.Context, carpetaID string) (*Drive, error) {
	cliente, err := google.DefaultClient(ctx, alcanceDrive)
	if err != nil {
		return nil, fmt.Errorf("credenciales de Google para Drive: %w", err)
	}
	return &Drive{CarpetaID: carpetaID, http: cliente, api: "https://www.googleapis.com/drive/v3", carpetas: map[string]string{}}, nil
}

// NuevoDriveConCliente permite inyectar un cliente HTTP y la URL de la API (pruebas).
func NuevoDriveConCliente(carpetaID string, c *http.Client, api string) *Drive {
	return &Drive{CarpetaID: carpetaID, http: c, api: api, carpetas: map[string]string{}}
}

func (d *Drive) Obtener(ctx context.Context, _ string, valor string) ([]byte, error) {
	segmentos := strings.Split(strings.Trim(strings.ReplaceAll(valor, "\\", "/"), "/"), "/")
	nombre := segmentos[len(segmentos)-1]
	if nombre == "" || nombre == "." || nombre == ".." {
		return nil, ErrNoEncontrada
	}

	// Primero se sigue la ruta (Solicitudes_Images/...) desde la carpeta de la
	// app; si DRIVE_CARPETA_ID ya apunta a la subcarpeta, se busca ahí directo.
	padre := d.CarpetaID
	for _, s := range segmentos[:len(segmentos)-1] {
		id, err := d.subcarpeta(ctx, padre, s)
		if err != nil {
			return nil, err
		}
		if id == "" {
			padre = d.CarpetaID
			break
		}
		padre = id
	}
	id, err := d.buscar(ctx, padre, nombre, false)
	if err != nil {
		return nil, err
	}
	if id == "" && padre != d.CarpetaID {
		if id, err = d.buscar(ctx, d.CarpetaID, nombre, false); err != nil {
			return nil, err
		}
	}
	if id == "" {
		return nil, ErrNoEncontrada
	}
	return descargar(ctx, d.http, fmt.Sprintf("%s/files/%s?alt=media&supportsAllDrives=true", d.api, url.PathEscape(id)))
}

func (d *Drive) subcarpeta(ctx context.Context, padre, nombre string) (string, error) {
	clave := padre + "/" + nombre
	d.mu.Lock()
	id, ok := d.carpetas[clave]
	d.mu.Unlock()
	if ok {
		return id, nil
	}
	id, err := d.buscar(ctx, padre, nombre, true)
	if err != nil {
		return "", err
	}
	if id != "" {
		d.mu.Lock()
		d.carpetas[clave] = id
		d.mu.Unlock()
	}
	return id, nil
}

func (d *Drive) buscar(ctx context.Context, padre, nombre string, carpeta bool) (string, error) {
	escapar := strings.NewReplacer(`\`, `\\`, `'`, `\'`)
	q := fmt.Sprintf("name = '%s' and '%s' in parents and trashed = false", escapar.Replace(nombre), escapar.Replace(padre))
	if carpeta {
		q += " and mimeType = 'application/vnd.google-apps.folder'"
	}
	params := url.Values{
		"q":                         {q},
		"fields":                    {"files(id,name,size)"},
		"pageSize":                  {"2"},
		"supportsAllDrives":         {"true"},
		"includeItemsFromAllDrives": {"true"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.api+"/files?"+params.Encode(), nil)
	if err != nil {
		return "", err
	}
	resp, err := d.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("drive: HTTP %d al buscar archivo", resp.StatusCode)
	}
	var lista struct {
		Files []struct {
			ID string `json:"id"`
		} `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&lista); err != nil {
		return "", fmt.Errorf("drive: respuesta no reconocida: %w", err)
	}
	if len(lista.Files) == 0 {
		return "", nil
	}
	return lista.Files[0].ID, nil
}
