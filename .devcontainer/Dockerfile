FROM registry.access.redhat.com/ubi9/ubi:latest

ARG GOLANGCI_LINT_VERSION=1.64.8

RUN dnf module enable nodejs:18 -y && \
    dnf install -y --allowerasing \
    git \
    make \
    jq \
    tar \
    gzip \
    wget \
    vim-minimal \
    findutils \
    procps-ng \
    sudo \
    go \
    npm \
    python3.12 \
    libstdc++ \
    postgresql \
    && dnf clean all

RUN ARCH=$(uname -m) && \
    if [ "$ARCH" = "aarch64" ]; then LINT_ARCH="arm64"; else LINT_ARCH="amd64"; fi && \
    rpm -i "https://github.com/golangci/golangci-lint/releases/download/v${GOLANGCI_LINT_VERSION}/golangci-lint-${GOLANGCI_LINT_VERSION}-linux-${LINT_ARCH}.rpm"

RUN ARCH=$(uname -m) && \
    if [ "$ARCH" = "aarch64" ]; then GCLOUD_ARCH="arm"; else GCLOUD_ARCH="x86_64"; fi && \
    curl -fsSL "https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-cli-linux-${GCLOUD_ARCH}.tar.gz" \
    | tar xz -C /opt && \
    /opt/google-cloud-sdk/install.sh --quiet --path-update=false && \
    ln -s /opt/google-cloud-sdk/bin/gcloud /usr/local/bin/gcloud && \
    ln -s /opt/google-cloud-sdk/bin/gsutil /usr/local/bin/gsutil && \
    ln -s /opt/google-cloud-sdk/bin/bq /usr/local/bin/bq

RUN groupadd -g 1000 vscode && \
    useradd -u 1000 -g 1000 -m -s /bin/bash vscode && \
    echo "vscode ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/vscode

ENV GOPATH=/home/vscode/go
ENV PATH="/home/vscode/.local/bin:${GOPATH}/bin:/usr/local/go/bin:${PATH}"

USER vscode

RUN go install gotest.tools/gotestsum@latest

# Cursor's install script calls base64 -D (macOS flag) but GNU coreutils uses -d
RUN sudo cp /usr/bin/base64 /usr/bin/base64.gnu && \
    printf '#!/bin/sh\nfor a in "$@"; do shift; case "$a" in -D) set -- "$@" -d;; *) set -- "$@" "$a";; esac; done\nexec /usr/bin/base64.gnu "$@"\n' | sudo tee /usr/bin/base64 > /dev/null && \
    sudo chmod +x /usr/bin/base64

# No versioned URL available; bust cache with CLAUDE_INSTALLER_VERSION to force a rebuild
ARG CLAUDE_INSTALLER_VERSION=latest
RUN curl -fsSL https://claude.ai/install.sh | sh

WORKDIR /workspace
