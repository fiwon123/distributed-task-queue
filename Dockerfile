# syntax=docker/dockerfile:1.6

# Multi-stage image.
#
#   build   — compiles the API and worker into static binaries.
#   runtime — tiny Alpine image carrying only those binaries (default target).
#   dev     — the full development sandbox (Go, gh, Node, air); kept LAST so the
#             default `docker compose build` still produces the sandbox image
#             that docker-compose.yml's `dev` service expects.
#
# The frontend dashboard is embedded with go:embed, and frontend/dist is committed, so the
# build stage needs no Node toolchain.

# --- build: compile static binaries -------------------------------------------
FROM golang:1.26-bookworm AS build

WORKDIR /src

# Cache module downloads separately from the source copy.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO is disabled so the binaries run on a minimal (musl) base image.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./backend/cmd/api \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/worker ./backend/cmd/worker

# --- runtime: API + worker binaries only --------------------------------------
FROM alpine:3.20 AS runtime

# Non-root user. Alpine's busybox `wget` (used by compose healthchecks) is kept.
RUN adduser -D -H -u 65532 app

COPY --from=build /out/api /usr/local/bin/api
COPY --from=build /out/worker /usr/local/bin/worker

USER app

EXPOSE 8080

# Default to the API; the worker service overrides `command`.
ENTRYPOINT ["/usr/local/bin/api"]

# --- dev sandbox image — distributed-task-queue --------------------------------
# Go toolchain comes from the base image; `air` is installed for fast reload.
# GitHub CLI is pinned to the same version as mise.toml.
FROM golang:1.26-bookworm AS dev

# Build-time ARGs are declared here, AFTER the first FROM. An ARG declared
# before FROM is in scope only for the FROM lines themselves — every
# instruction after FROM sees it empty, so a download URL built from an unset
# version 404s at build time in a way no linter catches.
ARG GH_CLI_VERSION=v2.102.0
ARG TARGETARCH
# Node powers the frontend dashboard (frontend/); keep in sync with mise.toml.
ARG NODE_VERSION=v24.21.0
# Playwright version baked into the screenshot toolchain; keep in sync with
# node_modules/playwright in frontend/package-lock.json (Dependabot bumps there
# mean bumping this ARG too — or `npx playwright install chromium` on first run).
ARG PLAYWRIGHT_VERSION=1.64.0
# The host uid/gid, forwarded as build args by docker-compose.yml so the image
# can create a passwd entry + home matching the host user (kills "I have no
# name!"). Defaults keep a standalone `docker build` working.
ARG HOST_UID=1000
ARG HOST_GID=1000

