package handlers

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"vpn-telegram-bot/internal/models"

	tele "gopkg.in/telebot.v3"
)

// broadcastState хранит состояние рассылки
type broadcastState struct {
	mu             sync.Mutex
	isActive       bool
	waitingMsg     bool
	waitingConfirm bool
	adminID        int64
	message        *tele.Message
}

var broadcast = &broadcastState{}

// issueState хранит состояние выдачи ключа
type issueState struct {
	mu       sync.Mutex
	sessions map[int64]*issueSession
}

type issueSession struct {
	step      int // 1=product, 2=days, 3=userID
	productID int64
	days      int
}

var issue = &issueState{
	sessions: make(map[int64]*issueSession),
}

// adminSearchState хранит состояние поиска пользователя
type adminSearchState struct {
	mu       sync.Mutex
	waiting  map[int64]bool  // adminID -> waiting for user input
	addBalTo map[int64]int64 // adminID -> targetUserID (waiting for amount)
}

var adminSearch = &adminSearchState{
	waiting:  make(map[int64]bool),
	addBalTo: make(map[int64]int64),
}

// promoWizardState хранит состояние создания промокода
type promoWizardState struct {
	mu       sync.Mutex
	sessions map[int64]*promoWizardSession
}

type promoWizardSession struct {
	step   int     // 1=code, 2=amount, 3=activations
	code   string
	amount float64
}

var promoWizard = &promoWizardState{
	sessions: make(map[int64]*promoWizardSession),
}

// promoDeleteState хранит состояние удаления промокода
type promoDeleteState struct {
	mu      sync.Mutex
	waiting map[int64]bool
}

var promoDelete = &promoDeleteState{
	waiting: make(map[int64]bool),
}

// userPromoState хранит состояние ввода промокода пользователем
type userPromoState struct {
	mu      sync.RWMutex
	waiting map[int64]bool // userID -> waiting for promo code
}

var userPromo = &userPromoState{
	waiting: make(map[int64]bool),
}

// SetUserPromoMode устанавливает режим ввода промокода
func SetUserPromoMode(userID int64, active bool) {
	userPromo.mu.Lock()
	defer userPromo.mu.Unlock()
	if active {
		userPromo.waiting[userID] = true
	} else {
		delete(userPromo.waiting, userID)
	}
}

// IsUserInPromoMode проверяет режим ввода промокода
func IsUserInPromoMode(userID int64) bool {
	userPromo.mu.RLock()
	defer userPromo.mu.RUnlock()
	return userPromo.waiting[userID]
}

// supportState хранит состояние тикет-системы поддержки
type supportState struct {
	mu              sync.RWMutex
	userInSupport   map[int64]bool  // userID -> в режиме поддержки
	adminReplyingTo map[int64]int64 // adminID -> userID которому отвечает
}

var support = &supportState{
	userInSupport:   make(map[int64]bool),
	adminReplyingTo: make(map[int64]int64),
}

// IsUserInSupportMode проверяет, находится ли пользователь в режиме поддержки
func IsUserInSupportMode(userID int64) bool {
	support.mu.RLock()
	defer support.mu.RUnlock()
	return support.userInSupport[userID]
}

// SetUserSupportMode устанавливает режим поддержки для пользователя
func SetUserSupportMode(userID int64, active bool) {
	support.mu.Lock()
	defer support.mu.Unlock()
	if active {
		support.userInSupport[userID] = true
	} else {
		delete(support.userInSupport, userID)
	}
}

// GetAdminReplyTarget возвращает ID пользователя, которому админ отвечает
func GetAdminReplyTarget(adminID int64) int64 {
	support.mu.RLock()
	defer support.mu.RUnlock()
	return support.adminReplyingTo[adminID]
}

// SetAdminReplyTarget устанавливает пользователя для ответа админа
func SetAdminReplyTarget(adminID int64, userID int64) {
	support.mu.Lock()
	defer support.mu.Unlock()
	if userID > 0 {
		support.adminReplyingTo[adminID] = userID
	} else {
		delete(support.adminReplyingTo, adminID)
	}
}

// AdminMiddleware проверяет, является ли пользователь администратором
func (h *Handler) AdminMiddleware() tele.MiddlewareFunc {
	return func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			userID := c.Sender().ID
			for _, adminID := range h.adminIDs {
				if userID == adminID {
					return next(c)
				}
			}
			return c.Send("❌ Доступ запрещён. Эта команда доступна только администраторам.")
		}
	}
}

// isAdmin проверяет, является ли пользователь администратором
func (h *Handler) isAdmin(userID int64) bool {
	for _, adminID := range h.adminIDs {
		if userID == adminID {
			return true
		}
	}
	return false
}

// RegisterAdmin регистрирует админ-обработчики
func (h *Handler) RegisterAdmin(b *tele.Bot) {
	adminGroup := b.Group()
	adminGroup.Use(h.AdminMiddleware())

	// Admin commands
	adminGroup.Handle("/admin", h.HandleAdmin)
	adminGroup.Handle("/stats", h.HandleAdminStats)
	adminGroup.Handle("/find", h.HandleFindUser)
	adminGroup.Handle("/addbal", h.HandleAddBalance)
	adminGroup.Handle("/gift", h.HandleGiftSub)
	adminGroup.Handle("/issue", h.HandleIssueStart)
	adminGroup.Handle("/broadcast", h.HandleAdminBroadcast)
	adminGroup.Handle("/ahelp", h.HandleAdminHelp)

	// Flash Sale
	h.RegisterFlashSale(b, adminGroup)

	// Admin callbacks
	adminGroup.Handle(&tele.Btn{Unique: "admin_stats"}, h.HandleAdminStats)
	adminGroup.Handle(&tele.Btn{Unique: "admin_users"}, h.HandleAdminUsers)
	adminGroup.Handle(&tele.Btn{Unique: "admin_broadcast"}, h.HandleAdminBroadcast)
	adminGroup.Handle(&tele.Btn{Unique: "admin_cancel_broadcast"}, h.HandleCancelBroadcast)
	adminGroup.Handle(&tele.Btn{Unique: "admin_confirm_broadcast"}, h.HandleConfirmBroadcast)
	adminGroup.Handle(&tele.Btn{Unique: "admin_back"}, h.HandleAdmin)
	adminGroup.Handle(&tele.Btn{Unique: "admin_issue"}, h.HandleIssueStart)
	adminGroup.Handle(&tele.Btn{Unique: "admin_help"}, h.HandleAdminHelp)
	adminGroup.Handle(&tele.Btn{Unique: "admin_find_user"}, h.HandleAdminFindUserStart)
	adminGroup.Handle(&tele.Btn{Unique: "admin_addbal_start"}, h.HandleAdminAddBalStart)

	// Quick flash sale buttons
	adminGroup.Handle(&tele.Btn{Unique: "flash_quick"}, h.HandleFlashQuick)

	// User-specific actions from profile
	adminGroup.Handle(&tele.Btn{Unique: "admin_addbal_user"}, h.HandleAdminAddBalUser)
	adminGroup.Handle(&tele.Btn{Unique: "admin_addbal_amount"}, h.HandleAdminAddBalAmountCallback)
	adminGroup.Handle(&tele.Btn{Unique: "admin_gift_user"}, h.HandleAdminGiftUser)
	adminGroup.Handle(&tele.Btn{Unique: "admin_gift_product"}, h.HandleAdminGiftProduct)
	adminGroup.Handle(&tele.Btn{Unique: "admin_gift_days"}, h.HandleAdminGiftDays)

	// Issue key flow callbacks
	adminGroup.Handle(&tele.Btn{Unique: "issue_product"}, h.HandleIssueProduct)
	adminGroup.Handle(&tele.Btn{Unique: "issue_days"}, h.HandleIssueDays)
	adminGroup.Handle(&tele.Btn{Unique: "issue_cancel"}, h.HandleIssueCancel)
	adminGroup.Handle(&tele.Btn{Unique: "issue_no_user"}, h.HandleIssueNoUser)

	// Support ticket reply
	adminGroup.Handle(&tele.Btn{Unique: "support_reply"}, h.HandleSupportReplyStart)
	adminGroup.Handle(&tele.Btn{Unique: "support_cancel_reply"}, h.HandleSupportCancelReply)

	// Promo code management
	adminGroup.Handle(&tele.Btn{Unique: "admin_promo"}, h.HandleAdminPromo)
	adminGroup.Handle(&tele.Btn{Unique: "admin_promo_create"}, h.HandleAdminPromoCreate)
	adminGroup.Handle(&tele.Btn{Unique: "admin_promo_list"}, h.HandleAdminPromoList)
	adminGroup.Handle(&tele.Btn{Unique: "admin_promo_delete"}, h.HandleAdminPromoDelete)
	adminGroup.Handle(&tele.Btn{Unique: "admin_promo_cancel"}, h.HandleAdminPromoCancel)
	adminGroup.Handle(&tele.Btn{Unique: "admin_promo_stats"}, h.HandleAdminPromoStats)

	// Top referrers
	adminGroup.Handle(&tele.Btn{Unique: "admin_top_refs"}, h.HandleAdminTopRefs)

	// Support ticket management (close ticket from group)
	b.Handle(&tele.Btn{Unique: "admin_close_ticket"}, h.HandleAdminCloseTicket)

	// Handle text messages for broadcast, issue, user search, and support reply
	b.Handle(tele.OnText, func(c tele.Context) error {
		userID := c.Sender().ID

		// DEBUG: Log every text message
		log.Printf("📨 OnText received from user %d, chat %d, text: %s", userID, c.Chat().ID, c.Text())
		log.Printf("📨 Support mode check: isAdmin=%v, inSupportMode=%v", h.isAdmin(userID), IsUserInSupportMode(userID))

		// === SUPPORT GROUP BRIDGE (Admin replies) ===
		// Проверяем если это сообщение из группы поддержки
		if c.Chat() != nil && c.Chat().ID == h.supportGroupID {
			log.Printf("📨 Message from support group, handling as admin reply")
			return h.handleSupportGroupMessage(c)
		}

		// === USER PROMO CODE MODE ===
		if !h.isAdmin(userID) && IsUserInPromoMode(userID) {
			return h.HandleUserPromoInput(c)
		}

		// === USER SUPPORT MODE ===
		// Check if user is in support chat mode (ANY user, including admins for testing)
		if IsUserInSupportMode(userID) {
			log.Printf("🎫 User %d in support mode, forwarding message to support group", userID)
			return h.HandleSupportUserMessage(c)
		}

		// === ADMIN HANDLERS ===
		if !h.isAdmin(userID) {
			return nil
		}

		// Check if admin is replying to support ticket
		replyTarget := GetAdminReplyTarget(userID)
		if replyTarget > 0 {
			return h.HandleSupportAdminReply(c, replyTarget)
		}

		// Check if admin is waiting for broadcast message
		broadcast.mu.Lock()
		waitingBroadcast := broadcast.waitingMsg && broadcast.adminID == userID
		broadcast.mu.Unlock()

		if waitingBroadcast {
			return h.HandleBroadcastMessage(c)
		}

		// Check if admin is waiting for user search input
		adminSearch.mu.Lock()
		waitingSearch := adminSearch.waiting[userID]
		addBalTarget := adminSearch.addBalTo[userID]
		adminSearch.mu.Unlock()

		if addBalTarget > 0 {
			return h.HandleAdminAddBalAmount(c, addBalTarget)
		}

		if waitingSearch {
			return h.HandleAdminFindUserInput(c)
		}

		// Check if admin is in issue flow waiting for user ID
		issue.mu.Lock()
		session, exists := issue.sessions[c.Sender().ID]
		issue.mu.Unlock()

		if exists && session.step == 3 {
			return h.HandleIssueUserID(c)
		}

		// Check if admin is in promo wizard
		promoWizard.mu.Lock()
		promoSession, promoExists := promoWizard.sessions[c.Sender().ID]
		promoWizard.mu.Unlock()

		if promoExists {
			return h.HandleAdminPromoWizardInput(c, promoSession)
		}

		// Check if admin is deleting promo
		promoDelete.mu.Lock()
		deletingPromo := promoDelete.waiting[c.Sender().ID]
		promoDelete.mu.Unlock()

		if deletingPromo {
			return h.HandleAdminPromoDeleteInput(c)
		}

		return nil
	})

	// Handle photo messages for broadcast and support
	b.Handle(tele.OnPhoto, func(c tele.Context) error {
		userID := c.Sender().ID

		// === SUPPORT GROUP BRIDGE (Admin replies with photos) ===
		if c.Chat() != nil && c.Chat().ID == h.supportGroupID {
			return h.handleSupportGroupMessage(c)
		}

		// User support mode - forward photos too (ANY user, including admins)
		if IsUserInSupportMode(userID) {
			log.Printf("🎫 User %d in support mode, forwarding photo", userID)
			return h.HandleSupportUserMessage(c)
		}

		// Admin broadcast
		broadcast.mu.Lock()
		waiting := broadcast.waitingMsg && broadcast.adminID == userID
		broadcast.mu.Unlock()

		if waiting && h.isAdmin(userID) {
			return h.HandleBroadcastMessage(c)
		}
		return nil
	})

	// Register support commands for all users
	b.Handle("/stop_support", h.HandleStopSupport)

	// Dashboard initialization (admin only, in support group)
	b.Handle("/init_dashboard", h.HandleInitDashboard)

	// Initialize support tracker
	InitSupportTracker(b, h.supportGroupID)
}

