// Mock del contrato para desarrollar sin Apps Script (`npm run dev:*`).
// Imita reglas del backend: permisos por rol, validaciones, advertencias y bitácora.
// Cambia de usuario con ?rol=ADMIN | SUPERVISOR | GUARDIA | ROTATIVO | INACTIVO | NOREG | SINID
import type {
  Accion,
  Advertencia,
  Agrupacion,
  DatosDe,
  ErrorApi,
  Movimiento,
  Pagina,
  Persona,
  Respuesta,
  RespuestaDe,
  Rol,
  TipoMovimiento,
  Usuario,
  Vehiculo,
} from '../contrato';
import { TIPOS_PERSONA, ROLES } from '../contrato';
import { normalizarPlaca, placaValida, sumarDias, ZONA_CAMPUS } from '../formato';
import {
  aFechaHora,
  bitacora,
  casetas,
  diaSemana,
  fechaDe,
  horaDe,
  inicioDia,
  movimientos,
  personas,
  usuarios,
  vehiculos,
  type MovimientoInterno,
  type VehiculoInterno,
} from './datos';

class ErrorMock extends Error {
  constructor(public api: ErrorApi) {
    super(api.mensaje);
  }
}

const fallar = (codigo: ErrorApi['codigo'], mensaje: string, campos?: Record<string, string>): never => {
  throw new ErrorMock({ codigo, mensaje, ...(campos ? { campos } : {}) });
};

const validacion = (campos: Record<string, string>): never => fallar('VALIDACION', 'Revisa los datos marcados.', campos);

const TODOS: Rol[] = ['ADMIN', 'SUPERVISOR', 'GUARDIA'];
const LECTURA: Rol[] = ['ADMIN', 'SUPERVISOR'];
const ADMIN: Rol[] = ['ADMIN'];

// Matriz de permisos del contrato §6.
const PERMISOS: Record<Accion, Rol[]> = {
  'app.iniciar': TODOS,
  'movimientos.verificarPlaca': TODOS,
  'movimientos.registrar': ['ADMIN', 'GUARDIA'],
  'movimientos.buscar': TODOS,
  'movimientos.dentro': TODOS,
  'reportes.resumen': LECTURA,
  'reportes.vehiculosFrecuentes': LECTURA,
  'reportes.afluencia': LECTURA,
  'casetas.listar': TODOS,
  'casetas.crear': ADMIN,
  'casetas.actualizar': ADMIN,
  'personas.listar': LECTURA,
  'personas.crear': ADMIN,
  'personas.actualizar': ADMIN,
  'vehiculos.listar': LECTURA,
  'vehiculos.crear': ADMIN,
  'vehiculos.actualizar': ADMIN,
  'usuarios.listar': ADMIN,
  'usuarios.crear': ADMIN,
  'usuarios.actualizar': ADMIN,
  'bitacora.listar': LECTURA,
  'bitacora.registrarExportacion': TODOS,
};

const ESCRIBE = new Set<Accion>([
  'movimientos.registrar',
  'casetas.crear',
  'casetas.actualizar',
  'personas.crear',
  'personas.actualizar',
  'vehiculos.crear',
  'vehiculos.actualizar',
  'usuarios.crear',
  'usuarios.actualizar',
  'bitacora.registrarExportacion',
]);

const CORREO_POR_ROL: Record<string, string> = {
  ADMIN: 'ana.torres@uat.edu.mx',
  SUPERVISOR: 'seguridad.cutm@uat.edu.mx',
  GUARDIA: 'guardia1@uat.edu.mx',
  ROTATIVO: 'guardia.relevo@uat.edu.mx',
  INACTIVO: 'guardia.noche@uat.edu.mx',
};

