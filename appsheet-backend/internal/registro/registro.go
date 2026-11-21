// Package registro da de alta al alumno en la tabla Alumnos de AppSheet
// después de confirmar, con su cuenta institucional, que está inscrito.
//
// Con esto el padrón se llena solo: el alumno inicia sesión una vez con su
// correo y contraseña de la UAT, el portal de alumnos confirma su situación y
// el backend escribe (o actualiza) su fila con el nombre oficial. Ese nombre
// es contra el que después se coteja la INE.
package registro

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"registrovehicular/internal/modelo"
	"registrovehicular/internal/uat"
	"registrovehicular/internal/validacion"
)

// ErrContrasena: falta la contraseña o es demasiado larga.
var ErrContrasena = errors.New("escribe tu contraseña institucional")

// ErrLimite: el alumno o su red hicieron demasiados intentos.
var ErrLimite = errors.New("demasiados intentos")

// ErrAlmacen: la UAT confirmó la cuenta pero no se pudo escribir en AppSheet.
var ErrAlmacen = errors.New("no se pudo guardar el registro en AppSheet")

type Verificador interface {
	Verificar(ctx context.Context, correo, contrasena, ipCliente string) (uat.Alumno, error)
}

type Almacen interface {
	Alumno(ctx context.Context, correo string) (*modelo.Alumno, error)
	GuardarAlumno(ctx context.Context, a modelo.Alumno, nuevo bool) error
}

type Opciones struct {
	Dominio            string   // dominio de los correos de alumno (alumnos.uat.edu.mx)
	SituacionesActivas []string // situaciones del portal que cuentan como inscrito
	AppURL             string   // enlace a la app de AppSheet que se muestra al terminar
	Ahora              func() time.Time
	Log                *slog.Logger
	// Límites de intentos (cero = valores por defecto).
	IntentosIP, IntentosCuenta int
	Ventana                    time.Duration
}

// Resultado es lo que ve el alumno.
type Resultado struct {
	Correo    string `json:"correo"`
	Matricula string `json:"matricula"`
	Nombre    string `json:"nombre"`
	Programa  string `json:"programa"`
	Situacion string `json:"situacion"`
	Activo    bool   `json:"activo"`
	// Guardado: la fila de Alumnos quedó escrita en AppSheet.
	Guardado bool   `json:"guardado"`
	Nuevo    bool   `json:"nuevo"`
	AppURL   string `json:"appUrl,omitempty"`
	Mensaje  string `json:"mensaje"`
}

type Servicio struct {
	uat     Verificador
	almacen Almacen // nil = AppSheet sin configurar: solo se verifica
	o       Opciones
	activas map[string]bool
	limites *limitador

	mu    sync.Mutex
	enUso map[string]bool
}

func Nuevo(v Verificador, a Almacen, o Opciones) *Servicio {
	if o.Ahora == nil {
		o.Ahora = time.Now
	}
	if o.Log == nil {
		o.Log = slog.Default()
	}
	if o.IntentosIP <= 0 {
		o.IntentosIP = 10
	}
	if o.IntentosCuenta <= 0 {
		o.IntentosCuenta = 5
	}
	if o.Ventana <= 0 {
		o.Ventana = 15 * time.Minute
	}
	activas := map[string]bool{}
	for _, s := range o.SituacionesActivas {
		if s = normalizar(s); s != "" {
			activas[s] = true
		}
	}
	return &Servicio{
		uat: v, almacen: a, o: o, activas: activas,
		limites: nuevoLimitador(o.Ventana, o.Ahora),
		enUso:   map[string]bool{},
	}
}

// Registrar verifica la cuenta con la UAT y escribe la fila del alumno.
func (s *Servicio) Registrar(ctx context.Context, usuario, contrasena, ipCliente string) (*Resultado, error) {
	correo, err := uat.NormalizarUsuario(usuario, s.o.Dominio)
	if err != nil {
		return nil, err
	}
	if contrasena == "" || len(contrasena) > 512 {
		return nil, ErrContrasena
	}
	// Por red: todos los intentos. Por cuenta: solo los rechazados, para que
	// nadie pueda probar contraseñas de una cuenta sin límite.
	if !s.limites.permitir("ip:"+ipCliente, s.o.IntentosIP) || s.limites.excedido("cuenta:"+correo, s.o.IntentosCuenta) {
		return nil, ErrLimite
	}
	if !s.ocupar(correo) {
		return nil, ErrLimite
	}
	defer s.liberar(correo)

	a, err := s.uat.Verificar(ctx, correo, contrasena, ipCliente)
	if errors.Is(err, uat.ErrCredenciales) {
		s.limites.permitir("cuenta:"+correo, s.o.IntentosCuenta)
	}
	if err != nil {
		return nil, err
	}

	r := &Resultado{
		Correo: a.Correo, Matricula: a.Matricula, Nombre: a.Nombre,
		Programa: a.Programa, Situacion: a.Situacion, Activo: s.activa(a.Situacion),
	}
	s.o.Log.Info("cuenta UAT verificada", "activo", r.Activo, "situacion", a.Situacion, "conNombre", a.Nombre != "")

	if s.almacen != nil {
		if err := s.guardar(ctx, a, r); err != nil {
			s.o.Log.Error("no se pudo guardar el alumno", "error", err)
			return nil, ErrAlmacen
		}
	}
	if r.Activo && r.Guardado {
		r.AppURL = s.o.AppURL
	}
	r.Mensaje = mensaje(r, s.almacen != nil)
	return r, nil
}

