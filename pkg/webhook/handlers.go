package webhook

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"
)

// GitHub webhook payload types
type GitHubWebhookPayload struct {
	Action      string             `json:"action"`
	PullRequest *GitHubPullRequest `json:"pull_request,omitempty"`
	Issue       *GitHubIssue       `json:"issue,omitempty"`
	Repository  *GitHubRepository  `json:"repository,omitempty"`
	Sender      *GitHubUser        `json:"sender,omitempty"`
	Commits     []GitHubCommit     `json:"commits,omitempty"`
	Ref         string             `json:"ref,omitempty"`
	Before      string             `json:"before,omitempty"`
	After       string             `json:"after,omitempty"`
}

type GitHubPullRequest struct {
	Number  int         `json:"number"`
	Title   string      `json:"title"`
	Body    string      `json:"body"`
	State   string      `json:"state"`
	User    *GitHubUser `json:"user"`
	HTMLURL string      `json:"html_url"`
}

type GitHubIssue struct {
	Number  int         `json:"number"`
	Title   string      `json:"title"`
	Body    string      `json:"body"`
	State   string      `json:"state"`
	User    *GitHubUser `json:"user"`
	HTMLURL string      `json:"html_url"`
}

type GitHubRepository struct {
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	HTMLURL  string `json:"html_url"`
}

type GitHubUser struct {
	Login   string `json:"login"`
	HTMLURL string `json:"html_url"`
}

