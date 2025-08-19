package dingtalk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
)

func TestNewClient(t *testing.T) {
	option := ClientOption{
		AccessToken: "test_token",
		Secret:      "test_secret",
		Timeout:     5 * time.Second,
	}

	client := NewClient(option)

	if client.accessToken != "test_token" {
		t.Errorf("Expected access token 'test_token', got '%s'", client.accessToken)
	}

	if client.secret != "test_secret" {
		t.Errorf("Expected secret 'test_secret', got '%s'", client.secret)
	}

	if client.webhookURL != webhookURL {
		t.Errorf("Expected webhook URL '%s', got '%s'", webhookURL, client.webhookURL)
	}
}

func TestSendText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		var msg Message
		json.NewDecoder(r.Body).Decode(&msg)

		if msg.MsgType != MsgTypeText {
			t.Errorf("Expected message type 'text', got '%s'", msg.MsgType)
		}

		if msg.Text.Content != "test message" {
			t.Errorf("Expected content 'test message', got '%s'", msg.Text.Content)
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

	err := client.SendText("test message", nil)
	if err != nil {
		t.Errorf("SendText failed: %v", err)
	}
}

func TestSendTextWithAt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg Message
		json.NewDecoder(r.Body).Decode(&msg)

		if msg.At == nil {
			t.Error("Expected At field to be set")
		}

		if len(msg.At.AtMobiles) != 2 {
			t.Errorf("Expected 2 at mobiles, got %d", len(msg.At.AtMobiles))
		}

		if !msg.At.IsAtAll {
			t.Error("Expected IsAtAll to be true")
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

	at := &At{
		AtMobiles: []string{"13800000000", "13900000000"},
		IsAtAll:   true,
	}

	err := client.SendText("test message with at", at)
	if err != nil {
		t.Errorf("SendText with At failed: %v", err)
	}
}

func TestSendLink(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg Message
		json.NewDecoder(r.Body).Decode(&msg)

		if msg.MsgType != MsgTypeLink {
			t.Errorf("Expected message type 'link', got '%s'", msg.MsgType)
		}

		if msg.Link.Title != "Test Title" {
			t.Errorf("Expected title 'Test Title', got '%s'", msg.Link.Title)
		}

		if msg.Link.MessageURL != "https://example.com" {
			t.Errorf("Expected messageURL 'https://example.com', got '%s'", msg.Link.MessageURL)
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

	err := client.SendLink("Test Title", "Test Text", "https://example.com", "")
	if err != nil {
		t.Errorf("SendLink failed: %v", err)
	}
}

func TestSendMarkdown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg Message
		json.NewDecoder(r.Body).Decode(&msg)

		if msg.MsgType != MsgTypeMarkdown {
			t.Errorf("Expected message type 'markdown', got '%s'", msg.MsgType)
		}

		if msg.Markdown.Title != "Markdown Title" {
			t.Errorf("Expected title 'Markdown Title', got '%s'", msg.Markdown.Title)
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

	markdown := "## Header\n- Item 1\n- Item 2"
	err := client.SendMarkdown("Markdown Title", markdown, nil)
	if err != nil {
		t.Errorf("SendMarkdown failed: %v", err)
	}
}

func TestSendActionCard(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg Message
		json.NewDecoder(r.Body).Decode(&msg)

		if msg.MsgType != MsgTypeActionCard {
			t.Errorf("Expected message type 'actionCard', got '%s'", msg.MsgType)
		}

		if msg.ActionCard.Title != "Action Card Title" {
			t.Errorf("Expected title 'Action Card Title', got '%s'", msg.ActionCard.Title)
		}

		if msg.ActionCard.SingleTitle != "Read More" {
			t.Errorf("Expected single title 'Read More', got '%s'", msg.ActionCard.SingleTitle)
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

	card := &ActionCard{
		Title:       "Action Card Title",
		Text:        "Action Card Text",
		SingleTitle: "Read More",
		SingleURL:   "https://example.com",
	}

	err := client.SendActionCard(card)
	if err != nil {
		t.Errorf("SendActionCard failed: %v", err)
	}
}

func TestSendActionCardWithMultipleButtons(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg Message
		json.NewDecoder(r.Body).Decode(&msg)

		if len(msg.ActionCard.Btns) != 2 {
			t.Errorf("Expected 2 buttons, got %d", len(msg.ActionCard.Btns))
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

	card := &ActionCard{
		Title:          "Multi Button Card",
		Text:           "Card with multiple buttons",
		BtnOrientation: "0",
		Btns: []ActionBtn{
			{Title: "Button 1", ActionURL: "https://example.com/1"},
			{Title: "Button 2", ActionURL: "https://example.com/2"},
		},
	}

	err := client.SendActionCard(card)
	if err != nil {
		t.Errorf("SendActionCard with multiple buttons failed: %v", err)
	}
}

func TestSendFeedCard(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg Message
		json.NewDecoder(r.Body).Decode(&msg)

		if msg.MsgType != MsgTypeFeedCard {
			t.Errorf("Expected message type 'feedCard', got '%s'", msg.MsgType)
		}

		if len(msg.FeedCard.Links) != 2 {
			t.Errorf("Expected 2 feed links, got %d", len(msg.FeedCard.Links))
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

	links := []FeedLink{
		{
			Title:      "Link 1",
			MessageURL: "https://example.com/1",
			PicURL:     "https://example.com/pic1.jpg",
		},
		{
			Title:      "Link 2",
			MessageURL: "https://example.com/2",
			PicURL:     "https://example.com/pic2.jpg",
		},
	}

	err := client.SendFeedCard(links)
	if err != nil {
		t.Errorf("SendFeedCard failed: %v", err)
	}
}

func TestSendWithError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := Response{
			ErrCode: 300001,
			ErrMsg:  "keywords not in content",
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		accessToken: "test_token",
		webhookURL:  server.URL,
		httpClient:  resty.New(),
	}

	err := client.SendText("test message", nil)
	if err == nil {
		t.Error("Expected error, got nil")
	}

	expectedError := "dingtalk api error: code=300001, msg=keywords not in content"
	if err.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
	}
}

func TestBuildURLWithSecret(t *testing.T) {
	client := &Client{
		accessToken: "test_token",
		secret:      "test_secret",
		webhookURL:  webhookURL,
	}

	url := client.buildURL()

	if url == "" {
		t.Error("Expected non-empty URL")
	}

	if !contains(url, "access_token=test_token") {
		t.Error("URL should contain access_token parameter")
	}

	if !contains(url, "timestamp=") {
		t.Error("URL should contain timestamp parameter when secret is set")
	}

	if !contains(url, "sign=") {
		t.Error("URL should contain sign parameter when secret is set")
	}
}

func TestBuildURLWithoutSecret(t *testing.T) {
	client := &Client{
		accessToken: "test_token",
		secret:      "",
		webhookURL:  webhookURL,
	}

	url := client.buildURL()

	if !contains(url, "access_token=test_token") {
		t.Error("URL should contain access_token parameter")
	}

	if contains(url, "timestamp=") {
		t.Error("URL should not contain timestamp parameter when secret is not set")
	}

	if contains(url, "sign=") {
		t.Error("URL should not contain sign parameter when secret is not set")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[len(s)-len(substr):] == substr || 
		   len(s) >= len(substr) && s[:len(substr)] == substr ||
		   len(s) > len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}