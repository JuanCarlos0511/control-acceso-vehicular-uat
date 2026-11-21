/** @OnlyCurrentDoc */
// Acceso a Google Sheets: una hoja por tabla, fila 1 = encabezados (contrato §4 y §5).
// Todas las columnas tienen formato de texto plano para que Sheets no convierta placas,
// matrículas ni fechas; las fechas se guardan como texto ISO 8601 en la zona del campus.

var ZONA_ = 'America/Mexico_City';
var VERSION_CONTRATO_ = '1.1';
var TIPOS_PERSONA_ = ['ESTUDIANTE', 'DOCENTE', 'ADMINISTRATIVO', 'VISITANTE'];
var ROLES_ = ['ADMIN', 'SUPERVISOR', 'GUARDIA'];

var TABLAS_ = {
  Casetas: ['id', 'nombre', 'ubicacion', 'activa'],
  Usuarios: ['correo', 'nombre', 'rol', 'casetaId', 'activo'],
  Personas: ['id', 'nombre', 'tipo', 'identificador', 'telefono', 'activo', 'creadoEn'],
  Vehiculos: ['placa', 'personaId', 'marca', 'modelo', 'color', 'autorizado', 'creadoEn'],
  Movimientos: ['id', 'tipo', 'placa', 'fechaHora', 'casetaId', 'casetaNombre', 'tipoPersona', 'personaId',
    'nombrePersona', 'vehiculoRegistrado', 'autorizado', 'motivo', 'observaciones', 'registradoPor'],
  Bitacora: ['fechaHora', 'usuario', 'accion', 'resultado', 'detalle'],
};

var CASETAS_INICIALES_ = [
  { id: 'cas-1', nombre: 'Caseta 1', ubicacion: 'Acceso principal', activa: true },
  { id: 'cas-2', nombre: 'Caseta 2', ubicacion: null, activa: true },
  { id: 'cas-3', nombre: 'Caseta 3', ubicacion: null, activa: true },
];

function libro_() {
  return SpreadsheetApp.getActiveSpreadsheet();
}

function hoja_(nombre) {
  return libro_().getSheetByName(nombre);
}

/**
 * Crea las hojas que falten con sus encabezados, las tres casetas y al dueño del script
 * como ADMIN. Es idempotente; se llama desde setup() y antes de cada solicitud.
 */
function asegurarHojas_() {
  var cache = CacheService.getScriptCache();
  if (cache.get('hojas-listas')) return;
  var libro = libro_();
  Object.keys(TABLAS_).forEach(function (nombre) {
    if (libro.getSheetByName(nombre)) return;
    var columnas = TABLAS_[nombre];
    var h = libro.insertSheet(nombre);
    h.getRange(1, 1, h.getMaxRows(), columnas.length).setNumberFormat('@');
    h.getRange(1, 1, 1, columnas.length).setValues([columnas]).setFontWeight('bold');
    h.setFrozenRows(1);
    if (h.getMaxColumns() > columnas.length) h.deleteColumns(columnas.length + 1, h.getMaxColumns() - columnas.length);
    if (nombre === 'Casetas') CASETAS_INICIALES_.forEach(function (c) { agregar_('Casetas', c); });
    if (nombre === 'Usuarios') {
      var dueno = Session.getEffectiveUser().getEmail().toLowerCase();
      if (dueno) agregar_('Usuarios', { correo: dueno, nombre: nombreDeCorreo_(dueno), rol: 'ADMIN', casetaId: null, activo: true });
    }
  });
  // La hoja vacía que trae un archivo nuevo ya no hace falta.
  libro.getSheets().forEach(function (h) {
    if (!TABLAS_[h.getName()] && h.getLastRow() === 0 && libro.getSheets().length > 1) libro.deleteSheet(h);
  });
  var props = PropertiesService.getScriptProperties();
  if (props.getProperty('ROL_ALTA_AUTOMATICA') === null) props.setProperty('ROL_ALTA_AUTOMATICA', 'ADMIN');
  cache.put('hojas-listas', '1', 21600);
}

/** Filas de una tabla como [{ fila, d }]: `fila` es el número de fila en la hoja y `d` el objeto del contrato. */
function filas_(nombre, convertir) {
  var h = hoja_(nombre);
  var ultima = h.getLastRow();
  if (ultima < 2) return [];
  var columnas = TABLAS_[nombre];
  return h.getRange(2, 1, ultima - 1, columnas.length).getValues()
    .map(function (valores, i) {
      var o = {};
      columnas.forEach(function (c, j) { o[c] = valores[j]; });
      return { fila: i + 2, d: convertir(o) };
    })
    .filter(function (x) { return x.d !== null; });
}

function leer_(nombre, convertir) {
  return filas_(nombre, convertir).map(function (x) { return x.d; });
}

function agregar_(nombre, obj) {
  hoja_(nombre).appendRow(TABLAS_[nombre].map(function (c) { return celda_(obj[c]); }));
}

function reescribir_(nombre, fila, obj) {
  var columnas = TABLAS_[nombre];
  hoja_(nombre).getRange(fila, 1, 1, columnas.length).setValues([columnas.map(function (c) { return celda_(obj[c]); })]);
}

/** Valor para una celda. Un texto que empieza con = + - @ se guarda como texto, no como fórmula. */
function celda_(v) {
  if (v === null || v === undefined) return '';
  if (typeof v === 'string' && /^[=+\-@]/.test(v)) return "'" + v;
  return v;
}

