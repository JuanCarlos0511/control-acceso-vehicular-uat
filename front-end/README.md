# Front-end

Tres aplicaciones web que comparten código. La caseta y el panel se sirven desde el proyecto de Apps Script; el registro de alumnos se publica aparte y habla con el backend en Go (`appsheet-backend/`):

| Carpeta | Para quién | Resultado de la compilación |
|---|---|---|
| `app-caseta/` | Guardias, en el celular | `dist/app-caseta.html` |
| `portal-admin/` | Administradores y supervisores, en escritorio | `dist/portal-admin.html` |
| `registro-alumno/` | Alumnos: confirman con su cuenta institucional que están inscritos | `dist/registro-alumno.html` |
| `shared/` | Código común: tipos del contrato, cliente de la API, mock, estilos y componentes | — |

**Stack:** React 19, TypeScript, Vite y Tailwind 4. Cada app se compila en **un solo archivo HTML** (con `vite-plugin-singlefile`), porque eso es lo que Apps Script puede servir con `HtmlService`.

## Uso

Requiere Node 22 o más reciente.

```bash
npm install
npm run dev:caseta   # http://localhost:5173
npm run dev:admin    # http://localhost:5174
npm run dev:registro # http://localhost:5175 (VITE_BACKEND_URL=http://localhost:8080 para usar un backend local)
npm run build        # revisa tipos y genera los tres HTML en dist/
```

### Desarrollo sin backend

Fuera de Apps Script, `llamarApi` usa el **mock del contrato** (`shared/mock/`):
- Los datos son ficticios, viven en memoria y se reinician al recargar.
- Lo que registras aparece en las consultas y en los reportes.
- Aplica la misma matriz de permisos y las mismas validaciones del contrato.
- No se incluye en la compilación final.

Para probar con otro usuario, agrega `?rol=` a la URL:

| `?rol=` | Usuario |
|---|---|
| `GUARDIA` (por defecto en caseta) | Guardia con caseta fija |
| `ROTATIVO` | Guardia sin caseta fija: elige caseta al comenzar turno |
| `SUPERVISOR` | Solo lectura |
| `ADMIN` (por defecto en el panel) | Todo |
| `INACTIVO`, `NOREG`, `SINID` | Pantallas de error de acceso |

### Publicar fuera de Apps Script

`Dockerfile` y `nginx.conf` sirven las tres apps como sitio estático: `/` es el registro de alumnos, `/caseta` y `/admin` las otras dos. Variables de compilación:

- `VITE_BACKEND_URL`: URL del backend en Go para el registro de alumnos.
- `VITE_DEMO=true`: incluye el mock en la compilación de la caseta y el panel y muestra el aviso «Demostración · datos ficticios». Sin Apps Script no tienen otro backend.

El despliegue está en `deploy/`.

## Reglas

- El backend se llama **solo** con `llamarApi` (`shared/api.ts`). Nunca uses `google.script.run` directamente.
- `shared/contrato.ts` es la copia en TypeScript de `docs/contrato-api.md`. Si cambia el contrato, se cambia primero el documento y luego este archivo.
- Qué se muestra en cada pantalla depende de los `permisos` que manda `app.iniciar`, nunca del nombre del rol.
- Los colores y la tipografía están en `shared/estilos.css`: asfalto para entradas, amarillo vial para salidas.

## Integración con Apps Script (lo hace el backend)

1. Correr `npm run build`.
2. Copiar `dist/app-caseta.html` y `dist/portal-admin.html` al proyecto de Apps Script con esos mismos nombres.
3. Servirlos según el parámetro `app` (contrato §1):

```js
function doGet(e) {
  const admin = e && e.parameter && e.parameter.app === 'admin';
  return HtmlService.createHtmlOutputFromFile(admin ? 'portal-admin' : 'app-caseta')
    .setTitle(admin ? 'Acceso vehicular · Panel' : 'Acceso vehicular · Caseta')
    .addMetaTag('viewport', 'width=device-width, initial-scale=1');
}
```

- Usar `createHtmlOutputFromFile` y **no** `createTemplateFromFile`: el JavaScript minificado puede contener `<?`, que la plantilla intentaría interpretar.
- La etiqueta viewport del HTML no tiene efecto dentro de HtmlService; por eso va `addMetaTag`.

## Por probar dentro de Apps Script

- **Descargas de Excel y PDF:** la app corre en un iframe con sandbox. Si el navegador bloquea la descarga, el plan B es abrir el archivo en una pestaña nueva o generarlo en el servidor.
- **Librerías de exportación:** SheetJS y jsPDF se cargan de `cdn.jsdelivr.net` solo al exportar, así que se necesita internet.
- **Cerrar sesión:** los enlaces de cerrar sesión y "Usar otra cuenta" van a `accounts.google.com/Logout` con `target="_top"`. En desarrollo no navegan: solo regresan a la pantalla de comenzar turno.
