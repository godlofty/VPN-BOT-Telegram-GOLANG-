package config

import (
	"os"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

// Config конфигурация приложения
type Config struct {
	Telegram    TelegramConfig `yaml:"telegram"`
	Database    DatabaseConfig `yaml:"database"`
	Marzban     MarzbanConfig  `yaml:"marzban"`
	DatabaseURL string         `yaml:"-"` // Loaded from environment
	AppEnv      string         `yaml:"-"` // "local" = mock mode, "production" = real Marzban
}

// TelegramConfig настройки Telegram бота
type TelegramConfig struct {
	Token    string  `yaml:"token"`
	AdminIDs []int64 `yaml:"admin_ids"`
}

// DatabaseConfig настройки базы данных PostgreSQL (legacy, kept for backwards compatibility)
type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"dbname"`
	SSLMode  string `yaml:"sslmode"`
}

// MarzbanConfig настройки Marzban VPN Panel
type MarzbanConfig struct {
	BaseURL  string `yaml:"base_url"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// Load загружает конфигурацию из файла и окружения
func Load(path string) (*Config, error) {
	// Load .env file if it exists (ignore error if file doesn't exist)
	_ = godotenv.Load()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Load from environment
	cfg.DatabaseURL = os.Getenv("DATABASE_URL")
	cfg.AppEnv = os.Getenv("APP_ENV")
	if cfg.AppEnv == "" {
		cfg.AppEnv = "local" // Default to mock mode for safety
	}

	return &cfg, nil
}

// IsMockMode returns true if running in local/development mode
func (c *Config) IsMockMode() bool {
	return c.AppEnv == "local" || c.AppEnv == "development"
}
