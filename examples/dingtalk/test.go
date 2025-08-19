package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/dingtalk"
)

func main() {
	// 命令行参数
	var (
		accessToken = flag.String("token", "", "DingTalk robot access token (required)")
		secret      = flag.String("secret", "", "DingTalk robot secret (optional)")
		message     = flag.String("msg", "测试消息：球球雷达系统正常运行中", "Message to send")
		msgType     = flag.String("type", "text", "Message type: text, markdown")
	)
	
	flag.Parse()
	
	// 检查必需参数
	if *accessToken == "" {
		// 尝试从环境变量获取
		*accessToken = os.Getenv("DINGTALK_TOKEN")
		if *accessToken == "" {
			fmt.Println("错误：需要提供 access token")
			fmt.Println("使用方法：")
			fmt.Println("  go run test.go -token YOUR_TOKEN")
			fmt.Println("  或设置环境变量 DINGTALK_TOKEN")
			os.Exit(1)
		}
	}
	
	if *secret == "" {
		*secret = os.Getenv("DINGTALK_SECRET")
	}
	
	// 创建客户端
	option := dingtalk.ClientOption{
		AccessToken: *accessToken,
		Secret:      *secret,
		Timeout:     10 * time.Second,
		Keywords:    []string{"球球雷达"}, // 使用默认关键词
	}
	
	client := dingtalk.NewClient(option)
	
	fmt.Printf("准备发送消息...\n")
	fmt.Printf("- 消息类型: %s\n", *msgType)
	fmt.Printf("- 消息内容: %s\n", *message)
	if *secret != "" {
		fmt.Printf("- 使用签名验证: 是\n")
	}
	fmt.Println()
	
	var err error
	
	switch *msgType {
	case "text":
		err = client.SendText(*message, nil)
		
	case "markdown":
		markdown := fmt.Sprintf(`## 🚀 测试消息
		
**时间**: %s
**来源**: Go Backend Kit DingTalk SDK
**状态**: ✅ 正常

### 消息内容
%s

---
_[球球雷达] 自动化监控系统_`, 
			time.Now().Format("2006-01-02 15:04:05"),
			*message,
		)
		err = client.SendMarkdown("测试消息", markdown, nil)
		
	default:
		log.Fatalf("不支持的消息类型: %s", *msgType)
	}
	
	if err != nil {
		log.Fatalf("❌ 发送失败: %v", err)
	}
	
	fmt.Println("✅ 消息发送成功！")
}