// Package uat consulta el servicio scrap-uat-service para confirmar que una
// cuenta institucional es de un alumno inscrito.
//
// El servicio inicia sesión en el portal de alumnos (alumnossur.uat.edu.mx)
// con el usuario y la contraseña que escribe el alumno. La contraseña solo
// viaja en esa petición: aquí no se guarda ni se registra.
package uat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	// ErrCredenciales: el portal de la UAT rechazó el usuario o la contraseña.
	ErrCredenciales = errors.New("el portal de la UAT rechazó el usuario o la contraseña")
	// ErrLimite: demasiados intentos; el alumno debe esperar.
	ErrLimite = errors.New("demasiados intentos de inicio de sesión")
	// ErrUsuario: el usuario no es una cuenta institucional de alumno.
	ErrUsuario = errors.New("el usuario no es una cuenta institucional de alumno")
)

// ErrServicio: el scraper o el portal de la UAT fallaron (no es culpa del alumno).
type ErrServicio struct {
	Estado int
	Codigo string
}

func (e *ErrServicio) Error() string {
	return fmt.Sprintf("servicio UAT: HTTP %d %s", e.Estado, e.Codigo)
}

// Alumno es lo que el portal informa de la cuenta.
type Alumno struct {
	Correo      string   `json:"correo"`
	Matricula   string   `json:"matricula"`
	Nombre      string   `json:"nombre"` // en mayúsculas, como lo escribe la UAT (y la INE)
	Programa    string   `json:"programa"`
	PlanID      int      `json:"planId,omitzero"`
	Situacion   string   `json:"situacion"`
	Situaciones []string `json:"situaciones,omitempty"`
}