# Fonts (fontconfig + Latin families) are REQUIRED for `make screenshots`:
# headless Chromium resolves text through fontconfig, and with no fonts it
# renders glyphs as nothing — the black-screenshot bug (issue #75).
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    curl \
    ca-certificates \
    git \
    zsh \
    postgresql-client \
    fontconfig \
    fonts-liberation \
    fonts-dejavu-core \
    && rm -rf /var/lib/apt/lists/*

RUN if [ -z "$TARGETARCH" ]; then \
      case "$(uname -m)" in \
        x86_64|amd64) TARGETARCH=amd64 ;; \
        aarch64|arm64) TARGETARCH=arm64 ;; \
        *) echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;; \
      esac; \
    fi \
    && curl -fsSL "https://github.com/cli/cli/releases/download/${GH_CLI_VERSION}/gh_${GH_CLI_VERSION#v}_linux_${TARGETARCH}.tar.gz" -o /tmp/gh.tar.gz \
    && tar -xzf /tmp/gh.tar.gz -C /tmp \
    && mv "/tmp/gh_${GH_CLI_VERSION#v}_linux_${TARGETARCH}/bin/gh" /usr/local/bin/gh \
    && rm -rf /tmp/gh.tar.gz "/tmp/gh_${GH_CLI_VERSION#v}_linux_${TARGETARCH}" \
    && gh --version

# Node.js for the dashboard build (`make frontend-build`). Installed from the
# official tarball — the same download-and-unpack shape as gh above — so the
# version is pinned here and matched in mise.toml.
RUN if [ -z "$TARGETARCH" ]; then \
      case "$(uname -m)" in \
        x86_64|amd64) TARGETARCH=amd64 ;; \
        aarch64|arm64) TARGETARCH=arm64 ;; \
        *) echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;; \
      esac; \
    fi \
    && case "$TARGETARCH" in \
         amd64) NODE_ARCH=x64 ;; \
         arm64) NODE_ARCH=arm64 ;; \
         *) echo "Unsupported node arch: $TARGETARCH" >&2; exit 1 ;; \
       esac \
    && curl -fsSL "https://nodejs.org/dist/${NODE_VERSION}/node-${NODE_VERSION}-linux-${NODE_ARCH}.tar.xz" -o /tmp/node.tar.xz \
    && tar -xJf /tmp/node.tar.xz -C /usr/local --strip-components=1 \
    && rm -f /tmp/node.tar.xz \
    && node --version && npm --version

# Screenshot toolchain for `make screenshots` (issue #75): bake the Playwright
# Chromium build and its system libraries into the sandbox so the README
# screenshots can be regenerated inside `make opencode` with no extra setup.
# Browsers live at /opt/ms-playwright — INSIDE the image — so the /workspace
# bind mount and the (ephemeral) /home/devuser or /workspace/.xdg-* caches can
# never shadow or lose them. Fonts were installed with the apt block above.
RUN PLAYWRIGHT_BROWSERS_PATH=/opt/ms-playwright \
    npx --yes "playwright@${PLAYWRIGHT_VERSION}" install --with-deps chromium \
    && chmod -R a+rX /opt/ms-playwright \
    && rm -rf /root/.npm /root/.cache/ms-playwright

# Export the baked-in browser location for the runtime user (devuser) too;
# playwright resolves its registry from this env var instead of $HOME.
ENV PLAYWRIGHT_BROWSERS_PATH=/opt/ms-playwright

# Fast reload for the Go dev loop. This runs as root ON PURPOSE — `go install`
# writes /go/bin, which is root-owned; the runtime user only needs to EXECUTE
# the result, and GOPATH/GOCACHE are redirected to /workspace/.go by compose.
RUN go install github.com/air-verse/air@v1.67.4

# --- runtime user (matches the host uid/gid from docker-compose.yml) ---
# The compose service runs as `user: "${HOST_UID}:${HOST_GID}"`. Without a
# passwd entry for that uid (or a home it can write), interactive shells greet
# the user with "I have no name!" and rc files never load. Create devuser if the
# uid is free; if the uid is already claimed by another account, fail the build
# readably instead of silently producing a broken shell.
RUN if ! getent group ${HOST_GID} >/dev/null; then groupadd --gid ${HOST_GID} devuser; fi \
    && if ! getent passwd ${HOST_UID} >/dev/null; then \
         useradd --uid ${HOST_UID} --gid ${HOST_GID} --create-home --shell /usr/bin/zsh devuser; \
       fi \
    && { getent passwd devuser >/dev/null \
         || { echo "ERROR: uid ${HOST_UID} is already used by another account, so 'devuser' cannot be created. Rebuild for a different host uid (make dev-build)." >&2; exit 1; }; } \
    && mkdir -p /home/devuser \
    && chown -R ${HOST_UID}:${HOST_GID} /home/devuser \
    && git config --system --add safe.directory /workspace

# --- dev-sandbox rc (badge + terminal title, loaded for BOTH users) ---
# `make dev-shell` opens an interactive *non-login* bash, which reads only
# ~/.bashrc (HOME comes from the compose environment), never /etc/profile.d.
# So the sourcing hook must live in devuser's rc files — and in root's, so
# `docker compose exec -u root bash` debugging keeps the badge too.
COPY scripts/dev-sandbox-rc.sh /etc/profile.d/00-dev-sandbox.sh
RUN chsh -s /usr/bin/zsh root 2>/dev/null || true \
    && printf '%s\n' '[ -f /etc/profile.d/00-dev-sandbox.sh ] && . /etc/profile.d/00-dev-sandbox.sh' >> /root/.zshrc \
    && printf '%s\n' '[ -f /etc/profile.d/00-dev-sandbox.sh ] && . /etc/profile.d/00-dev-sandbox.sh' >> /root/.bashrc \
    && printf '%s\n' '[ -f /etc/profile.d/00-dev-sandbox.sh ] && . /etc/profile.d/00-dev-sandbox.sh' >> /root/.zprofile \
    && printf '%s\n' '[ -f /etc/profile.d/00-dev-sandbox.sh ] && . /etc/profile.d/00-dev-sandbox.sh' >> /etc/zsh/zshrc \
    && printf '%s\n' '[ -f /etc/profile.d/00-dev-sandbox.sh ] && . /etc/profile.d/00-dev-sandbox.sh' >> /home/devuser/.zshrc \
    && printf '%s\n' '[ -f /etc/profile.d/00-dev-sandbox.sh ] && . /etc/profile.d/00-dev-sandbox.sh' >> /home/devuser/.bashrc \
    && printf '%s\n' '[ -f /etc/profile.d/00-dev-sandbox.sh ] && . /etc/profile.d/00-dev-sandbox.sh' >> /home/devuser/.zprofile \
    && chown ${HOST_UID}:${HOST_GID} \
        /home/devuser/.bashrc /home/devuser/.zshrc /home/devuser/.zprofile

# Default to the non-root user so a service added to docker-compose.yml without
# a `user:` directive can't silently reintroduce root-owned workspace files.
# Compose still pins the exact UID/GID via `user:`.
USER devuser

WORKDIR /workspace

CMD ["sleep", "infinity"]
