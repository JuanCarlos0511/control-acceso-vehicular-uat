// Package servidor expone la API HTTP del backend.
//
// Todas las respuestas usan el sobre del contrato del sistema
// (docs/contrato-api.md §2): {ok: true, datos} o {ok: false, error: {codigo, mensaje}}.
package servidor

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"registrovehicular/internal/modelo"
	"registrovehicular/internal/registro"
	"registrovehicular/internal/repuve"
	"registrovehicular/internal/servicio"
	"registrovehicular/internal/uat"
)

const (
	maxWebhook = 64 << 10
	maxPrueba  = 48 << 20
)

type Validador interface {
	Validar(ctx context.Context, id, origen string) (*servicio.Informe, error)
	Ultimo(id string) (servicio.Informe, bool)
	Probar(ctx context.Context, p servicio.EntradaPrueba) (*servicio.InformePrueba, error)
	ConsultarREPUVE(ctx context.Context, q repuve.Consulta) (*repuve.Resultado, error)
	EmitirPase(ctx context.Context, solicitudID string, forzar bool) (*servicio.InfoPase, error)
	VerificarPase(ctx context.Context, codigo string) (*servicio.Verificacion, error)
	ImagenPase(codigo string) ([]byte, error)
}

type Encolador interface {
	Encolar(id, origen string) bool
}

// Registrador da de alta al alumno con su cuenta institucional.
type Registrador interface {
	Registrar(ctx context.Context, usuario, contrasena, ipCliente string) (*registro.Resultado, error)
}

type Salud func(ctx context.Context) map[string]string

type Opciones struct {
	WebhookSecreto string
	APIToken       string
	Log            *slog.Logger
	Salud          Salud
	Registro       Registrador // nil = registro con cuenta institucional deshabilitado
	CORSOrigenes   []string    // orígenes del frontend que pueden llamar a las rutas públicas
}

type Servidor struct {
	v   Validador
	c   Encolador
	o   Opciones
	log *slog.Logger
}

func Nuevo(v Validador, c Encolador, o Opciones) *Servidor {
	if o.Log == nil {
		o.Log = slog.Default()
	}
	return &Servidor{v: v, c: c, o: o, log: o.Log}
}

// Rutas registra los endpoints.
func (s *Servidor) Rutas() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /salud", s.salud)
	m.HandleFunc("POST /webhooks/appsheet/solicitudes", s.conWebhook(s.webhookSolicitud))
	m.HandleFunc("POST /api/v1/solicitudes/{id}/validar", s.conToken(s.validar))
	m.HandleFunc("GET /api/v1/solicitudes/{id}", s.conToken(s.estado))
	m.HandleFunc("POST /api/v1/repuve", s.conToken(s.consultarREPUVE))
	m.HandleFunc("POST /api/v1/pruebas/validar", s.conToken(s.probar))
	m.HandleFunc("POST /api/v1/solicitudes/{id}/pase", s.conToken(s.emitirPase))
	m.HandleFunc("POST /api/v1/pases/verificar", s.conToken(s.verificarPase))
	// Pública: AppSheet carga la imagen desde el navegador del alumno, sin encabezados.
	m.HandleFunc("GET /pases/qr/{archivo}", s.imagenPase)
	// Pública: el alumno inicia sesión con su cuenta institucional desde el frontend.
	m.HandleFunc("POST /api/v1/alumnos/verificar", s.conCORS(s.verificarAlumno))
	m.HandleFunc("OPTIONS /api/v1/alumnos/verificar", s.conCORS(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fallo(w, http.StatusNotFound, "NO_ENCONTRADO", "Ruta no encontrada.")
	})
	return s.registrar(s.recuperar(m))
}

// ---- Sobre de respuesta ----

type errorAPI struct {
	Codigo  string `json:"codigo"`
	Mensaje string `json:"mensaje"`
}

func exito(w http.ResponseWriter, estado int, datos any) {
	escribirJSON(w, estado, struct {
		Ok    bool `json:"ok"`
		Datos any  `json:"datos"`
	}{true, datos})
}

func fallo(w http.ResponseWriter, estado int, codigo, mensaje string) {
	escribirJSON(w, estado, struct {
		Ok    bool     `json:"ok"`
		Error errorAPI `json:"error"`
	}{false, errorAPI{codigo, mensaje}})
}

func escribirJSON(w http.ResponseWriter, estado int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(estado)
	_ = json.NewEncoder(w).Encode(v)
}

// ---- Middleware ----

