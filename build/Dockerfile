# Multi-stage docker build
# Build stage
FROM golang:1.16 AS builder

LABEL maintainer="LitmusChaos"

ARG TARGETPLATFORM

ADD . /chaos-scheduler
WORKDIR /chaos-scheduler

RUN export GOOS=$(echo ${TARGETPLATFORM} | cut -d / -f1) && \
    export GOARCH=$(echo ${TARGETPLATFORM} | cut -d / -f2)

RUN go env

RUN CGO_ENABLED=0 go build -o /output/chaos-scheduler -v ./cmd/manager/

# Packaging stage
FROM registry.access.redhat.com/ubi8/ubi-minimal:8.4

LABEL maintainer="LitmusChaos"

ENV SCHEDULER=/usr/local/bin/chaos-scheduler \
    USER_UID=1001 \
    USER_NAME=chaos-scheduler

COPY --from=builder /output/chaos-scheduler ${SCHEDULER}

COPY build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
