/** @OnlyCurrentDoc */
// Acciones del contrato §7. Reciben (datos, usuario) y devuelven lo que el contrato indica;
// los errores se lanzan con errorApi_ y el despachador los convierte en el sobre de respuesta.

// ---------- app ----------

function appIniciar_(_datos, usuario) {
  return {
    usuario: usuario,
    permisos: permisosDe_(usuario.rol),
    casetas: leer_('Casetas', aCaseta_).filter(function (c) { return c.activa; }),
    servidor: { zonaHoraria: ZONA_, ahora: ahora_(), versionContrato: VERSION_CONTRATO_ },
  };
}

// ---------- movimientos ----------

function ultimoDe_(movimientos, placa) {
  for (var i = movimientos.length - 1; i >= 0; i--) if (movimientos[i].placa === placa) return movimientos[i];
  return null;
}

function advertenciasDePlaca_(placa, vehiculo) {
  if (!vehiculo) return [{ codigo: 'VEHICULO_NO_REGISTRADO', mensaje: 'La placa ' + placa + ' no está en el catálogo de vehículos.' }];
  if (!vehiculo.autorizado) return [{ codigo: 'VEHICULO_NO_AUTORIZADO', mensaje: 'El vehículo ' + placa + ' no está autorizado.' }];
  return [];
}

function mapaPersonas_() {
  var mapa = {};
  leer_('Personas', aPersona_).forEach(function (p) { mapa[p.id] = p; });
  return mapa;
}

/** Vehículo del contrato: agrega nombre y tipo de su persona. */
function aVehiculo_(v, personas) {
  var p = personas[v.personaId];
  return {
    placa: v.placa,
    personaId: v.personaId,
    nombrePersona: p ? p.nombre : '',
    tipoPersona: p ? p.tipo : 'VISITANTE',
    marca: v.marca,
    modelo: v.modelo,
    color: v.color,
    autorizado: v.autorizado,
    creadoEn: v.creadoEn,
  };
}

function verificarPlaca_(datos) {
  var placa = normalizarPlaca_(datos.placa);
  if (!placaValida_(placa)) validacion_({ placa: 'La placa debe tener entre 4 y 10 letras o números.' });
  var personas = mapaPersonas_();
  var v = leer_('Vehiculos', aVehiculoFila_).filter(function (x) { return x.placa === placa; })[0];
  var ultimo = ultimoDe_(leer_('Movimientos', aMovimiento_), placa);
  var dentro = Boolean(ultimo && ultimo.tipo === 'ENTRADA');
  return {
    placa: placa,
    vehiculo: v ? aVehiculo_(v, personas) : null,
    persona: v ? personas[v.personaId] || null : null,
    dentro: dentro,
    ultimoMovimiento: ultimo,
    siguienteTipo: dentro ? 'SALIDA' : 'ENTRADA',
    advertencias: advertenciasDePlaca_(placa, v),
  };
}

