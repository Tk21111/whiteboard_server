ARG GO_VERSION=1
FROM golang:${GO_VERSION}-alpine as builder

RUN apk add --no-cache sqlite sqlite-libs bash

WORKDIR /usr/src/app
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
RUN go build -v -o /run-app .


FROM debian:bookworm

COPY --from=builder /run-app /usr/local/bin/
CMD ["run-app"]
