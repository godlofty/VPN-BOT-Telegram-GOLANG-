package handlers

import (
	"fmt"
	"sort"
	"sync"
	"time"

	tele "gopkg.in/telebot.v3"
)

// TicketStatus статус тикета
type TicketStatus string

const (
	StatusWaiting  TicketStatus = "waiting"  // Ожидает ответа админа
	StatusReplied  TicketStatus = "replied"  // Админ ответил
)

// ActiveTicket информация об активном тикете
type ActiveTicket struct {
	UserID           int64
	Username         string
	LastMessageTime  time.Time
	Status           TicketStatus
	GroupMessageID   int    // ID сообщения в группе (для ссылки)
	MessageCount     int    // Количество сообщений от пользователя
}

// SupportTracker трекер активных тикетов
type SupportTracker struct {
	mu               sync.RWMutex
	tickets          map[int64]*ActiveTicket // userID -> ticket
	dashboardMsgID   int                     // ID закреплённого сообщения dashboard
	supportGroupID   int64
	bot              *tele.Bot
}

var tracker *SupportTracker

// InitSupportTracker инициализирует трекер
func InitSupportTracker(bot *tele.Bot, supportGroupID int64) {
	tracker = &SupportTracker{
		tickets:        make(map[int64]*ActiveTicket),
		supportGroupID: supportGroupID,
		bot:            bot,
	}
}

// GetTracker возвращает глобальный трекер
func GetTracker() *SupportTracker {
	return tracker
}

// SetDashboardMessageID устанавливает ID сообщения dashboard
func (t *SupportTracker) SetDashboardMessageID(msgID int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.dashboardMsgID = msgID
}

// GetDashboardMessageID возвращает ID сообщения dashboard
func (t *SupportTracker) GetDashboardMessageID() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.dashboardMsgID
}

// AddOrUpdateTicket добавляет или обновляет тикет
func (t *SupportTracker) AddOrUpdateTicket(userID int64, username string, groupMsgID int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if ticket, exists := t.tickets[userID]; exists {
		ticket.LastMessageTime = time.Now()
		ticket.Status = StatusWaiting
		ticket.MessageCount++
		if groupMsgID > 0 {
			ticket.GroupMessageID = groupMsgID
		}
	} else {
		t.tickets[userID] = &ActiveTicket{
			UserID:          userID,
			Username:        username,
			LastMessageTime: time.Now(),
			Status:          StatusWaiting,
			GroupMessageID:  groupMsgID,
			MessageCount:    1,
		}
	}
}

// SetTicketReplied помечает тикет как "отвечено"
func (t *SupportTracker) SetTicketReplied(userID int64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if ticket, exists := t.tickets[userID]; exists {
		ticket.Status = StatusReplied
	}
}

// RemoveTicket удаляет тикет (закрыт)
func (t *SupportTracker) RemoveTicket(userID int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.tickets, userID)
}

// GetAllTickets возвращает все активные тикеты
func (t *SupportTracker) GetAllTickets() []*ActiveTicket {
	t.mu.RLock()
	defer t.mu.RUnlock()

	tickets := make([]*ActiveTicket, 0, len(t.tickets))
	for _, ticket := range t.tickets {
		tickets = append(tickets, ticket)
	}

	// Сортируем: сначала waiting, потом по времени (старые сверху)
	sort.Slice(tickets, func(i, j int) bool {
		if tickets[i].Status != tickets[j].Status {
			return tickets[i].Status == StatusWaiting
		}
		return tickets[i].LastMessageTime.Before(tickets[j].LastMessageTime)
	})

	return tickets
}

// GetWaitingCount возвращает количество ожидающих ответа
func (t *SupportTracker) GetWaitingCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	count := 0
	for _, ticket := range t.tickets {
		if ticket.Status == StatusWaiting {
			count++
		}
	}
	return count
}

// UpdateDashboard обновляет закреплённое сообщение dashboard
func (t *SupportTracker) UpdateDashboard() {
	if t.bot == nil || t.dashboardMsgID == 0 {
		return
	}

	tickets := t.GetAllTickets()
	waitingCount := t.GetWaitingCount()
	totalCount := len(tickets)

	// Формируем текст dashboard
	var text string
	if totalCount == 0 {
		text = `📊 *Панель управления поддержкой*

✅ *Нет активных обращений*

_Все тикеты обработаны!_`
	} else {
		text = fmt.Sprintf(`📊 *Панель управления поддержкой*

🔥 *Ожидают ответа:* %d
💬 *Всего открыто:* %d

👇 *Список обращений:*

`, waitingCount, totalCount)

		// Извлекаем ID группы без префикса -100 для ссылки
		groupIDForLink := t.supportGroupID
		if groupIDForLink < 0 {
			// -1001234567890 -> 1234567890
			groupIDForLink = -groupIDForLink
			if groupIDForLink > 1000000000000 {
				groupIDForLink = groupIDForLink - 1000000000000
			}
		}

		for i, ticket := range tickets {
			if i >= 15 { // Лимит 15 тикетов в списке
				text += fmt.Sprintf("\n_... и ещё %d обращений_", totalCount-15)
				break
			}

			// Статус эмодзи
			statusEmoji := "🟢"
			statusText := "✅ Отвечено"
			if ticket.Status == StatusWaiting {
				statusEmoji = "🔴"
				waitTime := time.Since(ticket.LastMessageTime)
				if waitTime < time.Minute {
					statusText = "⏳ Только что"
				} else if waitTime < time.Hour {
					statusText = fmt.Sprintf("⏳ Ждет: %d мин", int(waitTime.Minutes()))
				} else {
					statusText = fmt.Sprintf("⏳ Ждет: %dч %dм", int(waitTime.Hours()), int(waitTime.Minutes())%60)
				}
			}

			// Username
			usernameStr := fmt.Sprintf("ID:%d", ticket.UserID)
			if ticket.Username != "" {
				usernameStr = "@" + ticket.Username
			}

			// Ссылка на сообщение (если есть)
			linkText := ""
			if ticket.GroupMessageID > 0 {
				linkText = fmt.Sprintf(" | [↗️ К диалогу](https://t.me/c/%d/%d)", groupIDForLink, ticket.GroupMessageID)
			}

			text += fmt.Sprintf("%d. %s *%s*\n   %s%s\n\n", i+1, statusEmoji, usernameStr, statusText, linkText)
		}
	}

	// Обновляем сообщение
	msg := &tele.Message{
		ID:   t.dashboardMsgID,
		Chat: &tele.Chat{ID: t.supportGroupID},
	}

	_, err := t.bot.Edit(msg, text, tele.ModeMarkdown, tele.NoPreview)
	if err != nil {
		// Если не удалось отредактировать - возможно сообщение удалено
		// log.Printf("Failed to update dashboard: %v", err)
	}
}

