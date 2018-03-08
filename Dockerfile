FROM golang:1.10 AS build

COPY . /go/src/sblogdriver
RUN cd /go/src/sblogdriver && go get && go build -o /usr/bin/sblogdriver

FROM alpine:3.7 
COPY --from=build /usr/bin/sblogdriver /usr/bin/sblogdriver
RUN mkdir -p run/docker/plugins
ENTRYPOINT ["/usr/bin/sblogdriver"]
CMD [""]
