package handlers

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"vpn-telegram-bot/internal/service"

	tele "gopkg.in/telebot.v3"
)

// MainBannerURL - Direct download link for the X-RAY VPN Banner
const MainBannerURL = "https://i.ibb.co/NdHPS9kS/wmremove-transformed.png"

// UseBannerImages - Set to true to enable banner images in menus
const UseBannerImages = true

// Handler обработчики бота
type Handler struct {
	svc            *service.Service
	adminIDs       []int64
	supportGroupID int64
}

// New создаёт новый handler
func New(svc *service.Service, adminIDs []int64, supportGroupID int64) *Handler {
	return &Handler{
		svc:            svc,
		adminIDs:       adminIDs,
		supportGroupID: supportGroupID,
	}
}

// Register регистрирует все обработчики
func (h *Handler) Register(b *tele.Bot) {
	// Commands
	b.Handle("/start", h.HandleStart)
	b.Handle("/help", h.HandleHelp)
	b.Handle("/tariffs", h.HandleTariffs)
	b.Handle("/mysubs", h.HandleMySubs)
	b.Handle("/privacy", h.HandlePrivacy)

	// Callbacks
	b.Handle(&tele.Btn{Unique: "tariffs"}, h.HandleTariffs)
	b.Handle(&tele.Btn{Unique: "mysubs"}, h.HandleMySubs)
	b.Handle(&tele.Btn{Unique: "instruction"}, h.HandleInstruction)
	b.Handle(&tele.Btn{Unique: "help"}, h.HandleHelp)
	b.Handle(&tele.Btn{Unique: "back_main"}, h.HandleBackToMain)
	b.Handle(&tele.Btn{Unique: "privacy"}, h.HandlePrivacy)

	// Product selection
	b.Handle(&tele.Btn{Unique: "product"}, h.HandleProductSelect)
	b.Handle(&tele.Btn{Unique: "xray_mode"}, h.HandleXRayMode)

	// Plan selection (months)
	b.Handle(&tele.Btn{Unique: "plan"}, h.HandlePlanSelect)

	// Subscription details
	b.Handle(&tele.Btn{Unique: "sub"}, h.HandleSubDetail)
	b.Handle(&tele.Btn{Unique: "copy_key"}, h.HandleCopyKey)
	b.Handle(&tele.Btn{Unique: "extend"}, h.HandleExtend)

	// Instructions
	b.Handle(&tele.Btn{Unique: "instr_android"}, h.HandleInstrAndroid)
	b.Handle(&tele.Btn{Unique: "instr_windows"}, h.HandleInstrWindows)
	b.Handle(&tele.Btn{Unique: "instr_iphone"}, h.HandleInstrIphone)
	b.Handle(&tele.Btn{Unique: "instr_mac"}, h.HandleInstrMac)

	// Help section
	b.Handle(&tele.Btn{Unique: "faq"}, h.HandleFAQ)
	b.Handle(&tele.Btn{Unique: "support"}, h.HandleSupportHub)
	b.Handle(&tele.Btn{Unique: "ticket_create"}, h.HandleCreateTicket)
	b.Handle(&tele.Btn{Unique: "ticket_list"}, h.HandleMyTickets)
	b.Handle(&tele.Btn{Unique: "exit_support"}, h.HandleExitSupport)
	b.Handle(&tele.Btn{Unique: "ticket_reply"}, h.HandleTicketReply)
	b.Handle(&tele.Btn{Unique: "ticket_solve"}, h.HandleTicketSolve)
	b.Handle(&tele.Btn{Unique: "ticket_cancel_reply"}, h.HandleTicketCancelReply)
	b.Handle(&tele.Btn{Unique: "back_to_support_hub"}, h.HandleBackToSupportHub)

	// Balance & Promo
	b.Handle(&tele.Btn{Unique: "balance"}, h.HandleBalance)
	b.Handle(&tele.Btn{Unique: "topup"}, h.HandleTopUp)
	b.Handle(&tele.Btn{Unique: "topup_amount"}, h.HandleTopUpAmount)
	b.Handle(&tele.Btn{Unique: "topup_pay_card"}, h.HandleTopUpPayCard)
	b.Handle(&tele.Btn{Unique: "topup_pay_crypto"}, h.HandleTopUpPayCrypto)
	b.Handle(&tele.Btn{Unique: "pay_balance"}, h.HandlePayWithBalance)
	b.Handle(&tele.Btn{Unique: "promo_enter"}, h.HandlePromoEnter)

	// Subscription Extension
	b.Handle(&tele.Btn{Unique: "extend_pay"}, h.HandleExtendPay)

	// Referral System
	b.Handle(&tele.Btn{Unique: "ref_system"}, h.HandleRefSystem)
	b.Handle(&tele.Btn{Unique: "ref_list"}, h.HandleRefList)
}

// ================= MAIN MENU =================

// HandleStart обрабатывает /start с поддержкой реферальных ссылок
func (h *Handler) HandleStart(c tele.Context) error {
	ctx := context.Background()
	telegramID := c.Sender().ID
	username := c.Sender().Username

	// Проверяем реферальную ссылку (/start 12345) - только для команды, не для callback
	if c.Message() != nil {
		payload := c.Message().Payload
		if payload != "" {
			referrerID, err := strconv.ParseInt(payload, 10, 64)
			if err == nil && referrerID != telegramID {
				// Проверяем что пользователь новый
				exists, _ := h.svc.UserExists(ctx, telegramID)
				if !exists {
					// Создаём пользователя с реферером
					_, err = h.svc.CreateUserWithReferrer(ctx, telegramID, username, referrerID)
					if err != nil {
						log.Printf("Error creating user with referrer: %v", err)
					} else {
						log.Printf("New user %d referred by %d", telegramID, referrerID)
					}
				}
			}
		}
	}

	// Получаем или создаём пользователя (если ещё не создан)
	_, err := h.svc.GetOrCreateUser(ctx, telegramID, username)
	if err != nil {
		log.Printf("Error getting user: %v", err)
	}

	return h.showMainMenu(c, false)
}

// HandleBackToMain возвращает в главное меню (для callback кнопок)
func (h *Handler) HandleBackToMain(c tele.Context) error {
	ctx := context.Background()
	_, err := h.svc.GetOrCreateUser(ctx, c.Sender().ID, c.Sender().Username)
	if err != nil {
		log.Printf("Error getting user: %v", err)
	}
	return h.showMainMenu(c, true)
}

