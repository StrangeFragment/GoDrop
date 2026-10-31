// ==========================================================================
// GoOverseer Main Web Application Script
// ==========================================================================

function showToast(message, type = 'info') {
    const container = document.getElementById('toastContainer');
    if (!container) return;

    const bgClass = type === 'success' ? 'border-success text-success' : 
                    type === 'danger' ? 'border-danger text-danger' : 
                    'border-info text-info';

    const toastEl = document.createElement('div');
    toastEl.className = `toast toast-custom align-items-center ${bgClass} border show mb-2`;
    toastEl.setAttribute('role', 'alert');
    toastEl.setAttribute('aria-live', 'assertive');
    toastEl.setAttribute('aria-atomic', 'true');

    toastEl.innerHTML = `
        <div class="d-flex">
            <div class="toast-body fw-semibold text-white">
                ${message}
            </div>
            <button type="button" class="btn-close btn-close-white me-2 m-auto" data-bs-dismiss="toast" aria-label="Close"></button>
        </div>
    `;

    container.appendChild(toastEl);
    setTimeout(() => {
        toastEl.classList.remove('show');
        setTimeout(() => toastEl.remove(), 300);
    }, 4500);
}

function confirmShutdown() {
    const modalEl = document.getElementById('shutdownModal');
    if (modalEl && window.bootstrap) {
        const modal = new bootstrap.Modal(modalEl);
        modal.show();
    } else if (confirm("Остановить оркестратор GoOverseer?")) {
        executeShutdown();
    }
}

function executeShutdown() {
    fetch('/api/shutdown', { method: 'POST' })
        .then(() => {
            document.body.innerHTML = `
                <div class="d-flex align-items-center justify-content-center vh-100 bg-dark text-white text-center p-4">
                    <div>
                        <h2 class="mb-3 text-danger">Оркестратор GoOverseer остановлен</h2>
                        <p class="text-muted">Сервер успешно завершил работу. Вкладку браузера можно закрыть.</p>
                    </div>
                </div>
            `;
        });
}

document.addEventListener("DOMContentLoaded", () => {
    loadUtilities();
});

