//go:build windows
// +build windows

package platform

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/google/uuid"
	"golang.org/x/sys/windows/registry"
	"warpmini/internal/cleanup"
)

// Windows: user file path detection mirroring project logic
func getWindowsDataDir() string {
	localAppData := os.Getenv("LOCALAPPDATA")
	appData := os.Getenv("APPDATA")
	userProfile := os.Getenv("USERPROFILE")
	candidates := []string{}
	if localAppData != "" {
		candidates = append(candidates, filepath.Join(localAppData, "warp", "Warp", "data"))
		candidates = append(candidates, filepath.Join(localAppData, "Warp", "data"))
	}
	if appData != "" {
		candidates = append(candidates, filepath.Join(appData, "warp", "data"))
	}
	if userProfile != "" {
		candidates = append(candidates, filepath.Join(userProfile, "AppData", "Local", "warp", "Warp", "data"))
	}
	// fallback
	if len(candidates) == 0 {
		candidates = append(candidates, filepath.Join("C:\\", "ProgramData", "warp", "data"))
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	// default to first
	return candidates[0]
}

// StoreToWindowsUserFile writes DPAPI-encrypted JSON to dev.warp.Warp-User
func StoreToWindowsUserFile(email string, jsonData []byte) error {
	dataDir := getWindowsDataDir()
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dataDir, "dev.warp.Warp-User")
	enc, err := dpapiEncrypt(jsonData)
	if err != nil {
		// fallback to plaintext
		enc = jsonData
	}
	if err := os.WriteFile(path, enc, 0o600); err != nil {
		return err
	}
	return nil
}

