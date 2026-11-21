package registro

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"registrovehicular/internal/modelo"
	"registrovehicular/internal/uat"
)

type uatFalsa struct {
	a     uat.Alumno
	err   error
	veces int
}

func (f *uatFalsa) Verificar(_ context.Context, correo, _, _ string) (uat.Alumno, error) {
	f.veces++
	if f.err != nil {
		return uat.Alumno{}, f.err
	}
	a := f.a
	a.Correo = correo
	return a, nil
}

type alumnosFalsos struct {
	filas    map[string]modelo.Alumno
	nuevos   int
	editados int
}

func (f *alumnosFalsos) Alumno(_ context.Context, correo string) (*modelo.Alumno, error) {
	if a, ok := f.filas[correo]; ok {
		return &a, nil
	}
	return nil, nil
}

func (f *alumnosFalsos) GuardarAlumno(_ context.Context, a modelo.Alumno, nuevo bool) error {
	if nuevo {
		f.nuevos++
	} else {
		f.editados++
	}
	f.filas[a.Correo] = a
	return nil
}

var ahora = time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)

func nuevo(u Verificador, a Almacen) *Servicio {
	return Nuevo(u, a, Opciones{
		Dominio:            "alumnos.uat.edu.mx",
		SituacionesActivas: []string{"ACTIVO", "INSCRITO", "REGULAR"},
		AppURL:             "https://www.appsheet.com/start/x",
		Ahora:              func() time.Time { return ahora },
		Log:                slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

func TestAltaDeAlumnoActivo(t *testing.T) {
	u := &uatFalsa{a: uat.Alumno{Matricula: "a1", Nombre: "ANA PÉREZ", Programa: "ISC", Situacion: "ACTIVO"}}
	alm := &alumnosFalsos{filas: map[string]modelo.Alumno{}}
	r, err := nuevo(u, alm).Registrar(context.Background(), "A1", "x", "1.1.1.1")
	if err != nil {
		t.Fatal(err)
	}
	if !r.Activo || !r.Guardado || !r.Nuevo || r.AppURL == "" || alm.nuevos != 1 {
		t.Fatalf("resultado %+v, nuevos %d", r, alm.nuevos)
	}
	fila := alm.filas["a1@alumnos.uat.edu.mx"]
	if !fila.Activo || fila.NombreCompleto != "ANA PÉREZ" || fila.SituacionUAT != "ACTIVO" || !fila.VerificadoUAT.Equal(ahora) {
		t.Errorf("fila %+v", fila)
	}
}

func TestBajaDesactivaAlumnoExistenteYConservaNombre(t *testing.T) {
	correo := "a1@alumnos.uat.edu.mx"
	alm := &alumnosFalsos{filas: map[string]modelo.Alumno{correo: {Correo: correo, NombreCompleto: "ANA PÉREZ", Activo: true}}}
	u := &uatFalsa{a: uat.Alumno{Situacion: "BAJA TEMPORAL"}}
	r, err := nuevo(u, alm).Registrar(context.Background(), correo, "x", "1.1.1.1")
	if err != nil {
		t.Fatal(err)
	}
	if r.Activo || !r.Guardado || r.Nuevo || r.AppURL != "" || alm.editados != 1 {
		t.Fatalf("resultado %+v", r)
	}
	if f := alm.filas[correo]; f.Activo || f.NombreCompleto != "ANA PÉREZ" {
		t.Errorf("fila %+v", f)
	}
}

func TestNoActivoNuevoNoSeGuarda(t *testing.T) {
	alm := &alumnosFalsos{filas: map[string]modelo.Alumno{}}
	r, err := nuevo(&uatFalsa{a: uat.Alumno{Situacion: "EGRESADO"}}, alm).Registrar(context.Background(), "a2", "x", "1.1.1.1")
	if err != nil || r.Activo || r.Guardado || alm.nuevos != 0 {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestSinAppSheetSoloVerifica(t *testing.T) {
	r, err := nuevo(&uatFalsa{a: uat.Alumno{Situacion: "Inscrito"}}, nil).Registrar(context.Background(), "a3", "x", "1.1.1.1")
	if err != nil || !r.Activo || r.Guardado || r.AppURL != "" {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestSituacionesActivas(t *testing.T) {
	s := nuevo(nil, nil)
	for sit, quiere := range map[string]bool{
		"ACTIVO": true, "Activo.": true, "ALUMNO REGULAR": true, "inscrito": true,
		"INACTIVO": false, "NO INSCRITO": false, "BAJA": false, "ACTIVO - BAJA TEMPORAL": false, "": false, "OYENTE": false,
	} {
		if got := s.activa(sit); got != quiere {
			t.Errorf("activa(%q) = %v", sit, got)
		}
	}
}

func TestLimiteDeIntentosFallidosPorCuenta(t *testing.T) {
	u := &uatFalsa{err: uat.ErrCredenciales}
	s := nuevo(u, nil)
	for i := range 5 {
		if _, err := s.Registrar(context.Background(), "a4", "mala", "10.0.0."+string(rune('1'+i))); !errors.Is(err, uat.ErrCredenciales) {
			t.Fatalf("intento %d: %v", i, err)
		}
	}
	if _, err := s.Registrar(context.Background(), "a4", "otra", "10.0.0.9"); !errors.Is(err, ErrLimite) {
		t.Fatalf("sexto intento: %v", err)
	}
	if u.veces != 5 {
		t.Errorf("el sexto intento llegó a la UAT (%d llamadas)", u.veces)
	}
}

func TestLimitePorIP(t *testing.T) {
	s := nuevo(&uatFalsa{a: uat.Alumno{Situacion: "ACTIVO"}}, nil)
	var err error
	for i := range 11 {
		_, err = s.Registrar(context.Background(), "a"+string(rune('a'+i))+"99", "x", "1.2.3.4")
	}
	if !errors.Is(err, ErrLimite) {
		t.Fatalf("intento 11 desde la misma IP: %v", err)
	}
}

func TestValidaEntradaSinLlamarALaUAT(t *testing.T) {
	u := &uatFalsa{}
	s := nuevo(u, nil)
	if _, err := s.Registrar(context.Background(), "maestro@uat.edu.mx", "x", ""); !errors.Is(err, uat.ErrUsuario) {
		t.Errorf("dominio de personal: %v", err)
	}
	if _, err := s.Registrar(context.Background(), "a5", "", ""); !errors.Is(err, ErrContrasena) {
		t.Errorf("sin contraseña: %v", err)
	}
	if u.veces != 0 {
		t.Error("se llamó a la UAT con datos inválidos")
	}
}
