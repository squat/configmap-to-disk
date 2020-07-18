ARG FROM=alpine
FROM $FROM
ARG GOARCH
LABEL maintainer="squat <lserven@gmail.com>"
COPY bin/$GOARCH/configmap-to-disk /usr/local/bin/configmap-to-disk
ENTRYPOINT ["/usr/local/bin/configmap-to-disk"]
