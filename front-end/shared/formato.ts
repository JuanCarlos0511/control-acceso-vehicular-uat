import type { CodigoAdvertencia, Rol, TipoMovimiento, TipoPersona } from './contrato';

export const ZONA_CAMPUS = 'America/Monterrey';

/** Igual que el servidor: mayúsculas, sin espacios, guiones ni puntos. */
export function normalizarPlaca(texto: string): string {
  return texto.toUpperCase().replace(/[^A-Z0-9]/g, '');
}

export function placaValida(placa: string): boolean {
  return /^[A-Z0-9]{4,10}$/.test(placa);
}

const fmtFecha = new Intl.DateTimeFormat('en-CA', { timeZone: ZONA_CAMPUS });
const fmtHora = new Intl.DateTimeFormat('es-MX', { timeZone: ZONA_CAMPUS, hour: '2-digit', minute: '2-digit', hourCycle: 'h23' });
const fmtHoraSeg = new Intl.DateTimeFormat('es-MX', {
  timeZone: ZONA_CAMPUS,
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hourCycle: 'h23',
});
const fmtDiaMes = new Intl.DateTimeFormat('es-MX', { timeZone: ZONA_CAMPUS, day: '2-digit', month: '2-digit' });
const fmtLargo = new Intl.DateTimeFormat('es-MX', {
  timeZone: ZONA_CAMPUS,
  weekday: 'long',
  day: 'numeric',
  month: 'long',
  year: 'numeric',
});

/** Fecha de hoy en el campus, "YYYY-MM-DD". */
export function hoy(): string {
  return fmtFecha.format(new Date());
}

/** Suma días a una fecha "YYYY-MM-DD". */
export function sumarDias(fecha: string, dias: number): string {
  const d = new Date(`${fecha}T12:00:00-06:00`);
  d.setUTCDate(d.getUTCDate() + dias);
  return fmtFecha.format(d);
}

export const hora = (iso: string) => fmtHora.format(new Date(iso));
export const horaConSegundos = (iso: string) => fmtHoraSeg.format(new Date(iso));
export const fechaHoraCorta = (iso: string) => `${fmtDiaMes.format(new Date(iso))} ${hora(iso)}`;

export function fechaLarga(fecha = new Date()): string {
  const texto = fmtLargo.format(fecha);
  return texto.charAt(0).toUpperCase() + texto.slice(1);
}

/** "hoy a las 07:45", "ayer a las 16:10" o "el 18/11 a las 09:02". */
export function cuando(iso: string): string {
  const dia = fmtFecha.format(new Date(iso));
  const h = hora(iso);
  if (dia === hoy()) return `hoy a las ${h}`;
  if (dia === sumarDias(hoy(), -1)) return `ayer a las ${h}`;
  return `el ${fmtDiaMes.format(new Date(iso))} a las ${h}`;
}

export function duracion(minutos: number): string {
  const h = Math.floor(minutos / 60);
  const m = minutos % 60;
  return h ? `${h} h ${String(m).padStart(2, '0')} min` : `${m} min`;
}

export const numero = (n: number) => n.toLocaleString('es-MX');

/** "1 entrada", "3 entradas". */
export const plural = (n: number, singular: string, varios = `${singular}s`) => `${numero(n)} ${n === 1 ? singular : varios}`;

export function iniciales(nombre: string): string {
  return nombre
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((p) => p[0]!.toUpperCase())
    .join('');
}

export const ETIQUETA_TIPO_PERSONA: Record<TipoPersona, string> = {
  ESTUDIANTE: 'Estudiante',
  DOCENTE: 'Docente',
  ADMINISTRATIVO: 'Administrativo',
  VISITANTE: 'Visitante',
};

export const ETIQUETA_MOVIMIENTO: Record<TipoMovimiento, string> = {
  ENTRADA: 'Entrada',
  SALIDA: 'Salida',
};

export const ETIQUETA_ROL: Record<Rol, string> = {
  ADMIN: 'Administrador',
  SUPERVISOR: 'Supervisor',
  GUARDIA: 'Guardia',
};

export const ETIQUETA_ADVERTENCIA: Record<CodigoAdvertencia, string> = {
  VEHICULO_NO_REGISTRADO: 'No registrada',
  VEHICULO_NO_AUTORIZADO: 'No autorizada',
  ENTRADA_DUPLICADA: 'Entrada repetida',
  SALIDA_SIN_ENTRADA: 'Salida sin entrada',
};

const ETIQUETA_ACCION: Record<string, string> = {
  ACCESO: 'Abrió la aplicación',
  'movimientos.registrar': 'Registró un movimiento',
  'bitacora.registrarExportacion': 'Exportó datos',
  'casetas.crear': 'Agregó una caseta',
  'casetas.actualizar': 'Editó una caseta',
  'personas.crear': 'Agregó una persona',
  'personas.actualizar': 'Editó una persona',
  'vehiculos.crear': 'Agregó un vehículo',
  'vehiculos.actualizar': 'Editó un vehículo',
  'usuarios.crear': 'Agregó un usuario',
  'usuarios.actualizar': 'Editó un usuario',
};

/** Nombre legible de una acción del contrato para la bitácora. */
export function etiquetaAccion(accion: string): string {
  return ETIQUETA_ACCION[accion] ?? `Intentó ${accion}`;
}
