FROM golang:1.10 AS build

COPY . /go/src/sblogdriver
RUN cd /go/src/sblogdriver && go get && go build -o /usr/bin/sblogdriver

FROM scratch 
COPY --from=build /usr/bin/sblogdriver /usr/bin/sblogdriver
RUN mkdir /run/docker/plugins # needed to communicate with Docker, this is where the jsonfile.sock will be
ENTRYPOINT ["/usr/bin/sblogdriver"]
