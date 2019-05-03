FROM golang:1.11
RUN curl https://glide.sh/get | sh \
 && git clone https://github.com/nicholasdille/insulatr /go/src/github.com/nicholasdille/insulatr \
 && cd /go/src/github.com/nicholasdille/insulatr \
 && glide install \
 && make static \
 && ls -l bin

FROM scratch
COPY --from=builder /go/src/github.com/nicholasdille/insulatr/bin/insulatr-$(uname -m) /insulatr
ENTRYPOINT [ "/insulatr" ]
