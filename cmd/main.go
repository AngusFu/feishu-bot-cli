package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	qrterminal "github.com/mdp/qrterminal/v3"
	"rsc.io/qr"
)

const (
	accountsFeishuBaseURL = "https://accounts.feishu.cn"
	accountsLarkBaseURL   = "https://accounts.larksuite.com"
	openFeishuBaseURL     = "https://open.feishu.cn"
	openLarkBaseURL       = "https://open.larksuite.com"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "new":
		runNew(os.Args[2:])
	case "verify":
		runVerify(os.Args[2:])
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`feishu-bot-cli - 飞书/Lark 机器人一键创建工具

Usage:
  feishu-bot-cli <command> [options]

Commands:
  new      扫码新建 PersonalAgent 机器人，自动获取 app_id / app_secret
  verify   验证已有的 app_id / app_secret 是否有效

Examples:
  # 新建机器人（默认 feishu，终端显示二维码 + URL）
  feishu-bot-cli new
  feishu-bot-cli new --platform lark
  feishu-bot-cli new --timeout 300 --output-qr-image qr.png

  # 验证凭证
  feishu-bot-cli verify cli_xxxx sec_xxxx
  feishu-bot-cli verify --platform lark cli_xxxx sec_xxxx`)
}

// ── new command ──────────────────────────────────────────────────

func runNew(args []string) {
	fs := flag.NewFlagSet("new", flag.ExitOnError)
	platform := fs.String("platform", "feishu", "platform type: feishu or lark")
	timeout := fs.Int("timeout", 600, "QR onboarding timeout in seconds")
	qrImage := fs.String("output-qr-image", "", "save QR code as PNG to this path")
	outputBase64 := fs.Bool("output-qr-base64", false, "also print QR as base64 data URI")
	debug := fs.Bool("debug", false, "print debug logs")
	_ = fs.Parse(args)

	if *platform != "feishu" && *platform != "lark" {
		fmt.Fprintf(os.Stderr, "Error: --platform must be 'feishu' or 'lark'\n")
		os.Exit(1)
	}

	// Pick the right accounts base URL
	accountsBase := accountsFeishuBaseURL
	openBase := openFeishuBaseURL
	if *platform == "lark" {
		accountsBase = accountsLarkBaseURL
		openBase = openLarkBaseURL
	}

	client := &http.Client{Timeout: 15 * time.Second}

	// Step 1: init
	fmt.Printf("🔗 正在连接 %s ...\n", accountsBase)
	initRes, err := regCall(client, accountsBase, "init", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: init failed: %v\n", err)
		os.Exit(1)
	}
	if errMsg := getStr(initRes, "error"); errMsg != "" {
		fmt.Fprintf(os.Stderr, "Error: init: %s: %s\n", errMsg, getStr(initRes, "error_description"))
		os.Exit(1)
	}

	// Check client_secret support
	if supported, ok := initRes["supported_auth_methods"].([]interface{}); ok && len(supported) > 0 {
		found := false
		for _, m := range supported {
			if strings.EqualFold(fmt.Sprintf("%v", m), "client_secret") {
				found = true
				break
			}
		}
		if !found {
			fmt.Fprintf(os.Stderr, "Error: current environment does not support client_secret auth\n")
			os.Exit(1)
		}
	}
	fmt.Println("✅ 环境检查通过，支持 client_secret 认证")

	// Step 2: begin
	beginRes, err := regCall(client, accountsBase, "begin", map[string]string{
		"archetype":         "PersonalAgent",
		"auth_method":       "client_secret",
		"request_user_info": "open_id",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: begin failed: %v\n", err)
		os.Exit(1)
	}
	if errMsg := getStr(beginRes, "error"); errMsg != "" {
		fmt.Fprintf(os.Stderr, "Error: begin: %s: %s\n", errMsg, getStr(beginRes, "error_description"))
		os.Exit(1)
	}

	deviceCode := getStr(beginRes, "device_code")
	qrURL := getStr(beginRes, "verification_uri_complete")
	if deviceCode == "" || qrURL == "" {
		fmt.Fprintf(os.Stderr, "Error: incomplete onboarding response\n")
		os.Exit(1)
	}

	interval := getInt(beginRes, "interval", 5)
	expireIn := getInt(beginRes, "expire_in", *timeout)

	// Step 3: show QR
	fmt.Println()
	fmt.Println("📱 请使用飞书/Lark 手机 App 扫码完成机器人创建与授权：")
	fmt.Printf("🔗 URL: %s\n\n", qrURL)

	// Terminal QR
	qrterminal.GenerateWithConfig(qrURL, qrterminal.Config{
		Level:      qrterminal.M,
		Writer:     os.Stdout,
		HalfBlocks: false,
		BlackChar:  "██",
		WhiteChar:  "  ",
		QuietZone:  4,
	})
	fmt.Println()

	// Base64 QR
	if *outputBase64 {
		code, err := qr.Encode(qrURL, qr.M)
		if err == nil {
			code.Scale = 8
			png := code.PNG()
			dataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
			fmt.Printf("🖼️  QR Base64: %s\n\n", dataURI)
		}
	}

	// Save QR image
	if *qrImage != "" {
		code, err := qr.Encode(qrURL, qr.M)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to encode QR: %v\n", err)
		} else {
			code.Scale = 8
			if err := os.WriteFile(*qrImage, code.PNG(), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save QR image: %v\n", err)
			} else {
				fmt.Printf("🖼️  QR image saved to: %s\n\n", *qrImage)
			}
		}
	}

	// Step 4: poll
	fmt.Printf("⏳ 等待扫码授权（超时 %d 秒）...\n", expireIn)
	timeoutAt := time.Now().Add(time.Duration(expireIn) * time.Second)
	if limitByFlag := time.Now().Add(time.Duration(*timeout) * time.Second); limitByFlag.Before(timeoutAt) {
		timeoutAt = limitByFlag
	}

	currentBase := accountsBase
	platformType := *platform

	for time.Now().Before(timeoutAt) {
		pollRes, err := regCall(client, currentBase, "poll", map[string]string{"device_code": deviceCode})
		if err != nil {
			if *debug {
				fmt.Fprintf(os.Stderr, "[debug] poll error: %v\n", err)
			}
			time.Sleep(time.Duration(interval) * time.Second)
			continue
		}

		// Check tenant_brand for auto-switch
		if userInfo, ok := pollRes["user_info"].(map[string]interface{}); ok {
			if brand, _ := userInfo["tenant_brand"].(string); strings.EqualFold(brand, "lark") {
				if currentBase != accountsLarkBaseURL {
					if *debug {
						fmt.Fprintln(os.Stderr, "[debug] detected lark brand, switching base URL")
					}
					currentBase = accountsLarkBaseURL
					platformType = "lark"
					continue
				}
			}
		}

		clientID := getStr(pollRes, "client_id")
		clientSecret := getStr(pollRes, "client_secret")
		if clientID != "" && clientSecret != "" {
			ownerOpenID := ""
			if userInfo, ok := pollRes["user_info"].(map[string]interface{}); ok {
				ownerOpenID = getStr(userInfo, "open_id")
			}
			fmt.Println()
			fmt.Println("🎉 机器人创建成功！")
			fmt.Printf("   Platform:  %s\n", platformType)
			fmt.Printf("   App ID:    %s\n", clientID)
			fmt.Printf("   App Secret: %s\n", clientSecret)
			if ownerOpenID != "" {
				fmt.Printf("   Owner:     %s\n", ownerOpenID)
			}
			fmt.Println()
			fmt.Println("💡 下一步：")
			fmt.Printf("   1. 前往 %s/app 查看应用详情\n", openBase)
			fmt.Println("   2. 在开放平台检查：权限状态、事件订阅、可用范围")
			fmt.Println("   3. 发布应用版本（如有需要）")
			return
		}

		errCode := getStr(pollRes, "error")
		switch errCode {
		case "", "authorization_pending":
			// still waiting
		case "slow_down":
			interval += 5
		case "access_denied":
			fmt.Fprintf(os.Stderr, "❌ 授权被拒绝\n")
			os.Exit(1)
		case "expired_token":
			fmt.Fprintf(os.Stderr, "❌ 授权已过期，超时\n")
			os.Exit(1)
		default:
			if errCode != "" {
				fmt.Fprintf(os.Stderr, "❌ %s: %s\n", errCode, getStr(pollRes, "error_description"))
				os.Exit(1)
			}
		}

		time.Sleep(time.Duration(interval) * time.Second)
	}

	fmt.Fprintf(os.Stderr, "❌ 等待扫码超时\n")
	os.Exit(1)
}

