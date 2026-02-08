-- Migration: 006_promo_codes
-- Description: Add promo codes system

-- Promo codes table
CREATE TABLE IF NOT EXISTS promo_codes (
    id SERIAL PRIMARY KEY,
    code VARCHAR(50) UNIQUE NOT NULL,
    amount DECIMAL(10,2) NOT NULL DEFAULT 0,
    max_activations INT NOT NULL DEFAULT 1,
    activations_used INT NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Promo activations table (tracks who used which code)
CREATE TABLE IF NOT EXISTS promo_activations (
    id SERIAL PRIMARY KEY,
    promo_id INT NOT NULL REFERENCES promo_codes(id) ON DELETE CASCADE,
    user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    telegram_id BIGINT NOT NULL,
    activated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(promo_id, user_id)
);

-- Index for faster lookups
CREATE INDEX IF NOT EXISTS idx_promo_codes_code ON promo_codes(code);
CREATE INDEX IF NOT EXISTS idx_promo_activations_user ON promo_activations(user_id);
CREATE INDEX IF NOT EXISTS idx_promo_activations_telegram ON promo_activations(telegram_id);

