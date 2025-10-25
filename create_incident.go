package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type IncidentTemplate struct {
	Title       string
	Description string
	Urgency     string
	Category    string
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

var incidents = []IncidentTemplate{
	// Database Issues (1-10)
	{
		Title:       "Database Connection Pool Exhausted",
		Description: "All database connections in the pool are in use. Application cannot acquire new connections. Users experiencing timeouts.",
		Urgency:     "high",
		Category:    "Database",
	},
	{
		Title:       "Slow Database Query Performance",
		Description: "Reports endpoint responding 10x slower than normal. Database queries taking 5+ seconds. Missing index suspected.",
		Urgency:     "high",
		Category:    "Database",
	},
	{
		Title:       "Database Replication Lag",
		Description: "Read replica is 15 minutes behind primary. Analytics dashboard showing stale data. Replication lag increasing.",
		Urgency:     "medium",
		Category:    "Database",
	},
	{
		Title:       "Database Disk Space Critical",
		Description: "PostgreSQL database disk usage at 95%. Transaction logs filling up rapidly. Auto-vacuum not keeping up.",
		Urgency:     "high",
		Category:    "Database",
	},
	{
		Title:       "Database Deadlock Spike",
		Description: "Deadlock rate increased 500% in last hour. Order processing failing intermittently. Multiple tables involved.",
		Urgency:     "high",
		Category:    "Database",
	},
	{
		Title:       "Redis Cache Miss Rate High",
		Description: "Cache hit rate dropped from 95% to 40%. Database load spiking. Redis memory eviction occurring.",
		Urgency:     "medium",
		Category:    "Database",
	},
	{
		Title:       "MongoDB Write Conflicts",
		Description: "High number of write conflicts in orders collection. Transaction retry rate at 30%. Performance degraded.",
		Urgency:     "medium",
		Category:    "Database",
	},
	{
		Title:       "Database Backup Failed",
		Description: "Nightly database backup job failed. Last successful backup was 2 days ago. Disk space issue suspected.",
		Urgency:     "medium",
		Category:    "Database",
	},
	{
		Title:       "SQL Injection Attempt Detected",
		Description: "WAF detected SQL injection patterns in API requests. 50+ attempts from same IP range. Users table targeted.",
		Urgency:     "high",
		Category:    "Database",
	},
	{
		Title:       "Database Connection Timeout",
		Description: "Application reporting connection timeouts to primary database. Network latency increased. 5% of requests failing.",
		Urgency:     "high",
		Category:    "Database",
	},

	// API/Service Issues (11-20)
	{
		Title:       "Payment Gateway API Down",
		Description: "Stripe API returning 503 errors. All payment transactions failing. Checkout flow broken for customers.",
		Urgency:     "high",
		Category:    "API",
	},
	{
		Title:       "API Rate Limit Exceeded",
		Description: "Third-party API rate limit hit. 429 errors increasing. Background jobs backing up. 2-hour delay in processing.",
		Urgency:     "medium",
		Category:    "API",
	},
	{
		Title:       "Authentication Service Latency",
		Description: "Auth0 response times increased from 100ms to 5s. User login attempts timing out. 30% failure rate.",
		Urgency:     "high",
		Category:    "API",
	},
	{
		Title:       "Microservice Circuit Breaker Open",
		Description: "Recommendation service circuit breaker open. Failing fast after timeout threshold exceeded. Fallback data served.",
		Urgency:     "medium",
		Category:    "API",
	},
	{
		Title:       "API Gateway 5xx Error Spike",
		Description: "API Gateway returning 502/504 errors. Backend service health checks failing. 15% error rate on production traffic.",
		Urgency:     "high",
		Category:    "API",
	},
	{
		Title:       "GraphQL Query Timeout",
		Description: "Complex GraphQL queries timing out. N+1 query problem suspected. Apollo Server memory usage high.",
		Urgency:     "medium",
		Category:    "API",
	},
	{
		Title:       "REST API Authentication Failures",
		Description: "JWT token validation failing. Users getting 401 errors after login. Clock skew between services suspected.",
		Urgency:     "high",
		Category:    "API",
	},
	{
		Title:       "Webhook Delivery Failures",
		Description: "Outbound webhooks failing to deliver. Retry queue building up. Customer integration broken.",
		Urgency:     "medium",
		Category:    "API",
	},
	{
		Title:       "API Response Size Too Large",
		Description: "API returning 10MB+ responses. Mobile app crashes on large datasets. Pagination not implemented correctly.",
		Urgency:     "medium",
		Category:    "API",
	},
	{
		Title:       "CORS Policy Blocking Requests",
		Description: "Frontend app blocked by CORS policy after deployment. API rejecting requests from new domain. Users cannot access app.",
		Urgency:     "high",
		Category:    "API",
	},

	// Infrastructure (21-30)
	{
		Title:       "Kubernetes Pod CrashLoopBackOff",
		Description: "Payment service pods crashing on startup. Out of memory errors in logs. Deployment rollout stuck at 50%.",
		Urgency:     "high",
		Category:    "Infrastructure",
	},
	{
		Title:       "Load Balancer Health Check Failures",
		Description: "ALB marking instances as unhealthy. Health check endpoint returning 503. Traffic not routing correctly.",
		Urgency:     "high",
		Category:    "Infrastructure",
	},
	{
		Title:       "CDN Cache Purge Failed",
		Description: "Cloudflare cache purge request failed. Users seeing stale content after deployment. CSS/JS files outdated.",
		Urgency:     "medium",
		Category:    "Infrastructure",
	},
	{
		Title:       "Auto-Scaling Not Triggering",
		Description: "CPU at 90% but no new instances launching. Auto-scaling policy misconfigured. Response times degrading.",
		Urgency:     "high",
		Category:    "Infrastructure",
	},
	{
		Title:       "SSL Certificate Expiring Soon",
		Description: "Production SSL certificate expires in 3 days. Auto-renewal failed. Manual intervention required.",
		Urgency:     "high",
		Category:    "Infrastructure",
	},
	{
		Title:       "DDoS Attack in Progress",
		Description: "Unusual traffic spike from multiple IPs. 50k requests/second. Application performance severely degraded.",
		Urgency:     "high",
		Category:    "Infrastructure",
	},
	{
		Title:       "Docker Registry Unavailable",
		Description: "Cannot pull images from Docker registry. New deployments blocked. Registry returning 500 errors.",
		Urgency:     "high",
		Category:    "Infrastructure",
	},
	{
		Title:       "Network Partition Detected",
		Description: "Lost connectivity between availability zones. Split-brain scenario possible. Database writes may be inconsistent.",
		Urgency:     "high",
		Category:    "Infrastructure",
	},
	{
		Title:       "S3 Bucket Access Denied",
		Description: "Application cannot read/write to S3 bucket. IAM policy change detected. File uploads failing for users.",
		Urgency:     "high",
		Category:    "Infrastructure",
	},
	{
		Title:       "DNS Resolution Failures",
		Description: "Intermittent DNS lookup failures for internal services. Route53 health checks failing. 10% of requests affected.",
		Urgency:     "high",
		Category:    "Infrastructure",
	},

	// Application (31-40)
	{
		Title:       "Memory Leak in Node.js Service",
		Description: "Node.js process memory usage growing 100MB/hour. Periodic restarts required. Heap snapshots show retained objects.",
		Urgency:     "medium",
		Category:    "Application",
	},
	{
		Title:       "Null Pointer Exception Spike",
		Description: "Java service throwing NullPointerExceptions. 500 errors returned to users. Recent code deployment suspected.",
		Urgency:     "high",
		Category:    "Application",
	},
	{
		Title:       "Frontend JavaScript Errors",
		Description: "Browser console showing uncaught exceptions. User actions not completing. React component rendering failing.",
		Urgency:     "high",
		Category:    "Application",
	},
	{
		Title:       "Session Store Full",
		Description: "Redis session store at capacity. Users being logged out unexpectedly. Session creation failing.",
		Urgency:     "high",
		Category:    "Application",
	},
	{
		Title:       "Background Job Queue Backed Up",
		Description: "Sidekiq queue depth at 50k jobs. Email sending delayed 3+ hours. Workers cannot keep up with rate.",
		Urgency:     "medium",
		Category:    "Application",
	},
	{
		Title:       "File Upload Size Limit Exceeded",
		Description: "Users unable to upload files larger than 5MB. Configuration mismatch between nginx and application.",
		Urgency:     "medium",
		Category:    "Application",
	},
	{
		Title:       "Race Condition in Payment Processing",
		Description: "Duplicate payment charges reported. Race condition in transaction handling. 10 customers affected.",
		Urgency:     "high",
		Category:    "Application",
	},
	{
		Title:       "Localization Strings Missing",
		Description: "Spanish language users seeing English text. i18n files not deployed correctly. Affects 20% of user base.",
		Urgency:     "medium",
		Category:    "Application",
	},
	{
		Title:       "Infinite Loop in Report Generation",
		Description: "Report generation job running for 6+ hours. CPU pegged at 100%. Suspected infinite loop in data processing.",
		Urgency:     "medium",
		Category:    "Application",
	},
	{
		Title:       "WebSocket Connection Drops",
		Description: "Real-time chat feature disconnecting users every 30 seconds. WebSocket keep-alive not working. Nginx timeout issue.",
		Urgency:     "medium",
		Category:    "Application",
	},

	// Security (41-50)
	{
		Title:       "Unauthorized Access Attempt Detected",
		Description: "Multiple failed login attempts from same IP. Brute force attack suspected. Account lockout triggered for 5 users.",
		Urgency:     "high",
		Category:    "Security",
	},
	{
		Title:       "API Key Leaked on GitHub",
		Description: "Production API key found in public repository. Immediate rotation required. Scanning for unauthorized usage.",
		Urgency:     "high",
		Category:    "Security",
	},
	{
		Title:       "XSS Vulnerability Reported",
		Description: "Security researcher reported stored XSS in comment section. CVE assigned. Patch deployment urgent.",
		Urgency:     "high",
		Category:    "Security",
	},
	{
		Title:       "Unusual Data Export Activity",
		Description: "Admin user exported entire user database. Action outside normal behavior pattern. Investigating for data breach.",
		Urgency:     "high",
		Category:    "Security",
	},
	{
		Title:       "Malware Detected on Server",
		Description: "Anti-virus detected cryptocurrency miner on application server. Process consuming 80% CPU. Quarantine initiated.",
		Urgency:     "high",
		Category:    "Security",
	},
	{
		Title:       "GDPR Data Deletion Request Failed",
		Description: "Automated GDPR deletion job failed. User data retention policy violated. Legal compliance risk.",
		Urgency:     "medium",
		Category:    "Security",
	},
	{
		Title:       "Insecure Direct Object Reference",
		Description: "Users can access other users' data by modifying URL parameters. Authorization check bypassed. Critical security flaw.",
		Urgency:     "high",
		Category:    "Security",
	},
	{
		Title:       "TLS 1.0 Connections Detected",
		Description: "Legacy clients using deprecated TLS 1.0 protocol. PCI compliance violation. Must disable within 24 hours.",
		Urgency:     "high",
		Category:    "Security",
	},
	{
		Title:       "Ransomware Attack Warning",
		Description: "Security vendor detected ransomware indicators. Suspicious encrypted files found. Isolating affected systems.",
		Urgency:     "high",
		Category:    "Security",
	},
	{
		Title:       "Privilege Escalation Vulnerability",
		Description: "Regular user can gain admin privileges through API exploit. CVE-2024-XXXXX. Zero-day vulnerability discovered.",
		Urgency:     "high",
		Category:    "Security",
	},
}

func getDefaultServiceID(apiToken string) (string, error) {
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

func createPagerDutyIncident(apiToken, email, serviceID string, template IncidentTemplate) (string, error) {
	req := PagerDutyIncidentRequest{}
	req.Incident.Type = "incident"
	req.Incident.Title = template.Title
	req.Incident.Service.ID = serviceID
	req.Incident.Service.Type = "service_reference"
	req.Incident.Urgency = template.Urgency
	req.Incident.Body.Type = "incident_body"
	req.Incident.Body.Details = template.Description

	jsonData, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", "https://api.pagerduty.com/incidents", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.pagerduty+json;version=2")
	httpReq.Header.Set("Authorization", "Token token="+apiToken)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("From", email)

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

	var pdResp PagerDutyIncidentResponse
	if err := json.NewDecoder(resp.Body).Decode(&pdResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return pdResp.Incident.ID, nil
}

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using environment variables")
	}

	apiToken := os.Getenv("PAGERDUTY_API_TOKEN")
	email := os.Getenv("PAGERDUTY_EMAIL")

	if apiToken == "" || email == "" {
		log.Fatal("PAGERDUTY_API_TOKEN and PAGERDUTY_EMAIL must be set in .env file")
	}

	// Check for incident number argument
	if len(os.Args) < 2 {
		fmt.Println("╔══════════════════════════════════════════════════════════════╗")
		fmt.Println("║           PagerDuty Incident Generator - Demo               ║")
		fmt.Println("╚══════════════════════════════════════════════════════════════╝")
		fmt.Println()
		fmt.Println("Usage: go run create_incident.go [incident_number]")
		fmt.Println("       go run create_incident.go list")
		fmt.Println()
		fmt.Println("Example: go run create_incident.go 15")
		fmt.Println()
		fmt.Println("Available incidents: 1-50")
		fmt.Println("Use 'list' to see all incident types")
		os.Exit(1)
	}

	// Handle list command
	if os.Args[1] == "list" {
		fmt.Println("╔══════════════════════════════════════════════════════════════╗")
		fmt.Println("║              Available Incident Templates (50)              ║")
		fmt.Println("╚══════════════════════════════════════════════════════════════╝")
		fmt.Println()

		currentCategory := ""
		for i, incident := range incidents {
			if incident.Category != currentCategory {
				currentCategory = incident.Category
				fmt.Printf("\n📁 %s Issues:\n", currentCategory)
				fmt.Println("─────────────────────────────────────────────────────────────")
			}
			urgencyIcon := "🟡"
			if incident.Urgency == "high" {
				urgencyIcon = "🔴"
			}
			fmt.Printf("%s [%2d] %s\n", urgencyIcon, i+1, incident.Title)
		}
		fmt.Println()
		return
	}

	// Parse incident number
	incidentNum, err := strconv.Atoi(os.Args[1])
	if err != nil || incidentNum < 1 || incidentNum > len(incidents) {
		log.Fatalf("Invalid incident number. Must be between 1 and %d", len(incidents))
	}

	template := incidents[incidentNum-1]

	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║           Creating Incident in PagerDuty                    ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Get service ID
	fmt.Println("🔍 Getting PagerDuty service ID...")
	serviceID, err := getDefaultServiceID(apiToken)
	if err != nil {
		log.Fatalf("Failed to get service ID: %v", err)
	}
	fmt.Printf("✅ Using service ID: %s\n\n", serviceID)

	// Display incident details
	fmt.Println("📋 Incident Details:")
	fmt.Println("─────────────────────────────────────────────────────────────")
	fmt.Printf("Category:    %s\n", template.Category)
	fmt.Printf("Title:       %s\n", template.Title)
	fmt.Printf("Urgency:     %s\n", template.Urgency)
	fmt.Printf("Description: %s\n", template.Description)
	fmt.Println()

	// Create incident
	fmt.Println("🚀 Creating incident in PagerDuty...")
	incidentID, err := createPagerDutyIncident(apiToken, email, serviceID, template)
	if err != nil {
		log.Fatalf("Failed to create incident: %v", err)
	}

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    ✅ SUCCESS!                               ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Printf("\n🎯 Incident ID: %s\n", incidentID)
	fmt.Printf("🔗 View in PagerDuty: https://app.pagerduty.com/incidents/%s\n", incidentID)
	fmt.Println()
	fmt.Println("⏳ The AI enrichment webhook will process this incident automatically.")
	fmt.Println("   Check the incident Notes section in 20-30 seconds for AI analysis!")
	fmt.Println()
}
