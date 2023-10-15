go build .
./cigno --dry-run \
    "FROM alpine:3.16.2 as builder" \
    "COPY / /tmp/"
