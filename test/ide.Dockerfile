# GLOBAL ARGS
ARG BIN_IMAGE
ARG IDE_IMAGE

ARG STACK_IMAGE

# FEATURE: install-devpod-bin-image
FROM ${DEVPOD_BIN_IMAGE} as devpod

# FEATURE: install-ide-image
FROM ${IDE_IMAGE} as ide

# CURRENT IMAGE
FROM ${STACK_IMAGE} as base

COPY --from=devpod --chown=root:root /usr/lib/bin/ /usr/lib/bin/
COPY --from=devpod --chown=root:root /usr/lib/config/ /usr/lib/config/
COPY --from=ide --chown=root:root /usr/lib/ide /usr/lib/ide
