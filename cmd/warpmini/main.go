package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	fyne "fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"warpmini/assets"
	"warpmini/internal/platform"
	"warpmini/pkg/theme"
)

// Version will be set by -ldflags "-X main.Version=..." in CI
var Version = "dev"

// Firebase secure token API (same as project)
const firebaseAPIKey = "AIzaSyBdy3O3S9hrdayLJxJ7mriBR4qgUaUygAs"

type tokenRefreshResp struct {
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
	UserID       string `json:"user_id"`
	ExpiresIn    string `json:"expires_in"`
	ProjectID    string `json:"project_id"`
}

type jwtPayload struct {
	Email   string `json:"email"`
	UserID  string `json:"user_id"`
	Sub     string `json:"sub"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
	Expiry  int64  `json:"exp"`
}

type keychainPayload struct {
	IDToken struct {
		IDToken       string `json:"id_token"`
		RefreshToken  string `json:"refresh_token"`
		ExpirationISO string `json:"expiration_time"`
	} `json:"id_token"`
	RefreshToken         string      `json:"refresh_token"`
	LocalID              string      `json:"local_id"`
	Email                string      `json:"email"`
	DisplayName          interface{} `json:"display_name"`
	PhotoURL             interface{} `json:"photo_url"`
	IsOnboarded          bool        `json:"is_onboarded"`
	NeedsSSOLink         bool        `json:"needs_sso_link"`
	AnonymousUserType    interface{} `json:"anonymous_user_type"`
	LinkedAt             interface{} `json:"linked_at"`
	PersonalObjectLimits interface{} `json:"personal_object_limits"`
	IsOnWorkDomain       bool        `json:"is_on_work_domain"`
}

// 缓存最近一次登录获得的令牌，供备份/恢复直接使用
var lastIDToken string
var lastRefreshToken string
var lastUserID string
var lastEmail string

func main() {
	a := app.New()
	a.SetIcon(assets.ResourceZWarpPng)
	theme.UseCNFontIfAvailable(a)
	w := a.NewWindow("WarpMini")
	w.Resize(fyne.NewSize(640, 200))

	input := widget.NewMultiLineEntry()
	input.SetPlaceHolder("请输入 refresh_token ...")
	input.Wrapping = fyne.TextWrapOff
	input.SetMinRowsVisible(3)

	status := widget.NewLabel("")

	refreshCheck := widget.NewCheck("登录前刷新机器码", nil)
	refreshCheck.SetChecked(true)

	loginBtn := widget.NewButton("登录", func() {
		refresh := strings.TrimSpace(input.Text)
		if refresh == "" {
			status.SetText("请输入 refresh_token")
			return
		}
		status.SetText("登录中…")

		go func() {
			// 先确保Warp客户端关闭，避免占用文件或状态异常
			if runtime.GOOS == "darwin" {
				_ = platform.EnsureWarpClosedMac()
			} else if runtime.GOOS == "windows" {
				_ = platform.EnsureWarpClosedWindows()
			}

			// Optionally refresh machine ID before login
			if refreshCheck.Checked {
				if runtime.GOOS == "darwin" {
					if err := platform.RefreshMacMachineID(); err != nil {
						status.SetText("刷新机器码失败，继续登录: " + err.Error())
					}
				} else if runtime.GOOS == "windows" {
					if err := platform.RefreshWindowsMachineID(); err != nil {
						status.SetText("刷新机器码失败，继续登录: " + err.Error())
					}
				}
			}

			kcJSON, email, err := loginAndBuildKeychainJSON(refresh)
			if err != nil {
				status.SetText("登录失败: " + err.Error())
				return
			}

			// 解析写入的JSON以缓存 token 和用户信息（供备份/恢复使用）
			var kc keychainPayload
			_ = json.Unmarshal(kcJSON, &kc)
			lastIDToken = kc.IDToken.IDToken
			lastRefreshToken = kc.IDToken.RefreshToken
			lastUserID = kc.LocalID
			lastEmail = email

			if runtime.GOOS == "darwin" {
				err = platform.StoreToMacKeychain(email, kcJSON)
			} else if runtime.GOOS == "windows" {
				err = platform.StoreToWindowsUserFile(email, kcJSON)
			} else {
				err = errors.New("当前系统未支持")
			}

			if err != nil {
				status.SetText("写入失败: " + err.Error())
				return
			}

			// 已写入凭据后，启动客户端
			var startErr error
			if runtime.GOOS == "darwin" {
				startErr = platform.StartWarpClientMac()
			} else if runtime.GOOS == "windows" {
				startErr = platform.StartWarpClientWindows()
			}
			if startErr != nil {
				status.SetText("✅ 已写入凭据；启动客户端失败：" + startErr.Error())
			} else {
				status.SetText("✅ 已写入凭据，已启动客户端")
			}
		}()
	})

	cleanupBtn := widget.NewButton("清理", func() {
		status.SetText("正在清理…")
		go func() {
			var err error
			if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
				err = platform.CleanupMac()
			} else if runtime.GOOS == "windows" {
				err = platform.CleanupWindows()
			} else {
				err = errors.New("当前系统未支持")
			}
			if err != nil {
				status.SetText("清理失败: " + err.Error())
				return
			}
			status.SetText("✅ 清理完成")
		}()
	})

	// 备份按钮：Go 实现，打包后可直接使用
	backupBtn := widget.NewButton("备份", func() {
		status.SetText("正在备份…")
		go func() {
			if strings.TrimSpace(lastIDToken) == "" || strings.TrimSpace(lastRefreshToken) == "" {
				status.SetText("需要先登录后再备份")
				return
			}
			
			// 检查备份文件是否存在
			backupPath, _ := backupFilePath()
			if _, err := os.Stat(backupPath); err == nil {
				// 文件已存在，直接覆盖（简化处理，避免对话框复杂度）
				status.SetText("备份文件已存在，正在覆盖...")
				mcp, rules, err := doBackupWithGoForced(lastIDToken, lastRefreshToken, lastEmail)
				if err != nil {
					status.SetText("备份失败: " + err.Error())
					return
				}
				status.SetText(fmt.Sprintf("✅ 备份完成（已覆盖）：MCP %d, 规则 %d（~/.warp_config/config_backup.json）", mcp, rules))
			} else {
				// 文件不存在，直接备份
				mcp, rules, err := doBackupWithGo(lastIDToken, lastRefreshToken, lastEmail)
				if err != nil {
					status.SetText("备份失败: " + err.Error())
					return
				}
				status.SetText(fmt.Sprintf("✅ 备份完成：MCP %d, 规则 %d（~/.warp_config/config_backup.json）", mcp, rules))
			}
		}()
	})

	// 恢复按钮：Go 实现
	restoreBtn := widget.NewButton("恢复", func() {
		status.SetText("正在恢复备份…")
		go func() {
			if strings.TrimSpace(lastIDToken) == "" || strings.TrimSpace(lastRefreshToken) == "" {
				status.SetText("需要先登录后再恢复")
				return
			}
			res, err := doRestoreWithGo(lastIDToken, lastRefreshToken, lastUserID)
			if err != nil {
				status.SetText("恢复失败: " + err.Error())
				return
			}
			if !res.Success {
				if res.Error != "" {
					status.SetText("恢复失败: " + res.Error)
				} else {
					status.SetText("恢复失败：未知错误")
				}
				return
			}
			status.SetText(fmt.Sprintf("✅ 恢复完成：成功 %d，跳过 %d，失败 %d", res.TotalSuccess, res.TotalSkipped, res.TotalFailed))
		}()
	})

	w.SetContent(container.NewVBox(
		widget.NewLabel("refresh_token:"),
		input,
		refreshCheck,
		container.NewHBox(loginBtn, cleanupBtn, backupBtn, restoreBtn),
		status,
	))
	w.ShowAndRun()
}

// loginAndBuildKeychainJSON exchanges refresh_token for id_token and builds the exact JSON payload.
func loginAndBuildKeychainJSON(refreshToken string) ([]byte, string, error) {
	idToken, newRefresh, userID, email, exp, name, picture, err := refreshFirebaseToken(refreshToken)
	if err != nil {
		return nil, "", err
	}
	// compute expiration_time as ISO8601 with +08:00 (to match project format)
	loc := time.FixedZone("CST-8", 8*3600)
	expiration := time.Unix(exp, 0).In(loc)
	expStr := expiration.Format("2006-01-02T15:04:05.000-07:00")

	payload := keychainPayload{
		RefreshToken:         "",
		LocalID:              userID,
		Email:                email,
		DisplayName:          nullableString(name),
		PhotoURL:             nullableString(picture),
		IsOnboarded:          true,
		NeedsSSOLink:         false,
		AnonymousUserType:    nil,
		LinkedAt:             nil,
		PersonalObjectLimits: nil,
		IsOnWorkDomain:       guessWorkDomain(email),
	}
	payload.IDToken.IDToken = idToken
	payload.IDToken.RefreshToken = newRefresh
	payload.IDToken.ExpirationISO = expStr

	b, err := json.Marshal(payload)
	if err != nil {
		return nil, "", err
	}
	return b, email, nil
}

// RestoreResult 承载恢复统计
type RestoreResult struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	Error        string `json:"error"`
	TotalSuccess int    `json:"total_success"`
	TotalFailed  int    `json:"total_failed"`
	TotalSkipped int    `json:"total_skipped"`
}

// BackupData 备份文件结构（与父级结构对齐，简化）
type BackupData struct {
	BackupTime   string           `json:"backup_time"`
	BackupType   string           `json:"backup_type"`
	MCPServers   []map[string]any `json:"mcp_servers"`
	Rules        []map[string]any `json:"rules"`
	Version      string           `json:"version"`
	Format       string           `json:"format"`
	DataSource   string           `json:"data_source"`
	AccountEmail string           `json:"account_email"`
}

// doBackupWithGo 使用 GraphQL 从云端获取配置并保存到本地
func doBackupWithGo(idToken, refreshToken, email string) (int, int, error) {
	return doBackupWithOverwriteOption(idToken, refreshToken, email, false)
}

// doBackupWithGoForced 强制覆盖备份
func doBackupWithGoForced(idToken, refreshToken, email string) (int, int, error) {
	return doBackupWithOverwriteOption(idToken, refreshToken, email, true)
}

// doBackupWithOverwriteOption 使用 GraphQL 从云端获取配置并保存到本地（带覆盖选项）
func doBackupWithOverwriteOption(idToken, refreshToken, email string, forceOverwrite bool) (int, int, error) {
	client := &gqlClient{IDToken: idToken, RefreshToken: refreshToken}
	cloud, err := client.GetUpdatedCloudObjects()
	if err != nil {
		return 0, 0, err
	}
	mcpServers := []map[string]any{}
	rules := []map[string]any{}
	if arr, ok := cloud["genericStringObjects"].([]any); ok {
		for _, it := range arr {
			m, _ := it.(map[string]any)
			if m == nil {
				continue
			}
			format, _ := m["format"].(string)
			serialized := asString(m["serializedModel"]) // 可能在另一个字段上
			// 有些响应把 serializedModel 外放，我们兜底
			if serialized == "" {
				serialized = asString(m["serialized_model"]) // 容错
			}
			if format == "JsonMCPServer" && serialized != "" {
				mcpServers = append(mcpServers, map[string]any{
					"format":          "JsonMCPServer",
					"serializedModel": serialized,
				})
			}
			if format == "JsonAIFact" && serialized != "" {
				rules = append(rules, map[string]any{
					"format":          "JsonAIFact",
					"serializedModel": serialized,
				})
			}
		}
	}
	// 组装备份
	bd := BackupData{
		BackupTime:   time.Now().Format(time.RFC3339),
		BackupType:   "global",
		MCPServers:   mcpServers,
		Rules:        rules,
		Version:      "2.3",
		Format:       "simplified",
		DataSource:   "warp_api",
		AccountEmail: email,
	}
	if err := saveBackupFile(bd, forceOverwrite); err != nil {
		return 0, 0, err
	}
	return len(mcpServers), len(rules), nil
}

// doRestoreWithGo 从本地备份恢复到当前账户
func doRestoreWithGo(idToken, refreshToken, userID string) (RestoreResult, error) {
	bd, err := loadBackupFile()
	if err != nil {
		return RestoreResult{Success: false, Error: err.Error()}, nil
	}
	client := &gqlClient{IDToken: idToken, RefreshToken: refreshToken}
	res := RestoreResult{}
	
	// 获取当前账号已有的配置，用于去重
	existingConfigs := make(map[string]bool)
	if existingData, err := client.GetUpdatedCloudObjects(); err == nil {
		if arr, ok := existingData["genericStringObjects"].([]any); ok {
			for _, it := range arr {
				m, _ := it.(map[string]any)
				if m == nil {
					continue
				}
				format, _ := m["format"].(string)
				serialized := asString(m["serializedModel"])
				if serialized == "" {
					serialized = asString(m["serialized_model"])
				}
				if serialized != "" {
					// 解析配置名称
					var configData map[string]any
					if err := json.Unmarshal([]byte(serialized), &configData); err == nil {
						var name string
						if format == "JsonMCPServer" {
							name, _ = configData["name"].(string)
							existingConfigs[fmt.Sprintf("mcp:%s", name)] = true
						} else if format == "JsonAIFact" {
							if memory, ok := configData["memory"].(map[string]any); ok {
								name, _ = memory["name"].(string)
								existingConfigs[fmt.Sprintf("rule:%s", name)] = true
							}
						}
					}
				}
			}
		}
	}
	
	// MCP 恢复
	for _, m := range bd.MCPServers {
		serialized := asString(m["serializedModel"])
		if serialized == "" {
			continue
		}
		
		// 检查是否已存在同名配置
		var configData map[string]any
		if err := json.Unmarshal([]byte(serialized), &configData); err == nil {
			if name, _ := configData["name"].(string); name != "" {
				if existingConfigs[fmt.Sprintf("mcp:%s", name)] {
					res.TotalSkipped++
					continue // 跳过已存在的配置
				}
			}
		}
		
		ok, skipped, err := client.CreateGenericStringObject("JsonMCPServer", serialized, userID)
		if err != nil {
			res.TotalFailed++
			continue
		}
		if skipped {
			res.TotalSkipped++
		} else if ok {
			res.TotalSuccess++
		}
	}
	// 规则恢复
	for _, m := range bd.Rules {
		serialized := asString(m["serializedModel"])
		if serialized == "" {
			continue
		}
		
		// 检查是否已存在同名配置
		var configData map[string]any
		if err := json.Unmarshal([]byte(serialized), &configData); err == nil {
			if memory, ok := configData["memory"].(map[string]any); ok {
				if name, _ := memory["name"].(string); name != "" {
					if existingConfigs[fmt.Sprintf("rule:%s", name)] {
						res.TotalSkipped++
						continue // 跳过已存在的配置
					}
				}
			}
		}
		
		ok, skipped, err := client.CreateGenericStringObject("JsonAIFact", serialized, userID)
		if err != nil {
			res.TotalFailed++
			continue
		}
		if skipped {
			res.TotalSkipped++
		} else if ok {
			res.TotalSuccess++
		}
	}
	res.Success = res.TotalFailed == 0
	if res.Success {
		res.Message = fmt.Sprintf("恢复完成: 成功 %d，跳过 %d", res.TotalSuccess, res.TotalSkipped)
	} else {
		res.Message = fmt.Sprintf("部分成功: 成功 %d，跳过 %d，失败 %d", res.TotalSuccess, res.TotalSkipped, res.TotalFailed)
	}
	return res, nil
}

// ===== GraphQL 客户端与工具 =====

type gqlClient struct {
	IDToken      string
	RefreshToken string
}

func (c *gqlClient) do(op string, payload map[string]any) (map[string]any, int, error) {
	url := "https://app.warp.dev/graphql/v2?op=" + op
	body, _ := json.Marshal(payload)
	doOnce := func() (map[string]any, int, error) {
		req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.IDToken)
		req.Header.Set("User-Agent", randomUA())
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, 0, err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			var m map[string]any
			_ = json.Unmarshal(b, &m)
			return m, resp.StatusCode, nil
		}
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		return m, resp.StatusCode, fmt.Errorf("http %d", resp.StatusCode)
	}
	res, code, err := doOnce()
	if code == 401 && c.RefreshToken != "" {
		// 尝试刷新一次
		id, refresh, _, _, _, _, _, rErr := refreshFirebaseToken(c.RefreshToken)
		if rErr == nil && id != "" {
			c.IDToken = id
			if refresh != "" {
				c.RefreshToken = refresh
			}
			// 更新全局缓存，便于后续请求
			lastIDToken = c.IDToken
			lastRefreshToken = c.RefreshToken
			return doOnce()
		}
	}
	return res, code, err
}

func (c *gqlClient) GetUpdatedCloudObjects() (map[string]any, error) {
	query := `
query GetUpdatedCloudObjects($input: UpdatedCloudObjectsInput!, $requestContext: RequestContext!) {
  updatedCloudObjects(input: $input, requestContext: $requestContext) {
    __typename
    ... on UpdatedCloudObjectsOutput {
      genericStringObjects {
        format
        serializedModel
        metadata { uid metadataLastUpdatedTs }
      }
      workflows {
        data
        metadata { uid metadataLastUpdatedTs }
      }
      responseContext { serverVersion }
    }
    ... on UserFacingError {
      error { __typename message }
      responseContext { serverVersion }
    }
  }
}
`
	variables := map[string]any{
		"input": map[string]any{
			"folders":              []any{},
			"forceRefresh":         true,
			"genericStringObjects": []any{},
			"notebooks":            []any{},
			"workflows":            []any{},
		},
		"requestContext": map[string]any{"osContext": map[string]any{}, "clientContext": map[string]any{}},
	}
	payload := map[string]any{"operationName": "GetUpdatedCloudObjects", "variables": variables, "query": query}
	res, _, err := c.do("GetUpdatedCloudObjects", payload)
	if err != nil {
		return nil, err
	}
	data, ok := res["data"].(map[string]any)
	if !ok {
		return nil, errors.New("响应格式异常: 缺少data")
	}
	uco, ok := data["updatedCloudObjects"].(map[string]any)
	if !ok {
		return nil, errors.New("响应格式异常: 缺少updatedCloudObjects")
	}
	typ, _ := uco["__typename"].(string)
	if typ == "UserFacingError" {
		errMap, _ := uco["error"].(map[string]any)
		msg := asString(errMap["message"])
		if msg == "" { msg = "API错误" }
		return nil, errors.New(msg)
	}
	if typ != "UpdatedCloudObjectsOutput" {
		return nil, errors.New("响应类型异常")
	}
	return uco, nil
}

func (c *gqlClient) CreateGenericStringObject(format, serializedModel, userUID string) (ok bool, skipped bool, err error) {
	mutation := `
mutation CreateGenericStringObject($input: CreateGenericStringObjectInput!, $requestContext: RequestContext!) {
  createGenericStringObject(input: $input, requestContext: $requestContext) {
    __typename
    ... on CreateGenericStringObjectOutput {
      genericStringObject { metadata { uid } format }
    }
    ... on UserFacingError { error { __typename message } }
  }
}
`
	variables := map[string]any{
		"input": map[string]any{
			"genericStringObject": map[string]any{
				"clientId":        fmt.Sprintf("Client-%d", time.Now().UnixNano()),
				"entrypoint":      "Unknown",
				"format":          format,
				"initialFolderId": nil,
				"serializedModel": serializedModel,
				"uniquenessKey":  nil,
			},
			"owner": map[string]any{"uid": userUID, "type": "User"},
		},
		"requestContext": map[string]any{
			"clientContext": map[string]any{"version": "v0.2025.09.03.08.11.stable_02"},
			"osContext":     map[string]any{"category": runtime.GOOS, "name": runtime.GOOS, "version": ""},
		},
	}
	payload := map[string]any{"operationName": "CreateGenericStringObject", "variables": variables, "query": mutation}
	res, _, e := c.do("CreateGenericStringObject", payload)
	if e != nil {
		return false, false, e
	}
	data, _ := res["data"].(map[string]any)
	if data == nil { return false, false, errors.New("响应缺少data") }
	cgo, _ := data["createGenericStringObject"].(map[string]any)
	if cgo == nil { return false, false, errors.New("响应缺少createGenericStringObject") }
	typ, _ := cgo["__typename"].(string)
	if typ == "CreateGenericStringObjectOutput" {
		return true, false, nil
	}
	if typ == "UserFacingError" {
		errMap, _ := cgo["error"].(map[string]any)
		msg := asString(errMap["message"])
		if strings.Contains(strings.ToLower(msg), "unique") || strings.Contains(msg, "UniqueKeyConflict") {
			return false, true, nil // 视为跳过（已存在）
		}
		return false, false, errors.New(msg)
	}
	return false, false, errors.New("未知响应类型")
}

// 随机 UA，遵循项目风格（轻量实现）
func randomUA() string {
	return "Mozilla/5.0 (Macintosh; Intel Mac OS X 13_0_1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.6777.120 Safari/537.36"
}

// 保存/读取备份文件
func backupFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil { return "", err }
	dir := filepath.Join(home, ".warp_config")
	if err := os.MkdirAll(dir, 0o755); err != nil { return "", err }
	return filepath.Join(dir, "config_backup.json"), nil
}

func saveBackupFile(b BackupData, forceOverwrite bool) error {
	path, err := backupFilePath()
	if err != nil { return err }
	
	// 检查文件是否已存在
	if _, err := os.Stat(path); err == nil && !forceOverwrite {
		// 文件存在，需要用户确认是否覆盖
		return fmt.Errorf("备份文件已存在: %s，请手动删除或选择覆盖", path)
	}
	
	f, err := os.Create(path)
	if err != nil { return err }
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(b)
}

func loadBackupFile() (BackupData, error) {
	var b BackupData
	path, err := backupFilePath()
	if err != nil { return b, err }
	data, err := os.ReadFile(path)
	if err != nil { return b, err }
	if err := json.Unmarshal(data, &b); err != nil { return b, err }
	return b, nil
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func refreshFirebaseToken(refreshToken string) (idToken, newRefresh, userID, email string, exp int64, name, picture string, err error) {
	url := fmt.Sprintf("https://securetoken.googleapis.com/v1/token?key=%s", firebaseAPIKey)
	body := "grant_type=refresh_token&refresh_token=" + urlEncode(refreshToken)
	req, _ := http.NewRequest("POST", url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{Timeout: 12 * time.Second}
	resp, e := client.Do(req)
	if e != nil {
		err = e
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		x, _ := io.ReadAll(resp.Body)
		err = fmt.Errorf("token refresh failed: %s", string(x))
		return
	}
	var tr tokenRefreshResp
	if e := json.NewDecoder(resp.Body).Decode(&tr); e != nil {
		err = e
		return
	}
	idToken = tr.IDToken
	newRefresh = tr.RefreshToken
	userID = tr.UserID
	// parse JWT to get email, exp, name, picture
	pay, e2 := parseJWTPayload(tr.IDToken)
	if e2 == nil {
		if pay.Email != "" {
			email = pay.Email
		}
		if pay.UserID != "" {
			userID = pay.UserID
		} else if pay.Sub != "" {
			userID = pay.Sub
		}
		exp = pay.Expiry
		name = pay.Name
		picture = pay.Picture
	}
	if email == "" {
		email = ""
	}
	if exp == 0 {
		// default to +1h
		exp = time.Now().Add(time.Hour).Unix()
	}
	return
}

func parseJWTPayload(idToken string) (jwtPayload, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return jwtPayload{}, errors.New("invalid jwt")
	}
	p := parts[1]
	// add padding for base64url
	switch len(p) % 4 {
	case 2:
		p += "=="
	case 3:
		p += "="
	}
	data, err := base64.URLEncoding.DecodeString(p)
	if err != nil {
		return jwtPayload{}, err
	}
	var pl jwtPayload
	if err := json.Unmarshal(data, &pl); err != nil {
		return jwtPayload{}, err
	}
	return pl, nil
}

func urlEncode(s string) string {
	return url.QueryEscape(s)
}

func nullableString(s string) interface{} {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func guessWorkDomain(email string) bool {
	idx := strings.LastIndex(email, "@")
	if idx <= 0 || idx+1 >= len(email) {
		return true
	}
	domain := strings.ToLower(email[idx+1:])
	// from project defaults
	workDomains := map[string]bool{
		"959585.xyz": true,
	}
	return workDomains[domain]
}
