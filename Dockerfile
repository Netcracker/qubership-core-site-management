FROM --platform=$BUILDPLATFORM golang:1.27@sha256:65b6f280bf050ec5af12716857e8ea8439d694dbba8f31ceeb7630670071f2bb AS build

WORKDIR /app

COPY site-management-service/ .

RUN go mod download
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -o site-management-service .

FROM ghcr.io/netcracker/qubership-core-base:2.4.0@sha256:803d5b5411835a2f46d577d6fb6810e4a11f9a954a12993e705f2a912db2621d AS run

COPY --chown=10001:0 --chmod=555 --from=build app/site-management-service /app/site-management
COPY --chown=10001:0 --chmod=444 --from=build app/application.yaml /app/
COPY --chown=10001:0 --chmod=444 --from=build app/docs/swagger.json /app/
COPY --chown=10001:0 --chmod=444 --from=build app/docs/swagger.yaml /app/

WORKDIR /app

CMD ["/app/site-management"]