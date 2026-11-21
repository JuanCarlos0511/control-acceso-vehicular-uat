import { useCallback, useEffect, useRef, useState } from 'react';
import type { Accion, DatosDe, ErrorApi, Respuesta, RespuestaDe } from './contrato';

interface ScriptRun {
  withSuccessHandler(fn: (r: unknown) => void): ScriptRun;
  withFailureHandler(fn: (e: Error) => void): ScriptRun;
  api(solicitud: { accion: string; datos: unknown }): void;
}

declare global {
  interface Window {
    google?: { script?: { run: ScriptRun } };
  }
}

const ERROR_RED: ErrorApi = {
  codigo: 'RED',
  mensaje: 'No hay conexión con el servidor. Intenta de nuevo.',
};

const ERROR_RESPUESTA_VACIA: ErrorApi = {
  codigo: 'ERROR_INTERNO',
  mensaje: 'El servidor respondió vacío. Revisa que no devuelva objetos Date.',
};

/**
 * Única forma de llamar al backend (contrato §1). Nunca rechaza: siempre resuelve un sobre.
 * Dentro de Apps Script usa google.script.run; en `npm run dev` usa el mock del contrato.
 */
export async function llamarApi<A extends Accion>(
  accion: A,
  datos: DatosDe<A>,
): Promise<Respuesta<RespuestaDe<A>>> {
  const run = window.google?.script?.run;
  if (run) {
    return new Promise((resolve) => {
      run
        .withSuccessHandler((r) =>
          resolve((r as Respuesta<RespuestaDe<A>> | null) ?? { ok: false, error: ERROR_RESPUESTA_VACIA }),
        )
        .withFailureHandler(() => resolve({ ok: false, error: ERROR_RED }))
        .api({ accion, datos });
    });
  }
  // VITE_DEMO=true incluye el mock en la compilación para publicar una demostración fuera de Apps Script.
  if (import.meta.env.DEV || import.meta.env.VITE_DEMO === 'true') {
    const { mockApi } = await import('./mock');
    return mockApi(accion, datos);
  }
  return { ok: false, error: ERROR_RED };
}

interface EstadoConsulta<T> {
  datos: T | null;
  error: ErrorApi | null;
  cargando: boolean;
}

/** Ejecuta una acción de lectura al montar y cada vez que cambian sus datos. */
export function useConsulta<A extends Accion>(accion: A, datos: DatosDe<A>, activa = true) {
  const [estado, setEstado] = useState<EstadoConsulta<RespuestaDe<A>>>({
    datos: null,
    error: null,
    cargando: activa,
  });
  const clave = JSON.stringify(datos);
  const turno = useRef(0);

  const recargar = useCallback(async () => {
    const mio = ++turno.current;
    setEstado((e) => ({ ...e, cargando: true }));
    const r = await llamarApi(accion, JSON.parse(clave) as DatosDe<A>);
    if (mio !== turno.current) return;
    setEstado(r.ok ? { datos: r.datos, error: null, cargando: false } : { datos: null, error: r.error, cargando: false });
  }, [accion, clave]);

  useEffect(() => {
    if (activa) void recargar();
  }, [recargar, activa]);

  return { ...estado, recargar };
}