func iguales(a, b string) bool {
	return a != "" && b != "" && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func (s *Servidor) conWebhook(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !iguales(r.Header.Get("X-Webhook-Secreto"), s.o.WebhookSecreto) {
			fallo(w, http.StatusUnauthorized, "PERMISO_DENEGADO", "Secreto del webhook inválido.")
			return
		}
		h(w, r)
	}
}

func (s *Servidor) conToken(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !iguales(strings.TrimSpace(token), s.o.APIToken) {
			fallo(w, http.StatusUnauthorized, "PERMISO_DENEGADO", "Token de la API inválido.")
			return
		}
		h(w, r)
	}
}

// conCORS permite que el frontend (otro dominio) llame a la ruta. Solo se
// responde con el origen exacto si está en la lista.
func (s *Servidor) conCORS(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Origin")
		origen := r.Header.Get("Origin")
		if origen != "" && slices.Contains(s.o.CORSOrigenes, origen) {
			w.Header().Set("Access-Control-Allow-Origin", origen)
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Max-Age", "600")
		} else if origen != "" && r.Method == http.MethodOptions {
			fallo(w, http.StatusForbidden, "PERMISO_DENEGADO", "Origen no permitido.")
			return
		}
		h(w, r)
	}
}

// ipCliente: Cloudflare manda la IP real en CF-Connecting-IP; detrás de
// Traefik sin Cloudflare queda X-Forwarded-For. Solo se usa para limitar intentos.
func ipCliente(r *http.Request) string {
	for _, v := range []string{r.Header.Get("CF-Connecting-IP"), strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]} {
		if ip := net.ParseIP(strings.TrimSpace(v)); ip != nil {
			return ip.String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Servidor) recuperar(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				s.log.Error("pánico en la API", "ruta", r.URL.Path, "panico", p)
				fallo(w, http.StatusInternalServerError, "ERROR_INTERNO", "Error interno del servidor.")
			}
		}()
		h.ServeHTTP(w, r)
	})
}

type grabadora struct {
	http.ResponseWriter
	estado int
}

func (g *grabadora) WriteHeader(c int) { g.estado = c; g.ResponseWriter.WriteHeader(c) }

func (s *Servidor) registrar(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inicio := time.Now()
		g := &grabadora{ResponseWriter: w, estado: http.StatusOK}
		h.ServeHTTP(g, r)
		// Solo método, ruta y estado: las URL y cuerpos no se registran porque pueden traer datos personales.
		s.log.Info("http", "metodo", r.Method, "ruta", r.Pattern, "estado", g.estado,
			"duracion", time.Since(inicio).Round(time.Millisecond).String())
	})
}

// ---- Handlers ----

func (s *Servidor) salud(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	componentes := map[string]string{}
	if s.o.Salud != nil {
		componentes = s.o.Salud(ctx)
	}
	estado := http.StatusOK
	for _, v := range componentes {
		if v != "ok" && v != "deshabilitado" {
			estado = http.StatusServiceUnavailable
		}
	}
	exito(w, estado, map[string]any{"version": servicio.Version, "componentes": componentes})
}

