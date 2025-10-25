package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	genai "github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// PagerDuty webhook payload structures
type WebhookPayload struct {
	Event WebhookEvent `json:"event"`
}

type WebhookEvent struct {
	ID           string       `json:"id"`
	EventType    string       `json:"event_type"`
	ResourceType string       `json:"resource_type"`
	OccurredAt   string       `json:"occurred_at"`
	Data         IncidentData `json:"data"`
}

type IncidentData struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Service     ServiceSummary `json:"service"`
	Urgency     string         `json:"urgency"`
	Status      string         `json:"status"`
}

type ServiceSummary struct {
	Summary string `json:"summary"`
}

type SearchResult struct {
	IncidentID string  `json:"incident_id"`
	Section    string  `json:"section"`
	Service    string  `json:"service"`
	Severity   string  `json:"severity"`
	Date       string  `json:"date"`
	Text       string  `json:"text"`
	Score      float32 `json:"score"`
}

// Webhook is the serverless function handler for Vercel
func Webhook(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// Handle OPTIONS preflight
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Only accept POST
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse webhook payload
	var payload WebhookPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Printf("Failed to decode payload: %v", err)
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	// Log the event
	log.Printf("Received webhook: %s - %s", payload.Event.EventType, payload.Event.Data.ID)

	// Only process incident.triggered events
	if payload.Event.EventType != "incident.triggered" {
		log.Printf("Ignoring event type: %s", payload.Event.EventType)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "ignored",
			"reason": "not an incident.triggered event",
		})
		return
	}

	// Return 200 immediately (async processing)
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"status":      "accepted",
		"incident_id": payload.Event.Data.ID,
	})

	// Process in background (non-blocking)
	go processIncident(payload.Event.Data)
}

func processIncident(data IncidentData) {
	log.Printf("Processing incident: %s - %s", data.ID, data.Title)

	ctx := context.Background()

	// Validate environment variables
	geminiKey := os.Getenv("GEMINI_API_KEY")
	qdrantURL := os.Getenv("QDRANT_URL")
	qdrantKey := os.Getenv("QDRANT_API_KEY")
	pdToken := os.Getenv("PAGERDUTY_API_TOKEN")
	pdEmail := os.Getenv("PAGERDUTY_EMAIL")
	collectionName := os.Getenv("COLLECTION_NAME")

	if geminiKey == "" {
		log.Println("GEMINI_API_KEY is required")
		return
	}
	if qdrantURL == "" {
		log.Println("QDRANT_URL is required")
		return
	}
	if qdrantKey == "" {
		log.Println("QDRANT_API_KEY is required")
		return
	}
	if pdToken == "" {
		log.Println("PAGERDUTY_API_TOKEN is required")
		return
	}
	if pdEmail == "" {
		log.Println("PAGERDUTY_EMAIL is required")
		return
	}
	if collectionName == "" {
		collectionName = "incident-knowledge-base"
	}

	// Initialize Gemini client
	client, err := genai.NewClient(ctx, option.WithAPIKey(geminiKey))
	if err != nil {
		log.Printf("Failed to create Gemini client: %v", err)
		return
	}
	defer client.Close()

	// Step 1: Generate embedding
	searchQuery := fmt.Sprintf("%s %s", data.Title, data.Description)
	embedding, err := generateEmbedding(ctx, client, searchQuery)
	if err != nil {
		log.Printf("Failed to generate embedding: %v", err)
		return
	}

	// Step 2: Search Qdrant
	results, err := searchQdrant(qdrantURL, qdrantKey, collectionName, embedding, 3)
	if err != nil {
		log.Printf("Failed to search Qdrant: %v", err)
		return
	}

	if len(results) == 0 {
		note := "================================\n       AI ENRICHMENT\n================================\n\nNo similar past incidents found in the knowledge base."
		postNoteToPagerDuty(data.ID, note, pdToken, pdEmail)
		return
	}

	// Step 3: Generate AI context
	prompt := buildPrompt(data, results)
	aiContext, err := generateContext(ctx, client, prompt)
	if err != nil {
		log.Printf("Failed to generate context: %v", err)
		return
	}

	// Step 4: Format and post note
	note := formatNote(aiContext, results)
	err = postNoteToPagerDuty(data.ID, note, pdToken, pdEmail)
	if err != nil {
		log.Printf("Failed to post note: %v", err)
		return
	}

	log.Printf("Successfully enriched incident: %s", data.ID)
}

