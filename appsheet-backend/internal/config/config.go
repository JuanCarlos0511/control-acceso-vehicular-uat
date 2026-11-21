// Package config lee la configuración del servicio desde variables de entorno.
package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Tablas son los nombres de las tablas en la app de AppSheet.
type Tablas struct {
	Solicitudes  string
	Alumnos      string
	Revisiones   string
	Resultados   string
	Validaciones string // vacía = no se registra la bitácora de validaciones
	Pases        string
}

type AppSheet struct {
	// Habilitado es falso si no hay APPSHEET_APP_ID, APPSHEET_ACCESS_KEY ni
	// APPSHEET_RUN_AS: el servicio arranca, pero no lee ni escribe la app.
	Habilitado bool
	BaseURL    string // https://api.appsheet.com (o https://eu.appsheet.com)
	AppID      string
	AccessKey  string
	RunAs      string // correo con el que el backend llama a la API (USEREMAIL())
	Locale     string
	Timezone   string // nombre de zona de AppSheet, p. ej. "Central Standard Time (Mexico)"
	Tablas     Tablas
}

type Imagenes struct {
	Fuente         string // drive | local | appsheet
	DriveCarpetaID string
	DirLocal       string
	AppName        string // nombre interno de la app, solo para la fuente "appsheet"
}

// REPUVE: la consulta la hace el analizador con Playwright; aquí solo se
// decide si se pide y cuánto se espera la respuesta (incluye la fila de
// consultas, que el analizador atiende de una en una). El resto de la
// configuración (REPUVE_URL, REPUVE_HEADLESS, REPUVE_INTERVALO_MIN, …) se le
// pasa al analizador.
type REPUVE struct {
	Habilitado bool
	EsperaMax  time.Duration
}

// Pases configura el código QR que se emite al aprobar una solicitud.
type Pases struct {
	Habilitado bool
	Secreto    string    // PASE_SECRETO: firma los códigos; si está vacío no se emiten pases
	URLPublica string    // URL_PUBLICA: base HTTPS del backend, para la imagen del QR
	Dias       int       // vigencia en días desde la emisión
	VenceFijo  time.Time // PASE_VENCE: fecha fija de vencimiento (p. ej. fin del ciclo escolar)
}

// UAT configura el registro del alumno con su cuenta institucional
// (scrap-uat-service inicia sesión en el portal de alumnos).
type UAT struct {
	Habilitado         bool
	URL                string // UAT_SERVICIO_URL, p. ej. http://scrap-uat:3100
	Clave              string // UAT_SERVICIO_CLAVE = API_GATEWAY_KEYS del servicio
	Dominio            string // UAT_DOMINIO_ALUMNOS
	SituacionesActivas []string
	Espera             time.Duration
	AppURL             string // APPSHEET_APP_URL: enlace que ve el alumno al terminar
}

type Config struct {
	Puerto           string
	ZonaHoraria      *time.Location
	LogNivel         string
	AppSheet         AppSheet
	GoogleSheetsID   string // modo directo, alternativo a la API de AppSheet
	WebhookSecreto   string
	APIToken         string
	AnalizadorURL    string
	AnalizadorToken  string
	Imagenes         Imagenes
	REPUVE           REPUVE
	Pases            Pases
	UAT              UAT
	CORSOrigenes     []string
	AutoAprobar      bool
	BarridoIntervalo time.Duration
	BarridoDias      int
	Trabajadores     int
	CorreoSistema    string
}

