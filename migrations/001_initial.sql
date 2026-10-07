CREATE TABLE IF NOT EXISTS students (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    telegram_id INTEGER UNIQUE,
    full_name TEXT NOT NULL UNIQUE,
    role TEXT NOT NULL DEFAULT 'student',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS collections (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    amount INTEGER NOT NULL,
    deadline_at DATETIME NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    closed_reason TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    closed_at DATETIME
);

CREATE TABLE IF NOT EXISTS payments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    mono_tx_id TEXT UNIQUE NOT NULL,
    amount INTEGER NOT NULL,
    comment_raw TEXT,
    received_at DATETIME NOT NULL,
    student_id INTEGER,
    collection_id INTEGER,
    match_method TEXT NOT NULL,
    matched_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(student_id) REFERENCES students(id),
    FOREIGN KEY(collection_id) REFERENCES collections(id)
);

CREATE TABLE IF NOT EXISTS reminder_state (
    collection_id INTEGER NOT NULL,
    student_id INTEGER NOT NULL,
    last_sent_at DATETIME NOT NULL,
    PRIMARY KEY (collection_id, student_id),
    FOREIGN KEY(collection_id) REFERENCES collections(id),
    FOREIGN KEY(student_id) REFERENCES students(id)
);

CREATE TABLE IF NOT EXISTS admin_reports (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    collection_id INTEGER NOT NULL,
    body TEXT NOT NULL,
    sent_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(collection_id) REFERENCES collections(id)
);
