package models

import "time"

// User представляет пользователя бота
type User struct {
	ID              int64     `db:"id"`
	TelegramID      int64     `db:"telegram_id"`
	Username        string    `db:"username"`
	Balance         float64   `db:"balance"`
	ReferrerID      *int64    `db:"referrer_id"`       // Telegram ID того, кто пригласил
	TotalRefEarnings float64  `db:"total_ref_earnings"` // Всего заработано с рефералов
	CreatedAt       time.Time `db:"created_at"`
}

// Product представляет VPN продукт/локацию
type Product struct {
	ID          int64   `db:"id"`
	Name        string  `db:"name"`
	CountryFlag string  `db:"country_flag"`
	BasePrice   float64 `db:"base_price"`
	MarzbanTag  string  `db:"marzban_tag"`
	Description string  `db:"description"`
	SortOrder   int     `db:"sort_order"`
}

// Subscription представляет подписку пользователя
type Subscription struct {
	ID        int64     `db:"id"`
	UserID    int64     `db:"user_id"`
	ProductID int64     `db:"product_id"`
	KeyString string    `db:"key_string"` // vless:// link
	ExpiresAt time.Time `db:"expires_at"`
	IsActive  bool      `db:"is_active"`
	CreatedAt time.Time `db:"created_at"`

	// Joined fields
	Product *Product `db:"-"`
}

// TransactionType тип транзакции
type TransactionType string

const (
	TransactionTopUp         TransactionType = "top_up"
	TransactionPurchase      TransactionType = "purchase"
	TransactionRefund        TransactionType = "refund"
	TransactionReferralBonus TransactionType = "referral_bonus"
)

// TransactionStatus статус транзакции
type TransactionStatus string

const (
	TransactionPending   TransactionStatus = "pending"
	TransactionCompleted TransactionStatus = "completed"
	TransactionFailed    TransactionStatus = "failed"
	TransactionCancelled TransactionStatus = "cancelled"
)

// Transaction представляет финансовую транзакцию
type Transaction struct {
	ID        int64             `db:"id"`
	UserID    int64             `db:"user_id"`
	Amount    float64           `db:"amount"`
	Type      TransactionType   `db:"type"`
	Status    TransactionStatus `db:"status"`
	CreatedAt time.Time         `db:"created_at"`
}

// PricingPlan план с расчётом цены
type PricingPlan struct {
	Months   int
	Discount int     // процент скидки
	Price    float64 // итоговая цена
}

// CalculatePricingPlans рассчитывает планы с учётом скидок
func CalculatePricingPlans(basePrice float64) []PricingPlan {
	return []PricingPlan{
		{Months: 1, Discount: 0, Price: basePrice},
		{Months: 3, Discount: 0, Price: basePrice * 3},
		{Months: 6, Discount: 10, Price: basePrice * 6 * 0.90},
		{Months: 12, Discount: 20, Price: basePrice * 12 * 0.80},
	}
}

// AdminStats статистика для админ-панели
type AdminStats struct {
	TotalUsers          int64   `db:"total_users"`
	NewUsersToday       int64   `db:"new_users_today"`
	ActiveSubscriptions int64   `db:"active_subscriptions"`
	RevenueToday        float64 `db:"revenue_today"`
	RevenueMonth        float64 `db:"revenue_month"`
	RevenueAllTime      float64 `db:"revenue_all_time"`
}

// TopReferrer информация о топ-рефоводе
type TopReferrer struct {
	TelegramID       int64   `db:"telegram_id"`
	Username         string  `db:"username"`
	ReferralCount    int     `db:"referral_count"`
	TotalRevenue     float64 `db:"total_revenue"`
}

// PromoStats расширенная статистика промокода
type PromoStats struct {
	Code            string  `db:"code"`
	Amount          float64 `db:"amount"`
	MaxActivations  int     `db:"max_activations"`
	ActivationsUsed int     `db:"activations_used"`
	TotalBonusPaid  float64 `db:"total_bonus_paid"`
}

// UserProfile профиль пользователя для админки
type UserProfile struct {
	User          *User
	Subscriptions []Subscription
	Transactions  []Transaction
}

// PromoCode представляет промокод
type PromoCode struct {
	ID              int64     `db:"id"`
	Code            string    `db:"code"`             // Уникальный код (например: SALE50)
	Amount          float64   `db:"amount"`           // Сумма начисления
	MaxActivations  int       `db:"max_activations"`  // Максимум активаций
	ActivationsUsed int       `db:"activations_used"` // Использовано активаций
	IsActive        bool      `db:"is_active"`        // Активен ли код
	CreatedAt       time.Time `db:"created_at"`
}

// PromoActivation запись об активации промокода пользователем
type PromoActivation struct {
	ID         int64     `db:"id"`
	PromoID    int64     `db:"promo_id"`
	UserID     int64     `db:"user_id"`
	TelegramID int64     `db:"telegram_id"`
	ActivatedAt time.Time `db:"activated_at"`
}

// ReferralInfo информация о реферале с его доходом
type ReferralInfo struct {
	TelegramID       int64     `db:"telegram_id"`
	Username         string    `db:"username"`
	GeneratedRevenue float64   `db:"generated_revenue"` // Сколько принёс рефереру
	JoinedAt         time.Time `db:"created_at"`
}

// ReferralListResult результат запроса списка рефералов с пагинацией
type ReferralListResult struct {
	Referrals     []*ReferralInfo
	TotalCount    int
	TotalEarnings float64
	CurrentPage   int
	TotalPages    int
}
