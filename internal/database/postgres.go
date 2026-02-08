package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"vpn-telegram-bot/internal/models"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB обёртка над пулом соединений PostgreSQL
type DB struct {
	Pool *pgxpool.Pool
}

// New создаёт новое подключение к БД используя DATABASE_URL
func New(databaseURL string) (*DB, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is not set")
	}

	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to database: %w", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("unable to ping database: %w", err)
	}

	return &DB{Pool: pool}, nil
}

// Close закрывает пул соединений
func (db *DB) Close() {
	db.Pool.Close()
}

// RunMigrations выполняет SQL миграции из указанной директории
func (db *DB) RunMigrations(ctx context.Context, migrationsDir string) error {
	// Ensure schema_migrations table exists
	_, err := db.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	// Read migration files
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("failed to read migrations directory: %w", err)
	}

	// Filter and sort SQL files
	var migrationFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			migrationFiles = append(migrationFiles, entry.Name())
		}
	}
	sort.Strings(migrationFiles)

	// Apply each migration
	for _, filename := range migrationFiles {
		version := strings.TrimSuffix(filename, ".sql")

		// Check if migration already applied
		var exists bool
		err := db.Pool.QueryRow(ctx, 
			"SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)", 
			version,
		).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to check migration status for %s: %w", version, err)
		}

		if exists {
			continue // Skip already applied migration
		}

		// Read and execute migration file
		filePath := filepath.Join(migrationsDir, filename)
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", filename, err)
		}

		// Execute migration in a transaction
		tx, err := db.Pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("failed to begin transaction for %s: %w", version, err)
		}

		_, err = tx.Exec(ctx, string(content))
		if err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("failed to execute migration %s: %w", version, err)
		}

		// Record migration
		_, err = tx.Exec(ctx, 
			"INSERT INTO schema_migrations (version) VALUES ($1)", 
			version,
		)
		if err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("failed to record migration %s: %w", version, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("failed to commit migration %s: %w", version, err)
		}

		fmt.Printf("✅ Applied migration: %s\n", version)
	}

	return nil
}

// === User Methods ===

// GetOrCreateUser получает или создаёт пользователя
func (db *DB) GetOrCreateUser(ctx context.Context, telegramID int64, username string) (*models.User, error) {
	var user models.User

	err := db.Pool.QueryRow(ctx, `
		INSERT INTO users (telegram_id, username)
		VALUES ($1, $2)
		ON CONFLICT (telegram_id) DO UPDATE SET username = EXCLUDED.username
		RETURNING id, telegram_id, username, balance, referrer_id, COALESCE(total_ref_earnings, 0), created_at
	`, telegramID, username).Scan(&user.ID, &user.TelegramID, &user.Username, &user.Balance, &user.ReferrerID, &user.TotalRefEarnings, &user.CreatedAt)

	if err != nil {
		return nil, err
	}

	return &user, nil
}

// CreateUserWithReferrer создаёт нового пользователя с реферером
func (db *DB) CreateUserWithReferrer(ctx context.Context, telegramID int64, username string, referrerTelegramID int64) (*models.User, error) {
	var user models.User

	err := db.Pool.QueryRow(ctx, `
		INSERT INTO users (telegram_id, username, referrer_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (telegram_id) DO NOTHING
		RETURNING id, telegram_id, username, balance, referrer_id, COALESCE(total_ref_earnings, 0), created_at
	`, telegramID, username, referrerTelegramID).Scan(&user.ID, &user.TelegramID, &user.Username, &user.Balance, &user.ReferrerID, &user.TotalRefEarnings, &user.CreatedAt)

	if err != nil {
		// Если пользователь уже существует, просто получим его
		return db.GetOrCreateUser(ctx, telegramID, username)
	}

	return &user, nil
}

