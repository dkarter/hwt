FROM golang:1.25.0-bookworm

ARG TARGETARCH
ARG HERDR_VERSION=0.9.0
RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates curl git jq \
  && case "$TARGETARCH" in \
    amd64) asset=x86_64; checksum=4fa1a01158dd8043da92d31b270780b0dcc10603038d9b61cac4d81ab63fb71f ;; \
    arm64) asset=aarch64; checksum=9c8db20fb7e7427b138d5367113f1621ffd319f2f65d6f009e2594029115f0d2 ;; \
    *) echo "unsupported architecture: $TARGETARCH" >&2; exit 1 ;; \
  esac \
  && curl -fsSLo /usr/local/bin/herdr-real "https://github.com/herdrdev/herdr/releases/download/v${HERDR_VERSION}/herdr-linux-${asset}" \
  && echo "${checksum}  /usr/local/bin/herdr-real" | sha256sum -c - \
  && chmod 0755 /usr/local/bin/herdr-real \
  && apt-get purge -y --auto-remove curl \
  && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY e2e/herdr/entrypoint.sh /usr/local/bin/hwt-herdr-e2e
ENTRYPOINT ["/usr/local/bin/hwt-herdr-e2e"]
