# The api and worker images for local compose. CI builds the cluster's images
# with ko from .ko.yaml; entrypoint, user and base match it.
FROM docker.io/library/golang:1.26-alpine@sha256:8ac98ca534ac3f51e1f420a1dd2c15e74c75cfa0f23f3ad27eb5d7236c349a0c AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/api ./cmd/api && \
    CGO_ENABLED=0 go build -trimpath -o /out/worker ./cmd/worker

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab AS api
COPY --from=build /out/api /ko-app/api
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/ko-app/api"]

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab AS worker
COPY --from=build /out/worker /ko-app/worker
USER 65532:65532
ENTRYPOINT ["/ko-app/worker"]
