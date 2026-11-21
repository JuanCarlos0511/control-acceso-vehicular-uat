/** @OnlyCurrentDoc */
// Punto de entrada del Web App. Solo doGet, api y setup son públicas; todo lo que termina
// en "_" es privado y no se puede llamar desde google.script.run (contrato §9).

/** La URL /exec abre la app de caseta; con ?app=admin, el portal de administración. */
function doGet(e) {
  var admin = Boolean(e && e.parameter && e.parameter.app === 'admin');
  return HtmlService.createHtmlOutputFromFile(admin ? 'portal-admin' : 'app-caseta')
    .setTitle(admin ? 'Portal de administración · Acceso vehicular' : 'Caseta · Acceso vehicular')
    .addMetaTag('viewport', 'width=device-width, initial-scale=1');
}

/**
 * Única función que llama la interfaz: api({ accion, datos }).
 * Siempre responde un sobre { ok, datos } o { ok: false, error } y nunca lanza (contrato §1 y §2).
 */
function api(solicitud) {
  var correo = '';
  var accion = solicitud && solicitud.accion;
  try {
    asegurarHojas_();
    var usuario = usuarioActual_();
    correo = usuario.correo;
    var def = acciones_()[accion];
    if (!def) throw errorApi_('ACCION_DESCONOCIDA', 'Acción no reconocida.');
    if (def.roles.indexOf(usuario.rol) < 0) throw errorApi_('PERMISO_DENEGADO', 'No tienes permiso para esta acción.');
    var datos = solicitud.datos && typeof solicitud.datos === 'object' ? solicitud.datos : {};
    var resultado = def.escribe
      ? conCandado_(function () {
          var r = def.fn(datos, usuario);
          registrarBitacora_(usuario.correo, accion, 'OK', resumirDatos_(accion, datos));
          return r;
        })
      : def.fn(datos, usuario);
    if (accion === 'app.iniciar') registrarBitacora_(usuario.correo, 'ACCESO', 'OK', 'Abrió la aplicación');
    return { ok: true, datos: resultado === undefined ? null : resultado };
  } catch (e) {
    if (e && e.api) {
      if (e.api.codigo === 'PERMISO_DENEGADO' && correo) registrarBitacora_(correo, String(accion), 'DENEGADO', e.api.mensaje);
      return { ok: false, error: e.api };
    }
    console.error(e && e.stack ? e.stack : e);
    return { ok: false, error: { codigo: 'ERROR_INTERNO', mensaje: 'Ocurrió un error inesperado en el servidor. Intenta de nuevo.' } };
  }
}

/** Crea las hojas, las tres casetas y al dueño como ADMIN. Se puede ejecutar desde el editor. */
function setup() {
  var activo = Session.getActiveUser().getEmail();
  if (activo && activo !== Session.getEffectiveUser().getEmail()) throw new Error('Solo el dueño del script puede ejecutar setup().');
  CacheService.getScriptCache().remove('hojas-listas');
  asegurarHojas_();
  return 'Listo: hojas ' + Object.keys(TABLAS_).join(', ') + '.';
}

// ---------- permisos (contrato §3 y §6) ----------

function acciones_() {
  var TODOS = ['ADMIN', 'SUPERVISOR', 'GUARDIA'];
  var LECTURA = ['ADMIN', 'SUPERVISOR'];
  var ADMIN = ['ADMIN'];
  var a = function (fn, roles, escribe) { return { fn: fn, roles: roles, escribe: Boolean(escribe) }; };
  return {
    'app.iniciar': a(appIniciar_, TODOS),
    'movimientos.verificarPlaca': a(verificarPlaca_, TODOS),
    'movimientos.registrar': a(registrarMovimiento_, ['ADMIN', 'GUARDIA'], true),
    'movimientos.buscar': a(buscarMovimientos_, TODOS),
    'movimientos.dentro': a(vehiculosDentro_, TODOS),
    'reportes.resumen': a(reporteResumen_, LECTURA),
    'reportes.vehiculosFrecuentes': a(reporteFrecuentes_, LECTURA),
    'reportes.afluencia': a(reporteAfluencia_, LECTURA),
    'casetas.listar': a(listarCasetas_, TODOS),
    'casetas.crear': a(crearCaseta_, ADMIN, true),
    'casetas.actualizar': a(actualizarCaseta_, ADMIN, true),
    'personas.listar': a(listarPersonas_, LECTURA),
    'personas.crear': a(crearPersona_, ADMIN, true),
    'personas.actualizar': a(actualizarPersona_, ADMIN, true),
    'vehiculos.listar': a(listarVehiculos_, LECTURA),
    'vehiculos.crear': a(crearVehiculo_, ADMIN, true),
    'vehiculos.actualizar': a(actualizarVehiculo_, ADMIN, true),
    'usuarios.listar': a(listarUsuarios_, ADMIN),
    'usuarios.crear': a(crearUsuario_, ADMIN, true),
    'usuarios.actualizar': a(actualizarUsuario_, ADMIN, true),
    'bitacora.listar': a(listarBitacora_, LECTURA),
    'bitacora.registrarExportacion': a(registrarExportacion_, TODOS, true),
  };
}

