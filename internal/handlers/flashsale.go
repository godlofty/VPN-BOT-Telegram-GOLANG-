package handlers

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	tele "gopkg.in/telebot.v3"
)

// flashSaleState хранит состояние флеш-распродажи
type flashSaleState struct {
	mu              sync.RWMutex
	discountPercent int
	endTime         time.Time
}

var flashSale = &flashSaleState{}

// SetFlashSale устанавливает флеш-распродажу
func (f *flashSaleState) Set(percent int, hours int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.discountPercent = percent
	f.endTime = time.Now().Add(time.Duration(hours) * time.Hour)
}

// Clear очищает флеш-распродажу
func (f *flashSaleState) Clear() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.discountPercent = 0
	f.endTime = time.Time{}
}

// IsActive проверяет, активна ли распродажа
func (f *flashSaleState) IsActive() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.discountPercent > 0 && time.Now().Before(f.endTime)
}

// GetDiscount возвращает текущую скидку (0 если не активна)
func (f *flashSaleState) GetDiscount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if time.Now().Before(f.endTime) {
		return f.discountPercent
	}
	return 0
}

// GetEndTime возвращает время окончания
func (f *flashSaleState) GetEndTime() time.Time {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.endTime
}

// ApplyDiscount применяет скидку к цене
func (f *flashSaleState) ApplyDiscount(originalPrice float64) float64 {
	discount := f.GetDiscount()
	if discount <= 0 {
		return originalPrice
	}
	return originalPrice * float64(100-discount) / 100
}

// GetFlashSale возвращает глобальный объект для использования вне handlers
func GetFlashSale() *flashSaleState {
	return flashSale
}

// flashSaleSession хранит состояние ввода админа
type flashSaleSession struct {
	step     int // 1=percent, 2=hours
	percent  int
	hours    int
	photoURL string
}

var flashSaleSessions = struct {
	mu       sync.Mutex
	sessions map[int64]*flashSaleSession
}{
	sessions: make(map[int64]*flashSaleSession),
}

// RegisterFlashSale регистрирует обработчики флеш-распродаж
func (h *Handler) RegisterFlashSale(b *tele.Bot, adminGroup *tele.Group) {
	adminGroup.Handle("/flashsale", h.HandleFlashSaleStart)
	adminGroup.Handle("/stopsale", h.HandleStopSale)
	adminGroup.Handle(&tele.Btn{Unique: "flash_start"}, h.HandleFlashSaleStart)
	adminGroup.Handle(&tele.Btn{Unique: "flash_manual"}, h.HandleFlashManual)
	adminGroup.Handle(&tele.Btn{Unique: "flash_stop"}, h.HandleStopSaleCallback)
	adminGroup.Handle(&tele.Btn{Unique: "flash_percent"}, h.HandleFlashPercent)
	adminGroup.Handle(&tele.Btn{Unique: "flash_hours"}, h.HandleFlashHours)
	adminGroup.Handle(&tele.Btn{Unique: "flash_confirm"}, h.HandleFlashConfirm)
	adminGroup.Handle(&tele.Btn{Unique: "flash_cancel"}, h.HandleFlashCancel)

	// Callback для удаления сообщения (доступен всем)
	b.Handle(&tele.Btn{Unique: "delete_msg"}, h.HandleDeleteMessage)
}

