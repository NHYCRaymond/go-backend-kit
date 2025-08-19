package dingtalk

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

const (
	webhookURL = "https://oapi.dingtalk.com/robot/send"
	defaultTimeout = 10 * time.Second
)

type Client struct {
	accessToken string
	secret      string
	webhookURL  string
	httpClient  *resty.Client
	keywords    []string
}

func NewClient(option ClientOption) *Client {
	timeout := option.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	httpClient := resty.New().
		SetTimeout(timeout).
		SetHeader("Content-Type", "application/json; charset=utf-8")

	// 如果没有指定关键词，使用默认关键词"球球雷达"
	keywords := option.Keywords
	if len(keywords) == 0 {
		keywords = []string{"球球雷达"}
	}

	return &Client{
		accessToken: option.AccessToken,
		secret:      option.Secret,
		webhookURL:  webhookURL,
		httpClient:  httpClient,
		keywords:    keywords,
	}
}

func (c *Client) Send(msg *Message) error {
	targetURL := c.buildURL()
	
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message failed: %w", err)
	}

	resp, err := c.httpClient.R().
		SetBody(body).
		SetResult(&Response{}).
		Post(targetURL)
	
	if err != nil {
		return fmt.Errorf("send request failed: %w", err)
	}

	result := resp.Result().(*Response)
	if result.ErrCode != 0 {
		return fmt.Errorf("dingtalk api error: code=%d, msg=%s", result.ErrCode, result.ErrMsg)
	}

	return nil
}

func (c *Client) SendText(content string, at *At) error {
	// 自动添加关键词（如果内容中还没有）
	content = c.ensureKeyword(content)
	
	msg := &Message{
		MsgType: MsgTypeText,
		Text: &Text{
			Content: content,
		},
		At: at,
	}
	return c.Send(msg)
}

func (c *Client) SendLink(title, text, messageURL string, picURL string) error {
	// 自动添加关键词到text
	text = c.ensureKeyword(text)
	
	msg := &Message{
		MsgType: MsgTypeLink,
		Link: &Link{
			Title:      title,
			Text:       text,
			MessageURL: messageURL,
			PicURL:     picURL,
		},
	}
	return c.Send(msg)
}

func (c *Client) SendMarkdown(title, text string, at *At) error {
	// 自动添加关键词到text
	text = c.ensureKeyword(text)
	
	msg := &Message{
		MsgType: MsgTypeMarkdown,
		Markdown: &Markdown{
			Title: title,
			Text:  text,
		},
		At: at,
	}
	return c.Send(msg)
}

func (c *Client) SendActionCard(card *ActionCard) error {
	// 自动添加关键词到text
	card.Text = c.ensureKeyword(card.Text)
	
	msg := &Message{
		MsgType:    MsgTypeActionCard,
		ActionCard: card,
	}
	return c.Send(msg)
}

func (c *Client) SendFeedCard(links []FeedLink) error {
	msg := &Message{
		MsgType: MsgTypeFeedCard,
		FeedCard: &FeedCard{
			Links: links,
		},
	}
	return c.Send(msg)
}

func (c *Client) buildURL() string {
	params := url.Values{}
	params.Add("access_token", c.accessToken)
	
	if c.secret != "" {
		timestamp := time.Now().UnixMilli()
		sign := c.sign(timestamp)
		params.Add("timestamp", fmt.Sprintf("%d", timestamp))
		params.Add("sign", sign)
	}
	
	return fmt.Sprintf("%s?%s", c.webhookURL, params.Encode())
}

func (c *Client) sign(timestamp int64) string {
	stringToSign := fmt.Sprintf("%d\n%s", timestamp, c.secret)
	h := hmac.New(sha256.New, []byte(c.secret))
	h.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func (c *Client) ensureKeyword(content string) string {
	// 检查内容中是否已包含任何关键词
	for _, keyword := range c.keywords {
		if strings.Contains(content, keyword) {
			return content
		}
	}
	
	// 如果没有包含关键词，添加第一个关键词
	if len(c.keywords) > 0 {
		// 在内容末尾添加关键词，用特殊格式使其不太明显
		return fmt.Sprintf("%s\n\n[%s]", content, c.keywords[0])
	}
	
	return content
}