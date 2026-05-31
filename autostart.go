package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// SetAutoStart cấu hình hoặc huỷ bỏ tự khởi động cùng hệ điều hành
func SetAutoStart(enable bool) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("không thể lấy đường dẫn tệp thực thi: %w", err)
	}

	switch runtime.GOOS {
	case "windows":
		return setWindowsAutoStart(exePath, enable)
	case "darwin":
		return setMacAutoStart(exePath, enable)
	default:
		return fmt.Errorf("hệ điều hành %s không được hỗ trợ tự khởi động", runtime.GOOS)
	}
}

// Cấu hình trên Windows thông qua gọi lệnh `reg` (tránh import registry độc quyền Windows)
func setWindowsAutoStart(exePath string, enable bool) error {
	keyName := "LiveTrackerBridge"
	
	if enable {
		// Thêm registry key: HKCU\Software\Microsoft\Windows\CurrentVersion\Run
		// Thêm dấu ngoặc kép bọc quanh exePath phòng trường hợp đường dẫn có khoảng trắng
		cmd := exec.Command("reg", "add", "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Run", "/v", keyName, "/t", "REG_SZ", "/d", fmt.Sprintf("\"%s\"", exePath), "/f")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("lỗi khi ghi vào Registry Windows: %w", err)
		}
	} else {
		// Xoá registry key
		cmd := exec.Command("reg", "delete", "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Run", "/v", keyName, "/f")
		// Bỏ qua lỗi nếu key không tồn tại
		_ = cmd.Run()
	}
	return nil
}

// Cấu hình trên macOS thông qua tạo tệp LaunchAgent plist
func setMacAutoStart(exePath string, enable bool) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("không tìm thấy thư mục người dùng macOS: %w", err)
	}

	plistDir := filepath.Join(homeDir, "Library", "LaunchAgents")
	_ = os.MkdirAll(plistDir, 0755)
	
	plistPath := filepath.Join(plistDir, "vn.livetracker.bridge.plist")

	if !enable {
		// Xoá plist file nếu có
		_ = os.Remove(plistPath)
		return nil
	}

	// Nội dung XML của LaunchAgent plist
	plistContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>vn.livetracker.bridge</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>`, exePath)

	err = os.WriteFile(plistPath, []byte(plistContent), 0644)
	if err != nil {
		return fmt.Errorf("không thể ghi file plist vào LaunchAgents: %w", err)
	}

	return nil
}
