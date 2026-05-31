package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"time"

	"github.com/minio/selfupdate"
)

// CurrentVersion xác định phiên bản hiện tại của chương trình Local Bridge
const CurrentVersion = "1.0.0"

// UpdateURL là URL trỏ tới tệp JSON kiểm tra phiên bản trên máy chủ của bạn
const UpdateURL = "https://raw.githubusercontent.com/duyzxje/LiveTrackerLocalBridge/main/version.json"

type VersionInfo struct {
	Version     string `json:"version"`
	WindowsURL  string `json:"windows_url"`
	MacosURL    string `json:"macos_url"`
	WindowsHash string `json:"windows_hash"`
	MacosHash   string `json:"macos_hash"`
}

// StartAutoUpdateChecker bắt đầu tiến trình chạy ngầm định kỳ kiểm tra cập nhật
func StartAutoUpdateChecker() {
	// 1. Kiểm tra ngay lập tức khi khởi động
	go checkAndUpdate()

	// 2. Chạy định kỳ mỗi 4 tiếng một lần
	ticker := time.NewTicker(4 * time.Hour)
	go func() {
		for range ticker.C {
			checkAndUpdate()
		}
	}()
}

// Kiểm tra phiên bản hiện tại so với máy chủ và tự tải về cài đặt nếu có bản mới
func checkAndUpdate() {
	log.Println("Đang kiểm tra phiên bản mới từ máy chủ...")

	// Thiết lập Client có Timeout để tránh treo khi mạng lỗi
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(UpdateURL)
	if err != nil {
		log.Printf("[AutoUpdate] Không thể kết nối tới máy chủ cập nhật: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[AutoUpdate] Máy chủ cập nhật trả về mã lỗi HTTP: %d", resp.StatusCode)
		return
	}

	var info VersionInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		log.Printf("[AutoUpdate] Lỗi giải mã dữ liệu JSON phiên bản: %v", err)
		return
	}

	// So sánh chuỗi phiên bản (ví dụ "1.0.1" > "1.0.0")
	if info.Version > CurrentVersion {
		log.Printf("[AutoUpdate] Phát hiện phiên bản mới: %s (Hiện tại: %s). Tiến hành tự động tải...", info.Version, CurrentVersion)

		var downloadURL string
		if runtime.GOOS == "windows" {
			downloadURL = info.WindowsURL
		} else if runtime.GOOS == "darwin" {
			downloadURL = info.MacosURL
		}

		if downloadURL == "" {
			log.Println("[AutoUpdate] Không tìm thấy link tải tương thích với hệ điều hành hiện tại.")
			return
		}

		err = doSelfUpdate(downloadURL)
		if err != nil {
			log.Printf("[AutoUpdate] Cập nhật thất bại: %v", err)
		} else {
			log.Println("[AutoUpdate] 🚀 ĐÃ TỰ ĐỘNG CẬP NHẬT THÀNH CÔNG lên phiên bản " + info.Version + "! Bản cập nhật sẽ có hiệu lực trong lần khởi động tiếp theo.")
		}
	} else {
		log.Println("[AutoUpdate] Bạn đang sử dụng phiên bản mới nhất: " + CurrentVersion)
	}
}

// doSelfUpdate tải tệp nhị phân mới và tự thay thế file chạy hiện tại một cách an toàn
func doSelfUpdate(url string) error {
	client := &http.Client{
		Timeout: 2 * time.Minute, // Cho phép 2 phút để tải file
	}

	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("không thể tải tệp nhị phân cập nhật: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("lỗi kết nối tải file nhị phân, HTTP status: %d", resp.StatusCode)
	}

	// Sử dụng minio/selfupdate để thực hiện thay thế in-place tệp tin thực thi đang chạy ngầm
	// Cơ chế này cực kỳ an toàn, không gây crash ứng dụng và tự động dọn dẹp file cũ
	err = selfupdate.Apply(resp.Body, selfupdate.Options{})
	if err != nil {
		return fmt.Errorf("gặp lỗi khi thay thế tệp thực thi đang hoạt động: %w", err)
	}

	return nil
}
