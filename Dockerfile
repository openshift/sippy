FROM registry.access.redhat.com/ubi8/ubi:latest AS builder
WORKDIR /go/src/sippy
COPY . .
ENV PATH="/go/bin:${PATH}"
ENV GOPATH="/go"
RUN dnf module enable nodejs:16 -y && dnf install -y go make npm && make build

FROM registry.access.redhat.com/ubi8/ubi:latest AS base
RUN mkdir -p /historical-data
RUN mkdir -p /config
COPY --from=builder /go/src/sippy/sippy /bin/sippy
COPY --from=builder /go/src/sippy/scripts/fetchdata.sh /bin/fetchdata.sh
COPY --from=builder /go/src/sippy/scripts/fetchdata-testgrid.sh /bin/fetchdata-testgrid.sh
COPY --from=builder /go/src/sippy/scripts/fetchdata-kube.sh /bin/fetchdata-kube.sh
COPY --from=builder /go/src/sippy/historical-data /historical-data/
COPY --from=builder /go/src/sippy/config/*.yaml /config
ENTRYPOINT ["/bin/sippy"]
EXPOSE 8080
