// Tipos del contrato de API (docs/contrato-api.md, v1.1).
// Si algo cambia en el contrato, se cambia primero allá y luego aquí.

export type TipoPersona = 'ESTUDIANTE' | 'DOCENTE' | 'ADMINISTRATIVO' | 'VISITANTE';
export type TipoMovimiento = 'ENTRADA' | 'SALIDA';
export type Rol = 'ADMIN' | 'SUPERVISOR' | 'GUARDIA';
/** ISO 8601 con desfase, p. ej. "2026-11-20T07:42:10-06:00". */
export type FechaHora = string;
/** "YYYY-MM-DD", fecha local del campus. */
export type Fecha = string;

export const TIPOS_PERSONA: TipoPersona[] = ['ESTUDIANTE', 'DOCENTE', 'ADMINISTRATIVO', 'VISITANTE'];
export const ROLES: Rol[] = ['ADMIN', 'SUPERVISOR', 'GUARDIA'];

export interface Usuario {
  correo: string;
  nombre: string;
  rol: Rol;
  casetaId: string | null;
  activo: boolean;
}

export interface Caseta {
  id: string;
  nombre: string;
  ubicacion: string | null;
  activa: boolean;
}

export interface Persona {
  id: string;
  nombre: string;
  tipo: TipoPersona;
  identificador: string | null;
  telefono: string | null;
  activo: boolean;
  creadoEn: FechaHora;
}

export interface Vehiculo {
  placa: string;
  personaId: string;
  nombrePersona: string;
  tipoPersona: TipoPersona;
  marca: string | null;
  modelo: string | null;
  color: string | null;
  autorizado: boolean;
  creadoEn: FechaHora;
}

export interface Movimiento {
  id: string;
  tipo: TipoMovimiento;
  placa: string;
  fechaHora: FechaHora;
  casetaId: string;
  casetaNombre: string;
  tipoPersona: TipoPersona;
  personaId: string | null;
  nombrePersona: string | null;
  vehiculoRegistrado: boolean;
  autorizado: boolean;
  motivo: string | null;
  observaciones: string | null;
  registradoPor: string;
}

export type CodigoAdvertencia =
  | 'VEHICULO_NO_REGISTRADO'
  | 'VEHICULO_NO_AUTORIZADO'
  | 'ENTRADA_DUPLICADA'
  | 'SALIDA_SIN_ENTRADA';

export interface Advertencia {
  codigo: CodigoAdvertencia;
  mensaje: string;
}

export interface EntradaBitacora {
  fechaHora: FechaHora;
  usuario: string;
  accion: string;
  resultado: 'OK' | 'DENEGADO';
  detalle: string;
}

export interface Pagina<T> {
  elementos: T[];
  total: number;
  pagina: number;
  porPagina: number;
}

export type CodigoError =
  | 'SIN_IDENTIDAD'
  | 'NO_REGISTRADO'
  | 'USUARIO_INACTIVO'
  | 'PERMISO_DENEGADO'
  | 'VALIDACION'
  | 'NO_ENCONTRADO'
  | 'DUPLICADO'
  | 'CONFLICTO'
  | 'OCUPADO'
  | 'ACCION_DESCONOCIDA'
  | 'ERROR_INTERNO'
  | 'RED';

export interface ErrorApi {
  codigo: CodigoError;
  mensaje: string;
  campos?: Record<string, string>;
}

export type Respuesta<T> = { ok: true; datos: T } | { ok: false; error: ErrorApi };

// ---- Solicitudes y respuestas por acción ----

type SinDatos = Record<string, never>;

export interface Inicio {
  usuario: Usuario;
  permisos: string[];
  casetas: Caseta[];
  servidor: { zonaHoraria: string; ahora: FechaHora; versionContrato: string };
}

export interface VerificacionPlaca {
  placa: string;
  vehiculo: Vehiculo | null;
  persona: Persona | null;
  dentro: boolean;
  ultimoMovimiento: Movimiento | null;
  siguienteTipo: TipoMovimiento;
  advertencias: Advertencia[];
}

export interface SolicitudRegistro {
  tipo: TipoMovimiento;
  placa: string;
  casetaId?: string;
  tipoPersona?: TipoPersona;
  nombrePersona?: string;
  motivo?: string;
  observaciones?: string;
}

export interface ResultadoRegistro {
  movimiento: Movimiento;
  advertencias: Advertencia[];
  permanenciaMinutos: number | null;
}

export interface Paginacion {
  pagina?: number;
  porPagina?: number;
}

export interface FiltrosMovimientos extends Paginacion {
  placa?: string;
  desde?: Fecha;
  hasta?: Fecha;
  casetaId?: string;
  tipoPersona?: TipoPersona;
  tipo?: TipoMovimiento;
}

