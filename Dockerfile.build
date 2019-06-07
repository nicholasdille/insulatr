FROM golang:1.11 as builder
ARG REF
RUN curl --silent https://glide.sh/get | sh
RUN git clone https://github.com/nicholasdille/insulatr /go/src/github.com/nicholasdille/insulatr
WORKDIR /go/src/github.com/nicholasdille/insulatr
RUN git fetch \
 && git fetch --tags \
 && git checkout ${REF:-master}
RUN COMMIT=$(git rev-list -1 HEAD) \
 && TAG=$(git describe 2>/dev/null || git describe --tags) \
 && glide install \
 && echo Building version $TAG from commit $COMMIT \
 && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -tags netgo -ldflags "-s -w -X main.GitCommit=$COMMIT -X main.BuildTime=$(date +%Y%m%d-%H%M%S) -X main.Version=$TAG" -o bin/insulatr *.go \
 && ls -l bin

FROM scratch
COPY --from=builder /go/src/github.com/nicholasdille/insulatr/bin/insulatr /insulatr
ENTRYPOINT [ "/insulatr" ]
