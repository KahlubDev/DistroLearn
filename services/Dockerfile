# Go services: api and workers from one module, two images.
#
# Build from the repository root so the whole services/ module is in context:
#   docker build -f services/Dockerfile --target api     -t distrolearn-api:sha .
#   docker build -f services/Dockerfile --target workers -t distrolearn-workers:sha .

# Pinned by digest. Verified 2026-10-09 against the registry index.
ARG GO_IMAGE=golang:1.27.1@sha256:162be5298a40ed317005c8339c6de4d10d3eef336d66dc8e9259b03ab9d3a6d2
# distroless/static-debian12:nonroot. The base already sets USER 65532, verified by
# reading the image config, so the runtime holds no shell and no package manager.
ARG RUNTIME_IMAGE=gcr.io/distroless/static-debian12@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

FROM ${GO_IMAGE} AS build
ARG TARGET=api
ARG VERSION=dev
WORKDIR /src

# The module has no external dependencies yet, so go.mod alone is the whole graph.
# That changes once a Postgres driver or the goose migration tool lands.
COPY services/go.mod ./
COPY services/ ./

# CGO off so the binary is static and needs no libc in the runtime image.
# -trimpath keeps build paths out of the binary. -s -w drop the symbol table and DWARF.
RUN CGO_ENABLED=0 GOOS=linux go build \
      -trimpath \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o /out/service ./cmd/${TARGET}

# Fail the build rather than ship an image whose entrypoint is missing.
RUN test -x /out/service

FROM ${RUNTIME_IMAGE} AS api
COPY --from=build /out/service /service
# Explicit, so the runtime user does not depend on base image behaviour. 65532 is
# nonroot. Ports 8080 and 8081 are above 1024, so no CAP_NET_BIND_SERVICE is needed.
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/service"]

FROM ${RUNTIME_IMAGE} AS workers
COPY --from=build /out/service /service
USER 65532:65532
EXPOSE 8081
ENTRYPOINT ["/service"]