function registrarMovimiento_(d, usuario) {
  var placa = normalizarPlaca_(d.placa);
  var campos = {};
  if (d.tipo !== 'ENTRADA' && d.tipo !== 'SALIDA') campos.tipo = 'Indica si es entrada o salida.';
  if (!placaValida_(placa)) campos.placa = 'La placa debe tener entre 4 y 10 letras o números.';
  largo_(campos, 'nombrePersona', d.nombrePersona, 100);
  largo_(campos, 'motivo', d.motivo, 200);
  largo_(campos, 'observaciones', d.observaciones, 500);
  if (Object.keys(campos).length) validacion_(campos);

  if (usuario.casetaId && d.casetaId && d.casetaId !== usuario.casetaId) {
    throw errorApi_('PERMISO_DENEGADO', 'Solo puedes registrar en tu caseta asignada.');
  }
  var caseta = casetaActiva_(d.casetaId || usuario.casetaId);

  var v = leer_('Vehiculos', aVehiculoFila_).filter(function (x) { return x.placa === placa; })[0];
  var p = v ? mapaPersonas_()[v.personaId] : null;
  var ultimo = ultimoDe_(leer_('Movimientos', aMovimiento_), placa);
  var entradaAbierta = ultimo && ultimo.tipo === 'ENTRADA' ? ultimo : null;

  // Datos de la persona, en el orden del contrato §7.2.
  var persona;
  if (p) {
    persona = { tipoPersona: p.tipo, personaId: p.id, nombrePersona: p.nombre, motivo: limpio_(d.motivo) };
  } else if (d.tipo === 'SALIDA' && entradaAbierta) {
    persona = { tipoPersona: entradaAbierta.tipoPersona, personaId: null, nombrePersona: entradaAbierta.nombrePersona, motivo: entradaAbierta.motivo };
  } else {
    if (TIPOS_PERSONA_.indexOf(d.tipoPersona) < 0) validacion_({ tipoPersona: 'Selecciona el tipo de usuario.' });
    persona = { tipoPersona: d.tipoPersona, personaId: null, nombrePersona: limpio_(d.nombrePersona), motivo: limpio_(d.motivo) };
  }

  var ahora = new Date();
  var m = {
    id: nuevoId_('mov'),
    tipo: d.tipo,
    placa: placa,
    fechaHora: aFechaHora_(ahora),
    casetaId: caseta.id,
    casetaNombre: caseta.nombre,
    tipoPersona: persona.tipoPersona,
    personaId: persona.personaId,
    nombrePersona: persona.nombrePersona,
    vehiculoRegistrado: Boolean(v),
    autorizado: Boolean(v && v.autorizado),
    motivo: persona.motivo,
    observaciones: limpio_(d.observaciones),
    registradoPor: usuario.correo,
  };

  var advertencias = advertenciasDePlaca_(placa, v);
  if (d.tipo === 'ENTRADA' && entradaAbierta) advertencias.push({ codigo: 'ENTRADA_DUPLICADA', mensaje: 'Este vehículo ya tenía una entrada sin salida.' });
  if (d.tipo === 'SALIDA' && !entradaAbierta) advertencias.push({ codigo: 'SALIDA_SIN_ENTRADA', mensaje: 'No hay una entrada registrada para este vehículo.' });

  agregar_('Movimientos', m);
  return {
    movimiento: m,
    advertencias: advertencias,
    permanenciaMinutos: d.tipo === 'SALIDA' && entradaAbierta ? minutosEntre_(entradaAbierta.fechaHora, ahora.getTime()) : null,
  };
}

function buscarMovimientos_(f) {
  validarFechas_(f.desde, f.hasta, false);
  var placa = f.placa ? normalizarPlaca_(f.placa) : '';
  var lista = leer_('Movimientos', aMovimiento_)
    .filter(function (m) {
      return (!placa || m.placa.indexOf(placa) >= 0) &&
        enPeriodo_(m, f.desde, f.hasta) &&
        (!f.casetaId || m.casetaId === f.casetaId) &&
        (!f.tipoPersona || m.tipoPersona === f.tipoPersona) &&
        (!f.tipo || m.tipo === f.tipo);
    })
    .reverse();
  if (f.porPagina === 0) {
    return { elementos: lista.slice(0, 5000), total: lista.length, pagina: 1, porPagina: 0, truncado: lista.length > 5000 };
  }
  var pagina = paginar_(lista, f.pagina, f.porPagina);
  pagina.truncado = false;
  return pagina;
}

function vehiculosDentro_(datos) {
  var ultimos = {};
  leer_('Movimientos', aMovimiento_).forEach(function (m) { ultimos[m.placa] = m; });
  var ahora = Date.now();
  return Object.keys(ultimos)
    .map(function (placa) { return ultimos[placa]; })
    .filter(function (m) { return m.tipo === 'ENTRADA' && (!datos.casetaId || m.casetaId === datos.casetaId); })
    .map(function (m) { return { entrada: m, permanenciaMinutos: minutosEntre_(m.fechaHora, ahora) }; })
    .sort(function (a, b) { return b.permanenciaMinutos - a.permanenciaMinutos; });
}

