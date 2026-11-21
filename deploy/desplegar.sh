#!/usr/bin/env bash
# Sube el repositorio al VPS y levanta el stack.
#   VPS_LLAVE=<llave SSH> VPS_DESTINO=<usuario>@<IP> VPS_PUERTO=<puerto SSH> ./deploy/desplegar.sh
set -euo pipefail
LLAVE=${VPS_LLAVE:?indica la llave SSH en VPS_LLAVE}
DESTINO=${VPS_DESTINO:?indica el usuario y la IP del servidor en VPS_DESTINO}
PUERTO=${VPS_PUERTO:-22}
DIR=${VPS_DIR:-/opt/hackathon}
SCRAP_REPO=${SCRAP_REPO:-https://github.com/Campus-TD/scrap-uat-service.git}
SCRAP_RAMA=${SCRAP_RAMA:-feature/dokploy-red-interna}
RAIZ=$(cd "$(dirname "$0")/.." && pwd)
SSH=(ssh -i "$LLAVE" -p "$PUERTO" "$DESTINO")

rsync -az --delete -e "ssh -i $LLAVE -p $PUERTO" \
  --exclude .git --exclude node_modules --exclude dist --exclude .venv --exclude __pycache__ --exclude .pytest_cache \
  --exclude deploy/.env --exclude "deploy/.env.respaldo-*" --exclude deploy/credenciales --exclude appsheet-backend/.env --exclude appsheet-backend/credenciales \
  "$RAIZ/" "$DESTINO:$DIR/control-acceso-vehicular-uat/"

"${SSH[@]}" bash -s -- "$DIR" "$SCRAP_REPO" "$SCRAP_RAMA" <<'REMOTO'
set -euo pipefail
DIR=$1 REPO=$2 RAMA=$3
if [ -d "$DIR/scrap-uat-service/.git" ]; then
  git -C "$DIR/scrap-uat-service" fetch -q origin "$RAMA"
  git -C "$DIR/scrap-uat-service" checkout -q -B "$RAMA" "origin/$RAMA"
else
  git clone -q --branch "$RAMA" "$REPO" "$DIR/scrap-uat-service"
fi
cd "$DIR/control-acceso-vehicular-uat/deploy"
./generar-env.sh
docker compose up -d --build --remove-orphans
docker compose ps
REMOTO