var localAlumno = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,63}$`)

// NormalizarUsuario acepta la matrícula sola o el correo institucional
// completo y devuelve el correo en minúsculas. Solo admite el dominio de
// alumnos para que una cuenta de personal no se registre como alumno.
func NormalizarUsuario(usuario, dominio string) (string, error) {
	u := strings.ToLower(strings.TrimSpace(usuario))
	dominio = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(dominio), "@"))
	local, dom, tieneArroba := strings.Cut(u, "@")
	if !tieneArroba {
		dom = dominio
	}
	if dom != dominio || !localAlumno.MatchString(local) {
		return "", ErrUsuario
	}
	return local + "@" + dominio, nil
}

// Cliente habla con scrap-uat-service.
type Cliente struct {
	base  string
	clave string
	http  *http.Client
}

func NuevoCliente(base, clave string, espera time.Duration) *Cliente {
	if espera <= 0 {
		espera = 90 * time.Second
	}
	return &Cliente{base: strings.TrimRight(base, "/"), clave: clave, http: &http.Client{Timeout: espera}}
}

// Salud consulta /health/ready del scraper.
func (c *Cliente) Salud(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/health/ready", nil)
	if err != nil {
		return err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4<<10))
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("scrap-uat-service: HTTP %d", res.StatusCode)
	}
	return nil
}

type respuesta struct {
	Authenticated  bool       `json:"authenticated"`
	Programs       []programa `json:"programs"`
	Situations     []string   `json:"situations"`
	SelectedCareer struct {
		Parametros map[string]any `json:"parametros"`
	} `json:"selectedCareer"`
	// Solo en versiones del servicio que ya extraen la identidad.
	Student *struct {
		FirstName       string `json:"firstName"`
		PaternalSurname string `json:"paternalSurname"`
		MaternalSurname string `json:"maternalSurname"`
	} `json:"student"`
}

type programa struct {
	PlanID    *int           `json:"planId"`
	Program   *string        `json:"program"`
	Situation *string        `json:"situation"`
	Raw       map[string]any `json:"raw"`
}

type errorServicio struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// Verificar inicia sesión en el portal de alumnos con las credenciales y
// devuelve los datos de la cuenta. correo ya debe venir de NormalizarUsuario.
// ipCliente se reenvía para que el límite de intentos del scraper sea por
// alumno y no global.
func (c *Cliente) Verificar(ctx context.Context, correo, contrasena, ipCliente string) (Alumno, error) {
	cuerpo, err := json.Marshal(map[string]any{
		"username": correo,
		"password": contrasena,
		"include": map[string]bool{
			"programPage": true, "schedule": false, "partialGrades": false, "finalGrades": false,
		},
	})
	if err != nil {
		return Alumno{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/v1/alumnos/scrape", bytes.NewReader(cuerpo))
	if err != nil {
		return Alumno{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.clave)
	if ipCliente != "" {
		req.Header.Set("X-Forwarded-For", ipCliente)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return Alumno{}, &ErrServicio{Codigo: "SIN_RESPUESTA"}
	}
	defer res.Body.Close()
	datos, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return Alumno{}, &ErrServicio{Estado: res.StatusCode, Codigo: "LECTURA"}
	}

	if res.StatusCode != http.StatusOK {
		var e errorServicio
		_ = json.Unmarshal(datos, &e)
		switch {
		case res.StatusCode == http.StatusUnauthorized && e.Error == "UAT_LOGIN_FAILED":
			return Alumno{}, ErrCredenciales
		case res.StatusCode == http.StatusTooManyRequests:
			return Alumno{}, ErrLimite
		case res.StatusCode == http.StatusBadRequest && e.Error == "VALIDATION_ERROR":
			return Alumno{}, ErrUsuario
		default:
			// GATEWAY_UNAUTHORIZED (clave mal configurada), UAT_PORTAL_ERROR, etc.
			return Alumno{}, &ErrServicio{Estado: res.StatusCode, Codigo: e.Error}
		}
	}

	var r respuesta
	if err := json.Unmarshal(datos, &r); err != nil || !r.Authenticated {
		return Alumno{}, &ErrServicio{Estado: res.StatusCode, Codigo: "RESPUESTA_INVALIDA"}
	}
	return interpretar(correo, r), nil
}

// interpretar elige el plan que seleccionó el portal (o el primero) y toma de
// él matrícula, nombre, programa y situación.
func interpretar(correo string, r respuesta) Alumno {
	a := Alumno{Correo: correo, Situaciones: r.Situations}
	if len(r.Programs) == 0 {
		a.Matricula = matriculaDeCorreo(correo)
		return a
	}
	elegido := r.Programs[0]
	if plan, ok := entero(r.SelectedCareer.Parametros["Id_Plan_Estudio_AlumnosUAT"]); ok {
		elegido = buscarPlan(r.Programs, plan, elegido)
	} else if plan, ok := entero(r.SelectedCareer.Parametros["Id_Plan_Estudio"]); ok {
		elegido = buscarPlan(r.Programs, plan, elegido)
	}
	if elegido.PlanID != nil {
		a.PlanID = *elegido.PlanID
	}
	if elegido.Program != nil {
		a.Programa = limpiar(*elegido.Program)
	}
	if elegido.Situation != nil {
		a.Situacion = strings.ToUpper(limpiar(*elegido.Situation))
	}
	if a.Situacion == "" && len(r.Programs) == 1 && len(r.Situations) > 0 {
		a.Situacion = strings.ToUpper(limpiar(r.Situations[0]))
	}

	a.Matricula = texto(elegido.Raw["Num_Matricula"])
	if a.Matricula == "" {
		a.Matricula = matriculaDeCorreo(correo)
	}
	a.Nombre = unir(texto(elegido.Raw["Txt_Nombre"]), texto(elegido.Raw["Txt_Paterno"]), texto(elegido.Raw["Txt_Materno"]))
	if a.Nombre == "" && r.Student != nil {
		a.Nombre = unir(r.Student.FirstName, r.Student.PaternalSurname, r.Student.MaternalSurname)
	}
	return a
}

func buscarPlan(ps []programa, plan int, defecto programa) programa {
	for _, p := range ps {
		if p.PlanID != nil && *p.PlanID == plan {
			return p
		}
	}
	return defecto
}

func matriculaDeCorreo(correo string) string {
	local, _, _ := strings.Cut(correo, "@")
	return local
}

func unir(partes ...string) string {
	var salida []string
	for _, p := range partes {
		if p = limpiar(p); p != "" {
			salida = append(salida, strings.ToUpper(p))
		}
	}
	return strings.Join(salida, " ")
}

func limpiar(v string) string { return strings.Join(strings.Fields(v), " ") }

func texto(v any) string {
	switch x := v.(type) {
	case string:
		return limpiar(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return ""
	}
}

func entero(v any) (int, bool) {
	switch x := v.(type) {
	case float64:
		return int(x), x > 0
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(x))
		return n, err == nil && n > 0
	default:
		return 0, false
	}
}
