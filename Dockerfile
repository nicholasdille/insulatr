FROM scratch
ARG SUFFIX=x86_64
COPY bin/insulatr-${SUFFIX} /insulatr
COPY insulatr.yaml /
ENTRYPOINT [ "/insulatr" ]