// Cargar lee y valida la configuración. Devuelve todos los errores juntos.
func Cargar() (Config, error) {
	var errs []error
	l := lector{errs: &errs}

	zona, err := time.LoadLocation(l.texto("ZONA_HORARIA", "America/Monterrey"))
	if err != nil {
		errs = append(errs, fmt.Errorf("ZONA_HORARIA: %w", err))
		zona = time.UTC
	}
	idSheets := l.texto("GOOGLE_SHEETS_ID", "")
	intervaloBarrido := 15 * time.Minute
	if idSheets != "" {
		intervaloBarrido = time.Minute
	}

	c := Config{
		Puerto:         l.texto("PUERTO", "8080"),
		ZonaHoraria:    zona,
		LogNivel:       l.texto("LOG_NIVEL", "info"),
		GoogleSheetsID: idSheets,
		AppSheet: AppSheet{
			BaseURL:   strings.TrimRight(l.texto("APPSHEET_BASE_URL", "https://api.appsheet.com"), "/"),
			AppID:     l.texto("APPSHEET_APP_ID", ""),
			AccessKey: l.texto("APPSHEET_ACCESS_KEY", ""),
			RunAs:     strings.ToLower(l.texto("APPSHEET_RUN_AS", "")),
			Locale:    l.texto("APPSHEET_LOCALE", "en-US"),
			Timezone:  l.texto("APPSHEET_TIMEZONE", "Central Standard Time (Mexico)"),
			Tablas: Tablas{
				Solicitudes:  l.texto("APPSHEET_TABLA_SOLICITUDES", "Solicitudes"),
				Alumnos:      l.texto("APPSHEET_TABLA_ALUMNOS", "Alumnos"),
				Revisiones:   l.texto("APPSHEET_TABLA_REVISIONES", "Revisiones"),
				Resultados:   l.texto("APPSHEET_TABLA_RESULTADOS", "Resultados"),
				Validaciones: l.texto("APPSHEET_TABLA_VALIDACIONES", ""),
				Pases:        l.texto("APPSHEET_TABLA_PASES", "Pases"),
			},
		},
		WebhookSecreto:  l.requerido("WEBHOOK_SECRETO"),
		APIToken:        l.requerido("API_TOKEN"),
		AnalizadorURL:   strings.TrimRight(l.texto("ANALIZADOR_URL", "http://localhost:8000"), "/"),
		AnalizadorToken: l.texto("ANALIZADOR_TOKEN", ""),
		Imagenes: Imagenes{
			Fuente:         l.texto("IMAGENES_FUENTE", "drive"),
			DriveCarpetaID: l.texto("DRIVE_CARPETA_ID", ""),
			DirLocal:       l.texto("IMAGENES_DIR_LOCAL", ""),
			AppName:        l.texto("APPSHEET_APP_NAME", ""),
		},
		REPUVE: REPUVE{
			Habilitado: l.booleano("REPUVE_HABILITADO", true),
			EsperaMax:  l.duracion("REPUVE_ESPERA_MAX", 5*time.Minute),
		},
		AutoAprobar:      l.booleano("AUTO_APROBAR", idSheets == ""),
		BarridoIntervalo: l.duracion("BARRIDO_INTERVALO", intervaloBarrido),
		BarridoDias:      l.entero("BARRIDO_DIAS", 14),
		Trabajadores:     l.entero("TRABAJADORES", 2),
	}
	c.CorreoSistema = strings.ToLower(l.texto("CORREO_SISTEMA", c.AppSheet.RunAs))

	// AppSheet: las tres credenciales juntas o ninguna.
	as := c.AppSheet
	c.AppSheet.Habilitado = as.AppID != "" || as.AccessKey != "" || as.RunAs != ""
	if idSheets != "" {
		if !regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(idSheets) {
			errs = append(errs, errors.New("GOOGLE_SHEETS_ID debe ser el ID del archivo, no su URL"))
		}
		if c.AppSheet.Habilitado {
			errs = append(errs, errors.New("usa GOOGLE_SHEETS_ID o las credenciales de AppSheet, no ambos"))
		}
		if c.BarridoIntervalo <= 0 {
			errs = append(errs, errors.New("con GOOGLE_SHEETS_ID se necesita BARRIDO_INTERVALO mayor que cero"))
		}
		if c.CorreoSistema == "" {
			c.CorreoSistema = "registro-vehicular@backend.invalid"
		}
	}
	if c.AppSheet.Habilitado {
		for clave, v := range map[string]string{"APPSHEET_APP_ID": as.AppID, "APPSHEET_ACCESS_KEY": as.AccessKey, "APPSHEET_RUN_AS": as.RunAs} {
			if v == "" {
				errs = append(errs, fmt.Errorf("%s es obligatoria si se configura AppSheet", clave))
			}
		}
	}

	c.UAT = UAT{
		URL:                strings.TrimRight(l.texto("UAT_SERVICIO_URL", ""), "/"),
		Clave:              l.texto("UAT_SERVICIO_CLAVE", ""),
		Dominio:            strings.ToLower(strings.TrimPrefix(l.texto("UAT_DOMINIO_ALUMNOS", "alumnos.uat.edu.mx"), "@")),
		SituacionesActivas: lista(l.texto("UAT_SITUACIONES_ACTIVAS", "ACTIVO,INSCRITO,REINSCRITO,REGULAR,IRREGULAR,VIGENTE")),
		Espera:             l.duracion("UAT_ESPERA", 90*time.Second),
		AppURL:             l.texto("APPSHEET_APP_URL", ""),
	}
	// También en el modo Sheets: el alumno se verifica con la UAT y su fila se
	// escribe en la pestaña Alumnos de la hoja.
	c.UAT.Habilitado = c.UAT.URL != ""
	if c.UAT.Habilitado && len(c.UAT.Clave) < 32 {
		errs = append(errs, errors.New("UAT_SERVICIO_CLAVE debe tener al menos 32 caracteres (es la API_GATEWAY_KEYS de scrap-uat-service)"))
	}
	if c.UAT.AppURL != "" && !strings.HasPrefix(c.UAT.AppURL, "https://") {
		errs = append(errs, errors.New("APPSHEET_APP_URL debe empezar con https://"))
	}
	c.CORSOrigenes = lista(l.texto("CORS_ORIGENES", ""))
	for _, o := range c.CORSOrigenes {
		if !strings.HasPrefix(o, "https://") && !strings.HasPrefix(o, "http://localhost") {
			errs = append(errs, fmt.Errorf("CORS_ORIGENES: %q debe ser un origen https://", o))
		}
	}

	c.Pases = Pases{
		Secreto:    l.texto("PASE_SECRETO", ""),
		URLPublica: strings.TrimRight(l.texto("URL_PUBLICA", ""), "/"),
		Dias:       l.entero("PASE_DIAS", 365),
	}
	c.Pases.Habilitado = c.Pases.Secreto != ""
	if v := l.texto("PASE_VENCE", ""); v != "" {
		t, err := time.ParseInLocation("2006-01-02", v, zona)
		if err != nil {
			errs = append(errs, fmt.Errorf("PASE_VENCE: %q no es una fecha AAAA-MM-DD", v))
		}
		c.Pases.VenceFijo = t
	}
	if c.Pases.Habilitado {
		if len(c.Pases.Secreto) < 32 {
			errs = append(errs, errors.New("PASE_SECRETO debe tener al menos 32 caracteres"))
		}
		if !strings.HasPrefix(c.Pases.URLPublica, "https://") && !strings.HasPrefix(c.Pases.URLPublica, "http://localhost") {
			errs = append(errs, errors.New("URL_PUBLICA debe ser la URL https del backend (AppSheet carga la imagen del QR desde ahí)"))
		}
		if c.Pases.Dias < 1 && c.Pases.VenceFijo.IsZero() {
			errs = append(errs, errors.New("PASE_DIAS debe ser al menos 1"))
		}
	}

	switch c.Imagenes.Fuente {
	case "drive":
		if c.Imagenes.DriveCarpetaID == "" && (c.AppSheet.Habilitado || idSheets != "") {
			errs = append(errs, errors.New("DRIVE_CARPETA_ID es obligatorio con IMAGENES_FUENTE=drive"))
		}
	case "local":
		if c.Imagenes.DirLocal == "" {
			errs = append(errs, errors.New("IMAGENES_DIR_LOCAL es obligatorio con IMAGENES_FUENTE=local"))
		}
	case "appsheet":
		if idSheets != "" {
			errs = append(errs, errors.New("con GOOGLE_SHEETS_ID usa fotos en Drive o un directorio local de prueba"))
		}
		if c.Imagenes.AppName == "" {
			errs = append(errs, errors.New("APPSHEET_APP_NAME es obligatorio con IMAGENES_FUENTE=appsheet"))
		}
	default:
		errs = append(errs, fmt.Errorf("IMAGENES_FUENTE=%q no es válida (drive, local o appsheet)", c.Imagenes.Fuente))
	}
	if c.Trabajadores < 1 {
		errs = append(errs, errors.New("TRABAJADORES debe ser al menos 1"))
	}
	if len(c.WebhookSecreto) > 0 && len(c.WebhookSecreto) < 24 {
		errs = append(errs, errors.New("WEBHOOK_SECRETO debe tener al menos 24 caracteres"))
	}
	if len(c.APIToken) > 0 && len(c.APIToken) < 24 {
		errs = append(errs, errors.New("API_TOKEN debe tener al menos 24 caracteres"))
	}
	return c, errors.Join(errs...)
}