// ---------- reportes ----------

function reporteResumen_(datos) {
  validarFechas_(datos.desde, datos.hasta, true);
  var agrupacion = datos.agrupacion || 'DIA';
  if (['DIA', 'SEMANA', 'MES'].indexOf(agrupacion) < 0) validacion_({ agrupacion: 'Agrupa por día, semana o mes.' });
  var lista = leer_('Movimientos', aMovimiento_).filter(function (m) {
    return enPeriodo_(m, datos.desde, datos.hasta) && (!datos.casetaId || m.casetaId === datos.casetaId);
  });
  var cuenta = function (xs) {
    return {
      entradas: xs.filter(function (m) { return m.tipo === 'ENTRADA'; }).length,
      salidas: xs.filter(function (m) { return m.tipo === 'SALIDA'; }).length,
    };
  };

  var periodos = [];
  var grupos = {};
  for (var f = datos.desde; f <= datos.hasta; f = sumarDias_(f, 1)) {
    var clave = periodoDe_(f, agrupacion);
    if (!grupos[clave]) { grupos[clave] = []; periodos.push(clave); }
  }
  lista.forEach(function (m) {
    var clave = periodoDe_(fechaDe_(m.fechaHora), agrupacion);
    if (grupos[clave]) grupos[clave].push(m);
  });

  var serie = periodos.map(function (periodo) {
    var c = cuenta(grupos[periodo]);
    return { periodo: periodo, entradas: c.entradas, salidas: c.salidas };
  });
  return {
    totales: cuenta(lista),
    serie: serie,
    porCaseta: leer_('Casetas', aCaseta_).map(function (c) {
      var n = cuenta(lista.filter(function (m) { return m.casetaId === c.id; }));
      return { casetaId: c.id, casetaNombre: c.nombre, entradas: n.entradas, salidas: n.salidas };
    }),
    porTipoPersona: TIPOS_PERSONA_.map(function (t) {
      var n = cuenta(lista.filter(function (m) { return m.tipoPersona === t; }));
      return { tipoPersona: t, entradas: n.entradas, salidas: n.salidas };
    }),
  };
}

function reporteFrecuentes_(datos) {
  validarFechas_(datos.desde, datos.hasta, true);
  var limite = datos.limite === undefined || datos.limite === null ? 10 : datos.limite;
  if (!(limite >= 1 && limite <= 100)) validacion_({ limite: 'Entre 1 y 100.' });
  var grupos = {};
  leer_('Movimientos', aMovimiento_).forEach(function (m) {
    if (m.tipo !== 'ENTRADA' || !enPeriodo_(m, datos.desde, datos.hasta)) return;
    if ((datos.casetaId && m.casetaId !== datos.casetaId) || (datos.tipoPersona && m.tipoPersona !== datos.tipoPersona)) return;
    (grupos[m.placa] = grupos[m.placa] || []).push(m);
  });
  return Object.keys(grupos)
    .map(function (placa) {
      var xs = grupos[placa];
      var ultima = xs[xs.length - 1];
      return {
        placa: placa,
        nombrePersona: ultima.nombrePersona,
        tipoPersona: ultima.tipoPersona,
        vehiculoRegistrado: ultima.vehiculoRegistrado,
        entradas: xs.length,
        ultimaEntrada: ultima.fechaHora,
      };
    })
    .sort(function (a, b) { return b.entradas - a.entradas || b.ultimaEntrada.localeCompare(a.ultimaEntrada); })
    .slice(0, limite);
}

