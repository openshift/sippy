FROM registry.access.redhat.com/ubi9/ubi:latest AS builder
WORKDIR /go/src/sippy
COPY . .
ENV PATH="/go/bin:${PATH}"
ENV GOPATH="/go"
RUN dnf module enable nodejs:18 -y && dnf install -y go make npm && make build

FROM registry.access.redhat.com/ubi9/ubi:latest AS base
RUN mkdir -p /historical-data
RUN mkdir -p /config
COPY --from=builder /go/src/sippy/sippy /bin/sippy
COPY --from=builder /go/src/sippy/sippy-daemon /bin/sippy-daemon
COPY --from=builder /go/src/sippy/scripts/fetchdata.sh /bin/fetchdata.sh
COPY --from=builder /go/src/sippy/config/*.yaml /config/
ENTRYPOINT ["/bin/sippy"]
EXPOSE 8080
