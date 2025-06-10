CREATE TABLE sales (
    id TEXT PRIMARY KEY,
    start_time TIMESTAMPTZ NOT NULL,
    total_items INT NOT NULL,
    items_sold INT DEFAULT 0
);

CREATE TABLE checkouts (
    id SERIAL PRIMARY KEY,
    sale_id TEXT REFERENCES sales(id),
    user_id TEXT NOT NULL,
    item_id TEXT NOT NULL,
    item_name TEXT NOT NULL,
    item_image_url TEXT NOT NULL,
    code TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE purchases (
    id SERIAL PRIMARY KEY,
    checkout_id INT REFERENCES checkouts(id),
    purchased_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_checkouts_code ON checkouts(code);