// HandleFlashSaleStart начинает создание флеш-распродажи
func (h *Handler) HandleFlashSaleStart(c tele.Context) error {
	// Проверяем аргументы: /flashsale 50 24
	args := c.Args()
	if len(args) >= 2 {
		percent, err1 := strconv.Atoi(args[0])
		hours, err2 := strconv.Atoi(args[1])
		if err1 == nil && err2 == nil && percent > 0 && percent <= 90 && hours > 0 {
			// Быстрый режим
			flashSaleSessions.mu.Lock()
			flashSaleSessions.sessions[c.Sender().ID] = &flashSaleSession{
				step:    3,
				percent: percent,
				hours:   hours,
			}
			flashSaleSessions.mu.Unlock()

			return h.showFlashConfirm(c, percent, hours)
		}
	}

	// Проверяем, есть ли активная распродажа
	var activeText string
	if flashSale.IsActive() {
		activeText = fmt.Sprintf("\n\n⚠️ *Активная акция:* -%d%% до %s",
			flashSale.GetDiscount(), flashSale.GetEndTime().Format("15:04"))
	}

	// Интерактивный режим с быстрыми кнопками
	text := fmt.Sprintf(`⚡️ *Флеш-распродажа*

Создайте срочную акцию со скидкой для всех пользователей.%s

🚀 *Быстрый запуск:*`, activeText)

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(
			menu.Data("🔥 50%% на 6ч", "flash_quick", "50:6"),
			menu.Data("🔥 50%% на 24ч", "flash_quick", "50:24"),
		),
		menu.Row(
			menu.Data("💥 30%% на 12ч", "flash_quick", "30:12"),
			menu.Data("💥 25%% на 48ч", "flash_quick", "25:48"),
		),
		menu.Row(menu.Data("⚙️ Настроить вручную", "flash_manual")),
		menu.Row(menu.Data("🛑 Остановить акцию", "flash_stop")),
		menu.Row(menu.Data("⬅️ Назад", "admin_back")),
	)

	if c.Callback() != nil {
		return c.Edit(text, menu, tele.ModeMarkdown)
	}
	return c.Send(text, menu, tele.ModeMarkdown)
}

// HandleFlashManual показывает ручную настройку
func (h *Handler) HandleFlashManual(c tele.Context) error {
	text := `⚙️ *Ручная настройка*

*Выберите размер скидки:*`

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(
			menu.Data("20%", "flash_percent", "20"),
			menu.Data("30%", "flash_percent", "30"),
			menu.Data("40%", "flash_percent", "40"),
		),
		menu.Row(
			menu.Data("50%", "flash_percent", "50"),
			menu.Data("60%", "flash_percent", "60"),
			menu.Data("70%", "flash_percent", "70"),
		),
		menu.Row(menu.Data("⬅️ Назад", "flash_start")),
	)

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// HandleStopSaleCallback останавливает распродажу (callback)
func (h *Handler) HandleStopSaleCallback(c tele.Context) error {
	if !flashSale.IsActive() {
		menu := &tele.ReplyMarkup{}
		menu.Inline(
			menu.Row(menu.Data("⬅️ Назад", "flash_start")),
		)
		return c.Edit("ℹ️ Сейчас нет активных распродаж.", menu)
	}

	flashSale.Clear()
	log.Printf("[FLASH SALE] Admin %d stopped flash sale via button", c.Sender().ID)

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("⚡️ Запустить новую", "flash_start")),
		menu.Row(menu.Data("⬅️ В админ-панель", "admin_back")),
	)

	return c.Edit("✅ *Флеш-распродажа остановлена.*\n\nЦены вернулись к обычным.", menu, tele.ModeMarkdown)
}

// HandleFlashPercent обрабатывает выбор процента скидки
func (h *Handler) HandleFlashPercent(c tele.Context) error {
	percent, err := strconv.Atoi(c.Callback().Data)
	if err != nil {
		return c.Send("❌ Ошибка")
	}

	flashSaleSessions.mu.Lock()
	flashSaleSessions.sessions[c.Sender().ID] = &flashSaleSession{
		step:    2,
		percent: percent,
	}
	flashSaleSessions.mu.Unlock()

	text := fmt.Sprintf(`⚙️ *Ручная настройка*

✅ Скидка: *%d%%*

*Выберите длительность акции:*`, percent)

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(
			menu.Data("1 час", "flash_hours", "1"),
			menu.Data("2 часа", "flash_hours", "2"),
			menu.Data("3 часа", "flash_hours", "3"),
		),
		menu.Row(
			menu.Data("6 часов", "flash_hours", "6"),
			menu.Data("12 часов", "flash_hours", "12"),
			menu.Data("24 часа", "flash_hours", "24"),
		),
		menu.Row(menu.Data("❌ Отмена", "flash_cancel")),
	)

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// HandleFlashHours обрабатывает выбор длительности
func (h *Handler) HandleFlashHours(c tele.Context) error {
	hours, err := strconv.Atoi(c.Callback().Data)
	if err != nil {
		return c.Send("❌ Ошибка")
	}

	flashSaleSessions.mu.Lock()
	session, exists := flashSaleSessions.sessions[c.Sender().ID]
	if !exists {
		flashSaleSessions.mu.Unlock()
		return h.HandleFlashSaleStart(c)
	}
	session.hours = hours
	session.step = 3
	percent := session.percent
	flashSaleSessions.mu.Unlock()

	return h.showFlashConfirm(c, percent, hours)
}

