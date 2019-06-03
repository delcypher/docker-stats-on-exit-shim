FROM golang:1.12 as builder

WORKDIR $GOPATH/src/github.com/delcypher/docker-stats-on-exit-shim

COPY main.go .

RUN git clone https://github.com/opencontainers/runc.git vendor/github.com/opencontainers/runc && \
    cd vendor/github.com/opencontainers/runc && \
    git checkout a2a6e82

RUN go get -d -v ./...

RUN go install -v ./...


RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /go/bin/docker-stats-on-exit-shim.

FROM alpine:latest

WORKDIR /

COPY --from=builder /go/bin/docker-stats-on-exit-shim .

CMD ["/docker-events-notifier"]
