package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	GeminiAPIKey    string
	QdrantURL       string
	QdrantAPIKey    string
	PagerDutyToken  string
	PagerDutyEmail  string
	CollectionName  string
	EmbeddingModel  string
	GenerativeModel string
	EmbeddingDim    int
}

var AppConfig *Config

func init() {
	// Load .env file (only works locally, Vercel uses env vars)
	_ = godotenv.Load()

	AppConfig = &Config{
		GeminiAPIKey:    getEnv("GEMINI_API_KEY", ""),
		QdrantURL:       getEnv("QDRANT_URL", ""),
		QdrantAPIKey:    getEnv("QDRANT_API_KEY", ""),
		PagerDutyToken:  getEnv("PAGERDUTY_API_TOKEN", ""),
		PagerDutyEmail:  getEnv("PAGERDUTY_EMAIL", ""),
		CollectionName:  getEnv("COLLECTION_NAME", "incident-knowledge-base"),
		EmbeddingModel:  getEnv("EMBEDDING_MODEL", "models/gemini-embedding-001"),
		GenerativeModel: getEnv("GENERATIVE_MODEL", "gemini-2.0-flash-exp"),
		EmbeddingDim:    768,
	}

	// Validate required fields
	if AppConfig.GeminiAPIKey == "" {
		log.Fatal("GEMINI_API_KEY is required")
	}
	if AppConfig.QdrantURL == "" {
		log.Fatal("QDRANT_URL is required")
	}
	if AppConfig.QdrantAPIKey == "" {
		log.Fatal("QDRANT_API_KEY is required")
	}
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
