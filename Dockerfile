# Warning: this Dockerfile only packages the binary built on the host (the
# runtimes and the reverse proxy are embedded in it), so build it with `just
# docker`, which builds the binary first.
FROM alpine:3.24

COPY tinyfaas-linux-amd64 /usr/local/bin/tinyfaas

WORKDIR /var/lib/tinyfaas

# Management service, reverse proxy (with /metrics), HTTP and gRPC endpoints.
EXPOSE 8080 8081 8000 9000

ENTRYPOINT ["/usr/local/bin/tinyfaas"]
