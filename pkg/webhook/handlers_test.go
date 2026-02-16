package webhook

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCreateGitHubHandler(t *testing.T) {
	prCalled := false
	issueCalled := false
	pushCalled := false

	handler := CreateGitHubHandler(GitHubHandlerOptions{
		Secret: "my-secret",
		OnPullRequest: func(pr *GitHubPullRequest) {
			prCalled = true
			assert.Equal(t, 123, pr.Number)
			assert.Equal(t, "Test PR", pr.Title)
		},
		OnIssue: func(issue *GitHubIssue) {
			issueCalled = true
			assert.Equal(t, 456, issue.Number)
			assert.Equal(t, "Test Issue", issue.Title)
		},
		OnPush: func(payload *GitHubWebhookPayload) {
			pushCalled = true
			assert.Equal(t, "refs/heads/main", payload.Ref)
		},
	})

	assert.Equal(t, "/webhook/github", handler.Path)
	assert.Equal(t, http.MethodPost, handler.Method)
	assert.Equal(t, "my-secret", handler.Secret)
	assert.Equal(t, "X-Hub-Signature-256", handler.SignatureHeader)
	assert.Equal(t, "sha256", handler.SignatureAlgorithm)

	// Test pull request event
	prPayload := GitHubWebhookPayload{
		Action: "opened",
		PullRequest: &GitHubPullRequest{
			Number: 123,
			Title:  "Test PR",
		},
	}
	prBody, _ := json.Marshal(prPayload)
	var prBodyInterface interface{}
	json.Unmarshal(prBody, &prBodyInterface)

	response, err := handler.Handler(WebhookParams{
		Body: prBodyInterface,
		Headers: map[string]string{
			"X-GitHub-Event": "pull_request",
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, response.Status)
	assert.True(t, prCalled)

	// Test issue event
	issuePayload := GitHubWebhookPayload{
		Action: "opened",
		Issue: &GitHubIssue{
			Number: 456,
			Title:  "Test Issue",
		},
	}
	issueBody, _ := json.Marshal(issuePayload)
	var issueBodyInterface interface{}
	json.Unmarshal(issueBody, &issueBodyInterface)

	response, err = handler.Handler(WebhookParams{
		Body: issueBodyInterface,
		Headers: map[string]string{
			"X-GitHub-Event": "issues",
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, response.Status)
	assert.True(t, issueCalled)

	// Test push event
	pushPayload := GitHubWebhookPayload{
		Ref: "refs/heads/main",
	}
	pushBody, _ := json.Marshal(pushPayload)
	var pushBodyInterface interface{}
	json.Unmarshal(pushBody, &pushBodyInterface)

	response, err = handler.Handler(WebhookParams{
		Body: pushBodyInterface,
		Headers: map[string]string{
			"X-GitHub-Event": "push",
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, response.Status)
	assert.True(t, pushCalled)
}

func TestCreateGitHubHandlerMissingEvent(t *testing.T) {
	handler := CreateGitHubHandler(GitHubHandlerOptions{
		Secret: "my-secret",
	})

	response, err := handler.Handler(WebhookParams{
		Body:    map[string]interface{}{},
		Headers: map[string]string{},
	})
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, response.Status)
}

func TestCreateGmailHandler(t *testing.T) {
	called := false
	handler := CreateGmailHandler(GmailHandlerOptions{
		OnNewMessage: func(notification *GmailNotification) {
			called = true
			assert.Equal(t, "user@example.com", notification.EmailAddress)
			assert.Equal(t, int64(12345), notification.HistoryID)
		},
	})

	assert.Equal(t, "/webhook/gmail", handler.Path)
	assert.Equal(t, http.MethodPost, handler.Method)

	// Create Gmail notification
	notification := GmailNotification{
		EmailAddress: "user@example.com",
		HistoryID:    12345,
	}
	notificationJSON, _ := json.Marshal(notification)
	encodedData := base64.StdEncoding.EncodeToString(notificationJSON)

	// Create Pub/Sub message
	pubsubMsg := GmailPubSubMessage{
		Message: struct {
			Data        string `json:"data"`
			MessageID   string `json:"messageId"`
			PublishTime string `json:"publishTime"`
		}{
			Data:      encodedData,
			MessageID: "msg-123",
		},
	}
	pubsubBody, _ := json.Marshal(pubsubMsg)
	var pubsubBodyInterface interface{}
	json.Unmarshal(pubsubBody, &pubsubBodyInterface)

	response, err := handler.Handler(WebhookParams{
		Body: pubsubBodyInterface,
	})
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, response.Status)
	assert.True(t, called)
}

func TestCreateTelegramHandler(t *testing.T) {
	messageCalled := false
	callbackCalled := false

	handler := CreateTelegramHandler(TelegramHandlerOptions{
		OnMessage: func(message *TelegramMessage) {
			messageCalled = true
			assert.Equal(t, "Hello", message.Text)
		},
		OnCallbackQuery: func(query *TelegramCallbackQuery) {
			callbackCalled = true
			assert.Equal(t, "callback-data", query.Data)
		},
	})

	assert.Equal(t, "/webhook/telegram", handler.Path)
	assert.Equal(t, http.MethodPost, handler.Method)

	// Test message update
	messageUpdate := TelegramUpdate{
		UpdateID: 1,
		Message: &TelegramMessage{
			MessageID: 123,
			Text:      "Hello",
			Chat: &TelegramChat{
				ID:   456,
				Type: "private",
			},
		},
	}
	messageBody, _ := json.Marshal(messageUpdate)
	var messageBodyInterface interface{}
	json.Unmarshal(messageBody, &messageBodyInterface)

	response, err := handler.Handler(WebhookParams{
		Body: messageBodyInterface,
	})
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, response.Status)
	assert.True(t, messageCalled)

	// Test callback query update
	callbackUpdate := TelegramUpdate{
		UpdateID: 2,
		CallbackQuery: &TelegramCallbackQuery{
			ID:   "query-123",
			Data: "callback-data",
			From: &TelegramUser{
				ID:        789,
				FirstName: "John",
			},
		},
	}
	callbackBody, _ := json.Marshal(callbackUpdate)
	var callbackBodyInterface interface{}
	json.Unmarshal(callbackBody, &callbackBodyInterface)

	response, err = handler.Handler(WebhookParams{
		Body: callbackBodyInterface,
	})
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, response.Status)
	assert.True(t, callbackCalled)
}

func TestCreateCustomHandler(t *testing.T) {
	called := false
	handler := CreateCustomHandler(
		"/webhook/custom",
		http.MethodPost,
		func(params WebhookParams) (WebhookResponse, error) {
			called = true
			return WebhookResponse{Status: http.StatusOK}, nil
		},
		WithSecret("my-secret", "sha256", "X-Custom-Signature"),
		WithTimeout(60*1000),
		WithDescription("Custom webhook"),
	)

	assert.Equal(t, "/webhook/custom", handler.Path)
	assert.Equal(t, http.MethodPost, handler.Method)
	assert.Equal(t, "my-secret", handler.Secret)
	assert.Equal(t, "sha256", handler.SignatureAlgorithm)
	assert.Equal(t, "X-Custom-Signature", handler.SignatureHeader)
	assert.Equal(t, "Custom webhook", handler.Description)

	response, err := handler.Handler(WebhookParams{})
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, response.Status)
	assert.True(t, called)
}

func TestWithSecretOption(t *testing.T) {
	config := WebhookConfig{}
	opt := WithSecret("test-secret", "sha1", "X-Test-Signature")
	opt(&config)

	assert.Equal(t, "test-secret", config.Secret)
	assert.Equal(t, "sha1", config.SignatureAlgorithm)
	assert.Equal(t, "X-Test-Signature", config.SignatureHeader)
}

func TestWithTimeoutOption(t *testing.T) {
	config := WebhookConfig{}
	opt := WithTimeout(45 * time.Second)
	opt(&config)

	assert.Equal(t, 45*time.Second, config.Timeout)
}

func TestWithDescriptionOption(t *testing.T) {
	config := WebhookConfig{}
	opt := WithDescription("Test description")
	opt(&config)

	assert.Equal(t, "Test description", config.Description)
}
