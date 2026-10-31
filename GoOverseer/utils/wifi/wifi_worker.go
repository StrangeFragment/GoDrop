package main

import (
	"encoding/json"
	"io"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/text/encoding/charmap"
)

// Расширенная структура данных Wi-Fi телеметрии
type WifiData struct {
	Time      string `json:"time"`
	Interface string `json:"interface"`
	SSID      string `json:"ssid"`
	BSSID     string `json:"bssid"`
	Signal    int32  `json:"signal"`
	Channel   int32  `json:"channel"`
	Standard  string `json:"standard"`
	RxRate    int32  `json:"rx_rate"`
	TxRate    int32  `json:"tx_rate"`
	State     string `json:"state"`
}

// Структура конфигурации демона
type Config struct {
	IntervalSec      string `json:"interval_sec"`
	MinSignalDropDbm string `json:"min_signal_drop_dbm"`
}

func main() {
	// 1. Вычисляем путь к рабочей директории демона в %APPDATA%\GoOverseer\WIFI
	exePath, err := os.Executable()
	if err != nil {
		os.Exit(1)
	}
	workDir := filepath.Dir(exePath)

	// Настраиваем системный логгер демона для отладки
	logFile, _ := os.OpenFile(filepath.Join(workDir, "wifi_daemon.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if logFile != nil {
		log.SetOutput(logFile)
		defer logFile.Close()
	}

	configPath := filepath.Join(workDir, "config.json")
	recordsPath := filepath.Join(workDir, "wifi_records.jsonl")

	var lastSignal int32 = -999
	var lastSSID string = ""
	var lastState string = ""
	var lastRecordTime time.Time

	for {
		// 2. Читаем актуальные настройки из config.json
		interval := 5
		minDropDbm := 0

		cfgData, err := os.ReadFile(configPath)
		if err == nil {
			var cfg Config
			if json.Unmarshal(cfgData, &cfg) == nil {
				if val, err := strconv.Atoi(cfg.IntervalSec); err == nil && val > 0 {
					interval = val
				}
				if val, err := strconv.Atoi(cfg.MinSignalDropDbm); err == nil && val > 0 {
					minDropDbm = val
				}
			}
		}

		// 3. Считываем данные Wi-Fi адаптера через netsh
		currentData := parseWindowsWifi()

		// 4. Проверяем условие логирования:
		// Если порог minDropDbm задан, пишем при изменении сигнала >= порога,
		// при смене сети/состояния или принудительно раз в 30 секунд (heartbeat для живого графика)
		shouldWrite := true
		if minDropDbm > 0 && lastSignal != -999 {
			diff := int(math.Abs(float64(currentData.Signal - lastSignal)))
			stateChanged := currentData.SSID != lastSSID || currentData.State != lastState
			heartbeatExpired := time.Since(lastRecordTime) >= 30*time.Second

			if diff < minDropDbm && !stateChanged && !heartbeatExpired {
				shouldWrite = false
			}
		}

		if shouldWrite {
			jsonData, err := json.Marshal(currentData)
			if err == nil {
				f, err := os.OpenFile(recordsPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
				if err == nil {
					_, _ = io.WriteString(f, string(jsonData)+"\n")
					f.Close()
				}
			}
			lastSignal = currentData.Signal
			lastSSID = currentData.SSID
			lastState = currentData.State
			lastRecordTime = time.Now()
		}

		time.Sleep(time.Duration(interval) * time.Second)
	}
}

// Парсинг дробных скоростей (например, "866.7" Мбит/с или "144,4")
func parseRate(val string) int32 {
	val = strings.TrimSpace(val)
	val = strings.ReplaceAll(val, ",", ".")
	if f, err := strconv.ParseFloat(val, 64); err == nil {
		return int32(math.Round(f))
	}
	if i, err := strconv.Atoi(val); err == nil {
		return int32(i)
	}
	return 0
}

// Парсер netsh wlan show interfaces
func parseWindowsWifi() WifiData {
	res := WifiData{
		Time:      time.Now().Format("2006-01-02 15:04:05"),
		Interface: "Неизвестно",
		SSID:      "Disconnected",
		BSSID:     "--:--:--:--:--:--",
		Signal:    -100,
		Channel:   0,
		Standard:  "None",
		RxRate:    0,
		TxRate:    0,
		State:     "отключено",
	}

	cmd := exec.Command("netsh", "wlan", "show", "interfaces")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}

	out, err := cmd.Output()
	if err != nil {
		return res
	}

	// Декодируем OEM CP866 вывод Windows консоли
	decoder := charmap.CodePage866.NewDecoder()
	utf8Bytes, err := decoder.Bytes(out)
	var rawStr string
	if err == nil {
		rawStr = string(utf8Bytes)
	} else {
		rawStr = string(out)
	}

	lines := strings.Split(rawStr, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch {
		case key == "Имя" || key == "Name":
			res.Interface = val
		case key == "SSID" || key == "Служба SSID":
			res.SSID = val
		case key == "BSSID" || key == "BSSID-адрес":
			res.BSSID = val
		case key == "Состояние" || key == "State":
			res.State = val
		case key == "Тип радиосети" || key == "Radio type":
			res.Standard = val
		case key == "Канал" || key == "Channel":
			ch, _ := strconv.Atoi(val)
			res.Channel = int32(ch)
		case strings.Contains(key, "Прием") || strings.Contains(key, "Receive rate"):
			res.RxRate = parseRate(val)
		case strings.Contains(key, "Передача") || strings.Contains(key, "Transmit rate"):
			res.TxRate = parseRate(val)
		case key == "Сигнал" || key == "Signal":
			val = strings.TrimSuffix(val, "%")
			pct, _ := strconv.Atoi(val)
			res.Signal = int32((pct / 2) - 100)
		}
	}
	return res
}
