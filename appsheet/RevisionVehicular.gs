/** @OnlyCurrentDoc */

const COLUMNAS_REGISTRO_ = {
  Alumnos: ['Correo', 'Matricula', 'NombreCompleto', 'Activo'],
  Solicitudes: ['SolicitudId', 'Correo', 'FechaEnvio', 'Placa', 'NIV', 'FotoINE', 'FotoINEReverso', 'FotoTarjeta', 'AvisoLeido'],
  Revisiones: ['SolicitudId', 'Identidad', 'INE', 'Tarjeta', 'REPUVE', 'NIVConsultado', 'FechaREPUVE', 'Decision', 'Revisor', 'FechaPublicacion'],
  Resultados: ['SolicitudId', 'Correo', 'Estado', 'Indicacion', 'ActualizadoEn'],
};

const OPCIONES_REVISION_ = {
  Identidad: ['COINCIDE', 'NO_COINCIDE', 'SIN_CONFIRMAR'],
  INE: ['VALIDADA', 'NO_VIGENTE', 'SIN_CONFIRMAR'],
  Tarjeta: ['VALIDADA', 'INCONSISTENTE', 'SIN_CONFIRMAR'],
  REPUVE: ['SIN_REPORTE', 'CON_REPORTE', 'RECUPERADO', 'NO_ENCONTRADO', 'NO_DISPONIBLE', 'DATOS_NO_COINCIDEN'],
  Decision: ['EN_REVISION', 'DEVOLVER', 'APROBAR'],
};

function onOpen() {
  SpreadsheetApp.getUi().createMenu('Registro vehicular')
    .addItem('Abrir revisión de la solicitud', 'abrirRevisionVehicular')
    .addItem('Publicar resultado seleccionado', 'publicarRevisionVehicular')
    .addToUi();
}

function abrirRevisionVehicular() {
  ejecutarRevisionVehicular_('Solicitudes', (libro, fila, revisor) => {
    const solicitudes = leerTablaVehicular_(libro, 'Solicitudes');
    const solicitud = leerFilaSeleccionada_(solicitudes, fila);
    buscarUnicoVehicular_(solicitudes, 'SolicitudId', solicitud.SolicitudId);
    validarSolicitudVehicular_(libro, solicitud);

    const revisiones = leerTablaVehicular_(libro, 'Revisiones');
    const previa = buscarUnicoVehicular_(revisiones, 'SolicitudId', solicitud.SolicitudId);
    if (previa) return { hoja: revisiones.hoja, fila: previa.fila };

    const nuevaFila = revisiones.hoja.getLastRow() + 1;
    if (nuevaFila > revisiones.hoja.getMaxRows()) {
      revisiones.hoja.insertRowAfter(revisiones.hoja.getMaxRows());
    }
    for (const [campo, opciones] of Object.entries(OPCIONES_REVISION_)) {
      const regla = SpreadsheetApp.newDataValidation()
        .requireValueInList(opciones, true)
        .setAllowInvalid(false)
        .build();
      revisiones.hoja.getRange(nuevaFila, revisiones.columnas.indexOf(campo) + 1).setDataValidation(regla);
    }
    escribirFilaVehicular_(revisiones, nuevaFila, {
      SolicitudId: solicitud.SolicitudId,
      Decision: 'EN_REVISION',
      Revisor: revisor,
    });
    return { hoja: revisiones.hoja, fila: nuevaFila };
  });
}

