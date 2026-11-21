package servicio

import "sync"

// historial guarda en memoria los últimos informes, para consultar el estado
// de una validación sin volver a leer AppSheet. Se pierde al reiniciar.
type historial struct {
	mu    sync.Mutex
	max   int
	orden []string
	datos map[string]Informe
}

func nuevoHistorial(max int) *historial {
	return &historial{max: max, datos: map[string]Informe{}}
}

func (h *historial) guardar(i Informe) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.datos[i.SolicitudID]; !ok {
		h.orden = append(h.orden, i.SolicitudID)
		if len(h.orden) > h.max {
			delete(h.datos, h.orden[0])
			h.orden = h.orden[1:]
		}
	}
	h.datos[i.SolicitudID] = i
}

func (h *historial) obtener(id string) (Informe, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	i, ok := h.datos[id]
	return i, ok
}
