# syntax=docker/dockerfile:1.7

# --- build stage ----------------------------------------------------------
FROM golang:1.22-alpine AS build
WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/ingestion-go .

# --- runtime stage --------------------------------------------------------
FROM alpine:3.20
RUN adduser -D -u 10001 app
WORKDIR /app
COPY --from=build /out/ingestion-go /app/ingestion-go
USER app
EXPOSE 8080
ENTRYPOINT ["/app/ingestion-go"]
