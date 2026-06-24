package app

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DatabaseURL     string
	Address         string
	FrontendOrigin  string
	OfficerID       string
	MemberID        string
	MemberPlate     string
	MemberBalance   int64
}

func LoadConfig() Config {
	return Config{
		DatabaseURL:    getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/parking_violation?sslmode=disable"),
		Address:        getEnv("API_ADDR", ":8080"),
		FrontendOrigin: getEnv("FRONTEND_ORIGIN", "http://localhost:5173"),
		OfficerID:      getEnv("SEED_OFFICER_ID", "11111111-1111-1111-1111-111111111111"),
		MemberID:       getEnv("SEED_MEMBER_ID", "22222222-2222-2222-2222-222222222222"),
		MemberPlate:    getEnv("SEED_MEMBER_PLATE", "B1234CD"),
		MemberBalance:  getEnvInt64("SEED_MEMBER_BALANCE", 1500000),
	}
}

func getEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func getEnvInt64(key string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}
