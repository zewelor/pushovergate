FROM --platform=$BUILDPLATFORM golang:1.27.4-trixie@sha256:430985542100b871e108ccc00e8dcc5eef08ca14ee5825e98ebf563d8ffec4ee AS test

WORKDIR /src

COPY go.mod main.go main_test.go ./

RUN go mod tidy -diff && \
    unformatted="$(gofmt -l .)" && \
    test -z "$unformatted" || { echo "$unformatted"; exit 1; } && \
    go vet ./... && \
    go test -race ./...

FROM --platform=$BUILDPLATFORM golang:1.27.4-trixie@sha256:430985542100b871e108ccc00e8dcc5eef08ca14ee5825e98ebf563d8ffec4ee AS build

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