// showMainMenu отображает главное меню
func (h *Handler) showMainMenu(c tele.Context, edit bool) error {
	text := `⚡️ *Система X-RAY VPN активирована.*

Добро пожаловать в цифровую тень. Твой трафик теперь проходит сквозь любые преграды, оставаясь невидимым для посторонних глаз.

*Почему мы?*
👻 *Абсолютная анонимность:* Мы не ведем логи. В системе сохраняется только твой Telegram ID и факт оплаты для доступа к ключу. Твоя история браузера — только твоя.
🛡 *Максимальная защита:* Твои данные в броне протокола Reality.
📱 *Мульти-доступ:* Подключай до 3-х устройств на один ключ (Телефон + ПК + Планшет).
🚀 *Космическая скорость:* Смотри 4K видео и играй без лагов.

Твой интернет — твои правила. Включай X-RAY.`

	menu := &tele.ReplyMarkup{}
	btnTariffs := menu.Data("💎 Тарифы", "tariffs")
	btnMySubs := menu.Data("🔑 Мои подписки", "mysubs")
	btnBalance := menu.Data("💰 Баланс", "balance")
	btnPromo := menu.Data("🎟 Промокод", "promo_enter")
	btnRefSystem := menu.Data("👥 Партнёрка", "ref_system")
	btnHelp := menu.Data("🛟 Помощь", "help")
	btnChannel := menu.URL("📢 Канал", "https://t.me/XRAY_MODE")
	btnChat := menu.URL("💬 Чат", "https://t.me/XRAY_LUV")

	menu.Inline(
		menu.Row(btnTariffs, btnMySubs),
		menu.Row(btnBalance, btnPromo),
		menu.Row(btnRefSystem, btnHelp),
		menu.Row(btnChannel, btnChat),
	)

	if UseBannerImages {
		photo := &tele.Photo{
			File:    tele.FromURL(MainBannerURL),
			Caption: text,
		}
		if edit {
			c.Delete()
		}
		return c.Send(photo, menu, tele.ModeMarkdown)
	}

	if edit {
		return c.Edit(text, menu, tele.ModeMarkdown)
	}
	return c.Send(text, menu, tele.ModeMarkdown)
}

// ================= TARIFFS =================

