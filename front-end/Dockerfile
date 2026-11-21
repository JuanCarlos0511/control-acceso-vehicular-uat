# Publica las tres apps como sitio estático (fuera de Apps Script):
#   /         registro del alumno con su cuenta institucional (habla con el backend en Go)
#   /caseta   app de caseta   ┐ en modo demostración (VITE_DEMO=true): usan el mock del
#   /admin    portal de admin ┘ contrato, porque su backend es Apps Script
FROM node:22-bookworm-slim AS compilacion
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY . .
ARG VITE_BACKEND_URL
ARG VITE_DEMO=true
ENV VITE_BACKEND_URL=$VITE_BACKEND_URL VITE_DEMO=$VITE_DEMO
RUN npm run build

FROM nginxinc/nginx-unprivileged:1.29-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=compilacion /src/dist /usr/share/nginx/html
EXPOSE 8080
