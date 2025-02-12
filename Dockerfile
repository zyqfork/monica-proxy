FROM golang:alpine AS builder

ADD ./ monica-proxy

WORKDIR monica-proxy

RUN go build -o monica-proxy

FROM alpine
LABEL maintainer="zouyq <zyqcn@live.com>"

COPY --from=builder /go/monica-proxy/monica-proxy /usr/local/bin

ENTRYPOINT ["monica-proxy"]