// HandleTariffs показывает тарифы
func (h *Handler) HandleTariffs(c tele.Context) error {
	const basePrice = 450.0

	var text string
	var btnText string

	// Проверяем активную флеш-распродажу
	if flashSale.IsActive() {
		discount := flashSale.GetDiscount()
		newPrice := flashSale.ApplyDiscount(basePrice)
		endTime := flashSale.GetEndTime()

		text = fmt.Sprintf(`🔥 *РАСПРОДАЖА -%d%%!*
⏳ До окончания: *%s*

🌍 *Выберите тариф:*`, discount, endTime.Format("02.01 15:04"))

		btnText = fmt.Sprintf("🌍 X-RAY MODE — ~%.0f~ %.0f ₽/мес 🔥", basePrice, newPrice)
	} else {
		text = `🌍 *Выберите тариф:*`

		btnText = "🌍 X-RAY MODE — 450 ₽/мес"
	}

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data(btnText, "xray_mode")),
		menu.Row(menu.URL("⭐️ Отзывы (Чат)", "https://t.me/XRAY_LUV")),
		menu.Row(menu.Data("⬅️ Вернуться", "back_main")),
	)

	if UseBannerImages {
		photo := &tele.Photo{
			File:    tele.FromURL(MainBannerURL),
			Caption: text,
		}
		c.Delete()
		return c.Send(photo, menu, tele.ModeMarkdown)
	}

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// HandleXRayMode показывает описание X-RAY MODE и выбор периода
func (h *Handler) HandleXRayMode(c tele.Context) error {
	// X-RAY MODE имеет product_id = 1 (Мульти в базе)
	const xrayModeProductID int64 = 1
	const basePrice float64 = 450

	var text string

	// Проверяем флеш-распродажу
	if flashSale.IsActive() {
		discount := flashSale.GetDiscount()
		endTime := flashSale.GetEndTime()

		text = fmt.Sprintf(`🔥 *РАСПРОДАЖА -%d%%!*
⏳ До: *%s*

🚀 *X-RAY MODE*
🇵🇱 Польша (Premium) — Ультра-низкий пинг
🔜 _Новые страны появятся автоматически!_

🛡 До 3-х устройств | ⚡️ Безлимит | 🔒 VLESS

👇 *Выберите период:*`, discount, endTime.Format("02.01 15:04"))
	} else {
		text = `🚀 *X-RAY MODE*

🇵🇱 *Польша (Premium)* — Ультра-низкий пинг
🔜 _Новые страны (🇳🇱 🇩🇪 🇺🇸) появятся автоматически!_

🛡 До 3-х устройств | ⚡️ Безлимит | 🔒 VLESS

👇 *Выберите период:*`
	}

	plans := h.svc.GetPricingPlans(basePrice)

	menu := &tele.ReplyMarkup{}
	var rows []tele.Row

	// Применяем флеш-скидку к ценам
	flashDiscount := flashSale.GetDiscount()

	for _, plan := range plans {
		var btnText string
		finalPrice := plan.Price

		// Применяем флеш-скидку
		if flashDiscount > 0 {
			finalPrice = plan.Price * float64(100-flashDiscount) / 100
		}

		if flashDiscount > 0 {
			// С флеш-скидкой показываем старую и новую цену
			if plan.Discount > 0 {
				btnText = fmt.Sprintf("%d мес. ~%d~ %d ₽ 🔥", plan.Months, int(plan.Price), int(finalPrice))
			} else if plan.Months == 1 {
				btnText = fmt.Sprintf("1 мес. ~%d~ %d ₽ 🔥", int(plan.Price), int(finalPrice))
			} else {
				btnText = fmt.Sprintf("%d мес. ~%d~ %d ₽ 🔥", plan.Months, int(plan.Price), int(finalPrice))
			}
		} else {
			// Обычные цены
			if plan.Discount > 0 {
				btnText = fmt.Sprintf("%d месяцев (-%d%%) — %d ₽", plan.Months, plan.Discount, int(plan.Price))
			} else if plan.Months == 1 {
				btnText = fmt.Sprintf("1 месяц — %d ₽", int(plan.Price))
			} else {
				btnText = fmt.Sprintf("%d месяца — %d ₽", plan.Months, int(plan.Price))
			}
		}
		btn := menu.Data(btnText, "plan", fmt.Sprintf("%d:%d", xrayModeProductID, plan.Months))
		rows = append(rows, menu.Row(btn))
	}

	rows = append(rows, menu.Row(menu.Data("⬅️ Назад", "tariffs")))
	menu.Inline(rows...)

	if UseBannerImages {
		photo := &tele.Photo{
			File:    tele.FromURL(MainBannerURL),
			Caption: text,
		}
		c.Delete()
		return c.Send(photo, menu, tele.ModeMarkdown)
	}

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// HandleProductSelect показывает детали продукта
func (h *Handler) HandleProductSelect(c tele.Context) error {
	productID, _ := strconv.ParseInt(c.Callback().Data, 10, 64)

	product, err := h.svc.GetProductByID(context.Background(), productID)
	if err != nil {
		return c.Send("❌ Продукт не найден")
	}

	plans := h.svc.GetPricingPlans(product.BasePrice)

	text := fmt.Sprintf(`%s *%s*

💰 Базовая цена: %d ₽/мес
📝 %s

*Скидки:*
• 6 месяцев — скидка 10%%
• 12 месяцев — скидка 20%%

Выберите срок подписки:`, product.CountryFlag, product.Name, int(product.BasePrice), product.Description)

	menu := &tele.ReplyMarkup{}
	var rows []tele.Row

	for _, plan := range plans {
		var btnText string
		if plan.Discount > 0 {
			btnText = fmt.Sprintf("%d месяцев (-%d%%) — %d ₽", plan.Months, plan.Discount, int(plan.Price))
		} else if plan.Months == 1 {
			btnText = fmt.Sprintf("1 месяц — %d ₽", int(plan.Price))
		} else {
			btnText = fmt.Sprintf("%d месяца — %d ₽", plan.Months, int(plan.Price))
		}
		btn := menu.Data(btnText, "plan", fmt.Sprintf("%d:%d", productID, plan.Months))
		rows = append(rows, menu.Row(btn))
	}

	rows = append(rows, menu.Row(menu.Data("⬅️ Вернуться", "tariffs")))
	menu.Inline(rows...)

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// HandlePlanSelect обрабатывает выбор плана
func (h *Handler) HandlePlanSelect(c tele.Context) error {
	parts := strings.Split(c.Callback().Data, ":")
	if len(parts) != 2 {
		return c.Send("❌ Ошибка")
	}

	productID, _ := strconv.ParseInt(parts[0], 10, 64)
	months, _ := strconv.Atoi(parts[1])

	product, err := h.svc.GetProductByID(context.Background(), productID)
	if err != nil {
		return c.Send("❌ Продукт не найден")
	}

	price, discount := h.svc.CalculatePrice(product.BasePrice, months)
	originalPrice := price

	// Применяем флеш-скидку
	flashDiscount := flashSale.GetDiscount()
	if flashDiscount > 0 {
		price = flashSale.ApplyDiscount(price)
	}

	var discountText string
	if flashDiscount > 0 {
		discountText = fmt.Sprintf(" 🔥 *АКЦИЯ -%d%%!*", flashDiscount)
	} else if discount > 0 {
		discountText = fmt.Sprintf(" (скидка %d%%)", discount)
	}

	var priceText string
	if flashDiscount > 0 {
		priceText = fmt.Sprintf("~%.0f~ *%.0f* ₽", originalPrice, price)
	} else {
		priceText = fmt.Sprintf("%d ₽", int(price))
	}

	// Формируем текст срока
	var periodText string
	if months == 1 {
		periodText = "1 месяц"
	} else if months < 5 {
		periodText = fmt.Sprintf("%d месяца", months)
	} else {
		periodText = fmt.Sprintf("%d месяцев", months)
	}

	text := fmt.Sprintf(`💳 *Счёт на оплату*
—————————————————
💎 *Тариф:* %s %s (%s)
💰 *Сумма:* %s%s

🎁 *БОНУС: +7 ДНЕЙ В ПОДАРОК!*
При оплате *Криптовалютой* (USDT, TON, BTC) срок вашей подписки увеличится автоматически.
✅ _Бонус начислится сразу после оплаты._

👇 *Выберите способ оплаты:*`, product.CountryFlag, product.Name, periodText, priceText, discountText)

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("💠 СБП (Быстрый платёж)", "pay_card", c.Callback().Data)),
		menu.Row(menu.Data("🌑 Криптовалюта (+7 дней 🎁)", "pay_crypto", c.Callback().Data)),
		menu.Row(menu.Data("💰 С баланса", "pay_balance", c.Callback().Data)),
		menu.Row(menu.Data("⬅️ Назад", "xray_mode")),
	)

	if UseBannerImages {
		photo := &tele.Photo{
			File:    tele.FromURL(MainBannerURL),
			Caption: text,
		}
		c.Delete()
		return c.Send(photo, menu, tele.ModeMarkdown)
	}

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// ================= MY SUBSCRIPTIONS =================

// HandleMySubs показывает подписки пользователя
func (h *Handler) HandleMySubs(c tele.Context) error {
	log.Printf("👉 HandleMySubs triggered for User: %d", c.Sender().ID)

	user, err := h.svc.GetOrCreateUser(context.Background(), c.Sender().ID, c.Sender().Username)
	if err != nil {
		return c.Send("❌ Ошибка")
	}

	subs, err := h.svc.GetUserSubscriptions(context.Background(), user.ID)
	if err != nil {
		return c.Send("❌ Ошибка загрузки подписок")
	}

	var text string
	menu := &tele.ReplyMarkup{}

	if len(subs) == 0 {
		text = `🔑 *Ваши подписки*

У вас пока нет активных подписок.
Выберите тариф и подключайтесь! 🚀`

		menu.Inline(
			menu.Row(menu.Data("💎 Выбрать тариф", "tariffs")),
			menu.Row(menu.Data("⬅️ Назад", "back_main")),
		)
	} else {
		text = "🔑 *Ваши подписки:*\n"

	var rows []tele.Row
	for _, sub := range subs {
		status := ""
		if sub.ExpiresAt.Before(time.Now()) || !sub.IsActive {
			status = " [Истёк]"
		}

		btnText := fmt.Sprintf("%s %s №%d%s", sub.Product.CountryFlag, sub.Product.Name, sub.ID, status)
		btn := menu.Data(btnText, "sub", strconv.FormatInt(sub.ID, 10))
		rows = append(rows, menu.Row(btn))
	}

		rows = append(rows, menu.Row(menu.Data("⬅️ Назад", "back_main")))
	menu.Inline(rows...)
	}

	if UseBannerImages {
		photo := &tele.Photo{
			File:    tele.FromURL(MainBannerURL),
			Caption: text,
		}
		c.Delete()
		return c.Send(photo, menu, tele.ModeMarkdown)
	}

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// HandleSubDetail показывает детали подписки
func (h *Handler) HandleSubDetail(c tele.Context) error {
	log.Printf("👉 HandleSubDetail triggered for User: %d, Data: %s", c.Sender().ID, c.Callback().Data)

	subID, err := strconv.ParseInt(c.Callback().Data, 10, 64)
	if err != nil {
		log.Printf("❌ HandleSubDetail: invalid subID: %v", err)
		return c.Respond(&tele.CallbackResponse{Text: "❌ Ошибка"})
	}

	sub, err := h.svc.GetSubscriptionByID(context.Background(), subID)
	if err != nil {
		log.Printf("❌ HandleSubDetail: subscription not found: %v", err)
		return c.Send("❌ Подписка не найдена")
	}

	status := "✅ Активна"
	if sub.ExpiresAt.Before(time.Now()) || !sub.IsActive {
		status = "❌ Истекла"
	}

	text := fmt.Sprintf(`📦 *Подписка №%d* %s %s

%s
📅 До: *%s*

🔑 *Ключ:* (нажми кнопку ниже)`, sub.ID, sub.Product.CountryFlag, sub.Product.Name, status, sub.ExpiresAt.Format("02.01.2006 15:04"))

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("📋 Скопировать ключ", "copy_key", strconv.FormatInt(subID, 10))),
		menu.Row(
			menu.Data("🔄 Продлить", "extend", strconv.FormatInt(subID, 10)),
			menu.Data("📚 Инструкция", "instruction"),
		),
		menu.Row(menu.Data("⬅️ Назад", "mysubs")),
	)

	if UseBannerImages {
		photo := &tele.Photo{
			File:    tele.FromURL(MainBannerURL),
			Caption: text,
		}
		c.Delete()
		return c.Send(photo, menu, tele.ModeMarkdown)
	}

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// HandleCopyKey отправляет ключ отдельным сообщением для копирования
func (h *Handler) HandleCopyKey(c tele.Context) error {
	subID, _ := strconv.ParseInt(c.Callback().Data, 10, 64)

	sub, err := h.svc.GetSubscriptionByID(context.Background(), subID)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "❌ Ошибка"})
	}

	// Отправляем ключ отдельным сообщением для удобного копирования
	c.Send(fmt.Sprintf("`%s`", sub.KeyString), tele.ModeMarkdown)

	return c.Respond(&tele.CallbackResponse{Text: "✅ Ключ отправлен"})
}

