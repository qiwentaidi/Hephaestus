# Local build script supplies the Linux binary. No host credentials or tools
# directories are copied into the execution image.
FROM golang:1.26-bookworm
RUN apt-get update && apt-get install -y --no-install-recommends python3-venv python3-pip python3-requests && rm -rf /var/lib/apt/lists/*
COPY hephaestus-egress /usr/local/bin/hephaestus-egress
ENTRYPOINT ["/usr/local/bin/hephaestus-egress"]