export interface VehiculoDentro {
  entrada: Movimiento;
  permanenciaMinutos: number;
}

export interface Periodo {
  desde: Fecha;
  hasta: Fecha;
  casetaId?: string;
}

export type Agrupacion = 'DIA' | 'SEMANA' | 'MES';

export interface Resumen {
  totales: { entradas: number; salidas: number };
  serie: Array<{ periodo: string; entradas: number; salidas: number }>;
  porCaseta: Array<{ casetaId: string; casetaNombre: string; entradas: number; salidas: number }>;
  porTipoPersona: Array<{ tipoPersona: TipoPersona; entradas: number; salidas: number }>;
}

export interface VehiculoFrecuente {
  placa: string;
  nombrePersona: string | null;
  tipoPersona: TipoPersona;
  vehiculoRegistrado: boolean;
  entradas: number;
  ultimaEntrada: FechaHora;
}

export interface Afluencia {
  porHora: Array<{ hora: number; entradas: number; salidas: number }>;
  porDiaSemana: Array<{ dia: number; entradas: number; salidas: number }>;
  matriz: number[][];
  pico: { dia: number; hora: number; total: number } | null;
}

export interface Acciones {
  'app.iniciar': { datos: SinDatos; respuesta: Inicio };

  'movimientos.verificarPlaca': { datos: { placa: string }; respuesta: VerificacionPlaca };
  'movimientos.registrar': { datos: SolicitudRegistro; respuesta: ResultadoRegistro };
  'movimientos.buscar': {
    datos: FiltrosMovimientos;
    respuesta: Pagina<Movimiento> & { truncado: boolean };
  };
  'movimientos.dentro': { datos: { casetaId?: string }; respuesta: VehiculoDentro[] };

  'reportes.resumen': { datos: Periodo & { agrupacion?: Agrupacion }; respuesta: Resumen };
  'reportes.vehiculosFrecuentes': {
    datos: Periodo & { tipoPersona?: TipoPersona; limite?: number };
    respuesta: VehiculoFrecuente[];
  };
  'reportes.afluencia': { datos: Periodo & { tipo?: TipoMovimiento }; respuesta: Afluencia };

  'casetas.listar': { datos: { incluirInactivas?: boolean }; respuesta: Caseta[] };
  'casetas.crear': { datos: { nombre: string; ubicacion?: string }; respuesta: Caseta };
  'casetas.actualizar': {
    datos: { id: string; nombre?: string; ubicacion?: string; activa?: boolean };
    respuesta: Caseta;
  };

  'personas.listar': {
    datos: Paginacion & { texto?: string; tipo?: TipoPersona; incluirInactivos?: boolean };
    respuesta: Pagina<Persona>;
  };
  'personas.crear': {
    datos: { nombre: string; tipo: TipoPersona; identificador?: string; telefono?: string };
    respuesta: Persona;
  };
  'personas.actualizar': {
    datos: {
      id: string;
      nombre?: string;
      tipo?: TipoPersona;
      identificador?: string;
      telefono?: string;
      activo?: boolean;
    };
    respuesta: Persona;
  };

  'vehiculos.listar': {
    datos: Paginacion & {
      texto?: string;
      personaId?: string;
      tipoPersona?: TipoPersona;
      autorizado?: boolean;
    };
    respuesta: Pagina<Vehiculo>;
  };
  'vehiculos.crear': {
    datos: {
      placa: string;
      personaId: string;
      marca?: string;
      modelo?: string;
      color?: string;
      autorizado?: boolean;
    };
    respuesta: Vehiculo;
  };
  'vehiculos.actualizar': {
    datos: {
      placa: string;
      personaId?: string;
      marca?: string;
      modelo?: string;
      color?: string;
      autorizado?: boolean;
    };
    respuesta: Vehiculo;
  };

  'usuarios.listar': { datos: { incluirInactivos?: boolean }; respuesta: Usuario[] };
  'usuarios.crear': {
    datos: { correo: string; nombre: string; rol: Rol; casetaId?: string };
    respuesta: Usuario;
  };
  'usuarios.actualizar': {
    datos: { correo: string; nombre?: string; rol?: Rol; casetaId?: string | null; activo?: boolean };
    respuesta: Usuario;
  };

  'bitacora.listar': {
    datos: Paginacion & { desde?: Fecha; hasta?: Fecha; usuario?: string; accion?: string };
    respuesta: Pagina<EntradaBitacora>;
  };
  'bitacora.registrarExportacion': {
    datos: { origen: string; formato: 'XLSX' | 'PDF'; filtros: object };
    respuesta: null;
  };
}

export type Accion = keyof Acciones;
export type DatosDe<A extends Accion> = Acciones[A]['datos'];
export type RespuestaDe<A extends Accion> = Acciones[A]['respuesta'];
