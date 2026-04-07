FROM ubuntu:24.04 AS base

LABEL app=safe-agentic
LABEL maintainer="florian"

# Avoid interactive prompts
ENV DEBIAN_FRONTEND=noninteractive
ENV LANG=en_US.UTF-8
ENV LC_ALL=en_US.UTF-8

# =============================================================================
# Layer 1: System packages (changes rarely)
# =============================================================================
RUN apt-get update && apt-get install -y --no-install-recommends \
    git curl wget unzip jq sudo openssh-client gnupg \
    software-properties-common build-essential ca-certificates \
    python3 python3-pip python3-venv \
    locales apt-transport-https \
  && locale-gen en_US.UTF-8 \
  && apt-get clean && rm -rf /var/lib/apt/lists/*

# =============================================================================
# Layer 2: Language runtimes (changes rarely)
# =============================================================================

# Node.js 22 LTS
RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
  && apt-get install -y --no-install-recommends nodejs \
  && apt-get clean && rm -rf /var/lib/apt/lists/*

# Go (latest 1.22+)
ARG GO_VERSION=1.23.4
RUN ARCH=$(dpkg --print-architecture) \
  && curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${ARCH}.tar.gz" | tar -C /usr/local -xz
ENV PATH="/usr/local/go/bin:${PATH}"

# =============================================================================
# Layer 3: SRE tools (changes monthly)
# =============================================================================

# HashiCorp repo (terraform, vault)
RUN curl -fsSL https://apt.releases.hashicorp.com/gpg | gpg --dearmor -o /usr/share/keyrings/hashicorp.gpg \
  && echo "deb [signed-by=/usr/share/keyrings/hashicorp.gpg] https://apt.releases.hashicorp.com $(. /etc/os-release && echo $VERSION_CODENAME) main" \
     > /etc/apt/sources.list.d/hashicorp.list \
  && apt-get update && apt-get install -y --no-install-recommends terraform vault \
  && apt-get clean && rm -rf /var/lib/apt/lists/*

# kubectl
RUN curl -fsSL https://pkgs.k8s.io/core:/stable:/v1.31/deb/Release.key | gpg --dearmor -o /usr/share/keyrings/kubernetes.gpg \
  && echo "deb [signed-by=/usr/share/keyrings/kubernetes.gpg] https://pkgs.k8s.io/core:/stable:/v1.31/deb/ /" \
     > /etc/apt/sources.list.d/kubernetes.list \
  && apt-get update && apt-get install -y --no-install-recommends kubectl \
  && apt-get clean && rm -rf /var/lib/apt/lists/*

# Helm
RUN curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

# AWS CLI v2
RUN ARCH=$(dpkg --print-architecture) \
  && if [ "$ARCH" = "arm64" ]; then AWSARCH="aarch64"; else AWSARCH="x86_64"; fi \
  && curl -fsSL "https://awscli.amazonaws.com/awscli-exe-linux-${AWSARCH}.zip" -o /tmp/awscli.zip \
  && unzip -q /tmp/awscli.zip -d /tmp && /tmp/aws/install && rm -rf /tmp/aws /tmp/awscli.zip

# k9s
RUN ARCH=$(dpkg --print-architecture) \
  && K9S_VERSION=$(curl -fsSL https://api.github.com/repos/derailed/k9s/releases/latest | jq -r .tag_name) \
  && if [ "$ARCH" = "arm64" ]; then K9SARCH="arm64"; else K9SARCH="amd64"; fi \
  && curl -fsSL "https://github.com/derailed/k9s/releases/download/${K9S_VERSION}/k9s_Linux_${K9SARCH}.tar.gz" \
     | tar -C /usr/local/bin -xz k9s

# =============================================================================
# Layer 4: Modern CLI tools (changes monthly)
# =============================================================================
RUN apt-get update && apt-get install -y --no-install-recommends \
    ripgrep fd-find bat fzf \
  && apt-get clean && rm -rf /var/lib/apt/lists/* \
  && ln -sf /usr/bin/fdfind /usr/local/bin/fd \
  && ln -sf /usr/bin/batcat /usr/local/bin/bat

# eza
RUN ARCH=$(dpkg --print-architecture) \
  && EZA_VERSION=$(curl -fsSL https://api.github.com/repos/eza-community/eza/releases/latest | jq -r .tag_name | sed 's/^v//') \
  && if [ "$ARCH" = "arm64" ]; then EZAARCH="aarch64"; else EZAARCH="x86_64"; fi \
  && curl -fsSL "https://github.com/eza-community/eza/releases/download/v${EZA_VERSION}/eza_${EZAARCH}-unknown-linux-gnu.tar.gz" \
     | tar -C /usr/local/bin -xz

# zoxide
RUN curl -fsSL https://raw.githubusercontent.com/ajeetdsouza/zoxide/main/install.sh | bash

# yq
RUN ARCH=$(dpkg --print-architecture) \
  && if [ "$ARCH" = "arm64" ]; then YQARCH="arm64"; else YQARCH="amd64"; fi \
  && curl -fsSL "https://github.com/mikefarah/yq/releases/latest/download/yq_linux_${YQARCH}" -o /usr/local/bin/yq \
  && chmod +x /usr/local/bin/yq

# delta (git-delta)
RUN ARCH=$(dpkg --print-architecture) \
  && DELTA_VERSION=$(curl -fsSL https://api.github.com/repos/dandavison/delta/releases/latest | jq -r .tag_name) \
  && if [ "$ARCH" = "arm64" ]; then DELTAARCH="aarch64"; else DELTAARCH="x86_64"; fi \
  && curl -fsSL "https://github.com/dandavison/delta/releases/download/${DELTA_VERSION}/delta-${DELTA_VERSION}-${DELTAARCH}-unknown-linux-gnu.tar.gz" \
     | tar -C /tmp -xz \
  && mv /tmp/delta-${DELTA_VERSION}-${DELTAARCH}-unknown-linux-gnu/delta /usr/local/bin/delta \
  && rm -rf /tmp/delta-*

# GitHub CLI
RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | dd of=/usr/share/keyrings/githubcli.gpg \
  && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli.gpg] https://cli.github.com/packages stable main" \
     > /etc/apt/sources.list.d/github-cli.list \
  && apt-get update && apt-get install -y --no-install-recommends gh \
  && apt-get clean && rm -rf /var/lib/apt/lists/*

# Starship prompt
RUN curl -fsSL https://starship.rs/install.sh | sh -s -- -y

# =============================================================================
# Layer 5: AI agent CLIs (changes weekly — cache-bust via build arg)
# =============================================================================
ARG CLI_CACHE_BUST=1
RUN npm install -g @anthropic-ai/claude-code @openai/codex

# =============================================================================
# Layer 6: User setup
# =============================================================================

# Create non-root agent user
RUN useradd -m -s /bin/bash -u 1000 agent \
  && echo "agent ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/agent

# Create workspace and cache directories
RUN mkdir -p /workspace \
  && chown agent:agent /workspace

USER agent
WORKDIR /workspace

# Set up home directories for caches
RUN mkdir -p \
  /home/agent/.npm \
  /home/agent/.cache/pip \
  /home/agent/go \
  /home/agent/.terraform.d/plugin-cache \
  /home/agent/.config \
  /home/agent/.ssh \
  /home/agent/.claude \
  /home/agent/.codex

# Copy configs
COPY --chown=agent:agent config/starship.toml /home/agent/.config/starship.toml
COPY --chown=agent:agent config/bashrc /home/agent/.bashrc

# SSH config: don't check host keys for GitHub (we trust the SSH agent)
RUN echo "Host github.com\n  StrictHostKeyChecking accept-new\n  UserKnownHostsFile /home/agent/.ssh/known_hosts" \
    > /home/agent/.ssh/config \
  && chmod 600 /home/agent/.ssh/config

# Go env
ENV GOPATH=/home/agent/go
ENV PATH="/home/agent/go/bin:/home/agent/.local/bin:${PATH}"

# Terraform plugin cache
ENV TF_PLUGIN_CACHE_DIR=/home/agent/.terraform.d/plugin-cache

COPY --chown=agent:agent entrypoint.sh /usr/local/bin/entrypoint.sh
RUN sudo chmod +x /usr/local/bin/entrypoint.sh

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["bash"]
