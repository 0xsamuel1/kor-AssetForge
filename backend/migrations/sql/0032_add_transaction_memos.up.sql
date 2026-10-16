-- Add memo/notes field to transactions for record-keeping
ALTER TABLE transactions
    ADD COLUMN IF NOT EXISTS memo VARCHAR(500);

CREATE INDEX IF NOT EXISTS idx_transactions_memo ON transactions USING gin(to_tsvector('english', coalesce(memo, '')));
