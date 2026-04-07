# Pin base image by digest for supply-chain reproducibility.
# To update: docker pull ubuntu:24.04 && docker inspect --format='{{index .RepoDigests 0}}' ubuntu:24.04
FROM ubuntu:24.04@sha256:84e77dee7d1bc93fb029a45e3c6cb9d8aa4831ccfcc7103d36e876938d28895b

LABEL app=safe-agentic
LABEL maintainer="florian"

ENV DEBIAN_FRONTEND=noninteractive
ENV LANG=en_US.UTF-8
ENV LC_ALL=en_US.UTF-8

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

ARG GO_VERSION=1.23.4
ARG GO_SHA256_AMD64=6924efde5de86fe277676e929dc9917d466efa02fb934197bc2eba35d5680971
ARG GO_SHA256_ARM64=16e5017863a7f6071363782b1b8042eb12c6ca4f4cd71528b2123f0a1275b13e

ARG HELM_VERSION=4.1.3
ARG HELM_SHA256_AMD64=02ce9722d541238f81459938b84cf47df2fdf1187493b4bfb2346754d82a4700
ARG HELM_SHA256_ARM64=5db45e027cc8de4677ec869e5d803fc7631b0bab1c1eb62ac603a62d22359a43

ARG EZA_VERSION=0.23.4
ARG EZA_SHA256_AMD64=0c38665440226cd8bef5d1d4f3bc6ff77c927fb0d68b752739105db7ab5b358d
ARG EZA_SHA256_ARM64=366e8430225f9955c3dc659b452150c169894833ccfef455e01765e265a3edda

ARG ZOXIDE_VERSION=0.9.9
ARG ZOXIDE_SHA256_AMD64=5ab14485571b00a2cd0d1f2e910f6bcd57ab4da59e4c9e974a9530c3d8ba23a3
ARG ZOXIDE_SHA256_ARM64=6ed2562bb8fad59e794ae5ea43eef9c5c61627744a3cc4cb93b75f56246d9338

ARG YQ_VERSION=4.52.5
ARG YQ_SHA256_AMD64=75d893a0d5940d1019cb7cdc60001d9e876623852c31cfc6267047bc31149fa9
ARG YQ_SHA256_ARM64=90fa510c50ee8ca75544dbfffed10c88ed59b36834df35916520cddc623d9aaa

ARG DELTA_VERSION=0.19.2
ARG DELTA_SHA256_AMD64=ea4f0222950ee750a3d38dd80d03bce4cee07a3f63928fc47548383bcaf23093
ARG DELTA_SHA256_ARM64=0edc36cf514f1bd84becac3e94ee8ae9f8818c6a1f99f7b2ee67b362afa253d3

ARG AWSCLI_VERSION=2.34.25

