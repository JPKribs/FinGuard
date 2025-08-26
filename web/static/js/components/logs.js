// Logs Management
class LogsManager {
    static logsRefreshInterval = null;
    static currentPage = 0;
    static pageSize = 20;
    static totalLogs = 0;
    static currentLevel = '';

    static async loadLogs(page = 0, level = '') {
        try {
            window.Utils.showLoading('logsList');

            const offset = page * LogsManager.pageSize;
            const query = new URLSearchParams({
                limit: LogsManager.pageSize,
                offset: offset
            });

            if (level) query.append('level', level);

            const response = await window.APIClient.getLogs(`?${query.toString()}`);
            const data = response.data;

            LogsManager.totalLogs = data.total;
            LogsManager.currentPage = page;
            LogsManager.currentLevel = level;

            LogsManager.renderLogs(data.logs);
            LogsManager.renderPagination();
        } catch (error) {
            console.error('Failed to load logs:', error);
            const logsList = document.getElementById('logsList');
            logsList.innerHTML = `<p style="color: var(--color-danger);">Failed to load logs</p>`;
        }
    }

    static renderLogs(logs) {
        const logsList = document.getElementById('logsList');

        if (logs.length === 0) {
            logsList.innerHTML = '<p style="color: var(--color-text-secondary); text-align: center; padding: 2rem;">No logs found</p>';
            return;
        }

        logsList.innerHTML = logs.map(log => {
            let contextHtml = '';
            if (log.context && Object.keys(log.context).length > 0) {
                const contextItems = Object.entries(log.context)
                    .map(([key, value]) => `<span class="context-item"><strong>${window.Utils.escapeHtml(key)}:</strong> ${window.Utils.escapeHtml(String(value))}</span>`)
                    .join('<br>• ');
                contextHtml = `<div class="log-context">${contextItems}</div>`;
            }

            return `
                <div class="list-item log-entry">
                    <div class="log-content">
                        <div class="log-header">
                            <strong>${new Date(log.timestamp).toLocaleString()}</strong>
                            <span class="status ${log.level.toLowerCase()}">${log.level}</span>
                        </div>
                        <div class="log-message">${window.Utils.escapeHtml(log.message)}</div>
                        ${contextHtml}
                    </div>
                </div>
            `;
        }).join('');
    }

    static renderPagination() {
        const paginationContainer = document.getElementById('logsPagination');
        if (!paginationContainer) return;

        const totalPages = Math.ceil(LogsManager.totalLogs / LogsManager.pageSize);

        paginationContainer.innerHTML = `
            <button ${LogsManager.currentPage === 0 ? 'disabled' : ''} onclick="LogsManager.loadLogs(${LogsManager.currentPage - 1}, '${LogsManager.currentLevel}')">Previous</button>
            <span>Page ${LogsManager.currentPage + 1} of ${totalPages}</span>
            <button ${LogsManager.currentPage + 1 >= totalPages ? 'disabled' : ''} onclick="LogsManager.loadLogs(${LogsManager.currentPage + 1}, '${LogsManager.currentLevel}')">Next</button>
        `;
    }

    static startLogsRefresh(intervalMs = 30000) {
        if (LogsManager.logsRefreshInterval) {
            clearInterval(LogsManager.logsRefreshInterval);
        }

        LogsManager.logsRefreshInterval = setInterval(() => {
            const logsTab = document.getElementById('logs');
            if (logsTab && logsTab.classList.contains('active') && window.FinGuardConfig.ADMIN_TOKEN) {
                LogsManager.loadLogs(LogsManager.currentPage, LogsManager.currentLevel);
            }
        }, intervalMs);
    }

    static stopLogsRefresh() {
        if (LogsManager.logsRefreshInterval) {
            clearInterval(LogsManager.logsRefreshInterval);
            LogsManager.logsRefreshInterval = null;
        }
    }

    static initializeFilter() {
        const filter = document.getElementById('logLevelFilter');
        if (filter) {
            filter.addEventListener('change', () => {
                LogsManager.loadLogs(0, filter.value);
            });
        }
    }
}

// Export to global scope
window.LogsManager = LogsManager;