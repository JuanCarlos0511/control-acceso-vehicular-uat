# Servidor en Google Apps Script

Web App que sirve la app de caseta y el portal de administración, y guarda los datos en la hoja de cálculo ligada al proyecto. Implementa el contrato de `docs/contrato-api.md`: una sola función pública `api({ accion, datos })`.

| Archivo | Contenido |
|---|---|
| `Codigo.js` | `doGet` (sirve las apps), `api` (despachador, permisos y bitácora), `setup` e identificación del usuario |
| `Acciones.js` | Las 22 acciones del contrato |
| `Datos.js` | Lectura y escritura de hojas, conversión a los tipos del contrato, fechas y utilidades |
| `appsscript.json` | Zona horaria y configuración del Web App: ejecutar como el propietario, acceso para cualquier cuenta de Google |

La URL `/exec` abre la caseta; `/exec?app=admin`, el portal. Al primer uso se crean las hojas `Casetas`, `Usuarios`, `Personas`, `Vehiculos`, `Movimientos` y `Bitacora`, las tres casetas y el propietario como `ADMIN`.

## Usuarios

Publicado desde una cuenta personal, Google solo entrega el correo del propietario. Las demás cuentas se identifican con `Session.getTemporaryActiveUserKey()` y se guardan como `cuenta-<clave>`. Una cuenta nueva se da de alta con el rol de la propiedad del script `ROL_ALTA_AUTOMATICA` (por omisión `ADMIN`, para el prototipo); vacía, desactiva el alta automática.

## Publicar

```bash
cd front-end && npm run build && cp dist/app-caseta.html dist/portal-admin.html ../apps-script/ && cd ..
npx @google/clasp login
npx @google/clasp create --type sheets --title "Control de Acceso Vehicular UAT CUTM Sur" --rootDir apps-script   # solo la primera vez
npx @google/clasp push
npx @google/clasp create-deployment --description "versión"
```

`clasp create` reemplaza `appsscript.json` con el de Google: restáuralo antes de `push`. Al abrir la URL por primera vez, el propietario autoriza el acceso a la hoja. Los HTML compilados y `.clasp.json` no se versionan.
