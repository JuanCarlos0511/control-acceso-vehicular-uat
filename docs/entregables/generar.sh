#!/usr/bin/env bash
# Compila los entregables a PDF con WeasyPrint.
#   ./generar.sh            todos
#   ./generar.sh 3-documento-de-requisitos.html
# Requiere WeasyPrint (pip install weasyprint). Los diagramas se regeneran con PlantUML:
#   java -jar plantuml.jar -tpng -Sdpi=200 -o ../img diagramas/*.puml
set -euo pipefail
cd "$(dirname "$0")"
WEASYPRINT=${WEASYPRINT:-weasyprint}
mkdir -p pdf
archivos=("$@")
[ ${#archivos[@]} -eq 0 ] && archivos=([0-9]-*.html)
for html in "${archivos[@]}"; do
  pdf="pdf/${html%.html}.pdf"
  "$WEASYPRINT" -q "$html" "$pdf"
  echo "$pdf"
done