// ================= ADMIN PANEL =================

// HandleAdmin показывает админ-панель (GUI Dashboard)
func (h *Handler) HandleAdmin(c tele.Context) error {
	ctx := context.Background()

	// Получаем статистику для дашборда
	stats, err := h.svc.GetAdminStats(ctx)
	if err != nil {
		log.Printf("Error getting admin stats: %v", err)
		stats = &models.AdminStats{} // fallback to zeros
	}

	// Проверяем активную распродажу
	var saleStatus string
	if flashSale.IsActive() {
		saleStatus = fmt.Sprintf("\n🔥 *Распродажа:* -%d%% (до %s)",
			flashSale.GetDiscount(), flashSale.GetEndTime().Format("15:04"))
	}

	// Форматируем dashboard
	text := fmt.Sprintf(`👮‍♂️ *Центр Управления X-RAY*

📅 *Сводка за сегодня:*
➕ Новых пользователей: *%d*
💰 Прибыль за сутки: *%.0f ₽*
💎 Активных подписок: *%d*
👥 Всего пользователей: *%d*%s

_Выберите действие в меню ниже:_`,
		stats.NewUsersToday,
		stats.RevenueToday,
		stats.ActiveSubscriptions,
		stats.TotalUsers,
		saleStatus)

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(
			menu.Data("📊 Полная статистика", "admin_stats"),
			menu.Data("📢 Рассылка", "admin_broadcast"),
		),
		menu.Row(
			menu.Data("🎟 Промокоды", "admin_promo"),
			menu.Data("🏆 Топ Рефоводов", "admin_top_refs"),
		),
		menu.Row(
			menu.Data("👥 Управление юзерами", "admin_users"),
			menu.Data("⚡️ Flash Sale", "flash_start"),
		),
		menu.Row(
			menu.Data("🔑 Выдать ключ", "admin_issue"),
			menu.Data("📜 Команды", "admin_help"),
		),
		menu.Row(menu.Data("⬅️ Выход", "back_main")),
	)

	// Try to edit, fallback to send
	if c.Callback() != nil {
		return c.Edit(text, menu, tele.ModeMarkdown)
	}
	return c.Send(text, menu, tele.ModeMarkdown)
}

// HandleAdminStats показывает статистику
func (h *Handler) HandleAdminStats(c tele.Context) error {
	stats, err := h.svc.GetAdminStats(context.Background())
	if err != nil {
		log.Printf("Error getting admin stats: %v", err)
		return c.Send("❌ Ошибка получения статистики")
	}

	text := fmt.Sprintf(`📊 *Статистика*

👥 *Пользователи:* %d
🔑 *Активные подписки:* %d

💰 *Доход:*
• Сегодня: %.2f ₽
• За месяц: %.2f ₽
• Всего: %.2f ₽`,
		stats.TotalUsers,
		stats.ActiveSubscriptions,
		stats.RevenueToday,
		stats.RevenueMonth,
		stats.RevenueAllTime,
	)

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("🔄 Обновить", "admin_stats")),
		menu.Row(menu.Data("⬅️ Назад", "admin_back")),
	)

	if c.Callback() != nil {
		return c.Edit(text, menu, tele.ModeMarkdown)
	}
	return c.Send(text, menu, tele.ModeMarkdown)
}

// HandleAdminUsers показывает меню управления пользователями
func (h *Handler) HandleAdminUsers(c tele.Context) error {
	text := `👥 *Управление пользователями*

Выберите действие или используйте команды:`

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(
			menu.Data("🔎 Найти юзера", "admin_find_user"),
			menu.Data("💳 Пополнить баланс", "admin_addbal_start"),
		),
		menu.Row(menu.Data("🔑 Выдать ключ", "admin_issue")),
		menu.Row(menu.Data("⬅️ Назад", "admin_back")),
	)

	if c.Callback() != nil {
		return c.Edit(text, menu, tele.ModeMarkdown)
	}
	return c.Send(text, menu, tele.ModeMarkdown)
}

// ================= INTERACTIVE USER SEARCH =================

// HandleAdminFindUserStart начинает интерактивный поиск пользователя
func (h *Handler) HandleAdminFindUserStart(c tele.Context) error {
	adminSearch.mu.Lock()
	adminSearch.waiting[c.Sender().ID] = true
	delete(adminSearch.addBalTo, c.Sender().ID)
	adminSearch.mu.Unlock()

	text := `🔎 *Поиск пользователя*

👇 Введите Telegram ID пользователя или его @username:

_(Или перешлите любое сообщение от него)_`

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("❌ Отмена", "admin_back")),
	)

	if c.Callback() != nil {
		return c.Edit(text, menu, tele.ModeMarkdown)
	}
	return c.Send(text, menu, tele.ModeMarkdown)
}

// HandleAdminFindUserInput обрабатывает ввод ID пользователя
func (h *Handler) HandleAdminFindUserInput(c tele.Context) error {
	adminSearch.mu.Lock()
	delete(adminSearch.waiting, c.Sender().ID)
	adminSearch.mu.Unlock()

	query := strings.TrimSpace(c.Text())
	query = strings.TrimPrefix(query, "@")

	// Проверяем если это переслано
	if c.Message().IsForwarded() && c.Message().OriginalSender != nil {
		query = strconv.FormatInt(c.Message().OriginalSender.ID, 10)
	}

	profile, err := h.svc.FindUser(context.Background(), query)
	if err != nil {
		menu := &tele.ReplyMarkup{}
		menu.Inline(
			menu.Row(menu.Data("🔎 Искать снова", "admin_find_user")),
		menu.Row(menu.Data("⬅️ Назад", "admin_back")),
		)
		return c.Send(fmt.Sprintf("❌ Пользователь `%s` не найден.", query), menu, tele.ModeMarkdown)
	}

	return h.showUserProfile(c, profile)
}

// showUserProfile отображает профиль найденного пользователя
func (h *Handler) showUserProfile(c tele.Context, profile *models.UserProfile) error {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("👤 *Пользователь #%d*\n\n", profile.User.ID))
	sb.WriteString(fmt.Sprintf("🆔 Telegram ID: `%d`\n", profile.User.TelegramID))
	if profile.User.Username != "" {
		sb.WriteString(fmt.Sprintf("📝 Username: @%s\n", profile.User.Username))
	}
	sb.WriteString(fmt.Sprintf("💰 Баланс: *%.0f ₽*\n", profile.User.Balance))
	sb.WriteString(fmt.Sprintf("📅 Регистрация: %s\n", profile.User.CreatedAt.Format("02.01.2006")))

	// Subscriptions
	if len(profile.Subscriptions) > 0 {
		sb.WriteString(fmt.Sprintf("\n🔑 *Подписки (%d):*\n", len(profile.Subscriptions)))
		for _, sub := range profile.Subscriptions {
			status := "✅"
			if !sub.IsActive || sub.ExpiresAt.Before(time.Now()) {
				status = "❌"
			}
			sb.WriteString(fmt.Sprintf("• %s %s до %s %s\n",
				sub.Product.CountryFlag, sub.Product.Name,
				sub.ExpiresAt.Format("02.01.06"), status))
		}
	} else {
		sb.WriteString("\n🔑 _Нет подписок_\n")
	}

	// Recent transactions
	if len(profile.Transactions) > 0 {
		sb.WriteString("\n💳 *Последние транзакции:*\n")
		count := len(profile.Transactions)
		if count > 5 {
			count = 5
		}
		for _, tx := range profile.Transactions[:count] {
			sb.WriteString(fmt.Sprintf("• %.0f ₽ (%s) — %s\n",
				tx.Amount, tx.Type, tx.CreatedAt.Format("02.01")))
		}
	}

	menu := &tele.ReplyMarkup{}
	userIDStr := strconv.FormatInt(profile.User.TelegramID, 10)
	menu.Inline(
		menu.Row(
			menu.Data("💳 Пополнить баланс", "admin_addbal_user", userIDStr),
			menu.Data("🎁 Подарить ключ", "admin_gift_user", userIDStr),
		),
		menu.Row(
			menu.Data("🔎 Найти другого", "admin_find_user"),
			menu.Data("⬅️ Назад", "admin_back"),
		),
	)

	return c.Send(sb.String(), menu, tele.ModeMarkdown)
}

