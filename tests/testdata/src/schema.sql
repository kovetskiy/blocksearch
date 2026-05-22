-- SQL: searching the table name returns the whole CREATE TABLE block.
CREATE TABLE invoices (
    id          SERIAL PRIMARY KEY,
    amount_cents BIGINT NOT NULL,
    currency    TEXT NOT NULL DEFAULT 'USD',
    paid        BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX invoices_paid_idx ON invoices (paid);