// webhookSolicitud lo llama el bot de AppSheet al agregarse una solicitud.
// Cuerpo: {"solicitudId": "<<[SolicitudId]>>"}. Responde 202 de inmediato.
func (s *Servidor) webhookSolicitud(w http.ResponseWriter, r *http.Request) {
	var cuerpo struct {
		SolicitudID string `json:"solicitudId"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxWebhook)).Decode(&cuerpo); err != nil {
		fallo(w, http.StatusBadRequest, "VALIDACION", "El cuerpo debe ser JSON con solicitudId.")
		return
	}
	id := strings.TrimSpace(cuerpo.SolicitudID)
	if !modelo.FolioValido(id) {
		fallo(w, http.StatusBadRequest, "VALIDACION", "solicitudId no tiene formato UUID.")
		return
	}
	encolada := s.c.Encolar(id, "webhook")
	exito(w, http.StatusAccepted, map[string]any{"solicitudId": id, "encolada": encolada})
}

// validar revalida una solicitud. Con ?esperar=1 responde con el informe.
func (s *Servidor) validar(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !modelo.FolioValido(id) {
		fallo(w, http.StatusBadRequest, "VALIDACION", "El folio no tiene formato UUID.")
		return
	}
	if r.URL.Query().Get("esperar") != "1" {
		exito(w, http.StatusAccepted, map[string]any{"solicitudId": id, "encolada": s.c.Encolar(id, "api")})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	inf, err := s.v.Validar(ctx, id, "api")
	switch {
	case errors.Is(err, servicio.ErrNoEncontrada):
		fallo(w, http.StatusNotFound, "NO_ENCONTRADO", err.Error())
	case errors.Is(err, servicio.ErrEnCurso):
		fallo(w, http.StatusConflict, "OCUPADO", err.Error())
	case err != nil:
		s.log.Error("validación desde la API fallida", "solicitud", id, "error", err)
		fallo(w, http.StatusBadGateway, "ERROR_INTERNO", "No se pudo completar la validación. Revisa los logs del servidor.")
	default:
		exito(w, http.StatusOK, inf)
	}
}

func (s *Servidor) estado(w http.ResponseWriter, r *http.Request) {
	inf, ok := s.v.Ultimo(r.PathValue("id"))
	if !ok {
		fallo(w, http.StatusNotFound, "NO_ENCONTRADO", "No hay validaciones recientes de esa solicitud en este servidor.")
		return
	}
	exito(w, http.StatusOK, inf)
}

// consultarREPUVE: {"niv": "..."} o {"placa": "..."}.
func (s *Servidor) consultarREPUVE(w http.ResponseWriter, r *http.Request) {
	var cuerpo struct {
		NIV   string `json:"niv"`
		Placa string `json:"placa"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxWebhook)).Decode(&cuerpo); err != nil {
		fallo(w, http.StatusBadRequest, "VALIDACION", "El cuerpo debe ser JSON con niv o placa.")
		return
	}
	q := repuve.Consulta{Criterio: repuve.CriterioNIV, Valor: cuerpo.NIV}
	if strings.TrimSpace(cuerpo.NIV) == "" {
		q = repuve.Consulta{Criterio: repuve.CriterioPlaca, Valor: cuerpo.Placa}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()
	res, err := s.v.ConsultarREPUVE(ctx, q)
	var invalida *repuve.ErrConsulta
	switch {
	case errors.As(err, &invalida):
		fallo(w, http.StatusBadRequest, "VALIDACION", invalida.Mensaje)
	case errors.Is(err, servicio.ErrREPUVEDeshabilitado):
		fallo(w, http.StatusConflict, "CONFLICTO", err.Error())
	case err != nil:
		s.log.Error("consulta REPUVE fallida", "error", err)
		fallo(w, http.StatusBadGateway, "ERROR_INTERNO", "El analizador no respondió la consulta a REPUVE.")
	default:
		exito(w, http.StatusOK, res)
	}
}

// probar recibe fotos por multipart (ineFrente, ineReverso, tarjeta) y los
// campos placa, niv, nombre y repuve=1. No lee ni escribe AppSheet.
func (s *Servidor) probar(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxPrueba)
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		fallo(w, http.StatusBadRequest, "VALIDACION", "Envía multipart/form-data con ineFrente, tarjeta, placa, niv y nombre.")
		return
	}
	defer r.MultipartForm.RemoveAll()
	leer := func(campo string) ([]byte, error) {
		f, _, err := r.FormFile(campo)
		if errors.Is(err, http.ErrMissingFile) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return io.ReadAll(io.LimitReader(f, 16<<20))
	}
	var p servicio.EntradaPrueba
	var err error
	if p.INEFrente, err = leer("ineFrente"); err == nil {
		if p.INEReverso, err = leer("ineReverso"); err == nil {
			p.Tarjeta, err = leer("tarjeta")
		}
	}
	if err != nil {
		fallo(w, http.StatusBadRequest, "VALIDACION", "No se pudieron leer los archivos.")
		return
	}
	p.Placa = r.FormValue("placa")
	p.NIV = r.FormValue("niv")
	p.NombreCompleto = r.FormValue("nombre")
	p.ConsultarREPUVE = r.FormValue("repuve") == "1"

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	inf, err := s.v.Probar(ctx, p)
	switch {
	case errors.Is(err, servicio.ErrFaltanFotos):
		fallo(w, http.StatusBadRequest, "VALIDACION", err.Error())
		return
	case err != nil:
		s.log.Error("prueba de validación fallida", "error", err)
		fallo(w, http.StatusBadGateway, "ERROR_INTERNO", "El analizador de documentos no respondió.")
		return
	}
	exito(w, http.StatusOK, inf)
}

// emitirPase devuelve el pase de una solicitud aprobada, o emite uno nuevo si
// no tiene ninguno activo (por ejemplo, después de revocar el anterior).
func (s *Servidor) emitirPase(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !modelo.FolioValido(id) {
		fallo(w, http.StatusBadRequest, "VALIDACION", "El folio no tiene formato UUID.")
		return
	}
	info, err := s.v.EmitirPase(r.Context(), id, true)
	switch {
	case errors.Is(err, servicio.ErrNoEncontrada):
		fallo(w, http.StatusNotFound, "NO_ENCONTRADO", err.Error())
	case errors.Is(err, servicio.ErrNoAprobada), errors.Is(err, servicio.ErrPasesDeshabilitados):
		fallo(w, http.StatusConflict, "CONFLICTO", err.Error())
	case err != nil:
		s.log.Error("emisión de pase fallida", "solicitud", id, "error", err)
		fallo(w, http.StatusBadGateway, "ERROR_INTERNO", "No se pudo emitir el pase. Revisa los logs del servidor.")
	default:
		exito(w, http.StatusOK, info)
	}
}