// HandleExtend показывает варианты продления
func (h *Handler) HandleExtend(c tele.Context) error {
	log.Printf("👉 HandleExtend triggered for User: %d", c.Sender().ID)

	subID, _ := strconv.ParseInt(c.Callback().Data, 10, 64)

	sub, err := h.svc.GetSubscriptionByID(context.Background(), subID)
	if err != nil {
		return c.Send("❌ Подписка не найдена")
	}

	plans := h.svc.GetPricingPlans(sub.Product.BasePrice)

	text := fmt.Sprintf(`🔄 *Продление подписки №%d*

%s %s
📅 Текущий срок: до %s

Выберите период (+к текущему сроку):`, sub.ID, sub.Product.CountryFlag, sub.Product.Name, sub.ExpiresAt.Format("02.01.2006"))

	menu := &tele.ReplyMarkup{}
	var rows []tele.Row

	for _, plan := range plans {
		var btnText string
		if plan.Discount > 0 {
			btnText = fmt.Sprintf("%d мес. (-%d%%) — %d ₽", plan.Months, plan.Discount, int(plan.Price))
		} else {
			btnText = fmt.Sprintf("%d мес. — %d ₽", plan.Months, int(plan.Price))
		}
		btn := menu.Data(btnText, "extend_pay", fmt.Sprintf("%d:%d", subID, plan.Months))
		rows = append(rows, menu.Row(btn))
	}

	rows = append(rows, menu.Row(menu.Data("⬅️ Назад", "sub", strconv.FormatInt(subID, 10))))
	menu.Inline(rows...)

	if UseBannerImages {
		photo := &tele.Photo{
			File:    tele.FromURL(MainBannerURL),
			Caption: text,
		}
		c.Delete()
		return c.Send(photo, menu, tele.ModeMarkdown)
	}

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// HandleExtendPay обрабатывает оплату продления подписки
func (h *Handler) HandleExtendPay(c tele.Context) error {
	ctx := context.Background()

	parts := strings.Split(c.Callback().Data, ":")
	if len(parts) != 2 {
		return c.Send("❌ Ошибка")
	}

	subID, _ := strconv.ParseInt(parts[0], 10, 64)
	months, _ := strconv.Atoi(parts[1])

	sub, err := h.svc.GetSubscriptionByID(ctx, subID)
	if err != nil {
		return c.Send("❌ Подписка не найдена")
	}

	price, discount := h.svc.CalculatePrice(sub.Product.BasePrice, months)

	// Применяем флеш-скидку
	if flashSale.IsActive() {
		price = flashSale.ApplyDiscount(price)
	}

	user, err := h.svc.GetOrCreateUser(ctx, c.Sender().ID, c.Sender().Username)
	if err != nil {
		return c.Send("❌ Ошибка")
	}

	// Проверяем баланс
	if user.Balance < price {
		var discountText string
		if flashSale.IsActive() {
			discountText = fmt.Sprintf(" 🔥 *АКЦИЯ -%d%%!*", flashSale.GetDiscount())
		} else if discount > 0 {
			discountText = fmt.Sprintf(" (скидка %d%%)", discount)
		}

		text := fmt.Sprintf(`❌ *Недостаточно средств для продления*

💰 Ваш баланс: %.0f ₽
💸 Требуется: %.0f ₽%s
📉 Не хватает: %.0f ₽

Пополните баланс для продления подписки.`, user.Balance, price, discountText, price-user.Balance)

		menu := &tele.ReplyMarkup{}
		menu.Inline(
			menu.Row(menu.Data("💳 Пополнить баланс", "topup")),
			menu.Row(menu.Data("⬅️ Назад", "extend", strconv.FormatInt(subID, 10))),
		)

		return c.Edit(text, menu, tele.ModeMarkdown)
	}

	// Списываем баланс
	err = h.svc.DeductBalance(ctx, user.ID, price)
	if err != nil {
		return c.Send("❌ Ошибка списания баланса")
	}

	// Продлеваем подписку (кумулятивно)
	err = h.svc.ExtendSubscription(ctx, subID, months)
	if err != nil {
		// Возвращаем деньги при ошибке
		h.svc.AddUserBalance(ctx, user.TelegramID, price)
		return c.Send("❌ Ошибка продления подписки. Средства возвращены на баланс.")
	}

	// Получаем обновлённую подписку для отображения новой даты
	updatedSub, err := h.svc.GetSubscriptionByID(ctx, subID)
	if err != nil {
		updatedSub = sub // fallback
	}

	var discountText string
	if discount > 0 {
		discountText = fmt.Sprintf(" (скидка %d%%)", discount)
	}

	text := fmt.Sprintf(`✅ *Подписка продлена!*

%s *%s* №%d
📅 Добавлено: +%d мес.%s
⏰ Новый срок: до *%s*

🔑 *Ваш ключ не изменился:*
`+"`%s`"+`

_(Можете продолжать пользоваться)_`,
		sub.Product.CountryFlag, sub.Product.Name, sub.ID,
		months, discountText,
		updatedSub.ExpiresAt.Format("02.01.2006"),
		sub.KeyString)

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("🔑 Мои подписки", "mysubs")),
		menu.Row(menu.Data("🏠 Главное меню", "back_main")),
	)

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// ================= INSTRUCTIONS =================