function permisosDe_(rol) {
  var todas = acciones_();
  return Object.keys(todas).filter(function (accion) { return todas[accion].roles.indexOf(rol) >= 0; });
}

/**
 * Usuario de la hoja Usuarios según la sesión de Google; el cliente nunca envía su identidad.
 * Un Web App publicado por una cuenta personal solo recibe el correo de su dueño. A las demás cuentas
 * se les identifica con la clave anónima que Google da por cuenta (getTemporaryActiveUserKey) y se
 * guardan como "cuenta-<clave>". Si la cuenta no está registrada, se da de alta con el rol de la
 * propiedad del script ROL_ALTA_AUTOMATICA; vacía, desactiva el alta automática.
 */
function usuarioActual_() {
  var correo = (Session.getActiveUser().getEmail() || '').toLowerCase();
  if (!correo) {
    var clave = String(Session.getTemporaryActiveUserKey() || '').replace(/[^A-Za-z0-9]/g, '').slice(0, 12).toLowerCase();
    if (clave) correo = 'cuenta-' + clave;
  }
  if (!correo) {
    throw errorApi_('SIN_IDENTIDAD', 'No se pudo identificar tu cuenta de Google. Inicia sesión con una cuenta de Google y usa una sola sesión en el navegador.');
  }
  var usuario = leer_('Usuarios', aUsuario_).filter(function (u) { return u.correo === correo; })[0];
  if (!usuario) usuario = altaAutomatica_(correo);
  if (!usuario) throw errorApi_('NO_REGISTRADO', 'Entraste como ' + correo + ', una cuenta que no está dada de alta.');
  if (!usuario.activo) throw errorApi_('USUARIO_INACTIVO', 'La cuenta ' + correo + ' fue desactivada por un administrador.');
  if (ROLES_.indexOf(usuario.rol) < 0) throw errorApi_('USUARIO_INACTIVO', 'La cuenta ' + correo + ' no tiene un rol válido. Pide a un administrador que lo corrija.');
  return usuario;
}

function altaAutomatica_(correo) {
  var rol = String(PropertiesService.getScriptProperties().getProperty('ROL_ALTA_AUTOMATICA') || '').trim().toUpperCase();
  if (ROLES_.indexOf(rol) < 0) return null;
  return conCandado_(function () {
    var existente = leer_('Usuarios', aUsuario_).filter(function (u) { return u.correo === correo; })[0];
    if (existente) return existente;
    var u = { correo: correo, nombre: nombreDeCorreo_(correo), rol: rol, casetaId: null, activo: true };
    agregar_('Usuarios', u);
    registrarBitacora_(correo, 'usuarios.crear', 'OK', 'Alta automática con rol ' + rol);
    return u;
  });
}

// ---------- errores, candado y bitácora ----------

function errorApi_(codigo, mensaje, campos) {
  var e = new Error(mensaje);
  e.api = { codigo: codigo, mensaje: mensaje };
  if (campos) e.api.campos = campos;
  return e;
}

function validacion_(campos) {
  throw errorApi_('VALIDACION', 'Revisa los datos marcados.', campos);
}

/** Las tres casetas pueden escribir a la vez: cada escritura toma el candado del script. */
function conCandado_(fn) {
  var candado = LockService.getScriptLock();
  if (!candado.tryLock(10000)) throw errorApi_('OCUPADO', 'Otra caseta está guardando en este momento. Intenta de nuevo.');
  try {
    return fn();
  } finally {
    SpreadsheetApp.flush();
    candado.releaseLock();
  }
}

function registrarBitacora_(usuario, accion, resultado, detalle) {
  agregar_('Bitacora', { fechaHora: ahora_(), usuario: usuario, accion: accion, resultado: resultado, detalle: detalle });
}

function resumirDatos_(accion, datos) {
  if (accion === 'movimientos.registrar') return String(datos.tipo) + ' de ' + normalizarPlaca_(datos.placa);
  if (accion === 'bitacora.registrarExportacion') return String(datos.formato) + ' de ' + String(datos.origen);
  return JSON.stringify(datos).slice(0, 200);
}
