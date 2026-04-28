FROM gcr.io/distroless/static-debian12:nonroot
COPY afcli /usr/local/bin/afcli
ENTRYPOINT ["/usr/local/bin/afcli"]
