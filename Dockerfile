FROM scratch
COPY bin/insulatr-x86_64 /insulatr
ENTRYPOINT [ "/insulatr" ]
