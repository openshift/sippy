FROM registry.fedoraproject.org/fedora:43

ARG GOLANGCI_LINT_VERSION=2.10.1

RUN dnf install -y \
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
    golang \
    npm \
    python3 \
    libstdc++ \
    postgresql \
    chromium-headless \
    which \
    && dnf clean all

RUN ARCH=$(uname -m) && \
    if [ "$ARCH" = "aarch64" ]; then LINT_ARCH="arm64"; else LINT_ARCH="amd64"; fi && \
    rpm -i "https://github.com/golangci/golangci-lint/releases/download/v${GOLANGCI_LINT_VERSION}/golangci-lint-${GOLANGCI_LINT_VERSION}-linux-${LINT_ARCH}.rpm"

ARG GH_VERSION=2.92.0
RUN ARCH=$(uname -m) && \
    if [ "$ARCH" = "aarch64" ]; then GH_ARCH="arm64"; else GH_ARCH="amd64"; fi && \
    curl -fsSL "https://github.com/cli/cli/releases/download/v${GH_VERSION}/gh_${GH_VERSION}_linux_${GH_ARCH}.tar.gz" \
    | tar xz -C /tmp && \
    mv "/tmp/gh_${GH_VERSION}_linux_${GH_ARCH}/bin/gh" /usr/local/bin/gh && \
    chmod +x /usr/local/bin/gh

RUN ARCH=$(uname -m) && \
    if [ "$ARCH" = "aarch64" ]; then GCLOUD_ARCH="arm"; else GCLOUD_ARCH="x86_64"; fi && \
    curl -fsSL "https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-cli-linux-${GCLOUD_ARCH}.tar.gz" \
    | tar xz -C /opt && \
    /opt/google-cloud-sdk/install.sh --quiet --path-update=false && \
    ln -s /opt/google-cloud-sdk/bin/gcloud /usr/local/bin/gcloud && \
    ln -s /opt/google-cloud-sdk/bin/gsutil /usr/local/bin/gsutil && \
    ln -s /opt/google-cloud-sdk/bin/bq /usr/local/bin/bq

RUN groupadd -g 1000 vscode && \
    useradd -u 1000 -g 0 -G 1000 -m -s /bin/bash vscode && \
    echo "vscode ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/vscode && \
    git config --system --add safe.directory /workspace

RUN chgrp -R 0 /home/vscode && chmod -R g=u /home/vscode

ENV GOPATH=/home/vscode/go
ENV PATH="/home/vscode/.local/bin:${GOPATH}/bin:/usr/local/go/bin:${PATH}"
ENV CHROME_PATH=/usr/lib64/chromium-browser/headless_shell

RUN curl -LsSf https://astral.sh/uv/install.sh | env INSTALLER_NO_MODIFY_PATH=1 UV_INSTALL_DIR=/usr/local/bin sh

USER vscode

RUN go install gotest.tools/gotestsum@latest

# Cursor's install script calls base64 -D (macOS flag) but GNU coreutils uses -d
RUN sudo cp /usr/bin/base64 /usr/bin/base64.gnu && \
    printf '#!/bin/sh\nfor a in "$@"; do shift; case "$a" in -D) set -- "$@" -d;; *) set -- "$@" "$a";; esac; done\nexec /usr/bin/base64.gnu "$@"\n' | sudo tee /usr/bin/base64 > /dev/null && \
    sudo chmod +x /usr/bin/base64

WORKDIR /workspace
