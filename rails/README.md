## Setup

1. Use Ruby 2.1.2
2. Install Postgres - `sudo apt-get install postgresql-9.3 postgresql-9.3-contrib libpq-dev`
3. `bundle install`
4. `sudo su postgres -c "psql -c \"CREATE ROLE "$(whoami)" NOSUPERUSER CREATEDB NOCREATEROLE INHERIT LOGIN;\""`
5. `mv config/database-example.yml config/database.yml`
6. `rake db:create db:migrate`