// ── verify command ─────────────────────────────────────────────────

func runVerify(args []string) {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	platform := fs.String("platform", "", "force platform: feishu or lark (auto-detect if empty)")
	_ = fs.Parse(args)

	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "Error: verify requires app_id and app_secret arguments")
		fmt.Fprintln(os.Stderr, "Usage: feishu-bot-cli verify <app_id> <app_secret>")
		os.Exit(1)
	}

	appID := fs.Arg(0)
	appSecret := fs.Arg(1)

	candidates := []string{"feishu", "lark"}
	if *platform == "feishu" || *platform == "lark" {
		candidates = []string{*platform}
	}

	for _, candidate := range candidates {
		base := openFeishuBaseURL
		if candidate == "lark" {
			base = openLarkBaseURL
		}
		ok, err := verifyAgainstBase(base, appID, appSecret)
		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  %s: %v\n", candidate, err)
			continue
		}
		if ok {
			fmt.Printf("✅ 凭证验证成功！ (%s)\n", candidate)
			fmt.Printf("   App ID: %s\n", appID)
			return
		}
	}

	fmt.Fprintf(os.Stderr, "❌ 凭证验证失败，请检查 app_id 和 app_secret\n")
	os.Exit(1)
}

// ── helpers ───────────────────────────────────────────────────────

func regCall(client *http.Client, baseURL, action string, params map[string]string) (map[string]interface{}, error) {
	form := url.Values{}
	form.Set("action", action)
	for k, v := range params {
		form.Set(k, v)
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+"/oauth/v1/app/registration", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return result, nil
}

func verifyAgainstBase(baseURL, appID, appSecret string) (bool, error) {
	body, _ := json.Marshal(map[string]string{
		"app_id":     appID,
		"app_secret": appSecret,
	})
	req, err := http.NewRequest(http.MethodPost, baseURL+"/open-apis/auth/v3/tenant_access_token/internal", bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return false, err
	}

	var parsed struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return false, fmt.Errorf("decode: %w", err)
	}
	if parsed.Code == 0 && parsed.TenantAccessToken != "" {
		return true, nil
	}
	if parsed.Msg != "" {
		return false, fmt.Errorf("code=%d msg=%s", parsed.Code, parsed.Msg)
	}
	return false, nil
}

func getStr(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getInt(m map[string]interface{}, key string, defaultVal int) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return defaultVal
}