// ================= INTERACTIVE ADD BALANCE =================

// HandleAdminAddBalStart начинает интерактивное пополнение баланса
func (h *Handler) HandleAdminAddBalStart(c tele.Context) error {
	// Проверяем, есть ли данные от кнопки (user ID)
	if c.Callback() != nil && c.Callback().Data != "" {
		userID, err := strconv.ParseInt(c.Callback().Data, 10, 64)
		if err == nil && userID > 0 {
			return h.promptAddBalAmount(c, userID)
		}
	}

	// Иначе спрашиваем ID
	adminSearch.mu.Lock()
	adminSearch.waiting[c.Sender().ID] = true
	delete(adminSearch.addBalTo, c.Sender().ID)
	adminSearch.mu.Unlock()

	text := `💳 *Пополнение баланса*

👇 Введите Telegram ID пользователя, которому нужно пополнить баланс:`

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("❌ Отмена", "admin_back")),
	)

	if c.Callback() != nil {
		return c.Edit(text, menu, tele.ModeMarkdown)
	}
	return c.Send(text, menu, tele.ModeMarkdown)
}

// promptAddBalAmount спрашивает сумму пополнения
func (h *Handler) promptAddBalAmount(c tele.Context, userID int64) error {
	adminSearch.mu.Lock()
	delete(adminSearch.waiting, c.Sender().ID)
	adminSearch.addBalTo[c.Sender().ID] = userID
	adminSearch.mu.Unlock()

	// Проверяем, существует ли пользователь
	user, err := h.svc.GetUserByTelegramID(context.Background(), userID)
	if err != nil {
		adminSearch.mu.Lock()
		delete(adminSearch.addBalTo, c.Sender().ID)
		adminSearch.mu.Unlock()

		menu := &tele.ReplyMarkup{}
		menu.Inline(
			menu.Row(menu.Data("🔎 Найти юзера", "admin_find_user")),
			menu.Row(menu.Data("⬅️ Назад", "admin_back")),
		)
		return c.Send(fmt.Sprintf("❌ Пользователь с ID `%d` не найден.", userID), menu, tele.ModeMarkdown)
	}

	text := fmt.Sprintf(`💳 *Пополнение баланса*

👤 Пользователь: `+"`%d`"+`
💰 Текущий баланс: *%.0f ₽*

👇 Введите сумму пополнения (в рублях):`, user.TelegramID, user.Balance)

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(
			menu.Data("100 ₽", "admin_addbal_amount", fmt.Sprintf("%d:100", userID)),
			menu.Data("450 ₽", "admin_addbal_amount", fmt.Sprintf("%d:450", userID)),
			menu.Data("1000 ₽", "admin_addbal_amount", fmt.Sprintf("%d:1000", userID)),
		),
		menu.Row(menu.Data("❌ Отмена", "admin_back")),
	)

	if c.Callback() != nil {
		return c.Edit(text, menu, tele.ModeMarkdown)
	}
	return c.Send(text, menu, tele.ModeMarkdown)
}

// HandleAdminAddBalAmount обрабатывает ввод суммы
func (h *Handler) HandleAdminAddBalAmount(c tele.Context, targetUserID int64) error {
	adminSearch.mu.Lock()
	delete(adminSearch.addBalTo, c.Sender().ID)
	adminSearch.mu.Unlock()

	amount, err := strconv.ParseFloat(strings.TrimSpace(c.Text()), 64)
	if err != nil || amount <= 0 {
		menu := &tele.ReplyMarkup{}
		menu.Inline(
			menu.Row(menu.Data("💳 Попробовать снова", "admin_addbal_start")),
			menu.Row(menu.Data("⬅️ Назад", "admin_back")),
		)
		return c.Send("❌ Некорректная сумма. Введите положительное число.", menu)
	}

	return h.addBalanceToUser(c, targetUserID, amount)
}

// addBalanceToUser добавляет баланс пользователю
func (h *Handler) addBalanceToUser(c tele.Context, telegramID int64, amount float64) error {
	if err := h.svc.AddUserBalance(context.Background(), telegramID, amount); err != nil {
		return c.Send(fmt.Sprintf("❌ Ошибка: %v", err))
	}

	text := fmt.Sprintf("✅ *Баланс пополнен!*\n\n👤 Пользователь: `%d`\n💰 Сумма: *+%.0f ₽*", telegramID, amount)

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("💳 Пополнить ещё", "admin_addbal_start")),
		menu.Row(menu.Data("🔎 Найти юзера", "admin_find_user")),
		menu.Row(menu.Data("⬅️ В админ-панель", "admin_back")),
	)

	return c.Send(text, menu, tele.ModeMarkdown)
}

// ================= QUICK FLASH SALE =================

// HandleFlashQuick запускает быструю флеш-распродажу
func (h *Handler) HandleFlashQuick(c tele.Context) error {
	parts := strings.Split(c.Callback().Data, ":")
	if len(parts) != 2 {
		return c.Send("❌ Ошибка")
	}

	percent, _ := strconv.Atoi(parts[0])
	hours, _ := strconv.Atoi(parts[1])

	if percent <= 0 || hours <= 0 {
		return c.Send("❌ Неверные параметры")
	}

	// Устанавливаем скидку
	flashSale.Set(percent, hours)
	endTime := flashSale.GetEndTime()

	log.Printf("[FLASH SALE] Admin %d started quick %d%% sale for %d hours", c.Sender().ID, percent, hours)

	c.Edit(fmt.Sprintf("✅ *Флеш-распродажа запущена!*\n\n🔥 Скидка: *%d%%*\n⏰ До: *%s*\n\n📤 Запускаю рассылку...",
		percent, endTime.Format("02.01 15:04")), tele.ModeMarkdown)

	// Запускаем рассылку в горутине
	go h.broadcastFlashSale(c.Bot(), c.Sender().ID, percent, hours, endTime)

	return nil
}

// HandleAdminAddBalUser пополняет баланс конкретного пользователя (из профиля)
func (h *Handler) HandleAdminAddBalUser(c tele.Context) error {
	userID, err := strconv.ParseInt(c.Callback().Data, 10, 64)
	if err != nil {
		return c.Send("❌ Ошибка")
	}
	return h.promptAddBalAmount(c, userID)
}

// HandleAdminAddBalAmountCallback обрабатывает нажатие кнопки с суммой
func (h *Handler) HandleAdminAddBalAmountCallback(c tele.Context) error {
	parts := strings.Split(c.Callback().Data, ":")
	if len(parts) != 2 {
		return c.Send("❌ Ошибка")
	}

	userID, _ := strconv.ParseInt(parts[0], 10, 64)
	amount, _ := strconv.ParseFloat(parts[1], 64)

	if userID <= 0 || amount <= 0 {
		return c.Send("❌ Неверные параметры")
	}

	return h.addBalanceToUser(c, userID, amount)
}

// HandleAdminGiftUser начинает выдачу ключа конкретному пользователю
func (h *Handler) HandleAdminGiftUser(c tele.Context) error {
	userID, err := strconv.ParseInt(c.Callback().Data, 10, 64)
	if err != nil {
		return c.Send("❌ Ошибка")
	}

	// Получаем продукты
	products, err := h.svc.GetAllProducts(context.Background())
	if err != nil {
		return c.Send("❌ Ошибка загрузки продуктов")
	}

	text := fmt.Sprintf("🎁 *Подарить ключ*\n\n👤 Пользователь: `%d`\n\nВыберите тариф:", userID)

	menu := &tele.ReplyMarkup{}
	var rows []tele.Row

	for _, p := range products {
		btnText := fmt.Sprintf("%s %s", p.CountryFlag, p.Name)
		btn := menu.Data(btnText, "admin_gift_product", fmt.Sprintf("%d:%d", userID, p.ID))
		rows = append(rows, menu.Row(btn))
	}

	rows = append(rows, menu.Row(menu.Data("❌ Отмена", "admin_back")))
	menu.Inline(rows...)

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// HandleAdminGiftProduct обрабатывает выбор продукта для подарка
func (h *Handler) HandleAdminGiftProduct(c tele.Context) error {
	parts := strings.Split(c.Callback().Data, ":")
	if len(parts) != 2 {
		return c.Send("❌ Ошибка")
	}

	userID, _ := strconv.ParseInt(parts[0], 10, 64)
	productID, _ := strconv.ParseInt(parts[1], 10, 64)

	product, err := h.svc.GetProductByID(context.Background(), productID)
	if err != nil {
		return c.Send("❌ Продукт не найден")
	}

	text := fmt.Sprintf("🎁 *Подарить ключ*\n\n👤 Пользователь: `%d`\n📦 Тариф: %s %s\n\nВыберите срок:",
		userID, product.CountryFlag, product.Name)

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(
			menu.Data("7 дней", "admin_gift_days", fmt.Sprintf("%d:%d:7", userID, productID)),
			menu.Data("14 дней", "admin_gift_days", fmt.Sprintf("%d:%d:14", userID, productID)),
		),
		menu.Row(
			menu.Data("30 дней", "admin_gift_days", fmt.Sprintf("%d:%d:30", userID, productID)),
			menu.Data("90 дней", "admin_gift_days", fmt.Sprintf("%d:%d:90", userID, productID)),
		),
		menu.Row(menu.Data("❌ Отмена", "admin_back")),
	)

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// HandleAdminGiftDays завершает выдачу подарочного ключа
func (h *Handler) HandleAdminGiftDays(c tele.Context) error {
	parts := strings.Split(c.Callback().Data, ":")
	if len(parts) != 3 {
		return c.Send("❌ Ошибка")
	}

	userID, _ := strconv.ParseInt(parts[0], 10, 64)
	productID, _ := strconv.ParseInt(parts[1], 10, 64)
	days, _ := strconv.Atoi(parts[2])

	sub, err := h.svc.GiftSubscription(context.Background(), userID, productID, days)
	if err != nil {
		return c.Send(fmt.Sprintf("❌ Ошибка: %v", err))
	}

	// Уведомляем админа
	text := fmt.Sprintf("✅ *Ключ выдан!*\n\n👤 Пользователь: `%d`\n📦 %s %s\n📅 Срок: %d дней\n📆 До: %s\n\n🔑 Ключ:\n`%s`",
		userID, sub.Product.CountryFlag, sub.Product.Name, days,
		sub.ExpiresAt.Format("02.01.2006"), sub.KeyString)

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("🎁 Выдать ещё", "admin_issue")),
		menu.Row(menu.Data("⬅️ В админ-панель", "admin_back")),
	)

	c.Edit(text, menu, tele.ModeMarkdown)

	// Уведомляем пользователя
	userMsg := fmt.Sprintf(`🎁 *Вам подарена подписка!*

%s %s
📅 Действует до: %s

🔑 Ваш ключ:
`+"`%s`"+`

Используйте /mysubs для просмотра подписок.`,
		sub.Product.CountryFlag, sub.Product.Name,
		sub.ExpiresAt.Format("02.01.2006"),
		sub.KeyString)

	_, err = c.Bot().Send(&tele.User{ID: userID}, userMsg, tele.ModeMarkdown)
	if err != nil {
		log.Printf("Failed to notify user %d about gift: %v", userID, err)
	}

	return nil
}

