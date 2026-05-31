package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg" // Đăng ký bộ giải mã JPEG
	_ "image/png"  // Đăng ký bộ giải mã PNG
	"log"
	"net/http"
	"runtime"
	"strings"
)

// Khai báo kiểu phản hồi chuẩn JSON
type StandardResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

// Chạy HTTP Server tại cổng chỉ định
func StartHttpServer(port string) {
	mux := http.NewServeMux()

	mux.HandleFunc("/status", handleStatus)
	mux.HandleFunc("/config", handleConfig)
	mux.HandleFunc("/print", handlePrint)

	// Thêm CORS middleware cho toàn bộ Server
	corsHandler := corsMiddleware(mux)

	log.Printf("Local Bridge Server đang khởi động tại http://localhost:%s", port)
	err := http.ListenAndServe(":"+port, corsHandler)
	if err != nil {
		log.Fatalf("Không thể khởi động HTTP Server: %v", err)
	}
}

// CORS Middleware giúp trình duyệt của website https://app.livetracker.vn kết nối localhost không bị chặn
// Xử lý đúng chuẩn Private Network Access (PNA) để Chrome hiển thị popup xin quyền truy cập
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}

		// Thiết lập CORS headers cơ bản cho mọi response
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, PATCH")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Access-Control-Request-Private-Network")
		w.Header().Set("Access-Control-Max-Age", "86400")

		// Private Network Access (PNA): Chrome gửi header này trong preflight
		// khi trang HTTPS cố gắng kết nối tới localhost/private IP
		// Server PHẢI phản hồi header này để Chrome hiển thị popup xin quyền
		if r.Header.Get("Access-Control-Request-Private-Network") == "true" {
			w.Header().Set("Access-Control-Allow-Private-Network", "true")
			log.Printf("[PNA] Preflight từ Origin: %s — Đã cho phép Private Network Access", origin)
		}

		// Vary headers giúp trình duyệt cache đúng CORS response theo origin
		w.Header().Add("Vary", "Origin")
		w.Header().Add("Vary", "Access-Control-Request-Method")
		w.Header().Add("Vary", "Access-Control-Request-Headers")
		w.Header().Add("Vary", "Access-Control-Request-Private-Network")

		// Trả về 200 OK ngay lập tức đối với pre-flight OPTIONS request
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}


func handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, StandardResponse{Success: false, Message: "Phương thức không hợp lệ"})
		return
	}

	cfg := GetConfig()
	
	// Liệt kê máy in USB đang cắm trong máy để gửi về giao diện web
	usbPrinters, err := ListUSBPrinters()
	if err != nil {
		usbPrinters = []string{}
	}

	statusData := map[string]any{
		"version":      "1.0.0",
		"os":           runtime.GOOS,
		"arch":         runtime.GOARCH,
		"config":       cfg,
		"usb_printers": usbPrinters,
	}

	writeJSON(w, http.StatusOK, StandardResponse{
		Success: true,
		Data:    statusData,
	})
}

// POST /config: Lưu thông số máy in và cập nhật khởi động cùng máy
func handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, StandardResponse{Success: false, Message: "Phương thức không hợp lệ"})
		return
	}

	var newCfg Config
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&newCfg); err != nil {
		writeJSON(w, http.StatusBadRequest, StandardResponse{Success: false, Message: "Dữ liệu JSON không hợp lệ"})
		return
	}

	// Lưu cấu hình xuống file
	if err := SaveConfig(newCfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, StandardResponse{Success: false, Message: "Lỗi ghi cấu hình: " + err.Error()})
		return
	}

	// Đồng bộ cấu hình Auto-Startup cùng hệ điều hành
	if err := SetAutoStart(newCfg.AutoStart); err != nil {
		log.Printf("Không thể thiết lập Auto-Startup: %v", err)
	}

	writeJSON(w, http.StatusOK, StandardResponse{
		Success: true,
		Message: "Đã lưu cấu hình máy in thành công",
	})
}

// Request cấu trúc in dạng JSON (hỗ trợ in qua chuỗi ảnh base64)
type PrintRequest struct {
	ImageBase64 string `json:"image_base64"` // e.g. "data:image/png;base64,iVBORw..."
}

// POST /print: Nhận ảnh in (hỗ trợ cả JSON base64 và multipart/form-data)
func handlePrint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, StandardResponse{Success: false, Message: "Phương thức không hợp lệ"})
		return
	}

	var img image.Image
	var err error

	// 1. Phân loại và phân giải ảnh in từ client gửi lên
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") {
		// A. Nhận dữ liệu dạng JSON (ảnh base64)
		var req PrintRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, StandardResponse{Success: false, Message: "Lỗi giải mã JSON"})
			return
		}

		rawBase64 := req.ImageBase64
		// Xử lý loại bỏ tiền tố data URI nếu có (ví dụ: "data:image/jpeg;base64,")
		if idx := strings.Index(rawBase64, ","); idx != -1 {
			rawBase64 = rawBase64[idx+1:]
		}

		imgReader := base64.NewDecoder(base64.StdEncoding, strings.NewReader(rawBase64))
		img, _, err = image.Decode(imgReader)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, StandardResponse{Success: false, Message: "Lỗi giải mã ảnh base64: " + err.Error()})
			return
		}
	} else if strings.Contains(contentType, "multipart/form-data") {
		// B. Nhận dữ liệu dạng Multipart Form
		err = r.ParseMultipartForm(10 << 20) // Giới hạn 10MB ảnh
		if err != nil {
			writeJSON(w, http.StatusBadRequest, StandardResponse{Success: false, Message: "Lỗi đọc form-data"})
			return
		}

		file, _, errUpload := r.FormFile("image")
		if errUpload != nil {
			writeJSON(w, http.StatusBadRequest, StandardResponse{Success: false, Message: "Không tìm thấy trường 'image' trong form"})
			return
		}
		defer file.Close()

		img, _, err = image.Decode(file)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, StandardResponse{Success: false, Message: "Lỗi giải mã ảnh tệp tải lên: " + err.Error()})
			return
		}
	} else {
		// C. Đọc trực tiếp Raw Body Binary nếu đẩy ảnh thô trực tiếp
		img, _, err = image.Decode(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, StandardResponse{Success: false, Message: "Lỗi giải mã ảnh raw binary: " + err.Error()})
			return
		}
	}

	// 2. Chuyển đổi ảnh vừa decode sang tập lệnh máy in ESC/POS
	cfg := GetConfig()
	escPosBytes, err := ConvertImageToEscPos(img, cfg.PaperWidth)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, StandardResponse{Success: false, Message: "Lỗi chuyển đổi ảnh sang ESC/POS: " + err.Error()})
		return
	}

	// 3. Thực hiện in tuỳ theo cấu hình kết nối đang hoạt động
	switch cfg.PrinterType {
	case "lan":
		err = PrintToLan(cfg.LanIP, cfg.LanPort, escPosBytes)
	case "usb":
		err = PrintToUSB(cfg.PrinterName, escPosBytes)
	default:
		err = fmt.Errorf("kiểu kết nối máy in không được hỗ trợ: %s", cfg.PrinterType)
	}

	if err != nil {
		writeJSON(w, http.StatusInternalServerError, StandardResponse{Success: false, Message: "Lỗi thiết bị in: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, StandardResponse{
		Success: true,
		Message: "Đã gửi lệnh in xuống máy in nhiệt thành công",
	})
}

// Viết phản hồi JSON chuẩn
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}