type GitHubCommit struct {
	ID      string `json:"id"`
	Message string `json:"message"`
	Author  struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"author"`
	URL string `json:"url"`
}

// Gmail Pub/Sub payload types
type GmailPubSubMessage struct {
	Message struct {
		Data        string `json:"data"`
		MessageID   string `json:"messageId"`
		PublishTime string `json:"publishTime"`
	} `json:"message"`
	Subscription string `json:"subscription"`
}

type GmailNotification struct {
	EmailAddress string `json:"emailAddress"`
	HistoryID    int64  `json:"historyId"`
}

// Telegram webhook payload types
type TelegramUpdate struct {
	UpdateID      int                    `json:"update_id"`
	Message       *TelegramMessage       `json:"message,omitempty"`
	CallbackQuery *TelegramCallbackQuery `json:"callback_query,omitempty"`
}

type TelegramMessage struct {
	MessageID int           `json:"message_id"`
	From      *TelegramUser `json:"from,omitempty"`
	Chat      *TelegramChat `json:"chat"`
	Date      int64         `json:"date"`
	Text      string        `json:"text,omitempty"`
}

type TelegramCallbackQuery struct {
	ID      string           `json:"id"`
	From    *TelegramUser    `json:"from"`
	Message *TelegramMessage `json:"message,omitempty"`
	Data    string           `json:"data,omitempty"`
}

type TelegramUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

type TelegramChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// GitHubHandlerOptions configures the GitHub webhook handler
type GitHubHandlerOptions struct {
	Secret        string
	OnPullRequest func(pr *GitHubPullRequest)
	OnIssue       func(issue *GitHubIssue)
	OnPush        func(payload *GitHubWebhookPayload)
}

// CreateGitHubHandler creates a GitHub webhook handler
func CreateGitHubHandler(options GitHubHandlerOptions) WebhookConfig {
	return WebhookConfig{
		Path:               "/webhook/github",
		Method:             http.MethodPost,
		Secret:             options.Secret,
		SignatureHeader:    "X-Hub-Signature-256",
		SignatureAlgorithm: "sha256",
		Timeout:            30 * time.Second,
		Description:        "GitHub webhook for PR, issue, and push events",
		Handler: func(params WebhookParams) (WebhookResponse, error) {
			// Get event type from header
			eventType, ok := params.Headers["X-Github-Event"]
			if !ok {
				eventType, ok = params.Headers["X-GitHub-Event"]
			}
			if !ok {
				return WebhookResponse{
					Status: http.StatusBadRequest,
					Body:   map[string]string{"error": "Missing X-GitHub-Event header"},
				}, nil
			}

			// Parse payload
			payloadBytes, err := json.Marshal(params.Body)
			if err != nil {
				return WebhookResponse{
					Status: http.StatusBadRequest,
					Body:   map[string]string{"error": "Invalid payload"},
				}, nil
			}

			var payload GitHubWebhookPayload
			if err := json.Unmarshal(payloadBytes, &payload); err != nil {
				return WebhookResponse{
					Status: http.StatusBadRequest,
					Body:   map[string]string{"error": "Failed to parse payload"},
				}, nil
			}

			// Handle different event types
			switch eventType {
			case "pull_request":
				if payload.Action == "opened" && options.OnPullRequest != nil && payload.PullRequest != nil {
					options.OnPullRequest(payload.PullRequest)
				}
			case "issues":
				if payload.Action == "opened" && options.OnIssue != nil && payload.Issue != nil {
					options.OnIssue(payload.Issue)
				}
			case "push":
				if options.OnPush != nil {
					options.OnPush(&payload)
				}
			}

			return WebhookResponse{
				Status: http.StatusOK,
				Body:   map[string]string{"status": "received"},
			}, nil
		},
	}
}

// GmailHandlerOptions configures the Gmail webhook handler
type GmailHandlerOptions struct {
	OnNewMessage func(notification *GmailNotification)
}

// CreateGmailHandler creates a Gmail Pub/Sub webhook handler
func CreateGmailHandler(options GmailHandlerOptions) WebhookConfig {
	return WebhookConfig{
		Path:        "/webhook/gmail",
		Method:      http.MethodPost,
		Timeout:     30 * time.Second,
		Description: "Gmail Pub/Sub webhook for new email notifications",
		Handler: func(params WebhookParams) (WebhookResponse, error) {
			// Parse Pub/Sub message
			payloadBytes, err := json.Marshal(params.Body)
			if err != nil {
				return WebhookResponse{
					Status: http.StatusBadRequest,
					Body:   map[string]string{"error": "Invalid payload"},
				}, nil
			}

			var pubsubMsg GmailPubSubMessage
			if err := json.Unmarshal(payloadBytes, &pubsubMsg); err != nil {
				return WebhookResponse{
					Status: http.StatusBadRequest,
					Body:   map[string]string{"error": "Failed to parse Pub/Sub message"},
				}, nil
			}

			// Decode base64 data
			data, err := base64.StdEncoding.DecodeString(pubsubMsg.Message.Data)
			if err != nil {
				return WebhookResponse{
					Status: http.StatusBadRequest,
					Body:   map[string]string{"error": "Failed to decode message data"},
				}, nil
			}

			// Parse Gmail notification
			var notification GmailNotification
			if err := json.Unmarshal(data, &notification); err != nil {
				return WebhookResponse{
					Status: http.StatusBadRequest,
					Body:   map[string]string{"error": "Failed to parse Gmail notification"},
				}, nil
			}

			// Handle new message
			if notification.HistoryID > 0 && options.OnNewMessage != nil {
				options.OnNewMessage(&notification)
			}

			return WebhookResponse{
				Status: http.StatusOK,
				Body:   map[string]string{"status": "received"},
			}, nil
		},
	}
}

// TelegramHandlerOptions configures the Telegram webhook handler
type TelegramHandlerOptions struct {
	OnMessage       func(message *TelegramMessage)
	OnCallbackQuery func(query *TelegramCallbackQuery)
}

// CreateTelegramHandler creates a Telegram webhook handler
func CreateTelegramHandler(options TelegramHandlerOptions) WebhookConfig {
	return WebhookConfig{
		Path:        "/webhook/telegram",
		Method:      http.MethodPost,
		Timeout:     30 * time.Second,
		Description: "Telegram webhook for bot updates",
		Handler: func(params WebhookParams) (WebhookResponse, error) {
			// Parse update
			payloadBytes, err := json.Marshal(params.Body)
			if err != nil {
				return WebhookResponse{
					Status: http.StatusBadRequest,
					Body:   map[string]string{"error": "Invalid payload"},
				}, nil
			}

			var update TelegramUpdate
			if err := json.Unmarshal(payloadBytes, &update); err != nil {
				return WebhookResponse{
					Status: http.StatusBadRequest,
					Body:   map[string]string{"error": "Failed to parse update"},
				}, nil
			}

			// Handle message
			if update.Message != nil && options.OnMessage != nil {
				options.OnMessage(update.Message)
			}

			// Handle callback query
			if update.CallbackQuery != nil && options.OnCallbackQuery != nil {
				options.OnCallbackQuery(update.CallbackQuery)
			}

			return WebhookResponse{
				Status: http.StatusOK,
				Body:   map[string]string{"status": "received"},
			}, nil
		},
	}
}

// CreateCustomHandler creates a custom webhook handler
func CreateCustomHandler(path string, method string, handler WebhookHandler, options ...func(*WebhookConfig)) WebhookConfig {
	config := WebhookConfig{
		Path:    path,
		Method:  method,
		Handler: handler,
		Timeout: 30 * time.Second,
	}

	// Apply options
	for _, opt := range options {
		opt(&config)
	}

	return config
}

// WithSecret sets the webhook secret for signature verification
func WithSecret(secret string, algorithm string, header string) func(*WebhookConfig) {
	return func(config *WebhookConfig) {
		config.Secret = secret
		config.SignatureAlgorithm = algorithm
		config.SignatureHeader = header
	}
}

// WithTimeout sets the webhook handler timeout
func WithTimeout(timeout time.Duration) func(*WebhookConfig) {
	return func(config *WebhookConfig) {
		config.Timeout = timeout
	}
}

// WithDescription sets the webhook description
func WithDescription(description string) func(*WebhookConfig) {
	return func(config *WebhookConfig) {
		config.Description = description
	}
}
