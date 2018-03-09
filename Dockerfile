FROM golang:1.10 AS build

COPY . /go/src/sblogdriver
WORKDIR /go/src/sblogdriver
RUN go get && go build --ldflags '-extldflags "-static"' -o /go/bin/sblogdriver
CMD ["/go/bin/sblogdriver"]

FROM alpine
RUN mkdir -p /run/docker/plugins /var/log/docker
COPY --from=build /go/bin/sblogdriver .
CMD ["sblogdriver"]
