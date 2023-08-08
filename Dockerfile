FROM alpine:edge
VOLUME /workdir
WORKDIR /workdir
COPY /blocksearch /usr/bin/blocksearch
ENTRYPOINT ["/usr/bin/blocksearch"]
