package imagenes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Encabezado mínimo de PNG para que DetectContentType lo reconozca como imagen.
var png = []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("\x00", 32))

func TestLocal(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "Solicitudes_Images"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Solicitudes_Images", "a.jpg"), png, 0o600); err != nil {
		t.Fatal(err)
	}
	l := Local{Dir: dir}
	if datos, err := l.Obtener(context.Background(), "Solicitudes", "Solicitudes_Images/a.jpg"); err != nil || len(datos) == 0 {
		t.Fatalf("Obtener: %v", err)
	}
	if _, err := l.Obtener(context.Background(), "Solicitudes", "../../etc/passwd"); err == nil {
		t.Fatal("debió rechazar una ruta fuera del directorio")
	}
	if _, err := l.Obtener(context.Background(), "Solicitudes", "Solicitudes_Images/no.jpg"); err != ErrNoEncontrada {
		t.Fatalf("err = %v", err)
	}
}

func TestPorURLSoloGoogle(t *testing.T) {
	if _, err := (PorURL{}).Obtener(context.Background(), "", "https://ejemplo.com/a.jpg"); err == nil {
		t.Fatal("debió rechazar un dominio ajeno")
	}
	if _, err := (PorURL{}).Obtener(context.Background(), "", "http://www.appsheet.com/a.jpg"); err == nil {
		t.Fatal("debió rechazar http sin TLS")
	}
}

func TestRechazaArchivosQueNoSonImagen(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.jpg"), []byte("<html>no</html>"), 0o600)
	if _, err := (Local{Dir: dir}).Obtener(context.Background(), "", "a.jpg"); err == nil {
		t.Fatal("debió rechazar un archivo que no es imagen")
	}
}

func TestDriveSigueLaRuta(t *testing.T) {
	var consultas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/files":
			q := r.URL.Query().Get("q")
			consultas = append(consultas, q)
			switch {
			case strings.Contains(q, "name = 'Solicitudes_Images'") && strings.Contains(q, "'raiz' in parents"):
				_, _ = w.Write([]byte(`{"files":[{"id":"carpeta-imagenes"}]}`))
			case strings.Contains(q, "name = 'a.FotoINE.jpg'") && strings.Contains(q, "'carpeta-imagenes' in parents"):
				_, _ = w.Write([]byte(`{"files":[{"id":"archivo-1"}]}`))
			default:
				_, _ = w.Write([]byte(`{"files":[]}`))
			}
		case r.URL.Path == "/files/archivo-1" && r.URL.Query().Get("alt") == "media":
			_, _ = w.Write(png)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	d := NuevoDriveConCliente("raiz", srv.Client(), srv.URL)
	datos, err := d.Obtener(context.Background(), "Solicitudes", "Solicitudes_Images/a.FotoINE.jpg")
	if err != nil || len(datos) == 0 {
		t.Fatalf("Obtener: %v (consultas: %v)", err, consultas)
	}
	if _, err := d.Obtener(context.Background(), "Solicitudes", "Solicitudes_Images/no.jpg"); err != ErrNoEncontrada {
		t.Fatalf("err = %v", err)
	}
}

func TestDriveEscapaComillas(t *testing.T) {
	var q string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q = r.URL.Query().Get("q")
		_, _ = w.Write([]byte(`{"files":[]}`))
	}))
	defer srv.Close()
	d := NuevoDriveConCliente("raiz", srv.Client(), srv.URL)
	_, _ = d.Obtener(context.Background(), "", "x' or name contains '")
	if !strings.Contains(q, `x\' or name contains \'`) {
		t.Fatalf("consulta sin escapar: %s", q)
	}
}
