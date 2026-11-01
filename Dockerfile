# Build hankoid (Hanko + admin frontend).
FROM --platform=$BUILDPLATFORM golang:1.26 AS builder
ARG TARGETOS=linux
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/hankoid ./cmd/hankoid

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /out/hankoid /hankoid
COPY deploy/config.yaml /config/config.yaml
USER 65532:65532
EXPOSE 8000 8001 8080
ENTRYPOINT ["/hankoid"]
CMD ["serve", "all", "--config", "/config/config.yaml"]
