package main

import (
	"bytes"
	"image"
	"image/color"
	"math"
)

// ConvertImageToEscPos chuyển đổi một đối tượng image.Image thành tập lệnh ESC/POS để in đồ hoạ
func ConvertImageToEscPos(img image.Image, targetPaperWidth int) ([]byte, error) {
	// 1. Xác định chiều rộng mục tiêu (80mm -> 576px, 58mm -> 384px)
	targetWidth := 576
	if targetPaperWidth == 58 {
		targetWidth = 384
	}

	// 2. Thay đổi kích thước ảnh về chiều rộng mục tiêu (giữ nguyên tỷ lệ chiều cao)
	resizedImg := resizeImage(img, targetWidth)
	bounds := resizedImg.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// 3. Thực hiện thuật toán Dithering Floyd-Steinberg để chuyển ảnh sang đen trắng sắc nét
	binarized := applyFloydSteinbergDithering(resizedImg)

	// 4. Đóng gói pixel đen trắng sang byte (1 byte = 8 pixels ngang, MSB là pixel bên trái)
	// Chiều rộng tính theo byte phải là bội số của 8 (đã đảm bảo vì 576/8 = 72 bytes, 384/8 = 48 bytes)
	widthBytes := width / 8
	var buf bytes.Buffer

	// Tập lệnh ESC/POS: Khởi tạo máy in (ESC @)
	buf.Write([]byte{0x1B, 0x40})

	// Căn lề giữa cho ảnh in (ESC a 1)
	buf.Write([]byte{0x1B, 0x61, 0x01})

	// Lệnh in đồ hoạ thô (GS v 0 m xL xH yL yH)
	// m = 0 (Normal mode)
	xL := byte(widthBytes % 256)
	xH := byte(widthBytes / 256)
	yL := byte(height % 256)
	yH := byte(height / 256)

	buf.Write([]byte{0x1D, 0x76, 0x30, 0x00, xL, xH, yL, yH})

	// Ghi các byte dữ liệu điểm ảnh
	for y := 0; y < height; y++ {
		for x := 0; x < widthBytes; x++ {
			var b byte = 0
			for bit := 0; bit < 8; bit++ {
				px := x*8 + bit
				// Nếu là điểm đen, set bit tương ứng thành 1 (ở máy in nhiệt, 1 là đen, 0 là trắng)
				if binarized[y][px] {
					b |= 1 << (7 - bit)
				}
			}
			buf.WriteByte(b)
		}
	}

	// Lệnh đẩy giấy lên và cắt (GS V 66 0): Đẩy lên và cắt một phần (nhưng giữ lại chút để không rơi bill)
	buf.Write([]byte{0x1D, 0x56, 0x42, 0x00})

	return buf.Bytes(), nil
}

// Thuật toán resize ảnh Nearest-Neighbor đơn giản nhưng nhanh gọn, không cần thư viện ngoài
func resizeImage(img image.Image, newWidth int) image.Image {
	bounds := img.Bounds()
	oldWidth := bounds.Dx()
	oldHeight := bounds.Dy()

	if oldWidth == newWidth {
		return img
	}

	newHeight := int(math.Round(float64(oldHeight) * (float64(newWidth) / float64(oldWidth))))
	newImg := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))

	for y := 0; y < newHeight; y++ {
		for x := 0; x < newWidth; x++ {
			srcX := int(float64(x) * float64(oldWidth) / float64(newWidth))
			srcY := int(float64(y) * float64(oldHeight) / float64(newHeight))
			newImg.Set(x, y, img.At(srcX, srcY))
		}
	}

	return newImg
}

// Thuật toán Dithering Floyd-Steinberg chuyển ảnh thành ma trận hai chiều boolean (true = đen, false = trắng)
// Giúp in ảnh xám mượt mà và giữ độ nét chữ cực cao trên máy in nhiệt
func applyFloydSteinbergDithering(img image.Image) [][]bool {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	// Khởi tạo ma trận lưu giá trị độ sáng (grayscale) dạng float64 để tính sai số lan truyền
	gray := make([][]float64, h)
	for y := 0; y < h; y++ {
		gray[y] = make([]float64, w)
		for x := 0; x < w; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			// Chuyển sang Grayscale sử dụng hệ số chuẩn NTSC: Y = 0.299R + 0.587G + 0.114B
			// Chia cho 257 để chuyển giá trị từ [0, 65535] về [0, 255]
			grayVal := (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 257.0
			gray[y][x] = grayVal
		}
	}

	// Ma trận kết quả nhị phân đen (true)/trắng (false)
	result := make([][]bool, h)
	for y := 0; y < h; y++ {
		result[y] = make([]bool, w)
	}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			oldPixel := gray[y][x]
			// Ngưỡng xám: Máy in nhiệt 0 là trắng (255), 1 là đen (0)
			// Nên nếu giá trị xám < 128 thì in đen (true)
			var newPixel float64
			if oldPixel < 128.0 {
				newPixel = 0.0 // Đen
				result[y][x] = true
			} else {
				newPixel = 255.0 // Trắng
				result[y][x] = false
			}

			// Tính sai số (error) giữa giá trị xám gốc và nhị phân
			quantError := oldPixel - newPixel

			// Lan truyền sai số sang các pixel lân cận theo ma trận Floyd-Steinberg
			// [        *   7/16 ]
			// [ 3/16  5/16  1/16 ]
			if x+1 < w {
				gray[y][x+1] += quantError * 7.0 / 16.0
			}
			if y+1 < h {
				if x-1 >= 0 {
					gray[y+1][x-1] += quantError * 3.0 / 16.0
				}
				gray[y+1][x] += quantError * 5.0 / 16.0
				if x+1 < w {
					gray[y+1][x+1] += quantError * 1.0 / 16.0
				}
			}
		}
	}

	return result
}

// BinarizeThreshold chuyển đổi ảnh dùng Threshold đơn giản không lan truyền lỗi (phòng hờ nếu cần chạy siêu tốc)
func BinarizeThreshold(img image.Image) [][]bool {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	result := make([][]bool, h)

	for y := 0; y < h; y++ {
		result[y] = make([]bool, w)
		for x := 0; x < w; x++ {
			c := color.GrayModel.Convert(img.At(x, y)).(color.Gray)
			result[y][x] = c.Y < 128
		}
	}
	return result
}
