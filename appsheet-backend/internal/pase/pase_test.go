package pase

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

const id = "6f1c2d3e-4a5b-4c6d-8e7f-001122334455"

func TestCodigoYVerificacion(t *testing.T) {
	zona, _ := time.LoadLocation("America/Monterrey")
	f := NuevoFirmador("secreto-de-prueba-con-32-caracteres!!", zona)
	vence := time.Date(2027, 7, 31, 10, 0, 0, 0, zona)
	codigo := f.Codigo(id, "XYZ123A", vence)

	if !strings.HasPrefix(codigo, "RV1."+id+".XYZ123A.20270731.") || len(codigo) > MaxLongitud {
		t.Fatalf("código %q", codigo)
	}
	d, err := f.Verificar(codigo)
	if err != nil {
		t.Fatal(err)
	}
	if d.PaseID != id || d.Placa != "XYZ123A" || d.Vence.Format("2006-01-02 15:04") != "2027-07-31 23:59" {
		t.Fatalf("datos %+v", d)
	}
}

func TestRechazaAlteraciones(t *testing.T) {
	f := NuevoFirmador("secreto-de-prueba-con-32-caracteres!!", time.UTC)
	codigo := f.Codigo(id, "XYZ123A", time.Date(2027, 7, 31, 0, 0, 0, 0, time.UTC))

	otraPlaca := strings.Replace(codigo, "XYZ123A", "ABC123A", 1)
	otraFecha := strings.Replace(codigo, "20270731", "20300731", 1)
	for _, c := range []string{otraPlaca, otraFecha} {
		if _, err := f.Verificar(c); !errors.Is(err, ErrFirma) {
			t.Errorf("Verificar(%q) = %v, se esperaba ErrFirma", c, err)
		}
	}
	if _, err := NuevoFirmador("otro-secreto-distinto-de-32-caracteres", time.UTC).Verificar(codigo); !errors.Is(err, ErrFirma) {
		t.Errorf("con otro secreto: %v", err)
	}
	for _, c := range []string{"", "hola", "RV1.a.b.c.d", "RV2" + codigo[3:], codigo + ".x"} {
		if _, err := f.Verificar(c); !errors.Is(err, ErrFormato) {
			t.Errorf("Verificar(%q) = %v, se esperaba ErrFormato", c, err)
		}
	}
}

func TestPNG(t *testing.T) {
	f := NuevoFirmador("secreto-de-prueba-con-32-caracteres!!", time.UTC)
	img, err := PNG(f.Codigo(id, "XYZ123A", time.Now()), 512)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(img, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatal("no es un PNG")
	}
}
