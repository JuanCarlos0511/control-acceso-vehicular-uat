#!/usr/bin/env bash
# Crea deploy/.env a partir de .env.example con secretos aleatorios.
# No sobrescribe un .env existente: solo llena los secretos que estén vacíos.
set -euo pipefail
cd "$(dirname "$0")"
umask 077
[ -f .env ] || cp .env.example .env
for v in WEBHOOK_SECRETO API_TOKEN ANALIZADOR_TOKEN PASE_SECRETO UAT_SERVICIO_CLAVE; do
  if grep -qE "^$v=$" .env; then
    sed -i "s|^$v=$|$v=$(openssl rand -hex 32)|" .env
  fi
done
chmod 600 .env
mkdir -p credenciales && chmod 755 credenciales
echo "deploy/.env listo. Faltan: $(grep -E '^(APPSHEET_APP_ID|APPSHEET_ACCESS_KEY|APPSHEET_RUN_AS|DRIVE_CARPETA_ID|APPSHEET_APP_URL)=$' .env | cut -d= -f1 | tr '\n' ' ')"
