ARG VERSION=dev

FROM --platform=$BUILDPLATFORM golang:1.27.1-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN apk add --no-cache ca-certificates

ARG VERSION
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags "-s -w -X github.com/pinheirolucas/peace-breaker-bot/cmd.Version=${VERSION}" \
    -o /out/peace-breaker-bot .

FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /out/peace-breaker-bot /peace-breaker-bot

ENV HOME=/home/app

# Starts as root: the binary itself chowns /home/app/.instants to
# PUID/PGID (default 65532:65532, see pkg/privdrop) and drops to it before
# doing anything else, so a freshly-mounted volume never needs to be
# pre-created or chowned by hand.
VOLUME /home/app/.instants
EXPOSE 9001

ENTRYPOINT ["/peace-breaker-bot"]
