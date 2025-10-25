package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type TestWebhook struct {
	Event struct {
		ID           string `json:"id"`
		EventType    string `json:"event_type"`
		ResourceType string `json:"resource_type"`
		OccurredAt   string `json:"occurred_at"`
		Data         struct {
			ID          string `json:"id"`
			Type        string `json:"type"`
			Title       string `json:"title"`
			Description string `json:"description"`
			Service     struct {
				Summary string `json:"summary"`
			} `json:"service"`
			Urgency string `json:"urgency"`
			Status  string `json:"status"`
		} `json:"data"`
	} `json:"event"`
}

type PagerDutyIncidentRequest struct {
	Incident struct {
		Type    string `json:"type"`
		Title   string `json:"title"`
		Service struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"service"`
		Urgency string `json:"urgency"`
		Body    struct {
			Type    string `json:"type"`
			Details string `json:"details"`
		} `json:"body"`
	} `json:"incident"`
}

type PagerDutyIncidentResponse struct {
	Incident struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"incident"`
}

func createPagerDutyIncident(apiToken, email, serviceID string) (string, error) {
	// Create incident request
	req := PagerDutyIncidentRequest{}
	req.Incident.Type = "incident"
	req.Incident.Title = "Test: High CPU usage on payment-service"
	req.Incident.Service.ID = serviceID
	req.Incident.Service.Type = "service_reference"
	req.Incident.Urgency = "high"
	req.Incident.Body.Type = "incident_body"
	req.Incident.Body.Details = "CPU utilization exceeded 90% threshold for 5 minutes. This is a test incident for RAG enrichment."

	jsonData, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequest("POST", "https://api.pagerduty.com/incidents", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.pagerduty+json;version=2")
	httpReq.Header.Set("Authorization", "Token token="+apiToken)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("From", email)

	// Send request
	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		var errResp map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&errResp)
		return "", fmt.Errorf("PagerDuty API returned status %d: %+v", resp.StatusCode, errResp)
	}

	// Parse response
	var pdResp PagerDutyIncidentResponse
	if err := json.NewDecoder(resp.Body).Decode(&pdResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return pdResp.Incident.ID, nil
}

func getDefaultServiceID(apiToken string) (string, error) {
	// Get list of services
	httpReq, err := http.NewRequest("GET", "https://api.pagerduty.com/services?limit=1", nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.pagerduty+json;version=2")
	httpReq.Header.Set("Authorization", "Token token="+apiToken)

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Services []struct {
			ID string `json:"id"`
		} `json:"services"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Services) == 0 {
		return "", fmt.Errorf("no services found in PagerDuty account")
	}

	return result.Services[0].ID, nil
}

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using environment variables")
	}

	apiToken := os.Getenv("PAGERDUTY_API_TOKEN")
	email := os.Getenv("PAGERDUTY_EMAIL")
	webhookURL := os.Getenv("WEBHOOK_URL")

	// Default to localhost if not specified
	if webhookURL == "" {
		webhookURL = "http://localhost:8080/api/webhook"
		log.Printf("WEBHOOK_URL not set in .env, using default: %s\n", webhookURL)
	}

	if apiToken == "" || email == "" {
		log.Fatal("PAGERDUTY_API_TOKEN and PAGERDUTY_EMAIL must be set in .env file")
	}

	// Step 1: Get default service ID
	fmt.Println("Step 1: Getting PagerDuty service ID...")
	serviceID, err := getDefaultServiceID(apiToken)
	if err != nil {
		log.Fatalf("Failed to get service ID: %v", err)
	}
	fmt.Printf("✓ Using service ID: %s\n\n", serviceID)

	// Step 2: Create a real PagerDuty incident
	fmt.Println("Step 2: Creating test incident in PagerDuty...")
	incidentID, err := createPagerDutyIncident(apiToken, email, serviceID)
	if err != nil {
		log.Fatalf("Failed to create incident: %v", err)
	}
	fmt.Printf("✓ Created incident: %s\n\n", incidentID)

	// Wait a moment for PagerDuty to process
	time.Sleep(2 * time.Second)

	// Step 3: Create test webhook payload
	fmt.Printf("Step 3: Sending webhook to %s...\n", webhookURL)
	payload := TestWebhook{}
	payload.Event.ID = "test-event-001"
	payload.Event.EventType = "incident.triggered"
	payload.Event.ResourceType = "incident"
	payload.Event.OccurredAt = time.Now().UTC().Format(time.RFC3339)
	payload.Event.Data.ID = incidentID // Use real incident ID
	payload.Event.Data.Type = "incident"
	payload.Event.Data.Title = "Test: High CPU usage on payment-service"
	payload.Event.Data.Description = "CPU utilization exceeded 90% threshold for 5 minutes"
	payload.Event.Data.Service.Summary = "payment-service"
	payload.Event.Data.Urgency = "high"
	payload.Event.Data.Status = "triggered"

	// Convert to JSON
	jsonData, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Webhook payload:")
	fmt.Println(string(jsonData))
	fmt.Println()

	// Send to webhook URL
	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Fatalf("Failed to send webhook: %v", err)
	}
	defer resp.Body.Close()

	fmt.Printf("Response Status: %s\n", resp.Status)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	fmt.Printf("Response Body:\n%s\n", string(resultJSON))

	fmt.Printf("\n✓ Test complete! Check incident %s in PagerDuty for the enrichment note.\n", incidentID)
}
