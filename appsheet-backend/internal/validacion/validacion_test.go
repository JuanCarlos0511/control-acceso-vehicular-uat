package validacion

import (
	"errors"
	"strings"
	"testing"
	"time"

	"registrovehicular/internal/analizador"
	"registrovehicular/internal/modelo"
	"registrovehicular/internal/repuve"
)

const (
	nivPrueba   = "1HGCM82633A004352"
	placaPrueba = "XYZ123A"
)

var ahora = time.Date(2026, 11, 21, 10, 0, 0, 0, time.FixedZone("CST", -6*3600))

func ptr[T any](v T) *T { return &v }

func ineValida() *analizador.INE {
	a := &analizador.INE{
		EsINE:              true,
		ConfianzaDocumento: 0.9,
		Nombre:             &analizador.Nombre{Completo: "PEREZ LOPEZ MARIA FERNANDA", Confianza: 0.97},
		CURP:               &analizador.CURP{Valor: "X", FormatoValido: true, DigitoValido: true},
		Vigencia:           &analizador.Vigencia{Anio: 2033, Confianza: 0.95},
		MRZ: &analizador.MRZ{
			Valida:          true,
			Controles:       analizador.ControlesMRZ{Documento: true, Nacimiento: true, Expiracion: true, Compuesto: true},
			FechaExpiracion: ptr("2033-12-31"),
			Apellidos:       ptr("PEREZ LOPEZ"),
			Nombres:         ptr("MARIA FERNANDA"),
		},
	}
	a.Consistencia.NacimientoCurpVsMrz = ptr(true)
	a.Calidad.Frente = &analizador.Calidad{Aceptable: true}
	return a
}

func tarjetaValida() *analizador.Tarjeta {
	t := &analizador.Tarjeta{EsTarjeta: true, ConfianzaDocumento: 0.8, Marca: ptr("NISSAN"), AnioModelo: ptr(2019)}
	t.NIV.Candidatos = []analizador.CandidatoNIV{{Valor: nivPrueba, Etiquetado: true, DigitoValido: true, Confianza: 0.95}}
	t.Placa.Candidatos = []analizador.CandidatoPlaca{{Valor: placaPrueba, Etiquetado: true, Confianza: 0.95}}
	t.Calidad = &analizador.Calidad{Aceptable: true}
	return t
}

func repuveSinReporte() *repuve.Resultado {
	return &repuve.Resultado{
		Estatus: modelo.REPUVESinReporte, Criterio: repuve.CriterioNIV, Valor: nivPrueba,
		ConsultadoEn: ahora.Add(-time.Minute),
		Vehiculo:     &repuve.Vehiculo{Placa: placaPrueba, NIV: nivPrueba, Marca: "NISSAN", Modelo: "VERSA", AnioModelo: "2019"},
	}
}

func entradaValida() Entrada {
	return Entrada{
		Solicitud: modelo.Solicitud{SolicitudID: "6f1c2d3e-4a5b-4c6d-8e7f-001122334455", Placa: "xyz-123-a", NIV: nivPrueba, AvisoLeido: true},
		Alumno:    &modelo.Alumno{Correo: "a@uat.edu.mx", Matricula: "2210123456", NombreCompleto: "María Fernanda Pérez López", Activo: true},
		INE:       ineValida(),
		Tarjeta:   tarjetaValida(),
		REPUVE:    repuveSinReporte(),
		Ahora:     ahora,
	}
}

func tieneMotivo(d Dictamen, codigo string) bool {
	for _, m := range d.Motivos {
		if m.Codigo == codigo {
			return true
		}
	}
	return false
}

func TestApruebaCuandoTodoCoincide(t *testing.T) {
	d := Evaluar(entradaValida(), Politica{AutoAprobar: true})
	if d.Decision != modelo.DecisionAprobar || d.Estado != modelo.EstadoAprobada {
		t.Fatalf("se esperaba APROBAR, llegó %s/%s: %+v", d.Decision, d.Estado, d.Motivos)
	}
	if d.Identidad != modelo.IdentidadCoincide || d.INE != modelo.INEValidada ||
		d.Tarjeta != modelo.TarjetaValidada || d.REPUVE != modelo.REPUVESinReporte {
		t.Fatalf("verificaciones: %+v", d)
	}
	if d.NIVConsultado != nivPrueba || d.FechaREPUVE.IsZero() {
		t.Fatalf("faltan NIVConsultado o FechaREPUVE: %+v", d)
	}
}

