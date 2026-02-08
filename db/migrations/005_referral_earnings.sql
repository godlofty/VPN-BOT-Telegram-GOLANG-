-- Migration: 005_referral_earnings
-- Description: Add total_ref_earnings column for referral stats

-- Add total_ref_earnings column to users table
ALTER TABLE users ADD COLUMN IF NOT EXISTS total_ref_earnings DECIMAL(10,2) DEFAULT 0;

