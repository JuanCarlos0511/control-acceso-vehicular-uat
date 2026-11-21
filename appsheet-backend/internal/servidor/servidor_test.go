package servidor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"registrovehicular/internal/repuve"
	"registrovehicular/internal/servicio"
	"registrovehicular/internal/validacion"
)

const (
	secreto = "secreto-del-webhook-de-prueba-123"
	token   = "token-de-la-api-de-prueba-123456"
	folio   = "6f1c2d3e-4a5b-4c6d-8e7f-001122334455"

	codigoValido = "RV1.6f1c2d3e-4a5b-4c6d-8e7f-001122334455.XYZ123A.20270731.firma"
)

type validadorFalso struct{ prueba servicio.EntradaPrueba }

func (v *validadorFalso) Validar(_ context.Context, id, origen string) (*servicio.Informe, error) {
	return &servicio.Informe{SolicitudID: id, Origen: origen, Guardado: true}, nil
}
func (v *validadorFalso) Ultimo(id string) (servicio.Informe, bool) {
	return servicio.Informe{SolicitudID: id}, id == folio
}
func (v *validadorFalso) Probar(_ context.Context, p servicio.EntradaPrueba) (*servicio.InformePrueba, error) {
	v.prueba = p
	if len(p.INEFrente) == 0 {
		return nil, servicio.ErrFaltanFotos
	}
	return &servicio.InformePrueba{Dictamen: validacion.Dictamen{Decision: "EN_REVISION"}}, nil
}
func (v *validadorFalso) EmitirPase(_ context.Context, id string, _ bool) (*servicio.InfoPase, error) {
	if id != folio {
		return nil, servicio.ErrNoAprobada
	}
	return &servicio.InfoPase{PaseID: "p", Codigo: codigoValido, Nuevo: true}, nil
}
func (v *validadorFalso) VerificarPase(_ context.Context, codigo string) (*servicio.Verificacion, error) {
	return &servicio.Verificacion{Valido: codigo == codigoValido, Placa: "XYZ123A"}, nil
}
func (v *validadorFalso) ImagenPase(codigo string) ([]byte, error) {
	if codigo != codigoValido {
		return nil, errors.New("firma inválida")
	}
	return []byte("\x89PNG"), nil
}
func (v *validadorFalso) ConsultarREPUVE(_ context.Context, q repuve.Consulta) (*repuve.Resultado, error) {
	return &repuve.Resultado{Estatus: "SIN_REPORTE", Criterio: q.Criterio, Valor: q.Valor}, nil
}

type colaFalsa struct{ ids []string }

func (c *colaFalsa) Encolar(id, _ string) bool { c.ids = append(c.ids, id); return true }

func preparar() (http.Handler, *colaFalsa, *validadorFalso) {
	c := &colaFalsa{}
	v := &validadorFalso{}
	return Nuevo(v, c, Opciones{WebhookSecreto: secreto, APIToken: token}).Rutas(), c, v
}

type sobre struct {
	Ok    bool            `json:"ok"`
	Datos json.RawMessage `json:"datos"`
	Error *errorAPI       `json:"error"`
}

func llamar(t *testing.T, h http.Handler, req *http.Request) (int, sobre) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var s sobre
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
		t.Fatalf("respuesta no es JSON: %s", rec.Body.String())
	}
	return rec.Code, s
}

func TestWebhook(t *testing.T) {
	h, cola, _ := preparar()
	req := httptest.NewRequest("POST", "/webhooks/appsheet/solicitudes", bytes.NewBufferString(`{"solicitudId":"`+folio+`"}`))
	req.Header.Set("X-Webhook-Secreto", secreto)
	codigo, s := llamar(t, h, req)
	if codigo != http.StatusAccepted || !s.Ok || len(cola.ids) != 1 || cola.ids[0] != folio {
		t.Fatalf("codigo=%d sobre=%+v cola=%v", codigo, s, cola.ids)
	}
}

func TestWebhookSinSecreto(t *testing.T) {
	h, cola, _ := preparar()
	req := httptest.NewRequest("POST", "/webhooks/appsheet/solicitudes", bytes.NewBufferString(`{"solicitudId":"`+folio+`"}`))
	req.Header.Set("X-Webhook-Secreto", "otro")
	codigo, s := llamar(t, h, req)
	if codigo != http.StatusUnauthorized || s.Ok || s.Error.Codigo != "PERMISO_DENEGADO" || len(cola.ids) != 0 {
		t.Fatalf("codigo=%d sobre=%+v", codigo, s)
	}
}

