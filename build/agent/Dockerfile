# syntax=docker/dockerfile:1
FROM golang:1.17 as builder

WORKDIR /build
COPY . .
ENV CGO_ENABLED=0 
ENV GOOS=linux

RUN cd cmd/agent;go build -o /runner -mod mod -a .

FROM gradle:7.4.1-jdk11
COPY --from=builder /runner /bin/runner

USER 1001

ENTRYPOINT ["/bin/runner"]