// RefreshWindowsMachineID kills Warp processes and removes key files.
func RefreshWindowsMachineID() error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Warp.dev\Warp`, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return fmt.Errorf("打开注册表失败: %w", err)
	}
	defer k.Close()
	newID := uuid.New().String()
	if err := k.SetStringValue("ExperimentId", newID); err != nil {
		return fmt.Errorf("写入ExperimentId失败: %w", err)
	}
	return nil
}

func EnsureWarpClosedWindows() error {
	procs := []string{"Warp.exe", "WarpTerminal.exe", "WarpTerminalService.exe", "Cloudflare WARP.exe", "warp-svc.exe", "warp-cli.exe", "warp-taskbar.exe"}
	for _, p := range procs {
		_ = exec.Command("taskkill", "/IM", p, "/T", "/F").Run()
	}
	return nil
}

// StartWarpClientWindows tries several common locations to launch Warp.
func StartWarpClientWindows() error {
	candidates := []string{}
	localAppData := os.Getenv("LOCALAPPDATA")
	programFiles := os.Getenv("ProgramFiles")
	programFilesX86 := os.Getenv("ProgramFiles(x86)")
	if localAppData != "" {
		candidates = append(candidates,
			filepath.Join(localAppData, "Programs", "Warp", "Warp.exe"),
			filepath.Join(localAppData, "Programs", "Warp Terminal", "Warp.exe"),
			filepath.Join(localAppData, "Warp", "Warp.exe"),
		)
	}
	if programFiles != "" {
		candidates = append(candidates,
			filepath.Join(programFiles, "Warp", "Warp.exe"),
			filepath.Join(programFiles, "Warp Terminal", "Warp.exe"),
		)
	}
	if programFilesX86 != "" {
		candidates = append(candidates,
			filepath.Join(programFilesX86, "Warp", "Warp.exe"),
			filepath.Join(programFilesX86, "Warp Terminal", "Warp.exe"),
		)
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return exec.Command(p).Start()
		}
	}
	// Fallback: try using start command to resolve from PATH or App Execution Alias
	return exec.Command("cmd", "/C", "start", "", "Warp").Start()
}

func CleanupWindows() error {
	_ = EnsureWarpClosedWindows()
	errs := &cleanup.Errors{}

	dataDir := getWindowsDataDir()
	cleanup.RemovePaths([]string{
		filepath.Join(dataDir, "dev.warp.Warp-User"),
		filepath.Join(dataDir, "warp.sqlite"),
	}, errs)

	localAppData := os.Getenv("LOCALAPPDATA")
	appData := os.Getenv("APPDATA")
	programData := os.Getenv("ProgramData")
	programFiles := os.Getenv("ProgramFiles")
	programFilesX86 := os.Getenv("ProgramFiles(x86)")
	tempDir := os.Getenv("TEMP")

	var extraPaths []string
	if localAppData != "" {
		extraPaths = append(extraPaths,
			filepath.Join(localAppData, "warp"),
			filepath.Join(localAppData, "Warp"),
			filepath.Join(localAppData, "Warp", "data"),
			filepath.Join(localAppData, "Warp", "logs"),
			filepath.Join(localAppData, "warp", "Warp"),
			filepath.Join(localAppData, "Programs", "Warp"),
			filepath.Join(localAppData, "Programs", "Warp Terminal"),
			filepath.Join(localAppData, "warp-terminal"),
		)
	}
	if appData != "" {
		extraPaths = append(extraPaths,
			filepath.Join(appData, "warp"),
			filepath.Join(appData, "Warp"),
			filepath.Join(appData, "warp-terminal"),
		)
	}
	if programData != "" {
		extraPaths = append(extraPaths,
			filepath.Join(programData, "warp"),
			filepath.Join(programData, "Warp"),
			filepath.Join(programData, "Microsoft", "Windows", "Start Menu", "Programs", "Warp"),
		)
	} else {
		extraPaths = append(extraPaths, filepath.Join("C:\\ProgramData", "Microsoft", "Windows", "Start Menu", "Programs", "Warp"))
	}
	if programFiles != "" {
		extraPaths = append(extraPaths,
			filepath.Join(programFiles, "Warp"),
			filepath.Join(programFiles, "Warp Terminal"),
		)
	}
	if programFilesX86 != "" {
		extraPaths = append(extraPaths,
			filepath.Join(programFilesX86, "Warp"),
			filepath.Join(programFilesX86, "Warp Terminal"),
		)
	}
	if tempDir != "" {
		extraPaths = append(extraPaths, filepath.Join(tempDir, "Warp"))
	}
	cleanup.RemovePaths(extraPaths, errs)

	if localAppData != "" {
		cleanup.RemoveGlob(filepath.Join(localAppData, "Warp", "logs", "*"), errs)
	}
	if tempDir != "" {
		cleanup.RemoveGlob(filepath.Join(tempDir, "Warp", "*"), errs)
	}
	cleanup.RemoveGlob(filepath.Join("C:\\Windows", "Prefetch", "WARP*.pf"), errs)
	cleanup.RemoveGlob(filepath.Join("C:\\Windows", "Prefetch", "WARPSETUP*.pf"), errs)

	startMenuPaths := []string{}
	if appData != "" {
		startMenuPaths = append(startMenuPaths, filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "Warp"))
	}
	startMenuPaths = append(startMenuPaths, filepath.Join("C:\\ProgramData", "Microsoft", "Windows", "Start Menu", "Programs", "Warp"))
	cleanup.RemovePaths(startMenuPaths, errs)

	regKeys := []string{
		`HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\warp-terminal-stable_is1`,
		`HKEY_LOCAL_MACHINE\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\warp-terminal-stable_is1`,
		`HKEY_CURRENT_USER\Software\Warp`,
		`HKEY_CURRENT_USER\Software\Warp.dev`,
		`HKEY_LOCAL_MACHINE\SOFTWARE\Warp`,
	}
	for _, key := range regKeys {
		cmd := exec.Command("reg", "delete", key, "/f")
		out, err := cmd.CombinedOutput()
		if err != nil {
			msg := strings.TrimSpace(string(out))
			lower := strings.ToLower(msg)
			if strings.Contains(lower, "cannot find") || strings.Contains(lower, "unable to find") || strings.Contains(msg, "找不到") {
				continue
			}
			errs.Merge(fmt.Errorf("删除注册表项 %s 失败: %v (%s)", key, err, msg))
		}
	}

	return errs.Err()
}

// DPAPI wrappers
const cryptprotectUIForbidden = 0x1

type dataBlob struct {
	cbData uint32
	pbData *byte
}

func dpapiEncrypt(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty data")
	}
	crypt32 := syscall.NewLazyDLL("crypt32.dll")
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	procCryptProtectData := crypt32.NewProc("CryptProtectData")
	procLocalFree := kernel32.NewProc("LocalFree")

	var inBlob dataBlob
	inBlob.cbData = uint32(len(data))
	inBlob.pbData = &data[0]
	var outBlob dataBlob

	r, _, err := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(&inBlob)),
		0,
		0,
		0,
		0,
		uintptr(cryptprotectUIForbidden),
		uintptr(unsafe.Pointer(&outBlob)),
	)
	if r == 0 {
		if err != nil {
			return nil, fmt.Errorf("CryptProtectData failed: %v", err)
		}
		return nil, fmt.Errorf("CryptProtectData failed")
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(outBlob.pbData)))

	enc := make([]byte, outBlob.cbData)
	// copy from C buffer
	copy(enc, (*[1 << 30]byte)(unsafe.Pointer(outBlob.pbData))[:outBlob.cbData:outBlob.cbData])
	return enc, nil
}