func TestWebhookFolioInvalido(t *testing.T) {
	h, _, _ := preparar()
	req := httptest.NewRequest("POST", "/webhooks/appsheet/solicitudes", bytes.NewBufferString(`{"solicitudId":"<<[SolicitudId]>>"}`))
	req.Header.Set("X-Webhook-Secreto", secreto)
	if codigo, s := llamar(t, h, req); codigo != http.StatusBadRequest || s.Error.Codigo != "VALIDACION" {
		t.Fatalf("codigo=%d sobre=%+v", codigo, s)
	}
}

func TestAPIRequiereToken(t *testing.T) {
	h, _, _ := preparar()
	req := httptest.NewRequest("POST", "/api/v1/solicitudes/"+folio+"/validar?esperar=1", nil)
	if codigo, _ := llamar(t, h, req); codigo != http.StatusUnauthorized {
		t.Fatalf("codigo=%d", codigo)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	codigo, s := llamar(t, h, req)
	if codigo != http.StatusOK || !s.Ok {
		t.Fatalf("codigo=%d sobre=%+v", codigo, s)
	}
}

func TestEstado(t *testing.T) {
	h, _, _ := preparar()
	req := httptest.NewRequest("GET", "/api/v1/solicitudes/"+folio, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	if codigo, _ := llamar(t, h, req); codigo != http.StatusOK {
		t.Fatalf("codigo=%d", codigo)
	}
}

func TestPruebaMultipart(t *testing.T) {
	h, _, v := preparar()
	var cuerpo bytes.Buffer
	w := multipart.NewWriter(&cuerpo)
	for campo, valor := range map[string]string{"placa": "XYZ123A", "niv": "1HGCM82633A004352", "nombre": "Ana"} {
		_ = w.WriteField(campo, valor)
	}
	for _, campo := range []string{"ineFrente", "tarjeta"} {
		f, _ := w.CreateFormFile(campo, campo+".jpg")
		_, _ = f.Write([]byte("datos"))
	}
	_ = w.Close()
	req := httptest.NewRequest("POST", "/api/v1/pruebas/validar", &cuerpo)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	codigo, s := llamar(t, h, req)
	if codigo != http.StatusOK || !s.Ok || v.prueba.Placa != "XYZ123A" || string(v.prueba.Tarjeta) != "datos" {
		t.Fatalf("codigo=%d sobre=%+v prueba=%+v", codigo, s, v.prueba)
	}
}

func TestRutaDesconocida(t *testing.T) {
	h, _, _ := preparar()
	if codigo, s := llamar(t, h, httptest.NewRequest("GET", "/nada", nil)); codigo != http.StatusNotFound || s.Ok {
		t.Fatalf("codigo=%d", codigo)
	}
}

func TestImagenPasePublica(t *testing.T) {
	h, _, _ := preparar()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/pases/qr/"+codigoValido+".png", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("codigo=%d tipo=%s", rec.Code, rec.Header().Get("Content-Type"))
	}
	for _, ruta := range []string{"/pases/qr/" + codigoValido, "/pases/qr/cualquier-texto.png"} {
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", ruta, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: codigo=%d", ruta, rec.Code)
		}
	}
}

func TestVerificarPase(t *testing.T) {
	h, _, _ := preparar()
	req := httptest.NewRequest("POST", "/api/v1/pases/verificar", bytes.NewBufferString(`{"codigo":"`+codigoValido+`"}`))
	if codigo, _ := llamar(t, h, req); codigo != http.StatusUnauthorized {
		t.Fatalf("sin token: %d", codigo)
	}
	req = httptest.NewRequest("POST", "/api/v1/pases/verificar", bytes.NewBufferString(`{"codigo":"`+codigoValido+`"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	codigo, s := llamar(t, h, req)
	var v servicio.Verificacion
	_ = json.Unmarshal(s.Datos, &v)
	if codigo != http.StatusOK || !v.Valido || v.Placa != "XYZ123A" {
		t.Fatalf("codigo=%d verificacion=%+v", codigo, v)
	}
}

func TestEmitirPaseNoAprobada(t *testing.T) {
	h, _, _ := preparar()
	req := httptest.NewRequest("POST", "/api/v1/solicitudes/11111111-2222-4333-8444-555555555555/pase", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	if codigo, s := llamar(t, h, req); codigo != http.StatusConflict || s.Error.Codigo != "CONFLICTO" {
		t.Fatalf("codigo=%d sobre=%+v", codigo, s)
	}
}
