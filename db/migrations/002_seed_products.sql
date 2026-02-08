-- Migration: 002_seed_products
-- Description: Seed initial VPN products

INSERT INTO products (name, country_flag, base_price, marzban_tag, description, sort_order)
VALUES 
    ('Мульти', '🌍', 450, 'multi', 'RU, DE, NL, US, FR', 1),
    ('Обход [WhiteList]', '🏴‍☠️', 300, 'whitelist', 'Без YouTube', 2),
    ('Россия [YT, INST]', '🇷🇺', 75, 'russia', 'YouTube, Instagram', 3),
    ('США', '🇺🇸', 150, 'usa', NULL, 4),
    ('Нидерланды', '🇳🇱', 150, 'netherlands', NULL, 5),
    ('Германия', '🇩🇪', 300, 'germany', NULL, 6),
    ('Франция', '🇫🇷', 225, 'france', NULL, 7)
ON CONFLICT (name) DO UPDATE SET
    country_flag = EXCLUDED.country_flag,
    base_price = EXCLUDED.base_price,
    marzban_tag = EXCLUDED.marzban_tag,
    description = EXCLUDED.description,
    sort_order = EXCLUDED.sort_order;

