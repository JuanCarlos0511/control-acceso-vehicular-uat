// Package pase emite y verifica el código del pase vehicular que se muestra
// como QR en AppSheet cuando una solicitud se aprueba.
//
// Formato del código (lo que va dentro del QR):
//
//	RV1.<PaseId>.<PLACA>.<AAAAMMDD>.<firma>
//
// La firma es HMAC-SHA256 con PASE_SECRETO sobre "RV1.<PaseId>.<PLACA>.<AAAAMMDD>",
// truncada a 16 bytes y codificada en base64url sin relleno. El código no lleva
// nombre, CURP ni correo: la caseta compara la placa del QR con la del vehículo,
// así que una captura de pantalla no sirve en otro coche.
package pase

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"registrovehicular/internal/modelo"
)

const (
	Prefijo     = "RV1"
	bytesFirma  = 16
	formatoDia  = "20060102"
	MaxLongitud = 128
)

var (
	ErrFormato = errors.New("el código no tiene el formato de un pase")
	ErrFirma   = errors.New("la firma del pase no es válida")
)

// Datos es lo que contiene un código válido.
type Datos struct {
	PaseID string
	Placa  string
	Vence  time.Time // último instante del día de vencimiento, en la zona del campus
}

type Firmador struct {
	secreto []byte
	zona    *time.Location
}

func NuevoFirmador(secreto string, zona *time.Location) *Firmador {
	if zona == nil {
		zona = time.UTC
	}
	return &Firmador{secreto: []byte(secreto), zona: zona}
}

func (f *Firmador) firma(cuerpo string) string {
	m := hmac.New(sha256.New, f.secreto)
	m.Write([]byte(cuerpo))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil)[:bytesFirma])
}

// Codigo arma el código firmado de un pase.
func (f *Firmador) Codigo(paseID, placa string, vence time.Time) string {
	cuerpo := strings.Join([]string{Prefijo, paseID, placa, vence.In(f.zona).Format(formatoDia)}, ".")
	return cuerpo + "." + f.firma(cuerpo)
}

// Verificar revisa el formato y la firma. No revisa la vigencia ni si el pase
// fue revocado: eso lo hace el servicio, que conoce la fecha y la tabla Pases.
func (f *Firmador) Verificar(codigo string) (Datos, error) {
	codigo = strings.TrimSpace(codigo)
	if len(codigo) > MaxLongitud {
		return Datos{}, ErrFormato
	}
	partes := strings.Split(codigo, ".")
	if len(partes) != 5 || partes[0] != Prefijo || !modelo.FolioValido(partes[1]) || !modelo.PlacaValida(partes[2]) {
		return Datos{}, ErrFormato
	}
	dia, err := time.ParseInLocation(formatoDia, partes[3], f.zona)
	if err != nil {
		return Datos{}, ErrFormato
	}
	cuerpo := strings.Join(partes[:4], ".")
	if !hmac.Equal([]byte(f.firma(cuerpo)), []byte(partes[4])) {
		return Datos{}, ErrFirma
	}
	return Datos{PaseID: partes[1], Placa: partes[2], Vence: dia.Add(24*time.Hour - time.Second)}, nil
}

// PNG dibuja el código como QR. Usa corrección de errores media, suficiente
// para leerlo desde la pantalla de un celular.
func PNG(codigo string, tamano int) ([]byte, error) {
	q, err := qrcode.New(codigo, qrcode.Medium)
	if err != nil {
		return nil, err
	}
	return q.PNG(tamano)
}
