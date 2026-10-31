// ==========================================================================
// GoOverseer Wi-Fi Telemetry, Analytics, Scanner & AP Security Logic
// ==========================================================================

let wifiChartInstance = null;
let lastRenderedTimestamp = "";
let cachedLogData = [];
let currentPointsLimit = 30;
let isPaused = false;
let currentLogFilter = 'all';
let searchQuery = '';

let pollTimer = null;
let pollIntervalMs = 3000;
let autoPingTimer = null;

document.addEventListener("DOMContentLoaded", () => {
    // Восстанавливаем сохраненный интервал опроса
    const savedInterval = localStorage.getItem('wifi_poll_interval');
    if (savedInterval) {
        const selectEl = document.getElementById('poll-interval-select');
        if (selectEl) selectEl.value = savedInterval;
        pollIntervalMs = parseInt(savedInterval, 10) * 1000;
        const pollLabel = document.getElementById('chart-poll-label');
        if (pollLabel) pollLabel.innerText = `каждые ${savedInterval} сек`;
    }

    initWifiChart();
    refreshWifiAnalytics(true);
    startPollingTimer();

    // Восстанавливаем фоновый пинг, если был активирован
    const savedAutoPing = localStorage.getItem('wifi_auto_ping') === 'true';
    const autoPingToggle = document.getElementById('toggle-auto-ping');
    if (autoPingToggle && savedAutoPing) {
        autoPingToggle.checked = true;
        toggleAutoPing(true);
    }

    // При возвращении на вкладку сразу обновляем данные
    document.addEventListener("visibilitychange", () => {
        if (!document.hidden && !isPaused) {
            refreshWifiAnalytics(true);
        }
    });

    // Обработчик переключения на вкладку безопасности
    const secTabBtn = document.getElementById('security-tab');
    if (secTabBtn) {
        secTabBtn.addEventListener('shown.bs.tab', () => {
            analyzeApRegistryAndSecurity();
        });
    }
});

function startPollingTimer() {
    if (pollTimer) clearInterval(pollTimer);
    pollTimer = setInterval(() => {
        if (!isPaused && !document.hidden) {
            refreshWifiAnalytics(false);
        }
    }, pollIntervalMs);
}

