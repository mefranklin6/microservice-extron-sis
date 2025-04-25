FROM golang:latest

COPY source /go/src

ENV GOPATH=

WORKDIR /go/src/microservice-framework
# TODO: change from framework fork to Dartmouth repo once PR #3 is merged there
RUN go mod init github.com/mefranklin6/microservice-framework
RUN go mod tidy

WORKDIR /go
# Change the module path for each microservice
RUN go mod init github.com/mefranklin6/microservice-extron-sis
RUN go mod edit -replace github.com/mefranklin6/microservice-framework=./src/microservice-framework
RUN go mod tidy

WORKDIR /go/src
RUN go get -u
RUN go build -o /go/bin/microservice

# Use this entrypoint for a a fully functional docker image
ENTRYPOINT ["/go/bin/microservice"]