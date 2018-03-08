FROM golang:1.10 AS build

COPY . /go/src/sblogdriver
RUN cd /go/src/sblogdriver && go get && go build -o /usr/bin/sblogdriver

FROM scratch 
COPY --from=build /usr/bin/sblogdriver /usr/bin/sblogdriver
ENTRYPOINT ["/usr/bin/sblogdriver"]
