# Multi-service Go Dockerfile.
# Args:
#   SERVICE — module directory (e.g. "proxy-gateway")
#   BIN     — binary name (e.g. "proxy")

FROM golang:1.25-alpine AS builder
ARG SERVICE
ARG BIN
WORKDIR /src

# Copy workspace + every Go module so cross-module replace works.
COPY go.work ./
COPY pkg/event/         pkg/event/
COPY pkg/redact/        pkg/redact/
COPY pkg/ratelimit/     pkg/ratelimit/
COPY proxy-gateway/     proxy-gateway/
COPY ingest-api/        ingest-api/
COPY log-consumer/      log-consumer/
COPY auth-api/          auth-api/
COPY stats-api/         stats-api/
COPY sdk-reference-go/  sdk-reference-go/
COPY cleanup/           cleanup/

RUN go work sync
RUN cd ${SERVICE} && CGO_ENABLED=0 go build -ldflags='-s -w' -o /out/app ./cmd/${BIN}

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/app /app
ENTRYPOINT ["/app"]
