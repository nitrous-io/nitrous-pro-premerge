From ubuntu:14.04
MAINTAINER Nitrous.IO <eng@nitrous.io>

RUN apt-get update && apt-get install -y \
  build-essential \
  curl \
  git \
  mercurial \
  postgresql-9.3 \
  postgresql-contrib-9.3 \
  libpq-dev \
  nodejs

RUN locale-gen en_US.utf8
RUN mkdir -p /usr/local/ruby

RUN curl -s https://storage.googleapis.com/golang/go1.3.linux-amd64.tar.gz \
  | tar -v -C /usr/local -xz

RUN curl -s https://intercityup.com/binaries/ruby/ubuntu/14.04/x86_64/ruby-2.1.2.tar.bz2 \
  | tar -v -C /usr/local/ruby -xj

ENV GOPATH /opt/src/github.com/nitrous-io/nitrous/.vendor:/opt
ENV PATH /usr/local/ruby/2.1.2/bin:/usr/local/go/bin:/opt/bin:$PATH
ENV GOBIN /opt/bin

RUN /usr/local/go/bin/go get github.com/nitrous-io/goop

RUN /usr/local/ruby/2.1.2/bin/gem install bundle

RUN mkdir -p /opt/src/github.com/nitrous-io/nitrous

ADD .buildkey /root/.ssh/id_rsa
ADD util/ssh_config /etc/ssh/ssh_config
ADD . /opt/src/github.com/nitrous-io/nitrous

RUN /etc/init.d/postgresql start &&\
    su postgres -c "psql -c \"CREATE ROLE "$(whoami)" NOSUPERUSER CREATEDB NOCREATEROLE INHERIT LOGIN;\""

WORKDIR /opt/src/github.com/nitrous-io/nitrous

RUN cp .env-example .env
RUN sed -i s/vagrant/root/ .env

RUN /etc/init.d/postgresql start &&\
    make createdb

EXPOSE 3000
CMD /etc/init.d/postgresql start && goop install && make run
