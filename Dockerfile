FROM golang:1-alpine AS builder
RUN apk update && apk add --no-cache git


WORKDIR $GOPATH/src/gcs_http/
COPY . .

RUN go get -d -v
RUN go build -o /go/bin/gcs_http

#---
FROM alpine:3.10
RUN apk add --no-cache ca-certificates
COPY --from=builder /go/bin/gcs_http /bin/gcs_http

ENTRYPOINT ["/bin/gcs_http"]
