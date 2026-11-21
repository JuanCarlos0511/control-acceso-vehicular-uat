# Contrato de API — v1.0

Acuerdo entre frontend y backend sobre **qué acciones existen, qué reciben, qué devuelven y quién puede usarlas**. Frontend y backend se construyen contra este documento, no contra el código del otro lado.

**Reglas de cambio**
- Este archivo es la fuente de verdad. Si el código y el contrato no coinciden, el que está mal es el código.
- Todo cambio va en un commit que modifique este archivo y que revisen ambos lados.
- Agregar campos opcionales o acciones nuevas → versión menor (1.1). Quitar o renombrar campos, o cambiar tipos → acuerdo explícito y versión mayor.
- Registrar cada cambio en el [historial](#12-historial-de-cambios).

---

## 1. Transporte

La interfaz se sirve con `HtmlService` y habla con el servidor mediante `google.script.run`. Hay **una sola función pública** en el servidor:

```js
api(solicitud)  // solicitud = { accion: string, datos: object }
```

- `accion` tiene el formato `modulo.verbo` (p. ej. `movimientos.registrar`).
- `datos` es un objeto plano; si la acción no recibe nada, se manda `{}`.
- Siempre devuelve un **sobre** `Respuesta` ([sección 2](#2-sobre-de-respuesta-y-errores)) y nunca lanza excepciones hacia el cliente.

**Cliente**: la única forma de llamar al backend.

```js
function llamarApi(accion, datos = {}) {
  return new Promise((resolve) => {
    google.script.run
      .withSuccessHandler(resolve)
      .withFailureHandler(() => resolve({
        ok: false,
        error: { codigo: 'RED', mensaje: 'No hay conexión con el servidor. Intenta de nuevo.' }
      }))
      .api({ accion, datos });
  });
}

// Uso
const r = await llamarApi('movimientos.registrar', { tipo: 'ENTRADA', placa: 'xyz-123-a' });
if (!r.ok) mostrarError(r.error);
```

**Servidor**: esquema del despachador.

```js
function api(solicitud) {
  try {
    const usuario = usuarioActual_();                  // SIN_IDENTIDAD | NO_REGISTRADO | USUARIO_INACTIVO
    const def = ACCIONES_[solicitud && solicitud.accion]; // { fn, roles, escribe }
    if (!def) throw errorApi_('ACCION_DESCONOCIDA', 'Acción no reconocida.');
    if (!def.roles.includes(usuario.rol)) throw errorApi_('PERMISO_DENEGADO', 'No tienes permiso para esta acción.');
    const datos = def.fn(solicitud.datos || {}, usuario);
    if (def.escribe) registrarBitacora_(usuario.correo, solicitud.accion, 'OK', solicitud.datos);
    return { ok: true, datos: datos === undefined ? null : datos };
  } catch (e) {
    return { ok: false, error: aErrorApi_(e) };        // excepción no prevista → ERROR_INTERNO
  }
}
```

---

## 2. Sobre de respuesta y errores

```ts
type Respuesta<T> =
  | { ok: true;  datos: T }
  | { ok: false; error: ErrorApi };

interface ErrorApi {
  codigo: CodigoError;
  mensaje: string;                  // en español, listo para mostrarse al usuario
  campos?: Record<string, string>;  // solo en VALIDACION y DUPLICADO: { nombreCampo: mensaje }
}
```

| `codigo` | Cuándo ocurre | Qué hace el frontend |
|---|---|---|
| `SIN_IDENTIDAD` | El servidor no pudo obtener el correo del usuario (despliegue o dominio mal configurados) | Pantalla de bloqueo con `mensaje` |
| `NO_REGISTRADO` | El correo no está en la hoja `Usuarios` | Pantalla "Sin acceso"; el `mensaje` incluye el correo detectado |
| `USUARIO_INACTIVO` | El usuario existe pero está desactivado | Pantalla "Sin acceso" |
| `PERMISO_DENEGADO` | El rol no puede ejecutar la acción, o un guardia intenta registrar en otra caseta | Aviso |
| `VALIDACION` | Datos inválidos; detalle en `campos` | Marcar cada campo con su mensaje |
| `NO_ENCONTRADO` | El `id` o la `placa` que se quiere actualizar no existe | Aviso |
| `DUPLICADO` | Placa, correo o identificador ya registrado; detalle en `campos` | Marcar el campo |
| `CONFLICTO` | Regla de negocio (p. ej. desactivar al último ADMIN) | Aviso |
| `OCUPADO` | No se obtuvo el candado de escritura a tiempo (registros simultáneos) | Reintentar una vez y luego avisar |
| `ACCION_DESCONOCIDA` | Nombre de acción mal escrito (error de programación) | `console.error` |
| `ERROR_INTERNO` | Excepción no prevista en el servidor | Aviso genérico |
| `RED` | **Solo lo genera el cliente**: `google.script.run` falló (red, tiempo agotado, sesión de Google) | Aviso + reintentar |

---

## 3. Autenticación y permisos

**Inicio de sesión con cuentas Google institucionales.** No hay acciones de login ni logout.

- El Web App se despliega con **Ejecutar como: Yo** (el dueño) y **Acceso: cualquier usuario del dominio de la UAT**. Google exige iniciar sesión antes de mostrar la app; ese es el "inicio de sesión" del manual.
- En cada llamada, el servidor obtiene el correo con `Session.getActiveUser().getEmail()` y busca al usuario en la hoja `Usuarios`. **El cliente nunca envía el correo del usuario actual.**
- **Requisito de despliegue**: el dueño del script debe pertenecer al **mismo dominio de Google Workspace** que los usuarios. Si no, el correo llega vacío y todas las llamadas responden `SIN_IDENTIDAD`.
- Los usuarios no necesitan acceso a la hoja de cálculo; solo el dueño la edita.
- Cerrar sesión: enlace a `https://accounts.google.com/Logout`.
- Problema conocido: con varias cuentas de Google abiertas en el mismo navegador, `google.script.run` puede fallar. Usar una sola cuenta o una ventana de incógnito.

**Roles**

| Rol | Para quién | Puede |
|---|---|---|
| `ADMIN` | Responsable administrativo | Todo |
| `SUPERVISOR` | Seguridad / coordinación | Solo lectura: consultas, reportes, catálogos, bitácora |
| `GUARDIA` | Personal de caseta | Registrar entradas y salidas, verificar placas, consultar movimientos |

- El servidor aplica los permisos en **cada** llamada (tabla de la [sección 6](#6-catálogo-de-acciones)).
- `app.iniciar` devuelve `permisos`, la lista de acciones que el usuario puede ejecutar. El frontend muestra u oculta menús con esa lista y **no copia la matriz de roles en su código**.

**Bitácora automática.** El despachador registra en `Bitacora`:
- toda acción marcada como "escribe" que termina bien (`resultado: 'OK'`);
- todo `PERMISO_DENEGADO` (`resultado: 'DENEGADO'`);
- cada `app.iniciar` como `ACCESO`.

---

## 4. Convenciones de datos

- **Nombres** en español y `camelCase` (`casetaId`, `tipoPersona`). Valores de enumeraciones en `MAYUSCULAS`.
- **Fechas**: nunca se envían objetos `Date` en ninguna dirección. Si la respuesta de `google.script.run` contiene un `Date`, el cliente recibe `null`.
  - `FechaHora`: texto ISO 8601 con desfase, p. ej. `"2026-11-20T07:42:10-06:00"`. Zona del campus: `America/Monterrey` (UTC−6).
  - `Fecha`: `"YYYY-MM-DD"`, fecha local del campus. En filtros, `desde` y `hasta` son **inclusivos** (`desde` 00:00:00 → `hasta` 23:59:59).
  - La fecha y hora de un movimiento la pone **siempre el servidor**; el cliente nunca la envía.
- **Placas**: el servidor las normaliza a mayúsculas sin espacios, guiones ni puntos (`"xyz-123-a"` → `"XYZ123A"`). Después de normalizar deben tener entre 4 y 10 caracteres `A-Z0-9`. Las respuestas siempre traen la placa normalizada.
- **IDs**: texto opaco; el frontend no debe suponer su formato. La placa es el identificador de un vehículo y el correo, el de un usuario.
- **Campos ausentes y nulos**
  - Las respuestas incluyen siempre todos los campos del tipo (`null` cuando no hay valor).
  - En las solicitudes, los campos marcados con `?` se pueden omitir.
  - En las acciones `actualizar`, un campo omitido no cambia.
  - El servidor recorta espacios en todos los textos; `""` en un campo opcional cuenta como `null`.
- **Borrado**: no se borra nada. Los catálogos se desactivan (`activo` / `activa` / `autorizado` = `false`) para no romper el historial.
- **Orden de los listados**: movimientos y bitácora, del más reciente al más antiguo; catálogos, por nombre ascendente.
- **Paginación**: `pagina` empieza en 1 (por defecto 1); `porPagina` va de 1 a 200 (por defecto 50).
- **Longitudes máximas**: `nombre` 100 · `ubicacion` 100 · `marca`/`modelo`/`color` 40 · `motivo` 200 · `observaciones` 500.

---

## 5. Tipos

```ts
type TipoPersona    = 'ESTUDIANTE' | 'DOCENTE' | 'ADMINISTRATIVO' | 'VISITANTE';
type TipoMovimiento = 'ENTRADA' | 'SALIDA';
type Rol            = 'ADMIN' | 'SUPERVISOR' | 'GUARDIA';
type FechaHora      = string;  // ISO 8601 con desfase
type Fecha          = string;  // 'YYYY-MM-DD'

interface Usuario {          // usuario del sistema
  correo: string;            // PK, en minúsculas
  nombre: string;
  rol: Rol;
  casetaId: string | null;   // caseta asignada (guardias); null = sin caseta fija
  activo: boolean;
}

interface Caseta {
  id: string;
  nombre: string;
  ubicacion: string | null;
  activa: boolean;
}

interface Persona {          // dueño de vehículos: estudiante, empleado o visitante
  id: string;
  nombre: string;
  tipo: TipoPersona;         // "empleado" = DOCENTE o ADMINISTRATIVO
  identificador: string | null;  // matrícula o número de empleado; único si no es null
  telefono: string | null;
  activo: boolean;
  creadoEn: FechaHora;
}

interface Vehiculo {
  placa: string;             // PK, normalizada
  personaId: string;
  nombrePersona: string;     // tomado de la persona, solo lectura
  tipoPersona: TipoPersona;  // tomado de la persona, solo lectura
  marca: string | null;
  modelo: string | null;
  color: string | null;
  autorizado: boolean;
  creadoEn: FechaHora;
}

interface Movimiento {
  id: string;
  tipo: TipoMovimiento;
  placa: string;
  fechaHora: FechaHora;      // la pone el servidor
  casetaId: string;
  casetaNombre: string;
  tipoPersona: TipoPersona;
  personaId: string | null;  // null si el vehículo no está registrado
  nombrePersona: string | null;
  vehiculoRegistrado: boolean;
  autorizado: boolean;       // estado del vehículo en el momento del registro
  motivo: string | null;     // motivo de la visita
  observaciones: string | null;
  registradoPor: string;     // correo de quien registró
}

interface Advertencia {      // avisos que no bloquean: el movimiento sí se guarda
  codigo: 'VEHICULO_NO_REGISTRADO' | 'VEHICULO_NO_AUTORIZADO' | 'ENTRADA_DUPLICADA' | 'SALIDA_SIN_ENTRADA';
  mensaje: string;
}

interface EntradaBitacora {
  fechaHora: FechaHora;
  usuario: string;           // correo
  accion: string;            // nombre de la acción o 'ACCESO'
  resultado: 'OK' | 'DENEGADO';
  detalle: string;           // resumen legible de los datos
}

interface Pagina<T> {
  elementos: T[];
  total: number;             // total de coincidencias sin paginar
  pagina: number;
  porPagina: number;
}
```

---

## 6. Catálogo de acciones

**P1** = indispensable para la demo. **P2** = si da tiempo.

| Acción | ADMIN | SUPERVISOR | GUARDIA | Escribe | Prioridad | RF |
|---|:-:|:-:|:-:|:-:|:-:|---|
| `app.iniciar` | ✓ | ✓ | ✓ | (`ACCESO`) | P1 | RF-015 |
| `movimientos.verificarPlaca` | ✓ | ✓ | ✓ | | P1 | RF-004 |
| `movimientos.registrar` | ✓ | | ✓ | ✓ | P1 | RF-001, RF-002 |
| `movimientos.buscar` | ✓ | ✓ | ✓ | | P1 | RF-003 a RF-007 |
| `movimientos.dentro` | ✓ | ✓ | ✓ | | P2 | Innovación |
| `reportes.resumen` | ✓ | ✓ | | | P1 | RF-011 |
| `reportes.vehiculosFrecuentes` | ✓ | ✓ | | | P1 | RF-012 |
| `reportes.afluencia` | ✓ | ✓ | | | P1 | RF-013 |
| `casetas.listar` | ✓ | ✓ | ✓ | | P1 | RF-010 |
| `casetas.crear` | ✓ | | | ✓ | P2 | RF-010 |
| `casetas.actualizar` | ✓ | | | ✓ | P2 | RF-010 |
| `personas.listar` | ✓ | ✓ | | | P1 | RF-009 |
| `personas.crear` | ✓ | | | ✓ | P1 | RF-009 |
| `personas.actualizar` | ✓ | | | ✓ | P2 | RF-009 |
| `vehiculos.listar` | ✓ | ✓ | | | P1 | RF-008 |
| `vehiculos.crear` | ✓ | | | ✓ | P1 | RF-008 |
| `vehiculos.actualizar` | ✓ | | | ✓ | P2 | RF-008 |
| `usuarios.listar` | ✓ | | | | P1 | RF-016 |
| `usuarios.crear` | ✓ | | | ✓ | P1 | RF-016 |
| `usuarios.actualizar` | ✓ | | | ✓ | P2 | RF-016 |
| `bitacora.listar` | ✓ | ✓ | | | P1 | RF-019 |
| `bitacora.registrarExportacion` | ✓ | ✓ | ✓ | ✓ | P2 | RF-014, RF-018 |

RF-017 (permisos por rol) y RF-018 (bitácora) los cumple el despachador en todas las acciones. RF-014 (exportar a Excel y PDF) se resuelve en el navegador con los datos de `movimientos.buscar` y `reportes.*`; no tiene acción propia.

Las casetas se dan de alta con el `setup()` del backend, por eso crearlas y editarlas es P2.

---

## 7. Detalle por acción

### 7.1 App

#### `app.iniciar`
Primera llamada al abrir la app. Junta en una sola llamada lo que necesita la interfaz para arrancar.

```ts
datos: {}
respuesta: {
  usuario: Usuario;
  permisos: string[];        // acciones que este usuario puede ejecutar
  casetas: Caseta[];         // solo activas
  servidor: { zonaHoraria: string; ahora: FechaHora; versionContrato: string };
}
errores: SIN_IDENTIDAD, NO_REGISTRADO, USUARIO_INACTIVO
```

### 7.2 Movimientos

#### `movimientos.verificarPlaca`
El guardia teclea la placa y ve quién es, si está autorizado y si el vehículo está dentro del campus. No guarda nada.

```ts
datos: { placa: string }
respuesta: {
  placa: string;                       // normalizada
  vehiculo: Vehiculo | null;           // null = no registrado
  persona: Persona | null;
  dentro: boolean;                     // el último movimiento de la placa es una ENTRADA
  ultimoMovimiento: Movimiento | null;
  siguienteTipo: TipoMovimiento;       // SALIDA si dentro, ENTRADA si no
  advertencias: Advertencia[];         // VEHICULO_NO_REGISTRADO | VEHICULO_NO_AUTORIZADO
}
errores: VALIDACION
```

#### `movimientos.registrar`
Registra una entrada o una salida.

```ts
datos: {
  tipo: TipoMovimiento;
  placa: string;
  casetaId?: string;          // ver "Regla de caseta"
  tipoPersona?: TipoPersona;  // ver "Datos de la persona"
  nombrePersona?: string;     // vehículos no registrados (visitantes)
  motivo?: string;
  observaciones?: string;
}
respuesta: {
  movimiento: Movimiento;
  advertencias: Advertencia[];
  permanenciaMinutos: number | null;  // solo en SALIDA con entrada abierta
}
errores: VALIDACION, PERMISO_DENEGADO, OCUPADO
```

**Regla de caseta**
- Usuario con `casetaId` asignada: se puede omitir `casetaId` y se usa la suya. Si envía otra distinta → `PERMISO_DENEGADO`.
- Usuario sin caseta asignada: `casetaId` es obligatorio.
- La caseta debe existir y estar activa; si no → `VALIDACION` en `campos.casetaId`.

**Datos de la persona** (`tipoPersona` y `nombrePersona`), en este orden:
1. Si el vehículo está registrado, se toman de su persona y lo que mande el cliente se ignora.
2. Si no está registrado y es una SALIDA con entrada abierta, se copian de esa entrada (incluido `motivo`).
3. En cualquier otro caso se usa lo enviado. `tipoPersona` es obligatorio (`VALIDACION`); el frontend propone `VISITANTE` por defecto.

**Advertencias**: no bloquean, el movimiento se guarda igual.
- `VEHICULO_NO_REGISTRADO`: la placa no está en el catálogo.
- `VEHICULO_NO_AUTORIZADO`: registrado, pero con `autorizado = false`.
- `ENTRADA_DUPLICADA`: ENTRADA cuando el último movimiento de la placa ya era una ENTRADA.
- `SALIDA_SIN_ENTRADA`: SALIDA sin una entrada abierta.

> Pendiente de validar en entrevistas: si un vehículo no autorizado debe **bloquearse**, se cambia a un error `NO_AUTORIZADO` en la v1.1.

#### `movimientos.buscar`
Una sola acción para todas las consultas; los filtros se combinan con Y.

```ts
datos: {
  placa?: string;             // coincidencia parcial, se normaliza igual que una placa
  desde?: Fecha;
  hasta?: Fecha;
  casetaId?: string;          // consulta por acceso
  tipoPersona?: TipoPersona;  // consulta por tipo de usuario
  tipo?: TipoMovimiento;
  pagina?: number;
  porPagina?: number;         // 0 = sin paginar, para exportar (máx. 5000 filas)
}
respuesta: Pagina<Movimiento> & { truncado: boolean }  // truncado = había más de 5000 filas
errores: VALIDACION
```

#### `movimientos.dentro` (P2)
Vehículos que están dentro del campus en este momento: su último movimiento es una ENTRADA.

```ts
datos: { casetaId?: string }   // filtra por la caseta de la entrada
respuesta: Array<{ entrada: Movimiento; permanenciaMinutos: number }>  // de mayor a menor permanencia
```

### 7.3 Reportes

Las tres acciones exigen `desde` y `hasta`, con un rango máximo de 366 días (si no → `VALIDACION`). Las horas y los días se calculan en la zona del campus.

#### `reportes.resumen`
Entradas y salidas por periodo.

```ts
datos: { desde: Fecha; hasta: Fecha; casetaId?: string; agrupacion?: 'DIA' | 'SEMANA' | 'MES' }  // por defecto DIA
respuesta: {
  totales: { entradas: number; salidas: number };
  serie: Array<{ periodo: string; entradas: number; salidas: number }>;
     // periodo: DIA 'YYYY-MM-DD' · SEMANA fecha del lunes 'YYYY-MM-DD' · MES 'YYYY-MM'
     // incluye periodos con 0 movimientos, en orden cronológico
  porCaseta: Array<{ casetaId: string; casetaNombre: string; entradas: number; salidas: number }>;
  porTipoPersona: Array<{ tipoPersona: TipoPersona; entradas: number; salidas: number }>;
}
```

#### `reportes.vehiculosFrecuentes`
Vehículos con más entradas en el periodo; incluye placas no registradas.

```ts
datos: { desde: Fecha; hasta: Fecha; casetaId?: string; tipoPersona?: TipoPersona; limite?: number }  // limite 1–100, por defecto 10
respuesta: Array<{
  placa: string;
  nombrePersona: string | null;
  tipoPersona: TipoPersona;
  vehiculoRegistrado: boolean;
  entradas: number;
  ultimaEntrada: FechaHora;
}>  // de mayor a menor número de entradas
```

#### `reportes.afluencia`
Horarios de mayor afluencia.

```ts
datos: { desde: Fecha; hasta: Fecha; casetaId?: string; tipo?: TipoMovimiento }  // sin tipo = entradas + salidas
respuesta: {
  porHora: Array<{ hora: number; entradas: number; salidas: number }>;      // 24 elementos, hora 0–23
  porDiaSemana: Array<{ dia: number; entradas: number; salidas: number }>;  // 7 elementos, 1 = lunes … 7 = domingo
  matriz: number[][];                                // [dia − 1][hora], 7 × 24, para un mapa de calor
  pico: { dia: number; hora: number; total: number } | null;  // null si no hay movimientos
}
```

### 7.4 Catálogos

#### Casetas
```ts
casetas.listar     datos: { incluirInactivas?: boolean }                                  → Caseta[]
casetas.crear      datos: { nombre: string; ubicacion?: string }                          → Caseta
casetas.actualizar datos: { id: string; nombre?: string; ubicacion?: string; activa?: boolean } → Caseta
```

#### Personas
```ts
personas.listar     datos: { texto?: string; tipo?: TipoPersona; incluirInactivos?: boolean; pagina?: number; porPagina?: number } → Pagina<Persona>
personas.crear      datos: { nombre: string; tipo: TipoPersona; identificador?: string; telefono?: string } → Persona
personas.actualizar datos: { id: string; nombre?: string; tipo?: TipoPersona; identificador?: string; telefono?: string; activo?: boolean } → Persona
```
- `texto` busca en `nombre` e `identificador`, sin distinguir mayúsculas ni acentos.
- `identificador` es obligatorio salvo para `VISITANTE` (`VALIDACION`) y debe ser único (`DUPLICADO`).

#### Vehículos
```ts
vehiculos.listar     datos: { texto?: string; personaId?: string; tipoPersona?: TipoPersona; autorizado?: boolean; pagina?: number; porPagina?: number } → Pagina<Vehiculo>
vehiculos.crear      datos: { placa: string; personaId: string; marca?: string; modelo?: string; color?: string; autorizado?: boolean } → Vehiculo
vehiculos.actualizar datos: { placa: string; personaId?: string; marca?: string; modelo?: string; color?: string; autorizado?: boolean } → Vehiculo
```
- `texto` busca en placa, nombre de la persona, marca y modelo.
- En `crear`, `autorizado` vale `true` por defecto. Si la placa ya existe → `DUPLICADO`; si `personaId` no existe o la persona está inactiva → `VALIDACION`.
- La placa no se puede cambiar: para eso se desautoriza el vehículo y se da de alta otro.

### 7.5 Usuarios del sistema

```ts
usuarios.listar     datos: { incluirInactivos?: boolean } → Usuario[]
usuarios.crear      datos: { correo: string; nombre: string; rol: Rol; casetaId?: string } → Usuario
usuarios.actualizar datos: { correo: string; nombre?: string; rol?: Rol; casetaId?: string | null; activo?: boolean } → Usuario
```
- El correo se guarda en minúsculas. Si ya existe → `DUPLICADO`.
- Responde `CONFLICTO` si la operación dejaría el sistema sin ningún `ADMIN` activo o si el usuario intenta desactivarse a sí mismo.

### 7.6 Bitácora

```ts
bitacora.listar datos: { desde?: Fecha; hasta?: Fecha; usuario?: string; accion?: string; pagina?: number; porPagina?: number } → Pagina<EntradaBitacora>
bitacora.registrarExportacion datos: { origen: string; formato: 'XLSX' | 'PDF'; filtros: object } → null
```
- `origen` es la acción de la que salieron los datos (p. ej. `"reportes.afluencia"`).
- El frontend llama a `registrarExportacion` después de generar cada archivo, para que la exportación quede en la bitácora.

---

## 8. Ejemplos

Datos ficticios. El frontend puede usarlos como respuestas simuladas mientras el backend no está listo.

**`app.iniciar`** (guardia)
```json
{
  "ok": true,
  "datos": {
    "usuario": { "correo": "guardia1@uat.edu.mx", "nombre": "Juan Pérez", "rol": "GUARDIA", "casetaId": "cas-1", "activo": true },
    "permisos": ["app.iniciar", "movimientos.verificarPlaca", "movimientos.registrar", "movimientos.buscar",
                 "movimientos.dentro", "casetas.listar", "bitacora.registrarExportacion"],
    "casetas": [
      { "id": "cas-1", "nombre": "Caseta 1", "ubicacion": "Acceso principal", "activa": true },
      { "id": "cas-2", "nombre": "Caseta 2", "ubicacion": "Acceso norte", "activa": true },
      { "id": "cas-3", "nombre": "Caseta 3", "ubicacion": "Acceso sur", "activa": true }
    ],
    "servidor": { "zonaHoraria": "America/Monterrey", "ahora": "2026-11-20T07:41:55-06:00", "versionContrato": "1.0" }
  }
}
```

**`movimientos.verificarPlaca`** → `{ "placa": "xyz-123-a" }`
```json
{
  "ok": true,
  "datos": {
    "placa": "XYZ123A",
    "vehiculo": { "placa": "XYZ123A", "personaId": "per-17", "nombrePersona": "María López", "tipoPersona": "ESTUDIANTE",
                  "marca": "Nissan", "modelo": "Versa", "color": "Gris", "autorizado": true, "creadoEn": "2026-08-15T10:00:00-06:00" },
    "persona": { "id": "per-17", "nombre": "María López", "tipo": "ESTUDIANTE", "identificador": "2210123456",
                 "telefono": null, "activo": true, "creadoEn": "2026-08-15T09:58:12-06:00" },
    "dentro": false,
    "ultimoMovimiento": { "id": "mov-880", "tipo": "SALIDA", "placa": "XYZ123A", "fechaHora": "2026-11-19T16:10:03-06:00",
                          "casetaId": "cas-2", "casetaNombre": "Caseta 2", "tipoPersona": "ESTUDIANTE", "personaId": "per-17",
                          "nombrePersona": "María López", "vehiculoRegistrado": true, "autorizado": true,
                          "motivo": null, "observaciones": null, "registradoPor": "guardia2@uat.edu.mx" },
    "siguienteTipo": "ENTRADA",
    "advertencias": []
  }
}
```

**`movimientos.registrar`**: visitante con vehículo no registrado
```json
{ "accion": "movimientos.registrar",
  "datos": { "tipo": "ENTRADA", "placa": "abc 987 b", "tipoPersona": "VISITANTE",
             "nombrePersona": "Carlos Ruiz", "motivo": "Entrega de material a Servicios Escolares" } }
```
```json
{
  "ok": true,
  "datos": {
    "movimiento": { "id": "mov-901", "tipo": "ENTRADA", "placa": "ABC987B", "fechaHora": "2026-11-20T07:45:31-06:00",
                    "casetaId": "cas-1", "casetaNombre": "Caseta 1", "tipoPersona": "VISITANTE", "personaId": null,
                    "nombrePersona": "Carlos Ruiz", "vehiculoRegistrado": false, "autorizado": false,
                    "motivo": "Entrega de material a Servicios Escolares", "observaciones": null,
                    "registradoPor": "guardia1@uat.edu.mx" },
    "advertencias": [{ "codigo": "VEHICULO_NO_REGISTRADO", "mensaje": "La placa ABC987B no está en el catálogo de vehículos." }],
    "permanenciaMinutos": null
  }
}
```

**Error de validación**
```json
{
  "ok": false,
  "error": {
    "codigo": "VALIDACION",
    "mensaje": "Revisa los datos marcados.",
    "campos": { "placa": "La placa debe tener entre 4 y 10 letras o números.", "tipoPersona": "Selecciona el tipo de usuario." }
  }
}
```

**`movimientos.buscar`** → `{ "placa": "xyz", "desde": "2026-11-01", "hasta": "2026-11-20", "porPagina": 50 }`
```json
{ "ok": true, "datos": { "elementos": ["…37 objetos Movimiento, como ultimoMovimiento en verificarPlaca…"], "total": 37, "pagina": 1, "porPagina": 50, "truncado": false } }
```

**`reportes.afluencia`** (recortado)
```json
{
  "ok": true,
  "datos": {
    "porHora": [{ "hora": 0, "entradas": 0, "salidas": 1 }, "…", { "hora": 7, "entradas": 412, "salidas": 15 }, "…"],
    "porDiaSemana": [{ "dia": 1, "entradas": 980, "salidas": 955 }, "…"],
    "matriz": [[0, 0, 0, 0, 0, 2, 40, 180, "…"], "…"],
    "pico": { "dia": 1, "hora": 7, "total": 96 }
  }
}
```

---

## 9. Notas para backend

- Solo `doGet`, `include` y `api` son funciones públicas. Todas las demás terminan en `_`: Apps Script no deja llamar a esas funciones desde `google.script.run`, así nadie puede saltarse la revisión de permisos.
- Antes de responder, convertir a `FechaHora` los `Date` que devuelve `getValues()`: `Utilities.formatDate(d, 'America/Monterrey', "yyyy-MM-dd'T'HH:mm:ssXXX")`.
- Dar formato de texto plano (`@`) a las columnas de placa, identificador y teléfono; si no, Sheets convierte `"0123"` en el número `123`.
- Las hojas pueden devolver `"TRUE"`/`"FALSE"` como texto: convertirlos a booleanos reales.
- Envolver las escrituras en `LockService.getScriptLock()` con espera de unos 10 s; si no se obtiene el candado → `OCUPADO`.
- `setup()` da de alta las 3 casetas y al dueño del script (`Session.getEffectiveUser()`) como `ADMIN`.
- Para probar desde el editor, crear una función que llame a `api({ accion, datos })`. Así la prueba pasa por la misma revisión de permisos y no abre ninguna puerta nueva. Para probar otro rol, cambiar el propio rol en la hoja `Usuarios`.

## 10. Notas para frontend

- Llamar al backend solo a través de `llamarApi`. Nunca usar `google.script.run` directamente.
- Mostrar u ocultar opciones según `permisos`; nunca según el nombre del rol.
- Deshabilitar el botón de enviar mientras haya una llamada pendiente, para evitar registros dobles.
- Mostrar `error.mensaje` tal cual y usar `error.campos` para marcar el formulario.
- Las etiquetas visibles de las enumeraciones (`ESTUDIANTE` → "Estudiante") son responsabilidad del frontend.
- Para exportar: pedir los datos con `porPagina: 0`, avisar si viene `truncado: true`, generar el archivo y luego llamar a `bitacora.registrarExportacion`.
- Cada llamada tarda aproximadamente entre 0.5 y 2 s: cargar lo común con `app.iniciar` y no repetir llamadas sin necesidad.

## 11. Trabajo en paralelo

- **Frontend**: desarrollar con respuestas simuladas basadas en la [sección 8](#8-ejemplos), detrás de la misma firma de `llamarApi`. Al integrar, solo se cambia la implementación de `llamarApi`.
- **Backend**: implementar las acciones P1 primero, en el orden de la tabla; probar cada una con `api(...)` desde el editor.
- **clasp**: `clasp push` **reemplaza todo** el proyecto remoto con los archivos locales. Si dos personas empujan al mismo proyecto, una borra el trabajo de la otra.
  - Cada quien desarrolla en **su propio proyecto de Apps Script**, con su propia copia de la hoja.
  - Hay un proyecto de integración para la demo, al que solo se empuja desde `main`.
  - Por lo mismo, `.clasp.json` no debe versionarse: cada quien tiene el suyo.

## 12. Historial de cambios

| Versión | Fecha | Cambio |
|---|---|---|
| 1.0 | 2026-11-20 | Versión inicial |
