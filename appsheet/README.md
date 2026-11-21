# Registro vehicular estudiantil en AppSheet

Antes de su primera solicitud, el alumno confirma que está inscrito en el [registro de alumnos](https://hackathonfrontend.452111.xyz) iniciando sesión con su cuenta de la UAT. Eso crea su fila en `Alumnos` con su nombre oficial (lo hace el backend, ver `appsheet-backend/README.md`). Después inicia sesión en AppSheet con su cuenta de Google institucional, registra su placa y número de identificación vehicular (NIV) y adjunta las fotos de su INE y de su tarjeta de circulación. Puede consultar el estado de su solicitud. El personal de Control Vehicular coteja los documentos y publica el resultado desde una hoja privada.

## Crear el borrador

1. Con la cuenta institucional propietaria, importa `RegistroVehicular.xlsx` como Google Sheets y mantén el archivo y su carpeta en acceso restringido. Comparte la hoja y las fotos únicamente con el personal responsable. Usa la zona horaria de Monterrey.
2. `Alumnos` se llena con el registro de cuenta institucional del backend. Sin el backend, carga el padrón oficial a mano: correo institucional en minúsculas, matrícula en texto, nombre completo y `Activo` como booleano. Usa una sola fila por correo. Para el piloto, usa cuentas de prueba y documentos ficticios, sin datos de alumnos.
3. En [AppSheet](https://www.appsheet.com/Account/Login), crea `Registro vehicular estudiantil` con **Create → App → Start with existing data → Google Sheets**. Selecciona `Solicitudes` como tabla inicial y agrega `Alumnos` y `Resultados`. La pestaña `Revisiones` se usa desde Sheets y debe quedar fuera de la app estudiantil.
4. En **Security**, exige inicio de sesión con Google y deja desactivado **Allow all signed-in users**. Comparte como **Use app** únicamente con las cuentas del padrón habilitadas. Usa **as app creator** para que los alumnos no tengan permisos de Drive sobre la hoja ni los archivos. Deja activado **Require Image and File URL Signing** y activa **Secure Image Access**. Desactiva inicio sin conexión y almacenamiento de contenido para uso sin conexión.
5. En la hoja de Google, abre **Extensiones → Apps Script**, pega `RevisionVehicular.gs` y guarda. Al volver a abrir la hoja, aparecerá el menú **Registro vehicular**. Cada revisor autorizado ejecuta el menú desde su cuenta institucional. No requiere desplegar un Web App.

## Tablas y permisos

Configura los filtros en **Security → Security filters**. **Show_If** y los slices sirven para presentación, por lo que también se necesitan estos filtros para impedir que otra cuenta sincronice los datos.

| Tabla | Clave | Actualizaciones permitidas | Filtro de seguridad |
| --- | --- | --- | --- |
| Alumnos | `Correo` | `READ_ONLY` | `AND(ISNOTBLANK(USEREMAIL()), [Correo] = LOWER(USEREMAIL()), [Activo] = TRUE)` |
| Solicitudes | `SolicitudId` | Expresión de abajo | `AND(ISNOTBLANK(USEREMAIL()), [Correo] = LOWER(USEREMAIL()), IN(LOWER(USEREMAIL()), Alumnos[Correo]))` |
| Resultados | `SolicitudId` | `READ_ONLY` | `AND(ISNOTBLANK(USEREMAIL()), [Correo] = LOWER(USEREMAIL()), IN(LOWER(USEREMAIL()), Alumnos[Correo]))` |

En **Are updates allowed?** de `Solicitudes`, pega esta expresión. Así solo puede enviar formularios una cuenta activa del padrón y las solicitudes enviadas se conservan para la revisión.

```appsheet
IF(
  AND(ISNOTBLANK(USEREMAIL()), IN(LOWER(USEREMAIL()), Alumnos[Correo])),
  "ADDS_ONLY",
  "READ_ONLY"
)
```

### Columnas de `Solicitudes`

| Columna | Tipo | Configuración |
| --- | --- | --- |
| `SolicitudId` | Text | Key. Initial value `UNIQUEID("UUID")`. Oculta en el formulario y no editable. |
| `Correo` | Email | App formula `LOWER(USEREMAIL())`. No editable. Valid if `AND([_THIS] = LOWER(USEREMAIL()), IN([_THIS], Alumnos[Correo]))`. |
| `FechaEnvio` | DateTime | Initial value `NOW()`. Oculta en el formulario y no editable. |
| `Placa` | Text | Obligatoria. Display name `Placa`. Descripción `Escríbela como aparece en tu tarjeta de circulación.` |
| `NIV` | Text | Obligatorio. Display name `NIV (17 caracteres)`. Valid if `LEN(TRIM([_THIS])) = 17`. |
| `FotoINE` | Image | Obligatoria. Display name `INE · frente`. |
| `FotoINEReverso` | Image | Opcional. Display name `INE · reverso`. Descripción `Adjunta el reverso si se necesita el código QR o los códigos de vigencia para verificar tu credencial.` |
| `FotoTarjeta` | Image | Obligatoria. Display name `Tarjeta de circulación`. |
| `AvisoLeido` | Yes/No | Obligatorio. Initial value `FALSE`, Valid if `[_THIS] = TRUE`. Display name `Leí el aviso de privacidad`. Agrega a la descripción el enlace al aviso institucional aprobado para este trámite. |

Usa esta expresión en **Valid if** de `Placa` para admitir los separadores de la tarjeta y mantener la longitud del catálogo de casetas. El script confirma los caracteres permitidos antes de aprobar.

```appsheet
AND(
  LEN(SUBSTITUTE(SUBSTITUTE(SUBSTITUTE(TRIM([_THIS]), " ", ""), "-", ""), ".", "")) >= 4,
  LEN(SUBSTITUTE(SUBSTITUTE(SUBSTITUTE(TRIM([_THIS]), " ", ""), "-", ""), ".", "")) <= 10
)
```

Usa `Email`, `Text`, `Name` y `Yes/No` para `Correo`, `Matricula`, `NombreCompleto` y `Activo` en `Alumnos`, y `Text`, `Text` y `DateTime` para `Programa`, `SituacionUAT` y `VerificadoUAT`, que llena el registro con cuenta institucional. En `Resultados` usa `Text`, `Email`, `Text`, `LongText`, `DateTime` en orden de la hoja. Mantén la clave `SolicitudId` como texto en todas las tablas.

### Vistas del alumno

- **Registrar vehículo**. Vista `Form` de `Solicitudes` con `Correo`, `Placa`, `NIV`, `FotoINE`, `FotoINEReverso`, `FotoTarjeta` y `AvisoLeido` en ese orden. Pide fotos legibles y completas. Para regresar a **Mis solicitudes**, crea una acción de `Solicitudes` del tipo **App: go to another view within this app** con destino `LINKTOVIEW("Mis solicitudes")` y selecciónala en **Form Saved**.
- **Mis solicitudes**. Vista `Deck` o `Table` de `Solicitudes` con placa, fecha y estado. Ordena por `FechaEnvio` descendente. En el detalle muestra también la indicación del equipo revisor. Oculta las vistas de navegación generadas para `Alumnos` y `Resultados`.

Agrega `Estado` e `Indicacion` como columnas virtuales de `Solicitudes`. Usa estas fórmulas en ese orden.

```appsheet
SWITCH(
  LOOKUP([_THISROW].[SolicitudId], "Resultados", "SolicitudId", "Estado"),
  "EN_REVISION", "En revisión",
  "REQUIERE_ATENCION", "Requiere atención",
  "APROBADA", "Aprobada",
  "Pendiente de revisión"
)
```

```appsheet
LOOKUP([_THISROW].[SolicitudId], "Resultados", "SolicitudId", "Indicacion")
```

## Revisar una solicitud

1. En la hoja privada selecciona una fila de `Solicitudes` y elige **Registro vehicular → Abrir revisión de la solicitud**. El menú abre su fila en `Revisiones`. Para ver las fotos, abre los archivos en el Drive institucional desde la carpeta del origen indicada en la columna de cada imagen.
2. **Identidad e INE**. Coteja el nombre de la identificación con el nombre del alumno del padrón asociado al correo autenticado. Consulta [vigencia en el INE](https://listanominal.ine.mx/scpln/) y verifica autenticidad con [Valida INE-QR](https://www.ine.mx/conoce-tu-credencial-para-votar/) para modelos compatibles o con el procedimiento institucional autorizado. Si falta el reverso, la foto no se puede leer o existe duda de identidad, deja la verificación sin confirmar. Registra el resultado de la revisión sin copiar CURP, CIC, OCR ni fotografías a `Revisiones`.
3. **Tarjeta**. Coteja placa y NIV de la tarjeta con la solicitud. Verifica su vigencia y autenticidad con el procedimiento de la autoridad emisora. Si falta evidencia, deja el campo sin confirmar.
4. **REPUVE**. Abre la [Consulta Ciudadana oficial](https://www.gob.mx/sesnsp/acciones-y-programas/consulta-ciudadana-del-registro-publico-vehicular-repuve), consulta por NIV y resuelve el código de imagen en el portal. Compara el registro con la tarjeta y guarda `NIVConsultado`, `FechaREPUVE` y el resultado. Usa `NO_DISPONIBLE` si falla el portal, `NO_ENCONTRADO` si no hay registro y `DATOS_NO_COINCIDEN` si hay discrepancias. REPUVE muestra datos y estatus del vehículo. La tarjeta requiere la verificación del paso anterior.
5. Selecciona `EN_REVISION`, `DEVOLVER` o `APROBAR` y usa **Publicar resultado seleccionado**. `APROBAR` exige alumno activo, coincidencia de identidad, INE y tarjeta validadas, y consulta REPUVE fechada para el mismo NIV con resultado `SIN_REPORTE`. El script impide otra aprobación para la misma placa o NIV. El alumno ve el resultado al sincronizar. Si la solicitud requiere una corrección documental, el alumno presenta una nueva solicitud.

El flujo usa consulta humana. Para automatizar la verificación, la institución necesita un canal oficial autorizado y su documentación. El [servicio institucional del INE](https://ine.mx/servicio-verificacion-datos-credencial-votar/) opera mediante convenio. La [consulta pública de REPUVE](https://www.gob.mx/segob/acciones-y-programas/consulta-al-registro-publico-vehicular) está protegida con reCAPTCHA. Una aprobación de la solicitud representa el resultado de este trámite. El alta operativa en el catálogo de casetas requiere el backend del sistema principal.

## Validación automática

El backend de [`appsheet-backend/`](../appsheet-backend/README.md) prellena la revisión de cada solicitud nueva: lee las fotos con OCR, verifica la MRZ y la CURP de la INE, coteja el nombre con el padrón, compara el NIV y la placa de la tarjeta y consulta REPUVE en el portal ciudadano. Escribe en `Revisiones` y `Resultados` con los mismos valores de este procedimiento y firma como `Revisor` con su propia cuenta. Aprueba solo cuando todo coincide (o deja la aprobación a una persona con `AUTO_APROBAR=false`), devuelve al alumno solo los problemas claros y deja todo lo demás `EN_REVISION`. Una revisión que ya publicó una persona no se vuelve a tocar.

Cuando la solicitud queda aprobada, por el backend o por una persona desde el menú, el backend emite un pase con código QR en la tabla `Pases`. El alumno lo ve en **Mis solicitudes** y lo muestra en la caseta. Para revocarlo, cambia su `Estado` a `REVOCADO` en la hoja.

Para usarlo, agrega `Revisiones` (sin vistas) y `Pases` a la app, permite al backend leer y escribir en los filtros de seguridad, agrega la columna virtual del QR y crea el bot que llama al webhook. Los pasos están en el README del backend.

## Verificar antes de compartir con alumnos

Completa el aviso institucional y el plazo de conservación de expedientes. Define quién puede revisar en Drive y quién retira los adjuntos cuando vence el plazo. Prueba con dos cuentas estudiantiles distintas y una cuenta fuera del padrón. Cada alumno debe poder enviar ambas fotos requeridas y ver solo sus propios datos y archivos. Comprueba que la cuenta externa no accede y que desactivar un alumno impide nuevas solicitudes y la sincronización de sus datos. Comprueba también que una foto ajena no se pueda abrir mediante su enlace, que las fotos no se almacenen en una carpeta compartida y que un reporte o consulta fallida se mantenga sin aprobación. La app queda lista para el piloto cuando estas pruebas pasen en el editor y con cuentas de prueba reales del dominio.