func (s *Servicio) guardar(ctx context.Context, a uat.Alumno, r *Resultado) error {
	existente, err := s.almacen.Alumno(ctx, a.Correo)
	if err != nil {
		return fmt.Errorf("leer Alumnos: %w", err)
	}
	if existente == nil && !r.Activo {
		// Minimización: quien no puede tramitar no necesita una fila.
		return nil
	}
	fila := modelo.Alumno{
		Correo:         a.Correo,
		Matricula:      a.Matricula,
		NombreCompleto: a.Nombre,
		Activo:         r.Activo,
		Programa:       a.Programa,
		SituacionUAT:   a.Situacion,
		VerificadoUAT:  s.o.Ahora(),
	}
	if existente != nil {
		// Si el portal no dio algún dato, conserva el que ya estaba en el padrón.
		if fila.NombreCompleto == "" {
			fila.NombreCompleto = existente.NombreCompleto
		}
		if fila.Matricula == "" {
			fila.Matricula = existente.Matricula
		}
		if fila.Programa == "" {
			fila.Programa = existente.Programa
		}
	}
	if err := s.almacen.GuardarAlumno(ctx, fila, existente == nil); err != nil {
		return err
	}
	r.Guardado = true
	r.Nuevo = existente == nil
	if r.Nombre == "" {
		r.Nombre = fila.NombreCompleto
	}
	return nil
}

func mensaje(r *Resultado, conAppSheet bool) string {
	switch {
	case r.Situacion == "":
		return "Tu cuenta es válida, pero el portal de la UAT no mostró tu situación escolar. Acude a Control Vehicular para completar tu registro."
	case !r.Activo:
		return fmt.Sprintf("La UAT reporta tu situación como «%s». Solo los alumnos inscritos pueden registrar un vehículo. Si es un error, acude a Servicios Escolares.", r.Situacion)
	case !conAppSheet:
		return "Confirmamos que eres alumno inscrito. El alta en la app de registro vehicular todavía no está conectada; vuelve a intentarlo más tarde."
	case r.Nombre == "":
		return "Quedaste registrado como alumno inscrito. El portal no mostró tu nombre, así que una persona cotejará tu INE."
	default:
		return "Quedaste registrado como alumno inscrito. Ya puedes registrar tu vehículo en la app con tu cuenta institucional."
	}
}

// activa decide si la situación del portal cuenta como alumno inscrito. Se
// compara palabra por palabra para que «INACTIVO» o «NO INSCRITO» no pasen.
func (s *Servicio) activa(situacion string) bool {
	palabras := strings.Fields(normalizar(situacion))
	if len(palabras) == 0 {
		return false
	}
	if s.activas[strings.Join(palabras, " ")] {
		return !tieneNegacion(palabras)
	}
	hay := false
	for _, p := range palabras {
		if s.activas[p] {
			hay = true
		}
	}
	return hay && !tieneNegacion(palabras)
}

var negaciones = map[string]bool{
	"NO": true, "BAJA": true, "INACTIVO": true, "SUSPENDIDO": true, "SUSPENSION": true,
	"CANCELADO": true, "EGRESADO": true, "TITULADO": true, "EXPULSADO": true,
}

func tieneNegacion(palabras []string) bool {
	for _, p := range palabras {
		if negaciones[p] {
			return true
		}
	}
	return false
}

// normalizar: mayúsculas, sin acentos ni signos («Activo.» → «ACTIVO»).
func normalizar(s string) string { return strings.Join(validacion.TokensNombre(s), " ") }

func (s *Servicio) ocupar(correo string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.enUso[correo] {
		return false
	}
	s.enUso[correo] = true
	return true
}

func (s *Servicio) liberar(correo string) {
	s.mu.Lock()
	delete(s.enUso, correo)
	s.mu.Unlock()
}

// limitador cuenta intentos en una ventana fija por clave.
type limitador struct {
	mu      sync.Mutex
	ventana time.Duration
	ahora   func() time.Time
	cuentas map[string]*cuenta
}

type cuenta struct {
	n      int
	inicio time.Time
}

func nuevoLimitador(v time.Duration, ahora func() time.Time) *limitador {
	return &limitador{ventana: v, ahora: ahora, cuentas: map[string]*cuenta{}}
}

// permitir suma un intento y dice si sigue dentro del límite.
func (l *limitador) permitir(clave string, max int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	c := l.vigente(clave)
	c.n++
	return c.n <= max
}

// excedido dice si la clave ya agotó sus intentos, sin sumar uno.
func (l *limitador) excedido(clave string, max int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.vigente(clave).n >= max
}

func (l *limitador) vigente(clave string) *cuenta {
	ahora := l.ahora()
	if len(l.cuentas) > 10000 {
		for k, c := range l.cuentas {
			if ahora.Sub(c.inicio) > l.ventana {
				delete(l.cuentas, k)
			}
		}
	}
	c, ok := l.cuentas[clave]
	if !ok || ahora.Sub(c.inicio) > l.ventana {
		c = &cuenta{inicio: ahora}
		l.cuentas[clave] = c
	}
	return c
}
