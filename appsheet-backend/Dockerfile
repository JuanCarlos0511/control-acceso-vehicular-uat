# Backend en Go. Sin navegador: la consulta a REPUVE la hace el analizador.
FROM golang:1.26-trixie AS compilacion
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /servidor ./cmd/servidor

# Incluye certificados de CA; la base de zonas horarias va dentro del binario (time/tzdata).
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=compilacion /servidor /servidor
ENV ZONA_HORARIA=America/Monterrey
EXPOSE 8080
ENTRYPOINT ["/servidor"]
