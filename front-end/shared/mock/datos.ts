// Datos ficticios del mock. Se generan igual en cada recarga (semilla fija).
import type { Caseta, EntradaBitacora, Movimiento, Persona, TipoPersona, Usuario } from '../contrato';
import { sumarDias, ZONA_CAMPUS } from '../formato';

export const DESFASE = '-06:00';

const partes = new Intl.DateTimeFormat('en-CA', {
  timeZone: ZONA_CAMPUS,
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hourCycle: 'h23',
});

export function aFechaHora(ms: number): string {
  const p = Object.fromEntries(partes.formatToParts(new Date(ms)).map((x) => [x.type, x.value]));
  return `${p.year}-${p.month}-${p.day}T${p.hour}:${p.minute}:${p.second}${DESFASE}`;
}

export const fechaDe = (ms: number) => aFechaHora(ms).slice(0, 10);
export const horaDe = (ms: number) => Number(aFechaHora(ms).slice(11, 13));
export const inicioDia = (fecha: string) => Date.parse(`${fecha}T00:00:00${DESFASE}`);

/** 1 = lunes … 7 = domingo. */
export function diaSemana(fecha: string): number {
  const d = new Date(`${fecha}T12:00:00${DESFASE}`).getUTCDay();
  return d === 0 ? 7 : d;
}

