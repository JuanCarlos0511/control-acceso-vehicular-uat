// Cliente del backend en Go (appsheet-backend). No usa el contrato de Apps
// Script: esta app vive fuera de HtmlService y habla por HTTPS.

export interface ResultadoRegistro {
  correo: string;
  matricula: string;
  nombre: string;
  programa: string;
  situacion: string;
  activo: boolean;
  guardado: boolean;
  nuevo: boolean;
  appUrl?: string;
  mensaje: string;
}

export interface ErrorRegistro {
  codigo: string;
  mensaje: string;
}

type Sobre = { ok: true; datos: ResultadoRegistro } | { ok: false; error: ErrorRegistro };

const BACKEND = (import.meta.env.VITE_BACKEND_URL ?? '').replace(/\/+$/, '');

export async function verificarAlumno(usuario: string, contrasena: string): Promise<Sobre> {
  try {
    const r = await fetch(`${BACKEND}/api/v1/alumnos/verificar`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ usuario, contrasena }),
      credentials: 'omit',
      cache: 'no-store',
    });
    const sobre = (await r.json().catch(() => null)) as Sobre | null;
    if (sobre && typeof sobre.ok === 'boolean') return sobre;
    return { ok: false, error: { codigo: 'ERROR_INTERNO', mensaje: `El servidor respondió de forma inesperada (HTTP ${r.status}).` } };
  } catch {
    return { ok: false, error: { codigo: 'RED', mensaje: 'No hay conexión con el servidor. Revisa tu internet e intenta de nuevo.' } };
  }
}
