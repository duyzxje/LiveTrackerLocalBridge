package main

import (
	"fmt"
	"net"
	"time"
)

// PrintToLan gửi mảng byte ESC/POS thô trực tiếp tới máy in qua cổng TCP/IP
func PrintToLan(ip string, port string, data []byte) error {
	if ip == "" {
		return fmt.Errorf("địa chỉ IP máy in LAN trống")
	}
	if port == "" {
		port = "9100" // Cổng mặc định cho in thô RAW socket
	}

	address := net.JoinHostPort(ip, port)

	// Kết nối với Timeout 3 giây để tránh treo ứng dụng nếu máy in offline
	conn, err := net.DialTimeout("tcp", address, 3*time.Second)
	if err != nil {
		return fmt.Errorf("không thể kết nối tới máy in mạng %s: %w", address, err)
	}
	defer conn.Close()

	// Đặt Deadline gửi dữ liệu để đề phòng lỗi đường truyền bị nghẽn
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))

	// Truyền luồng byte ESC/POS thô
	n, err := conn.Write(data)
	if err != nil {
		return fmt.Errorf("gặp lỗi khi truyền lệnh in xuống máy in LAN: %w", err)
	}
	if n < len(data) {
		return fmt.Errorf("chỉ gửi được %d/%d bytes lệnh in", n, len(data))
	}

	return nil
}
