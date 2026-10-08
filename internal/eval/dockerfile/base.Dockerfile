# Tools-only base image for sandboxed evals.
#
# Everything here is stable, build-time, root-owned content: a POSIX shell,
# git, gh, kiro-cli and the unprivileged `sandbox` user. Nothing that varies
# per project or per eval run is baked in. The harness supplies that at run
# time via bind mounts (see docs/evaluation.md), so this file and the build
# args below are the only inputs to the image cache key.
FROM alpine:3.19

# Pinned tool inputs, supplied by sandbox.ToolSet.BuildArgs.
ARG KIRO_CLI_URL
ARG GH_VERSION
ARG GH_ARCH

# curl and unzip are build-only and removed in the same layer.
RUN set -eux; \
    test -n "${KIRO_CLI_URL}" && test -n "${GH_VERSION}" && test -n "${GH_ARCH}"; \
    apk add --no-cache git bash ca-certificates; \
    apk add --no-cache --virtual .build-deps curl unzip; \
    cd /tmp; \
    curl -fsSL "${KIRO_CLI_URL}" -o kirocli.zip; \
    unzip -q kirocli.zip; \
    install -m 0755 kirocli/bin/kiro-cli /usr/local/bin/kiro-cli; \
    install -m 0755 kirocli/bin/kiro-cli-chat /usr/local/bin/kiro-cli-chat; \
    curl -fsSL "https://github.com/cli/cli/releases/download/v${GH_VERSION}/gh_${GH_VERSION}_linux_${GH_ARCH}.tar.gz" -o gh.tar.gz; \
    tar -xzf gh.tar.gz; \
    install -m 0755 "gh_${GH_VERSION}_linux_${GH_ARCH}/bin/gh" /usr/local/bin/gh; \
    rm -rf /tmp/kirocli /tmp/kirocli.zip /tmp/gh.tar.gz "/tmp/gh_${GH_VERSION}_linux_${GH_ARCH}"; \
    apk del .build-deps

# Unprivileged user. uid 1000 is part of the mount/ownership contract.
RUN adduser -D -u 1000 -s /bin/bash sandbox && \
    mkdir -p /workspace && \
    chown sandbox:sandbox /workspace

# Smoke test: fail the build if a baked tool is not runnable.
RUN kiro-cli --version && gh --version && git --version && sh -c true

WORKDIR /workspace

USER sandbox

CMD ["/bin/bash"]