// HandleInstruction показывает выбор устройства
func (h *Handler) HandleInstruction(c tele.Context) error {
	text := `📚 *Настройка подключения*

Рекомендуем приложение *Happ* — работает в один клик.

1. Установите Happ (ссылки ниже)
2. Скопируйте ключ (` + "`vless://...`" + `)
3. Откройте Happ — он сам добавит ключ
4. Нажмите *Подключиться*

👇 *Выберите устройство:*`

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(
			menu.Data("🤖 Android", "instr_android"),
			menu.Data("💻 Windows", "instr_windows"),
		),
		menu.Row(
			menu.Data("🍏 iOS", "instr_iphone"),
			menu.Data("🖥 Mac", "instr_mac"),
		),
		menu.Row(menu.Data("Вернуться", "back_main")),
	)

	if UseBannerImages {
		photo := &tele.Photo{
			File:    tele.FromURL(MainBannerURL),
			Caption: text,
		}
		c.Delete()
		return c.Send(photo, menu, tele.ModeMarkdown)
	}

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// HandleInstrAndroid инструкция для Android
func (h *Handler) HandleInstrAndroid(c tele.Context) error {
	text := `🤖 *Инструкция для Android:*

1. Скачайте приложение [Happ](https://play.google.com/store/apps/details?id=com.happproxy) из Google Play.
2. Скопируйте ключ подписки в буфер обмена.
3. Откройте Happ — приложение автоматически предложит добавить ключ из буфера.
4. Нажмите *Подключиться* — готово!`

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.URL("📥 Скачать Happ", "https://play.google.com/store/apps/details?id=com.happproxy")),
		menu.Row(menu.Data("⬅️ Назад", "instruction")),
	)

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// HandleInstrWindows инструкция для Windows
func (h *Handler) HandleInstrWindows(c tele.Context) error {
	text := `💻 *Инструкция для Windows:*

1. Скачайте и установите [Happ для Windows](https://github.com/Happ-proxy/happ-desktop/releases/latest/download/setup-Happ.x64.exe).
2. Скопируйте ключ подписки в буфер обмена.
3. Откройте Happ — приложение автоматически предложит добавить ключ из буфера.
4. Нажмите *Подключиться* — готово!`

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.URL("📥 Скачать Happ", "https://github.com/Happ-proxy/happ-desktop/releases/latest/download/setup-Happ.x64.exe")),
		menu.Row(menu.Data("⬅️ Назад", "instruction")),
	)

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// HandleInstrIphone инструкция для iPhone
func (h *Handler) HandleInstrIphone(c tele.Context) error {
	text := `🍏 *Инструкция для iOS (iPhone / iPad):*

1. Скачайте приложение [Happ](https://apps.apple.com/us/app/happ-proxy-utility/id6504287215) из App Store.
2. Скопируйте ключ подписки в буфер обмена.
3. Откройте Happ — приложение автоматически предложит добавить ключ из буфера.
4. Нажмите *Подключиться* — готово!`

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.URL("📥 Скачать Happ", "https://apps.apple.com/us/app/happ-proxy-utility/id6504287215")),
		menu.Row(menu.Data("⬅️ Назад", "instruction")),
	)

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// HandleInstrMac инструкция для Mac
func (h *Handler) HandleInstrMac(c tele.Context) error {
	text := `🖥 *Инструкция для Mac:*

1. Скачайте приложение [Happ](https://apps.apple.com/us/app/happ-proxy-utility/id6504287215) из App Store.
2. Скопируйте ключ подписки в буфер обмена.
3. Откройте Happ — приложение автоматически предложит добавить ключ из буфера.
4. Нажмите *Подключиться* — готово!`

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.URL("📥 Скачать Happ", "https://apps.apple.com/us/app/happ-proxy-utility/id6504287215")),
		menu.Row(menu.Data("⬅️ Назад", "instruction")),
	)

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// ================= HELP =================

// HandleHelp показывает раздел помощи
func (h *Handler) HandleHelp(c tele.Context) error {
	log.Printf("👉 HandleHelp triggered for User: %d", c.Sender().ID)

	text := `🛟 *Помощь*

Выберите интересующий раздел:`

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("⁉️ Часто задаваемые вопросы", "faq")),
		menu.Row(menu.Data("🛟 Поддержка", "support")),
		menu.Row(menu.Data("📄 Пользовательское соглашение", "privacy")),
		menu.Row(menu.Data("⬅️ Назад", "back_main")),
	)

	if UseBannerImages {
		photo := &tele.Photo{
			File:    tele.FromURL(MainBannerURL),
			Caption: text,
		}
		c.Delete()
		return c.Send(photo, menu, tele.ModeMarkdown)
	}

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// HandleFAQ показывает FAQ
func (h *Handler) HandleFAQ(c tele.Context) error {
	text := `⁉️ *Часто задаваемые вопросы*

🛠 *Что делать, если VPN не работает?*
Первым делом попробуйте перезагрузить устройство или переподключиться в приложении. Если проблема осталась — нажмите кнопку *«🛟 Поддержка»* ниже. Мы поможем!

📱 *Сколько устройств можно подключить?*
Один ключ доступа работает одновременно на *3-х устройствах*. Вы можете защитить телефон, компьютер и планшет одной подпиской.

💳 *Как можно оплатить?*
Мы принимаем всё: Банковские карты РФ, СБП (Система Быстрых Платежей) и Криптовалюту.

🎁 *Как пользоваться бесплатно?*
У нас работает щедрая реферальная программа!
• Вы получаете *25%* на баланс с каждой оплаты приглашенного друга.
• Пригласи *4-х друзей* — и твой VPN будет оплачиваться их бонусами. Пользуйся бесплатно!`

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("🛟 Поддержка", "support")),
		menu.Row(menu.Data("⬅️ Назад", "help")),
	)

	if UseBannerImages {
		photo := &tele.Photo{
			File:    tele.FromURL(MainBannerURL),
			Caption: text,
		}
		c.Delete()
		return c.Send(photo, menu, tele.ModeMarkdown)
	}

	return c.Edit(text, menu, tele.ModeMarkdown)
}


// HandleSupportHub показывает центр тикетов (Support Hub)
func (h *Handler) HandleSupportHub(c tele.Context) error {
	// Respond to callback to stop loading animation
	if c.Callback() != nil {
		c.Respond()
	}

	text := `🛟 *Поддержка*

Это центр тикетов: создавайте обращения, просматривайте ответы и историю.

• *Создать тикет* — опишите проблему или вопрос.
• *Мои тикеты* — статус и переписка.

_Старайтесь использовать тикеты — так мы быстрее поможем и ничего не потеряется._`

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("🎫 Создать тикет", "ticket_create")),
		menu.Row(menu.Data("📋 Мои тикеты", "ticket_list")),
		menu.Row(menu.Data("⬅️ Назад", "help")),
	)

	if UseBannerImages {
		photo := &tele.Photo{
			File:    tele.FromURL(MainBannerURL),
			Caption: text,
		}
		if c.Callback() != nil {
			c.Delete()
		}
		return c.Send(photo, menu, tele.ModeMarkdown)
	}

	if c.Callback() != nil {
		return c.Edit(text, menu, tele.ModeMarkdown)
	}
	return c.Send(text, menu, tele.ModeMarkdown)
}