function usuarioActual(): Usuario {
  const porDefecto = import.meta.env.MODE === 'admin' ? 'ADMIN' : 'GUARDIA';
  const rol = (new URLSearchParams(location.search).get('rol') ?? porDefecto).toUpperCase();
  if (rol === 'SINID') fallar('SIN_IDENTIDAD', 'No se pudo leer el correo de tu cuenta de Google.');
  const correo = CORREO_POR_ROL[rol] ?? 'invitado.prueba@uat.edu.mx';
  const u = usuarios.find((x) => x.correo === correo);
  if (!u) return fallar('NO_REGISTRADO', `Entraste como ${correo}, una cuenta que no está dada de alta.`);
  if (!u.activo) fallar('USUARIO_INACTIVO', `La cuenta ${correo} fue desactivada por un administrador.`);
  return u;
}

// ---------- utilidades ----------

const sinAcentos = (s: string) => s.normalize('NFD').replace(/\p{Diacritic}/gu, '').toLowerCase();
const contiene = (campo: string | null | undefined, texto: string) => sinAcentos(campo ?? '').includes(sinAcentos(texto));
const limpio = (s: string | undefined | null) => (s ?? '').trim() || null;

function paginar<T>(lista: T[], pagina = 1, porPagina = 50): Pagina<T> {
  if (pagina < 1) validacion({ pagina: 'La página empieza en 1.' });
  if (porPagina < 1 || porPagina > 200) validacion({ porPagina: 'Entre 1 y 200 por página.' });
  return { elementos: lista.slice((pagina - 1) * porPagina, pagina * porPagina), total: lista.length, pagina, porPagina };
}

function caseta(id: string | undefined, campo = 'casetaId') {
  const c = casetas.find((x) => x.id === id && x.activa);
  if (!c) validacion({ [campo]: 'Selecciona una caseta activa.' });
  return c!;
}

function aVehiculo(v: VehiculoInterno): Vehiculo {
  const p = personas.find((x) => x.id === v.personaId)!;
  return { ...v, nombrePersona: p.nombre, tipoPersona: p.tipo };
}

const publico = ({ ts: _ts, ...m }: MovimientoInterno): Movimiento => m;

function enPeriodo(m: MovimientoInterno, desde?: string, hasta?: string) {
  const f = fechaDe(m.ts);
  return (!desde || f >= desde) && (!hasta || f <= hasta);
}

function validarPeriodo(desde?: string, hasta?: string) {
  if (!desde || !hasta) validacion({ desde: 'Indica el periodo.', hasta: 'Indica el periodo.' });
  if (desde! > hasta!) validacion({ hasta: 'Debe ser igual o posterior a la fecha de inicio.' });
  if ((inicioDia(hasta!) - inicioDia(desde!)) / 86400000 > 366) validacion({ desde: 'El periodo máximo es de 366 días.' });
}

const ultimoDe = (placa: string) => {
  for (let i = movimientos.length - 1; i >= 0; i--) if (movimientos[i]!.placa === placa) return movimientos[i]!;
  return null;
};

function advertenciasDePlaca(placa: string, v: VehiculoInterno | undefined): Advertencia[] {
  if (!v) return [{ codigo: 'VEHICULO_NO_REGISTRADO', mensaje: `La placa ${placa} no está en el catálogo de vehículos.` }];
  if (!v.autorizado) return [{ codigo: 'VEHICULO_NO_AUTORIZADO', mensaje: `El vehículo ${placa} no está autorizado.` }];
  return [];
}

function periodoDe(fecha: string, agrupacion: Agrupacion) {
  if (agrupacion === 'MES') return fecha.slice(0, 7);
  if (agrupacion === 'SEMANA') return sumarDias(fecha, 1 - diaSemana(fecha));
  return fecha;
}

// ---------- manejadores ----------

type Manejadores = { [A in Accion]: (datos: DatosDe<A>, u: Usuario) => RespuestaDe<A> };

