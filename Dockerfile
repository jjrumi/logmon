ARG GO_VERSION=1.14.2-buster

FROM golang:$GO_VERSION AS base

RUN GO111MODULE=on go get -u github.com/mingrammer/flog@v0.4.0

ENV GOFLAGS="-mod=vendor"

WORKDIR /code
