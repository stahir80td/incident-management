package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/stahir80td/incident-management/config"
)

type PagerDutyService struct {
	token string
	email string
}

func NewPagerDutyService() *PagerDutyService {
	return &PagerDutyService{
		token: config.AppConfig.PagerDutyToken,
		email: config.AppConfig.PagerDutyEmail,
	}
}

// PostNote adds a note to a PagerDuty incident
func (pd *PagerDutyService) PostNote(incidentID, content string) error {
	url := fmt.Sprintf("https://api.pagerduty.com/incidents/%s/notes", incidentID)

	payload := map[string]interface{}{
		"note": map[string]string{
			"content": content,
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.pagerduty+json;version=2")
	req.Header.Set("Authorization", fmt.Sprintf("Token token=%s", pd.token))
	req.Header.Set("From", pd.email)

	// Make request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to post note: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("PagerDuty API returned status %d", resp.StatusCode)
	}

	return nil
}
