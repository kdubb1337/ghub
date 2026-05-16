FROM gcr.io/distroless/static-debian12
COPY ghub /usr/local/bin/ghub
ENTRYPOINT ["/usr/local/bin/ghub"]
