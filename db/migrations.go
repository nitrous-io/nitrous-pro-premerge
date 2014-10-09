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
				DROP INDEX index_containers_on_host_id_and_name;
				CREATE UNIQUE INDEX index_containers_on_name ON containers USING btree (name);
			`,
		},
		{
			Version: "110",
			Up: `
				ALTER TABLE containers DROP CONSTRAINT containers_pkey;
				ALTER TABLE containers RENAME COLUMN id TO cid;
				ALTER TABLE containers ADD COLUMN id bigserial PRIMARY KEY NOT NULL;
				CREATE INDEX index_containers_on_cid ON containers USING btree (cid);
			`,
		},
		{
			Version: "120",
			Up: `
				ALTER TABLE containers RENAME COLUMN name TO slug;
				ALTER TABLE containers RENAME COLUMN long_name TO name;
				DROP INDEX index_containers_on_name;
				CREATE UNIQUE INDEX index_containers_on_slug ON containers USING btree (slug);

				ALTER TABLE hosts RENAME COLUMN name TO slug;
				ALTER TABLE hosts RENAME COLUMN long_name TO name;
				DROP INDEX index_hosts_on_name;
				CREATE UNIQUE INDEX index_hosts_on_slug ON hosts USING btree (slug);
			`,
		},
		{
			Version: "130",
			Up: `
				ALTER TABLE hosts ADD COLUMN size character varying(255);
			`,
		},
		{
			Version: "140",
			Up: `
				CREATE TABLE key_pairs (
					id bigserial PRIMARY KEY NOT NULL,
					host_id bigint,
					public_key text NOT NULL,
					encrypted_private_key text NOT NULL,
					encrypted_private_key_iv character varying(255) NOT NULL,
					terminated_at timestamp without time zone,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE INDEX index_key_pairs_on_host_id ON key_pairs USING btree (host_id);
			`,
		},
		{
			Version: "150",
			Up: `
				CREATE TABLE public_keys (
					id bigserial PRIMARY KEY NOT NULL,
					user_id bigint REFERENCES users(id) NOT NULL,
					container_id bigserial REFERENCES containers(id) NOT NULL,
					key text NOT NULL,
					fingerprint character varying(255) NOT NULL,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE INDEX index_public_keys_on_user_id ON public_keys USING btree (user_id);
				CREATE UNIQUE INDEX index_public_keys_on_container_id_and_key ON public_keys USING btree (container_id, key);
			`,
		},
		{
			Version: "160",
			Up: `
				CREATE TABLE volumes (
					id bigserial PRIMARY KEY NOT NULL,
					host_id bigint REFERENCES hosts(id) NOT NULL,
					name character varying(255) NOT NULL,
					slug character varying(255) NOT NULL,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE INDEX index_volumes_on_host_id ON volumes USING btree (host_id);
				CREATE UNIQUE INDEX index_volumes_on_host_id_and_slug ON volumes USING btree (host_id, slug);
			`,
		},
		{
			Version: "170",
			Up: `
				CREATE TABLE registries (
					id bigserial PRIMARY KEY NOT NULL,
					user_id bigint REFERENCES users(id) NOT NULL,
					host_id bigint REFERENCES hosts(id) NOT NULL,
					url character varying(45),
					username character varying(45),
					email character varying(45),
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE INDEX index_registries_on_user_id ON registries USING btree (user_id);
				CREATE UNIQUE INDEX index_registries_on_host_id_and_url ON registries USING btree (host_id, url);
			`,
		},
		{
			Version: "180",
			Up: `
				CREATE TABLE certs (
					id bigserial PRIMARY KEY NOT NULL,
					host_id bigint,
					ca text NOT NULL,
					server_cert text NOT NULL,
					server_key text NOT NULL,
					client_cert text NOT NULL,
					client_key text NOT NULL,
					terminated_at timestamp without time zone,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE INDEX index_certs_on_host_id ON certs USING btree (host_id);
			`,
		},
		{
			Version: "190",
			Up: `
				CREATE TABLE container_volumes (
					id bigserial PRIMARY KEY NOT NULL,
					container_id bigint REFERENCES containers(id) NOT NULL,
					volume_id bigint REFERENCES volumes(id) NOT NULL,
					created_at timestamp without time zone DEFAULT now()
				);
				CREATE UNIQUE INDEX index_container_volumes_on_container_id_and_volume_id ON container_volumes USING btree (container_id, volume_id);
			`,
		},
		{
			Version: "200",
			Up: `
				ALTER TABLE containers DROP COLUMN ip_address;
			`,
		},
		{
			Version: "210",
			Up: `
				DROP TABLE certs;
			`,
		},
		{
			Version: "215",
			Up: `
				DROP INDEX index_plans_on_name;
        ALTER TABLE plans ADD COLUMN slug character varying(255);
        UPDATE plans SET slug=name;
        ALTER TABLE plans ALTER COLUMN slug SET NOT NULL;
				CREATE UNIQUE INDEX index_plans_on_slug ON plans USING btree (slug);
        DROP INDEX index_regions_on_name;
				ALTER TABLE regions RENAME COLUMN name TO slug;
				CREATE UNIQUE INDEX index_regions_on_slug ON regions USING btree (slug);
				ALTER TABLE regions RENAME COLUMN description TO name;
      `,
		},
		{
			Version: "220",
			Up: `
				ALTER TABLE tokens ALTER COLUMN token SET DEFAULT left(encode(gen_random_bytes(191), 'base64'), 255);
				ALTER TABLE hosts ADD COLUMN secret character varying(255) NOT NULL DEFAULT left(encode(gen_random_bytes(191), 'base64'), 255);
			`,
		},
		{
			Version: "225",
			Up: `
				ALTER TABLE regions ADD COLUMN rank bigint NOT NULL DEFAULT 0;
				ALTER TABLE plans ADD COLUMN cpu character varying(255) NOT NULL DEFAULT '';
				ALTER TABLE plans ADD COLUMN memory character varying(255) NOT NULL DEFAULT '';
				ALTER TABLE plans ADD COLUMN storage character varying(255) NOT NULL DEFAULT '';
			`,
		},
		{
			Version: "230",
			Up: `
				ALTER TABLE containers ADD COLUMN secret character varying(255) NOT NULL DEFAULT left(encode(gen_random_bytes(191), 'base64'), 255);
			`,
		},
		{
			Version: "240",
			Up: `
				ALTER TABLE containers ADD COLUMN locked_at timestamp without time zone;
			`,
		},
		{
			Version: "250",
			Up: `
				CREATE TABLE images (
					id bigserial PRIMARY KEY NOT NULL,
					user_id bigint REFERENCES users(id) NOT NULL,
					host_id bigint REFERENCES hosts(id) NOT NULL,
					freighter_id character varying(255) NOT NULL,
					name character varying(255),
					tag character varying(255),
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE INDEX index_images_on_user_id ON images USING btree (user_id);
				CREATE INDEX index_images_on_host_id ON images USING btree (host_id);
				CREATE INDEX index_images_on_freighter_id ON images USING btree (freighter_id);
			`,
		},
		{
			Version: "260",
			Up: `
				ALTER TABLE images ADD COLUMN slug character varying(255);
				CREATE INDEX index_images_on_slug_and_tag ON images USING btree (slug, tag);
			`,
		},
		{
			Version: "270",
			Up: `
				CREATE TABLE base_images (
					id bigserial PRIMARY KEY NOT NULL,
					name character varying(255) NOT NULL,
					repo character varying(255) NOT NULL,
					description character varying(255) NOT NULL,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE UNIQUE INDEX index_base_images_on_name ON base_images USING btree (name);
			`,
		},
		{
			Version: "280",
			Up: `
				CREATE TABLE certs (
					id bigserial PRIMARY KEY NOT NULL,
					host_id bigint,
					ca text NOT NULL,
					server_cert text NOT NULL,
					server_key text NOT NULL,
					terminated_at timestamp without time zone,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE INDEX index_certs_on_host_id ON certs USING btree (host_id);
			`,
		},
		{
			Version: "285",
			Up: `
				ALTER TABLE certs DROP COLUMN ca;
			`,
		},
		{
			Version: "300",
			Up: `
				CREATE TABLE user_profiles (
					id bigserial PRIMARY KEY NOT NULL,
					max_hosts bigint NOT NULL DEFAULT 1,
					user_id bigint REFERENCES users(id) NOT NULL,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE UNIQUE INDEX index_user_profiles_on_user_id ON user_profiles USING btree (user_id);
			`,
		},

		// Note: existing users will be limited to one host by default
		// If you have a test account, change this value manually
		{
			Version: "310",
			Up: `
				INSERT INTO user_profiles (user_id) (
					SELECT users.id FROM users WHERE NOT EXISTS (SELECT user_id FROM user_profiles)
				);
			`,
		},
		{
			Version: "315",
			Up: `
				ALTER TABLE base_images ADD COLUMN slug character varying(255) DEFAULT '' NOT NULL;
			`,
		},
		{
			Version: "320",
			Up: `
				ALTER TABLE volumes ADD COLUMN system boolean DEFAULT false NOT NULL;
			`,
		},
	}
}
