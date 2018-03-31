FROM golang:1-alpine

WORKDIR /go/src/cgt.name/pkg/titlebot
COPY . .

RUN go install cgt.name/pkg/titlebot

USER nobody
ENTRYPOINT ["titlebot"]
