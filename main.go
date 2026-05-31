package main

import (
	_ "embed"
	"os"
	"os/exec"
	"runtime"

	"github.com/getlantern/systray"
)

//go:embed icon32x32.png
var iconBytes []byte

func main() {
	// 1. Tải cấu hình đã lưu ban đầu
	cfg := LoadConfig()

	// 2. Khởi chạy HTTP Server cục bộ chạy ngầm tại cổng 13579
	go StartHttpServer("13579")

	// 3. Khởi chạy trình kiểm tra tự động cập nhật ngầm
	StartAutoUpdateChecker()

	// Đồng bộ tự khởi động cùng máy tính ban đầu
	_ = SetAutoStart(cfg.AutoStart)

	// 4. Khởi chạy System Tray Icon (giao diện khay hệ thống ngầm)
	// Hàm này sẽ block luồng chính (main thread) nên bắt buộc để cuối cùng
	systray.Run(onReady, onExit)
}

func onReady() {
	// Đặt Icon mặc định cho ứng dụng
	// Dùng mảng byte đại diện cho một biểu tượng máy in nhỏ trong suốt (PNG)
	// giúp biên dịch độc lập không cần file ảnh rời bên ngoài
	systray.SetIcon(getDefaultIconBytes())
	systray.SetTitle("LiveTracker")
	systray.SetTooltip("LiveTracker")

	// Tạo các menu item trong khay hệ thống
	mTitle := systray.AddMenuItem("LiveTracker v"+CurrentVersion, "Tên chương trình")
	mTitle.Disable()

	mStatus := systray.AddMenuItem("Trạng thái: Đang hoạt động", "Trạng thái kết nối")
	mStatus.Disable()

	systray.AddSeparator()

	mOpenWeb := systray.AddMenuItem("Mở Trang Chủ LiveTracker", "Truy cập app.livetracker.vn")
	
	mAutoStart := systray.AddMenuItemCheckbox("Khởi động cùng máy tính", "Tự khởi động cùng hệ thống", GetConfig().AutoStart)

	mCheckUpdate := systray.AddMenuItem("Kiểm tra Cập nhật ngay...", "Kiểm tra phiên bản mới")

	systray.AddSeparator()

	mQuit := systray.AddMenuItem("Thoát", "Đóng hoàn toàn ứng dụng")

	// Vòng lặp nhận sự kiện click chuột trên khay hệ thống
	go func() {
		for {
			select {
			case <-mOpenWeb.ClickedCh:
				openBrowser("https://app.livetracker.vn")
			case <-mAutoStart.ClickedCh:
				cfg := GetConfig()
				cfg.AutoStart = !cfg.AutoStart
				
				if err := SaveConfig(cfg); err == nil {
					_ = SetAutoStart(cfg.AutoStart)
					if cfg.AutoStart {
						mAutoStart.Check()
					} else {
						mAutoStart.Uncheck()
					}
				}
			case <-mCheckUpdate.ClickedCh:
				go checkAndUpdate()
			case <-mQuit.ClickedCh:
				systray.Quit()
				os.Exit(0)
			}
		}
	}()
}

func onExit() {
	// Dọn dẹp ứng dụng khi thoát
}

// Mở trình duyệt web mặc định của hệ thống
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

// Trả về ảnh máy in được đóng gói tự động thành định dạng ICO trên Windows và PNG thô trên macOS
func getDefaultIconBytes() []byte {
	if runtime.GOOS == "windows" {
		pngLen := len(iconBytes)
		icoData := make([]byte, 22+pngLen)
		
		// 1. ICO Header (6 bytes)
		icoData[0] = 0x00; icoData[1] = 0x00
		icoData[2] = 0x01; icoData[3] = 0x00 // Loại tệp: Icon
		icoData[4] = 0x01; icoData[5] = 0x00 // Số lượng ảnh: 1
		
		// Mặc định kích thước nếu không đọc được từ PNG
		var width byte = 32
		var height byte = 32

		// Đọc kích thước thực tế từ PNG IHDR chunk (nếu dữ liệu hợp lệ)
		if pngLen >= 24 {
			// PNG Width nằm ở byte 16-19, Height ở byte 20-23 dưới dạng Big Endian uint32
			w := (uint32(iconBytes[16]) << 24) | (uint32(iconBytes[17]) << 16) | (uint32(iconBytes[18]) << 8) | uint32(iconBytes[19])
			h := (uint32(iconBytes[20]) << 24) | (uint32(iconBytes[21]) << 16) | (uint32(iconBytes[22]) << 8) | uint32(iconBytes[23])
			
			if w < 256 {
				width = byte(w)
			} else {
				width = 0 // 0 đại diện cho kích thước >= 256
			}
			if h < 256 {
				height = byte(h)
			} else {
				height = 0
			}
		}

		// 2. Directory Entry (16 bytes)
		icoData[6] = width                   // Chiều rộng
		icoData[7] = height                  // Chiều cao
		icoData[8] = 0x00                    // Số màu: 0 (nhiều hơn 256 màu)
		icoData[9] = 0x00                    // Ký tự dành riêng: 0
		icoData[10] = 0x01; icoData[11] = 0x00 // Color planes: 1
		icoData[12] = 0x20; icoData[13] = 0x00 // Bits per pixel: 32
		
		// Kích thước tệp PNG (4 bytes - Little Endian)
		icoData[14] = byte(pngLen)
		icoData[15] = byte(pngLen >> 8)
		icoData[16] = byte(pngLen >> 16)
		icoData[17] = byte(pngLen >> 24)
		
		// Offset bắt đầu của dữ liệu PNG (4 bytes - Little Endian) -> 22 bytes
		icoData[18] = 22
		icoData[19] = 0x00
		icoData[20] = 0x00
		icoData[21] = 0x00
		
		// 3. Sao chép dữ liệu ảnh PNG thô vào sau tiêu đề ICO
		copy(icoData[22:], iconBytes)
		return icoData
	}
	
	// Trên macOS, trả về tệp ảnh PNG thô trực tiếp
	return iconBytes
}