RUN apt-get update \
 && apt-get install -y --no-install-recommends \
    apt-transport-https \
    build-essential \
    ca-certificates \
    curl \
    fd-find \
    fzf \
    git \
    gnupg \
    jq \
    locales \
    openssh-client \
    python3 \
    python3-pip \
    python3-venv \
    ripgrep \
    software-properties-common \
    unzip \
    wget \
 && locale-gen en_US.UTF-8 \
 && apt-get clean \
 && rm -rf /var/lib/apt/lists/*

RUN install -m 0755 -d /etc/apt/keyrings \
 && curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key \
    | gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg \
 && echo "deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_22.x nodistro main" \
    > /etc/apt/sources.list.d/nodesource.list \
 && curl -fsSL https://apt.releases.hashicorp.com/gpg \
    | gpg --dearmor -o /usr/share/keyrings/hashicorp.gpg \
 && echo "deb [signed-by=/usr/share/keyrings/hashicorp.gpg] https://apt.releases.hashicorp.com $(. /etc/os-release && echo "$VERSION_CODENAME") main" \
    > /etc/apt/sources.list.d/hashicorp.list \
 && curl -fsSL https://pkgs.k8s.io/core:/stable:/v1.31/deb/Release.key \
    | gpg --dearmor -o /usr/share/keyrings/kubernetes.gpg \
 && echo "deb [signed-by=/usr/share/keyrings/kubernetes.gpg] https://pkgs.k8s.io/core:/stable:/v1.31/deb/ /" \
    > /etc/apt/sources.list.d/kubernetes.list \
 && curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
    | gpg --dearmor -o /usr/share/keyrings/githubcli.gpg \
 && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli.gpg] https://cli.github.com/packages stable main" \
    > /etc/apt/sources.list.d/github-cli.list \
 && apt-get update \
 && apt-get install -y --no-install-recommends \
    bat \
    gh \
    kubectl \
    nodejs \
    terraform \
    vault \
 && apt-get clean \
 && rm -rf /var/lib/apt/lists/*

RUN ARCH=$(dpkg --print-architecture) \
 && case "$ARCH" in \
      amd64) GO_ARCH=amd64; GO_SHA256="$GO_SHA256_AMD64" ;; \
      arm64) GO_ARCH=arm64; GO_SHA256="$GO_SHA256_ARM64" ;; \
      *) echo "Unsupported Go arch: $ARCH" >&2; exit 1 ;; \
    esac \
 && curl -fsSLo /tmp/go.tgz "https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz" \
 && echo "${GO_SHA256}  /tmp/go.tgz" | sha256sum -c - \
 && tar -C /usr/local -xzf /tmp/go.tgz \
 && rm -f /tmp/go.tgz \
 && ln -sf /usr/bin/fdfind /usr/local/bin/fd \
 && ln -sf /usr/bin/batcat /usr/local/bin/bat

RUN ARCH=$(dpkg --print-architecture) \
 && case "$ARCH" in \
      amd64) HELM_ARCH=amd64; HELM_SHA256="$HELM_SHA256_AMD64" ;; \
      arm64) HELM_ARCH=arm64; HELM_SHA256="$HELM_SHA256_ARM64" ;; \
      *) echo "Unsupported Helm arch: $ARCH" >&2; exit 1 ;; \
    esac \
 && curl -fsSLo /tmp/helm.tgz "https://get.helm.sh/helm-v${HELM_VERSION}-linux-${HELM_ARCH}.tar.gz" \
 && echo "${HELM_SHA256}  /tmp/helm.tgz" | sha256sum -c - \
 && tar -C /tmp -xzf /tmp/helm.tgz \
 && install -m 0755 "/tmp/linux-${HELM_ARCH}/helm" /usr/local/bin/helm \
 && rm -rf /tmp/helm.tgz "/tmp/linux-${HELM_ARCH}"

RUN cat > /tmp/awscli-public-key.asc <<'EOF'
-----BEGIN PGP PUBLIC KEY BLOCK-----

mQINBF2Cr7UBEADJZHcgusOJl7ENSyumXh85z0TRV0xJorM2B/JL0kHOyigQluUG
ZMLhENaG0bYatdrKP+3H91lvK050pXwnO/R7fB/FSTouki4ciIx5OuLlnJZIxSzx
PqGl0mkxImLNbGWoi6Lto0LYxqHN2iQtzlwTVmq9733zd3XfcXrZ3+LblHAgEt5G
TfNxEKJ8soPLyWmwDH6HWCnjZ/aIQRBTIQ05uVeEoYxSh6wOai7ss/KveoSNBbYz
gbdzoqI2Y8cgH2nbfgp3DSasaLZEdCSsIsK1u05CinE7k2qZ7KgKAUIcT/cR/grk
C6VwsnDU0OUCideXcQ8WeHutqvgZH1JgKDbznoIzeQHJD238GEu+eKhRHcz8/jeG
94zkcgJOz3KbZGYMiTh277Fvj9zzvZsbMBCedV1BTg3TqgvdX4bdkhf5cH+7NtWO
lrFj6UwAsGukBTAOxC0l/dnSmZhJ7Z1KmEWilro/gOrjtOxqRQutlIqG22TaqoPG
fYVN+en3Zwbt97kcgZDwqbuykNt64oZWc4XKCa3mprEGC3IbJTBFqglXmZ7l9ywG
EEUJYOlb2XrSuPWml39beWdKM8kzr1OjnlOm6+lpTRCBfo0wa9F8YZRhHPAkwKkX
XDeOGpWRj4ohOx0d2GWkyV5xyN14p2tQOCdOODmz80yUTgRpPVQUtOEhXQARAQAB
tCFBV1MgQ0xJIFRlYW0gPGF3cy1jbGlAYW1hem9uLmNvbT6JAlQEEwEIAD4CGwMF
CwkIBwIGFQoJCAsCBBYCAwECHgECF4AWIQT7Xbd/1cEYuAURraimMQrMRnJHXAUC
aGveYQUJDMpiLAAKCRCmMQrMRnJHXKBYD/9Ab0qQdGiO5hObchG8xh8Rpb4Mjyf6
0JrVo6m8GNjNj6BHkSc8fuTQJ/FaEhaQxj3pjZ3GXPrXjIIVChmICLlFuRXYzrXc
Pw0lniybypsZEVai5kO0tCNBCCFuMN9RsmmRG8mf7lC4FSTbUDmxG/QlYK+0IV/l
uJkzxWa+rySkdpm0JdqumjegNRgObdXHAQDWlubWQHWyZyIQ2B4U7AxqSpcdJp6I
S4Zds4wVLd1WE5pquYQ8vS2cNlDm4QNg8wTj58e3lKN47hXHMIb6CHxRnb947oJa
pg189LLPR5koh+EorNkA1wu5mAJtJvy5YMsppy2y/kIjp3lyY6AmPT1posgGk70Z
CmToEZ5rbd7ARExtlh76A0cabMDFlEHDIK8RNUOSRr7L64+KxOUegKBfQHb9dADY
qqiKqpCbKgvtWlds909Ms74JBgr2KwZCSY1HaOxnIr4CY43QRqAq5YHOay/mU+6w
hhmdF18vpyK0vfkvvGresWtSXbag7Hkt3XjaEw76BzxQH21EBDqU8WJVjHgU6ru+
DJTs+SxgJbaT3hb/vyjlw0lK+hFfhWKRwgOXH8vqducF95NRSUxtS4fpqxWVaw3Q
V2OWSjbne99A5EPEySzryFTKbMGwaTlAwMCwYevt4YT6eb7NmFhTx0Fis4TalUs+
j+c7Kg92pDx2uQ==
=OBAt
-----END PGP PUBLIC KEY BLOCK-----
EOF

RUN export GNUPGHOME="$(mktemp -d)" \
 && gpg --batch --import /tmp/awscli-public-key.asc \
 && ARCH=$(dpkg --print-architecture) \
 && case "$ARCH" in \
      amd64) AWSCLI_ARCH=x86_64 ;; \
      arm64) AWSCLI_ARCH=aarch64 ;; \
      *) echo "Unsupported AWS CLI arch: $ARCH" >&2; exit 1 ;; \
    esac \
 && curl -fsSLo /tmp/awscliv2.zip "https://awscli.amazonaws.com/awscli-exe-linux-${AWSCLI_ARCH}-${AWSCLI_VERSION}.zip" \
 && curl -fsSLo /tmp/awscliv2.sig "https://awscli.amazonaws.com/awscli-exe-linux-${AWSCLI_ARCH}-${AWSCLI_VERSION}.zip.sig" \
 && gpg --batch --verify /tmp/awscliv2.sig /tmp/awscliv2.zip \
 && unzip -q /tmp/awscliv2.zip -d /tmp \
 && /tmp/aws/install \
 && rm -rf "$GNUPGHOME" /tmp/aws /tmp/awscliv2.zip /tmp/awscliv2.sig /tmp/awscli-public-key.asc

RUN ARCH=$(dpkg --print-architecture) \
 && case "$ARCH" in \
      amd64) EZA_ARCH=x86_64; EZA_SHA256="$EZA_SHA256_AMD64" ;; \
      arm64) EZA_ARCH=aarch64; EZA_SHA256="$EZA_SHA256_ARM64" ;; \
      *) echo "Unsupported eza arch: $ARCH" >&2; exit 1 ;; \
    esac \
 && curl -fsSLo /tmp/eza.tgz "https://github.com/eza-community/eza/releases/download/v${EZA_VERSION}/eza_${EZA_ARCH}-unknown-linux-gnu.tar.gz" \
 && echo "${EZA_SHA256}  /tmp/eza.tgz" | sha256sum -c - \
 && tar -C /tmp -xzf /tmp/eza.tgz ./eza \
 && install -m 0755 /tmp/eza /usr/local/bin/eza \
 && rm -f /tmp/eza /tmp/eza.tgz

RUN ARCH=$(dpkg --print-architecture) \
 && case "$ARCH" in \
      amd64) ZOXIDE_DEB="zoxide_${ZOXIDE_VERSION}-1_amd64.deb"; ZOXIDE_SHA256="$ZOXIDE_SHA256_AMD64" ;; \
      arm64) ZOXIDE_DEB="zoxide_${ZOXIDE_VERSION}-1_arm64.deb"; ZOXIDE_SHA256="$ZOXIDE_SHA256_ARM64" ;; \
      *) echo "Unsupported zoxide arch: $ARCH" >&2; exit 1 ;; \
    esac \
 && curl -fsSLo /tmp/zoxide.deb "https://github.com/ajeetdsouza/zoxide/releases/download/v${ZOXIDE_VERSION}/${ZOXIDE_DEB}" \
 && echo "${ZOXIDE_SHA256}  /tmp/zoxide.deb" | sha256sum -c - \
 && dpkg -i /tmp/zoxide.deb \
 && rm -f /tmp/zoxide.deb

RUN ARCH=$(dpkg --print-architecture) \
 && case "$ARCH" in \
      amd64) YQ_ARCH=amd64; YQ_SHA256="$YQ_SHA256_AMD64" ;; \
      arm64) YQ_ARCH=arm64; YQ_SHA256="$YQ_SHA256_ARM64" ;; \
      *) echo "Unsupported yq arch: $ARCH" >&2; exit 1 ;; \
    esac \
 && curl -fsSLo /usr/local/bin/yq "https://github.com/mikefarah/yq/releases/download/v${YQ_VERSION}/yq_linux_${YQ_ARCH}" \
 && echo "${YQ_SHA256}  /usr/local/bin/yq" | sha256sum -c - \
 && chmod +x /usr/local/bin/yq

RUN ARCH=$(dpkg --print-architecture) \
 && case "$ARCH" in \
      amd64) DELTA_DEB="git-delta_${DELTA_VERSION}_amd64.deb"; DELTA_SHA256="$DELTA_SHA256_AMD64" ;; \
      arm64) DELTA_DEB="git-delta_${DELTA_VERSION}_arm64.deb"; DELTA_SHA256="$DELTA_SHA256_ARM64" ;; \
      *) echo "Unsupported delta arch: $ARCH" >&2; exit 1 ;; \
    esac \
 && curl -fsSLo /tmp/delta.deb "https://github.com/dandavison/delta/releases/download/${DELTA_VERSION}/${DELTA_DEB}" \
 && echo "${DELTA_SHA256}  /tmp/delta.deb" | sha256sum -c - \
 && dpkg -i /tmp/delta.deb \
 && rm -f /tmp/delta.deb

RUN if id -u agent >/dev/null 2>&1; then \
      :; \
    elif getent passwd 1000 >/dev/null; then \
      existing_user="$(getent passwd 1000 | cut -d: -f1)"; \
      usermod -l agent -d /home/agent -m "$existing_user"; \
      if getent group "$existing_user" >/dev/null; then \
        groupmod -n agent "$existing_user"; \
      fi; \
    else \
      useradd -m -s /bin/bash -u 1000 agent; \
    fi \
 && usermod -g agent -G "" agent \
 && mkdir -p /workspace /opt/agent-cli \
 && chown -R 1000:1000 /workspace /home/agent /opt/agent-cli

COPY --chmod=644 bin/repo-url.sh /usr/local/lib/safe-agentic/repo-url.sh
COPY --chmod=755 entrypoint.sh /usr/local/bin/entrypoint.sh

USER agent

COPY --chown=agent:agent package.json package-lock.json /opt/agent-cli/

WORKDIR /opt/agent-cli
ARG CLI_CACHE_BUST=1
RUN test -n "$CLI_CACHE_BUST" \
 && npm ci --omit=dev \
 && npm cache clean --force

# Remove build-essential (gcc, make, etc.) now that native npm modules are compiled.
# Reduces attack surface — agents work with interpreted languages and pre-compiled Go binaries.
USER root
RUN apt-get purge -y --auto-remove build-essential cpp gcc g++ make dpkg-dev \
 && apt-get clean \
 && rm -rf /var/lib/apt/lists/*
USER agent

WORKDIR /workspace

RUN mkdir -p \
  /home/agent/.npm \
  /home/agent/.cache/pip \
  /home/agent/go \
  /home/agent/.terraform.d/plugin-cache \
  /home/agent/.config \
  /home/agent/.ssh \
  /home/agent/.claude \
  /home/agent/.codex

COPY --chown=agent:agent config/bashrc /home/agent/.bashrc

RUN mkdir -p /home/agent/.ssh.baked && { \
    echo "github.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl"; \
    echo "github.com ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBEmKSENjQEezOmxkZMy7opKgwFB9nkt5YRrYMjNuG5N87uRgg6CLrbo5wAdT/y6v0mKV0U2w0WZ2YB/++Tpockg="; \
    echo "github.com ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQCj7ndNxQowgcQnjshcLrqPEiiphnt+VTTvDP6mHBL9j1aNUkY4Ue1gvwnGLVlOhGeYrnZaMgRK6+PKCUXaDbC7qtbW8gIkhL7aGCsOr/C56SJMy/BCZfxd1nWzAOxSDPgVsmerOBYfNqltV9/hWCqBywINIR+5dIg6JTJ72pcEpEjcYgXkE2YCZEN+Mu0uCOIiAFkwj+JlMnZmQpW/xfjNHJt6KDi05czWEG+DiLiKC/t6M1SNpckHI5FmmTBV12KuMSg7DXKIHJTI+AY8VqVG/0cOHsKDMPsGk8mvHjEQ3YfPFW4CeLFe3MYi+OwNfsm86B75bDS6LwPNrVhk/6MQIT79ItA3R/CuPL2KhPNzJuGhiPCaZQvIClz9RVXR5GOPGnkNxBWMmJMXFiHj6GPRSMx3E4hxarUELzuI3tOpzq3FanQw+xzwHM2ujfBjX6y4Ebj3IfEyNUAnUL7gRUk/dKvFC0dqDkDpPfAh/aLNT5bbr7SOJYO/Q5Ck="; \
  } > /home/agent/.ssh.baked/known_hosts \
 && printf "Host github.com\n  StrictHostKeyChecking yes\n  UserKnownHostsFile /home/agent/.ssh/known_hosts\n" \
    > /home/agent/.ssh.baked/config \
 && chmod 600 /home/agent/.ssh.baked/*

ENV GOPATH=/home/agent/go
ENV GIT_CONFIG_GLOBAL=/home/agent/.config/git/config
ENV TF_PLUGIN_CACHE_DIR=/home/agent/.terraform.d/plugin-cache
ENV PATH="/opt/agent-cli/node_modules/.bin:/home/agent/go/bin:/home/agent/.local/bin:${PATH}"

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["bash"]
