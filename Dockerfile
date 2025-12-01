FROM gcr.io/distroless/static-debian12:nonroot

USER nonroot

COPY bin/app-linux-amd64 /app

ENTRYPOINT ["/app"]
CMD ["-port", "8080"]