// showFlashConfirm показывает подтверждение
func (h *Handler) showFlashConfirm(c tele.Context, percent, hours int) error {
	// Рассчитываем цены
	originalPrice := 450.0
	newPrice := originalPrice * float64(100-percent) / 100

	endTime := time.Now().Add(time.Duration(hours) * time.Hour)

	text := fmt.Sprintf(`🔥 *Подтверждение флеш-распродажи*

📊 *Параметры:*
• Скидка: *%d%%*
• Длительность: *%d ч.*
• Окончание: *%s*

💰 *Цены:*
• X-RAY MODE: ~%.0f ₽~ → *%.0f ₽*

📢 *Рассылка:*
Уведомление будет отправлено всем пользователям.

*Запустить распродажу?*`,
		percent, hours, endTime.Format("02.01 15:04"),
		originalPrice, newPrice)

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("🚀 Запустить!", "flash_confirm")),
		menu.Row(menu.Data("❌ Отмена", "flash_cancel")),
	)

	if c.Callback() != nil {
		return c.Edit(text, menu, tele.ModeMarkdown)
	}
	return c.Send(text, menu, tele.ModeMarkdown)
}

// HandleFlashConfirm подтверждает и запускает флеш-распродажу
func (h *Handler) HandleFlashConfirm(c tele.Context) error {
	flashSaleSessions.mu.Lock()
	session, exists := flashSaleSessions.sessions[c.Sender().ID]
	if !exists || session.step != 3 {
		flashSaleSessions.mu.Unlock()
		return c.Send("❌ Сессия истекла. Начните заново: /flashsale")
	}
	percent := session.percent
	hours := session.hours
	delete(flashSaleSessions.sessions, c.Sender().ID)
	flashSaleSessions.mu.Unlock()

	// Устанавливаем скидку
	flashSale.Set(percent, hours)
	endTime := flashSale.GetEndTime()

	log.Printf("[FLASH SALE] Admin %d started %d%% sale for %d hours", c.Sender().ID, percent, hours)

	c.Edit(fmt.Sprintf("✅ *Флеш-распродажа запущена!*\n\nСкидка %d%% активна до %s\n\n📤 Запускаю рассылку...",
		percent, endTime.Format("02.01 15:04")), tele.ModeMarkdown)

	// Запускаем рассылку в горутине
	go h.broadcastFlashSale(c.Bot(), c.Sender().ID, percent, hours, endTime)

	return nil
}

// FlashSaleBroadcastImageURL — изображение для рассылки флеш-распродажи
// TODO: Замените на актуальную ссылку на изображение "СКИДКИ XX%"
const FlashSaleBroadcastImageURL = "https://drive.google.com/uc?export=view&id=17ZGub9P-QQZ4X8_OTDORSWzuicuE5PD3"

