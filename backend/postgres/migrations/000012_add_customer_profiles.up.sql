ALTER TABLE users
    ADD COLUMN phone TEXT NOT NULL DEFAULT '',
    ADD COLUMN birth_date DATE,
    ADD COLUMN address_line1 TEXT NOT NULL DEFAULT '',
    ADD COLUMN address_line2 TEXT NOT NULL DEFAULT '',
    ADD COLUMN postal_code TEXT NOT NULL DEFAULT '',
    ADD COLUMN city TEXT NOT NULL DEFAULT '',
    ADD COLUMN country_code CHAR(2) NOT NULL DEFAULT 'DE',
    ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP;

ALTER TABLE users
    ADD CONSTRAINT users_phone_length_check CHECK (char_length(phone) <= 32),
    ADD CONSTRAINT users_address_line1_length_check CHECK (char_length(address_line1) <= 120),
    ADD CONSTRAINT users_address_line2_length_check CHECK (char_length(address_line2) <= 120),
    ADD CONSTRAINT users_postal_code_length_check CHECK (char_length(postal_code) <= 12),
    ADD CONSTRAINT users_city_length_check CHECK (char_length(city) <= 80),
    ADD CONSTRAINT users_country_code_check CHECK (country_code ~ '^[A-Z]{2}$'),
    ADD CONSTRAINT users_birth_date_check CHECK (birth_date IS NULL OR birth_date <= CURRENT_DATE);