function publicarRevisionVehicular() {
  ejecutarRevisionVehicular_('Revisiones', (libro, fila, revisor) => {
    const revisiones = leerTablaVehicular_(libro, 'Revisiones');
    const revision = leerFilaSeleccionada_(revisiones, fila);
    buscarUnicoVehicular_(revisiones, 'SolicitudId', revision.SolicitudId);
    const solicitudes = leerTablaVehicular_(libro, 'Solicitudes');
    const solicitud = buscarUnicoVehicular_(solicitudes, 'SolicitudId', revision.SolicitudId);
    if (!solicitud) throw new Error('La solicitud vinculada ya no existe.');
    validarSolicitudVehicular_(libro, solicitud.datos);

    const ahora = new Date();
    const resultado = resolverRevisionVehicular_(solicitud.datos, revision, ahora);
    const resultados = leerTablaVehicular_(libro, 'Resultados');
    const anterior = buscarUnicoVehicular_(resultados, 'SolicitudId', revision.SolicitudId);
    if (anterior?.datos.Estado === 'APROBADA') {
      throw new Error('La solicitud ya está aprobada. Corrige el expediente con el responsable del registro.');
    }
    if (resultado.Estado === 'APROBADA') {
      comprobarVehiculoRegistrado_(resultados, solicitudes, solicitud.datos);
    }

    escribirFilaVehicular_(resultados, anterior?.fila, {
      SolicitudId: revision.SolicitudId,
      Correo: solicitud.datos.Correo,
      Estado: resultado.Estado,
      Indicacion: resultado.Indicacion,
      ActualizadoEn: ahora,
    });
    revisiones.hoja.getRange(fila, revisiones.columnas.indexOf('Revisor') + 1, 1, 2)
      .setValues([[revisor, ahora]]);
    return { mensaje: `Resultado publicado: ${resultado.Estado}.` };
  });
}

function ejecutarRevisionVehicular_(nombreHoja, operacion) {
  const libro = SpreadsheetApp.getActiveSpreadsheet();
  const ui = SpreadsheetApp.getUi();
  const seleccion = libro.getActiveRange();
  const correo = Session.getActiveUser().getEmail().trim().toLowerCase();
  if (!correo || !seleccion || seleccion.getSheet().getName() !== nombreHoja ||
      seleccion.getNumRows() !== 1 || seleccion.getRow() < 2) {
    ui.alert(`Usa tu cuenta institucional y selecciona una sola fila de datos en ${nombreHoja}.`);
    return;
  }

  const candado = LockService.getDocumentLock();
  if (!candado.tryLock(5000)) {
    ui.alert('Hay otra revisión en curso. Vuelve a intentar.');
    return;
  }
  let salida;
  try {
    salida = operacion(libro, seleccion.getRow(), correo);
    SpreadsheetApp.flush();
  } catch (error) {
    salida = { mensaje: error.message };
  } finally {
    candado.releaseLock();
  }

  if (salida.hoja) salida.hoja.activate().setActiveSelection(`A${salida.fila}`);
  if (salida.mensaje) ui.alert(salida.mensaje);
}

function leerTablaVehicular_(libro, nombre) {
  const hoja = libro.getSheetByName(nombre);
  const columnas = COLUMNAS_REGISTRO_[nombre];
  const numColumnas = hoja?.getLastColumn() ?? 0;
  if (numColumnas < columnas.length || (nombre !== 'Alumnos' && numColumnas !== columnas.length)) {
    throw new Error(`Faltan columnas o la pestaña ${nombre}. Usa la estructura de la plantilla.`);
  }
  const [encabezado, ...valores] = hoja.getRange(1, 1, hoja.getLastRow(), columnas.length).getValues();
  if (columnas.some((columna, indice) => encabezado[indice] !== columna)) {
    throw new Error(`El encabezado de ${nombre} no coincide con la plantilla.`);
  }
  const registros = valores.map((fila, indice) => ({
    fila: indice + 2,
    datos: Object.fromEntries(columnas.map((columna, j) => [columna, fila[j]])),
  }));
  return { hoja, columnas, registros };
}

function leerFilaSeleccionada_(tabla, fila) {
  const datos = tabla.registros[fila - 2]?.datos;
  if (!datos?.SolicitudId) throw new Error('Selecciona una fila con una solicitud.');
  return datos;
}

function buscarUnicoVehicular_(tabla, columna, valor) {
  const registros = tabla.registros.filter(({ datos }) => datos[columna] === valor);
  if (registros.length > 1) throw new Error(`Hay claves duplicadas en ${tabla.hoja.getName()}.`);
  return registros[0];
}

function escribirFilaVehicular_(tabla, fila, datos) {
  const valores = tabla.columnas.map((columna) => datos[columna] ?? '');
  if (fila) {
    tabla.hoja.getRange(fila, 1, 1, valores.length).setValues([valores]);
  } else {
    tabla.hoja.appendRow(valores);
  }
}

function placaVehicular_(valor) {
  return String(valor).toUpperCase().replace(/[\s.-]/g, '');
}

function nivVehicular_(valor) {
  return String(valor).toUpperCase().trim();
}