// HandleFindUser ищет пользователя
func (h *Handler) HandleFindUser(c tele.Context) error {
	args := c.Args()
	if len(args) == 0 {
		return c.Send("❌ Использование: /find <telegram_id или username>")
	}

	query := args[0]
	profile, err := h.svc.FindUser(context.Background(), query)
	if err != nil {
		return c.Send(fmt.Sprintf("❌ Пользователь не найден: %v", err))
	}

	// Format user info
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("👤 *Пользователь #%d*\n\n", profile.User.ID))
	sb.WriteString(fmt.Sprintf("🆔 Telegram ID: `%d`\n", profile.User.TelegramID))
	sb.WriteString(fmt.Sprintf("📝 Username: @%s\n", profile.User.Username))
	sb.WriteString(fmt.Sprintf("💰 Баланс: %.2f ₽\n", profile.User.Balance))
	sb.WriteString(fmt.Sprintf("📅 Регистрация: %s\n\n", profile.User.CreatedAt.Format("02.01.2006")))

	// Subscriptions
	sb.WriteString(fmt.Sprintf("🔑 *Подписки (%d):*\n", len(profile.Subscriptions)))
	for _, sub := range profile.Subscriptions {
		status := "✅"
		if !sub.IsActive || sub.ExpiresAt.Before(time.Now()) {
			status = "❌"
		}
		sb.WriteString(fmt.Sprintf("• %s %s до %s %s\n",
			sub.Product.CountryFlag, sub.Product.Name,
			sub.ExpiresAt.Format("02.01.06"), status))
	}

	// Recent transactions
	if len(profile.Transactions) > 0 {
		sb.WriteString("\n💳 *Последние транзакции:*\n")
		for _, tx := range profile.Transactions[:min(5, len(profile.Transactions))] {
			sb.WriteString(fmt.Sprintf("• %.2f ₽ (%s) — %s\n",
				tx.Amount, tx.Type, tx.CreatedAt.Format("02.01.06")))
		}
	}

	return c.Send(sb.String(), tele.ModeMarkdown)
}

// HandleAddBalance пополняет баланс пользователя
func (h *Handler) HandleAddBalance(c tele.Context) error {
	args := c.Args()
	if len(args) < 2 {
		return c.Send("❌ Использование: /addbal <telegram_id> <сумма>")
	}

	telegramID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return c.Send("❌ Неверный telegram_id")
	}

	amount, err := strconv.ParseFloat(args[1], 64)
	if err != nil {
		return c.Send("❌ Неверная сумма")
	}

	if err := h.svc.AddUserBalance(context.Background(), telegramID, amount); err != nil {
		return c.Send(fmt.Sprintf("❌ Ошибка: %v", err))
	}

	return c.Send(fmt.Sprintf("✅ Баланс пользователя %d пополнен на %.2f ₽", telegramID, amount))
}

// HandleGiftSub дарит подписку пользователю
func (h *Handler) HandleGiftSub(c tele.Context) error {
	args := c.Args()
	if len(args) < 3 {
		return c.Send("❌ Использование: /gift <telegram_id> <product_id> <дней>\n\nПример: /gift 123456789 1 30")
	}

	telegramID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return c.Send("❌ Неверный telegram_id")
	}

	productID, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		return c.Send("❌ Неверный product_id")
	}

	days, err := strconv.Atoi(args[2])
	if err != nil {
		return c.Send("❌ Неверное количество дней")
	}

	sub, err := h.svc.GiftSubscription(context.Background(), telegramID, productID, days)
	if err != nil {
		return c.Send(fmt.Sprintf("❌ Ошибка: %v", err))
	}

	// Notify admin
	c.Send(fmt.Sprintf("✅ Подписка создана!\n\n🔑 %s %s\n📅 Действует до: %s\n\nКлюч:\n`%s`",
		sub.Product.CountryFlag, sub.Product.Name,
		sub.ExpiresAt.Format("02.01.2006"),
		sub.KeyString), tele.ModeMarkdown)

	// Notify user
	userMsg := fmt.Sprintf(`🎁 *Вам подарена подписка!*

%s %s
📅 Действует до: %s

🔑 Ваш ключ:
`+"`%s`"+`

Используйте /mysubs для просмотра подписок.`,
		sub.Product.CountryFlag, sub.Product.Name,
		sub.ExpiresAt.Format("02.01.2006"),
		sub.KeyString)

	_, err = c.Bot().Send(&tele.User{ID: telegramID}, userMsg, tele.ModeMarkdown)
	if err != nil {
		log.Printf("Failed to notify user %d about gift: %v", telegramID, err)
	}

	return nil
}

// ================= BROADCAST =================

// HandleAdminBroadcast начинает рассылку
func (h *Handler) HandleAdminBroadcast(c tele.Context) error {
	broadcast.mu.Lock()
	if broadcast.isActive {
		broadcast.mu.Unlock()
		return c.Send("❌ Рассылка уже выполняется. Дождитесь завершения.")
	}
	broadcast.waitingMsg = true
	broadcast.waitingConfirm = false
	broadcast.adminID = c.Sender().ID
	broadcast.message = nil
	broadcast.mu.Unlock()

	text := `📢 *Рассылка*

Отправьте сообщение (текст, фото или перешлите пост из канала), которое будет разослано всем пользователям.

⚠️ Для отмены нажмите кнопку ниже.`

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("❌ Отменить", "admin_cancel_broadcast")),
	)

	if c.Callback() != nil {
		return c.Edit(text, menu, tele.ModeMarkdown)
	}
	return c.Send(text, menu, tele.ModeMarkdown)
}

// HandleCancelBroadcast отменяет рассылку
func (h *Handler) HandleCancelBroadcast(c tele.Context) error {
	broadcast.mu.Lock()
	broadcast.waitingMsg = false
	broadcast.waitingConfirm = false
	broadcast.adminID = 0
	broadcast.message = nil
	broadcast.mu.Unlock()

	if c.Callback() != nil {
		return h.HandleAdmin(c)
	}
	return c.Send("❌ Рассылка отменена.")
}

// HandleBroadcastMessage обрабатывает сообщение для рассылки (запрос подтверждения)
func (h *Handler) HandleBroadcastMessage(c tele.Context) error {
	broadcast.mu.Lock()
	if !broadcast.waitingMsg || broadcast.adminID != c.Sender().ID {
		broadcast.mu.Unlock()
		return nil
	}
	broadcast.waitingMsg = false
	broadcast.waitingConfirm = true
	broadcast.message = c.Message()
	broadcast.mu.Unlock()

	// Get user count
	userIDs, err := h.svc.GetAllUserTelegramIDs(context.Background())
	if err != nil {
		broadcast.mu.Lock()
		broadcast.waitingConfirm = false
		broadcast.mu.Unlock()
		return c.Send(fmt.Sprintf("❌ Ошибка получения списка пользователей: %v", err))
	}

	totalUsers := len(userIDs)

	text := fmt.Sprintf(`📢 *Подтверждение рассылки*

Сообщение будет отправлено *%d* пользователям.

Отправить?`, totalUsers)

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(
			menu.Data("✅ Да, отправить", "admin_confirm_broadcast"),
			menu.Data("❌ Отмена", "admin_cancel_broadcast"),
		),
	)

	return c.Send(text, menu, tele.ModeMarkdown)
}