// Регулировка частоты обновления графика и телеметрии
function changePollInterval(secondsStr) {
    const sec = parseInt(secondsStr, 10) || 3;
    pollIntervalMs = sec * 1000;
    localStorage.setItem('wifi_poll_interval', sec);

    const pollLabel = document.getElementById('chart-poll-label');
    if (pollLabel) pollLabel.innerText = `каждые ${sec} сек`;

    // Синхронизируем интервал опроса с фоновым демоном через config.json
    const id = new URLSearchParams(window.location.search).get('id') || "WIFI";
    const minDrop = document.getElementById("cfg_live_min_signal_drop_dbm") ? document.getElementById("cfg_live_min_signal_drop_dbm").value : "0";
    const maxMb = document.getElementById("cfg_live_max_log_mb") ? document.getElementById("cfg_live_max_log_mb").value : "5";

    const cfgInput = document.getElementById("cfg_live_interval_sec");
    if (cfgInput) cfgInput.value = sec;

    fetch(`/api/utils/save?id=${encodeURIComponent(id)}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            interval_sec: String(sec),
            min_signal_drop_dbm: String(minDrop),
            max_log_mb: String(maxMb)
        })
    }).catch(e => console.warn('Не удалось обновить конфиг демона:', e));

    startPollingTimer();
    refreshWifiAnalytics(true);

    if (typeof showToast === 'function') {
        showToast(`Частота опроса телеметрии установлена: каждые ${sec} сек`, 'info');
    }
}

// Инициализация Chart.js графика радиоэфира
function initWifiChart() {
    const ctx = document.getElementById('wifiSignalChart');
    if (!ctx || typeof Chart === 'undefined') return;

    if (wifiChartInstance) {
        wifiChartInstance.destroy();
        wifiChartInstance = null;
    }

    const canvasCtx = ctx.getContext('2d');
    const gradient = canvasCtx.createLinearGradient(0, 0, 0, 350);
    gradient.addColorStop(0, 'rgba(6, 182, 212, 0.35)');
    gradient.addColorStop(1, 'rgba(6, 182, 212, 0.0)');

    wifiChartInstance = new Chart(canvasCtx, {
        type: 'line',
        data: {
            labels: [],
            datasets: [{
                label: 'Мощность сигнала (dBm)',
                data: [],
                borderColor: '#06b6d4',
                backgroundColor: gradient,
                borderWidth: 2.5,
                pointRadius: 2,
                pointHoverRadius: 6,
                pointBackgroundColor: '#22d3ee',
                pointBorderColor: '#0b0f19',
                pointBorderWidth: 2,
                tension: 0.35,
                fill: true
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            animation: false,
            normalized: true,
            plugins: {
                legend: { display: false },
                tooltip: {
                    animation: false,
                    backgroundColor: 'rgba(15, 23, 42, 0.95)',
                    titleColor: '#f8fafc',
                    bodyColor: '#38bdf8',
                    borderColor: 'rgba(255, 255, 255, 0.1)',
                    borderWidth: 1,
                    padding: 10,
                    boxPadding: 4,
                    usePointStyle: true,
                    callbacks: {
                        label: function(context) {
                            const val = context.parsed.y;
                            let quality = "Отличное";
                            if (val < -80) quality = "Очень слабое";
                            else if (val < -70) quality = "Удовлетворительное";
                            else if (val < -60) quality = "Хорошее";
                            return ` Сигнал: ${val} dBm (${quality})`;
                        }
                    }
                }
            },
            scales: {
                y: {
                    min: -100,
                    max: -30,
                    grid: { color: 'rgba(255, 255, 255, 0.05)' },
                    ticks: {
                        color: '#94a3b8',
                        stepSize: 10,
                        callback: function(v) { return v + ' dBm'; }
                    },
                    title: {
                        display: true,
                        text: 'Уровень сигнала (dBm)',
                        color: '#64748b',
                        font: { size: 11, weight: 'bold' }
                    }
                },
                x: {
                    grid: { color: 'rgba(255, 255, 255, 0.04)' },
                    ticks: {
                        color: '#94a3b8',
                        maxRotation: 0,
                        autoSkip: true,
                        maxTicksLimit: 12
                    },
                    title: {
                        display: true,
                        text: 'Время замеров',
                        color: '#64748b',
                        font: { size: 11, weight: 'bold' }
                    }
                }
            }
        }
    });
}

function formatBytes(bytes) {
    if (!bytes || isNaN(bytes) || bytes <= 0) return "0 КБ";
    if (bytes < 1024) return bytes + " Б";
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + " КБ";
    return (bytes / (1024 * 1024)).toFixed(2) + " МБ";
}

function refreshWifiAnalytics(forceRender = false) {
    const rawId = new URLSearchParams(window.location.search).get('id') || "WIFI";

    // Подгрузка полей конфигурации при первом открытии
    const fieldsContainer = document.getElementById("wifi-custom-config-fields");
    if (fieldsContainer && fieldsContainer.children.length === 0) {
        fieldsContainer.innerHTML = `
            <div class="col-md-4">
                <label class="form-label text-light small fw-bold">Интервал опроса адаптера (сек)</label>
                <div class="input-group input-group-sm">
                    <input type="number" min="1" max="300" class="form-control bg-dark text-light border-secondary" id="cfg_live_interval_sec" value="5">
                    <span class="input-group-text bg-dark text-muted border-secondary">сек</span>
                </div>
                <div class="form-text text-muted small">Частота считывания netsh в фоновом режиме.</div>
            </div>
            <div class="col-md-4">
                <label class="form-label text-light small fw-bold">Порог фиксации сигнала (dBm)</label>
                <div class="input-group input-group-sm">
                    <input type="number" min="0" max="50" class="form-control bg-dark text-light border-secondary" id="cfg_live_min_signal_drop_dbm" value="0">
                    <span class="input-group-text bg-dark text-muted border-secondary">dBm</span>
                </div>
                <div class="form-text text-muted small">Минимальное колебание для внеочередной записи.</div>
            </div>
            <div class="col-md-4">
                <label class="form-label text-light small fw-bold">Максимальный размер лога (МБ)</label>
                <div class="input-group input-group-sm">
                    <input type="number" min="1" max="100" class="form-control bg-dark text-light border-secondary" id="cfg_live_max_log_mb" value="5">
                    <span class="input-group-text bg-dark text-muted border-secondary">МБ</span>
                </div>
                <div class="form-text text-muted small">При превышении создается резервный .old архив.</div>
            </div>
        `;

        fetch(`/api/utils`)
            .then(res => res.json())
            .then(utils => {
                const wifi = utils.find(u => u.id.toLowerCase() === rawId.toLowerCase());
                if (wifi && wifi.settings) {
                    if (wifi.settings.interval_sec) document.getElementById("cfg_live_interval_sec").value = wifi.settings.interval_sec;
                    if (wifi.settings.min_signal_drop_dbm) document.getElementById("cfg_live_min_signal_drop_dbm").value = wifi.settings.min_signal_drop_dbm;
                    if (wifi.settings.max_log_mb) document.getElementById("cfg_live_max_log_mb").value = wifi.settings.max_log_mb;
                }
            })
            .catch(() => {});
    }

    // Запрос логов телеметрии
    fetch(`/api/utils/logs?id=${encodeURIComponent(rawId.toUpperCase())}`)
        .then(res => {
            const sizeHeader = res.headers.get("X-Log-Size-Bytes");
            if (sizeHeader) {
                const sizeBadge = document.getElementById("log-size-badge");
                if (sizeBadge) {
                    sizeBadge.innerText = formatBytes(parseInt(sizeHeader, 10));
                }
            }
            return res.json();
        })
        .then(data => {
            if (Array.isArray(data)) {
                cachedLogData = data;

                if (!forceRender && data.length > 0) {
                    const latestTime = data[data.length - 1].time;
                    if (latestTime === lastRenderedTimestamp) {
                        return;
                    }
                }

                renderAnalytics(data);
            }
        })
        .catch(err => {
            console.error("Ошибка получения логов:", err);
        });
}

// Отрисовка аналитики
function renderAnalytics(data) {
    const tableBody = document.getElementById("wifi-log-table-body");

    if (!wifiChartInstance && typeof Chart !== 'undefined') {
        initWifiChart();
    }

    // 1. ОБНОВЛЕНИЕ КАРТОЧЕК ЭКСПРЕСС-АНАЛИТИКИ
    if (data && data.length > 0) {
        const latest = data[data.length - 1];

        const elSsid = document.getElementById("card-ssid");
        const elBssid = document.getElementById("card-bssid");
        const elChannel = document.getElementById("card-channel");
        const elSignal = document.getElementById("card-signal");
        const elSpeeds = document.getElementById("card-speeds");
        const elStandard = document.getElementById("card-standard");
        const elInterface = document.getElementById("card-interface");
        const elBand = document.getElementById("card-frequency-band");
        const elQuality = document.getElementById("card-signal-quality");
        const elBadge = document.getElementById("badge-connection-state");
        const visualBars = document.getElementById("wifi-visual-bars");

        const isDisconnected = !latest.ssid || latest.ssid === "Disconnected" || latest.state === "отключено";

        if (elSsid) elSsid.innerText = isDisconnected ? "Отключено" : latest.ssid;
        if (elBssid) elBssid.innerText = latest.bssid || "—";
        if (elChannel) elChannel.innerText = latest.channel > 0 ? latest.channel : "—";
        if (elInterface) elInterface.innerText = latest.interface || "Wi-Fi";
        if (elStandard) elStandard.innerText = `Стандарт: ${latest.standard || '—'}`;

        if (elBand) {
            if (latest.channel >= 1 && latest.channel <= 14) {
                elBand.innerText = "2.4 ГГц";
                elBand.className = "badge bg-info-subtle text-info rounded-pill px-2 py-1";
            } else if (latest.channel >= 32 && latest.channel <= 177) {
                elBand.innerText = "5 ГГц";
                elBand.className = "badge bg-primary-subtle text-primary rounded-pill px-2 py-1";
            } else if (latest.channel > 177) {
                elBand.innerText = "6 ГГц";
                elBand.className = "badge bg-purple-subtle text-purple rounded-pill px-2 py-1";
            } else {
                elBand.innerText = "—";
            }
        }

        if (elSpeeds) {
            const rx = latest.rx_rate || 0;
            const tx = latest.tx_rate || 0;
            elSpeeds.innerText = `Линк: ⬇ ${rx} Мб / ⬆ ${tx} Мб`;
        }

        const sig = latest.signal ?? -100;
        if (elSignal) {
            elSignal.innerText = `${sig} dBm`;
            elSignal.className = "mb-1 fw-bold text-white";
        }

        let qualityPercent = Math.min(100, Math.max(0, (sig + 100) * 2));
        let qualityText = "Отличное";
        let barLevel = "signal-level-4";

        if (isDisconnected || sig <= -95) {
            qualityText = "Нет связи";
            barLevel = "signal-level-1";
        } else if (sig <= -80) {
            qualityText = "Слабый сигнал";
            barLevel = "signal-level-1";
        } else if (sig <= -70) {
            qualityText = "Удовлетворительное";
            barLevel = "signal-level-2";
        } else if (sig <= -60) {
            qualityText = "Хорошее качество";
            barLevel = "signal-level-3";
        }

        if (elQuality) {
            elQuality.innerText = `${qualityText} (${qualityPercent}%)`;
        }

        if (visualBars) {
            visualBars.className = `wifi-signal-bars ${barLevel}`;
        }

        if (elBadge) {
            if (isDisconnected) {
                elBadge.className = "badge badge-custom badge-disconnected";
                elBadge.innerText = "Отключено";
            } else {
                elBadge.className = "badge badge-custom badge-connected";
                elBadge.innerText = "Связь активна";
            }
        }
    }

    // 2. ОБНОВЛЕНИЕ ГРАФИКА CHART.JS
    if (wifiChartInstance && data && data.length > 0) {
        let chartData = data;
        if (currentPointsLimit > 0 && data.length > currentPointsLimit) {
            chartData = data.slice(-currentPointsLimit);
        }

        const labels = [];
        const points = [];
        let minSig = 0;
        let maxSig = -120;
        let sumSig = 0;

        for (let i = 0; i < chartData.length; i++) {
            const entry = chartData[i];
            let label = "—";
            if (entry.time) {
                label = entry.time.includes(" ") ? entry.time.split(" ")[1] : entry.time;
            }
            labels.push(label);
            points.push(entry.signal);

            if (entry.signal > maxSig) maxSig = entry.signal;
            if (minSig === 0 || entry.signal < minSig) minSig = entry.signal;
            sumSig += entry.signal;
        }

        wifiChartInstance.data.labels = labels;
        wifiChartInstance.data.datasets[0].data = points;
        wifiChartInstance.update('none');

        const latestSig = points[points.length - 1];
        const avgSig = Math.round(sumSig / points.length);

        const elStatCurr = document.getElementById("chart-stat-current");
        const elStatMax = document.getElementById("chart-stat-max");
        const elStatMin = document.getElementById("chart-stat-min");
        const elStatAvg = document.getElementById("chart-stat-avg");

        if (elStatCurr) elStatCurr.innerText = `${latestSig} dBm`;
        if (elStatMax) elStatMax.innerText = `${maxSig} dBm`;
        if (elStatMin) elStatMin.innerText = `${minSig} dBm`;
        if (elStatAvg) elStatAvg.innerText = `${avgSig} dBm`;
    }

    // 3. ОБНОВЛЕНИЕ ТАБЛИЦЫ ИСТОРИИ
    if (!tableBody) return;

    if (!data || data.length === 0) {
        tableBody.innerHTML = `<tr><td colspan="8" class="text-center text-muted py-5">История подключений пока пуста...</td></tr>`;
        return;
    }

    let filteredData = data;

    if (currentLogFilter === 'connected') {
        filteredData = filteredData.filter(d => d.ssid && d.ssid !== "Disconnected" && d.state !== "отключено");
    } else if (currentLogFilter === 'disconnected') {
        filteredData = filteredData.filter(d => !d.ssid || d.ssid === "Disconnected" || d.state === "отключено");
    }

    if (searchQuery.trim() !== '') {
        const q = searchQuery.toLowerCase();
        filteredData = filteredData.filter(d =>
            (d.ssid && d.ssid.toLowerCase().includes(q)) ||
            (d.bssid && d.bssid.toLowerCase().includes(q)) ||
            (String(d.channel).includes(q)) ||
            (d.time && d.time.toLowerCase().includes(q))
        );
    }

    const counterEl = document.getElementById("records-counter");
    if (counterEl) counterEl.innerText = filteredData.length;

    if (filteredData.length === 0) {
        tableBody.innerHTML = `<tr><td colspan="8" class="text-center text-muted py-4">События по заданному фильтру не найдены.</td></tr>`;
        return;
    }

    const displayRows = filteredData.slice(-60).reverse();
    const htmlChunks = [];

    for (let i = 0; i < displayRows.length; i++) {
        const entry = displayRows[i];
        const isDisc = !entry.ssid || entry.ssid === "Disconnected" || entry.state === "отключено";

        let statusBadge = `<span class="badge badge-custom badge-connected">Подключено</span>`;
        let signalClass = "text-success";

        if (isDisc) {
            statusBadge = `<span class="badge badge-custom badge-disconnected">Отключено</span>`;
            signalClass = "text-danger";
        } else if (entry.signal < -75) {
            signalClass = "text-warning";
        }

        const channelBadge = entry.channel > 0
            ? `<span class="badge bg-secondary-subtle text-light">${entry.channel}</span>`
            : `<span class="text-muted">—</span>`;

        htmlChunks.push(`
            <tr>
                <td><code>${entry.time || '—'}</code></td>
                <td><strong class="text-white">${entry.ssid || 'Disconnected'}</strong></td>
                <td><code class="text-secondary">${entry.bssid || '—'}</code></td>
                <td>${channelBadge}</td>
                <td><small class="text-muted">${entry.standard || '—'}</small></td>
                <td><small class="text-muted">⬇${entry.rx_rate || 0} / ⬆${entry.tx_rate || 0} Мб</small></td>
                <td><strong class="${signalClass}">${entry.signal ?? -100} dBm</strong></td>
                <td>${statusBadge}</td>
            </tr>
        `);
    }

    tableBody.innerHTML = htmlChunks.join("");
    lastRenderedTimestamp = data[data.length - 1].time;
}

// ==========================================================================
// Мониторинг задержки шлюза / пинга
// ==========================================================================

function checkGatewayPing() {
    const badge = document.getElementById('ping-result-badge');
    const details = document.getElementById('ping-details-text');
    const spinner = document.getElementById('ping-btn-spinner');

    if (spinner) spinner.classList.remove('d-none');
    if (badge) {
        badge.className = "badge bg-secondary-subtle text-light rounded-pill px-2 py-1";
        badge.innerText = "Измерение...";
    }

    fetch('/api/utils/wifi/ping?host=8.8.8.8')
        .then(res => res.json())
        .then(resp => {
            if (spinner) spinner.classList.add('d-none');
            if (!resp.alive) {
                if (badge) {
                    badge.className = "badge bg-danger-subtle text-danger rounded-pill px-2 py-1";
                    badge.innerText = "Обрыв связи / Таймаут";
                }
                if (details) details.innerText = `Шлюз не отвечает (${new Date().toLocaleTimeString()})`;
                return;
            }

            const ms = resp.latency_ms;
            let statusBadgeClass = "badge bg-success-subtle text-success rounded-pill px-2 py-1";
            let statusText = "Отлично";

            if (ms > 120) {
                statusBadgeClass = "badge bg-danger-subtle text-danger rounded-pill px-2 py-1";
                statusText = "Высокая задержка";
            } else if (ms > 50) {
                statusBadgeClass = "badge bg-warning-subtle text-warning rounded-pill px-2 py-1";
                statusText = "Удовлетворительно";
            }

            if (badge) {
                badge.className = statusBadgeClass;
                badge.innerText = `${ms} мс (${statusText})`;
            }
            if (details) {
                details.innerText = `Шлюз 8.8.8.8 стабилен • Время проверки: ${new Date().toLocaleTimeString()}`;
            }
        })
        .catch(err => {
            if (spinner) spinner.classList.add('d-none');
            if (badge) {
                badge.className = "badge bg-danger-subtle text-danger rounded-pill px-2 py-1";
                badge.innerText = "Ошибка проверки";
            }
        });
}

function toggleAutoPing(enabled) {
    localStorage.setItem('wifi_auto_ping', enabled ? 'true' : 'false');
    if (autoPingTimer) clearInterval(autoPingTimer);

    if (enabled) {
        checkGatewayPing();
        autoPingTimer = setInterval(() => {
            if (!document.hidden && !isPaused) {
                checkGatewayPing();
            }
        }, 15000);
        if (typeof showToast === 'function') {
            showToast("Фоновый мониторинг задержки шлюза активирован (каждые 15 сек)", "info");
        }
    } else {
        if (typeof showToast === 'function') {
            showToast("Фоновый мониторинг задержки отключен", "info");
        }
    }
}

// ==========================================================================
// Сканер зашумленности радиоэфира
// ==========================================================================

function performAirScan() {
    const spinner = document.getElementById('scan-btn-spinner');
    const tableBody = document.getElementById('scan-table-body');
    const totalCountEl = document.getElementById('scan-total-count');
    const scan5ghzBadge = document.getElementById('scan-5ghz-badge');

    if (spinner) spinner.classList.remove('d-none');
    if (tableBody) {
        tableBody.innerHTML = `<tr><td colspan="7" class="text-center text-info py-4"><span class="spinner-border spinner-border-sm me-2"></span>Аппаратный опрос радиоэфира (WlanScan)... Пожалуйста, подождите 1.5 сек.</td></tr>`;
    }

    fetch('/api/utils/wifi/scan')
        .then(res => res.json())
        .then(data => {
            if (spinner) spinner.classList.add('d-none');

            // Рекомендация
            const recTitle = document.getElementById('scan-rec-title');
            const recDesc = document.getElementById('scan-rec-desc');
            if (recTitle) recTitle.innerText = `Рекомендованный свободный канал 2.4 ГГц: Канал ${data.recommended_channel}`;
            if (recDesc) {
                recDesc.innerText = `Обнаружено точек: ${data.total_bssids}. Канал ${data.recommended_channel} имеет наименьшее количество конкурирующих роутеров вокруг вас. Время сканирования: ${data.scan_time}.`;
            }

            // Шкалы каналов 2.4 ГГц
            renderCrowdBars('crowd-bars-24', data.channel_crowd_24, [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13], data.recommended_channel);

            // Обработка 5 ГГц и проверка поддержки оборудованием
            if (scan5ghzBadge) {
                if (data.supports_5ghz) {
                    scan5ghzBadge.className = 'badge bg-success-subtle text-success border border-success';
                    scan5ghzBadge.innerText = 'Адаптер 5 ГГц поддерживается';
                } else {
                    scan5ghzBadge.className = 'badge bg-warning-subtle text-warning border border-warning';
                    scan5ghzBadge.innerText = 'Адаптер 2.4 ГГц (5 ГГц не поддерживается)';
                }
            }

            if (!data.supports_5ghz) {
                const c5 = document.getElementById('crowd-bars-5');
                if (c5) {
                    c5.innerHTML = `
                        <div class="p-3 text-center text-warning small">
                            <i class="bi bi-exclamation-triangle-fill fs-5 d-block mb-1"></i>
                            <strong>Физическое ограничение оборудования:</strong><br>
                            Ваш Wi-Fi адаптер поддерживает стандарты: <strong>${data.radio_types || '802.11b/g/n (2.4 GHz)'}</strong>.<br>
                            Диапазон 5 ГГц физически не поддерживается чипом адаптера, поэтому сети 5 ГГц не принимаются.
                        </div>
                    `;
                }
            } else {
                renderCrowdBars('crowd-bars-5', data.channel_crowd_5, [36, 40, 44, 48, 52, 56, 149, 153, 157, 161], data.recommended_channel_5 || null);
            }

            // Таблица обнаруженных точек
            if (totalCountEl) totalCountEl.innerText = data.total_bssids;

            if (!data.networks || data.networks.length === 0) {
                if (tableBody) tableBody.innerHTML = `<tr><td colspan="7" class="text-center text-muted py-4">Точки доступа вокруг не обнаружены.</td></tr>`;
                return;
            }

            const rows = [];
            data.networks.forEach(net => {
                net.bssids.forEach(b => {
                    const signalColor = b.signal >= 70 ? 'text-success' : (b.signal >= 40 ? 'text-warning' : 'text-danger');
                    rows.push(`
                        <tr>
                            <td><strong class="text-white">${net.ssid || 'Скрытая сеть'}</strong></td>
                            <td><code class="text-secondary">${b.bssid}</code></td>
                            <td><span class="badge bg-secondary-subtle text-light">${b.channel}</span></td>
                            <td><small class="text-muted">${b.band}</small></td>
                            <td><strong class="${signalColor}">${b.signal}% (${b.signal_dbm} dBm)</strong></td>
                            <td><small class="text-muted">${b.radio || '—'}</small></td>
                            <td><small class="text-muted">${net.auth || '—'} / ${net.encrypt || '—'}</small></td>
                        </tr>
                    `);
                });
            });

            if (tableBody) tableBody.innerHTML = rows.join("");
            if (typeof showToast === 'function') {
                showToast(`Радиоэфир просканирован! Найдено ${data.total_bssids} точек доступа.`, 'success');
            }
        })
        .catch(err => {
            if (spinner) spinner.classList.add('d-none');
            if (tableBody) tableBody.innerHTML = `<tr><td colspan="7" class="text-center text-danger py-4">Ошибка сканирования сетей: ${err}</td></tr>`;
        });
}

function renderCrowdBars(containerId, crowdMap, channels, recommendedCh) {
    const container = document.getElementById(containerId);
    if (!container) return;

    let maxCrowd = 1;
    channels.forEach(ch => {
        const count = crowdMap[ch] || 0;
        if (count > maxCrowd) maxCrowd = count;
    });

    const html = channels.map(ch => {
        const count = crowdMap[ch] || 0;
        const pct = Math.min(100, Math.round((count / maxCrowd) * 100));
        const isRec = ch === recommendedCh;
        const barColor = isRec ? 'bg-success' : (count > 3 ? 'bg-danger' : (count > 0 ? 'bg-warning' : 'bg-secondary'));

        return `
            <div class="mb-2">
                <div class="d-flex justify-content-between small">
                    <span class="${isRec ? 'text-success fw-bold' : 'text-light'}">Канал ${ch} ${isRec ? '★ (Свободный)' : ''}</span>
                    <span class="text-muted">${count} сетей</span>
                </div>
                <div class="channel-bar-container">
                    <div class="channel-bar-fill ${barColor}" style="width: ${count > 0 ? Math.max(8, pct) : 0}%;"></div>
                </div>
            </div>
        `;
    }).join("");

    container.innerHTML = html;
}

// ==========================================================================
// Детектор подмены точек доступа (Сумматор BSSID и аномалий)
// ==========================================================================

function analyzeApRegistryAndSecurity() {
    if (!cachedLogData || cachedLogData.length === 0) {
        document.getElementById('security-registry-table-body').innerHTML = `<tr><td colspan="7" class="text-center text-muted py-4">Журнал телеметрии пуст — запустите сбор данных.</td></tr>`;
        return;
    }

    const ssidRegistry = {};
    const anomalies = [];
    let lastSeenSsid = "";
    let lastSeenBssid = "";

    // Суммируем исторические данные
    cachedLogData.forEach(entry => {
        const ssid = entry.ssid;
        const bssid = entry.bssid;
        if (!ssid || ssid === "Disconnected" || !bssid || bssid === "--:--:--:--:--:--") {
            lastSeenSsid = "";
            lastSeenBssid = "";
            return;
        }

        // Детекция переключения точки в рамках одной сети
        if (lastSeenSsid === ssid && lastSeenBssid && lastSeenBssid !== bssid) {
            anomalies.push({
                time: entry.time,
                ssid: ssid,
                prevBssid: lastSeenBssid,
                newBssid: bssid
            });
        }
        lastSeenSsid = ssid;
        lastSeenBssid = bssid;

        if (!ssidRegistry[ssid]) {
            ssidRegistry[ssid] = {
                bssids: new Set(),
                channels: new Set(),
                count: 0,
                signalSum: 0,
                firstTime: entry.time,
                lastTime: entry.time
            };
        }

        const rec = ssidRegistry[ssid];
        rec.bssids.add(bssid);
        if (entry.channel > 0) rec.channels.add(entry.channel);
        rec.count++;
        rec.signalSum += entry.signal;
        rec.lastTime = entry.time;
    });

    const uniqueSsids = Object.keys(ssidRegistry);
    let totalUniqueBssids = 0;
    uniqueSsids.forEach(s => totalUniqueBssids += ssidRegistry[s].bssids.size);

    const elUssids = document.getElementById('sec-unique-ssids');
    const elUbssids = document.getElementById('sec-unique-bssids');
    const elAnomalies = document.getElementById('sec-anomaly-count');

    if (elUssids) elUssids.innerText = uniqueSsids.length;
    if (elUbssids) elUbssids.innerText = totalUniqueBssids;
    if (elAnomalies) elAnomalies.innerText = anomalies.length;

    // Заполняем таблицу реестра сетей
    const regTableBody = document.getElementById('security-registry-table-body');
    const regRows = [];

    uniqueSsids.forEach(ssid => {
        const info = ssidRegistry[ssid];
        const bssidList = Array.from(info.bssids);
        const channelList = Array.from(info.channels).join(", ") || "—";
        const avgSig = Math.round(info.signalSum / info.count);

        let statusBadge = `<span class="badge bg-success-subtle text-success">Одна точка (Стабильно)</span>`;
        if (bssidList.length > 1) {
            statusBadge = `<span class="badge badge-mesh">Множественные точки (${bssidList.length})</span>`;
        }

        const bssidDisplay = bssidList.map(b => `<code class="text-info d-inline-block me-2">${b}</code>`).join("");

        regRows.push(`
            <tr>
                <td><strong class="text-white">${ssid}</strong></td>
                <td>${bssidDisplay}</td>
                <td><span class="badge bg-secondary-subtle text-light">${channelList}</span></td>
                <td>${info.count}</td>
                <td><small class="text-muted">${info.firstTime} → ${info.lastTime}</small></td>
                <td><strong class="${avgSig >= -65 ? 'text-success' : 'text-warning'}">${avgSig} dBm</strong></td>
                <td>${statusBadge}</td>
            </tr>
        `);
    });

    if (regTableBody) {
        regTableBody.innerHTML = regRows.length > 0 ? regRows.join("") : `<tr><td colspan="7" class="text-center text-muted py-4">Данные сетей отсутствуют.</td></tr>`;
    }

    // Заполняем хронологию инцидентов / смены точек
    const anomTableBody = document.getElementById('security-anomalies-table-body');
    if (anomTableBody) {
        if (anomalies.length === 0) {
            anomTableBody.innerHTML = `<tr><td colspan="5" class="text-center text-success py-4">✓ Подмен или внезапных переключений точек доступа не зафиксировано. Все сети стабильны.</td></tr>`;
        } else {
            const anomRows = [...anomalies].reverse().map(anom => `
                <tr>
                    <td><code>${anom.time}</code></td>
                    <td><strong class="text-white">${anom.ssid}</strong></td>
                    <td><code class="text-secondary">${anom.prevBssid}</code></td>
                    <td><code class="text-warning">${anom.newBssid}</code></td>
                    <td><span class="badge badge-rogue-warning">⚠️ Переключение BSSID (Роуминг / Смена точки)</span></td>
                </tr>
            `);
            anomTableBody.innerHTML = anomRows.join("");
        }
    }
}

// Очистка логов модуля
function clearWifiLogs() {
    const rawId = new URLSearchParams(window.location.search).get('id') || "WIFI";
    if (confirm("Вы уверены, что хотите полностью очистить журнал телеметрии Wi-Fi? Это действие необратимо.")) {
        fetch(`/api/utils/logs/clear?id=${encodeURIComponent(rawId.toUpperCase())}`, { method: 'POST' })
            .then(res => {
                if (res.ok) {
                    if (typeof showToast === 'function') {
                        showToast("Журнал телеметрии успешно очищен!", 'info');
                    } else {
                        alert("Журнал успешно очищен!");
                    }
                    cachedLogData = [];
                    lastRenderedTimestamp = "";
                    renderAnalytics([]);
                    analyzeApRegistryAndSecurity();
                    const sizeBadge = document.getElementById("log-size-badge");
                    if (sizeBadge) sizeBadge.innerText = "0 КБ";
                } else {
                    if (typeof showToast === 'function') {
                        showToast("Не удалось очистить журнал.", 'danger');
                    } else {
                        alert("Ошибка очистки журнала.");
                    }
                }
            })
            .catch(err => {
                alert("Ошибка сети: " + err);
            });
    }
}

// Управление отображением графика
function setChartPointsLimit(limit) {
    currentPointsLimit = limit;

    document.querySelectorAll('[aria-label="Диапазон точек"] button').forEach(btn => {
        btn.classList.remove('active');
    });
    if (event && event.target) {
        event.target.classList.add('active');
    }

    renderAnalytics(cachedLogData);
}

function togglePauseLiveStream() {
    isPaused = !isPaused;
    const btnIcon = document.getElementById("pause-icon");
    if (btnIcon) {
        btnIcon.innerText = isPaused ? "▶ Возобновить" : "⏸ Пауза";
    }
}

// Фильтры и поиск по истории
function setLogFilter(filter) {
    currentLogFilter = filter;
    document.querySelectorAll('#filter-all, #filter-connected, #filter-disconnected').forEach(b => b.classList.remove('active'));
    const activeBtn = document.getElementById(`filter-${filter}`);
    if (activeBtn) activeBtn.classList.add('active');
    renderAnalytics(cachedLogData);
}

function handleSearchInput() {
    const input = document.getElementById("wifi-search-input");
    searchQuery = input ? input.value : "";
    renderAnalytics(cachedLogData);
}

// Копирование BSSID в буфер обмена
function copyBssidToClipboard() {
    const el = document.getElementById("card-bssid");
    if (!el || !el.innerText || el.innerText === "—") return;

    navigator.clipboard.writeText(el.innerText).then(() => {
        if (typeof showToast === 'function') {
            showToast(`BSSID скопирован: ${el.innerText}`, 'info');
        } else {
            alert(`BSSID скопирован: ${el.innerText}`);
        }
    });
}

// Экспорт истории в CSV
function exportLogsToCSV() {
    if (!cachedLogData || cachedLogData.length === 0) {
        alert("Нет данных для экспорта.");
        return;
    }

    let csv = "Time,SSID,BSSID,Channel,Standard,RxRate,TxRate,Signal,State\n";
    for (let i = 0; i < cachedLogData.length; i++) {
        const e = cachedLogData[i];
        csv += `"${e.time}","${e.ssid}","${e.bssid}",${e.channel},"${e.standard}",${e.rx_rate},${e.tx_rate},${e.signal},"${e.state}"\n`;
    }

    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.setAttribute("href", url);
    link.setAttribute("download", `wifi_history_${new Date().toISOString().slice(0, 10)}.csv`);
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    setTimeout(() => URL.revokeObjectURL(url), 1000);
}

// Экспорт истории в JSON
function exportLogsToJSON() {
    if (!cachedLogData || cachedLogData.length === 0) {
        alert("Нет данных для экспорта.");
        return;
    }

    const blob = new Blob([JSON.stringify(cachedLogData, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.setAttribute("href", url);
    link.setAttribute("download", `wifi_history_${new Date().toISOString().slice(0, 10)}.json`);
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    setTimeout(() => URL.revokeObjectURL(url), 1000);
}

// Сохранение настроек утилиты
function saveWifiSettings() {
    const id = new URLSearchParams(window.location.search).get('id') || "WIFI";
    const newSettings = {
        interval_sec: document.getElementById("cfg_live_interval_sec") ? document.getElementById("cfg_live_interval_sec").value : "5",
        min_signal_drop_dbm: document.getElementById("cfg_live_min_signal_drop_dbm") ? document.getElementById("cfg_live_min_signal_drop_dbm").value : "5",
        max_log_mb: document.getElementById("cfg_live_max_log_mb") ? document.getElementById("cfg_live_max_log_mb").value : "5"
    };

    fetch(`/api/utils/save?id=${encodeURIComponent(id)}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(newSettings)
    }).then(res => {
        if (res.ok) {
            if (typeof showToast === 'function') {
                showToast("Конфигурация успешно сохранена и применена на лету!", 'success');
            } else {
                alert("Конфигурация успешно сохранена!");
            }
            refreshWifiAnalytics(true);
        } else {
            if (typeof showToast === 'function') {
                showToast("Не удалось сохранить конфигурацию", 'danger');
            } else {
                alert("Ошибка сохранения конфигурации");
            }
        }
    });
}
