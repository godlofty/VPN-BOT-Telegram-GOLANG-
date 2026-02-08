package main

import (
	"context"
	"flag"
	"log"
	"time"

	"vpn-telegram-bot/internal/config"
	"vpn-telegram-bot/internal/database"
	"vpn-telegram-bot/internal/handlers"
	"vpn-telegram-bot/internal/service"

	tele "gopkg.in/telebot.v3"
)

// SupportGroupID - ID группы для тикетов поддержки
const SupportGroupID int64 = -1003561858830

func main() {
	// Парсим флаги
	configPath := flag.String("config", "config.yaml", "path to config file")
	migrationsPath := flag.String("migrations", "db/migrations", "path to migrations directory")
	flag.Parse()

	// Загружаем конфигурацию
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Подключаемся к базе данных используя DATABASE_URL
	db, err := database.New(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Выполняем миграции из SQL файлов
	ctx := context.Background()
	if err := db.RunMigrations(ctx, *migrationsPath); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	log.Println("✅ Database migrations completed")

	// Создаём VPN провайдер (mock или real в зависимости от APP_ENV)
	var vpnProvider service.VPNProvider
	if cfg.IsMockMode() {
		log.Println("🧪 Running in MOCK MODE (APP_ENV=local)")
		vpnProvider = service.NewMockVPNProvider()
	} else {
		log.Println("🚀 Running in PRODUCTION MODE (APP_ENV=production)")
		vpnProvider = service.NewMarzbanProvider(cfg.Marzban)
	}

	// Создаём сервис
	svc := service.New(db, vpnProvider)

	// Настраиваем бота
	pref := tele.Settings{
		Token:  cfg.Telegram.Token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}

	bot, err := tele.NewBot(pref)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	// Регистрируем обработчики
	h := handlers.New(svc, cfg.Telegram.AdminIDs, SupportGroupID)
	h.Register(bot)
	h.RegisterAdmin(bot)

	// Support Bridge: слушаем ответы в группе поддержки
	h.RegisterSupportBridge(bot, SupportGroupID)

	// Создаём и запускаем Watchdog
	watchdogConfig := service.DefaultWatchdogConfig()
	watchdog := service.NewWatchdog(bot, cfg.Telegram.AdminIDs, vpnProvider, watchdogConfig)
	watchdog.Start()
	defer watchdog.Stop()

	// Регистрируем команду для тестирования Watchdog (только для админов)
	bot.Handle("/watchdog_test", func(c tele.Context) error {
		for _, adminID := range cfg.Telegram.AdminIDs {
			if c.Sender().ID == adminID {
				watchdog.TestAlert()
				return c.Send("🧪 Тестовый алерт Watchdog отправлен!")
			}
		}
		return nil
	})

	log.Printf("🐸 Bot @%s started!", bot.Me.Username)
	log.Printf("👑 Admin IDs: %v", cfg.Telegram.AdminIDs)
	log.Println("🐕 Watchdog monitoring active")
	bot.Start()
}
