// Package analizador es el cliente del servicio de análisis de documentos
// (analizador/, en Python): OCR, MRZ, CURP, NIV y calidad de imagen.
package analizador

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

type Calidad struct {
	Ancho           int      `json:"ancho"`
	Alto            int      `json:"alto"`
	Nitidez         float64  `json:"nitidez"`
	Brillo          float64  `json:"brillo"`
	RostroDetectado *bool    `json:"rostroDetectado"`
	Aceptable       bool     `json:"aceptable"`
	Problemas       []string `json:"problemas"`
}

type Nombre struct {
	ApellidoPaterno *string `json:"apellidoPaterno"`
	ApellidoMaterno *string `json:"apellidoMaterno"`
	Nombres         *string `json:"nombres"`
	Completo        string  `json:"completo"`
	Confianza       float64 `json:"confianza"`
}

type CURP struct {
	Valor           string  `json:"valor"`
	FormatoValido   bool    `json:"formatoValido"`
	DigitoValido    bool    `json:"digitoValido"`
	FechaNacimiento *string `json:"fechaNacimiento"`
	Sexo            *string `json:"sexo"`
}

type Vigencia struct {
	Anio      int     `json:"anio"`
	Confianza float64 `json:"confianza"`
}

type ControlesMRZ struct {
	Documento  bool `json:"documento"`
	Nacimiento bool `json:"nacimiento"`
	Expiracion bool `json:"expiracion"`
	Compuesto  bool `json:"compuesto"`
}

type MRZ struct {
	Valida          bool         `json:"valida"`
	Controles       ControlesMRZ `json:"controles"`
	FechaNacimiento *string      `json:"fechaNacimiento"`
	FechaExpiracion *string      `json:"fechaExpiracion"`
	Sexo            *string      `json:"sexo"`
	Apellidos       *string      `json:"apellidos"`
	Nombres         *string      `json:"nombres"`
}

type Consistencia struct {
	NacimientoCurpVsMrz    *bool    `json:"nacimientoCurpVsMrz"`
	NacimientoCurpVsFrente *bool    `json:"nacimientoCurpVsFrente"`
	NombreFrenteVsMrz      *float64 `json:"nombreFrenteVsMrz"`
	VigenciaFrenteVsMrz    *bool    `json:"vigenciaFrenteVsMrz"`
}

type INE struct {
	EsINE              bool      `json:"esINE"`
	ConfianzaDocumento float64   `json:"confianzaDocumento"`
	Nombre             *Nombre   `json:"nombre"`
	CURP               *CURP     `json:"curp"`
	FechaNacimiento    *string   `json:"fechaNacimiento"`
	Sexo               *string   `json:"sexo"`
	Vigencia           *Vigencia `json:"vigencia"`
	MRZ                *MRZ      `json:"mrz"`
	QR                 struct {
		Detectados int `json:"detectados"`
		URLsINE    int `json:"urlsINE"`
	} `json:"qr"`
	Consistencia Consistencia `json:"consistencia"`
	Advertencias []string     `json:"advertencias"`
	Calidad      struct {
		Frente  *Calidad `json:"frente"`
		Reverso *Calidad `json:"reverso"`
	} `json:"calidad"`
}

type CandidatoNIV struct {
	Valor        string  `json:"valor"`
	Etiquetado   bool    `json:"etiquetado"`
	DigitoValido bool    `json:"digitoValido"`
	Confianza    float64 `json:"confianza"`
}

type CandidatoPlaca struct {
	Valor      string  `json:"valor"`
	Etiquetado bool    `json:"etiquetado"`
	Confianza  float64 `json:"confianza"`
}

type Tarjeta struct {
	EsTarjeta          bool    `json:"esTarjeta"`
	ConfianzaDocumento float64 `json:"confianzaDocumento"`
	NIV                struct {
		Valor      *string        `json:"valor"`
		Candidatos []CandidatoNIV `json:"candidatos"`
	} `json:"niv"`
	Placa struct {
		Valor      *string          `json:"valor"`
		Candidatos []CandidatoPlaca `json:"candidatos"`
	} `json:"placa"`
	Marca      *string `json:"marca"`
	AnioModelo *int    `json:"anioModelo"`
	Vigencia   struct {
		Anio       *int `json:"anio"`
		Indefinida bool `json:"indefinida"`
	} `json:"vigencia"`
	Advertencias []string `json:"advertencias"`
	Calidad      *Calidad `json:"calidad"`
}

// ErrImagen indica que el analizador rechazó la imagen (no es un problema de
// infraestructura: la foto no se pudo decodificar o es demasiado grande).
type ErrImagen struct {
	Codigo  string
	Mensaje string
}

func (e *ErrImagen) Error() string { return "analizador: " + e.Codigo + ": " + e.Mensaje }

type Cliente struct {
	base  string
	token string
	http  *http.Client
}

func NuevoCliente(base, token string, h *http.Client) *Cliente {
	if h == nil {
		h = &http.Client{Timeout: 90 * time.Second}
	}
	return &Cliente{base: base, token: token, http: h}
}

type archivo struct {
	campo string
	datos []byte
}

func (c *Cliente) AnalizarINE(ctx context.Context, frente, reverso []byte) (*INE, error) {
	archivos := []archivo{{"frente", frente}}
	if len(reverso) > 0 {
		archivos = append(archivos, archivo{"reverso", reverso})
	}
	var r INE
	return &r, c.enviar(ctx, "/v1/ine", archivos, &r)
}

func (c *Cliente) AnalizarTarjeta(ctx context.Context, imagen []byte) (*Tarjeta, error) {
	var r Tarjeta
	return &r, c.enviar(ctx, "/v1/tarjeta", []archivo{{"imagen", imagen}}, &r)
}

func (c *Cliente) Salud(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/salud", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("analizador: HTTP %d", resp.StatusCode)
	}
	return nil
}

func (c *Cliente) enviar(ctx context.Context, ruta string, archivos []archivo, destino any) error {
	var cuerpo bytes.Buffer
	w := multipart.NewWriter(&cuerpo)
	for _, a := range archivos {
		parte, err := w.CreateFormFile(a.campo, a.campo+".jpg")
		if err != nil {
			return err
		}
		if _, err := parte.Write(a.datos); err != nil {
			return err
		}
	}
	if err := w.Close(); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+ruta, &cuerpo)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	if c.token != "" {
		req.Header.Set("X-Token-Analizador", c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("analizador: %w", err)
	}
	defer resp.Body.Close()
	datos, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusRequestEntityTooLarge || resp.StatusCode == http.StatusUnprocessableEntity {
		var e struct {
			Error struct {
				Codigo  string `json:"codigo"`
				Mensaje string `json:"mensaje"`
			} `json:"error"`
		}
		_ = json.Unmarshal(datos, &e)
		return &ErrImagen{Codigo: e.Error.Codigo, Mensaje: e.Error.Mensaje}
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("analizador: HTTP %d", resp.StatusCode)
	}
	if err := json.Unmarshal(datos, destino); err != nil {
		return fmt.Errorf("analizador: respuesta no reconocida: %w", err)
	}
	return nil
}
