FROM golang:latest AS builder

COPY source /go/src

ENV GOPATH=

ARG TARGETOS=linux
ARG TARGETARCH=amd64

ENV CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH}

WORKDIR /go/src/microservice-framework
RUN go mod init github.com/mefranklin6/microservice-framework \
    && go mod tidy

WORKDIR /go
# Change the module path for each microservice
RUN go mod init github.com/mefranklin6/microservice-extron-sis \
    && go mod edit -replace github.com/mefranklin6/microservice-framework=./src/microservice-framework \
    && go mod tidy

WORKDIR /go/src
RUN go get -u \
    && go build -o /go/bin/microservice


FROM gcr.io/distroless/base:nonroot

COPY --from=builder /go/bin/microservice /microservice

EXPOSE 80

USER nonroot

ENTRYPOINT ["/microservice"]