// UserExists проверяет существует ли пользователь
func (db *DB) UserExists(ctx context.Context, telegramID int64) (bool, error) {
	var exists bool
	err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM users WHERE telegram_id = $1)
	`, telegramID).Scan(&exists)
	return exists, err
}

// GetUserByTelegramIDForReferral получает пользователя для реферальной программы
func (db *DB) GetUserByTelegramIDForReferral(ctx context.Context, telegramID int64) (*models.User, error) {
	var user models.User
	err := db.Pool.QueryRow(ctx, `
		SELECT id, telegram_id, username, balance, referrer_id, COALESCE(total_ref_earnings, 0), created_at
		FROM users WHERE telegram_id = $1
	`, telegramID).Scan(&user.ID, &user.TelegramID, &user.Username, &user.Balance, &user.ReferrerID, &user.TotalRefEarnings, &user.CreatedAt)

	if err != nil {
		return nil, err
	}

	return &user, nil
}

// GetUserByTelegramID получает пользователя по Telegram ID
func (db *DB) GetUserByTelegramID(ctx context.Context, telegramID int64) (*models.User, error) {
	var user models.User
	err := db.Pool.QueryRow(ctx, `
		SELECT id, telegram_id, username, balance, referrer_id, COALESCE(total_ref_earnings, 0), created_at
		FROM users WHERE telegram_id = $1
	`, telegramID).Scan(&user.ID, &user.TelegramID, &user.Username, &user.Balance, &user.ReferrerID, &user.TotalRefEarnings, &user.CreatedAt)

	if err != nil {
		return nil, err
	}

	return &user, nil
}

// === Product Methods ===

// GetAllProducts получает все продукты
func (db *DB) GetAllProducts(ctx context.Context) ([]models.Product, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, name, country_flag, base_price, marzban_tag, COALESCE(description, ''), sort_order
		FROM products ORDER BY sort_order
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []models.Product
	for rows.Next() {
		var p models.Product
		if err := rows.Scan(&p.ID, &p.Name, &p.CountryFlag, &p.BasePrice, &p.MarzbanTag, &p.Description, &p.SortOrder); err != nil {
			return nil, err
		}
		products = append(products, p)
	}

	return products, nil
}

// GetProductByID получает продукт по ID
func (db *DB) GetProductByID(ctx context.Context, id int64) (*models.Product, error) {
	var p models.Product
	err := db.Pool.QueryRow(ctx, `
		SELECT id, name, country_flag, base_price, marzban_tag, COALESCE(description, ''), sort_order
		FROM products WHERE id = $1
	`, id).Scan(&p.ID, &p.Name, &p.CountryFlag, &p.BasePrice, &p.MarzbanTag, &p.Description, &p.SortOrder)

	if err != nil {
		return nil, err
	}

	return &p, nil
}

// === Subscription Methods ===

// CreateSubscription создаёт подписку
func (db *DB) CreateSubscription(ctx context.Context, userID, productID int64, keyString string, expiresAt time.Time) (*models.Subscription, error) {
	var sub models.Subscription
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO subscriptions (user_id, product_id, key_string, expires_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, product_id, key_string, expires_at, is_active, created_at
	`, userID, productID, keyString, expiresAt).Scan(
		&sub.ID, &sub.UserID, &sub.ProductID, &sub.KeyString, &sub.ExpiresAt, &sub.IsActive, &sub.CreatedAt,
	)

	if err != nil {
		return nil, err
	}

	return &sub, nil
}

// GetUserSubscriptions получает подписки пользователя
func (db *DB) GetUserSubscriptions(ctx context.Context, userID int64) ([]models.Subscription, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT s.id, s.user_id, s.product_id, s.key_string, s.expires_at, s.is_active, s.created_at,
			   p.id, p.name, p.country_flag, p.base_price, p.marzban_tag
		FROM subscriptions s
		JOIN products p ON s.product_id = p.id
		WHERE s.user_id = $1
		ORDER BY s.created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []models.Subscription
	for rows.Next() {
		var s models.Subscription
		var p models.Product
		if err := rows.Scan(
			&s.ID, &s.UserID, &s.ProductID, &s.KeyString, &s.ExpiresAt, &s.IsActive, &s.CreatedAt,
			&p.ID, &p.Name, &p.CountryFlag, &p.BasePrice, &p.MarzbanTag,
		); err != nil {
			return nil, err
		}
		s.Product = &p
		subs = append(subs, s)
	}

	return subs, nil
}

