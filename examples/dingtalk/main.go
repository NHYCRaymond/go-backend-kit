package main

import (
	"fmt"
	"log"
	"time"

	"github.com/yourusername/go-backend-kit/dingtalk"
)

func main() {
	option := dingtalk.ClientOption{
		AccessToken: "YOUR_ACCESS_TOKEN",
		Secret:      "YOUR_SECRET", 
		Timeout:     10 * time.Second,
		Keywords:    []string{"球球雷达"}, // 设置关键词，如果不设置会使用默认的"球球雷达"
	}

	client := dingtalk.NewClient(option)

	exampleBasicMessages(client)
	
	exampleTemplateMessages(client)
	
	exampleCustomTemplate(client)
}

func exampleBasicMessages(client *dingtalk.Client) {
	fmt.Println("=== 基础消息类型示例 ===")
	
	err := client.SendText("这是一条文本消息", nil)
	if err != nil {
		log.Printf("发送文本消息失败: %v", err)
	} else {
		fmt.Println("✓ 文本消息发送成功")
	}

	at := &dingtalk.At{
		AtMobiles: []string{"13800138000"},
		IsAtAll:   false,
	}
	err = client.SendText("这是一条@某人的消息", at)
	if err != nil {
		log.Printf("发送@消息失败: %v", err)
	} else {
		fmt.Println("✓ @消息发送成功")
	}

	err = client.SendLink(
		"时代的火车向前开",
		"这个即将发布的新版本，创始人xx称它为红树林。",
		"https://www.dingtalk.com/",
		"",
	)
	if err != nil {
		log.Printf("发送链接消息失败: %v", err)
	} else {
		fmt.Println("✓ 链接消息发送成功")
	}

	markdown := `
#### 杭州天气 @13800138000
> 9度，西北风1级，空气良89，相对温度73%

> ![screenshot](https://img.alicdn.com/tfs/TB1NwmBEL9TBuNjy1zbXXXpepXa-2400-1218.png)
> ###### 10点20分发布 [天气](https://www.dingtalk.com)`

	err = client.SendMarkdown("杭州天气", markdown, nil)
	if err != nil {
		log.Printf("发送Markdown消息失败: %v", err)
	} else {
		fmt.Println("✓ Markdown消息发送成功")
	}

	actionCard := &dingtalk.ActionCard{
		Title: "乔布斯 20 年前想打造一间苹果咖啡厅",
		Text: `![screenshot](https://gw.alicdn.com/tfs/TB1ut3xxbsrBKNjSZFpXXcXhFXa-846-786.png) 
### 乔布斯 20 年前想打造的苹果咖啡厅
Apple Store 的设计正从原来满满的科技感走向生活化，而其生活化的走向其实可以追溯到 20 年前苹果一个建立咖啡馆的计划`,
		SingleTitle: "阅读全文",
		SingleURL:   "https://www.dingtalk.com/",
	}
	err = client.SendActionCard(actionCard)
	if err != nil {
		log.Printf("发送ActionCard消息失败: %v", err)
	} else {
		fmt.Println("✓ ActionCard消息发送成功")
	}

	multiActionCard := &dingtalk.ActionCard{
		Title: "我 20 年前想打造一间苹果咖啡厅，而它正是 Apple Store 的前身",
		Text: `![screenshot](https://img.alicdn.com/tfs/TB1NwmBEL9TBuNjy1zbXXXpepXa-2400-1218.png) 
### 乔布斯 20 年前想打造的苹果咖啡厅`,
		BtnOrientation: "0",
		Btns: []dingtalk.ActionBtn{
			{
				Title:     "内容不错",
				ActionURL: "https://www.dingtalk.com/",
			},
			{
				Title:     "不感兴趣",
				ActionURL: "https://www.dingtalk.com/",
			},
		},
	}
	err = client.SendActionCard(multiActionCard)
	if err != nil {
		log.Printf("发送多按钮ActionCard消息失败: %v", err)
	} else {
		fmt.Println("✓ 多按钮ActionCard消息发送成功")
	}

	feedLinks := []dingtalk.FeedLink{
		{
			Title:      "时代的火车向前开1",
			MessageURL: "https://www.dingtalk.com/",
			PicURL:     "https://img.alicdn.com/tfs/TB1NwmBEL9TBuNjy1zbXXXpepXa-2400-1218.png",
		},
		{
			Title:      "时代的火车向前开2",
			MessageURL: "https://www.dingtalk.com/",
			PicURL:     "https://img.alicdn.com/tfs/TB1NwmBEL9TBuNjy1zbXXXpepXa-2400-1218.png",
		},
	}
	err = client.SendFeedCard(feedLinks)
	if err != nil {
		log.Printf("发送FeedCard消息失败: %v", err)
	} else {
		fmt.Println("✓ FeedCard消息发送成功")
	}
}

