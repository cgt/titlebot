FROM golang:1.9-alpine

WORKDIR /go/src/cgt.name/pkg/titlebot
COPY . .

RUN go install cgt.name/pkg/titlebot

USER nobody
ENTRYPOINT ["titlebot"]
