package dingtalk

import "time"

type MessageType string

const (
	MsgTypeText       MessageType = "text"
	MsgTypeLink       MessageType = "link"
	MsgTypeMarkdown   MessageType = "markdown"
	MsgTypeActionCard MessageType = "actionCard"
	MsgTypeFeedCard   MessageType = "feedCard"
)

type Message struct {
	MsgType MessageType `json:"msgtype"`
	Text    *Text       `json:"text,omitempty"`
	Link    *Link       `json:"link,omitempty"`
	Markdown *Markdown  `json:"markdown,omitempty"`
	ActionCard *ActionCard `json:"actionCard,omitempty"`
	FeedCard *FeedCard   `json:"feedCard,omitempty"`
	At       *At         `json:"at,omitempty"`
}

type Text struct {
	Content string `json:"content"`
}

type Link struct {
	Title      string `json:"title"`
	Text       string `json:"text"`
	MessageURL string `json:"messageUrl"`
	PicURL     string `json:"picUrl,omitempty"`
}

type Markdown struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

type ActionCard struct {
	Title          string       `json:"title"`
	Text           string       `json:"text"`
	SingleTitle    string       `json:"singleTitle,omitempty"`
	SingleURL      string       `json:"singleURL,omitempty"`
	BtnOrientation string       `json:"btnOrientation,omitempty"`
	Btns           []ActionBtn  `json:"btns,omitempty"`
}

type ActionBtn struct {
	Title     string `json:"title"`
	ActionURL string `json:"actionURL"`
}

type FeedCard struct {
	Links []FeedLink `json:"links"`
}

type FeedLink struct {
	Title      string `json:"title"`
	MessageURL string `json:"messageURL"`
	PicURL     string `json:"picURL"`
}

type At struct {
	AtMobiles []string `json:"atMobiles,omitempty"`
	AtUserIds []string `json:"atUserIds,omitempty"`
	IsAtAll   bool     `json:"isAtAll,omitempty"`
}

type Response struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

type ClientOption struct {
	AccessToken string
	Secret      string
	Timeout     time.Duration
	Keywords    []string // 关键词列表，消息中需要包含至少一个关键词
}