function reporteAfluencia_(datos) {
  validarFechas_(datos.desde, datos.hasta, true);
  var porHora = [];
  for (var h = 0; h < 24; h++) porHora.push({ hora: h, entradas: 0, salidas: 0 });
  var porDiaSemana = [];
  var matriz = [];
  for (var d = 1; d <= 7; d++) {
    porDiaSemana.push({ dia: d, entradas: 0, salidas: 0 });
    var fila = [];
    for (var j = 0; j < 24; j++) fila.push(0);
    matriz.push(fila);
  }
  leer_('Movimientos', aMovimiento_).forEach(function (m) {
    if (!enPeriodo_(m, datos.desde, datos.hasta) || (datos.casetaId && m.casetaId !== datos.casetaId)) return;
    var hora = horaDe_(m.fechaHora);
    var dia = diaSemana_(fechaDe_(m.fechaHora));
    var clave = m.tipo === 'ENTRADA' ? 'entradas' : 'salidas';
    porHora[hora][clave]++;
    porDiaSemana[dia - 1][clave]++;
    if (!datos.tipo || m.tipo === datos.tipo) matriz[dia - 1][hora]++;
  });
  var pico = null;
  matriz.forEach(function (fila, i) {
    fila.forEach(function (total, hora) {
      if (total > 0 && (!pico || total > pico.total)) pico = { dia: i + 1, hora: hora, total: total };
    });
  });
  return { porHora: porHora, porDiaSemana: porDiaSemana, matriz: matriz, pico: pico };
}

function periodoDe_(fecha, agrupacion) {
  if (agrupacion === 'MES') return fecha.slice(0, 7);
  if (agrupacion === 'SEMANA') return sumarDias_(fecha, 1 - diaSemana_(fecha));
  return fecha;
}

// ---------- casetas ----------

function listarCasetas_(datos) {
  return leer_('Casetas', aCaseta_)
    .filter(function (c) { return datos.incluirInactivas || c.activa; })
    .sort(porNombre_);
}

function crearCaseta_(datos) {
  var campos = {};
  if (!limpio_(datos.nombre)) campos.nombre = 'Escribe el nombre de la caseta.';
  largo_(campos, 'nombre', datos.nombre, 100);
  largo_(campos, 'ubicacion', datos.ubicacion, 100);
  if (Object.keys(campos).length) validacion_(campos);
  var c = { id: nuevoId_('cas'), nombre: limpio_(datos.nombre), ubicacion: limpio_(datos.ubicacion), activa: true };
  agregar_('Casetas', c);
  return c;
}

function actualizarCaseta_(datos) {
  var x = filas_('Casetas', aCaseta_).filter(function (f) { return f.d.id === datos.id; })[0];
  if (!x) throw errorApi_('NO_ENCONTRADO', 'La caseta no existe.');
  var campos = {};
  if (datos.nombre !== undefined && !limpio_(datos.nombre)) campos.nombre = 'Escribe el nombre de la caseta.';
  largo_(campos, 'nombre', datos.nombre, 100);
  largo_(campos, 'ubicacion', datos.ubicacion, 100);
  if (Object.keys(campos).length) validacion_(campos);
  var c = x.d;
  if (datos.nombre !== undefined) c.nombre = limpio_(datos.nombre);
  if (datos.ubicacion !== undefined) c.ubicacion = limpio_(datos.ubicacion);
  if (datos.activa !== undefined) c.activa = Boolean(datos.activa);
  reescribir_('Casetas', x.fila, c);
  return c;
}

function casetaActiva_(id, campo) {
  var c = leer_('Casetas', aCaseta_).filter(function (x) { return x.id === id && x.activa; })[0];
  if (!c) {
    var campos = {};
    campos[campo || 'casetaId'] = 'Selecciona una caseta activa.';
    validacion_(campos);
  }
  return c;
}

// ---------- personas ----------

function listarPersonas_(f) {
  var lista = leer_('Personas', aPersona_)
    .filter(function (p) {
      return (f.incluirInactivos || p.activo) && (!f.tipo || p.tipo === f.tipo) &&
        (!f.texto || contiene_(p.nombre, f.texto) || contiene_(p.identificador, f.texto));
    })
    .sort(porNombre_);
  return paginar_(lista, f.pagina, f.porPagina);
}

