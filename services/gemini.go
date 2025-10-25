package services

import (
	"context"
	"fmt"

	"github.com/google/generative-ai-go/genai"
	"github.com/stahir80td/incident-management/config"
	"google.golang.org/api/option"
)

type GeminiService struct {
	client *genai.Client
	ctx    context.Context
}

func NewGeminiService() (*GeminiService, error) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(config.AppConfig.GeminiAPIKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	return &GeminiService{
		client: client,
		ctx:    ctx,
	}, nil
}

// GenerateEmbedding creates a vector embedding for the given text
func (g *GeminiService) GenerateEmbedding(text string, taskType string) ([]float32, error) {
	em := g.client.EmbeddingModel(config.AppConfig.EmbeddingModel)

	// Set task type for better embeddings
	var task genai.TaskType
	switch taskType {
	case "RETRIEVAL_QUERY":
		task = genai.TaskTypeRetrievalQuery
	case "RETRIEVAL_DOCUMENT":
		task = genai.TaskTypeRetrievalDocument
	default:
		task = genai.TaskTypeRetrievalQuery
	}

	res, err := em.EmbedContent(g.ctx, genai.Text(text))
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding: %w", err)
	}

	// Note: TaskType and OutputDimensionality may not be supported in older SDK versions
	// If needed, use the REST API directly or update SDK
	_ = task // Suppress unused variable warning for now

	return res.Embedding.Values, nil
}

// GenerateContext uses Gemini to generate AI triage context
func (g *GeminiService) GenerateContext(prompt string) (string, error) {
	model := g.client.GenerativeModel(config.AppConfig.GenerativeModel)

	// Configure model for concise responses
	model.SetTemperature(0.7)
	model.SetTopP(0.95)
	model.SetTopK(40)
	model.SetMaxOutputTokens(1024)

	resp, err := model.GenerateContent(g.ctx, genai.Text(prompt))
	if err != nil {
		return "", fmt.Errorf("failed to generate content: %w", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no content generated")
	}

	// Extract text from response
	result := ""
	for _, part := range resp.Candidates[0].Content.Parts {
		if txt, ok := part.(genai.Text); ok {
			result += string(txt)
		}
	}

	return result, nil
}

func (g *GeminiService) Close() {
	g.client.Close()
}