// verificarPase: {"codigo": "<contenido del QR>"}. Lo usa la caseta al escanear.
func (s *Servidor) verificarPase(w http.ResponseWriter, r *http.Request) {
	var cuerpo struct {
		Codigo string `json:"codigo"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxWebhook)).Decode(&cuerpo); err != nil {
		fallo(w, http.StatusBadRequest, "VALIDACION", "El cuerpo debe ser JSON con codigo.")
		return
	}
	v, err := s.v.VerificarPase(r.Context(), cuerpo.Codigo)
	switch {
	case errors.Is(err, servicio.ErrPasesDeshabilitados):
		fallo(w, http.StatusConflict, "CONFLICTO", err.Error())
	case err != nil:
		s.log.Error("verificación de pase fallida", "error", err)
		fallo(w, http.StatusBadGateway, "ERROR_INTERNO", "No se pudo consultar el pase. Intenta de nuevo.")
	default:
		exito(w, http.StatusOK, v)
	}
}

// imagenPase sirve el PNG del QR. Solo dibuja códigos con firma válida, para
// que no sirva como generador de QR arbitrarios.
func (s *Servidor) imagenPase(w http.ResponseWriter, r *http.Request) {
	codigo, ok := strings.CutSuffix(r.PathValue("archivo"), ".png")
	if !ok {
		http.NotFound(w, r)
		return
	}
	png, err := s.v.ImagenPase(codigo)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(png)
}

// verificarAlumno: {"usuario": "matricula@alumnos.uat.edu.mx", "contrasena": "..."}.
// La contraseña solo se reenvía al portal de la UAT; no se guarda ni se registra.
func (s *Servidor) verificarAlumno(w http.ResponseWriter, r *http.Request) {
	if s.o.Registro == nil {
		fallo(w, http.StatusServiceUnavailable, "NO_DISPONIBLE", "El registro con cuenta institucional no está habilitado.")
		return
	}
	var cuerpo struct {
		Usuario    string `json:"usuario"`
		Contrasena string `json:"contrasena"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&cuerpo); err != nil {
		fallo(w, http.StatusBadRequest, "VALIDACION", "Envía tu correo institucional y tu contraseña.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	res, err := s.o.Registro.Registrar(ctx, cuerpo.Usuario, cuerpo.Contrasena, ipCliente(r))
	cuerpo.Contrasena = ""
	var servicioUAT *uat.ErrServicio
	switch {
	case errors.Is(err, uat.ErrUsuario):
		fallo(w, http.StatusBadRequest, "VALIDACION", "Usa tu correo institucional de alumno (matrícula@alumnos.uat.edu.mx) o solo tu matrícula.")
	case errors.Is(err, registro.ErrContrasena):
		fallo(w, http.StatusBadRequest, "VALIDACION", "Escribe tu contraseña institucional.")
	case errors.Is(err, uat.ErrCredenciales):
		fallo(w, http.StatusUnauthorized, "CREDENCIALES", "La UAT no reconoció tu usuario o contraseña. Usa los mismos datos con los que entras al portal de alumnos.")
	case errors.Is(err, uat.ErrLimite), errors.Is(err, registro.ErrLimite):
		fallo(w, http.StatusTooManyRequests, "LIMITE", "Hiciste demasiados intentos. Espera unos minutos y vuelve a intentarlo.")
	case errors.As(err, &servicioUAT):
		s.log.Warn("servicio UAT falló", "estado", servicioUAT.Estado, "codigo", servicioUAT.Codigo)
		fallo(w, http.StatusBadGateway, "UAT_NO_DISPONIBLE", "El portal de la UAT no respondió. Intenta de nuevo en unos minutos.")
	case errors.Is(err, registro.ErrAlmacen):
		fallo(w, http.StatusBadGateway, "ERROR_INTERNO", "Confirmamos tu cuenta, pero no se pudo guardar tu registro. Intenta de nuevo.")
	case err != nil:
		s.log.Error("registro de alumno fallido", "error", err)
		fallo(w, http.StatusInternalServerError, "ERROR_INTERNO", "Error interno del servidor.")
	default:
		exito(w, http.StatusOK, res)
	}
}
