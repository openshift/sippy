FROM golang:1.15.2
WORKDIR /go/src/sippy
COPY . .
RUN make build
ENTRYPOINT ["/go/src/sippy/sippy"]
EXPOSE 8080