// HandleConfirmBroadcast подтверждает и запускает рассылку
func (h *Handler) HandleConfirmBroadcast(c tele.Context) error {
	broadcast.mu.Lock()
	if !broadcast.waitingConfirm || broadcast.adminID != c.Sender().ID || broadcast.message == nil {
		broadcast.mu.Unlock()
		return c.Send("❌ Нет сообщения для рассылки.")
	}
	broadcast.waitingConfirm = false
	broadcast.isActive = true
	msg := broadcast.message
	broadcast.mu.Unlock()

	// Get all user IDs
	userIDs, err := h.svc.GetAllUserTelegramIDs(context.Background())
	if err != nil {
		broadcast.mu.Lock()
		broadcast.isActive = false
		broadcast.mu.Unlock()
		return c.Send(fmt.Sprintf("❌ Ошибка получения списка пользователей: %v", err))
	}

	totalUsers := len(userIDs)

	log.Printf("[BROADCAST] Admin %d started broadcast to %d users", c.Sender().ID, totalUsers)

	c.Edit(fmt.Sprintf("📤 *Рассылка запущена!*\n\nОтправляю сообщение %d пользователям...", totalUsers), tele.ModeMarkdown)

	// Run broadcast in goroutine
	go func() {
		bot := c.Bot()
		adminID := c.Sender().ID

		var sent, failed int
		ticker := time.NewTicker(50 * time.Millisecond) // 20 messages per second
		defer ticker.Stop()

		for _, userID := range userIDs {
			<-ticker.C

			var err error
			if msg.Photo != nil {
				// Send photo with caption
				photo := &tele.Photo{
					File:    msg.Photo.File,
					Caption: msg.Caption,
				}
				_, err = bot.Send(&tele.User{ID: userID}, photo, tele.ModeMarkdown)
			} else if msg.Document != nil {
				// Send document
				doc := &tele.Document{
					File:    msg.Document.File,
					Caption: msg.Caption,
				}
				_, err = bot.Send(&tele.User{ID: userID}, doc, tele.ModeMarkdown)
			} else if msg.Video != nil {
				// Send video
				video := &tele.Video{
					File:    msg.Video.File,
					Caption: msg.Caption,
				}
				_, err = bot.Send(&tele.User{ID: userID}, video, tele.ModeMarkdown)
			} else {
				// Send text
				_, err = bot.Send(&tele.User{ID: userID}, msg.Text, tele.ModeMarkdown)
			}

			if err != nil {
				failed++
				log.Printf("[BROADCAST] Failed for user %d: %v", userID, err)
			} else {
				sent++
			}

			// Progress update every 100 users
			if (sent+failed)%100 == 0 && totalUsers > 100 {
				bot.Send(&tele.User{ID: adminID},
					fmt.Sprintf("📤 Прогресс: %d/%d", sent+failed, totalUsers))
			}
		}

		// Final report
		broadcast.mu.Lock()
		broadcast.isActive = false
		broadcast.message = nil
		broadcast.mu.Unlock()

		log.Printf("[BROADCAST] Finished. Sent: %d, Failed: %d", sent, failed)

		bot.Send(&tele.User{ID: adminID},
			fmt.Sprintf("✅ *Рассылка завершена!*\n\n📤 Отправлено: %d\n❌ Ошибок: %d\n📊 Всего: %d",
				sent, failed, totalUsers), tele.ModeMarkdown)
	}()

	return nil
}

// min возвращает минимальное из двух чисел
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ================= ISSUE KEY =================

// HandleIssueStart начинает процесс выдачи ключа
func (h *Handler) HandleIssueStart(c tele.Context) error {
	// Clear any existing session
	issue.mu.Lock()
	issue.sessions[c.Sender().ID] = &issueSession{step: 1}
	issue.mu.Unlock()

	// Get products
	products, err := h.svc.GetAllProducts(context.Background())
	if err != nil {
		return c.Send("❌ Ошибка загрузки продуктов")
	}

	text := "🔑 *Выдача ключа*\n\nВыберите локацию/продукт:"

	menu := &tele.ReplyMarkup{}
	var rows []tele.Row

	for _, p := range products {
		btnText := fmt.Sprintf("%s %s", p.CountryFlag, p.Name)
		btn := menu.Data(btnText, "issue_product", strconv.FormatInt(p.ID, 10))
		rows = append(rows, menu.Row(btn))
	}

	rows = append(rows, menu.Row(menu.Data("❌ Отмена", "issue_cancel")))
	menu.Inline(rows...)

	if c.Callback() != nil {
		return c.Edit(text, menu, tele.ModeMarkdown)
	}
	return c.Send(text, menu, tele.ModeMarkdown)
}

// HandleIssueProduct обрабатывает выбор продукта
func (h *Handler) HandleIssueProduct(c tele.Context) error {
	productID, err := strconv.ParseInt(c.Callback().Data, 10, 64)
	if err != nil {
		return c.Send("❌ Ошибка")
	}

	issue.mu.Lock()
	session, exists := issue.sessions[c.Sender().ID]
	if !exists {
		issue.mu.Unlock()
		return h.HandleIssueStart(c)
	}
	session.productID = productID
	session.step = 2
	issue.mu.Unlock()

	product, err := h.svc.GetProductByID(context.Background(), productID)
	if err != nil {
		return c.Send("❌ Продукт не найден")
	}

	text := fmt.Sprintf("🔑 *Выдача ключа*\n\n%s %s\n\nВыберите срок действия:",
		product.CountryFlag, product.Name)

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(
			menu.Data("30 дней", "issue_days", "30"),
			menu.Data("90 дней", "issue_days", "90"),
		),
		menu.Row(
			menu.Data("180 дней", "issue_days", "180"),
			menu.Data("365 дней", "issue_days", "365"),
		),
		menu.Row(menu.Data("⬅️ Назад", "admin_issue")),
	)

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// HandleIssueDays обрабатывает выбор срока
func (h *Handler) HandleIssueDays(c tele.Context) error {
	days, err := strconv.Atoi(c.Callback().Data)
	if err != nil {
		return c.Send("❌ Ошибка")
	}

	issue.mu.Lock()
	session, exists := issue.sessions[c.Sender().ID]
	if !exists {
		issue.mu.Unlock()
		return h.HandleIssueStart(c)
	}
	session.days = days
	session.step = 3
	issue.mu.Unlock()

	product, _ := h.svc.GetProductByID(context.Background(), session.productID)

	text := fmt.Sprintf(`🔑 *Выдача ключа*

%s %s
📅 Срок: %d дней

Введите Telegram ID пользователя или нажмите кнопку ниже для создания ключа без привязки:`,
		product.CountryFlag, product.Name, days)

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("🔓 Создать без привязки к пользователю", "issue_no_user")),
		menu.Row(menu.Data("❌ Отмена", "issue_cancel")),
	)

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// HandleIssueUserID обрабатывает ввод user ID
func (h *Handler) HandleIssueUserID(c tele.Context) error {
	issue.mu.Lock()
	session, exists := issue.sessions[c.Sender().ID]
	if !exists || session.step != 3 {
		issue.mu.Unlock()
		return nil
	}
	productID := session.productID
	days := session.days
	delete(issue.sessions, c.Sender().ID)
	issue.mu.Unlock()

	telegramID, err := strconv.ParseInt(strings.TrimSpace(c.Text()), 10, 64)
	if err != nil {
		return c.Send("❌ Неверный Telegram ID. Введите число или используйте кнопку.")
	}

	// Create subscription for user
	sub, err := h.svc.GiftSubscription(context.Background(), telegramID, productID, days)
	if err != nil {
		return c.Send(fmt.Sprintf("❌ Ошибка создания ключа: %v", err))
	}

	// Notify admin
	c.Send(fmt.Sprintf("✅ *Ключ создан и отправлен пользователю!*\n\n%s %s\n📅 Срок: %d дней\n👤 User ID: `%d`\n\n🔑 Ключ:\n`%s`",
		sub.Product.CountryFlag, sub.Product.Name, days, telegramID, sub.KeyString), tele.ModeMarkdown)

	// Notify user
	userMsg := fmt.Sprintf(`🎁 *Вам выдан VPN ключ!*

%s %s
📅 Действует до: %s

🔑 Ваш ключ:
`+"`%s`"+`

Используйте /mysubs для просмотра подписок.
Инструкция по настройке: /help → 📚 Инструкция`,
		sub.Product.CountryFlag, sub.Product.Name,
		sub.ExpiresAt.Format("02.01.2006"),
		sub.KeyString)

	_, err = c.Bot().Send(&tele.User{ID: telegramID}, userMsg, tele.ModeMarkdown)
	if err != nil {
		c.Send(fmt.Sprintf("⚠️ Не удалось отправить ключ пользователю: %v", err))
	}

	return nil
}

// HandleIssueNoUser создаёт ключ без привязки к пользователю
func (h *Handler) HandleIssueNoUser(c tele.Context) error {
	issue.mu.Lock()
	session, exists := issue.sessions[c.Sender().ID]
	if !exists || session.step != 3 {
		issue.mu.Unlock()
		return h.HandleIssueStart(c)
	}
	productID := session.productID
	days := session.days
	delete(issue.sessions, c.Sender().ID)
	issue.mu.Unlock()

	// Create key for admin (system key)
	sub, err := h.svc.GiftSubscription(context.Background(), c.Sender().ID, productID, days)
	if err != nil {
		return c.Send(fmt.Sprintf("❌ Ошибка создания ключа: %v", err))
	}

	text := fmt.Sprintf(`✅ *Ключ создан!*

%s %s
📅 Срок: %d дней
📅 Действует до: %s

🔑 *Скопируйте ключ:*
`+"`%s`"+`

Этот ключ не привязан к пользователю. Вы можете передать его вручную.`,
		sub.Product.CountryFlag, sub.Product.Name,
		days, sub.ExpiresAt.Format("02.01.2006"),
		sub.KeyString)

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("🔑 Выдать ещё", "admin_issue")),
		menu.Row(menu.Data("⬅️ В админ-панель", "admin_back")),
	)

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// HandleIssueCancel отменяет выдачу ключа
func (h *Handler) HandleIssueCancel(c tele.Context) error {
	issue.mu.Lock()
	delete(issue.sessions, c.Sender().ID)
	issue.mu.Unlock()

	return h.HandleAdmin(c)
}

// ================= ADMIN NOTIFICATIONS =================

// NotifyAdminSale отправляет уведомление админу о новой продаже
func (h *Handler) NotifyAdminSale(bot *tele.Bot, username string, userID int64, productFlag, productName string, months int, amount float64) {
	if len(h.adminIDs) == 0 {
		return
	}

	text := fmt.Sprintf(`💰 *Новая продажа!*

👤 Пользователь: @%s (ID: `+"`%d`"+`)
📦 Тариф: %s %s (%d мес.)
💵 Сумма: %.0f ₽`,
		username, userID, productFlag, productName, months, amount)

	for _, adminID := range h.adminIDs {
		_, err := bot.Send(&tele.User{ID: adminID}, text, tele.ModeMarkdown)
		if err != nil {
			log.Printf("Failed to notify admin %d about sale: %v", adminID, err)
		}
	}
}

// NotifyAdminNewUser отправляет уведомление админу о новом пользователе
func (h *Handler) NotifyAdminNewUser(bot *tele.Bot, username string, userID int64) {
	if len(h.adminIDs) == 0 {
		return
	}

	text := fmt.Sprintf(`👤 *Новый пользователь!*

Username: @%s
ID: `+"`%d`", username, userID)

	for _, adminID := range h.adminIDs {
		_, err := bot.Send(&tele.User{ID: adminID}, text, tele.ModeMarkdown)
		if err != nil {
			log.Printf("Failed to notify admin %d about new user: %v", adminID, err)
		}
	}
}

