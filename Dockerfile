FROM centos:7

RUN yum install -y git

ENTRYPOINT ["jx-pipeline"]

COPY ./build/linux/jx-pipeline /usr/bin/jx-pipeline