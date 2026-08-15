FROM golang:1.27rc2-alpine3.23@sha256:f12c2dc8d14504742f545658e8e49e09e545f2e396788b49797c8052f53434ba AS build

WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY api ./api
COPY cmd ./cmd
COPY internal ./internal

ARG TARGETOS=linux
ARG TARGETARCH
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/pve-storage-guard ./cmd/pve-storage-guard

FROM gcr.io/distroless/static-debian13:nonroot@sha256:f7f8f729987ad0fdf6b05eeeae94b26e6a0f613bdf46feea7fc40f7bd72953e6

COPY --from=build /out/pve-storage-guard /usr/local/bin/pve-storage-guard
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/pve-storage-guard"]
