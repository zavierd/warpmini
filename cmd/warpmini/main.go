package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

func main() {
	a := app.New()
	a.SetIcon(assets.ResourceZWarpPng)
	theme.UseCNFontIfAvailable(a)
	w := a.NewWindow("WarpMini")
	w.Resize(fyne.NewSize(520, 180))

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

	w.SetContent(container.NewVBox(
		widget.NewLabel("refresh_token:"),
		input,
		refreshCheck,
		container.NewHBox(loginBtn, cleanupBtn),
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
