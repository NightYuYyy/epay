// Package config provides configuration loading, defaults, and validation.
package config

import (
	"fmt"
	"strings"
	"sync"

	"github.com/spf13/viper"
)

// Config is the top-level configuration structure.
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Redis    RedisConfig    `mapstructure:"redis"`
	Alipay   AlipayConfig   `mapstructure:"alipay"`
	Wxpay    WxpayConfig    `mapstructure:"wxpay"`
	JWT      JWTConfig      `mapstructure:"jwt"`
	Admin    AdminConfig    `mapstructure:"admin"`
	Default  DefaultConfig  `mapstructure:"default"`
	Platform PlatformConfig `mapstructure:"platform"`
}

// PlatformConfig holds rainbow-EasyPay platform-level secrets and toggles.
//
//   - RSAPrivateKey: PEM (or raw base64) RSA-2048 private key used to sign
//     downstream artifacts — notify_url callbacks for version=1 orders and
//     `s=path` API responses. The corresponding public key is shared with
//     merchants so they can verify our signatures.
//   - RSAPublicKey: optional — exposed via documentation/endpoint for merchant
//     verification.
//   - SysKey: shared secret used for platform-issued queries
//     (`act=order` with sign=md5(SYS_KEY + trade_no + SYS_KEY)).
//   - UserRefundEnabled: equivalent to rainbow `$conf['user_refund']`; when
//     false, `act=refund` rejects with "未开启商户后台自助退款".
type PlatformConfig struct {
	RSAPrivateKey     string `mapstructure:"rsa_private_key"`
	RSAPublicKey      string `mapstructure:"rsa_public_key"`
	SysKey            string `mapstructure:"sys_key"`
	UserRefundEnabled bool   `mapstructure:"user_refund_enabled"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"` // debug / release / test
}

// DatabaseConfig holds PostgreSQL connection settings.
type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
}

// DSN returns the PostgreSQL connection string.
func (d *DatabaseConfig) DSN() string {
	if d.Password == "" {
		return fmt.Sprintf(
			"host=%s port=%d user=%s dbname=%s sslmode=%s",
			d.Host, d.Port, d.User, d.DBName, d.SSLMode,
		)
	}
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.DBName, d.SSLMode,
	)
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// AlipayConfig holds Alipay payment gateway settings.
type AlipayConfig struct {
	AppID      string `mapstructure:"app_id"`
	PrivateKey string `mapstructure:"private_key"`
	PublicKey  string `mapstructure:"public_key"`
	NotifyURL  string `mapstructure:"notify_url"`
	ReturnURL  string `mapstructure:"return_url"`
}

// WxpayConfig holds WeChat Pay gateway settings.
type WxpayConfig struct {
	AppID        string `mapstructure:"app_id"`
	MchID        string `mapstructure:"mch_id"`
	PrivateKey   string `mapstructure:"private_key"`
	APIv3Key     string `mapstructure:"apiv3_key"`
	PublicKey    string `mapstructure:"public_key"`
	PublicKeyID  string `mapstructure:"public_key_id"`
	SerialNo     string `mapstructure:"serial_no"`
	NotifyURL    string `mapstructure:"notify_url"`
}

// JWTConfig holds JWT authentication settings.
type JWTConfig struct {
	Secret     string `mapstructure:"secret"`
	ExpireHour int    `mapstructure:"expire_hour"`
}

// AdminConfig holds admin user bootstrap settings.
type AdminConfig struct {
	DefaultPassword string `mapstructure:"default_password"`
}

// DefaultConfig holds default business parameters.
type DefaultConfig struct {
	OfficialAlipayRate  float64 `mapstructure:"official_alipay_rate"`
	OfficialWxpayRate   float64 `mapstructure:"official_wxpay_rate"`
	DefaultPlatformRate float64 `mapstructure:"default_platform_rate"`
}

var (
	cfg   *Config
	once  sync.Once
	cfgMu sync.Mutex
)

