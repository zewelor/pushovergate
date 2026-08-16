FROM --platform=$BUILDPLATFORM golang:1.26.5-trixie@sha256:f1a132429b98724a904e9b3bdbaed399d8f923203c3e5170e6def66d0a7cc04c AS test

WORKDIR /src

COPY go.mod main.go main_test.go ./

RUN unformatted="$(gofmt -l .)" && \
    test -z "$unformatted" || { echo "$unformatted"; exit 1; } && \
    go vet ./... && \
    go test -race ./...

FROM --platform=$BUILDPLATFORM golang:1.26.5-trixie@sha256:f1a132429b98724a904e9b3bdbaed399d8f923203c3e5170e6def66d0a7cc04c AS build

WORKDIR /src

COPY go.mod main.go ./

ARG TARGETOS
ARG TARGETARCH

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /pushovergate .

FROM gcr.io/distroless/static-debian13:nonroot@sha256:f7f8f729987ad0fdf6b05eeeae94b26e6a0f613bdf46feea7fc40f7bd72953e6 AS runtime

COPY --from=build --chown=nonroot:nonroot /pushovergate /pushovergate
COPY --chown=nonroot:nonroot LICENSE /LICENSE

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/pushovergate"]
