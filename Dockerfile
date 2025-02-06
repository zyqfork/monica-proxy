FROM golang:alpine AS builder

RUN apk add git

RUN git clone https://github.com/zyqfork/monica-proxy.git
WORKDIR monica-proxy

RUN go build -o monica-proxy

FROM alpine
LABEL maintainer="zouyq <zyqcn@live.com>"

COPY --from=builder /go/monica-proxy/monica-proxy /usr/local/bin

ENTRYPOINT ["monica-proxy"]





