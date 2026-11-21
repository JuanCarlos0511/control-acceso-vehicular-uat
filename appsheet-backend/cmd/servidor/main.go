// Comando servidor: backend del registro vehicular estudiantil en AppSheet.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // zonas horarias dentro del binario (la imagen distroless no las trae)

	"registrovehicular/internal/analizador"
	"registrovehicular/internal/appsheet"
	"registrovehicular/internal/config"
	"registrovehicular/internal/imagenes"
	"registrovehicular/internal/pase"
	"registrovehicular/internal/registro"
	"registrovehicular/internal/repuve"
	"registrovehicular/internal/servicio"
	"registrovehicular/internal/servidor"
	"registrovehicular/internal/uat"
	"registrovehicular/internal/validacion"
)

func main() {
	if err := ejecutar(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func ejecutar() error {
	cfg, err := config.Cargar()
	if err != nil {
		return fmt.Errorf("configuración inválida:\n%w", err)
	}
	log := nuevoLog(cfg.LogNivel)
	slog.SetDefault(log)

	ctx, detener := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer detener()

	cliente := appsheet.NuevoCliente(appsheet.Opciones{
		BaseURL:   cfg.AppSheet.BaseURL,
		AppID:     cfg.AppSheet.AppID,
		AccessKey: cfg.AppSheet.AccessKey,
		RunAs:     cfg.AppSheet.RunAs,
		Locale:    cfg.AppSheet.Locale,
		Timezone:  cfg.AppSheet.Timezone,
	})
	repo := appsheet.NuevoRepositorio(cliente, cfg.AppSheet.Tablas, cfg.ZonaHoraria, cfg.AppSheet.Locale)
	var sheets *appsheet.ClienteSheets
	if cfg.GoogleSheetsID != "" {
		sheets, err = appsheet.NuevoClienteSheets(ctx, cfg.GoogleSheetsID, cfg.AppSheet.Tablas, cfg.ZonaHoraria)
		if err != nil {
			return err
		}
		repo = appsheet.NuevoRepositorioSheets(sheets, cfg.AppSheet.Tablas, cfg.ZonaHoraria, cfg.AppSheet.Locale)
	}

	fuente, err := fuenteImagenes(ctx, cfg)
	if err != nil {
		return err
	}
	anal := analizador.NuevoCliente(cfg.AnalizadorURL, cfg.AnalizadorToken, nil)

	var consultor repuve.Consultor
	if cfg.REPUVE.Habilitado {
		consultor = repuve.NuevoCliente(cfg.AnalizadorURL, cfg.AnalizadorToken, cfg.REPUVE.EsperaMax)
	}

	svc := servicio.Nuevo(repo, fuente, anal, consultor, servicio.Opciones{
		TablaSolicitudes: cfg.AppSheet.Tablas.Solicitudes,
		CorreoSistema:    cfg.CorreoSistema,
		Politica:         validacion.Politica{AutoAprobar: cfg.AutoAprobar},
		Zona:             cfg.ZonaHoraria,
		Log:              log.With("componente", "servicio"),
	})

	if cfg.Pases.Habilitado {
		svc.HabilitarPases(repo, servicio.OpcionesPases{
			Firmador:   pase.NuevoFirmador(cfg.Pases.Secreto, cfg.ZonaHoraria),
			URLPublica: cfg.Pases.URLPublica,
			Dias:       cfg.Pases.Dias,
			VenceFijo:  cfg.Pases.VenceFijo,
		})
	}

	cola := servicio.NuevaCola(svc, log.With("componente", "cola"))
	cola.Iniciar(ctx, cfg.Trabajadores)
	if cfg.AppSheet.Habilitado || sheets != nil {
		go cola.Barrer(ctx, cfg.BarridoIntervalo, cfg.BarridoDias)
	} else {
		log.Warn("Sin origen de datos para las solicitudes: configura Google Sheets o la API de AppSheet")
	}

	var registrador servidor.Registrador
	var clienteUAT *uat.Cliente
	if cfg.UAT.Habilitado {
		clienteUAT = uat.NuevoCliente(cfg.UAT.URL, cfg.UAT.Clave, cfg.UAT.Espera)
		var alumnos registro.Almacen
		if cfg.AppSheet.Habilitado || sheets != nil {
			alumnos = repo
		}
		registrador = registro.Nuevo(clienteUAT, alumnos, registro.Opciones{
			Dominio:            cfg.UAT.Dominio,
			SituacionesActivas: cfg.UAT.SituacionesActivas,
			AppURL:             cfg.UAT.AppURL,
			Log:                log.With("componente", "registro"),
		})
	}

	api := servidor.Nuevo(svc, cola, servidor.Opciones{
		WebhookSecreto: cfg.WebhookSecreto,
		APIToken:       cfg.APIToken,
		Log:            log.With("componente", "http"),
		Registro:       registrador,
		CORSOrigenes:   cfg.CORSOrigenes,
		Salud: func(ctx context.Context) map[string]string {
			estado := map[string]string{"analizador": "ok", "repuve": "deshabilitado", "pases": "deshabilitado",
				"appsheet": "deshabilitado", "uat": "deshabilitado"}
			if cfg.Pases.Habilitado {
				estado["pases"] = "ok"
			}
			if cfg.AppSheet.Habilitado {
				estado["appsheet"] = "ok"
			}
			if sheets != nil {
				estado["sheets"] = "ok"
				if err := sheets.Salud(ctx, cfg.AppSheet.Tablas.Solicitudes); err != nil {
					estado["sheets"] = "sin respuesta o sin acceso"
				}
			}
			if clienteUAT != nil {
				estado["uat"] = "ok"
				if err := clienteUAT.Salud(ctx); err != nil {
					estado["uat"] = "sin respuesta"
				}
			}
			if err := anal.Salud(ctx); err != nil {
				estado["analizador"] = "sin respuesta"
			}
			if consultor != nil {
				estado["repuve"] = "ok"
			}
			return estado
		},
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Puerto,
		Handler:           api.Rutas(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       2 * time.Minute,
		WriteTimeout:      6 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}
	errSrv := make(chan error, 1)
	go func() {
		log.Info("servidor escuchando", "puerto", cfg.Puerto, "version", servicio.Version,
			"repuve", cfg.REPUVE.Habilitado, "pases", cfg.Pases.Habilitado, "autoAprobar", cfg.AutoAprobar, "imagenes", cfg.Imagenes.Fuente,
			"appsheet", cfg.AppSheet.Habilitado, "sheets", sheets != nil, "registroUAT", cfg.UAT.Habilitado)
		errSrv <- srv.ListenAndServe()
	}()

	select {
	case err := <-errSrv:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	log.Info("apagando")
	apagar, cancelar := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelar()
	_ = srv.Shutdown(apagar)
	cola.Esperar()
	return nil
}

func fuenteImagenes(ctx context.Context, cfg config.Config) (imagenes.Fuente, error) {
	var archivos imagenes.Fuente
	switch cfg.Imagenes.Fuente {
	case "drive":
		if cfg.Imagenes.DriveCarpetaID == "" { // solo sin AppSheet: no hay fotos que leer
			archivos = sinFotos{}
			break
		}
		d, err := imagenes.NuevoDrive(ctx, cfg.Imagenes.DriveCarpetaID)
		if err != nil {
			return nil, err
		}
		archivos = d
	case "local":
		archivos = imagenes.Local{Dir: cfg.Imagenes.DirLocal}
	case "appsheet":
		archivos = imagenes.URLAppSheet{Base: "https://www.appsheet.com", AppName: cfg.Imagenes.AppName}
	}
	if cfg.GoogleSheetsID != "" {
		return archivos, nil
	}
	return imagenes.Enrutador{Archivos: archivos, URLs: imagenes.PorURL{}}, nil
}

type sinFotos struct{}

func (sinFotos) Obtener(context.Context, string, string) ([]byte, error) {
	return nil, errors.New("fotos sin configurar: falta DRIVE_CARPETA_ID")
}

func nuevoLog(nivel string) *slog.Logger {
	var l slog.Level
	if err := l.UnmarshalText([]byte(nivel)); err != nil {
		l = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l}))
}
