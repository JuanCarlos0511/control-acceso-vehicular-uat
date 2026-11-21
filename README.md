# Control de Acceso Vehicular — UAT CUTM Sur

Sistema web para registrar y administrar las entradas y salidas de vehículos en las tres casetas de acceso del Campus CUTM Sur de la Universidad Autónoma de Tamaulipas. Proyecto del Hackatón de la carrera de Ingeniería en Sistemas Computacionales, Facultad de Ingeniería Tampico.

## Stack

- **Google Apps Script** (V8), publicado como Web App
- **Google Sheets** como base de datos
- **React + TypeScript + Vite + Tailwind** para las dos interfaces: la app de caseta (celular) y el portal de administración (escritorio). Cada una se compila en un solo HTML que sirve Apps Script
- **SheetJS** y **jsPDF** para exportar a Excel y PDF
- **clasp** para sincronizar este repositorio con el proyecto de Apps Script

## Estructura

- `docs/contrato-api.md`: contrato entre frontend y backend (acciones, tipos, errores y permisos)
- `apps-script/`: servidor en Google Apps Script que sirve la caseta y el portal y guarda los datos en Google Sheets (ver su `README.md`)
- `docs/entregables/`: documentos de entrega (requisitos, manual, evidencias, sistema funcional y código fuente) con sus fuentes, diagramas UML y PDF
- `front-end/`: app de caseta, portal de administración y código compartido (ver su `README.md`)
- `appsheet/`: plantilla y configuración del registro vehicular estudiantil en AppSheet, con revisión de expedientes en Google Sheets
- `appsheet-backend/`: backend en Go (con un analizador de documentos en Python) que valida automáticamente las solicitudes de AppSheet: INE contra el padrón, tarjeta de circulación y consulta a REPUVE. También da de alta a los alumnos que confirman su inscripción con su cuenta institucional (ver su `README.md`)
- `deploy/`: despliegue en el VPS con Dokploy. Backend en `hackathon.452111.xyz`, frontend en `hackathonfrontend.452111.xyz` y `scrap-uat-service` en `hackathonapi.452111.xyz`

## Módulos

- **Catálogos**: vehículos autorizados, personas (estudiantes, empleados, visitantes), casetas
- **Operación**: registro de entrada y salida con fecha y hora automáticas
- **Consultas**: por placa, rango de fechas, acceso y tipo de usuario
- **Reportes**: entradas y salidas por periodo, vehículos más frecuentes, horarios de mayor afluencia
- **Seguridad**: inicio de sesión, permisos por rol, bitácora de actividades

## Estado

En desarrollo.