function crearPersona_(datos) {
  var personas = leer_('Personas', aPersona_);
  var v = validarPersona_(datos, personas, null);
  var p = { id: nuevoId_('per'), nombre: v.nombre, tipo: v.tipo, identificador: v.identificador, telefono: v.telefono, activo: true, creadoEn: ahora_() };
  agregar_('Personas', p);
  return p;
}

function actualizarPersona_(datos) {
  var filas = filas_('Personas', aPersona_);
  var x = filas.filter(function (f) { return f.d.id === datos.id; })[0];
  if (!x) throw errorApi_('NO_ENCONTRADO', 'La persona no existe.');
  var p = x.d;
  var v = validarPersona_({
    nombre: datos.nombre !== undefined ? datos.nombre : p.nombre,
    tipo: datos.tipo !== undefined ? datos.tipo : p.tipo,
    identificador: datos.identificador !== undefined ? datos.identificador : p.identificador,
    telefono: datos.telefono !== undefined ? datos.telefono : p.telefono,
  }, filas.map(function (f) { return f.d; }), p.id);
  p.nombre = v.nombre;
  p.tipo = v.tipo;
  p.identificador = v.identificador;
  p.telefono = v.telefono;
  if (datos.activo !== undefined) p.activo = Boolean(datos.activo);
  reescribir_('Personas', x.fila, p);
  return p;
}

function validarPersona_(d, personas, idActual) {
  var campos = {};
  var nombre = limpio_(d.nombre);
  var identificador = limpio_(d.identificador);
  if (!nombre) campos.nombre = 'Escribe el nombre.';
  largo_(campos, 'nombre', d.nombre, 100);
  if (TIPOS_PERSONA_.indexOf(d.tipo) < 0) campos.tipo = 'Selecciona el tipo.';
  if (d.tipo && d.tipo !== 'VISITANTE' && !identificador) campos.identificador = 'Escribe la matrícula o el número de empleado.';
  if (Object.keys(campos).length) validacion_(campos);
  var repetida = identificador && personas.some(function (p) { return p.identificador === identificador && p.id !== idActual; });
  if (repetida) {
    throw errorApi_('DUPLICADO', 'Ya hay una persona con ese identificador.', { identificador: 'Ya hay una persona con ese identificador.' });
  }
  return { nombre: nombre, tipo: d.tipo, identificador: identificador, telefono: limpio_(d.telefono) };
}

// ---------- vehículos ----------

function listarVehiculos_(f) {
  var personas = mapaPersonas_();
  var texto = f.texto ? String(f.texto) : '';
  var placaTexto = normalizarPlaca_(texto);
  var lista = leer_('Vehiculos', aVehiculoFila_)
    .map(function (v) { return aVehiculo_(v, personas); })
    .filter(function (v) {
      return (!f.personaId || v.personaId === f.personaId) &&
        (!f.tipoPersona || v.tipoPersona === f.tipoPersona) &&
        (f.autorizado === undefined || f.autorizado === null || v.autorizado === f.autorizado) &&
        (!texto || (placaTexto && v.placa.indexOf(placaTexto) >= 0) || contiene_(v.nombrePersona, texto) || contiene_(v.marca, texto) || contiene_(v.modelo, texto));
    })
    .sort(function (a, b) { return a.nombrePersona.localeCompare(b.nombrePersona, 'es'); });
  return paginar_(lista, f.pagina, f.porPagina);
}