const manejadores: Manejadores = {
  'app.iniciar': (_d, u) => ({
    usuario: u,
    permisos: (Object.keys(PERMISOS) as Accion[]).filter((a) => PERMISOS[a].includes(u.rol)),
    casetas: casetas.filter((c) => c.activa),
    servidor: { zonaHoraria: ZONA_CAMPUS, ahora: aFechaHora(Date.now()), versionContrato: '1.1' },
  }),

  'movimientos.verificarPlaca': ({ placa: texto }) => {
    const placa = normalizarPlaca(texto ?? '');
    if (!placaValida(placa)) validacion({ placa: 'La placa debe tener entre 4 y 10 letras o números.' });
    const v = vehiculos.find((x) => x.placa === placa);
    const ultimo = ultimoDe(placa);
    const dentro = ultimo?.tipo === 'ENTRADA';
    return {
      placa,
      vehiculo: v ? aVehiculo(v) : null,
      persona: v ? personas.find((p) => p.id === v.personaId) ?? null : null,
      dentro,
      ultimoMovimiento: ultimo ? publico(ultimo) : null,
      siguienteTipo: dentro ? 'SALIDA' : 'ENTRADA',
      advertencias: advertenciasDePlaca(placa, v),
    };
  },

  'movimientos.registrar': (d, u) => {
    const placa = normalizarPlaca(d.placa ?? '');
    const campos: Record<string, string> = {};
    if (d.tipo !== 'ENTRADA' && d.tipo !== 'SALIDA') campos.tipo = 'Indica si es entrada o salida.';
    if (!placaValida(placa)) campos.placa = 'La placa debe tener entre 4 y 10 letras o números.';
    if (Object.keys(campos).length) validacion(campos);

    if (u.casetaId && d.casetaId && d.casetaId !== u.casetaId) {
      fallar('PERMISO_DENEGADO', 'Solo puedes registrar en tu caseta asignada.');
    }
    const c = caseta(d.casetaId ?? u.casetaId ?? undefined);

    const v = vehiculos.find((x) => x.placa === placa);
    const p = v ? personas.find((x) => x.id === v.personaId) : undefined;
    const ultimo = ultimoDe(placa);
    const entradaAbierta = ultimo?.tipo === 'ENTRADA' ? ultimo : null;

    let datosPersona: Pick<Movimiento, 'tipoPersona' | 'personaId' | 'nombrePersona' | 'motivo'>;
    if (p) {
      datosPersona = { tipoPersona: p.tipo, personaId: p.id, nombrePersona: p.nombre, motivo: limpio(d.motivo) };
    } else if (d.tipo === 'SALIDA' && entradaAbierta) {
      datosPersona = { tipoPersona: entradaAbierta.tipoPersona, personaId: null, nombrePersona: entradaAbierta.nombrePersona, motivo: entradaAbierta.motivo };
    } else {
      if (!d.tipoPersona || !TIPOS_PERSONA.includes(d.tipoPersona)) validacion({ tipoPersona: 'Selecciona el tipo de usuario.' });
      datosPersona = { tipoPersona: d.tipoPersona!, personaId: null, nombrePersona: limpio(d.nombrePersona), motivo: limpio(d.motivo) };
    }

    const ts = Date.now();
    const m: MovimientoInterno = {
      id: `mov-${movimientos.length + 1}`,
      ts,
      tipo: d.tipo,
      placa,
      fechaHora: aFechaHora(ts),
      casetaId: c.id,
      casetaNombre: c.nombre,
      ...datosPersona,
      vehiculoRegistrado: Boolean(v),
      autorizado: Boolean(v?.autorizado),
      observaciones: limpio(d.observaciones),
      registradoPor: u.correo,
    };

    const advertencias = advertenciasDePlaca(placa, v);
    if (d.tipo === 'ENTRADA' && entradaAbierta) advertencias.push({ codigo: 'ENTRADA_DUPLICADA', mensaje: 'Este vehículo ya tenía una entrada sin salida.' });
    if (d.tipo === 'SALIDA' && !entradaAbierta) advertencias.push({ codigo: 'SALIDA_SIN_ENTRADA', mensaje: 'No hay una entrada registrada para este vehículo.' });

    movimientos.push(m);
    return {
      movimiento: publico(m),
      advertencias,
      permanenciaMinutos: d.tipo === 'SALIDA' && entradaAbierta ? Math.round((ts - entradaAbierta.ts) / 60000) : null,
    };
  },

  'movimientos.buscar': (f) => {
    const placa = f.placa ? normalizarPlaca(f.placa) : '';
    const lista = movimientos
      .filter(
        (m) =>
          (!placa || m.placa.includes(placa)) &&
          enPeriodo(m, f.desde, f.hasta) &&
          (!f.casetaId || m.casetaId === f.casetaId) &&
          (!f.tipoPersona || m.tipoPersona === f.tipoPersona) &&
          (!f.tipo || m.tipo === f.tipo),
      )
      .reverse()
      .map(publico);
    if (f.porPagina === 0) {
      return { elementos: lista.slice(0, 5000), total: lista.length, pagina: 1, porPagina: 0, truncado: lista.length > 5000 };
    }
    return { ...paginar(lista, f.pagina, f.porPagina), truncado: false };
  },

  'movimientos.dentro': ({ casetaId }) => {
    const ultimos = new Map<string, MovimientoInterno>();
    for (const m of movimientos) ultimos.set(m.placa, m);
    const ahora = Date.now();
    return [...ultimos.values()]
      .filter((m) => m.tipo === 'ENTRADA' && (!casetaId || m.casetaId === casetaId))
      .map((m) => ({ entrada: publico(m), permanenciaMinutos: Math.round((ahora - m.ts) / 60000) }))
      .sort((a, b) => b.permanenciaMinutos - a.permanenciaMinutos);
  },

  'reportes.resumen': ({ desde, hasta, casetaId, agrupacion = 'DIA' }) => {
    validarPeriodo(desde, hasta);
    const lista = movimientos.filter((m) => enPeriodo(m, desde, hasta) && (!casetaId || m.casetaId === casetaId));
    const cuenta = (xs: MovimientoInterno[]) => ({
      entradas: xs.filter((m) => m.tipo === 'ENTRADA').length,
      salidas: xs.filter((m) => m.tipo === 'SALIDA').length,
    });

    const serie = new Map<string, MovimientoInterno[]>();
    for (let f = desde; f <= hasta; f = sumarDias(f, 1)) serie.set(periodoDe(f, agrupacion), []);
    for (const m of lista) serie.get(periodoDe(fechaDe(m.ts), agrupacion))?.push(m);

    return {
      totales: cuenta(lista),
      serie: [...serie].map(([periodo, xs]) => ({ periodo, ...cuenta(xs) })),
      porCaseta: casetas.map((c) => ({ casetaId: c.id, casetaNombre: c.nombre, ...cuenta(lista.filter((m) => m.casetaId === c.id)) })),
      porTipoPersona: TIPOS_PERSONA.map((t) => ({ tipoPersona: t, ...cuenta(lista.filter((m) => m.tipoPersona === t)) })),
    };
  },

  'reportes.vehiculosFrecuentes': ({ desde, hasta, casetaId, tipoPersona, limite = 10 }) => {
    validarPeriodo(desde, hasta);
    if (limite < 1 || limite > 100) validacion({ limite: 'Entre 1 y 100.' });
    const grupos = new Map<string, MovimientoInterno[]>();
    for (const m of movimientos) {
      if (m.tipo !== 'ENTRADA' || !enPeriodo(m, desde, hasta)) continue;
      if ((casetaId && m.casetaId !== casetaId) || (tipoPersona && m.tipoPersona !== tipoPersona)) continue;
      grupos.set(m.placa, [...(grupos.get(m.placa) ?? []), m]);
    }
    return [...grupos.values()]
      .map((xs) => {
        const ultima = xs[xs.length - 1]!;
        return { placa: ultima.placa, nombrePersona: ultima.nombrePersona, tipoPersona: ultima.tipoPersona, vehiculoRegistrado: ultima.vehiculoRegistrado, entradas: xs.length, ultimaEntrada: ultima.fechaHora };
      })
      .sort((a, b) => b.entradas - a.entradas || b.ultimaEntrada.localeCompare(a.ultimaEntrada))
      .slice(0, limite);
  },

  'reportes.afluencia': ({ desde, hasta, casetaId, tipo }) => {
    validarPeriodo(desde, hasta);
    const porHora = Array.from({ length: 24 }, (_, hora) => ({ hora, entradas: 0, salidas: 0 }));
    const porDiaSemana = Array.from({ length: 7 }, (_, i) => ({ dia: i + 1, entradas: 0, salidas: 0 }));
    const matriz = Array.from({ length: 7 }, () => Array<number>(24).fill(0));
    for (const m of movimientos) {
      if (!enPeriodo(m, desde, hasta) || (casetaId && m.casetaId !== casetaId)) continue;
      const h = horaDe(m.ts);
      const dia = diaSemana(fechaDe(m.ts));
      const clave = m.tipo === 'ENTRADA' ? 'entradas' : 'salidas';
      porHora[h]![clave]++;
      porDiaSemana[dia - 1]![clave]++;
      if (!tipo || m.tipo === tipo) matriz[dia - 1]![h]!++;
    }
    let pico: { dia: number; hora: number; total: number } | null = null;
    matriz.forEach((fila, d) =>
      fila.forEach((total, hora) => {
        if (total > 0 && (!pico || total > pico.total)) pico = { dia: d + 1, hora, total };
      }),
    );
    return { porHora, porDiaSemana, matriz, pico };
  },

  'casetas.listar': ({ incluirInactivas }) =>
    casetas.filter((c) => incluirInactivas || c.activa).sort((a, b) => a.nombre.localeCompare(b.nombre)),

  'casetas.crear': ({ nombre, ubicacion }) => {
    if (!limpio(nombre)) validacion({ nombre: 'Escribe el nombre de la caseta.' });
    const c = { id: `cas-${casetas.length + 1}`, nombre: limpio(nombre)!, ubicacion: limpio(ubicacion), activa: true };
    casetas.push(c);
    return c;
  },

  'casetas.actualizar': ({ id, ...cambios }) => {
    const c = casetas.find((x) => x.id === id) ?? fallar('NO_ENCONTRADO', 'La caseta no existe.');
    if (cambios.nombre !== undefined && !limpio(cambios.nombre)) validacion({ nombre: 'Escribe el nombre de la caseta.' });
    if (cambios.nombre !== undefined) c.nombre = limpio(cambios.nombre)!;
    if (cambios.ubicacion !== undefined) c.ubicacion = limpio(cambios.ubicacion);
    if (cambios.activa !== undefined) c.activa = cambios.activa;
    return c;
  },

  'personas.listar': ({ texto, tipo, incluirInactivos, pagina, porPagina }) =>
    paginar(
      personas
        .filter((p) => (incluirInactivos || p.activo) && (!tipo || p.tipo === tipo) && (!texto || contiene(p.nombre, texto) || contiene(p.identificador, texto)))
        .sort((a, b) => a.nombre.localeCompare(b.nombre)),
      pagina,
      porPagina,
    ),

  'personas.crear': (d) => {
    const p = validarPersona(d);
    const nueva: Persona = { id: `per-${personas.length + 1}`, nombre: p.nombre, tipo: p.tipo, identificador: p.identificador, telefono: p.telefono, activo: true, creadoEn: aFechaHora(Date.now()) };
    personas.push(nueva);
    return nueva;
  },

  'personas.actualizar': ({ id, activo, ...d }) => {
    const p = personas.find((x) => x.id === id) ?? fallar('NO_ENCONTRADO', 'La persona no existe.');
    const v = validarPersona({ nombre: d.nombre ?? p.nombre, tipo: d.tipo ?? p.tipo, identificador: d.identificador ?? p.identificador ?? undefined, telefono: d.telefono ?? p.telefono ?? undefined }, id);
    Object.assign(p, v, activo === undefined ? {} : { activo });
    return p;
  },

  'vehiculos.listar': ({ texto, personaId, tipoPersona, autorizado, pagina, porPagina }) =>
    paginar(
      vehiculos
        .map(aVehiculo)
        .filter(
          (v) =>
            (!personaId || v.personaId === personaId) &&
            (!tipoPersona || v.tipoPersona === tipoPersona) &&
            (autorizado === undefined || v.autorizado === autorizado) &&
            (!texto || v.placa.includes(normalizarPlaca(texto)) || contiene(v.nombrePersona, texto) || contiene(v.marca, texto) || contiene(v.modelo, texto)),
        )
        .sort((a, b) => a.nombrePersona.localeCompare(b.nombrePersona)),
      pagina,
      porPagina,
    ),

  'vehiculos.crear': (d) => {
    const placa = normalizarPlaca(d.placa ?? '');
    if (!placaValida(placa)) validacion({ placa: 'La placa debe tener entre 4 y 10 letras o números.' });
    if (vehiculos.some((v) => v.placa === placa)) fallar('DUPLICADO', 'Ya existe un vehículo con esa placa.', { placa: `Ya existe un vehículo con la placa ${placa}.` });
    validarDueno(d.personaId);
    const v: VehiculoInterno = { placa, personaId: d.personaId, marca: limpio(d.marca), modelo: limpio(d.modelo), color: limpio(d.color), autorizado: d.autorizado ?? true, creadoEn: aFechaHora(Date.now()) };
    vehiculos.push(v);
    return aVehiculo(v);
  },

  'vehiculos.actualizar': ({ placa, ...c }) => {
    const v = vehiculos.find((x) => x.placa === normalizarPlaca(placa)) ?? fallar('NO_ENCONTRADO', 'El vehículo no existe.');
    if (c.personaId !== undefined) validarDueno(c.personaId);
    Object.assign(v, {
      ...(c.personaId !== undefined && { personaId: c.personaId }),
      ...(c.marca !== undefined && { marca: limpio(c.marca) }),
      ...(c.modelo !== undefined && { modelo: limpio(c.modelo) }),
      ...(c.color !== undefined && { color: limpio(c.color) }),
      ...(c.autorizado !== undefined && { autorizado: c.autorizado }),
    });
    return aVehiculo(v);
  },

  'usuarios.listar': ({ incluirInactivos }) =>
    usuarios.filter((u) => incluirInactivos || u.activo).sort((a, b) => a.nombre.localeCompare(b.nombre)),

  'usuarios.crear': ({ correo, nombre, rol, casetaId }) => {
    const c = (correo ?? '').trim().toLowerCase();
    const campos: Record<string, string> = {};
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(c)) campos.correo = 'Escribe un correo válido.';
    if (!limpio(nombre)) campos.nombre = 'Escribe el nombre.';
    if (!ROLES.includes(rol)) campos.rol = 'Selecciona un rol.';
    if (Object.keys(campos).length) validacion(campos);
    if (usuarios.some((u) => u.correo === c)) fallar('DUPLICADO', 'Ese correo ya tiene usuario.', { correo: 'Ese correo ya tiene usuario.' });
    if (casetaId) caseta(casetaId);
    const u: Usuario = { correo: c, nombre: limpio(nombre)!, rol, casetaId: casetaId ?? null, activo: true };
    usuarios.push(u);
    return u;
  },

  'usuarios.actualizar': ({ correo, ...c }, actual) => {
    const u = usuarios.find((x) => x.correo === correo) ?? fallar('NO_ENCONTRADO', 'El usuario no existe.');
    const dejaDeSerAdmin = u.rol === 'ADMIN' && u.activo && ((c.rol && c.rol !== 'ADMIN') || c.activo === false);
    if (c.activo === false && u.correo === actual.correo) fallar('CONFLICTO', 'No puedes desactivar tu propia cuenta.');
    if (dejaDeSerAdmin && usuarios.filter((x) => x.rol === 'ADMIN' && x.activo).length === 1) {
      fallar('CONFLICTO', 'El sistema necesita al menos un administrador activo.');
    }
    if (c.nombre !== undefined && !limpio(c.nombre)) validacion({ nombre: 'Escribe el nombre.' });
    if (c.casetaId) caseta(c.casetaId);
    Object.assign(u, {
      ...(c.nombre !== undefined && { nombre: limpio(c.nombre) }),
      ...(c.rol !== undefined && { rol: c.rol }),
      ...(c.casetaId !== undefined && { casetaId: c.casetaId }),
      ...(c.activo !== undefined && { activo: c.activo }),
    });
    return u;
  },

  'bitacora.listar': ({ desde, hasta, usuario, accion, pagina, porPagina }) =>
    paginar(
      bitacora
        .filter((b) => {
          const f = b.fechaHora.slice(0, 10);
          return (!desde || f >= desde) && (!hasta || f <= hasta) && (!usuario || contiene(b.usuario, usuario)) && (!accion || b.accion === accion);
        })
        .slice()
        .reverse(),
      pagina,
      porPagina,
    ),

  'bitacora.registrarExportacion': () => null,
};

