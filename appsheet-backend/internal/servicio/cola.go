package servicio

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// Cola procesa solicitudes en segundo plano con un número fijo de
// trabajadores. Ignora un folio que ya está en la cola o en proceso.
type Cola struct {
	s          *Servicio
	log        *slog.Logger
	trabajos   chan trabajo
	mu         sync.Mutex
	pendientes map[string]bool
	wg         sync.WaitGroup
	limite     time.Duration
}

type trabajo struct {
	id, origen string
}

func NuevaCola(s *Servicio, log *slog.Logger) *Cola {
	return &Cola{
		s: s, log: log, trabajos: make(chan trabajo, 1024),
		pendientes: map[string]bool{}, limite: 6 * time.Minute,
	}
}

// Iniciar arranca los trabajadores; terminan cuando ctx se cancela.
func (c *Cola) Iniciar(ctx context.Context, trabajadores int) {
	for range trabajadores {
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case t := <-c.trabajos:
					c.procesar(ctx, t)
				}
			}
		}()
	}
}

func (c *Cola) procesar(ctx context.Context, t trabajo) {
	defer func() {
		c.mu.Lock()
		delete(c.pendientes, t.id)
		c.mu.Unlock()
		if r := recover(); r != nil {
			c.log.Error("pánico al validar", "solicitud", t.id, "panico", r)
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, c.limite)
	defer cancel()
	if _, err := c.s.Validar(ctx, t.id, t.origen); err != nil && !errors.Is(err, ErrEnCurso) {
		c.log.Error("validación fallida; se reintentará en el siguiente barrido", "solicitud", t.id, "origen", t.origen, "error", err)
	}
}

// Encolar agrega un folio. Devuelve false si ya estaba en la cola o si la cola está llena.
func (c *Cola) Encolar(id, origen string) bool {
	c.mu.Lock()
	if c.pendientes[id] {
		c.mu.Unlock()
		return false
	}
	c.pendientes[id] = true
	c.mu.Unlock()
	select {
	case c.trabajos <- trabajo{id, origen}:
		return true
	default:
		c.mu.Lock()
		delete(c.pendientes, id)
		c.mu.Unlock()
		c.log.Warn("cola llena; la solicitud se tomará en el siguiente barrido", "solicitud", id)
		return false
	}
}

// Esperar bloquea hasta que terminen los trabajadores.
func (c *Cola) Esperar() { c.wg.Wait() }

// Barrer encola periódicamente las solicitudes pendientes, por si se perdió
// algún webhook o el servicio estuvo apagado.
func (c *Cola) Barrer(ctx context.Context, intervalo time.Duration, dias int) {
	if intervalo <= 0 {
		return
	}
	ejecutar := func() {
		bctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		ids, err := c.s.Pendientes(bctx, dias)
		if err != nil {
			c.log.Error("barrido de pendientes fallido", "error", err)
			return
		}
		encoladas := 0
		for _, id := range ids {
			if c.Encolar(id, "barrido") {
				encoladas++
			}
		}
		if encoladas > 0 {
			c.log.Info("barrido de pendientes", "encoladas", encoladas)
		}
		if emitidos, err := c.s.EmitirPasesPendientes(bctx); err != nil {
			c.log.Error("barrido de pases fallido", "error", err)
		} else if emitidos > 0 {
			c.log.Info("pases emitidos en el barrido", "emitidos", emitidos)
		}
	}
	// Primer barrido poco después de arrancar.
	select {
	case <-ctx.Done():
		return
	case <-time.After(15 * time.Second):
	}
	ejecutar()
	t := time.NewTicker(intervalo)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			ejecutar()
		}
	}
}
