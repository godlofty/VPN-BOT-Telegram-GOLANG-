-- Migration: 003_xray_mode_only
-- Description: Simplify products to X-RAY MODE only

-- Delete all products except the first one (Мульти -> X-RAY MODE)
DELETE FROM products WHERE id > 1;

-- Update the first product to be X-RAY MODE
UPDATE products SET
    name = 'X-RAY MODE',
    country_flag = '🌍',
    base_price = 450,
    marzban_tag = 'xray_mode',
    description = 'Все локации: US, DE, NL, FR, RU + YouTube & Instagram',
    sort_order = 1
WHERE id = 1;

