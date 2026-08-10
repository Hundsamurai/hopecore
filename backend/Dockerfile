# Сборка статического бинаря без CGO: драйвер SQLite (glebarez/sqlite)
# реализован на чистом Go, поэтому компилятор C в образе не нужен.
FROM golang:1.23-alpine AS build

WORKDIR /src

# Зависимости слоем отдельно от кода, чтобы кеш не сбрасывался на каждой правке.
# go.sum по глобу: на Этапе 1 внешних зависимостей ещё нет и файла может не быть.
COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.20

# ca-certificates нужны для HTTPS-запросов к сайтам вакансий (Task 5),
# wget входит в busybox и используется healthcheck-ом в docker-compose.
RUN apk add --no-cache ca-certificates \
    && addgroup -S app && adduser -S -G app app \
    && mkdir -p /data && chown app:app /data

COPY --from=build /out/server /usr/local/bin/server

USER app
WORKDIR /data
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/server"]
