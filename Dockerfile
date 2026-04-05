FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /victorialogs-compat-layer .

FROM alpine:3.20
COPY --from=builder /victorialogs-compat-layer /usr/local/bin/victorialogs-compat-layer
EXPOSE 8471
HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://localhost:8471/healthz || exit 1
ENTRYPOINT ["victorialogs-compat-layer"]