func lista(v string) []string {
	var salida []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimRight(strings.TrimSpace(p), "/"); p != "" {
			salida = append(salida, p)
		}
	}
	return salida
}

type lector struct{ errs *[]error }

func (l lector) texto(clave, defecto string) string {
	if v, ok := os.LookupEnv(clave); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return defecto
}

func (l lector) requerido(clave string) string {
	v := l.texto(clave, "")
	if v == "" {
		*l.errs = append(*l.errs, fmt.Errorf("%s es obligatoria", clave))
	}
	return v
}

func (l lector) booleano(clave string, defecto bool) bool {
	v := l.texto(clave, "")
	if v == "" {
		return defecto
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		*l.errs = append(*l.errs, fmt.Errorf("%s: %q no es booleano", clave, v))
		return defecto
	}
	return b
}

func (l lector) entero(clave string, defecto int) int {
	v := l.texto(clave, "")
	if v == "" {
		return defecto
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		*l.errs = append(*l.errs, fmt.Errorf("%s: %q no es entero", clave, v))
		return defecto
	}
	return n
}

func (l lector) duracion(clave string, defecto time.Duration) time.Duration {
	v := l.texto(clave, "")
	if v == "" {
		return defecto
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		*l.errs = append(*l.errs, fmt.Errorf("%s: %q no es una duración (p. ej. 30s, 10m)", clave, v))
		return defecto
	}
	return d
}
