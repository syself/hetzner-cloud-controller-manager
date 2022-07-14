FROM golang:1.18 AS build
COPY . /src
ENV CGO_ENABLED=0
RUN cd /src && go build .

FROM alpine:3.16
RUN apk add --no-cache ca-certificates
COPY --from=build /src/hetzner-cloud-controller-manager /bin/hetzner-cloud-controller-manager
ENTRYPOINT ["/bin/hetzner-cloud-controller-manager"]