function validarPersona(d: { nombre?: string; tipo?: Persona['tipo']; identificador?: string; telefono?: string }, idActual?: string) {
  const campos: Record<string, string> = {};
  const nombre = limpio(d.nombre);
  const identificador = limpio(d.identificador);
  if (!nombre) campos.nombre = 'Escribe el nombre.';
  if (!d.tipo || !TIPOS_PERSONA.includes(d.tipo)) campos.tipo = 'Selecciona el tipo.';
  if (d.tipo && d.tipo !== 'VISITANTE' && !identificador) campos.identificador = 'Escribe la matrícula o el número de empleado.';
  if (Object.keys(campos).length) validacion(campos);
  if (identificador && personas.some((p) => p.identificador === identificador && p.id !== idActual)) {
    fallar('DUPLICADO', 'Ya hay una persona con ese identificador.', { identificador: 'Ya hay una persona con ese identificador.' });
  }
  return { nombre: nombre!, tipo: d.tipo!, identificador, telefono: limpio(d.telefono) };
}

function validarDueno(personaId: string | undefined) {
  const p = personas.find((x) => x.id === personaId);
  if (!p || !p.activo) validacion({ personaId: 'Selecciona una persona activa.' });
}

function resumirDatos(accion: Accion, datos: unknown): string {
  const d = datos as Record<string, unknown>;
  if (accion === 'movimientos.registrar') return `${d.tipo as TipoMovimiento} de ${normalizarPlaca(String(d.placa ?? ''))}`;
  return JSON.stringify(datos).slice(0, 120);
}

