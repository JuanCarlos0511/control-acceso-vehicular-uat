# Despliegue (VPS con Dokploy)

Un solo proyecto de Docker Compose (`hackathon`) con cuatro servicios. El Traefik de Dokploy los publica por etiquetas en la red `dokploy-network` y emite los certificados de Let's Encrypt. Cloudflare está delante.

| Dominio | Servicio | Qué es |
|---|---|---|
| `hackathon.452111.xyz` | `hackathon-backend` | Backend en Go: webhook de AppSheet, pases QR y registro de alumnos (`appsheet-backend/`) |
| `hackathonfrontend.452111.xyz` | `hackathon-frontend` | `/` registro del alumno con su cuenta institucional; `/caseta` y `/admin` en modo demostración (`front-end/`) |
| `hackathonapi.452111.xyz` | `hackathon-scrap` | [`scrap-uat-service`](https://github.com/Campus-TD/scrap-uat-service), rama `feature/dokploy-red-interna`. Pide `X-API-Key` |
| — | `hackathon-analizador` | OCR y REPUVE (Python). Solo en la red interna |

El backend llama al analizador y al scraper por la red interna del proyecto (`http://hackathon-analizador:8000`, `http://hackathon-scrap:3100`), no por los dominios públicos. Los servicios llevan el prefijo `hackathon-` porque `dokploy-network` es compartida con otros proyectos del VPS: un nombre como `backend` o `scrap-uat-service` resolvería también a contenedores ajenos.

La caseta y el panel se publican con `VITE_DEMO=true` (el mock del contrato, datos ficticios y un aviso en pantalla), porque su backend es Apps Script y se sirve con `HtmlService` según `docs/contrato-api.md`.

## Desplegar o actualizar

Desde la raíz del repositorio, en tu máquina:

```bash
VPS_LLAVE=<llave SSH> VPS_DESTINO=<usuario>@<IP del servidor> VPS_PUERTO=<puerto SSH> ./deploy/desplegar.sh
```

El script:

1. Sincroniza el repositorio con `/opt/hackathon/control-acceso-vehicular-uat` por rsync, sin `.git`, `node_modules`, `deploy/.env` ni `deploy/credenciales/`.
2. Clona o actualiza `scrap-uat-service` en `/opt/hackathon/scrap-uat-service`.
3. Ejecuta `generar-env.sh`: crea `deploy/.env` (permisos 600) desde `.env.example` y llena con `openssl rand -hex 32` los secretos vacíos. Nunca sobrescribe valores existentes.
4. Ejecuta `docker compose up -d --build`.

## Lo que falta llenar en el VPS

Para un piloto con AppSheet Free, el backend también puede usar la misma hoja como origen mediante la API de Google Sheets. Deja vacías las credenciales de la API de AppSheet y sigue la [configuración del piloto por Google Sheets](../appsheet-backend/README.md#piloto-con-google-sheets-sin-api-de-appsheet). En este VPS:

1. Sube la llave JSON de la cuenta de servicio. El contenedor la lee como usuario sin privilegios, así que queda `644` dentro de `/opt/hackathon`, que solo puede abrir `ubuntu`:

   ```bash
   scp -P <puerto SSH> -i <llave SSH> llave.json <usuario>@<IP del servidor>:/opt/hackathon/control-acceso-vehicular-uat/deploy/credenciales/cuenta-servicio.json
   ```

2. En `deploy/.env` llena `GOOGLE_SHEETS_ID` y `DRIVE_CARPETA_ID`. Deja vacías `AUTO_APROBAR` y `BARRIDO_INTERVALO` para que tomen los valores del modo Sheets: aprobación humana y barrido cada minuto.
3. Ejecuta `docker compose up -d --force-recreate hackathon-backend` y revisa que `https://hackathon.452111.xyz/salud` diga `"sheets": "ok"`. Si dice `sin respuesta o sin acceso`, falta compartir la hoja con la cuenta de servicio o activar Google Sheets API.

El registro de alumnos de `hackathonfrontend.452111.xyz` sigue activo en modo Sheets y escribe en la pestaña `Alumnos`, que necesita las columnas `Programa`, `SituacionUAT` y `VerificadoUAT`.

Edita `/opt/hackathon/control-acceso-vehicular-uat/deploy/.env` y vuelve a ejecutar `docker compose up -d` en esa carpeta:

| Variable | De dónde sale |
|---|---|
| `APPSHEET_APP_ID`, `APPSHEET_ACCESS_KEY` | AppSheet → Manage → Integrations → IN |
| `APPSHEET_RUN_AS` | Correo dueño de la app |
| `APPSHEET_APP_URL` | Enlace para compartir la app (`https://www.appsheet.com/start/…`) |
| `DRIVE_CARPETA_ID` | Carpeta de la app en Drive, y la llave de la cuenta de servicio en `deploy/credenciales/cuenta-servicio.json` |

Mientras falten, el backend arranca sin AppSheet. El registro de alumnos confirma la inscripción con la UAT pero no guarda la fila, y el webhook no valida solicitudes. `/salud` lo indica con `"appsheet": "deshabilitado"`.

Para configurar AppSheet con los secretos generados, léelos en el VPS:

```bash
ssh -i <llave SSH> -p <puerto SSH> <usuario>@<IP del servidor> \
  'grep -E "^(WEBHOOK_SECRETO|API_TOKEN)=" /opt/hackathon/control-acceso-vehicular-uat/deploy/.env'
```

- Bot de AppSheet: `POST https://hackathon.452111.xyz/webhooks/appsheet/solicitudes` con `X-Webhook-Secreto: <WEBHOOK_SECRETO>`.
- API del scraper: `UAT_SERVICIO_CLAVE` es su `X-API-Key`.

## Operación

```bash
cd /opt/hackathon/control-acceso-vehicular-uat/deploy
docker compose ps
docker compose logs -f hackathon-backend
docker compose up -d --build hackathon-frontend   # reconstruir un servicio
curl -s https://hackathon.452111.xyz/salud
```

Límites de memoria: backend 256 MB, scraper 512 MB, frontend 64 MB, analizador 3 GB y 2 CPU (Chromium y los modelos de OCR).

Este stack no aparece en el panel de Dokploy: se administra con `docker compose` desde la carpeta de arriba. Traefik lo descubre igual por las etiquetas.
