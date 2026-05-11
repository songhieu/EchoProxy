FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY main.go .
RUN go mod init mock-upstream && CGO_ENABLED=0 go build -o /out/app .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/app /app
ENTRYPOINT ["/app"]
