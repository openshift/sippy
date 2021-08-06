FROM registry.access.redhat.com/ubi8/ubi:latest
WORKDIR /go/src/sippy
COPY . .
RUN if which dnf; then dnf install -y go make; fi && make build
ENTRYPOINT ["/go/src/sippy/sippy"]
EXPOSE 8080