// Load reads configuration from the given YAML file path, applies environment
// variable overrides, validates required fields, and returns the parsed Config.
//
// Environment variables follow the pattern CONFIG_KEY (e.g. SERVER_HOST,
// DATABASE_PASSWORD). Nested keys use underscore delimiters.
//
// If configPath is empty, viper searches standard locations: current directory,
// ./config/, and /etc/epay/.
func Load(configPath string) (*Config, error) {
	v := viper.New()
	v.SetConfigType("yaml")

	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("config")
		v.AddConfigPath(".")        // current directory
		v.AddConfigPath("./config") // config subdirectory
		v.AddConfigPath("/etc/epay")
	}

	// Environment variable overrides: e.g. SERVER_HOST, DATABASE_PASSWORD
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Set defaults
	setDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config error: %w", err)
		}
		// Config file not found is okay — defaults + env vars may suffice
	}

	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, fmt.Errorf("unmarshal config error: %w", err)
	}

	// Normalize
	trimStrings(&c)

	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("validate config error: %w", err)
	}

	return &c, nil
}

// Get returns the singleton Config instance. On first call it loads from the
// default search paths (equivalent to calling Load("")).
func Get() *Config {
	once.Do(func() {
		c, err := Load("")
		if err != nil {
			// Descriptive panic — callers should use Load explicitly if they
			// want to handle errors gracefully.
			panic(fmt.Sprintf("config: Load() failed: %v", err))
		}
		cfg = c
	})
	return cfg
}

// GetWithError returns the singleton Config, loading it on first call with the
// given path. Unlike Get, it returns the error instead of panicking.
func GetWithError(configPath string) (*Config, error) {
	cfgMu.Lock()
	defer cfgMu.Unlock()

	if cfg != nil {
		return cfg, nil
	}
	c, err := Load(configPath)
	if err != nil {
		return nil, err
	}
	cfg = c
	return cfg, nil
}

// setDefaults registers viper defaults for all configuration fields.
func setDefaults(v *viper.Viper) {
	// Server
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.mode", "release")

	// Database
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.user", "postgres")
	v.SetDefault("database.password", "")
	v.SetDefault("database.dbname", "epay")
	v.SetDefault("database.sslmode", "prefer")

	// Redis
	v.SetDefault("redis.addr", "localhost:6379")
	v.SetDefault("redis.password", "")
	v.SetDefault("redis.db", 0)

	// Alipay
	v.SetDefault("alipay.app_id", "")
	v.SetDefault("alipay.private_key", "")
	v.SetDefault("alipay.public_key", "")
	v.SetDefault("alipay.notify_url", "")
	v.SetDefault("alipay.return_url", "")

	// Wxpay
	v.SetDefault("wxpay.app_id", "")
	v.SetDefault("wxpay.mch_id", "")
	v.SetDefault("wxpay.private_key", "")
	v.SetDefault("wxpay.apiv3_key", "")
	v.SetDefault("wxpay.public_key", "")
	v.SetDefault("wxpay.public_key_id", "")
	v.SetDefault("wxpay.serial_no", "")
	v.SetDefault("wxpay.notify_url", "")

	// JWT
	v.SetDefault("jwt.secret", "")
	v.SetDefault("jwt.expire_hour", 24)

	// Admin
	v.SetDefault("admin.default_password", "admin123")

	// Default rates
	v.SetDefault("default.official_alipay_rate", 0.006)
	v.SetDefault("default.official_wxpay_rate", 0.006)
	v.SetDefault("default.default_platform_rate", 0.009)

	// Platform (rainbow EasyPay extensions)
	v.SetDefault("platform.rsa_private_key", "")
	v.SetDefault("platform.rsa_public_key", "")
	v.SetDefault("platform.sys_key", "")
	v.SetDefault("platform.user_refund_enabled", false)
}

