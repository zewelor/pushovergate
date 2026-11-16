FROM --platform=$BUILDPLATFORM golang:1.27.4-trixie@sha256:f18c3d73a184fb0582828510722e723b63a93f7188d11dd223d9be6f6880e65d AS test

WORKDIR /src

COPY go.mod main.go main_test.go ./

RUN go mod tidy -diff && \
    unformatted="$(gofmt -l .)" && \
    test -z "$unformatted" || { echo "$unformatted"; exit 1; } && \
    go vet ./... && \
    go test -race ./...

FROM --platform=$BUILDPLATFORM golang:1.27.4-trixie@sha256:f18c3d73a184fb0582828510722e723b63a93f7188d11dd223d9be6f6880e65d AS build

WORKDIR /src

COPY go.mod main.go ./

ARG TARGETOS
ARG TARGETARCH

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /pushovergate .

# Use distroless static to provide root CA certificates for Pushover TLS and a nonroot user.
# This avoids manual CA cert copying and raw UID configuration required by scratch.
FROM gcr.io/distroless/static-debian13:nonroot@sha256:f7f8f729987ad0fdf6b05eeeae94b26e6a0f613bdf46feea7fc40f7bd72953e6 AS runtime

COPY --from=build --chown=nonroot:nonroot /pushovergate /pushovergate
COPY --chown=nonroot:nonroot LICENSE /LICENSE

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/pushovergate"]
