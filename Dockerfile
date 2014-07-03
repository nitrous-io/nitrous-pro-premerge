From ubuntu:13.10
MAINTAINER Nitrous.IO <eng@nitrous.io>

RUN apt-get update && apt-get install -y \
  build-essential \
  curl \
  git \
  mercurial

RUN locale-gen en_US.utf8

RUN echo "deb http://apt.postgresql.org/pub/repos/apt/ saucy-pgdg main" > /etc/apt/sources.list.d/pgdg.list
RUN curl -s https://www.postgresql.org/media/keys/ACCC4CF8.asc | apt-key add -

RUN apt-get update && apt-get install -y \
  postgresql-9.3 \
  postgresql-contrib-9.3 \
  libpq-dev

RUN curl -s https://storage.googleapis.com/golang/go1.2.2.linux-amd64.tar.gz \
  | tar -v -C /usr/local -xz

ENV GOPATH /opt/src/github.com/nitrous-io/nitrous/.vendor:/opt
ENV PATH /usr/local/go/bin:/opt/bin:$PATH
ENV GOBIN /opt/bin

RUN /usr/local/go/bin/go get github.com/nitrous-io/goop

RUN curl -s http://cache.ruby-lang.org/pub/ruby/2.1/ruby-2.1.2.tar.gz \
  | tar -v -C /tmp -xz
RUN cd /tmp/ruby-2.1.2 && ./configure && make && make install

RUN gem install bundle

RUN mkdir -p /opt/src/github.com/nitrous-io/nitrous
WORKDIR /opt/src/github.com/nitrous-io/
ADD . /opt/src/github.com/nitrous-io/nitrous

RUN /etc/init.d/postgresql start &&\
    su postgres -c "psql -c \"CREATE ROLE "$(whoami)" NOSUPERUSER CREATEDB NOCREATEROLE INHERIT LOGIN;\""

WORKDIR /opt/src/github.com/nitrous-io/nitrous
# FIXME Remove this step when goop has support for private repos
RUN echo "machine github.com login <token>" > $HOME/.netrc
RUN goop install

RUN /etc/init.d/postgresql start &&\
    make createdb

RUN cd rails && bundle install

CMD /etc/init.d/postgresql start &&\
    NITROUS_PG_URL="host=/var/run/postgresql user="$(whoami)" dbname=nitrous_development sslmode=disable"\
    make