// Validate checks all required fields and value constraints.
func (c *Config) Validate() error {
	// Server
	if c.Server.Host == "" {
		return fmt.Errorf("server.host is required")
	}
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535")
	}
	switch c.Server.Mode {
	case "debug", "release", "test":
	default:
		return fmt.Errorf("server.mode must be one of: debug/release/test")
	}

	// Database
	if c.Database.Host == "" {
		return fmt.Errorf("database.host is required")
	}
	if c.Database.Port <= 0 || c.Database.Port > 65535 {
		return fmt.Errorf("database.port must be between 1 and 65535")
	}
	if c.Database.User == "" {
		return fmt.Errorf("database.user is required")
	}
	if c.Database.DBName == "" {
		return fmt.Errorf("database.dbname is required")
	}

	// Redis
	if c.Redis.Addr == "" {
		return fmt.Errorf("redis.addr is required")
	}
	if c.Redis.DB < 0 {
		return fmt.Errorf("redis.db must be non-negative")
	}

	// JWT
	if c.JWT.Secret == "" {
		return fmt.Errorf("jwt.secret is required")
	}
	if c.JWT.ExpireHour <= 0 {
		return fmt.Errorf("jwt.expire_hour must be positive")
	}

	// Default rates
	if c.Default.OfficialAlipayRate < 0 {
		return fmt.Errorf("default.official_alipay_rate must be non-negative")
	}
	if c.Default.OfficialWxpayRate < 0 {
		return fmt.Errorf("default.official_wxpay_rate must be non-negative")
	}
	if c.Default.DefaultPlatformRate < 0 {
		return fmt.Errorf("default.default_platform_rate must be non-negative")
	}

	return nil
}

// trimStrings strips whitespace from all string fields in the config.
func trimStrings(c *Config) {
	c.Server.Host = strings.TrimSpace(c.Server.Host)
	c.Server.Mode = strings.TrimSpace(c.Server.Mode)

	c.Database.Host = strings.TrimSpace(c.Database.Host)
	c.Database.User = strings.TrimSpace(c.Database.User)
	c.Database.Password = strings.TrimSpace(c.Database.Password)
	c.Database.DBName = strings.TrimSpace(c.Database.DBName)
	c.Database.SSLMode = strings.TrimSpace(c.Database.SSLMode)

	c.Redis.Addr = strings.TrimSpace(c.Redis.Addr)
	c.Redis.Password = strings.TrimSpace(c.Redis.Password)

	c.Alipay.AppID = strings.TrimSpace(c.Alipay.AppID)
	c.Alipay.PrivateKey = strings.TrimSpace(c.Alipay.PrivateKey)
	c.Alipay.PublicKey = strings.TrimSpace(c.Alipay.PublicKey)
	c.Alipay.NotifyURL = strings.TrimSpace(c.Alipay.NotifyURL)
	c.Alipay.ReturnURL = strings.TrimSpace(c.Alipay.ReturnURL)

	c.Wxpay.AppID = strings.TrimSpace(c.Wxpay.AppID)
	c.Wxpay.MchID = strings.TrimSpace(c.Wxpay.MchID)
	c.Wxpay.PrivateKey = strings.TrimSpace(c.Wxpay.PrivateKey)
	c.Wxpay.APIv3Key = strings.TrimSpace(c.Wxpay.APIv3Key)
	c.Wxpay.PublicKey = strings.TrimSpace(c.Wxpay.PublicKey)
	c.Wxpay.PublicKeyID = strings.TrimSpace(c.Wxpay.PublicKeyID)
	c.Wxpay.SerialNo = strings.TrimSpace(c.Wxpay.SerialNo)
	c.Wxpay.NotifyURL = strings.TrimSpace(c.Wxpay.NotifyURL)

	c.JWT.Secret = strings.TrimSpace(c.JWT.Secret)

	c.Admin.DefaultPassword = strings.TrimSpace(c.Admin.DefaultPassword)

	c.Platform.RSAPrivateKey = strings.TrimSpace(c.Platform.RSAPrivateKey)
	c.Platform.RSAPublicKey = strings.TrimSpace(c.Platform.RSAPublicKey)
	c.Platform.SysKey = strings.TrimSpace(c.Platform.SysKey)
}
