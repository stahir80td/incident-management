package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/stahir80td/incident-management/services"
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

	// Create RAG service
	ragService, err := services.NewRAGService()
	if err != nil {
		log.Printf("Failed to create RAG service: %v", err)
		return
	}
	defer ragService.Close()

	// Build incident data
	incident := services.IncidentData{
		ID:          data.ID,
		Title:       data.Title,
		Description: data.Description,
		Service:     data.Service.Summary,
		Urgency:     data.Urgency,
	}

	// Enrich incident with AI context
	err = ragService.EnrichIncident(incident)
	if err != nil {
		log.Printf("Failed to enrich incident %s: %v", data.ID, err)
		return
	}

	log.Printf("Successfully enriched incident: %s", data.ID)
}

// For local testing
func main() {
	http.HandleFunc("/api/webhook", Webhook)

	port := "8080"
	log.Printf("Starting server on port %s", port)
	if err := http.ListenAndServe(fmt.Sprintf(":%s", port), nil); err != nil {
		log.Fatal(err)
	}
}
