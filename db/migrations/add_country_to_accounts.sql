-- Migration: Add country field to accounts table
-- Date: 2025-12-18
-- Description: Adds a country column to the accounts table to support country-specific account filtering

-- Add country column to accounts table
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS country VARCHAR(10) NOT NULL DEFAULT 'US';

-- Create index on country column for better query performance
CREATE INDEX IF NOT EXISTS idx_accounts_country ON accounts(country);

-- Update existing accounts to set country based on currency
UPDATE accounts SET country =
  CASE
    WHEN currency = 'USD' THEN 'US'
    WHEN currency = 'GBP' THEN 'GB'
    WHEN currency = 'NGN' THEN 'NG'
    WHEN currency = 'EUR' THEN 'EU'
    WHEN currency = 'CAD' THEN 'CA'
    WHEN currency = 'AUD' THEN 'AU'
    WHEN currency = 'INR' THEN 'IN'
    WHEN currency = 'CNY' THEN 'CN'
    WHEN currency = 'JPY' THEN 'JP'
    WHEN currency = 'KES' THEN 'KE'
    WHEN currency = 'ZAR' THEN 'ZA'
    ELSE 'US'
  END
WHERE country = 'US'; -- Only update rows that still have the default value

-- Create composite index for efficient filtering by user and country
CREATE INDEX IF NOT EXISTS idx_accounts_owner_country ON accounts(owner_user_id, country);

-- Comments
COMMENT ON COLUMN accounts.country IS 'Country code (e.g., US, GB, NG, EU) for country-specific accounts';
