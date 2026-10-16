ALTER TABLE users DROP COLUMN IF EXISTS recovery_codes_generated_at;
DROP TABLE IF EXISTS user_recovery_codes;
