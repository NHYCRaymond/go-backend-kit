package dingtalk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-resty/resty/v2"
)

func TestTemplateManagerInitialization(t *testing.T) {
	client := &Client{
		accessToken: "test_token",
		webhookURL:  "https://example.com",
		httpClient:  resty.New(),
	}

	tm := NewTemplateManager(client)

	expectedTemplates := []TemplateType{
		TplAlert, TplNotify, TplSuccess, TplError, TplWarning, TplInfo,
	}

	for _, tplType := range expectedTemplates {
		if _, exists := tm.templates[tplType]; !exists {
			t.Errorf("Expected template %s to be initialized", tplType)
		}
	}
}

func TestRegisterCustomTemplate(t *testing.T) {
	client := &Client{
		accessToken: "test_token",
		webhookURL:  "https://example.com",
		httpClient:  resty.New(),
	}

	tm := NewTemplateManager(client)

	customTemplate := `## Custom Template
Title: {{.Title}}
Content: {{.Content}}`

	err := tm.RegisterTemplate(TplCustom, customTemplate)
	if err != nil {
		t.Errorf("Failed to register custom template: %v", err)
	}

	if _, exists := tm.templates[TplCustom]; !exists {
		t.Error("Custom template was not registered")
	}
}

func TestRegisterInvalidTemplate(t *testing.T) {
	client := &Client{
		accessToken: "test_token",
		webhookURL:  "https://example.com",
		httpClient:  resty.New(),
	}

	tm := NewTemplateManager(client)

	invalidTemplate := `## Invalid Template
{{.UnclosedVariable`

	err := tm.RegisterTemplate(TplCustom, invalidTemplate)
	if err == nil {
		t.Error("Expected error for invalid template, got nil")
	}
}

func TestSendAlert(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg Message
		json.NewDecoder(r.Body).Decode(&msg)

		if msg.MsgType != MsgTypeMarkdown {
			t.Errorf("Expected message type 'markdown', got '%s'", msg.MsgType)
		}

		if !strings.Contains(msg.Markdown.Text, "🚨") {
			t.Error("Alert template should contain alert emoji")
		}

		if !strings.Contains(msg.Markdown.Text, "test-service") {
			t.Error("Alert should contain service name")
		}

		response := Response{ErrCode: 0, ErrMsg: "ok"}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		accessToken: "test_token",
		webhookURL:  server.URL,
		httpClient:  resty.New(),
	}

	tm := NewTemplateManager(client)

	err := tm.SendAlert(
		"Test Alert",
		"This is a test alert",
		"High",
		"test-service",
		"production",
		map[string]interface{}{
			"Error": "Connection timeout",
			"Count": 10,
		},
		nil,
	)

	if err != nil {
		t.Errorf("SendAlert failed: %v", err)
	}
}

func TestSendSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg Message
		json.NewDecoder(r.Body).Decode(&msg)

		if !strings.Contains(msg.Markdown.Text, "✅") {
			t.Error("Success template should contain success emoji")
		}

		response := Response{ErrCode: 0, ErrMsg: "ok"}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		accessToken: "test_token",
		webhookURL:  server.URL,
		httpClient:  resty.New(),
	}

	tm := NewTemplateManager(client)

	err := tm.SendSuccess(
		"Deployment Success",
		"Version 1.0.0 deployed",
		"api-service",
		"production",
		map[string]interface{}{
			"Version": "1.0.0",
			"Time":    "10:30:00",
		},
	)

	if err != nil {
		t.Errorf("SendSuccess failed: %v", err)
	}
}

func TestSendError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg Message
		json.NewDecoder(r.Body).Decode(&msg)

		if !strings.Contains(msg.Markdown.Text, "❌") {
			t.Error("Error template should contain error emoji")
		}

		response := Response{ErrCode: 0, ErrMsg: "ok"}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		accessToken: "test_token",
		webhookURL:  server.URL,
		httpClient:  resty.New(),
	}

	tm := NewTemplateManager(client)

	err := tm.SendError(
		"System Error",
		"Database connection failed",
		"db-service",
		"production",
		map[string]interface{}{
			"ErrorCode": "DB_CONN_FAILED",
			"Retries":   3,
		},
		nil,
	)

	if err != nil {
		t.Errorf("SendError failed: %v", err)
	}
}

func TestSendWarning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg Message
		json.NewDecoder(r.Body).Decode(&msg)

		if !strings.Contains(msg.Markdown.Text, "⚠️") {
			t.Error("Warning template should contain warning emoji")
		}

		response := Response{ErrCode: 0, ErrMsg: "ok"}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		accessToken: "test_token",
		webhookURL:  server.URL,
		httpClient:  resty.New(),
	}

	tm := NewTemplateManager(client)

	err := tm.SendWarning(
		"Performance Warning",
		"High CPU usage detected",
		"compute-service",
		"production",
		map[string]interface{}{
			"CPU": "85%",
			"Threshold": "80%",
		},
		nil,
	)

	if err != nil {
		t.Errorf("SendWarning failed: %v", err)
	}
}

func TestSendInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg Message
		json.NewDecoder(r.Body).Decode(&msg)

		if !strings.Contains(msg.Markdown.Text, "ℹ️") {
			t.Error("Info template should contain info emoji")
		}

		response := Response{ErrCode: 0, ErrMsg: "ok"}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		accessToken: "test_token",
		webhookURL:  server.URL,
		httpClient:  resty.New(),
	}

	tm := NewTemplateManager(client)

	err := tm.SendInfo(
		"System Info",
		"Daily backup completed",
		"backup-service",
		map[string]interface{}{
			"Size": "10GB",
			"Duration": "30 minutes",
		},
	)

	if err != nil {
		t.Errorf("SendInfo failed: %v", err)
	}
}

func TestSendNotify(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg Message
		json.NewDecoder(r.Body).Decode(&msg)

		if !strings.Contains(msg.Markdown.Text, "📢") {
			t.Error("Notify template should contain notify emoji")
		}

		response := Response{ErrCode: 0, ErrMsg: "ok"}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		accessToken: "test_token",
		webhookURL:  server.URL,
		httpClient:  resty.New(),
	}

	tm := NewTemplateManager(client)

	err := tm.SendNotify(
		"New User Registration",
		"A new user has registered",
		"user-service",
		map[string]interface{}{
			"UserID": "U10001",
			"Email": "test@example.com",
		},
		nil,
	)

	if err != nil {
		t.Errorf("SendNotify failed: %v", err)
	}
}

func TestSendCustomTemplate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg Message
		json.NewDecoder(r.Body).Decode(&msg)

		if msg.MsgType == MsgTypeMarkdown {
			if !strings.Contains(msg.Markdown.Text, "Custom Report") {
				t.Error("Custom template should contain custom content")
			}
		}

		response := Response{ErrCode: 0, ErrMsg: "ok"}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		accessToken: "test_token",
		webhookURL:  server.URL,
		httpClient:  resty.New(),
	}

	tm := NewTemplateManager(client)

	markdownTemplate := `## Custom Report
Name: {{.Name}}
Value: {{.Value}}`

	err := tm.SendCustomTemplate(markdownTemplate, map[string]interface{}{
		"Name":  "Test Report",
		"Value": "100",
	}, nil)

	if err != nil {
		t.Errorf("SendCustomTemplate failed: %v", err)
	}
}

func TestSendCustomTemplateAsText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg Message
		json.NewDecoder(r.Body).Decode(&msg)

		if msg.MsgType != MsgTypeText {
			t.Errorf("Expected message type 'text', got '%s'", msg.MsgType)
		}

		if !strings.Contains(msg.Text.Content, "Simple text message") {
			t.Error("Custom template should contain simple text")
		}

		response := Response{ErrCode: 0, ErrMsg: "ok"}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		accessToken: "test_token",
		webhookURL:  server.URL,
		httpClient:  resty.New(),
	}

	tm := NewTemplateManager(client)

	textTemplate := `Simple text message: {{.Message}}`

	err := tm.SendCustomTemplate(textTemplate, map[string]interface{}{
		"Message": "Hello World",
	}, nil)

	if err != nil {
		t.Errorf("SendCustomTemplate as text failed: %v", err)
	}
}

func TestSendWithTemplateNonExistent(t *testing.T) {
	client := &Client{
		accessToken: "test_token",
		webhookURL:  "https://example.com",
		httpClient:  resty.New(),
	}

	tm := NewTemplateManager(client)

	err := tm.SendWithTemplate("non_existent", TemplateData{}, nil)
	if err == nil {
		t.Error("Expected error for non-existent template, got nil")
	}

	if !strings.Contains(err.Error(), "template non_existent not found") {
		t.Errorf("Expected 'template not found' error, got: %v", err)
	}
}

func TestTemplateDataWithAutoTime(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg Message
		json.NewDecoder(r.Body).Decode(&msg)

		if !strings.Contains(msg.Markdown.Text, "20") {
			t.Error("Template should contain auto-generated timestamp")
		}

		response := Response{ErrCode: 0, ErrMsg: "ok"}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		accessToken: "test_token",
		webhookURL:  server.URL,
		httpClient:  resty.New(),
	}

	tm := NewTemplateManager(client)

	err := tm.SendWithTemplate(TplInfo, TemplateData{
		Title:   "Test",
		Content: "Test content",
		Service: "test-service",
	}, nil)

	if err != nil {
		t.Errorf("SendWithTemplate with auto time failed: %v", err)
	}
}