func exampleTemplateMessages(client *dingtalk.Client) {
	fmt.Println("\n=== 模板消息示例 ===")
	
	tm := dingtalk.NewTemplateManager(client)

	err := tm.SendAlert(
		"服务异常告警",
		"API响应时间超过阈值，请及时处理",
		"高",
		"user-service",
		"production",
		map[string]interface{}{
			"接口":    "/api/v1/users",
			"响应时间": "3.5s",
			"阈值":    "1s",
			"影响用户": "1000+",
		},
		&dingtalk.At{IsAtAll: true},
	)
	if err != nil {
		log.Printf("发送告警消息失败: %v", err)
	} else {
		fmt.Println("✓ 告警消息发送成功")
	}

	err = tm.SendSuccess(
		"部署成功",
		"版本 v1.2.3 已成功部署到生产环境",
		"payment-service",
		"production",
		map[string]interface{}{
			"版本号":   "v1.2.3",
			"部署时间": "2024-01-20 15:30:00",
			"部署人":   "张三",
			"变更内容": "修复支付bug",
		},
	)
	if err != nil {
		log.Printf("发送成功消息失败: %v", err)
	} else {
		fmt.Println("✓ 成功消息发送成功")
	}

	err = tm.SendError(
		"系统错误",
		"数据库连接失败，服务不可用",
		"database-service",
		"production",
		map[string]interface{}{
			"错误代码":   "DB_CONN_FAILED",
			"错误信息":   "Connection timeout",
			"数据库地址": "10.0.0.1:3306",
			"重试次数":   3,
		},
		&dingtalk.At{
			AtMobiles: []string{"13800138000"},
		},
	)
	if err != nil {
		log.Printf("发送错误消息失败: %v", err)
	} else {
		fmt.Println("✓ 错误消息发送成功")
	}

	err = tm.SendWarning(
		"性能警告",
		"CPU使用率接近阈值",
		"compute-service",
		"production",
		map[string]interface{}{
			"当前CPU": "85%",
			"阈值":     "90%",
			"节点":     "node-01",
			"持续时间":  "5分钟",
		},
		nil,
	)
	if err != nil {
		log.Printf("发送警告消息失败: %v", err)
	} else {
		fmt.Println("✓ 警告消息发送成功")
	}

	err = tm.SendInfo(
		"系统通知",
		"定时任务执行完成",
		"scheduler-service",
		map[string]interface{}{
			"任务名称": "数据备份",
			"执行时间": "02:00:00",
			"耗时":    "15分钟",
			"结果":    "成功",
		},
	)
	if err != nil {
		log.Printf("发送信息消息失败: %v", err)
	} else {
		fmt.Println("✓ 信息消息发送成功")
	}

	err = tm.SendNotify(
		"新用户注册",
		"有新用户完成注册",
		"user-service",
		map[string]interface{}{
			"用户ID":   "U10001",
			"用户名":    "testuser",
			"注册时间":  "2024-01-20 16:00:00",
			"注册来源":  "Web",
		},
		nil,
	)
	if err != nil {
		log.Printf("发送通知消息失败: %v", err)
	} else {
		fmt.Println("✓ 通知消息发送成功")
	}
}

func exampleCustomTemplate(client *dingtalk.Client) {
	fmt.Println("\n=== 自定义模板示例 ===")
	
	tm := dingtalk.NewTemplateManager(client)

	customMarkdownTemplate := `## 📊 {{.Title}}
**报告日期**: {{.Date}}
**部门**: {{.Department}}

### 📈 销售数据
- **总销售额**: ¥{{.TotalSales}}
- **订单数量**: {{.OrderCount}}
- **新客户数**: {{.NewCustomers}}

### 🏆 TOP3 产品
{{range $i, $product := .TopProducts}}
{{add $i 1}}. {{$product.Name}} - ¥{{$product.Sales}}
{{end}}

### 📝 备注
{{.Notes}}

---
_报告生成时间: {{.GeneratedAt}}_`

	err := tm.RegisterTemplate(dingtalk.TplCustom, customMarkdownTemplate)
	if err != nil {
		log.Printf("注册自定义模板失败: %v", err)
		return
	}

	data := dingtalk.TemplateData{
		Title: "日销售报告",
		Extra: map[string]interface{}{
			"Date":         "2024-01-20",
			"Department":   "销售一部",
			"TotalSales":   "128500.00",
			"OrderCount":   156,
			"NewCustomers": 23,
			"TopProducts": []map[string]interface{}{
				{"Name": "产品A", "Sales": "45000.00"},
				{"Name": "产品B", "Sales": "38000.00"},
				{"Name": "产品C", "Sales": "25500.00"},
			},
			"Notes":       "今日促销活动效果良好",
			"GeneratedAt": time.Now().Format("2006-01-02 15:04:05"),
		},
	}

	err = tm.SendWithTemplate(dingtalk.TplCustom, data, nil)
	if err != nil {
		log.Printf("发送自定义模板消息失败: %v", err)
	} else {
		fmt.Println("✓ 自定义模板消息发送成功")
	}

	simpleTemplate := `🔔 任务提醒
任务: {{.TaskName}}
截止时间: {{.Deadline}}
负责人: {{.Assignee}}
优先级: {{.Priority}}`

	err = tm.SendCustomTemplate(simpleTemplate, map[string]interface{}{
		"TaskName": "完成Q1季度报告",
		"Deadline": "2024-01-25 18:00",
		"Assignee": "李四",
		"Priority": "高",
	}, nil)
	if err != nil {
		log.Printf("发送简单自定义模板失败: %v", err)
	} else {
		fmt.Println("✓ 简单自定义模板发送成功")
	}
}