// HandleCreateTicket начинает создание тикета (включает режим поддержки)
func (h *Handler) HandleCreateTicket(c tele.Context) error {
	if c.Callback() != nil {
		c.Respond()
	}

	// Включаем режим поддержки
	SetUserSupportMode(c.Sender().ID, true)
	log.Printf("🎫 Support mode ENABLED for user %d", c.Sender().ID)

	text := `✍️ *Новое обращение*

Пожалуйста, опишите вашу проблему одним сообщением.
Вы можете прикрепить скриншот или фото чека.

*Оператор ответит вам в этом чате.*`

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("🚫 Отмена", "back_to_support_hub")),
	)

	if UseBannerImages {
		photo := &tele.Photo{
			File:    tele.FromURL(MainBannerURL),
			Caption: text,
		}
		// Сначала отправляем новое сообщение, потом удаляем старое
		_, err := c.Bot().Send(c.Chat(), photo, menu, tele.ModeMarkdown)
		if err != nil {
			log.Printf("HandleCreateTicket: Failed to send photo: %v", err)
			return c.Send(text, menu, tele.ModeMarkdown)
		}
		if c.Callback() != nil {
			c.Delete()
		}
		return nil
	}

	if c.Callback() != nil {
		return c.Edit(text, menu, tele.ModeMarkdown)
	}
	return c.Send(text, menu, tele.ModeMarkdown)
}

// HandleBackToSupportHub возвращает в центр тикетов и сбрасывает состояние
func (h *Handler) HandleBackToSupportHub(c tele.Context) error {
	if c.Callback() != nil {
		c.Respond()
	}

	// Сбрасываем режим поддержки
	SetUserSupportMode(c.Sender().ID, false)

	// Возвращаем в центр тикетов
	return h.HandleSupportHub(c)
}

// HandleMyTickets показывает список тикетов пользователя
func (h *Handler) HandleMyTickets(c tele.Context) error {
	if c.Callback() != nil {
		c.Respond()
	}

	userID := c.Sender().ID
	var text string
	menu := &tele.ReplyMarkup{}

	// Проверяем есть ли активный тикет (пользователь в режиме поддержки)
	if IsUserInSupportMode(userID) {
		// Сценарий A: Есть активный диалог
		text = `📂 *Мои обращения*

🟢 *Активный диалог*
⚡️ **Статус:** Переписка открыта

Вы можете просто писать сообщения в этот чат — они автоматически попадут в поддержку.`

		menu.Inline(
			menu.Row(menu.Data("✏️ Написать сообщение", "ticket_reply")),
			menu.Row(menu.Data("✅ Вопрос решён", "ticket_solve")),
			menu.Row(menu.Data("⬅️ Назад", "back_to_support_hub")),
		)
	} else {
		// Сценарий B: Нет активных обращений
		text = `📂 *Мои обращения*

У вас сейчас нет открытых запросов.
Если возникла проблема — создайте новый тикет.

_Ответы от поддержки приходят прямо в этот чат._`

		menu.Inline(
			menu.Row(menu.Data("🎫 Создать тикет", "ticket_create")),
			menu.Row(menu.Data("⬅️ Назад", "back_to_support_hub")),
		)
	}

	if UseBannerImages {
		photo := &tele.Photo{
			File:    tele.FromURL(MainBannerURL),
			Caption: text,
		}
		if c.Callback() != nil {
			c.Delete()
		}
		return c.Send(photo, menu, tele.ModeMarkdown)
	}

	if c.Callback() != nil {
		return c.Edit(text, menu, tele.ModeMarkdown)
	}
	return c.Send(text, menu, tele.ModeMarkdown)
}

// HandleExitSupport выходит из режима поддержки и возвращает в центр тикетов
func (h *Handler) HandleExitSupport(c tele.Context) error {
	if c.Callback() != nil {
		c.Respond()
	}

	// Выключаем режим поддержки
	SetUserSupportMode(c.Sender().ID, false)

	// Возвращаем в центр тикетов
	return h.HandleSupportHub(c)
}

// HandleTicketReply позволяет пользователю ответить на тикет
func (h *Handler) HandleTicketReply(c tele.Context) error {
	// Acknowledge callback (остановить loading на кнопке)
	if c.Callback() != nil {
		c.Respond()
	}

	// Включаем режим поддержки для продолжения диалога
	SetUserSupportMode(c.Sender().ID, true)
	log.Printf("🎫 Support mode ENABLED for reply, user %d", c.Sender().ID)

	// ВАЖНО: Используем Send, а не Edit — чтобы сохранить историю чата!
	text := `✍️ *Продолжение диалога*

Напишите ваш ответ оператору.
Можете прикрепить фото, видео или документ.`

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("🚫 Отмена", "ticket_cancel_reply")),
	)

	// Всегда Send — не удаляем сообщение админа
	return c.Send(text, menu, tele.ModeMarkdown)
}

// HandleTicketSolve закрывает тикет (вопрос решен)
func (h *Handler) HandleTicketSolve(c tele.Context) error {
	if c.Callback() != nil {
		c.Respond()
	}

	userID := c.Sender().ID
	username := c.Sender().Username

	// Выключаем режим поддержки
	SetUserSupportMode(userID, false)

	// Удаляем из трекера и обновляем dashboard
	if tracker := GetTracker(); tracker != nil {
		tracker.RemoveTicket(userID)
		go tracker.UpdateDashboard()
	}

	// Уведомляем админов в группе поддержки
	usernameStr := "нет"
	if username != "" {
		usernameStr = "@" + username
	}

	adminNotification := fmt.Sprintf("✅ *Тикет закрыт пользователем*\n\n👤 %s\n🆔 `#user_%d`\n\n_Диалог завершён._", usernameStr, userID)
	supportGroup := &tele.Chat{ID: h.supportGroupID}
	_, err := c.Bot().Send(supportGroup, adminNotification, tele.ModeMarkdown)
	if err != nil {
		log.Printf("HandleTicketSolve: Failed to notify admin group: %v", err)
	}

	// Сообщение пользователю
	text := `✅ *Тикет закрыт*

Спасибо за обращение!
Если у вас снова возникнут вопросы — мы всегда на связи.`

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("🏠 Главное меню", "back_main")),
		menu.Row(menu.Data("🛟 Поддержка", "support")),
	)

	if c.Callback() != nil {
		return c.Edit(text, menu, tele.ModeMarkdown)
	}
	return c.Send(text, menu, tele.ModeMarkdown)
}

// HandleTicketCancelReply отменяет ответ и возвращает в обычный режим
func (h *Handler) HandleTicketCancelReply(c tele.Context) error {
	if c.Callback() != nil {
		c.Respond()
	}

	// Выключаем режим поддержки
	SetUserSupportMode(c.Sender().ID, false)

	text := `ℹ️ Ответ отменён.

Если вам ответят — вы получите уведомление.`

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("🏠 Главное меню", "back_main")),
	)

	if c.Callback() != nil {
		return c.Edit(text, menu, tele.ModeMarkdown)
	}
	return c.Send(text, menu, tele.ModeMarkdown)
}

// HandlePrivacy показывает пользовательское соглашение
func (h *Handler) HandlePrivacy(c tele.Context) error {
	text := `📄 *Пользовательское соглашение*

Публичная оферта на заключение лицензионного договора.`

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.URL("📖 Читать соглашение", "https://telegra.ph/Publichnaya-oferta-na-zaklyuchenie-licenzionnogo-dogovora-dlya-ispolzovaniya-VPN-servisa-06-14")),
		menu.Row(menu.Data("⬅️ Назад", "help")),
	)

	if UseBannerImages {
		photo := &tele.Photo{
			File:    tele.FromURL(MainBannerURL),
			Caption: text,
		}
		c.Delete()
		return c.Send(photo, menu, tele.ModeMarkdown)
	}

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// ================= BALANCE & PROMO =================

