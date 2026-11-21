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
- `front-end/`: app de caseta, portal de administración y código compartido (ver su `README.md`)

## Módulos

- **Catálogos**: vehículos autorizados, personas (estudiantes, empleados, visitantes), casetas
- **Operación**: registro de entrada y salida con fecha y hora automáticas
- **Consultas**: por placa, rango de fechas, acceso y tipo de usuario
- **Reportes**: entradas y salidas por periodo, vehículos más frecuentes, horarios de mayor afluencia
- **Seguridad**: inicio de sesión, permisos por rol, bitácora de actividades

## Estado

En desarrollo.
