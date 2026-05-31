//go:build windows

package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"
)

var (
	winspool = syscall.NewLazyDLL("winspool.drv")

	procOpenPrinter      = winspool.NewProc("OpenPrinterW")
	procClosePrinter     = winspool.NewProc("ClosePrinter")
	procStartDocPrinter  = winspool.NewProc("StartDocPrinterW")
	procEndDocPrinter    = winspool.NewProc("EndDocPrinter")
	procStartPagePrinter = procStartDocPrinter // Đôi khi gộp chung hoặc không cần, nhưng đúng chuẩn cần StartPagePrinter
	procEndPagePrinter   = winspool.NewProc("EndPagePrinter")
	procWritePrinter     = winspool.NewProc("WritePrinter")
)

type DOC_INFO_1 struct {
	pDocName    *uint16
	pOutputFile *uint16
	pDatatype   *uint16
}

// PrintToUSB gửi luồng byte ESC/POS thô trực tiếp đến máy in USB thông qua driver máy in (ở Windows là Winspool API)
func PrintToUSB(printerName string, data []byte) error {
	return PrintToWindowsUSB(printerName, data)
}

// PrintToWindowsUSB gửi luồng byte ESC/POS thô trực tiếp đến máy in USB thông qua Windows Print Spooler
func PrintToWindowsUSB(printerName string, data []byte) error {
	if printerName == "" {
		return fmt.Errorf("tên máy in USB Windows trống")
	}

	// 1. Chuyển đổi tên máy in sang UTF-16 pointer cho Windows API
	pPrinterName, err := syscall.UTF16PtrFromString(printerName)
	if err != nil {
		return fmt.Errorf("không thể chuyển đổi tên máy in sang UTF16: %w", err)
	}

	var hPrinter syscall.Handle
	// Gọi OpenPrinterW
	r1, _, errOpen := procOpenPrinter.Call(
		uintptr(unsafe.Pointer(pPrinterName)),
		uintptr(unsafe.Pointer(&hPrinter)),
		0,
	)
	if r1 == 0 {
		return fmt.Errorf("không thể mở máy in '%s' (hãy chắc chắn tên máy in chính xác): %w", printerName, errOpen)
	}
	defer procClosePrinter.Call(uintptr(hPrinter))

	// 2. Thiết lập thông tin tài liệu in DOC_INFO_1
	docNameStr := "LiveTracker Receipt"
	dataTypeStr := "RAW" // BẮT BUỘC dùng RAW để in thô ESC/POS trực tiếp qua driver mà không bị Windows parse lại

	pDocName, _ := syscall.UTF16PtrFromString(docNameStr)
	pDataType, _ := syscall.UTF16PtrFromString(dataTypeStr)

	docInfo := DOC_INFO_1{
		pDocName:    pDocName,
		pOutputFile: nil,
		pDatatype:   pDataType,
	}

	// Gọi StartDocPrinterW
	r1, _, errStartDoc := procStartDocPrinter.Call(
		uintptr(hPrinter),
		1,
		uintptr(unsafe.Pointer(&docInfo)),
	)
	if r1 == 0 {
		return fmt.Errorf("lỗi khởi tạo tài liệu in (StartDocPrinter): %w", errStartDoc)
	}
	defer procEndDocPrinter.Call(uintptr(hPrinter))

	// Khởi tạo trang in (StartPagePrinter)
	winspool.NewProc("StartPagePrinter").Call(uintptr(hPrinter))
	defer procEndPagePrinter.Call(uintptr(hPrinter))

	// 3. Ghi dữ liệu thô xuống máy in (WritePrinter)
	var bytesWritten uint32
	r1, _, errWrite := procWritePrinter.Call(
		uintptr(hPrinter),
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(len(data)),
		uintptr(unsafe.Pointer(&bytesWritten)),
	)
	if r1 == 0 {
		return fmt.Errorf("lỗi truyền lệnh in xuống máy in qua Windows Spooler: %w", errWrite)
	}
	if bytesWritten < uint32(len(data)) {
		return fmt.Errorf("chỉ truyền được %d/%d bytes lệnh in", bytesWritten, len(data))
	}

	return nil
}

// ListUSBPrinters liệt kê danh sách tên các máy in đã cài đặt trong Windows thông qua PowerShell
// Cách tiếp cận gọi exec PowerShell cực kỳ an toàn, không cần tệp DLL C phức tạp
func ListUSBPrinters() ([]string, error) {
	cmd := exec.Command("powershell", "-Command", "Get-Printer | Select-Object -ExpandProperty Name")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		// Dự phòng nếu không chạy được PowerShell, dùng wmic (cũ hơn nhưng hoạt động)
		cmdWmic := exec.Command("wmic", "printer", "get", "name")
		var stdoutWmic bytes.Buffer
		cmdWmic.Stdout = &stdoutWmic
		if errWmic := cmdWmic.Run(); errWmic != nil {
			return nil, fmt.Errorf("không thể liệt kê máy in bằng cả PowerShell và WMIC: %v / %v", err, errWmic)
		}
		
		lines := strings.Split(stdoutWmic.String(), "\n")
		var printers []string
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && !strings.EqualFold(line, "name") {
				printers = append(printers, line)
			}
		}
		return printers, nil
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
