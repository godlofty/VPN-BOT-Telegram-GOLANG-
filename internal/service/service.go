package service

import (
	"context"
	"fmt"
	"time"

	"vpn-telegram-bot/internal/database"
	"vpn-telegram-bot/internal/models"
)

// Service бизнес-логика приложения
type Service struct {
	db  *database.DB
	vpn VPNProvider
}

// New создаёт новый сервис
func New(db *database.DB, vpn VPNProvider) *Service {
	return &Service{
		db:  db,
		vpn: vpn,
	}
}

// GetOrCreateUser получает или создаёт пользователя
func (s *Service) GetOrCreateUser(ctx context.Context, telegramID int64, username string) (*models.User, error) {
	return s.db.GetOrCreateUser(ctx, telegramID, username)
}

// GetAllProducts возвращает все продукты
func (s *Service) GetAllProducts(ctx context.Context) ([]models.Product, error) {
	return s.db.GetAllProducts(ctx)
}

// GetProductByID возвращает продукт по ID
func (s *Service) GetProductByID(ctx context.Context, id int64) (*models.Product, error) {
	return s.db.GetProductByID(ctx, id)
}

// GetUserSubscriptions возвращает подписки пользователя
func (s *Service) GetUserSubscriptions(ctx context.Context, userID int64) ([]models.Subscription, error) {
	return s.db.GetUserSubscriptions(ctx, userID)
}

// GetSubscriptionByID возвращает подписку по ID
func (s *Service) GetSubscriptionByID(ctx context.Context, id int64) (*models.Subscription, error) {
	return s.db.GetSubscriptionByID(ctx, id)
}

// CalculatePrice рассчитывает цену с учётом скидки
func (s *Service) CalculatePrice(basePrice float64, months int) (float64, int) {
	var discount int
	switch months {
	case 6:
		discount = 10
	case 12:
		discount = 20
	default:
		discount = 0
	}

	price := basePrice * float64(months)
	if discount > 0 {
		price = price * (1 - float64(discount)/100)
	}

	return price, discount
}

// GetPricingPlans возвращает все планы с ценами
func (s *Service) GetPricingPlans(basePrice float64) []models.PricingPlan {
	return models.CalculatePricingPlans(basePrice)
}

// CreateSubscription создаёт новую подписку
func (s *Service) CreateSubscription(ctx context.Context, user *models.User, productID int64, months int) (*models.Subscription, error) {
	product, err := s.db.GetProductByID(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("product not found: %w", err)
	}

	// Рассчитываем дату истечения
	expiresAt := time.Now().AddDate(0, months, 0)

	// Генерируем username для VPN
	vpnUsername := fmt.Sprintf("tg_%d_%d", user.TelegramID, time.Now().Unix())

	// Создаём пользователя в VPN панели
	keyString, err := s.vpn.CreateUser(ctx, vpnUsername, product.MarzbanTag, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create VPN user: %w", err)
	}

	// Сохраняем подписку в БД
	sub, err := s.db.CreateSubscription(ctx, user.ID, productID, keyString, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("failed to save subscription: %w", err)
	}

	sub.Product = product
	return sub, nil
}

// ExtendSubscription продлевает подписку
func (s *Service) ExtendSubscription(ctx context.Context, subID int64, months int) error {
	sub, err := s.db.GetSubscriptionByID(ctx, subID)
	if err != nil {
		return err
	}

	// Если подписка истекла, продлеваем от текущей даты
	baseTime := sub.ExpiresAt
	if baseTime.Before(time.Now()) {
		baseTime = time.Now()
	}

	newExpiresAt := baseTime.AddDate(0, months, 0)

	// Продлеваем в VPN панели
	// vpnUsername можно извлечь из key_string или хранить отдельно
	// TODO: implement proper username extraction

	return s.db.ExtendSubscription(ctx, subID, newExpiresAt)
}

// === Admin Methods ===

// GetAdminStats возвращает статистику для админ-панели
func (s *Service) GetAdminStats(ctx context.Context) (*models.AdminStats, error) {
	return s.db.GetAdminStats(ctx)
}

