//go:build darwin
// +build darwin

package platform

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"

	"github.com/google/uuid"
	"warpmini/internal/cleanup"
)

const macKeychainService = "dev.warp.Warp-Stable"

var macKeychainServices = []string{
	"dev.warp.Warp-Stable",
	"dev.warp.Warp",
	"dev.warp.Warp-Canary",
}

// StoreToMacKeychain cleans old entries and writes JSON under both accounts: email and "User".
func StoreToMacKeychain(email string, jsonData []byte) error {
	if runtime.GOOS != "darwin" {
		return errors.New("当前系统未支持")
	}
	if email == "" {
		return errors.New("无法确定邮箱")
	}
	if err := macCleanupKeychainAll(); err != nil {
		// continue but report
		fmt.Println("Keychain cleanup warning:", err)
	}
	if err := macAddGenericPassword(email, jsonData); err != nil {
		return err
	}
	if err := macAddGenericPassword("User", jsonData); err != nil {
		return err
	}
	return nil
}

func macAddGenericPassword(account string, jsonData []byte) error {
	if runtime.GOOS != "darwin" {
		return errors.New("当前系统未支持")
	}
	cmd := exec.Command("security", "add-generic-password",
		"-a", account,
		"-s", macKeychainService,
		"-w", string(jsonData),
		"-U", // update
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("security add-generic-password failed: %v: %s", err, stderr.String())
	}
	return nil
}

// macCleanupKeychainAll deletes all generic-password items for known Warp services.
func macCleanupKeychainAll() error {
	if runtime.GOOS != "darwin" {
		return nil
	}
	for _, svc := range macKeychainServices {
		// loop until find returns non-zero
		for i := 0; i < 100; i++ {
			check := exec.Command("security", "find-generic-password", "-s", svc)
			if err := check.Run(); err != nil {
				break
			}
			cmd := exec.Command("security", "delete-generic-password", "-s", svc)
			_ = cmd.Run() // ignore error and loop again
		}
	}
	return nil
}

// RefreshMacMachineID performs the ban cleanup subset: kill Warp and remove keychain entries and common folders.
func RefreshMacMachineID() error {
	if runtime.GOOS != "darwin" {
		return nil
	}
	// generate uuid and write defaults keys
	newID := uuid.New().String()
	// write ExperimentId
	cmd := exec.Command("defaults", "write", "dev.warp.Warp-Networking.WarpNetworking", "ExperimentId", "-string", newID)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("写入ExperimentId失败: %w", err)
	}
	// reset login flag
	cmd2 := exec.Command("defaults", "write", "dev.warp.Warp-Networking.WarpNetworking", "DidNonAnonymousUserLogIn", "-bool", "false")
	_ = cmd2.Run() // 非关键
	return nil
}

func EnsureWarpClosedMac() error {
	switch runtime.GOOS {
	case "darwin":
		_ = exec.Command("osascript", "-e", "tell application \"Warp\" to quit").Run()
		_ = exec.Command("/bin/sleep", "1").Run()
		for _, proc := range []string{"Warp", "Cloudflare WARP"} {
			_ = exec.Command("killall", "-9", proc).Run()
		}
	case "linux":
		for _, name := range []string{"warp", "warp-terminal", "warp-cli", "cloudflare-warp"} {
			_ = exec.Command("pkill", "-f", name).Run()
		}
	}
	return nil
}

// StartWarpClientMac launches the Warp app on macOS.
func StartWarpClientMac() error {
	if runtime.GOOS != "darwin" {
		return nil
	}
	cmd := exec.Command("open", "-a", "Warp")
	return cmd.Run()
}

