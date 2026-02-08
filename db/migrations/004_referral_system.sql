-- Migration: 004_referral_system
-- Description: Add referral system support (referrer_id column)

-- Add referrer_id column to users table (stores Telegram ID of referrer)
ALTER TABLE users ADD COLUMN IF NOT EXISTS referrer_id BIGINT DEFAULT NULL;

-- Add index for referral queries
CREATE INDEX IF NOT EXISTS idx_users_referrer_id ON users(referrer_id);

-- Add transaction type for referral rewards
-- (No schema change needed, just using new type value 'referral_reward')