func TestSinAutoAprobarQuedaEnRevision(t *testing.T) {
	d := Evaluar(entradaValida(), Politica{AutoAprobar: false})
	if d.Decision != modelo.DecisionEnRevision || !tieneMotivo(d, "APROBACION_MANUAL") {
		t.Fatalf("se esperaba EN_REVISION con APROBACION_MANUAL: %+v", d)
	}
}

func TestCasos(t *testing.T) {
	casos := []struct {
		nombre   string
		cambiar  func(*Entrada)
		decision string
		revisar  func(*testing.T, Dictamen)
	}{
		{"nombre distinto se devuelve", func(e *Entrada) {
			e.Alumno.NombreCompleto = "Juan Carlos Ramírez Soto"
		}, modelo.DecisionDevolver, func(t *testing.T, d Dictamen) {
			if d.Identidad != modelo.IdentidadNoCoincide || !strings.Contains(d.Indicacion, "INE") {
				t.Errorf("%+v", d)
			}
		}},
		{"nombre con un apellido de menos queda en revisión", func(e *Entrada) {
			e.Alumno.NombreCompleto = "María Pérez"
		}, modelo.DecisionEnRevision, func(t *testing.T, d Dictamen) {
			if d.Identidad != modelo.IdentidadSinConfirmar {
				t.Errorf("identidad %s", d.Identidad)
			}
		}},
		{"consulta REPUVE fechada segundos después (otro reloj)", func(e *Entrada) {
			e.REPUVE.ConsultadoEn = e.Ahora.Add(5 * time.Second)
		}, modelo.DecisionAprobar, nil},
		{"fecha de REPUVE en el futuro bloquea", func(e *Entrada) {
			e.REPUVE.ConsultadoEn = e.Ahora.Add(time.Hour)
		}, modelo.DecisionEnRevision, nil},
		{"orden y acentos distintos coinciden", func(e *Entrada) {
			e.Alumno.NombreCompleto = "PEREZ LOPEZ, maria fernanda"
		}, modelo.DecisionAprobar, nil},
		{"INE vencida según MRZ", func(e *Entrada) {
			e.INE.MRZ.FechaExpiracion = ptr("2025-12-31")
		}, modelo.DecisionDevolver, func(t *testing.T, d Dictamen) {
			if d.INE != modelo.INENoVigente {
				t.Errorf("INE %s", d.INE)
			}
		}},
		{"sin reverso no se valida la INE", func(e *Entrada) {
			e.INE.MRZ = nil
		}, modelo.DecisionEnRevision, func(t *testing.T, d Dictamen) {
			if d.INE != modelo.INESinConfirmar || !tieneMotivo(d, "SIN_MRZ") {
				t.Errorf("%+v", d)
			}
		}},
		{"MRZ con dígitos inválidos", func(e *Entrada) {
			e.INE.MRZ.Valida = false
			e.INE.MRZ.Controles.Expiracion = false
		}, modelo.DecisionEnRevision, nil},
		{"fecha de nacimiento inconsistente", func(e *Entrada) {
			e.INE.Consistencia.NacimientoCurpVsMrz = ptr(false)
		}, modelo.DecisionEnRevision, func(t *testing.T, d Dictamen) {
			if !tieneMotivo(d, "INCONSISTENCIA_NACIMIENTO") {
				t.Errorf("%+v", d.Motivos)
			}
		}},
		{"NIV de la tarjeta distinto", func(e *Entrada) {
			e.Tarjeta.NIV.Candidatos = []analizador.CandidatoNIV{{Valor: "3N1CN7AD8KL812345", Etiquetado: true}}
		}, modelo.DecisionDevolver, func(t *testing.T, d Dictamen) {
			if d.Tarjeta != modelo.TarjetaInconsistente || !strings.Contains(d.Indicacion, "tarjeta") {
				t.Errorf("%+v", d)
			}
		}},
		{"NIV leído con un carácter de diferencia", func(e *Entrada) {
			e.Tarjeta.NIV.Candidatos = []analizador.CandidatoNIV{{Valor: "1HGCM82633A004358", Etiquetado: true}}
		}, modelo.DecisionEnRevision, func(t *testing.T, d Dictamen) {
			if d.Tarjeta != modelo.TarjetaSinConfirmar || !tieneMotivo(d, "NIV_DUDOSO") {
				t.Errorf("%+v", d)
			}
		}},
		{"placa ilegible pero confirmada por REPUVE", func(e *Entrada) {
			e.Tarjeta.Placa.Candidatos = nil
		}, modelo.DecisionAprobar, nil},
		{"placa capturada distinta a la tarjeta", func(e *Entrada) {
			e.Solicitud.Placa = "ABC9876"
			e.REPUVE.Vehiculo.Placa = "XYZ123A"
		}, modelo.DecisionDevolver, nil},
		{"REPUVE con reporte de robo no se revela al alumno", func(e *Entrada) {
			e.REPUVE.Estatus = modelo.REPUVEConReporte
		}, modelo.DecisionEnRevision, func(t *testing.T, d Dictamen) {
			if d.Indicacion != modelo.IndicacionEnRevision {
				t.Errorf("indicación %q", d.Indicacion)
			}
		}},
		{"REPUVE no disponible", func(e *Entrada) {
			e.REPUVE = &repuve.Resultado{Estatus: modelo.REPUVENoDisponible, Criterio: repuve.CriterioNIV, Valor: nivPrueba, Detalle: "timeout"}
		}, modelo.DecisionEnRevision, func(t *testing.T, d Dictamen) {
			if d.REPUVE != modelo.REPUVENoDisponible || d.NIVConsultado != "" || !d.FechaREPUVE.IsZero() {
				t.Errorf("%+v", d)
			}
		}},
		{"REPUVE con placa distinta y tarjeta que confirma la solicitud", func(e *Entrada) {
			e.REPUVE.Vehiculo.Placa = "OLD1234"
		}, modelo.DecisionEnRevision, func(t *testing.T, d Dictamen) {
			if d.REPUVE != modelo.REPUVESinReporte || !tieneMotivo(d, "REPUVE_PLACA_DISTINTA") {
				t.Errorf("%+v", d)
			}
		}},
		{"marca distinta en REPUVE bloquea", func(e *Entrada) {
			e.REPUVE.Vehiculo.Marca = "TOYOTA"
		}, modelo.DecisionEnRevision, nil},
		{"alias de marca VW", func(e *Entrada) {
			e.Tarjeta.Marca = ptr("VOLKSWAGEN")
			e.REPUVE.Vehiculo.Marca = "VW"
		}, modelo.DecisionAprobar, nil},
		{"vehículo ya aprobado para otro alumno", func(e *Entrada) {
			e.Duplicadas = []modelo.Solicitud{{SolicitudID: "otra"}}
		}, modelo.DecisionEnRevision, nil},
		{"alumno inactivo", func(e *Entrada) {
			e.Alumno.Activo = false
		}, modelo.DecisionEnRevision, nil},
		{"NIV con formato inválido", func(e *Entrada) {
			e.Solicitud.NIV = "123"
		}, modelo.DecisionDevolver, nil},
		{"foto de INE rechazada por el analizador", func(e *Entrada) {
			e.INE, e.ErrINE = nil, &analizador.ErrImagen{Codigo: "IMAGEN_INVALIDA", Mensaje: "no es imagen"}
		}, modelo.DecisionDevolver, func(t *testing.T, d Dictamen) {
			if !strings.Contains(d.Indicacion, "fotos") {
				t.Errorf("indicación %q", d.Indicacion)
			}
		}},
		{"falla de red del analizador no se publica como problema del alumno", func(e *Entrada) {
			e.INE, e.ErrINE = nil, errors.New("connection refused")
		}, modelo.DecisionEnRevision, nil},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			e := entradaValida()
			c.cambiar(&e)
			d := Evaluar(e, Politica{AutoAprobar: true})
			if d.Decision != c.decision {
				t.Fatalf("decisión %s, se esperaba %s; motivos: %+v", d.Decision, c.decision, d.Motivos)
			}
			if c.revisar != nil {
				c.revisar(t, d)
			}
		})
	}
}

func TestCobertura(t *testing.T) {
	casos := []struct {
		ref, doc       string
		minRef, minDoc float64
		maxRef         float64
	}{
		{"María Fernanda Pérez López", "PEREZ LOPEZ MARIA FERNANDA", 1, 1, 1},
		{"José de la Cruz Núñez", "CRUZ NUNEZ JOSE", 1, 1, 1},
		{"MARIA FERNANDA PEREZ LOPEZ", "PEREZ LOPEZ MARIA FERNAND", 0.95, 0.95, 1}, // MRZ truncada
		{"MARIA FERNANDA PEREZ LOPEZ", "PERFZ LOPEZ MARIA FERNANDA", 0.9, 0.9, 1},  // una letra mal leída
		{"JUAN RAMIREZ SOTO", "PEREZ LOPEZ MARIA", 0, 0, 0.1},
	}
	for _, c := range casos {
		ref, doc := Cobertura(c.ref, c.doc)
		if ref < c.minRef || doc < c.minDoc || ref > c.maxRef {
			t.Errorf("Cobertura(%q, %q) = %.2f, %.2f", c.ref, c.doc, ref, doc)
		}
	}
}
