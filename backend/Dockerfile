# The api and load images for local compose. CI builds the cluster's images
# with ko from .ko.yaml; entrypoint, user and base match it.
FROM docker.io/library/golang:1.27-alpine@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/api ./cmd/api && \
    CGO_ENABLED=0 go build -trimpath -o /out/load ./cmd/load

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab AS api
COPY --from=build /out/api /ko-app/api
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/ko-app/api"]

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab AS load
COPY --from=build /out/load /ko-app/load
USER 65532:65532
ENTRYPOINT ["/ko-app/load"]
