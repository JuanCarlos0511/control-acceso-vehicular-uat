# Backend del registro vehicular estudiantil (AppSheet)

Valida automáticamente cada solicitud que el alumno envía desde la app de AppSheet (`appsheet/`) y guarda el resultado en la base de datos de la app:

0. **Antes, una sola vez**, el alumno confirma que está inscrito iniciando sesión con su cuenta institucional en el frontend (`front-end/registro-alumno`). El backend lo verifica con el portal de alumnos de la UAT mediante [`scrap-uat-service`](https://github.com/Campus-TD/scrap-uat-service) y escribe su fila en `Alumnos` con el nombre oficial (ver [Registro con cuenta institucional](#registro-con-cuenta-institucional)).
1. El bot de AppSheet llama al webhook cuando se agrega una fila en `Solicitudes`.
2. El backend lee la solicitud y al alumno del padrón (`Alumnos`) con la API de AppSheet.
3. Descarga las fotos (INE frente y reverso, tarjeta de circulación) desde Google Drive.
4. El **analizador** (Python) lee las fotos con OCR y verifica la INE: MRZ del reverso con sus dígitos de control, CURP con su dígito verificador, vigencia, consistencia entre frente y reverso y calidad de la foto. De la tarjeta extrae NIV (con dígito verificador ISO 3779), placa, marca, año y vigencia.
5. El analizador consulta **REPUVE** con el NIV en el portal ciudadano, con Playwright: datos del vehículo, reportes de robo (fiscalías y OCRA) y avisos judiciales.
6. Decide con las mismas reglas que `appsheet/RevisionVehicular.gs` y escribe en `Revisiones`, `Resultados` y, si se configura, `Validaciones`.
7. Si la solicitud queda **aprobada**, emite un **pase con código QR** en la tabla `Pases`. El alumno lo ve en AppSheet y lo muestra en la caseta.

```
AppSheet (alumno) ──bot──▶ POST /webhooks/appsheet/solicitudes
                                │
                 ┌──────────────┼─────────────────┬─────────────────────┐
                 ▼              ▼                 ▼                     ▼
          API de AppSheet   Google Drive    Analizador (Python) ──▶ portal REPUVE
          Solicitudes,      fotos           OCR, MRZ, CURP, NIV     (Playwright):
          Alumnos                           y consulta REPUVE       vehículo, robo, avisos
                 │
                 ▼
          Revisiones · Resultados · Validaciones · Pases  ──▶  el alumno ve su estado y su QR al sincronizar
```

## Componentes

| Carpeta | Lenguaje | Qué hace |
|---|---|---|
| `cmd/servidor`, `internal/` | Go 1.26 | API HTTP, webhook, cola de trabajo, cliente de AppSheet y Drive, reglas de decisión y pases QR |
| `analizador/` | Python 3.13 | Servicio FastAPI: análisis de imágenes con RapidOCR (modelos ONNX) y OpenCV, y consulta a REPUVE con Playwright y Chromium |

## Qué se valida y cómo se decide

Cada verificación llena una columna de `Revisiones` con los mismos valores que usa el equipo revisor.

| Columna | Valor | Cuándo |
|---|---|---|
| `Identidad` | `COINCIDE` | El nombre de la INE (MRZ o frente) cubre ≥ 90 % de las palabras del nombre en el padrón, sin importar orden ni acentos, y tolerando 1–2 letras mal leídas |
| | `NO_COINCIDE` | Coincidencia < 50 % con una lectura confiable |
| | `SIN_CONFIRMAR` | Cualquier caso intermedio o sin lectura |
| `INE` | `VALIDADA` | Es una INE, la MRZ del reverso pasa los dígitos de control de documento, nacimiento y expiración, está vigente y frente y reverso son consistentes |
| | `NO_VIGENTE` | La vigencia (MRZ o lectura confiable del frente) ya pasó |
| | `SIN_CONFIRMAR` | Falta el reverso, MRZ inválida, inconsistencias o foto ilegible |
| `Tarjeta` | `VALIDADA` | El NIV de la tarjeta es igual al de la solicitud y la placa coincide (en la tarjeta o en REPUVE) |
| | `INCONSISTENTE` | Se leyó con claridad un NIV o una placa distintos |
| | `SIN_CONFIRMAR` | No se pudo leer o la lectura difiere en 1–2 caracteres |
| `REPUVE` | `SIN_REPORTE`, `CON_REPORTE`, `RECUPERADO`, `NO_ENCONTRADO`, `NO_DISPONIBLE`, `DATOS_NO_COINCIDEN` | Resultado del portal. `DATOS_NO_COINCIDEN` si la placa o el NIV de la solicitud no coinciden con REPUVE |

Decisión (`Decision` en `Revisiones`, `Estado` en `Resultados`):

- **`APROBAR` → `APROBADA`**: las cuatro verificaciones pasan, REPUVE se consultó con el mismo NIV, el alumno está activo, no hay otra solicitud aprobada con esa placa o NIV y nada bloquea (por ejemplo, marca o año distintos a REPUVE). Con `AUTO_APROBAR=false` estas solicitudes quedan `EN_REVISION` con la revisión ya llena, para que una persona publique.
- **`DEVOLVER` → `REQUIERE_ATENCION`**: solo cuando el problema es claro y el alumno lo puede corregir: nombre distinto, INE vencida, NIV o placa distintos, formato inválido o fotos ilegibles. La indicación es la misma que publica el script de revisión.
- **`EN_REVISION`** en todos los demás casos. Un reporte de robo nunca se le muestra al alumno: queda en revisión con la indicación genérica.

Reglas de convivencia con la revisión humana:

- El backend firma `Revisor` con `CORREO_SISTEMA` (por defecto `APPSHEET_RUN_AS`). Si una persona ya publicó o tomó la revisión (otro `Revisor`), el backend no la toca.
- Nunca modifica una solicitud `APROBADA`.
- El menú **Registro vehicular** de la hoja sigue funcionando igual: abre la fila que dejó el backend y, al publicar, la persona queda como revisor.
- No escribe CURP, CIC, OCR, nombres ni fotos en ninguna tabla ni en los logs. La tabla `Validaciones` guarda solo estados, motivos y un resumen del vehículo (marca, modelo, año).

## Pase con código QR

Cuando una solicitud queda `APROBADA`, el backend agrega una fila en `Pases` con el código del pase y la URL de su imagen QR. AppSheet muestra esa imagen en la app del alumno.

- **Cuándo se emite.** Al aprobar automáticamente se emite en el momento. Si la aprobó una persona desde el menú de la hoja, el barrido la detecta y emite el pase en menos de `BARRIDO_INTERVALO`. También se puede pedir con `POST /api/v1/solicitudes/{id}/pase`. Nunca se emite un pase sin aprobación, y revalidar no lo duplica.
- **Qué contiene el QR.** `RV1.<PaseId>.<PLACA>.<AAAAMMDD>.<firma>`. La firma es HMAC-SHA256 con `PASE_SECRETO`. No lleva nombre, CURP ni correo. El guardia compara la placa del QR con la del vehículo, así que una captura de pantalla no sirve en otro coche.
- **Vigencia.** `PASE_DIAS` desde la emisión, o la fecha fija `PASE_VENCE` (por ejemplo, el fin del ciclo escolar).
- **Revocar.** En la hoja, cambia `Estado` del pase a `REVOCADO`. La verificación lo rechaza de inmediato y el barrido no lo vuelve a emitir. Para darle otro, usa `POST /api/v1/solicitudes/{id}/pase`.
- **Imagen.** `GET <URL_PUBLICA>/pases/qr/<codigo>.png` es pública porque la carga el navegador del alumno, pero solo dibuja códigos con firma válida.
- **Verificar en la caseta.** `POST /api/v1/pases/verificar` con `{"codigo": "<lo que leyó el escáner>"}` revisa firma, vigencia, revocación y que la solicitud siga aprobada, y devuelve la placa. Desde Apps Script del sistema principal:

  ```js
  function verificarPase_(codigo) {
    const p = PropertiesService.getScriptProperties();
    const r = UrlFetchApp.fetch(p.getProperty('BACKEND_URL') + '/api/v1/pases/verificar', {
      method: 'post', contentType: 'application/json', muteHttpExceptions: true,
      headers: { Authorization: 'Bearer ' + p.getProperty('BACKEND_TOKEN') },
      payload: JSON.stringify({ codigo: codigo }),
    });
    return JSON.parse(r.getContentText()); // { ok, datos: { valido, motivo, placa, venceEn, mensaje } }
  }
  ```

  Con la placa que devuelve se sigue el flujo normal de `movimientos.verificarPlaca` del contrato. Agregar esta acción al contrato es un cambio de versión menor que deben acordar ambos lados.

## Registro con cuenta institucional

El padrón (`Alumnos`) ya no se carga a mano: lo llena el propio alumno.

```
Frontend /  ──POST /api/v1/alumnos/verificar──▶  backend  ──▶  scrap-uat-service  ──▶  alumnossur.uat.edu.mx
(correo y contraseña UAT)                           │            (red interna)          login, carreras, /Home/Programa
                                                    ▼
                                         AppSheet: Alumnos (Add o Edit)
```

1. El alumno escribe su correo `matrícula@alumnos.uat.edu.mx` (o solo la matrícula) y su contraseña.
2. El backend llama a `POST /api/v1/alumnos/scrape` del scraper sin horario ni calificaciones, solo programa y situación. Reenvía la IP del alumno para que el límite de intentos del scraper sea por persona.
3. Del plan que selecciona el portal toma matrícula (`Num_Matricula`), nombre (`Txt_Nombre`, `Txt_Paterno`, `Txt_Materno`), programa y **Situación**.
4. **Inscrito** = la situación contiene una palabra de `UAT_SITUACIONES_ACTIVAS` (`ACTIVO`, `INSCRITO`, `REGULAR`…) y ninguna negación (`NO`, `BAJA`, `INACTIVO`, `EGRESADO`…).
5. Escribe en `Alumnos`: `Correo`, `Matricula`, `NombreCompleto`, `Activo`, `Programa`, `SituacionUAT` y `VerificadoUAT`. Si el alumno ya existía, actualiza la fila. Si la UAT lo reporta de baja, lo desactiva. Si no está inscrito y no existía, no crea la fila.

El correo con el que inicia sesión en la UAT es el mismo con el que entra a AppSheet, así que la fila solo le sirve a quien demostró tener esa cuenta. `NombreCompleto` sale de la UAT, no de lo que escriba el alumno, y es contra el que se coteja la INE.

**Contraseña.** Solo viaja en esa petición, por HTTPS, al scraper y de ahí al portal. El backend no la guarda ni la escribe en logs: registra método, ruta y estado, y del resultado solo si el alumno quedó activo. Aun así, pedir la contraseña institucional en un sitio que no es de la UAT requiere autorización de la universidad y un aviso de privacidad. El frontend lo dice y ofrece la alternativa de acudir a Control Vehicular.

**Límites de intentos.** 10 por IP y 5 contraseñas rechazadas por cuenta cada 15 minutos en el backend, y 5 por minuto por IP en el scraper. Así nadie puede usar el endpoint para probar contraseñas contra la UAT.

Sin AppSheet configurado (`APPSHEET_APP_ID` vacío), el endpoint verifica con la UAT y responde, pero no guarda nada (`guardado: false`).

## Piloto con Google Sheets sin API de AppSheet

AppSheet sigue siendo el formulario y utiliza la misma hoja. El backend lee las solicitudes nuevas con un barrido periódico y escribe `Revisiones`, `Resultados` y `Pases` en Sheets. El alumno ve el resultado al sincronizar AppSheet. El bot puede permanecer desactivado.

1. Activa **Google Sheets API** y **Google Drive API** en el proyecto de la cuenta de servicio. Dale permiso de **Editor** sobre la hoja privada del registro y de **Lector** sobre la carpeta de fotos. Configura sus credenciales como Application Default Credentials. El Compose del VPS ya monta `deploy/credenciales/cuenta-servicio.json` como archivo de credenciales. Limita quién puede leer ese archivo.
2. El registro con cuenta institucional sigue activo: cada alumno que se verifica con la UAT queda en la pestaña `Alumnos` con su nombre oficial. La pestaña necesita las columnas `Correo`, `Matricula`, `NombreCompleto`, `Activo`, `Programa`, `SituacionUAT` y `VerificadoUAT`. El registro no da acceso a la app: con AppSheet Free compártela solo con las cuentas de prueba. El formulario usa el inicio de sesión de Google de AppSheet.
3. Deja vacías `APPSHEET_APP_ID`, `APPSHEET_ACCESS_KEY` y `APPSHEET_RUN_AS`. Los nombres de las pestañas usan las variables `APPSHEET_TABLA_*` que ya usa el backend. Configura `APPSHEET_TABLA_VALIDACIONES=Validaciones` si quieres que se escriba esa bitácora.

```dotenv
GOOGLE_SHEETS_ID=<ID del archivo entre /d/ y /edit>
DRIVE_CARPETA_ID=<ID de la carpeta privada de fotos>
IMAGENES_FUENTE=drive
BARRIDO_INTERVALO=1m
AUTO_APROBAR=false
```

4. Usa la misma zona horaria en Sheets y `ZONA_HORARIA`. Si la hoja guarda fechas como texto con día primero, usa `APPSHEET_LOCALE=es-MX`. El backend escribe fechas y booleanos como valores nativos de Sheets.
5. Prueba con una solicitud ficticia. `/salud` debe indicar `sheets: ok`. Tras el barrido, debe aparecer una fila en `Revisiones` y una en `Resultados`. Con `AUTO_APROBAR=false`, una persona debe publicar la aprobación para que el backend genere el pase. El modo directo está pensado para una sola instancia del backend. Usa vistas de filtro en Sheets para revisar el expediente sin cambiar el orden de las filas físicas mientras se escribe.

Esto evita usar la [API de AppSheet, disponible en planes Enterprise](https://support.google.com/appsheet/answer/10105769). AppSheet Free sigue sujeto al [límite del prototipo de hasta 10 usuarios de prueba, incluyendo al propietario](https://support.google.com/appsheet/answer/10106235).

## Alcance y límites (léelo antes de usarlo con alumnos)

- **INE.** Los dígitos de control de la MRZ y de la CURP detectan errores de captura y alteraciones burdas; no detectan una falsificación profesional. La verificación oficial contra la Lista Nominal es el [servicio de verificación del INE](https://ine.mx/servicio-verificacion-datos-credencial-votar/), que requiere convenio. Por eso, sin reverso legible la INE queda `SIN_CONFIRMAR`, como pide el procedimiento de `appsheet/README.md`.
- **REPUVE.** El portal ciudadano (`www2.repuve.gob.mx:8443/ciudadania`) es una app Angular protegida con reCAPTCHA v3. El analizador abre el portal con Playwright como lo haría una persona (escribe el NIV, pulsa **Buscar** y abre las pestañas de robo) y lee las respuestas del propio portal. **No resuelve ni evade el reCAPTCHA**: Chromium se identifica como navegador automatizado y, si REPUVE rechaza la sesión, el resultado es `NO_DISPONIBLE` y la solicitud queda para revisión humana. Hace una consulta a la vez, con una pausa mínima de 20 s y caché de 6 h por NIV.
- **Prueba contra el portal real (21 nov 2026).** Con Chromium headless, el formulario y los selectores funcionaron, pero el portal respondió `401 "El reCaptcha no fue superado correctamente"`. Mientras eso no cambie, la consulta automática termina en `NO_DISPONIBLE` y cada solicitud pasa por la revisión manual de REPUVE. El backend reintenta como máximo 3 veces por solicitud. Para no consultar el portal en vano, puedes poner `REPUVE_HABILITADO=false`. Para producción, el canal adecuado es un convenio con el SESNSP. Los scrapers antiguos, como `christianroman/repuve-api` (2013), usaban el captcha de imagen y el servlet del puerto 8080, que ya no existen (responden 503).
- **Selectores del portal.** Si REPUVE cambia su página, se ajustan las constantes al inicio de `analizador/app/repuve.py`. Para ver los selectores reales: `npx playwright codegen https://www2.repuve.gob.mx:8443/ciudadania/`. Con `REPUVE_DEPURACION_DIR` el analizador guarda una traza de cada consulta fallida, que se abre con `playwright show-trace <archivo>.zip` (red, capturas y DOM paso a paso).
- **OCR.** Funciona con fotos nítidas, completas y de frente. Con fotos malas el resultado es `SIN_CONFIRMAR`, no un rechazo.
- **Datos personales.** La UAT es sujeto obligado de la Ley General de Protección de Datos Personales en Posesión de Sujetos Obligados. El aviso de privacidad del trámite debe mencionar la verificación automatizada y la consulta a REPUVE.

## Configurar AppSheet

Usa `APPSHEET_RUN_AS` = la cuenta institucional dueña de la app (en los ejemplos, `registro.vehicular@uat.edu.mx`). Las llamadas a la API se evalúan como ese usuario, así que los filtros de seguridad deben dejarle ver y escribir.

1. **Activa la API.** En el editor: **Manage → Integrations → IN: from cloud services to your app**, activa la integración y crea una **Application Access Key**. Copia el **App ID** y la llave a `.env`.
2. **Agrega `Revisiones` a la app** (Data → Add table). Clave `SolicitudId`. Tipos: `Text` para `Identidad`, `INE`, `Tarjeta`, `REPUVE`, `NIVConsultado` y `Decision`; `Email` para `Revisor`; `DateTime` para `FechaREPUVE` y `FechaPublicacion`. No crees vistas para esta tabla y no dejes que AppSheet agregue columnas: el script de revisión valida el encabezado.
3. **(Opcional) Agrega la pestaña `Validaciones`** a la hoja con estas columnas y luego como tabla de la app: `ValidacionId` (clave, Text), `SolicitudId`, `FechaHora` (DateTime), `Origen`, `Identidad`, `INE`, `Tarjeta`, `REPUVE`, `Decision`, `PuntajeNombre` (Decimal), `Motivos` (LongText), `VehiculoREPUVE`, `DuracionSeg` (Decimal), `Version`. Si no la quieres, deja vacío `APPSHEET_TABLA_VALIDACIONES`.
4. **Agrega la pestaña `Pases`** a la hoja con estas columnas y luego como tabla de la app: `PaseId` (clave, Text), `SolicitudId` (Text), `Correo` (Email), `Placa` (Text), `Codigo` (Text), `QR` (**Image**), `EmitidoEn` (DateTime), `VenceEn` (DateTime), `Estado` (Enum: `ACTIVO`, `REVOCADO`).
5. **Filtros de seguridad** (Security → Security filters). Agrega al backend con `OR` a los filtros existentes:

   | Tabla | Filtro |
   |---|---|
   | Alumnos | `OR(LOWER(USEREMAIL()) = "registro.vehicular@uat.edu.mx", AND(ISNOTBLANK(USEREMAIL()), [Correo] = LOWER(USEREMAIL()), [Activo] = TRUE))`. Agrega antes a la hoja las columnas `Programa` (Text), `SituacionUAT` (Text) y `VerificadoUAT` (DateTime), ya incluidas en `RegistroVehicular.xlsx`, y regenera la estructura de la tabla |
   | Solicitudes | `OR(LOWER(USEREMAIL()) = "registro.vehicular@uat.edu.mx", AND(ISNOTBLANK(USEREMAIL()), [Correo] = LOWER(USEREMAIL()), IN(LOWER(USEREMAIL()), Alumnos[Correo])))` |
   | Resultados | Igual que Solicitudes |
   | Pases | Igual que Solicitudes (el alumno solo ve su pase) |
   | Revisiones, Validaciones | `LOWER(USEREMAIL()) = "registro.vehicular@uat.edu.mx"` |

6. **Actualizaciones permitidas** (Are updates allowed?) de `Alumnos`, `Resultados`, `Revisiones`, `Validaciones` y `Pases`:

   ```appsheet
   IF(LOWER(USEREMAIL()) = "registro.vehicular@uat.edu.mx", "ADDS_AND_UPDATES", "READ_ONLY")
   ```

   `Solicitudes` se queda como está: el backend solo la lee. `Alumnos` cambia porque el registro con cuenta institucional agrega y actualiza filas.
   Como el padrón se llena solo, en **Security** activa **Allow all signed-in users** limitado al dominio de alumnos, o comparte la app con el dominio: los filtros de seguridad y `Are updates allowed?` de `Solicitudes` ya impiden que una cuenta sin fila activa en `Alumnos` vea o envíe algo. Pon el enlace de la app en `APPSHEET_APP_URL` para que el alumno lo vea al terminar su registro.
7. **Muestra el QR al alumno.** En `Solicitudes`, agrega una columna virtual `PaseQR` de tipo **Image** con esta fórmula, display name `Pase de acceso` y Show_If `ISNOTBLANK([_THIS])`. Ponla en el detalle de **Mis solicitudes**.

   ```appsheet
   ANY(SELECT(Pases[QR], AND([SolicitudId] = [_THISROW].[SolicitudId], [Estado] = "ACTIVO")))
   ```

   Si prefieres una pantalla propia, crea una vista **Detail** o **Card** `Mi pase` sobre `Pases` con `QR`, `Placa` y `VenceEn`. El filtro de seguridad ya limita la tabla al alumno. AppSheet carga la imagen desde `URL_PUBLICA`, que debe ser HTTPS.
8. **Bot.** Automation → Bots → nuevo bot:
   - Evento: **Data change**, **Adds only**, tabla `Solicitudes`.
   - Paso: **Call a webhook**. URL `https://<tu-servidor>/webhooks/appsheet/solicitudes`, verbo `POST`, tipo `JSON`.
   - Encabezado: `X-Webhook-Secreto: <WEBHOOK_SECRETO>`.
   - Cuerpo:

     ```json
     { "solicitudId": "<<[SolicitudId]>>" }
     ```

   El backend responde `202` de inmediato y valida en segundo plano. Si se pierde un webhook, el barrido periódico (`BARRIDO_INTERVALO`) toma las solicitudes sin resultado de los últimos `BARRIDO_DIAS` días.

## Configurar Google Drive

AppSheet guarda en la celda una ruta como `Solicitudes_Images/1a2b.FotoINE.103412.jpg`, relativa a la carpeta de la app en el Drive del dueño (`appsheet/data/<App>-<n>/`).

1. En Google Cloud, crea una cuenta de servicio sin permisos de proyecto y descarga su llave JSON a `credenciales/cuenta-servicio.json` (la carpeta está en `.gitignore`). Activa la **Google Drive API** en el proyecto.
2. Comparte **solo** la carpeta de la app (o su subcarpeta `Solicitudes_Images`) con el correo de la cuenta de servicio, como **Lector**. Si la política del dominio impide compartir con cuentas externas, usa una cuenta de servicio del proyecto institucional o `gcloud auth application-default login --scopes=https://www.googleapis.com/auth/drive.readonly` en el servidor.
3. Pon el ID de esa carpeta en `DRIVE_CARPETA_ID` (es la última parte de su URL).

## Ejecutar

### Con Docker

```bash
cp .env.example .env      # y llena los valores
docker compose up --build
```

El backend queda en `http://localhost:8080` y el analizador solo es visible dentro de la red de Compose. Para recibir el webhook, AppSheet necesita una URL pública con HTTPS.

El despliegue del hackatón (backend, analizador, scraper y frontend detrás del Traefik de Dokploy) está en [`deploy/`](../deploy/README.md).

### Sin Docker

```bash
# Analizador
cd analizador
python3 -m venv .venv && .venv/bin/pip install -r requirements-dev.txt
.venv/bin/python -m playwright install chromium   # Chromium de Playwright, en ~/.cache/ms-playwright
.venv/bin/uvicorn app.main:app --port 8000

# Backend
set -a; . ./.env; set +a
go run ./cmd/servidor
```

Para ver la consulta a REPUVE en una ventana de Chromium, arranca el analizador con `REPUVE_HEADLESS=false`.

## API

Todas las respuestas usan el sobre del contrato del sistema (`docs/contrato-api.md` §2): `{ "ok": true, "datos": … }` o `{ "ok": false, "error": { "codigo", "mensaje" } }`. Los endpoints `/api/v1/*` piden `Authorization: Bearer <API_TOKEN>`.

| Método y ruta | Para qué |
|---|---|
| `GET /salud` | Estado del backend y del analizador |
| `POST /webhooks/appsheet/solicitudes` | Lo llama el bot. Encabezado `X-Webhook-Secreto`; cuerpo `{"solicitudId"}`. Responde `202` |
| `POST /api/v1/solicitudes/{id}/validar` | Revalida una solicitud. Con `?esperar=1` espera y devuelve el dictamen |
| `GET /api/v1/solicitudes/{id}` | Último dictamen en memoria de esa solicitud (sin datos personales) |
| `POST /api/v1/repuve` | Consulta directa `{"niv": "…"}` o `{"placa": "…"}` |
| `POST /api/v1/solicitudes/{id}/pase` | Devuelve el pase activo de una solicitud aprobada, o emite uno nuevo si no tiene (por ejemplo, después de revocarlo) |
| `POST /api/v1/pases/verificar` | Verificación en la caseta: `{"codigo": "…"}` → `{valido, motivo, placa, venceEn, mensaje}` |
| `GET /pases/qr/{codigo}.png` | Imagen del QR. Pública; solo dibuja códigos con firma válida |
| `POST /api/v1/alumnos/verificar` | **Pública** (la llama el frontend, con CORS para `CORS_ORIGENES`). `{"usuario", "contrasena"}` → `{correo, matricula, nombre, programa, situacion, activo, guardado, nuevo, appUrl, mensaje}`. Errores: `VALIDACION` 400, `CREDENCIALES` 401, `LIMITE` 429, `UAT_NO_DISPONIBLE` 502, `NO_DISPONIBLE` 503 si no hay `UAT_SERVICIO_URL` |
| `POST /api/v1/pruebas/validar` | Prueba sin AppSheet: `multipart/form-data` con `ineFrente`, `ineReverso` (opcional), `tarjeta`, `placa`, `niv`, `nombre` y `repuve=1` para consultar REPUVE. Devuelve también lo que leyó el OCR y no guarda nada |

Ejemplo de prueba con fotos de ejemplo:

```bash
curl -X POST http://localhost:8080/api/v1/pruebas/validar \
  -H "Authorization: Bearer $API_TOKEN" \
  -F ineFrente=@frente.jpg -F ineReverso=@reverso.jpg -F tarjeta=@tarjeta.jpg \
  -F placa=XYZ123A -F niv=1HGCM82633A004352 -F "nombre=María Fernanda Pérez López"
```

El analizador también se puede llamar solo: `POST /v1/ine` (campos `frente` y `reverso`) y `POST /v1/tarjeta` (campo `imagen`), con `X-Token-Analizador` si se configuró `ANALIZADOR_TOKEN`.

## Pruebas

```bash
go test -race ./...                         # reglas, pases, AppSheet, Drive, servicio, API, REPUVE y registro UAT
cd analizador && .venv/bin/pytest -q        # MRZ, CURP, NIV, OCR con imágenes sintéticas y REPUVE con Chromium real
```

Las pruebas usan datos ficticios. La consulta a REPUVE se prueba con Chromium real contra `tests/portal_falso.py`, un portal local que imita el formulario, las rutas de API, las pestañas y la alerta de reCAPTCHA del sitio. Si Chromium no está instalado, esas pruebas se omiten. Contra el portal real se prueba a mano con `POST /api/v1/repuve`.

## Variables de entorno

Ver `.env.example`. Las obligatorias son `WEBHOOK_SECRETO` y `API_TOKEN`. `APPSHEET_APP_ID`, `APPSHEET_ACCESS_KEY` y `APPSHEET_RUN_AS` van juntas: sin ellas el servidor arranca sin leer ni escribir AppSheet (lo indica en `/salud` y en el log), y con ellas se necesita también la fuente de fotos. Para el registro con cuenta institucional: `UAT_SERVICIO_URL` y `UAT_SERVICIO_CLAVE` (la `API_GATEWAY_KEYS` del scraper, al menos 32 caracteres) y `CORS_ORIGENES` con el dominio del frontend. Para emitir pases se necesitan además `PASE_SECRETO` (al menos 32 caracteres) y `URL_PUBLICA`. Si cambias `PASE_SECRETO`, todos los QR emitidos dejan de ser válidos. El servidor no arranca y lista todo lo que falta si alguna es inválida.