// ================= ADMIN HELP =================

// HandleAdminHelp показывает справку для админа
func (h *Handler) HandleAdminHelp(c tele.Context) error {
	text := `📜 *Список команд*

━━━━━━━━━━━━━━━━━━━━
*🔧 Основные:*
/admin — панель администратора
/stats — статистика
/ahelp — эта справка

*👥 Пользователи:*
/find <ID> — найти пользователя
/addbal <ID> <сумма> — пополнить баланс

*🔑 Ключи:*
/issue — интерактивная выдача
/gift <ID> <product> <дней> — быстрая выдача

*📢 Маркетинг:*
/broadcast — начать рассылку
/flashsale — запустить акцию
/flashsale <%%> <часов> — быстрый запуск
/stopsale — остановить акцию
━━━━━━━━━━━━━━━━━━━━

*💡 Примеры:*
` + "`/find 123456789`" + `
` + "`/addbal 123456789 500`" + `
` + "`/gift 123456789 1 30`" + `
` + "`/flashsale 50 6`"

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("🔙 Назад", "admin_back")),
	)

	if c.Callback() != nil {
		return c.Edit(text, menu, tele.ModeMarkdown)
	}
	return c.Send(text, menu, tele.ModeMarkdown)
}

// ================= SUPPORT TICKET SYSTEM =================

// HandleSupportUserMessage пересылает сообщение пользователя в группу поддержки
func (h *Handler) HandleSupportUserMessage(c tele.Context) error {
	userID := c.Sender().ID
	username := c.Sender().Username

	// Получаем баланс пользователя
	ctx := context.Background()
	user, _ := h.svc.GetOrCreateUser(ctx, userID, username)
	balance := float64(0)
	if user != nil {
		balance = user.Balance
	}

	// Форматируем username
	usernameStr := "нет"
	if username != "" {
		usernameStr = "@" + username
	}

	supportGroup := &tele.Chat{ID: h.supportGroupID}

	// Формируем заголовок с #user_ тегом (КРИТИЧНО для ответа!)
	header := fmt.Sprintf("🎫 #user_%d\n👤 %s | 💰 %.0f ₽\n━━━━━━━━━━━━━━━━━━━━\n", userID, usernameStr, balance)

	// Кнопка "Закрыть тикет" для админа (с userID в payload)
	adminMenu := &tele.ReplyMarkup{}
	adminMenu.Inline(
		adminMenu.Row(adminMenu.Data("🔒 Закрыть тикет", "admin_close_ticket", strconv.FormatInt(userID, 10))),
	)

	// 1. Отправляем сообщение в группу с тегом в тексте и кнопкой
	if c.Message().Photo != nil {
		// Фото: добавляем тег в caption
		photo := c.Message().Photo
		caption := header
		if c.Message().Caption != "" {
			caption += c.Message().Caption
		} else {
			caption += "[Фото без подписи]"
		}
		photo.Caption = caption
		_, err := c.Bot().Send(supportGroup, photo, adminMenu)
		if err != nil {
			log.Printf("Failed to send support photo: %v", err)
			return c.Send("❌ Ошибка отправки. Попробуйте позже.")
		}
	} else if c.Message().Document != nil {
		// Документ
		doc := c.Message().Document
		caption := header
		if c.Message().Caption != "" {
			caption += c.Message().Caption
		} else {
			caption += "[Документ]"
		}
		doc.Caption = caption
		_, err := c.Bot().Send(supportGroup, doc, adminMenu)
		if err != nil {
			log.Printf("Failed to send support document: %v", err)
			return c.Send("❌ Ошибка отправки. Попробуйте позже.")
		}
	} else if c.Message().Voice != nil {
		// Голосовое: сначала отправляем текст с тегом, потом голосовое
		_, err := c.Bot().Send(supportGroup, header+"[Голосовое сообщение ниже]", adminMenu)
		if err == nil {
			c.Bot().Send(supportGroup, c.Message().Voice)
		}
	} else {
		// Текст: добавляем тег в начало
		text := header + c.Message().Text
		_, err := c.Bot().Send(supportGroup, text, adminMenu)
		if err != nil {
			log.Printf("Failed to send support text: %v", err)
			return c.Send("❌ Ошибка отправки. Попробуйте позже.")
		}
	}

	log.Printf("🎫 Support ticket sent to group from user %d", userID)

	// 2. НЕ сбрасываем режим — пользователь может отправить ещё сообщения (фото, уточнения)
	// SetUserSupportMode(userID, false) — убрано для seamless mode

	// 3. Обновляем трекер и dashboard
	if tracker := GetTracker(); tracker != nil {
		tracker.AddOrUpdateTicket(userID, username, 0) // groupMsgID можно добавить если сохранять
		go tracker.UpdateDashboard()
	}

	// 4. Компактное подтверждение — просто reply на сообщение пользователя
	return c.Reply("✅ Отправлено. Ожидайте ответа.", tele.ModeMarkdown)
}

// HandleSupportReplyStart начинает ответ на тикет
func (h *Handler) HandleSupportReplyStart(c tele.Context) error {
	userID, err := strconv.ParseInt(c.Callback().Data, 10, 64)
	if err != nil {
		return c.Send("❌ Ошибка")
	}

	// Устанавливаем режим ответа для админа
	SetAdminReplyTarget(c.Sender().ID, userID)

	text := fmt.Sprintf(`✍️ *Ответ на тикет*

👤 Пользователь: `+"`%d`"+`

Введите текст ответа:

_(Отправьте /cancel чтобы отменить)_`, userID)

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("❌ Отмена", "support_cancel_reply")),
	)

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// HandleSupportAdminReply отправляет ответ админа пользователю
func (h *Handler) HandleSupportAdminReply(c tele.Context, targetUserID int64) error {
	// Сбрасываем режим ответа
	SetAdminReplyTarget(c.Sender().ID, 0)

	// Формируем ответ для пользователя
	replyText := fmt.Sprintf("👨‍💻 *Поддержка:*\n\n%s", c.Message().Text)

	// Отправляем пользователю
	_, err := c.Bot().Send(&tele.User{ID: targetUserID}, replyText, tele.ModeMarkdown)
	if err != nil {
		return c.Send(fmt.Sprintf("❌ Не удалось отправить ответ: %v", err))
	}

	// Подтверждение админу
	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("↩️ Ответить ещё", "support_reply", strconv.FormatInt(targetUserID, 10))),
		menu.Row(menu.Data("🔙 В админ-панель", "admin_back")),
	)

	return c.Send(fmt.Sprintf("✅ Ответ отправлен пользователю `%d`", targetUserID), menu, tele.ModeMarkdown)
}

// HandleStopSupport выводит пользователя из режима поддержки
func (h *Handler) HandleStopSupport(c tele.Context) error {
	userID := c.Sender().ID

	if !IsUserInSupportMode(userID) {
		return c.Send("ℹ️ Вы не находитесь в режиме поддержки.")
	}

	SetUserSupportMode(userID, false)

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("🏠 Главное меню", "back_main")),
	)

	return c.Send("✅ Вы вышли из режима поддержки.\n\nТеперь вы можете использовать меню бота.", menu)
}

// HandleSupportCancelReply отменяет режим ответа на тикет
func (h *Handler) HandleSupportCancelReply(c tele.Context) error {
	SetAdminReplyTarget(c.Sender().ID, 0)
	return h.HandleAdmin(c)
}

// ================= PROMO CODE MANAGEMENT =================

// HandleAdminPromo показывает меню управления промокодами
func (h *Handler) HandleAdminPromo(c tele.Context) error {
	text := `🎟 *Управление промокодами*

Выберите действие:`

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("➕ Создать код", "admin_promo_create")),
		menu.Row(menu.Data("📋 Список активных", "admin_promo_list")),
		menu.Row(menu.Data("📊 Статистика", "admin_promo_stats")),
		menu.Row(menu.Data("🗑 Удалить код", "admin_promo_delete")),
		menu.Row(menu.Data("⬅️ Назад", "admin_back")),
	)

	if c.Callback() != nil {
		return c.Edit(text, menu, tele.ModeMarkdown)
	}
	return c.Send(text, menu, tele.ModeMarkdown)
}

// HandleAdminPromoCreate начинает создание промокода
func (h *Handler) HandleAdminPromoCreate(c tele.Context) error {
	promoWizard.mu.Lock()
	promoWizard.sessions[c.Sender().ID] = &promoWizardSession{step: 1}
	promoWizard.mu.Unlock()

	text := `➕ *Создание промокода*

*Шаг 1/3:* Введите название кода

_Например: SALE50, START2025, VIP100_`

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("❌ Отмена", "admin_promo_cancel")),
	)

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// HandleAdminPromoWizardInput обрабатывает ввод в визарде создания промокода
func (h *Handler) HandleAdminPromoWizardInput(c tele.Context, session *promoWizardSession) error {
	input := strings.TrimSpace(c.Text())

	switch session.step {
	case 1: // Ввод кода
		if len(input) < 3 || len(input) > 20 {
			return c.Send("❌ Код должен быть от 3 до 20 символов. Попробуйте снова:")
		}
		// Проверяем, не существует ли уже
		existing, _ := h.svc.GetPromoByCode(context.Background(), input)
		if existing != nil {
			return c.Send("❌ Такой промокод уже существует. Введите другой:")
		}

		promoWizard.mu.Lock()
		session.code = strings.ToUpper(input)
		session.step = 2
		promoWizard.mu.Unlock()

		text := fmt.Sprintf(`➕ *Создание промокода*

📝 Код: ` + "`%s`" + `

*Шаг 2/3:* Введите сумму бонуса (в рублях)

_Например: 100_`, session.code)

		menu := &tele.ReplyMarkup{}
		menu.Inline(
			menu.Row(menu.Data("❌ Отмена", "admin_promo_cancel")),
		)
		return c.Send(text, menu, tele.ModeMarkdown)

	case 2: // Ввод суммы
		amount, err := strconv.ParseFloat(input, 64)
		if err != nil || amount <= 0 {
			return c.Send("❌ Некорректная сумма. Введите положительное число:")
		}

		promoWizard.mu.Lock()
		session.amount = amount
		session.step = 3
		promoWizard.mu.Unlock()

		text := fmt.Sprintf(`➕ *Создание промокода*

📝 Код: `+"`%s`"+`
💰 Сумма: *%.0f ₽*

*Шаг 3/3:* Введите количество активаций

_Например: 50_`, session.code, session.amount)

		menu := &tele.ReplyMarkup{}
		menu.Inline(
			menu.Row(menu.Data("❌ Отмена", "admin_promo_cancel")),
		)
		return c.Send(text, menu, tele.ModeMarkdown)

	case 3: // Ввод количества активаций
		activations, err := strconv.Atoi(input)
		if err != nil || activations <= 0 {
			return c.Send("❌ Некорректное количество. Введите положительное число:")
		}

		// Создаём промокод
		promo, err := h.svc.CreatePromoCode(context.Background(), session.code, session.amount, activations)

		// Очищаем сессию
		promoWizard.mu.Lock()
		delete(promoWizard.sessions, c.Sender().ID)
		promoWizard.mu.Unlock()

		if err != nil {
			return c.Send(fmt.Sprintf("❌ Ошибка создания: %v", err))
		}

		text := fmt.Sprintf(`✅ *Промокод создан!*

📝 Код: `+"`%s`"+`
💰 Сумма: *%.0f ₽*
🔢 Активаций: *%d*

Пользователи могут активировать его через кнопку "🎟 Промокод" в главном меню.`,
			promo.Code, promo.Amount, promo.MaxActivations)

		menu := &tele.ReplyMarkup{}
		menu.Inline(
			menu.Row(menu.Data("➕ Создать ещё", "admin_promo_create")),
			menu.Row(menu.Data("⬅️ К промокодам", "admin_promo")),
		)
		return c.Send(text, menu, tele.ModeMarkdown)
	}

	return nil
}

