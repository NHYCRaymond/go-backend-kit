package dingtalk

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
	"time"
)

type TemplateType string

const (
	TplAlert   TemplateType = "alert"
	TplNotify  TemplateType = "notify"
	TplSuccess TemplateType = "success"
	TplError   TemplateType = "error"
	TplWarning TemplateType = "warning"
	TplInfo    TemplateType = "info"
	TplCustom  TemplateType = "custom"
)

type TemplateData struct {
	Title       string
	Content     string
	Level       string
	Time        string
	Service     string
	Environment string
	Details     map[string]interface{}
	Extra       map[string]interface{}
}

type TemplateManager struct {
	templates map[TemplateType]*template.Template
	client    *Client
}

func NewTemplateManager(client *Client) *TemplateManager {
	tm := &TemplateManager{
		templates: make(map[TemplateType]*template.Template),
		client:    client,
	}
	tm.initDefaultTemplates()
	return tm
}

func (tm *TemplateManager) initDefaultTemplates() {
	alertTemplate := `## 🚨 {{.Title}}
**级别**: {{.Level}}
**服务**: {{.Service}}
**环境**: {{.Environment}}
**时间**: {{.Time}}

### 详情
{{.Content}}

{{range $key, $value := .Details}}
**{{$key}}**: {{$value}}
{{end}}`

	notifyTemplate := `## 📢 {{.Title}}
**服务**: {{.Service}}
**时间**: {{.Time}}

{{.Content}}

{{range $key, $value := .Details}}
- **{{$key}}**: {{$value}}
{{end}}`

	successTemplate := `## ✅ {{.Title}}
**服务**: {{.Service}}
**环境**: {{.Environment}}
**时间**: {{.Time}}

{{.Content}}

{{range $key, $value := .Details}}
✓ {{$key}}: {{$value}}
{{end}}`

	errorTemplate := `## ❌ {{.Title}}
**级别**: 错误
**服务**: {{.Service}}
**环境**: {{.Environment}}
**时间**: {{.Time}}

### 错误信息
{{.Content}}

### 错误详情
{{range $key, $value := .Details}}
**{{$key}}**: {{$value}}
{{end}}`

	warningTemplate := `## ⚠️ {{.Title}}
**级别**: 警告
**服务**: {{.Service}}
**环境**: {{.Environment}}
**时间**: {{.Time}}

{{.Content}}

{{range $key, $value := .Details}}
⚠ {{$key}}: {{$value}}
{{end}}`

	infoTemplate := `## ℹ️ {{.Title}}
**服务**: {{.Service}}
**时间**: {{.Time}}

{{.Content}}

{{range $key, $value := .Details}}
• {{$key}}: {{$value}}
{{end}}`

	tm.RegisterTemplate(TplAlert, alertTemplate)
	tm.RegisterTemplate(TplNotify, notifyTemplate)
	tm.RegisterTemplate(TplSuccess, successTemplate)
	tm.RegisterTemplate(TplError, errorTemplate)
	tm.RegisterTemplate(TplWarning, warningTemplate)
	tm.RegisterTemplate(TplInfo, infoTemplate)
}

func (tm *TemplateManager) RegisterTemplate(tplType TemplateType, tplContent string) error {
	tmpl, err := template.New(string(tplType)).Parse(tplContent)
	if err != nil {
		return fmt.Errorf("parse template failed: %w", err)
	}
	tm.templates[tplType] = tmpl
	return nil
}

func (tm *TemplateManager) SendWithTemplate(tplType TemplateType, data TemplateData, at *At) error {
	tmpl, exists := tm.templates[tplType]
	if !exists {
		return fmt.Errorf("template %s not found", tplType)
	}

	if data.Time == "" {
		data.Time = time.Now().Format("2006-01-02 15:04:05")
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("execute template failed: %w", err)
	}

	title := data.Title
	if title == "" {
		title = string(tplType)
	}

	return tm.client.SendMarkdown(title, buf.String(), at)
}

func (tm *TemplateManager) SendAlert(title, content, level, service, environment string, details map[string]interface{}, at *At) error {
	return tm.SendWithTemplate(TplAlert, TemplateData{
		Title:       title,
		Content:     content,
		Level:       level,
		Service:     service,
		Environment: environment,
		Details:     details,
	}, at)
}

func (tm *TemplateManager) SendNotify(title, content, service string, details map[string]interface{}, at *At) error {
	return tm.SendWithTemplate(TplNotify, TemplateData{
		Title:   title,
		Content: content,
		Service: service,
		Details: details,
	}, at)
}

func (tm *TemplateManager) SendSuccess(title, content, service, environment string, details map[string]interface{}) error {
	return tm.SendWithTemplate(TplSuccess, TemplateData{
		Title:       title,
		Content:     content,
		Service:     service,
		Environment: environment,
		Details:     details,
	}, nil)
}

func (tm *TemplateManager) SendError(title, content, service, environment string, details map[string]interface{}, at *At) error {
	return tm.SendWithTemplate(TplError, TemplateData{
		Title:       title,
		Content:     content,
		Service:     service,
		Environment: environment,
		Details:     details,
	}, at)
}

func (tm *TemplateManager) SendWarning(title, content, service, environment string, details map[string]interface{}, at *At) error {
	return tm.SendWithTemplate(TplWarning, TemplateData{
		Title:       title,
		Content:     content,
		Service:     service,
		Environment: environment,
		Details:     details,
	}, at)
}

func (tm *TemplateManager) SendInfo(title, content, service string, details map[string]interface{}) error {
	return tm.SendWithTemplate(TplInfo, TemplateData{
		Title:   title,
		Content: content,
		Service: service,
		Details: details,
	}, nil)
}

func (tm *TemplateManager) SendCustomTemplate(tplContent string, data interface{}, at *At) error {
	tmpl, err := template.New("custom").Parse(tplContent)
	if err != nil {
		return fmt.Errorf("parse custom template failed: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("execute custom template failed: %w", err)
	}

	content := buf.String()
	
	if strings.HasPrefix(content, "##") {
		lines := strings.Split(content, "\n")
		title := strings.TrimSpace(strings.TrimPrefix(lines[0], "##"))
		return tm.client.SendMarkdown(title, content, at)
	}
	
	return tm.client.SendText(content, at)
}