function texto_(v) {
  if (v === null || v === undefined) return null;
  if (v instanceof Date) return aFechaHora_(v);
  var s = String(v).trim();
  return s === '' ? null : s;
}

/** Las hojas pueden devolver booleanos reales o los textos "TRUE"/"FALSE". */
function bool_(v) {
  return v === true || String(v).toUpperCase() === 'TRUE';
}

// ---------- conversión de filas a los tipos del contrato (§5) ----------

function aCaseta_(o) {
  if (!texto_(o.id)) return null;
  return { id: texto_(o.id), nombre: texto_(o.nombre) || '', ubicacion: texto_(o.ubicacion), activa: bool_(o.activa) };
}

function aUsuario_(o) {
  if (!texto_(o.correo)) return null;
  return {
    correo: texto_(o.correo).toLowerCase(),
    nombre: texto_(o.nombre) || '',
    rol: texto_(o.rol),
    casetaId: texto_(o.casetaId),
    activo: bool_(o.activo),
  };
}

function aPersona_(o) {
  if (!texto_(o.id)) return null;
  return {
    id: texto_(o.id),
    nombre: texto_(o.nombre) || '',
    tipo: texto_(o.tipo),
    identificador: texto_(o.identificador),
    telefono: texto_(o.telefono),
    activo: bool_(o.activo),
    creadoEn: texto_(o.creadoEn),
  };
}

/** Vehículo como se guarda; `aVehiculo_` le agrega nombre y tipo de la persona. */
function aVehiculoFila_(o) {
  if (!texto_(o.placa)) return null;
  return {
    placa: normalizarPlaca_(o.placa),
    personaId: texto_(o.personaId),
    marca: texto_(o.marca),
    modelo: texto_(o.modelo),
    color: texto_(o.color),
    autorizado: bool_(o.autorizado),
    creadoEn: texto_(o.creadoEn),
  };
}

function aMovimiento_(o) {
  if (!texto_(o.id)) return null;
  return {
    id: texto_(o.id),
    tipo: texto_(o.tipo),
    placa: normalizarPlaca_(o.placa),
    fechaHora: texto_(o.fechaHora),
    casetaId: texto_(o.casetaId),
    casetaNombre: texto_(o.casetaNombre) || '',
    tipoPersona: texto_(o.tipoPersona),
    personaId: texto_(o.personaId),
    nombrePersona: texto_(o.nombrePersona),
    vehiculoRegistrado: bool_(o.vehiculoRegistrado),
    autorizado: bool_(o.autorizado),
    motivo: texto_(o.motivo),
    observaciones: texto_(o.observaciones),
    registradoPor: texto_(o.registradoPor) || '',
  };
}

function aBitacora_(o) {
  if (!texto_(o.fechaHora)) return null;
  return {
    fechaHora: texto_(o.fechaHora),
    usuario: texto_(o.usuario) || '',
    accion: texto_(o.accion) || '',
    resultado: texto_(o.resultado) || 'OK',
    detalle: texto_(o.detalle) || '',
  };
}

// ---------- fechas en la zona del campus ----------

function aFechaHora_(d) {
  return Utilities.formatDate(d, ZONA_, "yyyy-MM-dd'T'HH:mm:ssXXX");
}

function ahora_() {
  return aFechaHora_(new Date());
}

/** Fecha local "YYYY-MM-DD" de una FechaHora guardada por el servidor. */
function fechaDe_(fechaHora) {
  return String(fechaHora).slice(0, 10);
}

function horaDe_(fechaHora) {
  return Number(String(fechaHora).slice(11, 13));
}

function sumarDias_(fecha, dias) {
  var d = new Date(fecha + 'T12:00:00Z');
  d.setUTCDate(d.getUTCDate() + dias);
  return d.toISOString().slice(0, 10);
}

/** 1 = lunes … 7 = domingo. */
function diaSemana_(fecha) {
  var d = new Date(fecha + 'T12:00:00Z').getUTCDay();
  return d === 0 ? 7 : d;
}

function diasEntre_(desde, hasta) {
  return Math.round((Date.parse(hasta + 'T12:00:00Z') - Date.parse(desde + 'T12:00:00Z')) / 86400000);
}

function minutosEntre_(desdeFechaHora, hastaMs) {
  return Math.round((hastaMs - new Date(desdeFechaHora).getTime()) / 60000);
}

// ---------- utilidades ----------

function normalizarPlaca_(texto) {
  return String(texto === null || texto === undefined ? '' : texto).toUpperCase().replace(/[^A-Z0-9]/g, '');
}

function placaValida_(placa) {
  return /^[A-Z0-9]{4,10}$/.test(placa);
}

function sinAcentos_(s) {
  return String(s || '').normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase();
}

function contiene_(campo, texto) {
  return sinAcentos_(campo).indexOf(sinAcentos_(texto)) >= 0;
}

/** Recorta espacios; "" cuenta como null (contrato §4). */
function limpio_(s) {
  if (s === null || s === undefined) return null;
  var t = String(s).trim();
  return t === '' ? null : t;
}

function nuevoId_(prefijo) {
  return prefijo + '-' + Utilities.getUuid().slice(0, 8);
}

function nombreDeCorreo_(correo) {
  if (correo.indexOf('cuenta-') === 0) return 'Invitado ' + correo.slice(7, 11).toUpperCase();
  return correo.split('@')[0];
}

function porNombre_(a, b) {
  return a.nombre.localeCompare(b.nombre, 'es');
}
