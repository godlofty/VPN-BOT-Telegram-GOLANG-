package service

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	tele "gopkg.in/telebot.v3"
)

// WatchdogConfig конфигурация для Watchdog
type WatchdogConfig struct {
	CheckInterval    time.Duration
	CPUThreshold     float64 // процент CPU для алерта
	NetworkThreshold float64 // Mbps для алерта
	AlertCooldown    time.Duration
}

// DefaultWatchdogConfig возвращает конфигурацию по умолчанию
func DefaultWatchdogConfig() WatchdogConfig {
	return WatchdogConfig{
		CheckInterval:    30 * time.Second,
		CPUThreshold:     85.0,
		NetworkThreshold: 400.0, // 400 Mbps
		AlertCooldown:    5 * time.Minute,
	}
}

// Watchdog сервис мониторинга нагрузки
type Watchdog struct {
	bot      *tele.Bot
	adminIDs []int64
	vpn      VPNProvider
	config   WatchdogConfig

	mu            sync.Mutex
	lastAlertTime time.Time
	isRunning     bool
	stopChan      chan struct{}
}

// NewWatchdog создаёт новый Watchdog
func NewWatchdog(bot *tele.Bot, adminIDs []int64, vpn VPNProvider, config WatchdogConfig) *Watchdog {
	return &Watchdog{
		bot:      bot,
		adminIDs: adminIDs,
		vpn:      vpn,
		config:   config,
		stopChan: make(chan struct{}),
	}
}

// Start запускает мониторинг
func (w *Watchdog) Start() {
	w.mu.Lock()
	if w.isRunning {
		w.mu.Unlock()
		return
	}
	w.isRunning = true
	w.mu.Unlock()

	log.Println("🐕 Watchdog started")

	go w.runLoop()
}

// Stop останавливает мониторинг
func (w *Watchdog) Stop() {
	w.mu.Lock()
	if !w.isRunning {
		w.mu.Unlock()
		return
	}
	w.isRunning = false
	w.mu.Unlock()

	close(w.stopChan)
	log.Println("🐕 Watchdog stopped")
}

func (w *Watchdog) runLoop() {
	ticker := time.NewTicker(w.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopChan:
			return
		case <-ticker.C:
			w.checkSystem()
		}
	}
}

func (w *Watchdog) checkSystem() {
	ctx := context.Background()

	stats, err := w.vpn.GetSystemStats(ctx)
	if err != nil {
		log.Printf("Watchdog: failed to get system stats: %v", err)
		return
	}

	// Проверяем пороговые значения
	cpuAlert := stats.CPUPercent >= w.config.CPUThreshold
	networkAlert := stats.NetworkRxMbps >= w.config.NetworkThreshold

	if !cpuAlert && !networkAlert {
		return
	}

	// Проверяем cooldown
	w.mu.Lock()
	if time.Since(w.lastAlertTime) < w.config.AlertCooldown {
		w.mu.Unlock()
		return
	}
	w.lastAlertTime = time.Now()
	w.mu.Unlock()

	// Отправляем алерт
	w.sendAlert(ctx, stats, cpuAlert, networkAlert)
}

func (w *Watchdog) sendAlert(ctx context.Context, stats *SystemStats, cpuAlert, networkAlert bool) {
	// Получаем топ пользователей
	topUsers, err := w.getTopUsers(ctx, 3)
	if err != nil {
		log.Printf("Watchdog: failed to get top users: %v", err)
	}

	// Формируем сообщение
	message := w.formatAlertMessage(stats, cpuAlert, networkAlert, topUsers)

	// Отправляем всем админам
	for _, adminID := range w.adminIDs {
		_, err := w.bot.Send(&tele.User{ID: adminID}, message, tele.ModeMarkdown)
		if err != nil {
			log.Printf("Watchdog: failed to send alert to admin %d: %v", adminID, err)
		}
	}

	log.Printf("🚨 Watchdog alert sent: CPU=%.1f%%, Network=%.1f Mbps", stats.CPUPercent, stats.NetworkRxMbps)
}

func (w *Watchdog) getTopUsers(ctx context.Context, limit int) ([]VPNUser, error) {
	users, err := w.vpn.GetAllUsers(ctx)
	if err != nil {
		return nil, err
	}

	// Фильтруем только активных
	var activeUsers []VPNUser
	for _, u := range users {
		if u.IsActive {
			activeUsers = append(activeUsers, u)
		}
	}

	// Сортируем по использованному трафику (убывание)
	sort.Slice(activeUsers, func(i, j int) bool {
		return activeUsers[i].UsedTraffic > activeUsers[j].UsedTraffic
	})

	// Возвращаем топ N
	if len(activeUsers) > limit {
		return activeUsers[:limit], nil
	}
	return activeUsers, nil
}

func (w *Watchdog) formatAlertMessage(stats *SystemStats, cpuAlert, networkAlert bool, topUsers []VPNUser) string {
	// CPU статус
	cpuStatus := "🟢"
	if stats.CPUPercent >= 90 {
		cpuStatus = "🔴"
	} else if stats.CPUPercent >= 70 {
		cpuStatus = "🟡"
	}

	// Network статус
	networkStatus := ""
	if stats.NetworkRxMbps >= 300 {
		networkStatus = "🚀"
	}

	msg := fmt.Sprintf(`☠️ *DDoS / HIGH LOAD ALERT*

⚠️ *Anomaly Detected!*
📉 *CPU:* %.1f%% %s
📶 *Network RX:* %.0f Mbps %s
💾 *Memory:* %.1f%%

👥 *Active Users:* %d / %d`,
		stats.CPUPercent, cpuStatus,
		stats.NetworkRxMbps, networkStatus,
		stats.MemoryPercent,
		stats.ActiveUsers, stats.TotalUsers,
	)

	// Добавляем топ пользователей
	if len(topUsers) > 0 {
		msg += "\n\n👮‍♂️ *Top Active Users (Potential Suspects):*"
		for i, user := range topUsers {
			trafficGB := float64(user.UsedTraffic) / (1024 * 1024 * 1024)
			msg += fmt.Sprintf("\n%d. 👤 *%s* — %.1f GB Total", i+1, user.Username, trafficGB)
		}
	}

	msg += "\n\n_Check Marzban Panel immediately._"

	return msg
}

// ForceCheck принудительно проверяет систему (для тестирования)
func (w *Watchdog) ForceCheck() {
	w.checkSystem()
}

// TestAlert отправляет тестовый алерт (для админа)
func (w *Watchdog) TestAlert() {
	ctx := context.Background()

	// Создаём тестовые данные с высокой нагрузкой
	stats := &SystemStats{
		CPUPercent:    98.5,
		MemoryPercent: 75.0,
		NetworkRxMbps: 450.0,
		NetworkTxMbps: 120.0,
		TotalUsers:    100,
		ActiveUsers:   45,
	}

	topUsers, _ := w.getTopUsers(ctx, 3)
	message := w.formatAlertMessage(stats, true, true, topUsers)

	for _, adminID := range w.adminIDs {
		w.bot.Send(&tele.User{ID: adminID}, "🧪 *TEST ALERT* (симуляция)\n\n"+message, tele.ModeMarkdown)
	}
}



