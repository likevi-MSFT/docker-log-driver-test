FROM golang:1.10-alpine

RUN apk add --no-cache git

COPY . /go/src/sblogdriver
WORKDIR /go/src/sblogdriver
RUN go get && go build -o /sblogdriver

RUN mkdir -p /run/docker/plugins /var/log/docker

CMD ["/sblogdriver"]