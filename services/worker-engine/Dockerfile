# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 go build -trimpath -o /worker ./cmd/worker

FROM alpine:3.20
RUN apk add --no-cache ca-certificates \
  && addgroup -S coldharbor \
  && adduser -S -G coldharbor coldharbor
WORKDIR /app
COPY --from=build --chown=coldharbor:coldharbor /worker /app/worker
USER coldharbor
EXPOSE 9091
HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
  CMD wget -qO- http://localhost:9091/healthz | grep -q '"status":"UP"'
ENTRYPOINT ["/app/worker"]
