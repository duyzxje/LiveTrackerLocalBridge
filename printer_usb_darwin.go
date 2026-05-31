//go:build !windows

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// PrintToUSB gửi luồng byte ESC/POS thô trực tiếp đến máy in USB thông qua driver máy in (ở macOS là hệ thống CUPS)
func PrintToUSB(printerName string, data []byte) error {
	return PrintToMacUSB(printerName, data)
}

// PrintToMacUSB gửi dữ liệu in thô xuống máy in USB trên macOS thông qua hệ thống in CUPS (lệnh `lp`)
func PrintToMacUSB(printerName string, data []byte) error {
	if printerName == "" {
		return fmt.Errorf("tên máy in USB macOS trống")
	}

	// 1. Tạo tệp tạm chứa mảng byte ESC/POS thô
	tmpFile, err := os.CreateTemp("", "receipt_*.bin")
	if err != nil {
		return fmt.Errorf("không thể tạo tệp tạm cho máy in macOS: %w", err)
	}
	// Đảm bảo xoá tệp tạm khi in xong
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if _, err := tmpFile.Write(data); err != nil {
		return fmt.Errorf("không thể ghi dữ liệu in vào tệp tạm: %w", err)
	}

	// 2. Chạy lệnh `lp` với tham số `-o raw` để bỏ qua xử lý đồ hoạ CUPS
	// Lệnh này ép hệ thống đẩy byte trực tiếp đến máy in nhiệt
	cmd := exec.Command("lp", "-d", printerName, "-o", "raw", tmpFile.Name())
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("lỗi khi gọi lệnh in lp trên macOS: %w (chi tiết lỗi: %s)", err, stderr.String())
	}

	return nil
}

// ListUSBPrinters liệt kê danh sách các máy in đã cấu hình trên macOS bằng lệnh `lpstat -e`
func ListUSBPrinters() ([]string, error) {
	cmd := exec.Command("lpstat", "-e")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("lỗi khi liệt kê máy in macOS bằng lệnh lpstat: %w", err)
	}

	lines := strings.Split(stdout.String(), "\n")
	var printers []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			printers = append(printers, line)
		}
	}
	return printers, nil
}
