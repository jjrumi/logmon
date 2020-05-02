ARG GO_VERSION=1.14.2-buster

FROM golang:$GO_VERSION AS base

# Install random log generator tool:
RUN GO111MODULE=on go get -u github.com/mingrammer/flog@v0.4.0

ENV GOFLAGS="-mod=vendor"

# Install and build the log monitor:
COPY . /code
WORKDIR /code
RUN make go-install
RUN make go-build

# Create the expected default log file:
RUN touch /tmp/access.log