function generador(semilla: number) {
  return () => {
    semilla = (semilla + 0x6d2b79f5) | 0;
    let t = Math.imul(semilla ^ (semilla >>> 15), 1 | semilla);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

const azar = generador(20261120);
const elegir = <T>(xs: readonly T[]): T => xs[Math.floor(azar() * xs.length)]!;

export interface MovimientoInterno extends Movimiento {
  ts: number;
}

export interface VehiculoInterno {
  placa: string;
  personaId: string;
  marca: string | null;
  modelo: string | null;
  color: string | null;
  autorizado: boolean;
  creadoEn: string;
}

const ALTA = '2026-08-15T09:00:00-06:00';

export const casetas: Caseta[] = [
  { id: 'cas-1', nombre: 'Caseta 1', ubicacion: 'Acceso principal', activa: true },
  { id: 'cas-2', nombre: 'Caseta 2', ubicacion: 'Acceso norte', activa: true },
  { id: 'cas-3', nombre: 'Caseta 3', ubicacion: 'Acceso sur', activa: true },
];

export const usuarios: Usuario[] = [
  { correo: 'ana.torres@uat.edu.mx', nombre: 'Ana Torres', rol: 'ADMIN', casetaId: null, activo: true },
  { correo: 'seguridad.cutm@uat.edu.mx', nombre: 'Marco Díaz', rol: 'SUPERVISOR', casetaId: null, activo: true },
  { correo: 'guardia1@uat.edu.mx', nombre: 'Juan Pérez', rol: 'GUARDIA', casetaId: 'cas-1', activo: true },
  { correo: 'guardia2@uat.edu.mx', nombre: 'Pedro Gómez', rol: 'GUARDIA', casetaId: 'cas-2', activo: true },
  { correo: 'guardia3@uat.edu.mx', nombre: 'Luisa Castro', rol: 'GUARDIA', casetaId: 'cas-3', activo: true },
  { correo: 'guardia.relevo@uat.edu.mx', nombre: 'Iván Rosas', rol: 'GUARDIA', casetaId: null, activo: true },
  { correo: 'guardia.noche@uat.edu.mx', nombre: 'Ramón Silva', rol: 'GUARDIA', casetaId: null, activo: false },
];

export const personas: Persona[] = [];
export const vehiculos: VehiculoInterno[] = [];

const CONOCIDOS: Array<[string, TipoPersona, string, string, string, string, boolean]> = [
  ['María López', 'ESTUDIANTE', 'XYZ123A', 'Nissan', 'Versa', 'gris', true],
  ['Sofía Navarro', 'ESTUDIANTE', 'RST226M', 'Kia', 'Rio', 'rojo', false],
  ['Rosa Hernández', 'DOCENTE', 'KLM904E', 'Toyota', 'Corolla', 'blanco', true],
  ['Patricia Morales', 'ADMINISTRATIVO', 'TUV362G', 'Chevrolet', 'Aveo', 'azul', true],
  ['Diego Salas', 'ESTUDIANTE', 'GHT730A', 'Volkswagen', 'Jetta', 'negro', true],
  ['Jorge Ramírez', 'DOCENTE', 'JKL707J', 'Mazda', '3', 'gris', true],
  ['Ana Ruiz', 'ESTUDIANTE', 'WSD551B', 'Honda', 'Fit', 'plata', true],
  ['Raúl Ortiz', 'DOCENTE', 'QWE810K', 'Ford', 'Ranger', 'blanco', true],
  ['Laura Vega', 'ADMINISTRATIVO', 'MNB219D', 'Hyundai', 'Accent', 'gris', true],
  ['Luis García', 'ESTUDIANTE', 'PQR118F', 'Nissan', 'March', 'rojo', true],
];

const NOMBRES = ['Carmen', 'Fernando', 'Gabriela', 'Ricardo', 'Verónica', 'Alejandro', 'Daniela', 'Miguel', 'Paola', 'Óscar', 'Karla', 'Andrés', 'Elena', 'Héctor', 'Mónica', 'Sergio'];
const APELLIDOS = ['Castillo', 'Reyes', 'Mendoza', 'Aguilar', 'Santos', 'Domínguez', 'Flores', 'Cruz', 'Medina', 'Vargas', 'Ríos', 'Luna', 'Guerrero', 'Chávez'];
const AUTOS = [['Nissan', 'Versa'], ['Nissan', 'March'], ['Kia', 'Rio'], ['Toyota', 'Corolla'], ['Chevrolet', 'Aveo'], ['Volkswagen', 'Vento'], ['Mazda', '2'], ['Honda', 'City'], ['Hyundai', 'Grand i10'], ['Renault', 'Kwid']] as const;
const COLORES = ['gris', 'blanco', 'negro', 'rojo', 'azul', 'plata'] as const;
const LETRAS = 'ABCDEFGHJKLMNPRSTUVWXYZ'.split('');
const MOTIVOS = ['Entrega de material a Servicios Escolares', 'Trámite en Control Escolar', 'Proveedor de cafetería', 'Mantenimiento de aires acondicionados', 'Visita a la Coordinación de Sistemas', 'Paquetería'];

const placaAzar = () => `${elegir(LETRAS)}${elegir(LETRAS)}${elegir(LETRAS)}${100 + Math.floor(azar() * 900)}${elegir(LETRAS)}`;

function identificador(tipo: TipoPersona): string | null {
  if (tipo === 'VISITANTE') return null;
  if (tipo === 'ESTUDIANTE') return `22${String(10000000 + Math.floor(azar() * 89999999))}`;
  return `E${10000 + Math.floor(azar() * 89999)}`;
}

function tipoAzar(): TipoPersona {
  const r = azar();
  return r < 0.66 ? 'ESTUDIANTE' : r < 0.82 ? 'DOCENTE' : r < 0.95 ? 'ADMINISTRATIVO' : 'VISITANTE';
}

function agregarPersonaConVehiculo(nombre: string, tipo: TipoPersona, placa: string, marca: string, modelo: string, color: string, autorizado: boolean) {
  const id = `per-${personas.length + 1}`;
  personas.push({ id, nombre, tipo, identificador: identificador(tipo), telefono: null, activo: true, creadoEn: ALTA });
  vehiculos.push({ placa, personaId: id, marca, modelo, color, autorizado, creadoEn: ALTA });
}

for (const c of CONOCIDOS) agregarPersonaConVehiculo(...c);
for (let i = 0; i < 300; i++) {
  const [marca, modelo] = elegir(AUTOS);
  agregarPersonaConVehiculo(`${elegir(NOMBRES)} ${elegir(APELLIDOS)}`, tipoAzar(), placaAzar(), marca, modelo, elegir(COLORES), azar() > 0.03);
}

const GUARDIA_DE: Record<string, string> = { 'cas-1': 'guardia1@uat.edu.mx', 'cas-2': 'guardia2@uat.edu.mx', 'cas-3': 'guardia3@uat.edu.mx' };
// Peso de cada hora de llegada, de 6 a 21 h.
const PESO_HORA = [3, 30, 14, 8, 6, 6, 8, 10, 7, 5, 4, 6, 5, 3, 2, 1];

function horaDeLlegada(): number {
  const total = PESO_HORA.reduce((a, b) => a + b, 0);
  let r = azar() * total;
  for (let i = 0; i < PESO_HORA.length; i++) {
    r -= PESO_HORA[i]!;
    if (r <= 0) return 6 + i;
  }
  return 21;
}

function casetaAzar(): Caseta {
  const r = azar();
  return casetas[r < 0.48 ? 0 : r < 0.79 ? 1 : 2]!;
}

export const movimientos: MovimientoInterno[] = [];
export const bitacora: EntradaBitacora[] = [];

function generarMovimientos(ahora: number) {
  const hoyCampus = fechaDe(ahora);
  const nuevos: Omit<MovimientoInterno, 'id'>[] = [];

  const agregar = (ts: number, tipo: Movimiento['tipo'], placa: string, caseta: Caseta, datos: Pick<Movimiento, 'tipoPersona' | 'personaId' | 'nombrePersona' | 'vehiculoRegistrado' | 'autorizado' | 'motivo'>) => {
    if (ts > ahora) return;
    nuevos.push({ ts, tipo, placa, fechaHora: aFechaHora(ts), casetaId: caseta.id, casetaNombre: caseta.nombre, observaciones: null, registradoPor: GUARDIA_DE[caseta.id]!, ...datos });
  };

  for (let d = 13; d >= 0; d--) {
    const fecha = sumarDias(hoyCampus, -d);
    const dia = diaSemana(fecha);
    const feriado = fecha.endsWith('-11-16');
    const prob = feriado ? 0.12 : dia === 6 ? 0.28 : dia === 7 ? 0.08 : 0.85;
    const base = inicioDia(fecha);

    const visitas = Array.from({ length: dia >= 6 || feriado ? 2 : 9 }, () => ({ placa: placaAzar(), nombre: `${elegir(NOMBRES)} ${elegir(APELLIDOS)}`, motivo: elegir(MOTIVOS) }));
    const flota = [
      ...vehiculos.map((v) => {
        const p = personas.find((x) => x.id === v.personaId)!;
        return { placa: v.placa, datos: { tipoPersona: p.tipo, personaId: p.id, nombrePersona: p.nombre, vehiculoRegistrado: true, autorizado: v.autorizado, motivo: null }, prob };
      }),
      ...visitas.map((v) => ({ placa: v.placa, datos: { tipoPersona: 'VISITANTE' as const, personaId: null, nombrePersona: v.nombre, vehiculoRegistrado: false, autorizado: false, motivo: v.motivo }, prob: 1 })),
    ];

    for (const v of flota) {
      if (azar() > v.prob) continue;
      const entrada = base + (horaDeLlegada() * 60 + Math.floor(azar() * 60)) * 60000;
      const casetaEntrada = casetaAzar();
      agregar(entrada, 'ENTRADA', v.placa, casetaEntrada, v.datos);
      if (azar() < 0.02) continue; // salida que nunca se registró
      const cierre = base + (20.5 + azar() * 2.4) * 3600000;
      const salida = Math.min(entrada + (2 + azar() * 8) * 3600000, Math.max(cierre, entrada + 1800000));
      agregar(salida, 'SALIDA', v.placa, azar() < 0.7 ? casetaEntrada : casetaAzar(), v.datos);
    }
  }

  nuevos.sort((a, b) => a.ts - b.ts);
  nuevos.forEach((m, i) => movimientos.push({ ...m, id: `mov-${i + 1}` }));

  // Bitácora de hoy a partir de los movimientos, más algunas acciones de administración.
  const inicioHoy = inicioDia(hoyCampus);
  for (const m of movimientos) {
    if (m.ts < inicioHoy) continue;
    bitacora.push({ fechaHora: m.fechaHora, usuario: m.registradoPor, accion: 'movimientos.registrar', resultado: 'OK', detalle: `${m.tipo} de ${m.placa} en ${m.casetaNombre}` });
  }
  const extra = (h: number, usuario: string, accion: string, resultado: 'OK' | 'DENEGADO', detalle: string) => {
    const ts = inicioHoy + h * 3600000;
    if (ts <= ahora) bitacora.push({ fechaHora: aFechaHora(ts), usuario, accion, resultado, detalle });
  };
  extra(6.95, 'guardia1@uat.edu.mx', 'ACCESO', 'OK', 'Abrió la aplicación');
  extra(9.2, 'ana.torres@uat.edu.mx', 'vehiculos.actualizar', 'OK', 'RST226M: autorizado pasó a no');
  extra(11.5, 'guardia3@uat.edu.mx', 'reportes.resumen', 'DENEGADO', 'El rol Guardia no tiene este permiso');
  extra(13.1, 'seguridad.cutm@uat.edu.mx', 'bitacora.registrarExportacion', 'OK', 'reportes.afluencia en PDF');
  bitacora.sort((a, b) => a.fechaHora.localeCompare(b.fechaHora));
}

generarMovimientos(Date.now());
