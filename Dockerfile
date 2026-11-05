# Pinned by digest. Update with:
#   docker buildx imagetools inspect debian:trixie-slim --format '{{.Manifest.Digest}}'
FROM debian:trixie-slim@sha256:a72b4433de1cf7d6bae0c4cee74d0547f58601a643b3375f070c51600d63b6b2 AS base

RUN apt-get update \
    && apt-get install --no-install-recommends -y \
        ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /work
ENTRYPOINT ["nem"]
CMD ["--help"]

# Runtime-only variant. Select with `docker build --target rootless`.
FROM base AS rootless

RUN groupadd --gid 1000 nem \
    && useradd --uid 1000 --gid nem --create-home --shell /bin/bash nem \
    && chown nem:nem /work

# goreleaser dockers_v2 places pre-built binaries in <TARGETPLATFORM>/nem
# within the build context (e.g. linux/amd64/nem, linux/arm64/nem).
ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/nem /usr/local/bin/nem

USER nem
ENV NEM_HOME=/home/nem/.nem

# Default image
FROM base AS toolchain

# Removing any of these packages breaks source-built packages at runtime, not at image build time.
RUN apt-get update \
    && apt-get install --no-install-recommends -y \
        build-essential \
        perl \
        binutils \
        curl \
        bzip2 \
        xz-utils \
        autoconf \
        automake \
        libtool \
        m4 \
        gperf \
        gettext \
        bison \
        flex \
        texinfo \
        patch \
    && rm -rf /var/lib/apt/lists/*

ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/nem /usr/local/bin/nem

ENV NEM_HOME=/root/.nem