// HandleAdminPromoList показывает список промокодов
func (h *Handler) HandleAdminPromoList(c tele.Context) error {
	promos, err := h.svc.GetAllPromoCodes(context.Background())
	if err != nil {
		return c.Send("❌ Ошибка загрузки промокодов")
	}

	if len(promos) == 0 {
		text := `📋 *Список промокодов*

_Нет активных промокодов._`
		menu := &tele.ReplyMarkup{}
		menu.Inline(
			menu.Row(menu.Data("➕ Создать", "admin_promo_create")),
			menu.Row(menu.Data("⬅️ Назад", "admin_promo")),
		)
		if c.Callback() != nil {
			return c.Edit(text, menu, tele.ModeMarkdown)
		}
		return c.Send(text, menu, tele.ModeMarkdown)
	}

	var sb strings.Builder
	sb.WriteString("📋 *Список промокодов*\n\n")

	for i, p := range promos {
		status := "✅"
		if !p.IsActive || p.ActivationsUsed >= p.MaxActivations {
			status = "❌"
		}
		sb.WriteString(fmt.Sprintf("%d. `%s` — *%.0f₽* (исп: %d/%d) %s\n",
			i+1, p.Code, p.Amount, p.ActivationsUsed, p.MaxActivations, status))
	}

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("🔄 Обновить", "admin_promo_list")),
		menu.Row(menu.Data("⬅️ Назад", "admin_promo")),
	)

	if c.Callback() != nil {
		return c.Edit(sb.String(), menu, tele.ModeMarkdown)
	}
	return c.Send(sb.String(), menu, tele.ModeMarkdown)
}

// HandleAdminPromoDelete начинает удаление промокода
func (h *Handler) HandleAdminPromoDelete(c tele.Context) error {
	promoDelete.mu.Lock()
	promoDelete.waiting[c.Sender().ID] = true
	promoDelete.mu.Unlock()

	text := `🗑 *Удаление промокода*

Введите код промокода для удаления:

_Например: SALE50_`

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("❌ Отмена", "admin_promo_cancel")),
	)

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// HandleAdminPromoDeleteInput обрабатывает удаление промокода
func (h *Handler) HandleAdminPromoDeleteInput(c tele.Context) error {
	promoDelete.mu.Lock()
	delete(promoDelete.waiting, c.Sender().ID)
	promoDelete.mu.Unlock()

	code := strings.TrimSpace(c.Text())

	// Проверяем существование
	promo, err := h.svc.GetPromoByCode(context.Background(), code)
	if err != nil || promo == nil {
		menu := &tele.ReplyMarkup{}
		menu.Inline(
			menu.Row(menu.Data("🗑 Попробовать снова", "admin_promo_delete")),
			menu.Row(menu.Data("⬅️ Назад", "admin_promo")),
		)
		return c.Send("❌ Промокод не найден.", menu)
	}

	// Удаляем
	if err := h.svc.DeletePromoCode(context.Background(), code); err != nil {
		return c.Send(fmt.Sprintf("❌ Ошибка удаления: %v", err))
	}

	text := fmt.Sprintf("✅ Промокод `%s` удалён.", promo.Code)

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("📋 Список кодов", "admin_promo_list")),
		menu.Row(menu.Data("⬅️ К промокодам", "admin_promo")),
	)
	return c.Send(text, menu, tele.ModeMarkdown)
}

// HandleAdminPromoCancel отменяет действие с промокодами
func (h *Handler) HandleAdminPromoCancel(c tele.Context) error {
	promoWizard.mu.Lock()
	delete(promoWizard.sessions, c.Sender().ID)
	promoWizard.mu.Unlock()

	promoDelete.mu.Lock()
	delete(promoDelete.waiting, c.Sender().ID)
	promoDelete.mu.Unlock()

	return h.HandleAdminPromo(c)
}

// HandleAdminPromoStats показывает статистику по промокодам
func (h *Handler) HandleAdminPromoStats(c tele.Context) error {
	stats, err := h.svc.GetPromoStats(context.Background())
	if err != nil {
		return c.Send("❌ Ошибка загрузки статистики промокодов")
	}

	if len(stats) == 0 {
		text := `🎟 *Статистика промокодов*

_Нет активных промокодов._`
		menu := &tele.ReplyMarkup{}
		menu.Inline(
			menu.Row(menu.Data("➕ Создать", "admin_promo_create")),
			menu.Row(menu.Data("⬅️ Назад", "admin_promo")),
		)
		if c.Callback() != nil {
			return c.Edit(text, menu, tele.ModeMarkdown)
		}
		return c.Send(text, menu, tele.ModeMarkdown)
	}

	var sb strings.Builder
	sb.WriteString("🎟 *Статистика промокодов:*\n\n")

	var totalBonusPaid float64
	for i, p := range stats {
		percent := 0
		if p.MaxActivations > 0 {
			percent = (p.ActivationsUsed * 100) / p.MaxActivations
		}
		sb.WriteString(fmt.Sprintf("%d. `%s`\n", i+1, p.Code))
		sb.WriteString(fmt.Sprintf("   ├ Активаций: *%d / %d* (%d%%)\n", p.ActivationsUsed, p.MaxActivations, percent))
		sb.WriteString(fmt.Sprintf("   └ Выдано бонусов: *%.0f ₽*\n\n", p.TotalBonusPaid))
		totalBonusPaid += p.TotalBonusPaid
	}

	sb.WriteString(fmt.Sprintf("💰 *Всего выдано:* %.0f ₽", totalBonusPaid))

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("🔄 Обновить", "admin_promo_stats")),
		menu.Row(menu.Data("⬅️ Назад", "admin_promo")),
	)

	if c.Callback() != nil {
		return c.Edit(sb.String(), menu, tele.ModeMarkdown)
	}
	return c.Send(sb.String(), menu, tele.ModeMarkdown)
}

// ================= TOP REFERRERS =================

// HandleAdminTopRefs показывает топ-10 рефоводов
func (h *Handler) HandleAdminTopRefs(c tele.Context) error {
	refs, err := h.svc.GetTopReferrers(context.Background())
	if err != nil {
		return c.Send("❌ Ошибка загрузки топ рефоводов")
	}

	if len(refs) == 0 {
		text := `🏆 *Топ-10 Партнеров (Рефоводов)*

_Пока нет пользователей с рефералами._`
		menu := &tele.ReplyMarkup{}
		menu.Inline(
			menu.Row(menu.Data("⬅️ Назад", "admin_back")),
		)
		if c.Callback() != nil {
			return c.Edit(text, menu, tele.ModeMarkdown)
		}
		return c.Send(text, menu, tele.ModeMarkdown)
	}

	var sb strings.Builder
	sb.WriteString("🏆 *Топ-10 Партнеров (Рефоводов)*\n\n")

	for i, ref := range refs {
		// Медаль для топ-3
		var medal string
		switch i {
		case 0:
			medal = "🥇 "
		case 1:
			medal = "🥈 "
		case 2:
			medal = "🥉 "
		default:
			medal = ""
		}

		// Форматируем username
		username := ref.Username
		if username == "" {
			username = fmt.Sprintf("ID:%d", ref.TelegramID)
		} else {
			username = "@" + username
		}

		sb.WriteString(fmt.Sprintf("%d. %s*%s* (ID: `%d`)\n", i+1, medal, username, ref.TelegramID))
		sb.WriteString(fmt.Sprintf("   ├ Пригласил: *%d чел.*\n", ref.ReferralCount))
		sb.WriteString(fmt.Sprintf("   └ Принес в кассу: *%.0f ₽*\n\n", ref.TotalRevenue))
	}

	sb.WriteString("_💡 Совет: Свяжитесь с лидерами для улучшения условий._")

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("🔄 Обновить", "admin_top_refs")),
		menu.Row(menu.Data("⬅️ Назад", "admin_back")),
	)

	if c.Callback() != nil {
		return c.Edit(sb.String(), menu, tele.ModeMarkdown)
	}
	return c.Send(sb.String(), menu, tele.ModeMarkdown)
}

// ================= USER PROMO CODE ACTIVATION =================