function loadUtilities() {
    const container = document.getElementById("utils-container");
    if (!container) return;

    fetch('/api/utils')
        .then(response => response.json())
        .then(utils => {
            container.innerHTML = "";

            if (!utils || utils.length === 0) {
                container.innerHTML = `
                    <div class="col-12 text-center py-5 glass-card">
                        <div class="text-muted fs-5">Доступные модули не обнаружены.</div>
                    </div>`;
                return;
            }

            utils.forEach(util => {
                const col = document.createElement("div");
                col.className = "col-12";

                let statusBadge = `
                    <span class="badge bg-secondary-subtle text-secondary border border-secondary-subtle px-3 py-2 rounded-pill">
                        Не активен
                    </span>`;
                
                let actionButton = `
                    <button class="btn btn-cyan btn-sm rounded-pill px-4 py-2" onclick="activateUtil('${util.id}')">
                        <svg width="14" height="14" fill="currentColor" viewBox="0 0 16 16" class="me-1" style="vertical-align: -2px;">
                            <path d="M11.534 7h3.932a.25.25 0 0 1 .192.41l-1.966 2.36a.25.25 0 0 1-.384 0l-1.966-2.36a.25.25 0 0 1 .192-.41zm-11 2h3.932a.25.25 0 0 0 .192-.41L2.692 6.23a.25.25 0 0 0-.384 0L.342 8.59A.25.25 0 0 0 .534 9z"/>
                            <path fill-rule="evenodd" d="M8 3c-1.552 0-2.94.707-3.857 1.818a.5.5 0 1 1-.771-.636A6.002 6.002 0 0 1 13.917 7H12.9A5.002 5.002 0 0 0 8 3zM3.1 9a5.002 5.002 0 0 0 8.757 2.182.5.5 0 1 1 .771.636A6.002 6.002 0 0 1 2.083 9H3.1z"/>
                        </svg>
                        Активировать модуль
                    </button>`;

                if (util.active) {
                    statusBadge = `
                        <span class="badge bg-success-subtle text-success border border-success-subtle px-3 py-2 rounded-pill d-inline-flex align-items-center gap-2">
                            <span class="pulse-dot"></span>
                            Активен (В автозагрузке)
                        </span>`;
                    
                    let viewButton = "";
                    if (util.view_type && util.view_type !== "") {
                        viewButton = `
                            <a href="/utils/view?id=${util.id}" class="btn btn-cyan btn-sm rounded-pill px-3 py-2">
                                <svg width="14" height="14" fill="currentColor" viewBox="0 0 16 16" class="me-1" style="vertical-align: -2px;">
                                    <path d="M0 0h1v15h15v1H0V0Zm10 3.5a.5.5 0 0 1 .5-.5h4a.5.5 0 0 1 .5.5v4a.5.5 0 0 1-1 0V4.707l-4.146 4.147a.5.5 0 0 1-.708 0L7 6.707l-3.646 3.647a.5.5 0 0 1-.708-.708l4-4a.5.5 0 0 1 .708 0L9.5 7.793l3.646-3.647L11 4.146a.5.5 0 0 1-.5-.5Z"/>
                                </svg>
                                Панель аналитики
                            </a>`;
                    }

                    actionButton = `
                        <div class="d-flex flex-wrap gap-2">
                            ${viewButton}
                            <button class="btn btn-outline-danger btn-sm rounded-pill px-3 py-2" onclick="deactivateUtil('${util.id}')">
                                Деактивировать
                            </button>
                        </div>
                    `;
                }

                col.innerHTML = `
                    <div class="card glass-card glass-card-interactive">
                        <div class="card-body p-4">
                            <div class="d-flex flex-column flex-md-row justify-content-between align-items-start gap-3 mb-3">
                                <div>
                                    <div class="d-flex align-items-center flex-wrap gap-3">
                                        <h4 class="card-title fw-bold m-0 text-white">${util.name}</h4>
                                        <div>${statusBadge}</div>
                                    </div>
                                    <p class="card-text text-muted mt-2 mb-0">${util.description}</p>
                                </div>
                                <div class="align-self-start align-self-md-center">
                                    ${actionButton}
                                </div>
                            </div>
                            <div class="d-flex flex-wrap gap-4 pt-3 border-top border-secondary-subtle text-muted small">
                                <div>Журнал: <code class="text-info">${util.log_file}</code></div>
                                <div>Процесс: <code class="text-warning">${util.exe_name}</code></div>
                                <div>Режим: <span class="text-light">${util.view_type || 'Стандарт'}</span></div>
                            </div>
                        </div>
                    </div>
                `;

                container.appendChild(col);
            });
        })
        .catch(err => {
            container.innerHTML = `
                <div class="col-12 text-center py-4 glass-card text-danger">
                    Ошибка связи с сервером GoOverseer: ${err}
                </div>`;
        });
}

function activateUtil(id) {
    fetch(`/api/utils/activate?id=${id}`, { method: 'POST' })
        .then(async res => {
            if (res.ok) {
                showToast(`Модуль ${id} успешно активирован и добавлен в автозагрузку!`, 'success');
                loadUtilities();
            } else {
                const errText = await res.text();
                showToast(`Ошибка активации: ${errText || 'Неизвестная ошибка'}`, 'danger');
            }
        })
        .catch(err => showToast(`Ошибка связи: ${err}`, 'danger'));
}

function deactivateUtil(id) {
    if (confirm(`Деактивировать модуль ${id}, выгрузить процесс и полностью зачистить все его следы с компьютера?`)) {
        fetch(`/api/utils/deactivate?id=${id}`, { method: 'POST' })
            .then(async res => {
                if (res.ok) {
                    showToast(`Модуль ${id} полностью деактивирован, следы зачищены.`, 'info');
                    loadUtilities();
                } else {
                    const errText = await res.text();
                    showToast(`Ошибка деактивации: ${errText || 'Неизвестная ошибка'}`, 'danger');
                }
            })
            .catch(err => showToast(`Ошибка связи: ${err}`, 'danger'));
    }
}
