FROM ubuntu as builder
RUN apt-get update \
 && apt-get -y install git golang \
 && mkdir -p /go/src
ENV GOPATH=/go
RUN go get github.com/nicholasdille/insulatr
WORKDIR /go/src/github.com/nicholasdille/insulatr
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -tags netgo -ldflags '-w' -o bin/insulatr *.go
RUN ls -l bin

FROM scratch
COPY --from=builder /go/src/github.com/nicholasdille/insulatr/bin/insulatr /insulatr
COPY insulatr.yaml /
ENTRYPOINT [ "/insulatr" ]
