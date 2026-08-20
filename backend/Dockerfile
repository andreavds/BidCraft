FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server

FROM alpine:3.20

RUN adduser -D -u 10001 bidcraft
COPY --from=build /out/server /usr/local/bin/server

RUN mkdir -p /app/uploads && chown -R bidcraft /app

USER bidcraft
WORKDIR /app
ENV UPLOAD_DIR=/app/uploads
EXPOSE 8080
CMD ["server"]
