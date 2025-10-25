package config

import (
	"fmt"
	"os"
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

// GetConfig returns a new Config instance with values from environment variables
// This function doesn't use init() or godotenv, making it work in Vercel
func GetConfig() *Config {
	return &Config{
		GeminiAPIKey:    getEnv("GEMINI_API_KEY", ""),
		QdrantURL:       getEnv("QDRANT_URL", ""),
		QdrantAPIKey:    getEnv("QDRANT_API_KEY", ""),
		PagerDutyToken:  getEnv("PAGERDUTY_API_TOKEN", ""),
		PagerDutyEmail:  getEnv("PAGERDUTY_EMAIL", ""),
		CollectionName:  getEnv("COLLECTION_NAME", "incident-knowledge-base"),
		EmbeddingModel:  getEnv("EMBEDDING_MODEL", "models/gemini-embedding-001"),
		GenerativeModel: getEnv("GENERATIVE_MODEL", "gemini-2.0-flash-exp"),
		EmbeddingDim:    3072, // Changed to 3072 for full Gemini embeddings
	}
}

// Validate checks if required configuration values are present
func (c *Config) Validate() error {
	if c.GeminiAPIKey == "" {
		return fmt.Errorf("GEMINI_API_KEY is required")
	}
	if c.QdrantURL == "" {
		return fmt.Errorf("QDRANT_URL is required")
	}
	if c.QdrantAPIKey == "" {
		return fmt.Errorf("QDRANT_API_KEY is required")
	}
	return nil
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
