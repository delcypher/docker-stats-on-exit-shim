FROM golang:1.12 as builder

WORKDIR $GOPATH/src/github.com/delcypher/docker-stats-on-exit-shim

COPY . .

RUN git submodule init && git submodule update
RUN go get -d -v ./...

RUN go install -v ./...


RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /go/bin/docker-stats-on-exit-shim.

FROM alpine:latest

WORKDIR /

COPY --from=builder /go/bin/docker-stats-on-exit-shim .

CMD ["/docker-events-notifier"]
