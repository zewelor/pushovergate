FROM --platform=$BUILDPLATFORM golang:1.27.4-trixie@sha256:b54b2639de8c1bfe538ec5ffc45f29b1932235ac5f6d63c059853646677112d9 AS test

WORKDIR /src

COPY go.mod main.go main_test.go ./

RUN go mod tidy -diff && \
    unformatted="$(gofmt -l .)" && \
    test -z "$unformatted" || { echo "$unformatted"; exit 1; } && \
    go vet ./... && \
    go test -race ./...

FROM --platform=$BUILDPLATFORM golang:1.27.4-trixie@sha256:b54b2639de8c1bfe538ec5ffc45f29b1932235ac5f6d63c059853646677112d9 AS build

WORKDIR /src

COPY go.mod main.go ./

ARG TARGETOS
ARG TARGETARCH

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /pushovergate .

# Use distroless static to provide root CA certificates for Pushover TLS and a nonroot user.
# This avoids manual CA cert copying and raw UID configuration required by scratch.
FROM gcr.io/distroless/static-debian13:nonroot@sha256:aafe993665bf6200db38a02c11a9351e782a84b6a45e7f33b6695638c88e3931 AS runtime

COPY --from=build --chown=nonroot:nonroot /pushovergate /pushovergate
COPY --chown=nonroot:nonroot LICENSE /LICENSE

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/pushovergate"]