// GetSubscriptionByID получает подписку по ID
func (db *DB) GetSubscriptionByID(ctx context.Context, id int64) (*models.Subscription, error) {
	var s models.Subscription
	var p models.Product

	err := db.Pool.QueryRow(ctx, `
		SELECT s.id, s.user_id, s.product_id, s.key_string, s.expires_at, s.is_active, s.created_at,
			   p.id, p.name, p.country_flag, p.base_price, p.marzban_tag
		FROM subscriptions s
		JOIN products p ON s.product_id = p.id
		WHERE s.id = $1
	`, id).Scan(
		&s.ID, &s.UserID, &s.ProductID, &s.KeyString, &s.ExpiresAt, &s.IsActive, &s.CreatedAt,
		&p.ID, &p.Name, &p.CountryFlag, &p.BasePrice, &p.MarzbanTag,
	)

	if err != nil {
		return nil, err
	}

	s.Product = &p
	return &s, nil
}

// ExtendSubscription продлевает подписку
func (db *DB) ExtendSubscription(ctx context.Context, id int64, newExpiresAt time.Time) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE subscriptions SET expires_at = $1, is_active = true WHERE id = $2
	`, newExpiresAt, id)
	return err
}

// === Admin Methods ===

// GetAdminStats возвращает статистику для админ-панели
func (db *DB) GetAdminStats(ctx context.Context) (*models.AdminStats, error) {
	stats := &models.AdminStats{}

	// Total users
	err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&stats.TotalUsers)
	if err != nil {
		return nil, err
	}

	// New users today
	err = db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM users 
		WHERE created_at >= CURRENT_DATE
	`).Scan(&stats.NewUsersToday)
	if err != nil {
		return nil, err
	}

	// Active subscriptions
	err = db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM subscriptions 
		WHERE is_active = true AND expires_at > NOW()
	`).Scan(&stats.ActiveSubscriptions)
	if err != nil {
		return nil, err
	}

	// Revenue today
	err = db.Pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount), 0) FROM transactions 
		WHERE type = 'purchase' AND status = 'completed' 
		AND created_at >= CURRENT_DATE
	`).Scan(&stats.RevenueToday)
	if err != nil {
		return nil, err
	}

	// Revenue this month
	err = db.Pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount), 0) FROM transactions 
		WHERE type = 'purchase' AND status = 'completed' 
		AND created_at >= DATE_TRUNC('month', CURRENT_DATE)
	`).Scan(&stats.RevenueMonth)
	if err != nil {
		return nil, err
	}

	// Revenue all time
	err = db.Pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount), 0) FROM transactions 
		WHERE type = 'purchase' AND status = 'completed'
	`).Scan(&stats.RevenueAllTime)
	if err != nil {
		return nil, err
	}

	return stats, nil
}

// GetUserByID получает пользователя по ID
func (db *DB) GetUserByID(ctx context.Context, id int64) (*models.User, error) {
	var user models.User
	err := db.Pool.QueryRow(ctx, `
		SELECT id, telegram_id, username, balance, referrer_id, COALESCE(total_ref_earnings, 0), created_at
		FROM users WHERE id = $1
	`, id).Scan(&user.ID, &user.TelegramID, &user.Username, &user.Balance, &user.ReferrerID, &user.TotalRefEarnings, &user.CreatedAt)

	if err != nil {
		return nil, err
	}

	return &user, nil
}

// FindUserByUsername ищет пользователя по username
func (db *DB) FindUserByUsername(ctx context.Context, username string) (*models.User, error) {
	var user models.User
	err := db.Pool.QueryRow(ctx, `
		SELECT id, telegram_id, username, balance, referrer_id, COALESCE(total_ref_earnings, 0), created_at
		FROM users WHERE LOWER(username) = LOWER($1)
	`, username).Scan(&user.ID, &user.TelegramID, &user.Username, &user.Balance, &user.ReferrerID, &user.TotalRefEarnings, &user.CreatedAt)

	if err != nil {
		return nil, err
	}

	return &user, nil
}

// AddUserBalance добавляет баланс пользователю и создаёт транзакцию
func (db *DB) AddUserBalance(ctx context.Context, userID int64, amount float64, txType string) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Update balance
	_, err = tx.Exec(ctx, `
		UPDATE users SET balance = balance + $1 WHERE id = $2
	`, amount, userID)
	if err != nil {
		return err
	}

	// Create transaction record
	_, err = tx.Exec(ctx, `
		INSERT INTO transactions (user_id, amount, type, status)
		VALUES ($1, $2, $3, 'completed')
	`, userID, amount, txType)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// GetUserTransactions получает транзакции пользователя
