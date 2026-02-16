package webhook_test

import (
	"fmt"
	"net/http"
	"os"

	"github.com/harun/ranya/pkg/agent"
	"github.com/harun/ranya/pkg/commandqueue"
	"github.com/harun/ranya/pkg/webhook"
	"github.com/rs/zerolog"
)

// Example demonstrates basic webhook server usage
func Example() {
	// Create dependencies
	logger := zerolog.New(os.Stdout).Level(zerolog.Disabled)
	cq := commandqueue.New()
	runner := &agent.Runner{}

	// Create webhook server
	options := webhook.ServerOptions{
		Port:               3001,
		Host:               "0.0.0.0",
		RateLimitPerMinute: 100,
	}

	server, err := webhook.NewServer(options, cq, runner, nil, logger)
	if err != nil {
		panic(err)
	}

	// Register a custom webhook
	config := webhook.WebhookConfig{
		Path:   "/webhook/custom",
		Method: http.MethodPost,
		Handler: func(params webhook.WebhookParams) (webhook.WebhookResponse, error) {
			fmt.Printf("Received webhook: %v\n", params.Body)
			return webhook.WebhookResponse{
				Status: http.StatusOK,
				Body:   map[string]string{"status": "received"},
			}, nil
		},
	}

	if err := server.RegisterWebhook(config); err != nil {
		panic(err)
	}

	// Start server (in production, this would be in a goroutine)
	// server.Start()

	fmt.Println("Webhook server configured")
	// Output: Webhook server configured
}

// ExampleCreateGitHubHandler demonstrates GitHub webhook setup
func ExampleCreateGitHubHandler() {
	handler := webhook.CreateGitHubHandler(webhook.GitHubHandlerOptions{
		Secret: "github-webhook-secret",
		OnPullRequest: func(pr *webhook.GitHubPullRequest) {
			fmt.Printf("PR opened: #%d - %s\n", pr.Number, pr.Title)
		},
		OnIssue: func(issue *webhook.GitHubIssue) {
			fmt.Printf("Issue created: #%d - %s\n", issue.Number, issue.Title)
		},
		OnPush: func(payload *webhook.GitHubWebhookPayload) {
			fmt.Printf("Push to %s\n", payload.Ref)
		},
	})

	fmt.Printf("GitHub webhook configured at %s\n", handler.Path)
	// Output: GitHub webhook configured at /webhook/github
}

// ExampleCreateGmailHandler demonstrates Gmail webhook setup
func ExampleCreateGmailHandler() {
	handler := webhook.CreateGmailHandler(webhook.GmailHandlerOptions{
		OnNewMessage: func(notification *webhook.GmailNotification) {
			fmt.Printf("New email for %s\n", notification.EmailAddress)
		},
	})

	fmt.Printf("Gmail webhook configured at %s\n", handler.Path)
	// Output: Gmail webhook configured at /webhook/gmail
}

// ExampleCreateTelegramHandler demonstrates Telegram webhook setup
func ExampleCreateTelegramHandler() {
	handler := webhook.CreateTelegramHandler(webhook.TelegramHandlerOptions{
		OnMessage: func(message *webhook.TelegramMessage) {
			fmt.Printf("Message from %s: %s\n", message.From.Username, message.Text)
		},
		OnCallbackQuery: func(query *webhook.TelegramCallbackQuery) {
			fmt.Printf("Callback query: %s\n", query.Data)
		},
	})

	fmt.Printf("Telegram webhook configured at %s\n", handler.Path)
	// Output: Telegram webhook configured at /webhook/telegram
}

// ExampleCreateCustomHandler demonstrates custom webhook with options
func ExampleCreateCustomHandler() {
	handler := webhook.CreateCustomHandler(
		"/webhook/stripe",
		http.MethodPost,
		func(params webhook.WebhookParams) (webhook.WebhookResponse, error) {
			return webhook.WebhookResponse{Status: http.StatusOK}, nil
		},
		webhook.WithSecret("stripe-webhook-secret", "sha256", "Stripe-Signature"),
		webhook.WithDescription("Stripe payment webhook"),
	)

	fmt.Printf("Custom webhook configured at %s with signature verification\n", handler.Path)
	// Output: Custom webhook configured at /webhook/stripe with signature verification
}
