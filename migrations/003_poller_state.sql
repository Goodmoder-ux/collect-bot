CREATE TABLE IF NOT EXISTS poller_state (
    account_id TEXT PRIMARY KEY,
    last_poll_at DATETIME NOT NULL
);
