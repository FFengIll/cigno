ARG IDE=cloud-ide-standard:cranit

FROM ${IDE} as ide

FROM stack-java:cranit

COPY --from=ide --chown=0:0 --chmod=X /usr/lib/ide /usr/lib/ide