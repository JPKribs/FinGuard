
// LOGS MANAGEMENT

class LogsManager {
    static logsRefreshInterval = null;
    static currentPage = 0;
    static pageSize = 20;
    static totalLogs = 0;
    static currentLevel = '';

    // LOG LOADING

    // MARK: loadLogs
    static async loadLogs(page = 0, level = '') {
        try {
            window.Utils.showLoading('logsList');
            const response = await this.fetchLogs(page, level);
            this.updateLogsState(response.data, page, level);
            this.renderLogsData(response.data);
        } catch (error) {
            this.handleLogsError(error);
        }
    }

    // MARK: fetchLogs
    static async fetchLogs(page, level) {
        const query = this.buildLogsQuery(page, level);
        return await window.APIClient.getLogs(`?${query.toString()}`);
    }

    // MARK: buildLogsQuery
    static buildLogsQuery(page, level) {
        const offset = page * this.pageSize;
        const query = new URLSearchParams({
            limit: this.pageSize,
            offset: offset
        });

        if (level) {
            query.append('level', level);
        }

        return query;
    }

    // MARK: updateLogsState
    static updateLogsState(data, page, level) {
        this.totalLogs = data.total;
        this.currentPage = page;
        this.currentLevel = level;
    }

    // MARK: renderLogsData
    static renderLogsData(data) {
        this.renderLogs(data.logs);
        this.renderPagination();
    }

    // MARK: handleLogsError
    static handleLogsError(error) {
        console.error('Failed to load logs:', error);
        const logsList = document.getElementById('logsList');
        logsList.innerHTML = `<p style="color: var(--color-danger);">Failed to load logs</p>`;
    }

    // LOG RENDERING

    // MARK: renderLogs
    static renderLogs(logs) {
        const logsList = document.getElementById('logsList');

        if (logs.length === 0) {
            this.renderEmptyState(logsList);
            return;
        }

        logsList.innerHTML = this.generateLogsHTML(logs);
    }

    // MARK: renderEmptyState
    static renderEmptyState(container) {
        container.innerHTML = '<p style="color: var(--color-text-secondary); text-align: center; padding: 2rem;">No logs found</p>';
    }

    // MARK: generateLogsHTML
    static generateLogsHTML(logs) {
        return logs.map(log => this.generateLogEntryHTML(log)).join('');
    }

    // MARK: generateLogEntryHTML
    static generateLogEntryHTML(log) {
        const contextHtml = this.generateContextHTML(log.context);
        
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
    }

    // MARK: generateContextHTML
    static generateContextHTML(context) {
        if (!context || Object.keys(context).length === 0) {
            return '';
        }

        const contextItems = Object.entries(context)
            .map(([key, value]) => this.generateContextItem(key, value))
            .join('<br>• ');

        return `<div class="log-context">${contextItems}</div>`;
    }

    // MARK: generateContextItem
    static generateContextItem(key, value) {
        const escapedKey = window.Utils.escapeHtml(key);
        const escapedValue = window.Utils.escapeHtml(String(value));
        return `<span class="context-item"><strong>${escapedKey}:</strong> ${escapedValue}</span>`;
    }

    // PAGINATION

    // MARK: renderPagination
    static renderPagination() {
        const paginationContainer = document.getElementById('logsPagination');
        if (!paginationContainer) return;

        const totalPages = Math.ceil(this.totalLogs / this.pageSize);
        paginationContainer.innerHTML = this.generatePaginationHTML(totalPages);
    }

    // MARK: generatePaginationHTML
    static generatePaginationHTML(totalPages) {
        const prevDisabled = this.currentPage === 0 ? 'disabled' : '';
        const nextDisabled = this.currentPage + 1 >= totalPages ? 'disabled' : '';
        
        return `
            <button ${prevDisabled} onclick="LogsManager.loadLogs(${this.currentPage - 1}, '${this.currentLevel}')">Previous</button>
            <span>Page ${this.currentPage + 1} of ${totalPages}</span>
            <button ${nextDisabled} onclick="LogsManager.loadLogs(${this.currentPage + 1}, '${this.currentLevel}')">Next</button>
        `;
    }

    // AUTO-REFRESH

    // MARK: startLogsRefresh
    static startLogsRefresh(intervalMs = 30000) {
        this.stopLogsRefresh();
        
        this.logsRefreshInterval = setInterval(() => {
            this.refreshLogsIfActive();
        }, intervalMs);
    }

    // MARK: refreshLogsIfActive
    static refreshLogsIfActive() {
        const logsTab = document.getElementById('logs');
        const isActive = logsTab && logsTab.classList.contains('active');
        const hasToken = window.FinGuardConfig.ADMIN_TOKEN;
        
        if (isActive && hasToken) {
            this.loadLogs(this.currentPage, this.currentLevel);
        }
    }

    // MARK: stopLogsRefresh
    static stopLogsRefresh() {
        if (this.logsRefreshInterval) {
            clearInterval(this.logsRefreshInterval);
            this.logsRefreshInterval = null;
        }
    }

    // INITIALIZATION

    // MARK: initializeFilter
    static initializeFilter() {
        const filter = document.getElementById('logLevelFilter');
        if (filter) {
            filter.addEventListener('change', this.handleFilterChange.bind(this));
        }
    }

    // MARK: handleFilterChange
    static handleFilterChange(event) {
        const filterValue = event.target.value;
        this.loadLogs(0, filterValue);
    }
}

// GLOBAL SCOPE EXPORT

window.LogsManager = LogsManager;