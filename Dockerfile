FROM --platform=$BUILDPLATFORM golang:1.27.4@sha256:205c7636d74e0ffbe3d007162df7d74f859f5a8c8322dfb9f907977686c126cf AS build

WORKDIR /app

COPY site-management-service/ .

RUN go mod download
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -o site-management-service .

FROM ghcr.io/netcracker/qubership-core-base:alpine-2.6.1@sha256:7be670fc57c243195c63597106413e1b0365381c2d186a71cb6d12fcffc2278b AS run

COPY --chown=10001:0 --chmod=555 --from=build app/site-management-service /app/site-management
COPY --chown=10001:0 --chmod=444 --from=build app/application.yaml /app/
COPY --chown=10001:0 --chmod=444 --from=build app/docs/swagger.json /app/
COPY --chown=10001:0 --chmod=444 --from=build app/docs/swagger.yaml /app/

WORKDIR /app

CMD ["/app/site-management"]