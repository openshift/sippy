FROM registry.access.redhat.com/ubi8/ubi:latest AS builder
WORKDIR /go/src/sippy
COPY . .
ENV PATH="/go/bin:${PATH}"
ENV GOPATH="/go"
RUN dnf install -y go make npm && make build

FROM registry.access.redhat.com/ubi8/ubi:latest AS base
COPY --from=builder /go/src/sippy/sippy /bin/sippy
COPY --from=builder /go/src/sippy/scripts/fetchdata.sh /bin/fetchdata.sh
ENTRYPOINT ["/bin/sippy"]
EXPOSE 8080
