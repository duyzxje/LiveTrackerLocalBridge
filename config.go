package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Config định nghĩa các tham số cấu hình của Local Bridge
type Config struct {
	PrinterType string `json:"printer_type"` // "usb" hoặc "lan"
	PrinterName string `json:"printer_name"` // Tên máy in USB (ví dụ: "Xprinter XP-N160I")
	LanIP       string `json:"lan_ip"`       // IP máy in LAN (ví dụ: "192.168.1.100")
	LanPort     string `json:"lan_port"`     // Cổng in LAN (thông thường là "9100")
	PaperWidth  int    `json:"paper_width"`  // Khổ giấy: 80 hoặc 58 (mm)
	AutoStart   bool   `json:"auto_start"`   // Tự khởi động cùng hệ thống
}

var (
	configPath string
	activeConfig Config
	configLock   sync.RWMutex
)

// Khởi tạo thư mục và đường dẫn file config mặc định
func initConfigPath() {
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		userConfigDir = os.TempDir()
	}
	appDir := filepath.Join(userConfigDir, "LiveTrackerBridge")
	_ = os.MkdirAll(appDir, 0755)
	configPath = filepath.Join(appDir, "config.json")
}

// LoadConfig đọc cấu hình từ file JSON
func LoadConfig() Config {
	configLock.RLock()
	defer configLock.RUnlock()

	if configPath == "" {
		initConfigPath()
	}

	// Cấu hình mặc định
	defaultConfig := Config{
		PrinterType: "lan",
		PrinterName: "",
		LanIP:       "192.168.1.100",
		LanPort:     "9100",
		PaperWidth:  80,
		AutoStart:   true,
	}

	file, err := os.Open(configPath)
	if err != nil {
		activeConfig = defaultConfig
		// Lưu cấu hình mặc định xuống file nếu chưa có
		go SaveConfig(defaultConfig)
		return defaultConfig
	}
	defer file.Close()

	var cfg Config
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		activeConfig = defaultConfig
		return defaultConfig
	}

	activeConfig = cfg
	return cfg
}

// SaveConfig lưu cấu hình hiện tại xuống file JSON
func SaveConfig(cfg Config) error {
	configLock.Lock()
	defer configLock.Unlock()

	if configPath == "" {
		initConfigPath()
	}

	file, err := os.Create(configPath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(cfg); err != nil {
		return err
	}

	activeConfig = cfg
	return nil
}

// GetConfig trả về bản sao cấu hình đang hoạt động an toàn đa luồng
func GetConfig() Config {
	configLock.RLock()
	defer configLock.RUnlock()
	return activeConfig
}