function crearVehiculo_(d) {
  var placa = normalizarPlaca_(d.placa);
  var campos = {};
  if (!placaValida_(placa)) campos.placa = 'La placa debe tener entre 4 y 10 letras o números.';
  largo_(campos, 'marca', d.marca, 40);
  largo_(campos, 'modelo', d.modelo, 40);
  largo_(campos, 'color', d.color, 40);
  if (Object.keys(campos).length) validacion_(campos);
  if (leer_('Vehiculos', aVehiculoFila_).some(function (v) { return v.placa === placa; })) {
    throw errorApi_('DUPLICADO', 'Ya existe un vehículo con esa placa.', { placa: 'Ya existe un vehículo con la placa ' + placa + '.' });
  }
  var personas = mapaPersonas_();
  validarDueno_(d.personaId, personas);
  var v = {
    placa: placa,
    personaId: d.personaId,
    marca: limpio_(d.marca),
    modelo: limpio_(d.modelo),
    color: limpio_(d.color),
    autorizado: d.autorizado === undefined || d.autorizado === null ? true : Boolean(d.autorizado),
    creadoEn: ahora_(),
  };
  agregar_('Vehiculos', v);
  return aVehiculo_(v, personas);
}

function actualizarVehiculo_(d) {
  var placa = normalizarPlaca_(d.placa);
  var x = filas_('Vehiculos', aVehiculoFila_).filter(function (f) { return f.d.placa === placa; })[0];
  if (!x) throw errorApi_('NO_ENCONTRADO', 'El vehículo no existe.');
  var campos = {};
  largo_(campos, 'marca', d.marca, 40);
  largo_(campos, 'modelo', d.modelo, 40);
  largo_(campos, 'color', d.color, 40);
  if (Object.keys(campos).length) validacion_(campos);
  var personas = mapaPersonas_();
  if (d.personaId !== undefined) validarDueno_(d.personaId, personas);
  var v = x.d;
  if (d.personaId !== undefined) v.personaId = d.personaId;
  if (d.marca !== undefined) v.marca = limpio_(d.marca);
  if (d.modelo !== undefined) v.modelo = limpio_(d.modelo);
  if (d.color !== undefined) v.color = limpio_(d.color);
  if (d.autorizado !== undefined) v.autorizado = Boolean(d.autorizado);
  reescribir_('Vehiculos', x.fila, v);
  return aVehiculo_(v, personas);
}

function validarDueno_(personaId, personas) {
  var p = personas[personaId];
  if (!p || !p.activo) validacion_({ personaId: 'Selecciona una persona activa.' });
}

// ---------- usuarios ----------

function listarUsuarios_(datos) {
  return leer_('Usuarios', aUsuario_)
    .filter(function (u) { return datos.incluirInactivos || u.activo; })
    .sort(porNombre_);
}

function crearUsuario_(d) {
  var correo = String(d.correo || '').trim().toLowerCase();
  var campos = {};
  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(correo)) campos.correo = 'Escribe un correo válido.';
  if (!limpio_(d.nombre)) campos.nombre = 'Escribe el nombre.';
  largo_(campos, 'nombre', d.nombre, 100);
  if (ROLES_.indexOf(d.rol) < 0) campos.rol = 'Selecciona un rol.';
  if (Object.keys(campos).length) validacion_(campos);
  if (leer_('Usuarios', aUsuario_).some(function (u) { return u.correo === correo; })) {
    throw errorApi_('DUPLICADO', 'Ese correo ya tiene usuario.', { correo: 'Ese correo ya tiene usuario.' });
  }
  if (d.casetaId) casetaActiva_(d.casetaId);
  var u = { correo: correo, nombre: limpio_(d.nombre), rol: d.rol, casetaId: d.casetaId || null, activo: true };
  agregar_('Usuarios', u);
  return u;
}