// FindUser ищет пользователя по ID или username
func (s *Service) FindUser(ctx context.Context, query string) (*models.UserProfile, error) {
	var user *models.User
	var err error

	// Пробуем найти по telegram_id
	if telegramID, parseErr := parseIntSafe(query); parseErr == nil {
		user, err = s.db.GetUserByTelegramID(ctx, telegramID)
	}

	// Если не нашли по ID, ищем по username
	if user == nil || err != nil {
		user, err = s.db.FindUserByUsername(ctx, query)
	}

	if err != nil {
		return nil, err
	}

	// Получаем подписки
	subs, err := s.db.GetUserSubscriptions(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	// Получаем транзакции
	txs, err := s.db.GetUserTransactions(ctx, user.ID, 10)
	if err != nil {
		return nil, err
	}

	return &models.UserProfile{
		User:          user,
		Subscriptions: subs,
		Transactions:  txs,
	}, nil
}

// AddUserBalance добавляет баланс пользователю (admin)
func (s *Service) AddUserBalance(ctx context.Context, telegramID int64, amount float64) error {
	user, err := s.db.GetUserByTelegramID(ctx, telegramID)
	if err != nil {
		return err
	}
	return s.db.AddUserBalance(ctx, user.ID, amount, "manual_deposit")
}

// GiftSubscription создаёт бесплатную подписку (admin)
func (s *Service) GiftSubscription(ctx context.Context, telegramID int64, productID int64, days int) (*models.Subscription, error) {
	user, err := s.db.GetUserByTelegramID(ctx, telegramID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	product, err := s.db.GetProductByID(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("product not found: %w", err)
	}

	// Рассчитываем дату истечения
	expiresAt := time.Now().AddDate(0, 0, days)

	// Генерируем username для VPN
	vpnUsername := fmt.Sprintf("gift_tg_%d_%d", user.TelegramID, time.Now().Unix())

	// Создаём пользователя в VPN панели
	keyString, err := s.vpn.CreateUser(ctx, vpnUsername, product.MarzbanTag, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create VPN user: %w", err)
	}

	// Сохраняем подписку в БД
	sub, err := s.db.CreateSubscription(ctx, user.ID, productID, keyString, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("failed to save subscription: %w", err)
	}

	sub.Product = product
	return sub, nil
}

// GetAllUserTelegramIDs возвращает все telegram_id для рассылки
func (s *Service) GetAllUserTelegramIDs(ctx context.Context) ([]int64, error) {
	return s.db.GetAllUserTelegramIDs(ctx)
}

// GetUserByTelegramID получает пользователя по Telegram ID
func (s *Service) GetUserByTelegramID(ctx context.Context, telegramID int64) (*models.User, error) {
	return s.db.GetUserByTelegramID(ctx, telegramID)
}

// === Referral System ===

// UserExists проверяет существует ли пользователь
func (s *Service) UserExists(ctx context.Context, telegramID int64) (bool, error) {
	return s.db.UserExists(ctx, telegramID)
}

// CreateUserWithReferrer создаёт пользователя с реферером
func (s *Service) CreateUserWithReferrer(ctx context.Context, telegramID int64, username string, referrerTelegramID int64) (*models.User, error) {
	return s.db.CreateUserWithReferrer(ctx, telegramID, username, referrerTelegramID)
}

// GetReferralCount возвращает количество рефералов пользователя
func (s *Service) GetReferralCount(ctx context.Context, telegramID int64) (int, error) {
	return s.db.GetReferralCount(ctx, telegramID)
}

// GetUserReferrals возвращает список рефералов пользователя
func (s *Service) GetUserReferrals(ctx context.Context, telegramID int64) ([]*models.User, error) {
	return s.db.GetUserReferrals(ctx, telegramID)
}

// GetReferralsPaginated возвращает список рефералов с пагинацией
func (s *Service) GetReferralsPaginated(ctx context.Context, referrerTelegramID int64, page int) (*models.ReferralListResult, error) {
	const perPage = 10
	if page < 1 {
		page = 1
	}
	return s.db.GetReferralsPaginated(ctx, referrerTelegramID, page, perPage)
}

// TopUpBalanceWithReferral пополняет баланс с учётом реферальной программы
// Возвращает: referrerTelegramID, referralBonus, error
func (s *Service) TopUpBalanceWithReferral(ctx context.Context, userID int64, amount float64) (*int64, float64, error) {
	return s.db.TopUpBalanceWithReferral(ctx, userID, amount)
}

// DeductBalance списывает баланс пользователя
func (s *Service) DeductBalance(ctx context.Context, userID int64, amount float64) error {
	return s.db.DeductBalance(ctx, userID, amount)
}

// CreateSubscription создаёт новую подписку (упрощённая версия для оплаты с баланса)
func (s *Service) CreateSubscriptionSimple(ctx context.Context, userID int64, productID int64, expiresAt time.Time) (*models.Subscription, error) {
	product, err := s.db.GetProductByID(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("product not found: %w", err)
	}

	user, err := s.db.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	// Генерируем username для VPN
	vpnUsername := fmt.Sprintf("tg_%d_%d", user.TelegramID, time.Now().Unix())

	// Создаём пользователя в VPN панели
	keyString, err := s.vpn.CreateUser(ctx, vpnUsername, product.MarzbanTag, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create VPN user: %w", err)
	}

	// Сохраняем подписку в БД
	sub, err := s.db.CreateSubscription(ctx, userID, productID, keyString, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("failed to save subscription: %w", err)
	}

	sub.Product = product
	return sub, nil
}

// parseIntSafe безопасно парсит int64
func parseIntSafe(s string) (int64, error) {
	var result int64
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}

// ================= PROMO CODES =================

// CreatePromoCode создаёт новый промокод
func (s *Service) CreatePromoCode(ctx context.Context, code string, amount float64, maxActivations int) (*models.PromoCode, error) {
	return s.db.CreatePromoCode(ctx, code, amount, maxActivations)
}

// GetPromoByCode получает промокод по коду
func (s *Service) GetPromoByCode(ctx context.Context, code string) (*models.PromoCode, error) {
	return s.db.GetPromoByCode(ctx, code)
}

// ActivatePromoForUser активирует промокод для пользователя
// Возвращает: amount (сумма начисления), error
func (s *Service) ActivatePromoForUser(ctx context.Context, code string, userID int64, telegramID int64) (float64, error) {
	// 1. Получаем промокод
	promo, err := s.db.GetPromoByCode(ctx, code)
	if err != nil {
		return 0, fmt.Errorf("промокод не найден")
	}

	// 2. Проверяем активен ли
	if !promo.IsActive {
		return 0, fmt.Errorf("промокод неактивен")
	}

	// 3. Проверяем лимит активаций
	if promo.ActivationsUsed >= promo.MaxActivations {
		return 0, fmt.Errorf("промокод исчерпан")
	}

	// 4. Проверяем не использовал ли уже
	used, err := s.db.HasUserActivatedPromo(ctx, promo.ID, telegramID)
	if err != nil {
		return 0, err
	}
	if used {
		return 0, fmt.Errorf("вы уже использовали этот промокод")
	}

	// 5. Активируем
	err = s.db.ActivatePromoCode(ctx, promo.ID, userID, telegramID, promo.Amount)
	if err != nil {
		return 0, err
	}

	return promo.Amount, nil
}

// GetAllPromoCodes получает все промокоды
func (s *Service) GetAllPromoCodes(ctx context.Context) ([]*models.PromoCode, error) {
	return s.db.GetAllPromoCodes(ctx)
}

// DeletePromoCode удаляет промокод
func (s *Service) DeletePromoCode(ctx context.Context, code string) error {
	return s.db.DeletePromoCode(ctx, code)
}

// GetTopReferrers возвращает топ-10 рефоводов
func (s *Service) GetTopReferrers(ctx context.Context) ([]*models.TopReferrer, error) {
	return s.db.GetTopReferrers(ctx, 10)
}

// GetPromoStats возвращает статистику по промокодам
func (s *Service) GetPromoStats(ctx context.Context) ([]*models.PromoStats, error) {
	return s.db.GetPromoStats(ctx)
}