// broadcastFlashSale рассылает уведомление о распродаже с картинкой
func (h *Handler) broadcastFlashSale(bot *tele.Bot, adminID int64, percent, hours int, endTime time.Time) {
	ctx := context.Background()

	userIDs, err := h.svc.GetAllUserTelegramIDs(ctx)
	if err != nil {
		log.Printf("[FLASH SALE] Failed to get user IDs: %v", err)
		bot.Send(&tele.User{ID: adminID}, fmt.Sprintf("❌ Ошибка получения списка пользователей: %v", err))
		return
	}

	// Рассчитываем новую цену
	originalPrice := 450.0
	newPrice := originalPrice * float64(100-percent) / 100

	// Формируем текст (caption для фото)
	var hoursText string
	switch hours {
	case 1:
		hoursText = "1 час"
	case 2, 3, 4:
		hoursText = fmt.Sprintf("%d часа", hours)
	default:
		hoursText = fmt.Sprintf("%d часов", hours)
	}

	caption := fmt.Sprintf(`🚨 *РАСПРОДАЖА! СКИДКИ -%d%%*

Только ближайшие *%s*!
Цены на все тарифы снижены. Успей забрать свой VPN за копейки.

💰 X-RAY MODE: ~%.0f ₽~ → *%.0f ₽*

⏳ Акция закончится: *%s*`,
		percent, hoursText,
		originalPrice, newPrice,
		endTime.Format("02.01.2006 15:04"))

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("💎 Выбрать тариф", "tariffs")),
		menu.Row(menu.Data("⏰ Продлить подписку", "mysubs")),
		menu.Row(menu.Data("❌ Закрыть", "delete_msg")),
	)

	// Создаём фото с caption
	photo := &tele.Photo{
		File:    tele.FromURL(FlashSaleBroadcastImageURL),
		Caption: caption,
	}

	totalUsers := len(userIDs)
	var sent, failed int
	ticker := time.NewTicker(50 * time.Millisecond) // 20 messages per second
	defer ticker.Stop()

	for _, userID := range userIDs {
		<-ticker.C

		_, err := bot.Send(&tele.User{ID: userID}, photo, menu, tele.ModeMarkdown)
		if err != nil {
			failed++
			if !strings.Contains(err.Error(), "blocked") && !strings.Contains(err.Error(), "deactivated") {
				log.Printf("[FLASH SALE] Failed for user %d: %v", userID, err)
			}
		} else {
			sent++
		}

		// Прогресс каждые 100 пользователей
		if (sent+failed)%100 == 0 && totalUsers > 100 {
			bot.Send(&tele.User{ID: adminID},
				fmt.Sprintf("📤 Прогресс рассылки: %d/%d", sent+failed, totalUsers))
		}
	}

	log.Printf("[FLASH SALE] Broadcast finished. Sent: %d, Failed: %d", sent, failed)

	bot.Send(&tele.User{ID: adminID},
		fmt.Sprintf("✅ *Рассылка завершена!*\n\n📤 Отправлено: %d\n❌ Ошибок: %d\n📊 Всего: %d\n\n🔥 Распродажа активна до %s",
			sent, failed, totalUsers, endTime.Format("02.01 15:04")), tele.ModeMarkdown)
}

// HandleFlashCancel отменяет создание флеш-распродажи
func (h *Handler) HandleFlashCancel(c tele.Context) error {
	flashSaleSessions.mu.Lock()
	delete(flashSaleSessions.sessions, c.Sender().ID)
	flashSaleSessions.mu.Unlock()

	return h.HandleAdmin(c)
}

// HandleStopSale останавливает текущую распродажу
func (h *Handler) HandleStopSale(c tele.Context) error {
	if !flashSale.IsActive() {
		return c.Send("ℹ️ Сейчас нет активных распродаж.")
	}

	flashSale.Clear()
	log.Printf("[FLASH SALE] Admin %d stopped flash sale", c.Sender().ID)

	return c.Send("✅ Флеш-распродажа остановлена. Цены вернулись к обычным.")
}

// HandleDeleteMessage удаляет сообщение (для кнопки "Закрыть")
func (h *Handler) HandleDeleteMessage(c tele.Context) error {
	return c.Delete()
}
