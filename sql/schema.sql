CREATE TABLE IF NOT EXISTS payments (
    correlation_id TEXT PRIMARY KEY,
    amount REAL NOT NULL,
    processor TEXT NOT NULL,
    processed_at TEXT NOT NULL
);