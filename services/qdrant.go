package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/stahir80td/incident-management/config"
)

type QdrantService struct {
	baseURL    string
	apiKey     string
	ctx        context.Context
	collection string
	httpClient *http.Client
}

type SearchResult struct {
	IncidentID string
	Section    string
	Service    string
	Severity   string
	Date       string
	Text       string
	Score      float32
}

// Qdrant REST API structures
type searchRequest struct {
	Vector      []float32 `json:"vector"`
	Limit       int       `json:"limit"`
	WithPayload bool      `json:"with_payload"`
}

type searchResponse struct {
	Result []struct {
		ID      interface{}            `json:"id"`
		Version int                    `json:"version"`
		Score   float32                `json:"score"`
		Payload map[string]interface{} `json:"payload"`
	} `json:"result"`
}

func NewQdrantService() (*QdrantService, error) {
	ctx := context.Background()

	// Clean URL - remove port if present for cloud
	baseURL := strings.TrimSuffix(config.AppConfig.QdrantURL, ":6333")
	if !strings.HasPrefix(baseURL, "http") {
		baseURL = "https://" + baseURL
	}

	return &QdrantService{
		baseURL:    baseURL,
		apiKey:     config.AppConfig.QdrantAPIKey,
		ctx:        ctx,
		collection: config.AppConfig.CollectionName,
		httpClient: &http.Client{},
	}, nil
}

// SearchSimilarIncidents finds similar incidents using vector search
func (q *QdrantService) SearchSimilarIncidents(embedding []float32, limit uint64) ([]SearchResult, error) {
	// Build search request
	searchReq := searchRequest{
		Vector:      embedding,
		Limit:       int(limit),
		WithPayload: true,
	}

	jsonData, err := json.Marshal(searchReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal search request: %w", err)
	}

	// Make HTTP request
	url := fmt.Sprintf("%s/collections/%s/points/search", q.baseURL, q.collection)
	req, err := http.NewRequestWithContext(q.ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", q.apiKey)

	resp, err := q.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var searchResp searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Convert to SearchResult
	results := make([]SearchResult, 0, len(searchResp.Result))
	for _, point := range searchResp.Result {
		result := SearchResult{
			Score: point.Score,
		}

		// Extract payload fields
		if val, ok := point.Payload["incident_id"].(string); ok {
			result.IncidentID = val
		}
		if val, ok := point.Payload["section"].(string); ok {
			result.Section = val
		}
		if val, ok := point.Payload["service"].(string); ok {
			result.Service = val
		}
		if val, ok := point.Payload["severity"].(string); ok {
			result.Severity = val
		}
		if val, ok := point.Payload["date"].(string); ok {
			result.Date = val
		}
		if val, ok := point.Payload["text"].(string); ok {
			result.Text = val
		}

		results = append(results, result)
	}

	return results, nil
}

func (q *QdrantService) Close() {
	// HTTP client doesn't need explicit close
}
