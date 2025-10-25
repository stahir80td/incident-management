package services

import (
	"fmt"
	"strings"
)

type RAGService struct {
	gemini    *GeminiService
	qdrant    *QdrantService
	pagerduty *PagerDutyService
}

type IncidentData struct {
	ID          string
	Title       string
	Description string
	Service     string
	Urgency     string
}

func NewRAGService() (*RAGService, error) {
	gemini, err := NewGeminiService()
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini service: %w", err)
	}

	qdrant, err := NewQdrantService()
	if err != nil {
		return nil, fmt.Errorf("failed to create Qdrant service: %w", err)
	}

	pagerduty := NewPagerDutyService()

	return &RAGService{
		gemini:    gemini,
		qdrant:    qdrant,
		pagerduty: pagerduty,
	}, nil
}

// EnrichIncident performs the full RAG pipeline
func (r *RAGService) EnrichIncident(incident IncidentData) error {
	// Step 1: Create search query from incident
	searchQuery := fmt.Sprintf("%s %s", incident.Title, incident.Description)

	// Step 2: Generate embedding
	embedding, err := r.gemini.GenerateEmbedding(searchQuery, "RETRIEVAL_QUERY")
	if err != nil {
		return fmt.Errorf("failed to generate embedding: %w", err)
	}

	// Step 3: Search for similar incidents
	results, err := r.qdrant.SearchSimilarIncidents(embedding, 3)
	if err != nil {
		return fmt.Errorf("failed to search similar incidents: %w", err)
	}

	if len(results) == 0 {
		// No similar incidents found, post generic note
		note := "================================\n       AI ENRICHMENT\n================================\n\nNo similar past incidents found in the knowledge base."
		return r.pagerduty.PostNote(incident.ID, note)
	}

	// Step 4: Build prompt for LLM
	prompt := r.buildPrompt(incident, results)

	// Step 5: Generate AI context
	aiContext, err := r.gemini.GenerateContext(prompt)
	if err != nil {
		return fmt.Errorf("failed to generate context: %w", err)
	}

	// Step 6: Format and post note to PagerDuty
	note := r.formatNote(aiContext, results)
	err = r.pagerduty.PostNote(incident.ID, note)
	if err != nil {
		return fmt.Errorf("failed to post note: %w", err)
	}

	return nil
}

func (r *RAGService) buildPrompt(incident IncidentData, results []SearchResult) string {
	var sb strings.Builder

	sb.WriteString("You are an expert SRE assistant helping with incident triage.\n\n")
	sb.WriteString("NEW ALERT:\n")
	sb.WriteString(fmt.Sprintf("Title: %s\n", incident.Title))
	sb.WriteString(fmt.Sprintf("Description: %s\n", incident.Description))
	sb.WriteString(fmt.Sprintf("Service: %s\n", incident.Service))
	sb.WriteString(fmt.Sprintf("Urgency: %s\n\n", incident.Urgency))

	sb.WriteString("SIMILAR PAST INCIDENTS:\n\n")
	for idx, result := range results {
		sb.WriteString(fmt.Sprintf("%d. %s (%s section, %.0f%% match)\n", idx+1, result.IncidentID, result.Section, result.Score*100))
		sb.WriteString(fmt.Sprintf("   Service: %s | Severity: %s | Date: %s\n", result.Service, result.Severity, result.Date))

		// Truncate text to first 300 chars
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

func (r *RAGService) formatNote(aiContext string, results []SearchResult) string {
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

func (r *RAGService) Close() {
	r.gemini.Close()
	r.qdrant.Close()
}
