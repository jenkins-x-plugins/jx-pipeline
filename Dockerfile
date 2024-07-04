FROM alpine:3.20

RUN apk add --update git

ENTRYPOINT ["jx-pipeline"]

COPY ./build/linux/jx-pipeline /usr/bin/jx-pipeline