// HandleBalance показывает баланс пользователя
func (h *Handler) HandleBalance(c tele.Context) error {
	ctx := context.Background()
	user, err := h.svc.GetOrCreateUser(ctx, c.Sender().ID, c.Sender().Username)
	if err != nil {
		return c.Send("❌ Ошибка загрузки данных")
	}

	text := fmt.Sprintf(`💰 *Ваш кошелёк*

🆔 ID: `+"`%d`"+`
💵 *Текущий баланс:* *%.0f ₽*

ℹ️ Баланс можно использовать для оплаты подписок и продлений.`, user.TelegramID, user.Balance)

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("💳 Пополнить баланс", "topup")),
		menu.Row(menu.Data("⬅️ Назад", "back_main")),
	)

	if UseBannerImages {
		photo := &tele.Photo{
			File:    tele.FromURL(MainBannerURL),
			Caption: text,
		}
		c.Delete()
		return c.Send(photo, menu, tele.ModeMarkdown)
	}

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// HandlePromoEnter показывает экран ввода промокода
func (h *Handler) HandlePromoEnter(c tele.Context) error {
	// Устанавливаем режим ввода промокода
	SetUserPromoMode(c.Sender().ID, true)

	text := `🎟 *Активация промокода*

Введите ваш промокод в чат, чтобы получить бонус на баланс.

💡 *Где взять промокод?*
Мы регулярно публикуем их в нашем *Канале*, *Чате*, а также отправляем активным пользователям прямо здесь, в *боте*.`

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.URL("📢 Наш канал", "https://t.me/XRAY_MODE")),
		menu.Row(menu.URL("💬 Наш чат", "https://t.me/XRAY_LUV")),
		menu.Row(menu.Data("⬅️ Назад", "back_main")),
	)

	if UseBannerImages {
		photo := &tele.Photo{
			File:    tele.FromURL(MainBannerURL),
			Caption: text,
		}
		c.Delete()
		return c.Send(photo, menu, tele.ModeMarkdown)
	}

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// HandleTopUp показывает варианты пополнения баланса
func (h *Handler) HandleTopUp(c tele.Context) error {
	text := `💳 *Пополнение кошелька*

Выберите сумму пополнения.

Средства зачисляются на ваш внутренний баланс. Вы сможете использовать их для оплаты подписки в любой момент.`

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(
			menu.Data("450 ₽", "topup_amount", "450"),
			menu.Data("1350 ₽", "topup_amount", "1350"),
		),
		menu.Row(
			menu.Data("2430 ₽", "topup_amount", "2430"),
			menu.Data("4320 ₽", "topup_amount", "4320"),
		),
		menu.Row(menu.Data("⬅️ Назад", "balance")),
	)

	if UseBannerImages {
		photo := &tele.Photo{
			File:    tele.FromURL(MainBannerURL),
			Caption: text,
		}
		c.Delete()
		return c.Send(photo, menu, tele.ModeMarkdown)
	}

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// HandleTopUpAmount обрабатывает выбор суммы пополнения
func (h *Handler) HandleTopUpAmount(c tele.Context) error {
	amount, err := strconv.ParseFloat(c.Callback().Data, 64)
	if err != nil {
		return c.Send("❌ Некорректная сумма")
	}

	text := fmt.Sprintf(`💳 *Счёт на оплату*
—————————————————
💰 *Назначение:* Пополнение баланса
💵 *Сумма:* *%.0f ₽*

🎁 *БОНУС: +7 ДНЕЙ В ПОДАРОК!*
При оплате *Криптовалютой* (USDT, TON, BTC) вы получите бонусные дни при покупке подписки.
✅ _Бонус начислится сразу после оплаты._

👇 *Выберите способ оплаты:*`, amount)

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("💠 СБП (Быстрый платёж)", "topup_pay_card", fmt.Sprintf("%.0f", amount))),
		menu.Row(menu.Data("🌑 Криптовалюта (+7 дней 🎁)", "topup_pay_crypto", fmt.Sprintf("%.0f", amount))),
		menu.Row(menu.Data("⬅️ Назад", "topup")),
	)

	if UseBannerImages {
		photo := &tele.Photo{
			File:    tele.FromURL(MainBannerURL),
			Caption: text,
		}
		c.Delete()
		return c.Send(photo, menu, tele.ModeMarkdown)
	}

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// HandleTopUpPayCard обработка оплаты пополнения через СБП
func (h *Handler) HandleTopUpPayCard(c tele.Context) error {
	amount := c.Callback().Data

	text := fmt.Sprintf(`💠 *Оплата через СБП*

💵 Сумма: *%s ₽*

Для оплаты напишите в поддержку — мы отправим реквизиты для перевода.

После оплаты отправьте чек/скриншот в поддержку, и баланс будет пополнен в течение 15 минут.`, amount)

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.URL("📥 Написать в поддержку", "https://t.me/XRAY_LUV")),
		menu.Row(menu.Data("⬅️ Назад", "topup_amount", amount)),
	)

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// HandleTopUpPayCrypto обработка оплаты пополнения криптой
func (h *Handler) HandleTopUpPayCrypto(c tele.Context) error {
	amount := c.Callback().Data

	text := fmt.Sprintf(`🌑 *Оплата криптовалютой*

💵 Сумма: *%s ₽*
🎁 Бонус: *+7 дней* к подписке!

Для оплаты напишите в поддержку — мы отправим адрес кошелька (USDT, TON, BTC).

После оплаты отправьте хэш транзакции в поддержку, и баланс будет пополнен в течение 15 минут.`, amount)

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.URL("📥 Написать в поддержку", "https://t.me/XRAY_LUV")),
		menu.Row(menu.Data("⬅️ Назад", "topup_amount", amount)),
	)

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// HandlePayWithBalance оплата с баланса
func (h *Handler) HandlePayWithBalance(c tele.Context) error {
	ctx := context.Background()

	parts := strings.Split(c.Callback().Data, ":")
	if len(parts) != 2 {
		return c.Send("❌ Ошибка")
	}

	productID, _ := strconv.ParseInt(parts[0], 10, 64)
	months, _ := strconv.Atoi(parts[1])

	product, err := h.svc.GetProductByID(ctx, productID)
	if err != nil {
		return c.Send("❌ Продукт не найден")
	}

	price, _ := h.svc.CalculatePrice(product.BasePrice, months)

	// Применяем флеш-скидку
	if flashSale.IsActive() {
		price = flashSale.ApplyDiscount(price)
	}

	user, err := h.svc.GetOrCreateUser(ctx, c.Sender().ID, c.Sender().Username)
	if err != nil {
		return c.Send("❌ Ошибка")
	}

	// Проверяем баланс
	if user.Balance < price {
		text := fmt.Sprintf(`❌ *Недостаточно средств*

💰 Ваш баланс: %.0f ₽
💸 Требуется: %.0f ₽
📉 Не хватает: %.0f ₽

Пополните баланс для оформления подписки.`, user.Balance, price, price-user.Balance)

		menu := &tele.ReplyMarkup{}
		menu.Inline(
			menu.Row(menu.Data("💳 Пополнить баланс", "topup")),
			menu.Row(menu.Data("⬅️ Назад", "tariffs")),
		)

		return c.Edit(text, menu, tele.ModeMarkdown)
	}

	// Списываем баланс
	err = h.svc.DeductBalance(ctx, user.ID, price)
	if err != nil {
		return c.Send("❌ Ошибка списания баланса")
	}

	// Создаём подписку
	expiresAt := time.Now().AddDate(0, months, 0)
	sub, err := h.svc.CreateSubscriptionSimple(ctx, user.ID, productID, expiresAt)
	if err != nil {
		// Возвращаем деньги при ошибке
		h.svc.AddUserBalance(ctx, user.TelegramID, price)
		return c.Send("❌ Ошибка создания подписки. Средства возвращены на баланс.")
	}

	text := fmt.Sprintf(`✅ *Подписка активирована!*

%s *%s*
📅 Срок: %d мес.
⏰ Действует до: %s

🔑 *Ваш ключ:*
`+"`%s`"+`

_(Нажмите на ключ, чтобы скопировать)_

Перейдите в раздел «📚 Инструкция» для настройки.`,
		product.CountryFlag, product.Name, months,
		expiresAt.Format("02.01.2006"),
		sub.KeyString)

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("📚 Инструкция", "instruction")),
		menu.Row(menu.Data("🔑 Мои подписки", "mysubs")),
		menu.Row(menu.Data("🏠 Главное меню", "back_main")),
	)

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// ================= REFERRAL SYSTEM =================

