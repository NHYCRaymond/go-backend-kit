# DingTalk SDK for Go

Go SDK for DingTalk (钉钉) robot webhook messaging with template support.

## Features

- ✅ All message types support (Text, Link, Markdown, ActionCard, FeedCard)
- ✅ Security signature verification
- ✅ Built-in message templates (Alert, Error, Warning, Success, Info, Notify)
- ✅ Custom template support
- ✅ @ mentions support
- ✅ Resty HTTP client integration
- ✅ Automatic keyword injection (default: "球球雷达")

## Installation

```bash
go get github.com/yourusername/go-backend-kit/dingtalk
```

## Quick Start

### Basic Usage

```go
package main

import (
    "github.com/yourusername/go-backend-kit/dingtalk"
    "time"
)

func main() {
    // Create client
    client := dingtalk.NewClient(dingtalk.ClientOption{
        AccessToken: "YOUR_ACCESS_TOKEN",
        Secret:      "YOUR_SECRET", // Optional, for signature verification
        Timeout:     10 * time.Second,
    })

    // Send text message
    err := client.SendText("Hello from DingTalk SDK!", nil)
    
    // Send text with @ mentions
    at := &dingtalk.At{
        AtMobiles: []string{"13800138000"},
        IsAtAll:   false,
    }
    err = client.SendText("Important message!", at)
    
    // Send markdown message
    markdown := `## Project Status
- ✅ Development: Complete
- 🚧 Testing: In Progress
- ⏳ Deployment: Pending`
    
    err = client.SendMarkdown("Daily Report", markdown, nil)
}
```

### Using Templates

```go
// Create template manager
tm := dingtalk.NewTemplateManager(client)

// Send alert
err := tm.SendAlert(
    "Service Alert",
    "API response time exceeded threshold",
    "High",
    "api-service",
    "production",
    map[string]interface{}{
        "Endpoint": "/api/v1/users",
        "Response Time": "3.5s",
        "Threshold": "1s",
    },
    &dingtalk.At{IsAtAll: true},
)

// Send success notification
err = tm.SendSuccess(
    "Deployment Complete",
    "Version 1.2.3 deployed successfully",
    "payment-service",
    "production",
    map[string]interface{}{
        "Version": "v1.2.3",
        "Deploy Time": "2024-01-20 15:30:00",
    },
)
```

### Custom Templates

```go
// Register custom template
customTemplate := `## 📊 {{.Title}}
**Date**: {{.Date}}
**Total Sales**: ${{.TotalSales}}
**Orders**: {{.OrderCount}}`

tm.RegisterTemplate(dingtalk.TplCustom, customTemplate)

// Use custom template
err := tm.SendWithTemplate(dingtalk.TplCustom, dingtalk.TemplateData{
    Title: "Sales Report",
    Extra: map[string]interface{}{
        "Date": "2024-01-20",
        "TotalSales": "10,000",
        "OrderCount": 150,
    },
}, nil)

// Or use inline custom template
err = tm.SendCustomTemplate(
    "Task: {{.Task}}\nDeadline: {{.Deadline}}",
    map[string]interface{}{
        "Task": "Complete Q1 Report",
        "Deadline": "2024-01-25",
    },
    nil,
)
```

## Message Types

### Text Message
```go
client.SendText("Simple text message", nil)
```

### Link Message
```go
client.SendLink(
    "Article Title",
    "Article description...",
    "https://example.com/article",
    "https://example.com/cover.jpg", // Optional
)
```

### Markdown Message
```go
markdown := `## Header
- Item 1
- Item 2
> Quote`

client.SendMarkdown("Title", markdown, nil)
```

### ActionCard Message
```go
// Single button
card := &dingtalk.ActionCard{
    Title:       "Card Title",
    Text:        "Card content...",
    SingleTitle: "Read More",
    SingleURL:   "https://example.com",
}
client.SendActionCard(card)

// Multiple buttons
card := &dingtalk.ActionCard{
    Title:          "Card Title",
    Text:           "Card content...",
    BtnOrientation: "0", // 0: vertical, 1: horizontal
    Btns: []dingtalk.ActionBtn{
        {Title: "Accept", ActionURL: "https://example.com/accept"},
        {Title: "Reject", ActionURL: "https://example.com/reject"},
    },
}
client.SendActionCard(card)
```

### FeedCard Message
```go
links := []dingtalk.FeedLink{
    {
        Title:      "Article 1",
        MessageURL: "https://example.com/1",
        PicURL:     "https://example.com/pic1.jpg",
    },
    {
        Title:      "Article 2",
        MessageURL: "https://example.com/2",
        PicURL:     "https://example.com/pic2.jpg",
    },
}
client.SendFeedCard(links)
```

## Built-in Templates

| Template | Method | Use Case |
|----------|--------|----------|
| Alert | `SendAlert()` | System alerts and warnings |
| Error | `SendError()` | Error notifications |
| Warning | `SendWarning()` | Warning messages |
| Success | `SendSuccess()` | Success confirmations |
| Info | `SendInfo()` | Information messages |
| Notify | `SendNotify()` | General notifications |

## Configuration

### With Signature Verification

```go
client := dingtalk.NewClient(dingtalk.ClientOption{
    AccessToken: "YOUR_ACCESS_TOKEN",
    Secret:      "YOUR_SECRET", // Enable signature verification
    Timeout:     10 * time.Second,
    Keywords:    []string{"球球雷达", "监控"}, // Custom keywords (default: ["球球雷达"])
})
```

### Keywords

DingTalk robots require messages to contain specific keywords. This SDK automatically adds keywords if they're not present:

- Default keyword: "球球雷达"
- You can specify custom keywords in ClientOption
- Keywords are automatically added to messages if not already present
- The keyword is added in a subtle format at the end of the message

### @ Mentions

```go
at := &dingtalk.At{
    AtMobiles: []string{"13800138000", "13900139000"}, // @ specific users
    AtUserIds: []string{"user123"},                    // @ by user ID
    IsAtAll:   true,                                   // @ everyone
}
```

## Error Handling

```go
err := client.SendText("message", nil)
if err != nil {
    // API errors will include error code and message
    // e.g., "dingtalk api error: code=300001, msg=keywords not in content"
    log.Printf("Failed to send message: %v", err)
}
```

## Testing

```bash
# Run tests
go test ./dingtalk

# Run with coverage
go test -cover ./dingtalk
```

## License

Part of Go Backend Kit project.