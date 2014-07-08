package db

var Migrations []*Migration

func init() {
	Migrations = []*Migration{
		{
			Version: "10",
			Up: `
				CREATE TABLE users (
					id bigserial PRIMARY KEY NOT NULL,
					username character varying(30) NOT NULL,
					email character varying(255) NOT NULL,
					encrypted_password character varying(255) NOT NULL,
					name character varying(255) DEFAULT '' NOT NULL,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE UNIQUE INDEX index_users_on_username ON users USING btree (username);
				CREATE UNIQUE INDEX index_users_on_email ON users USING btree (email);
			`,
		},
		{
			Version: "20",
			Up: `
				CREATE TABLE tokens (
					id bigserial PRIMARY KEY NOT NULL,
					user_id bigint REFERENCES users(id) NOT NULL,
					token character varying(255) NOT NULL,
					created_at timestamp without time zone DEFAULT now()
				);
				CREATE UNIQUE INDEX index_tokens_on_token ON tokens USING btree (token);
			`,
		},
		{
			Version: "30",
			Up: `
				CREATE TABLE regions (
					id bigserial PRIMARY KEY NOT NULL,
					name character varying(255) NOT NULL,
					description character varying(255) NOT NULL,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE UNIQUE INDEX index_regions_on_name ON regions USING btree (name);
			`,
		},
		{
			Version: "40",
			Up: `
				CREATE TABLE plans (
					id bigserial PRIMARY KEY NOT NULL,
					name character varying(255) NOT NULL,
					max_containers bigint NOT NULL,
					price bigint NOT NULL, -- Price in cents so that we always deal with integers.
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE UNIQUE INDEX index_plans_on_name ON plans USING btree (name);
			`,
		},
		{
			Version: "50",
			Up: `
				CREATE TABLE provider_templates (
					id bigserial PRIMARY KEY NOT NULL,
					region_id bigint REFERENCES regions(id) NOT NULL,
					plan_id bigint REFERENCES plans(id) NOT NULL,
					name character varying(255) NOT NULL,
					provider character varying(255) NOT NULL,
					config hstore,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE INDEX index_provider_templates_on_region_id ON provider_templates USING btree (region_id);
				CREATE INDEX index_provider_templates_on_plan_id ON provider_templates USING btree (plan_id);
			`,
		},
		{
			Version: "60",
			Up: `
				CREATE TABLE hosts (
					id bigserial PRIMARY KEY NOT NULL,
					user_id bigint REFERENCES users(id) NOT NULL,
					region_id bigint REFERENCES regions(id) NOT NULL,
					plan_id bigint REFERENCES plans(id) NOT NULL,
					provider_template_id bigint REFERENCES provider_templates(id) NOT NULL,
					name character varying(63) NOT NULL, -- Used as DNS label, see http://tools.ietf.org/html/rfc1034#section-3.1.
					long_name character varying(255),
					state character varying(255) NOT NULL DEFAULT 'born',
					ip_address character varying(45), -- See http://tools.ietf.org/html/rfc4291.
					provider_identifier character varying(255),
					max_containers bigint,
					terminated_at timestamp without time zone,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE INDEX index_hosts_on_user_id ON hosts USING btree (user_id);
				CREATE INDEX index_hosts_on_region_id ON hosts USING btree (region_id);
				CREATE UNIQUE INDEX index_hosts_on_user_id_and_name ON hosts USING btree (user_id, name);
			`,
		},
		{
			Version: "70",
			Up: `
				CREATE TABLE containers (
					id character varying(255) PRIMARY KEY NOT NULL,
					user_id bigint REFERENCES users(id) NOT NULL,
					host_id bigint REFERENCES hosts(id) NOT NULL,
					image character varying(255) NOT NULL,
					name character varying(255) NOT NULL,
					long_name character varying(255),
					ip_address character varying(45),
					terminated_at timestamp without time zone,
					touched_at timestamp without time zone,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE INDEX index_containers_on_user_id ON containers USING btree (user_id);
				CREATE UNIQUE INDEX index_containers_on_host_id_and_name ON containers USING btree (host_id, name);
			`,
		},
		{
			Version: "80",
			Up: `
				DROP INDEX index_hosts_on_user_id_and_name;
				CREATE UNIQUE INDEX index_hosts_on_name ON hosts USING btree (name);
			`,
		},
		{
			Version: "90",
			Up: `
				ALTER TABLE hosts ADD COLUMN freighterd_port bigint;
			`,
		},
		{
			Version: "100",
			Up: `
				CREATE TABLE public_keys (
					id bigserial PRIMARY KEY NOT NULL,
					user_id bigint REFERENCES users(id) NOT NULL,
					container_id character varying(255) REFERENCES containers(id) NOT NULL,
					key text NOT NULL,
					fingerprint character varying(255) NOT NULL,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE INDEX index_public_keys_on_user_id ON public_keys USING btree (user_id);
				CREATE UNIQUE INDEX index_public_keys_on_container_id_and_key ON public_keys USING btree (container_id, key);
			`,
		},
	}
}
