FROM golang:1.13.3 AS builder
RUN apt-get update \
    && apt-get install -y git \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /go/src/github.com/grigorov/subspace
RUN go get -v \
    github.com/go-bindata/go-bindata/...
COPY go.mod ./
COPY go.sum ./
COPY *.go ./
COPY static ./static
COPY templates ./templates
COPY email ./email

ARG BUILD_VERSION=unknown

ENV GODEBUG="netdns=go http2server=0"
ENV GOPATH="/go"

RUN go-bindata --pkg main static/... templates/... email/... \
    && go fmt \
    && go vet --all

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -v --compiler gc --ldflags "-extldflags -static -s -w -X main.version=${BUILD_VERSION}" -o /usr/bin/subspace-linux-amd64

FROM phusion/baseimage:0.11

COPY --from=builder /usr/bin/subspace-linux-amd64 /usr/bin/subspace
COPY entrypoint.sh /usr/local/bin/entrypoint.sh

ENV DEBIAN_FRONTEND noninteractive

RUN chmod +x /usr/bin/subspace /usr/local/bin/entrypoint.sh

RUN apt-get update \
    && apt-get install -y iproute2 iptables socat

ENTRYPOINT [ "/usr/local/bin/entrypoint.sh" ]

CMD [ "/sbin/my_init" ]