function validarSolicitudVehicular_(libro, solicitud) {
  if (!/^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$/i.test(solicitud.SolicitudId)) {
    throw new Error('El folio de la solicitud no es válido.');
  }
  const alumnos = leerTablaVehicular_(libro, 'Alumnos');
  const alumno = buscarUnicoVehicular_(alumnos, 'Correo', solicitud.Correo)?.datos;
  if (alumno?.Activo !== true || !alumno.Matricula || !alumno.NombreCompleto) {
    throw new Error('La cuenta no corresponde a un alumno activo con datos completos en el padrón.');
  }
  if (solicitud.AvisoLeido !== true || !solicitud.FotoINE || !solicitud.FotoTarjeta) {
    throw new Error('Faltan las dos fotos requeridas o la confirmación de lectura del aviso.');
  }
}

function resolverRevisionVehicular_(solicitud, revision, ahora) {
  for (const [campo, opciones] of Object.entries(OPCIONES_REVISION_)) {
    if (revision[campo] && !opciones.includes(revision[campo])) {
      throw new Error(`El valor de ${campo} no está en sus opciones.`);
    }
  }

  if (revision.Decision === 'EN_REVISION') {
    return { Estado: 'EN_REVISION', Indicacion: 'El equipo está verificando tu solicitud.' };
  }

  const placaValida = /^[A-Z0-9]{4,10}$/.test(placaVehicular_(solicitud.Placa));
  const niv = nivVehicular_(solicitud.NIV);
  const nivValido = /^[A-HJ-NPR-Z0-9]{17}$/.test(niv);
  if (revision.Decision === 'DEVOLVER') {
    const indicaciones = [];
    if (revision.Identidad !== 'COINCIDE' || revision.INE !== 'VALIDADA') {
      indicaciones.push('Verifica que tu INE vigente sea legible y que el nombre coincida con tu padrón escolar.');
    }
    if (!placaValida || !nivValido || revision.Tarjeta !== 'VALIDADA' || revision.REPUVE === 'DATOS_NO_COINCIDEN') {
      indicaciones.push('Revisa que la placa y el NIV coincidan con la tarjeta de circulación vigente.');
    }
    return {
      Estado: 'REQUIERE_ATENCION',
      Indicacion: indicaciones.join(' ') || 'Contacta al equipo de registro vehicular para revisar el expediente.',
    };
  }

  if (revision.Decision !== 'APROBAR') throw new Error('Selecciona la decisión del equipo revisor.');
  const fecha = revision.FechaREPUVE;
  if (!placaValida || !nivValido || revision.Identidad !== 'COINCIDE' || revision.INE !== 'VALIDADA' ||
      revision.Tarjeta !== 'VALIDADA' || revision.REPUVE !== 'SIN_REPORTE' ||
      nivVehicular_(revision.NIVConsultado) !== niv || !(fecha instanceof Date) ||
      !Number.isFinite(fecha.getTime()) || fecha > ahora) {
    throw new Error('Para aprobar, confirma identidad, INE, tarjeta y una consulta REPUVE sin reporte para el mismo NIV, con fecha válida.');
  }
  return { Estado: 'APROBADA', Indicacion: 'Tu registro vehicular fue aprobado.' };
}

function comprobarVehiculoRegistrado_(resultados, solicitudes, candidata) {
  const solicitudesPorId = new Map();
  for (const { datos } of solicitudes.registros) {
    if (!datos.SolicitudId) continue;
    if (solicitudesPorId.has(datos.SolicitudId)) throw new Error('Hay claves duplicadas en Solicitudes.');
    solicitudesPorId.set(datos.SolicitudId, datos);
  }
  for (const { datos } of resultados.registros) {
    if (datos.Estado !== 'APROBADA' || datos.SolicitudId === candidata.SolicitudId) continue;
    const aprobada = solicitudesPorId.get(datos.SolicitudId);
    if (!aprobada) throw new Error('Falta la solicitud de un registro aprobado. Revisa la integridad de la hoja.');
    if (placaVehicular_(aprobada.Placa) === placaVehicular_(candidata.Placa) ||
        nivVehicular_(aprobada.NIV) === nivVehicular_(candidata.NIV)) {
      throw new Error('Ya hay una solicitud aprobada con esa placa o NIV. Revisa el registro previo.');
    }
  }
}