func (db *DB) GetUserTransactions(ctx context.Context, userID int64, limit int) ([]models.Transaction, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, user_id, amount, type, status, created_at
		FROM transactions WHERE user_id = $1
		ORDER BY created_at DESC LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transactions []models.Transaction
	for rows.Next() {
		var t models.Transaction
		if err := rows.Scan(&t.ID, &t.UserID, &t.Amount, &t.Type, &t.Status, &t.CreatedAt); err != nil {
			return nil, err
		}
		transactions = append(transactions, t)
	}

	return transactions, nil
}

// GetAllUserTelegramIDs возвращает все telegram_id для рассылки
func (db *DB) GetAllUserTelegramIDs(ctx context.Context) ([]int64, error) {
	rows, err := db.Pool.Query(ctx, `SELECT telegram_id FROM users`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	return ids, nil
}

// TopUpBalanceWithReferral пополняет баланс и начисляет реферальный бонус
// Возвращает: referrerTelegramID (если есть), referralBonus, error
func (db *DB) TopUpBalanceWithReferral(ctx context.Context, userID int64, amount float64) (*int64, float64, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback(ctx)

	// 1. Пополняем баланс пользователя
	_, err = tx.Exec(ctx, `
		UPDATE users SET balance = balance + $1 WHERE id = $2
	`, amount, userID)
	if err != nil {
		return nil, 0, err
	}

	// 2. Создаём транзакцию пополнения
	_, err = tx.Exec(ctx, `
		INSERT INTO transactions (user_id, amount, type, status)
		VALUES ($1, $2, 'top_up', 'completed')
	`, userID, amount)
	if err != nil {
		return nil, 0, err
	}

	// 3. Проверяем есть ли реферер
	var referrerTelegramID *int64
	err = tx.QueryRow(ctx, `
		SELECT referrer_id FROM users WHERE id = $1
	`, userID).Scan(&referrerTelegramID)
	if err != nil {
		return nil, 0, err
	}

	var referralBonus float64 = 0

	// 4. Если есть реферер - начисляем ему 25%
	if referrerTelegramID != nil {
		referralBonus = amount * 0.25

		// Получаем ID реферера по telegram_id
		var referrerID int64
		err = tx.QueryRow(ctx, `
			SELECT id FROM users WHERE telegram_id = $1
		`, *referrerTelegramID).Scan(&referrerID)
		if err == nil {
			// Начисляем бонус рефереру на баланс и в статистику
			_, err = tx.Exec(ctx, `
				UPDATE users SET balance = balance + $1, total_ref_earnings = COALESCE(total_ref_earnings, 0) + $1 WHERE id = $2
			`, referralBonus, referrerID)
			if err != nil {
				return nil, 0, err
			}

			// Создаём транзакцию реферального бонуса
			_, err = tx.Exec(ctx, `
				INSERT INTO transactions (user_id, amount, type, status)
				VALUES ($1, $2, 'referral_bonus', 'completed')
			`, referrerID, referralBonus)
			if err != nil {
				return nil, 0, err
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, 0, err
	}

	return referrerTelegramID, referralBonus, nil
}

// DeductBalance списывает баланс пользователя
func (db *DB) DeductBalance(ctx context.Context, userID int64, amount float64) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Проверяем достаточно ли средств
	var balance float64
	err = tx.QueryRow(ctx, `SELECT balance FROM users WHERE id = $1`, userID).Scan(&balance)
	if err != nil {
		return err
	}

	if balance < amount {
		return fmt.Errorf("insufficient balance: have %.2f, need %.2f", balance, amount)
	}

	// Списываем
	_, err = tx.Exec(ctx, `
		UPDATE users SET balance = balance - $1 WHERE id = $2
	`, amount, userID)
	if err != nil {
		return err
	}

	// Создаём транзакцию покупки
	_, err = tx.Exec(ctx, `
		INSERT INTO transactions (user_id, amount, type, status)
		VALUES ($1, $2, 'purchase', 'completed')
	`, userID, -amount)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// GetReferralCount возвращает количество рефералов пользователя
func (db *DB) GetReferralCount(ctx context.Context, telegramID int64) (int, error) {
	var count int
	err := db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM users WHERE referrer_id = $1
	`, telegramID).Scan(&count)
	return count, err
}

// GetUserReferrals возвращает список рефералов пользователя
func (db *DB) GetUserReferrals(ctx context.Context, telegramID int64) ([]*models.User, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, telegram_id, username, balance, referrer_id, COALESCE(total_ref_earnings, 0), created_at
		FROM users WHERE referrer_id = $1
		ORDER BY created_at DESC
		LIMIT 50
	`, telegramID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		var user models.User
		err := rows.Scan(&user.ID, &user.TelegramID, &user.Username, &user.Balance, &user.ReferrerID, &user.TotalRefEarnings, &user.CreatedAt)
		if err != nil {
			return nil, err
		}
		users = append(users, &user)
	}

	return users, nil
}

// GetReferralsPaginated возвращает список рефералов с пагинацией и сортировкой по доходу
func (db *DB) GetReferralsPaginated(ctx context.Context, referrerTelegramID int64, page int, perPage int) (*models.ReferralListResult, error) {
	result := &models.ReferralListResult{
		CurrentPage: page,
		Referrals:   make([]*models.ReferralInfo, 0),
	}

	// 1. Получаем общее количество рефералов и общий доход
	err := db.Pool.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(SUM(t.amount), 0)
		FROM users u
		LEFT JOIN transactions t ON t.user_id = (SELECT id FROM users WHERE telegram_id = $1) 
			AND t.type = 'referral_bonus' AND t.status = 'completed'
		WHERE u.referrer_id = $1
	`, referrerTelegramID).Scan(&result.TotalCount, &result.TotalEarnings)
	if err != nil {
		return nil, err
	}

	// Вычисляем количество страниц
	if result.TotalCount == 0 {
		result.TotalPages = 1
		return result, nil
	}

	result.TotalPages = (result.TotalCount + perPage - 1) / perPage
	if page > result.TotalPages {
		page = result.TotalPages
	}
	result.CurrentPage = page

	offset := (page - 1) * perPage

	// 2. Получаем рефералов с их доходом (доход = 25% от их пополнений)
	// Доход считаем как сумму referral_bonus транзакций, связанных с этим рефералом
	rows, err := db.Pool.Query(ctx, `
		WITH referral_earnings AS (
			SELECT 
				u.telegram_id,
				u.username,
				u.created_at,
				COALESCE(
					(SELECT SUM(t.amount) * 0.25 
					 FROM transactions t 
					 WHERE t.user_id = u.id 
					 AND t.type IN ('top_up', 'purchase') 
					 AND t.status = 'completed'
					), 0
				) as generated_revenue
			FROM users u
			WHERE u.referrer_id = $1
		)
		SELECT telegram_id, username, generated_revenue, created_at
		FROM referral_earnings
		ORDER BY generated_revenue DESC, created_at DESC
		LIMIT $2 OFFSET $3
	`, referrerTelegramID, perPage, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var ref models.ReferralInfo
		err := rows.Scan(&ref.TelegramID, &ref.Username, &ref.GeneratedRevenue, &ref.JoinedAt)
		if err != nil {
			return nil, err
		}
		result.Referrals = append(result.Referrals, &ref)
	}

	return result, nil
}

// ================= PROMO CODES =================

// CreatePromoCode создаёт новый промокод
func (db *DB) CreatePromoCode(ctx context.Context, code string, amount float64, maxActivations int) (*models.PromoCode, error) {
	var promo models.PromoCode
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO promo_codes (code, amount, max_activations)
		VALUES (UPPER($1), $2, $3)
		RETURNING id, code, amount, max_activations, activations_used, is_active, created_at
	`, code, amount, maxActivations).Scan(
		&promo.ID, &promo.Code, &promo.Amount, &promo.MaxActivations,
		&promo.ActivationsUsed, &promo.IsActive, &promo.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &promo, nil
}

// GetPromoByCode получает промокод по коду
func (db *DB) GetPromoByCode(ctx context.Context, code string) (*models.PromoCode, error) {
	var promo models.PromoCode
	err := db.Pool.QueryRow(ctx, `
		SELECT id, code, amount, max_activations, activations_used, is_active, created_at
		FROM promo_codes WHERE UPPER(code) = UPPER($1)
	`, code).Scan(
		&promo.ID, &promo.Code, &promo.Amount, &promo.MaxActivations,
		&promo.ActivationsUsed, &promo.IsActive, &promo.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &promo, nil
}

// HasUserActivatedPromo проверяет, активировал ли пользователь промокод
func (db *DB) HasUserActivatedPromo(ctx context.Context, promoID int64, telegramID int64) (bool, error) {
	var exists bool
	err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM promo_activations WHERE promo_id = $1 AND telegram_id = $2)
	`, promoID, telegramID).Scan(&exists)
	return exists, err
}

// ActivatePromoCode активирует промокод для пользователя
func (db *DB) ActivatePromoCode(ctx context.Context, promoID int64, userID int64, telegramID int64, amount float64) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// 1. Добавляем запись об активации
	_, err = tx.Exec(ctx, `
		INSERT INTO promo_activations (promo_id, user_id, telegram_id)
		VALUES ($1, $2, $3)
	`, promoID, userID, telegramID)
	if err != nil {
		return err
	}

	// 2. Увеличиваем счётчик активаций
	_, err = tx.Exec(ctx, `
		UPDATE promo_codes SET activations_used = activations_used + 1 WHERE id = $1
	`, promoID)
	if err != nil {
		return err
	}

	// 3. Начисляем баланс пользователю
	_, err = tx.Exec(ctx, `
		UPDATE users SET balance = balance + $1 WHERE id = $2
	`, amount, userID)
	if err != nil {
		return err
	}

	// 4. Создаём транзакцию
	_, err = tx.Exec(ctx, `
		INSERT INTO transactions (user_id, amount, type, status)
		VALUES ($1, $2, 'promo_bonus', 'completed')
	`, userID, amount)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// GetAllPromoCodes получает все промокоды
func (db *DB) GetAllPromoCodes(ctx context.Context) ([]*models.PromoCode, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, code, amount, max_activations, activations_used, is_active, created_at
		FROM promo_codes ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var promos []*models.PromoCode
	for rows.Next() {
		var promo models.PromoCode
		err := rows.Scan(&promo.ID, &promo.Code, &promo.Amount, &promo.MaxActivations,
			&promo.ActivationsUsed, &promo.IsActive, &promo.CreatedAt)
		if err != nil {
			return nil, err
		}
		promos = append(promos, &promo)
	}
	return promos, nil
}

// DeletePromoCode удаляет промокод
func (db *DB) DeletePromoCode(ctx context.Context, code string) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM promo_codes WHERE UPPER(code) = UPPER($1)`, code)
	return err
}

// GetTopReferrers возвращает топ-10 рефоводов
func (db *DB) GetTopReferrers(ctx context.Context, limit int) ([]*models.TopReferrer, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT 
			u.telegram_id,
			u.username,
			COUNT(r.id) as referral_count,
			COALESCE(u.total_ref_earnings, 0) as total_revenue
		FROM users u
		LEFT JOIN users r ON r.referrer_id = u.telegram_id
		GROUP BY u.id, u.telegram_id, u.username, u.total_ref_earnings
		HAVING COUNT(r.id) > 0
		ORDER BY total_revenue DESC, referral_count DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var referrers []*models.TopReferrer
	for rows.Next() {
		var ref models.TopReferrer
		err := rows.Scan(&ref.TelegramID, &ref.Username, &ref.ReferralCount, &ref.TotalRevenue)
		if err != nil {
			return nil, err
		}
		referrers = append(referrers, &ref)
	}
	return referrers, nil
}

// GetPromoStats возвращает статистику по промокодам
func (db *DB) GetPromoStats(ctx context.Context) ([]*models.PromoStats, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT 
			p.code,
			p.amount,
			p.max_activations,
			p.activations_used,
			(p.amount * p.activations_used) as total_bonus_paid
		FROM promo_codes p
		WHERE p.is_active = true
		ORDER BY p.activations_used DESC, p.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []*models.PromoStats
	for rows.Next() {
		var s models.PromoStats
		err := rows.Scan(&s.Code, &s.Amount, &s.MaxActivations, &s.ActivationsUsed, &s.TotalBonusPaid)
		if err != nil {
			return nil, err
		}
		stats = append(stats, &s)
	}
	return stats, nil
}