function actualizarUsuario_(d, actual) {
  var filas = filas_('Usuarios', aUsuario_);
  var correo = String(d.correo || '').trim().toLowerCase();
  var x = filas.filter(function (f) { return f.d.correo === correo; })[0];
  if (!x) throw errorApi_('NO_ENCONTRADO', 'El usuario no existe.');
  var u = x.d;
  if (d.activo === false && u.correo === actual.correo) throw errorApi_('CONFLICTO', 'No puedes desactivar tu propia cuenta.');
  var dejaDeSerAdmin = u.rol === 'ADMIN' && u.activo && ((d.rol && d.rol !== 'ADMIN') || d.activo === false);
  var adminsActivos = filas.filter(function (f) { return f.d.rol === 'ADMIN' && f.d.activo; }).length;
  if (dejaDeSerAdmin && adminsActivos === 1) throw errorApi_('CONFLICTO', 'El sistema necesita al menos un administrador activo.');
  var campos = {};
  if (d.nombre !== undefined && !limpio_(d.nombre)) campos.nombre = 'Escribe el nombre.';
  largo_(campos, 'nombre', d.nombre, 100);
  if (d.rol !== undefined && ROLES_.indexOf(d.rol) < 0) campos.rol = 'Selecciona un rol.';
  if (Object.keys(campos).length) validacion_(campos);
  if (d.casetaId) casetaActiva_(d.casetaId);
  if (d.nombre !== undefined) u.nombre = limpio_(d.nombre);
  if (d.rol !== undefined) u.rol = d.rol;
  if (d.casetaId !== undefined) u.casetaId = d.casetaId || null;
  if (d.activo !== undefined) u.activo = Boolean(d.activo);
  reescribir_('Usuarios', x.fila, u);
  return u;
}

// ---------- bitácora ----------

function listarBitacora_(f) {
  var lista = leer_('Bitacora', aBitacora_)
    .filter(function (b) {
      var fecha = fechaDe_(b.fechaHora);
      return (!f.desde || fecha >= f.desde) && (!f.hasta || fecha <= f.hasta) &&
        (!f.usuario || contiene_(b.usuario, f.usuario)) && (!f.accion || b.accion === f.accion);
    })
    .reverse();
  return paginar_(lista, f.pagina, f.porPagina);
}

/** La interfaz genera el archivo; esta acción solo existe para que la exportación quede en la bitácora. */
function registrarExportacion_(datos) {
  if (['XLSX', 'PDF'].indexOf(datos.formato) < 0) validacion_({ formato: 'Formato no válido.' });
  return null;
}

// ---------- validaciones comunes ----------

function largo_(campos, nombre, valor, maximo) {
  if (valor !== null && valor !== undefined && String(valor).trim().length > maximo && !campos[nombre]) {
    campos[nombre] = 'Máximo ' + maximo + ' caracteres.';
  }
}

function enPeriodo_(m, desde, hasta) {
  var f = fechaDe_(m.fechaHora);
  return (!desde || f >= desde) && (!hasta || f <= hasta);
}

/** Valida "YYYY-MM-DD". Con `obligatorio`, exige ambas fechas y un rango de 366 días como máximo (§7.3). */
function validarFechas_(desde, hasta, obligatorio) {
  var formato = /^\d{4}-\d{2}-\d{2}$/;
  var campos = {};
  if (obligatorio && !desde) campos.desde = 'Indica el periodo.';
  if (obligatorio && !hasta) campos.hasta = 'Indica el periodo.';
  if (desde && !formato.test(desde)) campos.desde = 'Fecha no válida.';
  if (hasta && !formato.test(hasta)) campos.hasta = 'Fecha no válida.';
  if (Object.keys(campos).length) validacion_(campos);
  if (desde && hasta && desde > hasta) validacion_({ hasta: 'Debe ser igual o posterior a la fecha de inicio.' });
  if (obligatorio && diasEntre_(desde, hasta) > 366) validacion_({ desde: 'El periodo máximo es de 366 días.' });
}

function paginar_(lista, pagina, porPagina) {
  pagina = pagina === undefined || pagina === null ? 1 : pagina;
  porPagina = porPagina === undefined || porPagina === null ? 50 : porPagina;
  if (!(pagina >= 1)) validacion_({ pagina: 'La página empieza en 1.' });
  if (!(porPagina >= 1 && porPagina <= 200)) validacion_({ porPagina: 'Entre 1 y 200 por página.' });
  return {
    elementos: lista.slice((pagina - 1) * porPagina, pagina * porPagina),
    total: lista.length,
    pagina: pagina,
    porPagina: porPagina,
  };
}