func generateEmbedding(ctx context.Context, client *genai.Client, text string) ([]float32, error) {
	em := client.EmbeddingModel("text-embedding-004")
	res, err := em.EmbedContent(ctx, genai.Text(text))
	if err != nil {
		return nil, err
	}
	return res.Embedding.Values, nil
}

func searchQdrant(url, apiKey, collection string, embedding []float32, limit int) ([]SearchResult, error) {
	searchURL := fmt.Sprintf("%s/collections/%s/points/search", url, collection)

	payload := map[string]interface{}{
		"vector":       embedding,
		"limit":        limit,
		"with_payload": true,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", searchURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("qdrant returned status %d", resp.StatusCode)
	}

	var searchResp struct {
		Result []struct {
			Score   float32                `json:"score"`
			Payload map[string]interface{} `json:"payload"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, err
	}

	results := make([]SearchResult, 0, len(searchResp.Result))
	for _, point := range searchResp.Result {
		result := SearchResult{Score: point.Score}
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

func generateContext(ctx context.Context, client *genai.Client, prompt string) (string, error) {
	model := client.GenerativeModel("gemini-1.5-flash")
	model.SetTemperature(0.7)
	model.SetMaxOutputTokens(800)

	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", err
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no response from Gemini")
	}

	return fmt.Sprintf("%v", resp.Candidates[0].Content.Parts[0]), nil
}

func buildPrompt(incident IncidentData, results []SearchResult) string {
	var sb strings.Builder

	sb.WriteString("You are an expert SRE assistant helping with incident triage.\n\n")
	sb.WriteString("NEW ALERT:\n")
	sb.WriteString(fmt.Sprintf("Title: %s\n", incident.Title))
	sb.WriteString(fmt.Sprintf("Description: %s\n", incident.Description))
	sb.WriteString(fmt.Sprintf("Service: %s\n", incident.Service.Summary))
	sb.WriteString(fmt.Sprintf("Urgency: %s\n\n", incident.Urgency))

	sb.WriteString("SIMILAR PAST INCIDENTS:\n\n")
	for idx, result := range results {
		sb.WriteString(fmt.Sprintf("%d. %s (%s section, %.0f%% match)\n", idx+1, result.IncidentID, result.Section, result.Score*100))
		sb.WriteString(fmt.Sprintf("   Service: %s | Severity: %s | Date: %s\n", result.Service, result.Severity, result.Date))

		text := result.Text
		if len(text) > 300 {
			text = text[:300] + "..."
		}
		sb.WriteString(fmt.Sprintf("   Content: %s\n\n", text))
	}

	sb.WriteString("TASK:\n")
	sb.WriteString("Generate a concise triage note (max 400 words) with:\n")
	sb.WriteString("1. Likely Root Cause (based on similar incidents)\n")
	sb.WriteString("2. Recommended Resolution Steps (specific and actionable)\n")
	sb.WriteString("3. Related Incident IDs for reference\n\n")
	sb.WriteString("Format the response in clear, professional sections using proper headers.\n")
	sb.WriteString("Use plain text formatting - no bold, italics, or markdown styling.\n")
	sb.WriteString("Be concise and action-oriented. Focus on what the on-call engineer should do NOW.\n")

	return sb.String()
}

func formatNote(aiContext string, results []SearchResult) string {
	var sb strings.Builder

	sb.WriteString("================================\n")
	sb.WriteString("       AI ENRICHMENT\n")
	sb.WriteString("================================\n\n")
	sb.WriteString(aiContext)
	sb.WriteString("\n\n")
	sb.WriteString("--------------------------------\n")
	sb.WriteString("SIMILARITY SCORES\n")
	sb.WriteString("--------------------------------\n")
	for idx, result := range results {
		sb.WriteString(fmt.Sprintf("  [%d] %s: %.1f%% match (%s)\n", idx+1, result.IncidentID, result.Score*100, result.Section))
	}
	sb.WriteString("\n")

	return sb.String()
}

func postNoteToPagerDuty(incidentID, note, token, email string) error {
	url := fmt.Sprintf("https://api.pagerduty.com/incidents/%s/notes", incidentID)

	payload := map[string]interface{}{
		"note": map[string]string{
			"content": note,
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "application/vnd.pagerduty+json;version=2")
	req.Header.Set("Authorization", "Token token="+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("From", email)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		return fmt.Errorf("PagerDuty API returned status %d", resp.StatusCode)
	}

	return nil
}
