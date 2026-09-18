package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	ServerPort      string
	BindHost        string
	DatabasePath    string
	AuthToken       string
	JWTSecret       string
	WitnessEnabled  bool
	WitnessInterval int // in seconds
}

func LoadConfig() *Config {
	authToken := getSecretEnv("AUTH_TOKEN", "AUTH_TOKEN_FILE", "opennhrp-secret-token")
	cfg := &Config{
		ServerPort:      getEnv("PORT", "8080"),
		BindHost:        getEnv("BIND_HOST", "0.0.0.0"),
		DatabasePath:    getEnv("DB_PATH", "/etc/opennhrp-manager/database.db"),
		AuthToken:       authToken,
		JWTSecret:       getEnv("JWT_SECRET", authToken),
		WitnessEnabled:  getBoolEnv("WITNESS_ENABLED", true),
		WitnessInterval: 5,
	}

	return cfg
}

func getSecretEnv(valueKey, fileKey, defaultVal string) string {
	if value := os.Getenv(valueKey); value != "" {
		return value
	}
	if path := os.Getenv(fileKey); path != "" {
		if value, err := os.ReadFile(path); err == nil {
			if value := strings.TrimSpace(string(value)); value != "" {
				return value
			}
		}
	}
	return defaultVal
}

func getBoolEnv(key string, defaultVal bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok {
		return defaultVal
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return defaultVal
	}
	return parsed
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
