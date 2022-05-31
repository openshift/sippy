FROM registry.access.redhat.com/ubi8/ubi:latest AS builder
WORKDIR /go/src/sippy
COPY . .
ENV PATH="/go/bin:${PATH}"
ENV GOPATH="/go"
RUN dnf install -y go make npm && make build
RUN go install github.com/pressly/goose/v3/cmd/goose@latest

FROM registry.access.redhat.com/ubi8/ubi:latest AS base
RUN mkdir -p /historical-data
COPY --from=builder /go/src/sippy/sippy /bin/sippy
COPY --from=builder /go/src/sippy/scripts/fetchdata.sh /bin/fetchdata.sh
COPY --from=builder /go/src/sippy/scripts/fetchdata-prow.sh /bin/fetchdata-prow.sh
COPY --from=builder /go/src/sippy/scripts/fetchdata-kube.sh /bin/fetchdata-kube.sh
COPY --from=builder /go/src/sippy/historical-data /historical-data/
COPY --from=builder /go/bin/goose /bin/goose
COPY ./dbmigration /usr/share/sippydbmigrations
ENTRYPOINT ["/bin/sippy"]
EXPOSE 8080
