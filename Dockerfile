FROM golang:1.12 as builder
RUN go get -u github.com/golang/dep/...

WORKDIR $GOPATH/src/github.com/delcypher/docker-stats-on-exit-shim

COPY Gopkg.toml .
COPY main.go .

RUN dep ensure

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /docker-stats-on-exit-shim

FROM alpine:latest

WORKDIR /

COPY --from=builder /docker-stats-on-exit-shim .

ENTRYPOINT ["/docker-stats-on-exit-shim"]

CMD ["sleep", "1"]
