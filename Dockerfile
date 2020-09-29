FROM golang:1.15.2
#RUN INSTALL_PKGS="golang" && \
#    dnf install -y --setopt=tsflags=nodocs $INSTALL_PKGS && \
#    rpm -V $INSTALL_PKGS && \
#    dnf clean all -y
RUN mkdir /tmp/src
COPY . /tmp/src
RUN cd /tmp/src; make
ENTRYPOINT /tmp/src/sippy


