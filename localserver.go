package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	_ "github.com/stahir80td/incident-management/config"
	"github.com/stahir80td/incident-management/services"
)

// Webhook payload structures
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

// Health check handler
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().UTC(),
		"service":   "incident-triage-rag-api",
	}

	json.NewEncoder(w).Encode(response)
}

// Webhook handler
func webhookHandler(w http.ResponseWriter, r *http.Request) {
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
	log.Printf("✅ Received webhook: %s - %s", payload.Event.EventType, payload.Event.Data.ID)

	// Only process incident.triggered events
	if payload.Event.EventType != "incident.triggered" {
		log.Printf("⏭️  Ignoring event type: %s", payload.Event.EventType)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "ignored",
			"reason": "not an incident.triggered event",
		})
		return
	}

	// Return 202 immediately (async processing)
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"status":      "accepted",
		"incident_id": payload.Event.Data.ID,
	})

	// Process in background
	go processIncident(payload.Event.Data)
}

func processIncident(data IncidentData) {
	log.Printf("🔄 Processing incident: %s - %s", data.ID, data.Title)

	// Create RAG service
	ragService, err := services.NewRAGService()
	if err != nil {
		log.Printf("❌ Failed to create RAG service: %v", err)
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
		log.Printf("❌ Failed to enrich incident %s: %v", data.ID, err)
		return
	}

	log.Printf("✅ Successfully enriched incident: %s", data.ID)
}

func main() {
	http.HandleFunc("/api/webhook", webhookHandler)
	http.HandleFunc("/api/health", healthHandler)

	port := "8080"
	//log.Println("=" + "="*60)
	log.Println("🚀 Incident Triage RAG API - Local Server")
	//log.Println("=" + "="*60)
	log.Printf("📍 Webhook endpoint: http://localhost:%s/api/webhook", port)
	log.Printf("💚 Health endpoint:  http://localhost:%s/api/health", port)
	//log.Println("=" + "="*60)
	log.Printf("✨ Server listening on port %s...\n", port)

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