func CleanupMac() error {
	_ = EnsureWarpClosedMac()
	errs := &cleanup.Errors{}

	if runtime.GOOS == "darwin" {
		errs.Add("keychain(dev.warp.*)", macCleanupKeychainAll())
	}

	home, err := os.UserHomeDir()
	if err != nil {
		errs.Merge(fmt.Errorf("获取用户目录失败: %w", err))
		return errs.Err()
	}

	// Known domain prefixes (cover all variants)
	domains := []string{
		"dev.warp.Warp-Stable",
		"dev.warp.Warp",
		"dev.warp.Warp-Canary",
		"dev.warp.Warp-Networking.WarpNetworking",
	}

	// Fixed paths for each domain
	var dataPaths []string
	for _, d := range domains {
		dataPaths = append(dataPaths,
			filepath.Join(home, "Library", "Application Support", d),
			filepath.Join(home, "Library", "Caches", d),
			filepath.Join(home, "Library", "Preferences", d+".plist"),
			filepath.Join(home, "Library", "Saved Application State", d+".savedState"),
			filepath.Join(home, "Library", "WebKit", d),
			filepath.Join(home, "Library", "HTTPStorages", d),
			filepath.Join(home, "Library", "Cookies", d+".binarycookies"),
		)
	}
	cleanup.RemovePaths(dataPaths, errs)

	// Glob paths for each domain (ByHost, etc.)
	for _, d := range domains {
		cleanup.RemoveGlob(filepath.Join(home, "Library", "Preferences", "ByHost", d+".*"), errs)
	}

	// Recent documents entries
	cleanup.RemoveGlob(filepath.Join(home, "Library", "Application Support", "com.apple.sharedfilelist", "com.apple.LSSharedFileList.ApplicationRecentDocuments", "dev.warp.*.sfl2"), errs)

	// Logs (user level)
	logPatterns := []string{
		filepath.Join(home, "Library", "Logs", "*warp*"),
		filepath.Join(home, "Library", "Logs", "*Warp*"),
		filepath.Join(home, "Library", "Logs", "*dev.warp*"),
	}
	for _, pattern := range logPatterns {
		cleanup.RemoveGlob(pattern, errs)
	}

	// Diagnostic reports (system and user)
	username := os.Getenv("USER")
	if username == "" {
		if u, uErr := user.Current(); uErr == nil {
			username = u.Username
		}
	}
	diagDirs := []string{
		filepath.Join("/Library", "Logs", "DiagnosticReports"),
		filepath.Join(home, "Library", "Logs", "DiagnosticReports"),
	}
	diagPatterns := []string{"*Warp*.crash", "*Warp*.hang", "*warp*.crash", "*warp*.hang", "*warp*.diag", "*Warp*.diag"}
	if username != "" {
		userSpecific := []string{
			fmt.Sprintf("*stable*%s*.hang", username),
			fmt.Sprintf("*stable*%s*.crash", username),
			fmt.Sprintf("*stable*%s*.diag", username),
		}
		diagPatterns = append(diagPatterns, userSpecific...)
	}
	for _, dir := range diagDirs {
		for _, pattern := range diagPatterns {
			cleanup.RemoveGlob(filepath.Join(dir, pattern), errs)
		}
	}

	// CrashReporter receipts
	crashReporterPatterns := []string{
		filepath.Join(home, "Library", "Application Support", "CrashReporter", "*stable_*.plist"),
		filepath.Join(home, "Library", "Application Support", "CrashReporter", "*warp*.plist"),
		filepath.Join(home, "Library", "Application Support", "CrashReporter", "*Warp*.plist"),
		filepath.Join(home, "Library", "Application Support", "CrashReporter", "*dev.warp*.plist"),
	}
	for _, pattern := range crashReporterPatterns {
		cleanup.RemoveGlob(pattern, errs)
	}

	// Login items / LaunchAgents
	cleanup.RemoveGlob(filepath.Join(home, "Library", "LaunchAgents", "*warp*.plist"), errs)

	// App Containers (if sandboxed in some variants)
	cleanup.RemoveGlob(filepath.Join(home, "Library", "Containers", "dev.warp.*"), errs)
	cleanup.RemoveGlob(filepath.Join(home, "Library", "Group Containers", "dev.warp.*"), errs)
	cleanup.RemoveGlob(filepath.Join(home, "Library", "Application Scripts", "dev.warp.*"), errs)

	// Home dot directories and XDG-style paths (clear all)
	// 注意：排除 .warp_config 备份目录
	for _, pat := range []string{
		filepath.Join(home, ".warp"),       // 只删除 .warp 文件夹本身
		filepath.Join(home, ".warp-*"),     // 删除 .warp-xxx 但不会匹配 .warp_config
		filepath.Join(home, ".warp_cache"), // 只删除特定的缓存目录
		filepath.Join(home, ".warp_temp"),  // 只删除特定的临时目录
		filepath.Join(home, ".config", "warp"),
		filepath.Join(home, ".local", "share", "warp"),
		filepath.Join(home, ".cache", "warp"),
	} {
		cleanup.RemoveGlob(pat, errs)
	}

	// System temp caches
	tempPatterns := []string{
		"/private/var/folders/*/*/C/dev.warp.Warp-Stable",
		"/private/var/folders/*/*/T/dev.warp.Warp-Stable",
	}
	for _, pattern := range tempPatterns {
		cleanup.RemoveGlob(pattern, errs)
	}

	return errs.Err()
}
