FROM golang:1.25-bookworm
WORKDIR /src
COPY . .
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOAMD64=v1
RUN go mod download
ARG BUILD_TAGS="with_gvisor with_quic with_utls"
# Keep with_gvisor by default because the production test profile needs TUN/global-proxy support.
# For VLESS Reality + Hysteria2-only nodes, build with --build-arg BUILD_TAGS="with_quic with_utls" to save about 0.8 MiB RSS in the measured DE test environment.
RUN go build -trimpath -tags "$BUILD_TAGS" -ldflags "-s -w -buildid=" -o /out/star1ight-agent ./cmd/star1ight-agent