// HandleUserPromoInput обрабатывает ввод промокода пользователем
func (h *Handler) HandleUserPromoInput(c tele.Context) error {
	SetUserPromoMode(c.Sender().ID, false)

	code := strings.TrimSpace(c.Text())
	if len(code) < 3 {
		menu := &tele.ReplyMarkup{}
		menu.Inline(
			menu.Row(menu.Data("🎟 Попробовать снова", "promo_enter")),
			menu.Row(menu.Data("🏠 Главное меню", "back_main")),
		)
		return c.Send("❌ Некорректный промокод.", menu)
	}

	// Получаем пользователя
	ctx := context.Background()
	user, err := h.svc.GetOrCreateUser(ctx, c.Sender().ID, c.Sender().Username)
	if err != nil {
		return c.Send("❌ Ошибка. Попробуйте позже.")
	}

	// Активируем промокод
	amount, err := h.svc.ActivatePromoForUser(ctx, code, user.ID, c.Sender().ID)
	if err != nil {
		menu := &tele.ReplyMarkup{}
		menu.Inline(
			menu.Row(menu.Data("🎟 Попробовать другой", "promo_enter")),
			menu.Row(menu.Data("🏠 Главное меню", "back_main")),
		)
		return c.Send(fmt.Sprintf("❌ %s", err.Error()), menu)
	}

	// Успех!
	text := fmt.Sprintf(`✅ *Успешно!*

Промокод `+"`%s`"+` активирован.
💰 На ваш баланс зачислено: *%.0f ₽*`, strings.ToUpper(code), amount)

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("💰 Мой баланс", "balance")),
		menu.Row(menu.Data("🏠 Главное меню", "back_main")),
	)

	if UseBannerImages {
		photo := &tele.Photo{
			File:    tele.FromURL(MainBannerURL),
			Caption: text,
		}
		return c.Send(photo, menu, tele.ModeMarkdown)
	}

	return c.Send(text, menu, tele.ModeMarkdown)
}

// ================= SUPPORT BRIDGE =================

// RegisterSupportBridge регистрирует middleware для обработки ответов из группы поддержки
// ВАЖНО: Не регистрируем отдельные OnText/OnPhoto, т.к. они переопределят основные обработчики!
// Вместо этого проверка группы интегрирована в основные обработчики.
func (h *Handler) RegisterSupportBridge(b *tele.Bot, supportGroupID int64) {
	log.Printf("🎫 Support Bridge registered for group: %d", supportGroupID)
}

// handleSupportGroupMessage обрабатывает ответы админов в группе поддержки
func (h *Handler) handleSupportGroupMessage(c tele.Context) error {
	// Проверяем что это ответ на сообщение
	if c.Message() == nil || c.Message().ReplyTo == nil {
		return nil // Не ответ - игнорируем
	}

	replyTo := c.Message().ReplyTo

	// Ищем user ID в тексте сообщения (паттерн #user_123456)
	var targetUserID int64

	// 1. Проверяем текст сообщения на которое отвечают
	if replyTo.Text != "" {
		targetUserID = extractUserIDFromTicket(replyTo.Text)
		log.Printf("Support bridge: Checking replyTo.Text='%s', extracted ID=%d", replyTo.Text[:min(50, len(replyTo.Text))], targetUserID)
	}

	// 2. Если не нашли в тексте, проверяем caption (для фото)
	if targetUserID == 0 && replyTo.Caption != "" {
		targetUserID = extractUserIDFromTicket(replyTo.Caption)
		log.Printf("Support bridge: Checking replyTo.Caption, extracted ID=%d", targetUserID)
	}

	// 3. Fallback: если отвечают на пересланное сообщение с открытым профилем
	if targetUserID == 0 && replyTo.OriginalSender != nil {
		targetUserID = replyTo.OriginalSender.ID
		log.Printf("Support bridge: Using OriginalSender ID=%d", targetUserID)
	}

	// 4. Fallback: проверяем ReplyTo.ReplyTo (цепочка ответов)
	if targetUserID == 0 && replyTo.ReplyTo != nil {
		if replyTo.ReplyTo.Text != "" {
			targetUserID = extractUserIDFromTicket(replyTo.ReplyTo.Text)
			log.Printf("Support bridge: Checking nested ReplyTo.Text, extracted ID=%d", targetUserID)
		}
	}

	if targetUserID == 0 {
		log.Printf("Support bridge: Could not extract user ID from reply (replyTo.Sender=%v, replyTo.OriginalSender=%v)", 
			replyTo.Sender, replyTo.OriginalSender)
		return nil
	}

	// Отправляем ответ пользователю
	targetUser := &tele.User{ID: targetUserID}

	// Кнопки прикрепляются ПРЯМО к сообщению (Compact Mode)
	responseMenu := &tele.ReplyMarkup{}
	responseMenu.Inline(
		responseMenu.Row(
			responseMenu.Data("✍️ Ответить", "ticket_reply"),
			responseMenu.Data("✅ Решено", "ticket_solve"),
		),
	)

	// Отправляем контент с кнопками в зависимости от типа сообщения
	msg := c.Message()
	var err error

	if msg.Photo != nil {
		// Фото с кнопками
		photo := msg.Photo
		photo.Caption = "👨‍💻 *Ответ поддержки:*"
		if msg.Caption != "" {
			photo.Caption = msg.Caption
		}
		_, err = c.Bot().Send(targetUser, photo, responseMenu)
	} else if msg.Document != nil {
		// Документ с кнопками
		_, err = c.Bot().Send(targetUser, msg.Document, responseMenu)
	} else if msg.Voice != nil {
		// Голосовое с кнопками
		_, err = c.Bot().Send(targetUser, msg.Voice, responseMenu)
	} else if msg.Video != nil {
		// Видео с кнопками
		_, err = c.Bot().Send(targetUser, msg.Video, responseMenu)
	} else if msg.Sticker != nil {
		// Стикер — сначала стикер, потом кнопки отдельно (стикеры не поддерживают inline)
		c.Bot().Send(targetUser, msg.Sticker)
		_, err = c.Bot().Send(targetUser, "👆 _Ответ от поддержки_", responseMenu, tele.ModeMarkdown)
	} else {
		// Текст с кнопками
		text := fmt.Sprintf("👨‍💻 *Поддержка:*\n\n%s", msg.Text)
		_, err = c.Bot().Send(targetUser, text, responseMenu, tele.ModeMarkdown)
	}

	if err != nil {
		log.Printf("Support bridge: Failed to send reply to user %d: %v", targetUserID, err)
		return nil
	}

	// Обновляем трекер — помечаем как "отвечено"
	if tracker := GetTracker(); tracker != nil {
		tracker.SetTicketReplied(targetUserID)
		go tracker.UpdateDashboard()
	}

	log.Printf("🎫 Support bridge: Sent reply to user %d from admin %d", targetUserID, c.Sender().ID)
	return nil
}

// extractUserIDFromTicket извлекает user ID из текста тикета
func extractUserIDFromTicket(text string) int64 {
	// Ищем паттерн #user_123456
	re := regexp.MustCompile(`#user_(\d+)`)
	matches := re.FindStringSubmatch(text)
	if len(matches) >= 2 {
		id, err := strconv.ParseInt(matches[1], 10, 64)
		if err == nil {
			return id
		}
	}
	return 0
}

// HandleInitDashboard создаёт закреплённое сообщение dashboard
func (h *Handler) HandleInitDashboard(c tele.Context) error {
	// Только для группы поддержки
	if c.Chat().ID != h.supportGroupID {
		return c.Send("❌ Эта команда работает только в группе поддержки.")
	}

	// Создаём начальное сообщение dashboard
	text := `📊 *Панель управления поддержкой*

✅ *Нет активных обращений*

_Все тикеты обработаны!_`

	msg, err := c.Bot().Send(c.Chat(), text, tele.ModeMarkdown)
	if err != nil {
		return c.Send("❌ Ошибка создания dashboard: " + err.Error())
	}

	// Закрепляем сообщение
	err = c.Bot().Pin(msg, tele.Silent)
	if err != nil {
		log.Printf("Failed to pin dashboard: %v", err)
	}

	// Сохраняем ID сообщения в трекере
	if tracker := GetTracker(); tracker != nil {
		tracker.SetDashboardMessageID(msg.ID)
	}

	return c.Send(fmt.Sprintf("✅ Dashboard создан! Message ID: %d\n\nСообщение закреплено.", msg.ID))
}

// HandleAdminCloseTicket закрывает тикет из группы поддержки
func (h *Handler) HandleAdminCloseTicket(c tele.Context) error {
	if c.Callback() == nil {
		return nil
	}
	c.Respond()

	// Извлекаем userID из payload кнопки
	args := c.Args()
	if len(args) == 0 {
		log.Printf("HandleAdminCloseTicket: No args in callback")
		return nil
	}

	targetUserID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		log.Printf("HandleAdminCloseTicket: Failed to parse userID: %v", err)
		return nil
	}

	// 1. Сбрасываем состояние пользователя
	SetUserSupportMode(targetUserID, false)

	// 2. Удаляем из трекера и обновляем dashboard
	if tracker := GetTracker(); tracker != nil {
		tracker.RemoveTicket(targetUserID)
		go tracker.UpdateDashboard()
	}

	// 3. Уведомляем пользователя
	targetUser := &tele.User{ID: targetUserID}
	userNotification := `✅ *Тикет закрыт*

Оператор завершил обращение.
Если у вас появятся новые вопросы — создайте новый тикет в разделе *Поддержка*.`

	userMenu := &tele.ReplyMarkup{}
	userMenu.Inline(
		userMenu.Row(userMenu.Data("🛟 Поддержка", "support")),
		userMenu.Row(userMenu.Data("🏠 Главное меню", "back_main")),
	)

	_, err = c.Bot().Send(targetUser, userNotification, userMenu, tele.ModeMarkdown)
	if err != nil {
		log.Printf("HandleAdminCloseTicket: Failed to notify user %d: %v", targetUserID, err)
	}

	// 4. СРАЗУ обновляем сообщение в группе — убираем кнопку чтобы нельзя было нажать повторно!
	adminUsername := c.Sender().Username
	if adminUsername == "" {
		adminUsername = fmt.Sprintf("ID:%d", c.Sender().ID)
	}

	// Формируем текст закрытого тикета
	originalText := c.Message().Text
	if c.Message().Caption != "" {
		originalText = c.Message().Caption
	}

	closedText := fmt.Sprintf("✅ *Тикет закрыт*\n\n%s\n\n_Закрыл: @%s_", originalText, adminUsername)

	// Убираем inline keyboard передавая пустой ReplyMarkup
	emptyMenu := &tele.ReplyMarkup{}
	
	// Редактируем сообщение (для фото редактируем caption, для текста - текст)
	if c.Message().Photo != nil {
		_, err = c.Bot().EditCaption(c.Message(), closedText, emptyMenu, tele.ModeMarkdown)
		return err
	}
	
	return c.Edit(closedText, emptyMenu, tele.ModeMarkdown)
}