const espera = (ms: number) => new Promise((r) => setTimeout(r, ms));

export async function mockApi<A extends Accion>(accion: A, datos: DatosDe<A>): Promise<Respuesta<RespuestaDe<A>>> {
  await espera(250 + Math.random() * 350);
  let correo = '';
  try {
    const u = usuarioActual();
    correo = u.correo;
    const roles = PERMISOS[accion] ?? fallar('ACCION_DESCONOCIDA', `Acción no reconocida: ${accion}`);
    if (!roles.includes(u.rol)) fallar('PERMISO_DENEGADO', 'No tienes permiso para esta acción.');
    const manejar = manejadores[accion] as (d: DatosDe<A>, u: Usuario) => RespuestaDe<A>;
    const resultado = manejar(structuredClone(datos), u);
    if (ESCRIBE.has(accion)) {
      bitacora.push({ fechaHora: aFechaHora(Date.now()), usuario: u.correo, accion, resultado: 'OK', detalle: resumirDatos(accion, datos) });
    }
    if (accion === 'app.iniciar') {
      bitacora.push({ fechaHora: aFechaHora(Date.now()), usuario: u.correo, accion: 'ACCESO', resultado: 'OK', detalle: 'Abrió la aplicación' });
    }
    return { ok: true, datos: structuredClone(resultado) };
  } catch (e) {
    if (e instanceof ErrorMock) {
      if (e.api.codigo === 'PERMISO_DENEGADO' && correo) {
        bitacora.push({ fechaHora: aFechaHora(Date.now()), usuario: correo, accion, resultado: 'DENEGADO', detalle: e.api.mensaje });
      }
      return { ok: false, error: e.api };
    }
    console.error('[mock]', e);
    return { ok: false, error: { codigo: 'ERROR_INTERNO', mensaje: 'Error inesperado en el mock.' } };
  }
}