// HandleRefSystem показывает партнёрскую программу
func (h *Handler) HandleRefSystem(c tele.Context) error {
	ctx := context.Background()
	user, err := h.svc.GetOrCreateUser(ctx, c.Sender().ID, c.Sender().Username)
	if err != nil {
		return c.Send("❌ Ошибка получения данных")
	}

	// Получаем количество рефералов
	refCount, _ := h.svc.GetReferralCount(ctx, c.Sender().ID)

	// Получаем username бота
	botUsername := c.Bot().Me.Username
	refLink := fmt.Sprintf("https://t.me/%s?start=%d", botUsername, c.Sender().ID)

	text := fmt.Sprintf(`👥 *Партнёрская программа*

📊 *Ваша статистика:*
• Приглашено друзей: *%d*
• Заработано всего: *%.0f ₽*

💰 *Условия:*
• Вы получаете *25%%* с каждого пополнения друга сразу на баланс.
• Друг получает *+3 дня* к подписке при первой покупке.

🔗 *Ваша пригласительная ссылка:*
`+"`%s`", refCount, user.TotalRefEarnings, refLink)

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("👥 Мои рефералы", "ref_list")),
		menu.Row(menu.Data("⬅️ Назад", "back_main")),
	)

	if UseBannerImages {
		photo := &tele.Photo{
			File:    tele.FromURL(MainBannerURL),
			Caption: text,
		}
		c.Delete()
		return c.Send(photo, menu, tele.ModeMarkdown)
	}

	return c.Edit(text, menu, tele.ModeMarkdown)
}

// HandleRefList показывает список рефералов с пагинацией
func (h *Handler) HandleRefList(c tele.Context) error {
	ctx := context.Background()

	// Определяем номер страницы
	page := 1
	if c.Callback() != nil && c.Callback().Data != "" {
		// Формат: "1" или пустая строка для первой страницы
		if p, err := strconv.Atoi(c.Callback().Data); err == nil && p > 0 {
			page = p
		}
	}

	// Получаем рефералов с пагинацией
	result, err := h.svc.GetReferralsPaginated(ctx, c.Sender().ID, page)
	if err != nil {
		return c.Send("❌ Ошибка загрузки рефералов")
	}

	// Если рефералов нет
	if result.TotalCount == 0 {
		text := `👥 *Ваши рефералы*

У вас пока нет приглашённых друзей.

🔗 Поделитесь своей ссылкой и получайте *25%* с каждого пополнения друга!`

		menu := &tele.ReplyMarkup{}
		menu.Inline(
			menu.Row(menu.Data("⬅️ Назад", "ref_system")),
		)

		if UseBannerImages {
			photo := &tele.Photo{
				File:    tele.FromURL(MainBannerURL),
				Caption: text,
			}
			c.Delete()
			return c.Send(photo, menu, tele.ModeMarkdown)
		}

		return c.Edit(text, menu, tele.ModeMarkdown)
	}

	// Формируем текст со списком рефералов
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("👥 *Ваши рефералы*\n_Страница %d из %d_\n\n", result.CurrentPage, result.TotalPages))

	for i, ref := range result.Referrals {
		position := (result.CurrentPage-1)*10 + i + 1

		// Медаль для топ-3
		var medal string
		switch position {
		case 1:
			medal = "🥇 "
		case 2:
			medal = "🥈 "
		case 3:
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

		sb.WriteString(fmt.Sprintf("%d. %s*%s* — принёс: *%.0f ₽*\n",
			position, medal, username, ref.GeneratedRevenue))
		sb.WriteString(fmt.Sprintf("   _(Регистрация: %s)_\n", ref.JoinedAt.Format("02.01.2006")))
	}

	sb.WriteString(fmt.Sprintf("\n📊 *Всего рефералов:* %d чел.\n", result.TotalCount))
	sb.WriteString(fmt.Sprintf("💰 *Общий доход:* %.0f ₽", result.TotalEarnings))

	// Формируем клавиатуру с пагинацией
	menu := &tele.ReplyMarkup{}
	var navRow []tele.Btn

	// Кнопка "назад" по страницам
	if result.CurrentPage > 1 {
		navRow = append(navRow, menu.Data("⬅️ Туда", "ref_list", strconv.Itoa(result.CurrentPage-1)))
	}

	// Индикатор страницы (пассивная кнопка)
	if result.TotalPages > 1 {
		navRow = append(navRow, menu.Data(fmt.Sprintf("📄 %d/%d", result.CurrentPage, result.TotalPages), "ref_list", strconv.Itoa(result.CurrentPage)))
	}

	// Кнопка "вперёд" по страницам
	if result.CurrentPage < result.TotalPages {
		navRow = append(navRow, menu.Data("Сюда ➡️", "ref_list", strconv.Itoa(result.CurrentPage+1)))
	}

	var rows []tele.Row
	if len(navRow) > 0 {
		rows = append(rows, menu.Row(navRow...))
	}
	rows = append(rows, menu.Row(menu.Data("⬅️ Назад", "ref_system")))
	menu.Inline(rows...)

	if UseBannerImages {
		photo := &tele.Photo{
			File:    tele.FromURL(MainBannerURL),
			Caption: sb.String(),
		}
		c.Delete()
		return c.Send(photo, menu, tele.ModeMarkdown)
	}

	return c.Edit(sb.String(), menu, tele.ModeMarkdown)
}
