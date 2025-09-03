
// LOGS MANAGEMENT

class LogsManager {
    static logsRefreshInterval = null;
    static currentPage = 0;
    static pageSize = 20;
    static totalLogs = 0;
    static currentLevel = '';

    // LOG LOADING

    // MARK: loadLogs
    // Loads logs with pagination and level filtering
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
    // Fetches logs from the API with query parameters
    static async fetchLogs(page, level) {
        const query = this.buildLogsQuery(page, level);
        return await window.APIClient.getLogs(`?${query.toString()}`);
    }

    // MARK: buildLogsQuery
    // Builds URL search parameters for logs API request
    static buildLogsQuery(page, level) {
        const offset = page * this.pageSize;
        const query = new URLSearchParams({
            limit: this.pageSize,
            offset: offset
        });

        if (level && level.trim() !== '') {
            query.append('level', level);
        }

        return query;
    }

    // MARK: updateLogsState
    // Updates internal state tracking variables
    static updateLogsState(data, page, level) {
        this.totalLogs = data.total;
        this.currentPage = page;
        this.currentLevel = level;
        
        // Update the filter dropdown to match current level
        const filter = document.getElementById('logLevelFilter');
        if (filter) {
            filter.value = level;
        }
    }

    // MARK: renderLogsData
    // Renders both logs and pagination
    static renderLogsData(data) {
        this.renderLogs(data.logs);
        this.renderPagination();
    }

    // MARK: handleLogsError
    // Handles errors during log loading
    static handleLogsError(error) {
        console.error('Failed to load logs:', error);
        const logsList = document.getElementById('logsList');
        logsList.innerHTML = `<p style="color: var(--color-danger);">Failed to load logs: ${error.message}</p>`;
    }

    // LOG RENDERING

    // MARK: renderLogs
    // Renders the list of log entries
    static renderLogs(logs) {
        const logsList = document.getElementById('logsList');

        if (logs.length === 0) {
            this.renderEmptyState(logsList);
            return;
        }

        logsList.innerHTML = this.generateLogsHTML(logs);
    }

    // MARK: renderEmptyState
    // Renders empty state message
    static renderEmptyState(container) {
        const levelText = this.currentLevel ? ` for level "${this.currentLevel.toUpperCase()}"` : '';
        container.innerHTML = `<p style="color: var(--color-text-secondary); text-align: center; padding: 2rem;">No logs found${levelText}</p>`;
    }

    // MARK: generateLogsHTML
    // Generates HTML for all log entries
    static generateLogsHTML(logs) {
        return logs.map(log => this.generateLogEntryHTML(log)).join('');
    }

    // MARK: generateLogEntryHTML
    // Generates HTML for a single log entry
    static generateLogEntryHTML(log) {
        const contextHtml = this.generateContextHTML(log.context);
        
        return `
            <div class="list-item log-entry">
                <div class="log-content">
                    <div class="log-header">
                        <strong>${new Date(log.timestamp).toLocaleString()}</strong>
                        <span class="status ${log.level.toLowerCase()}">${log.level.toUpperCase()}</span>
                    </div>
                    <div class="log-message">${window.Utils.escapeHtml(log.message)}</div>
                    ${contextHtml}
                </div>
            </div>
        `;
    }

    // MARK: generateContextHTML
    // Generates HTML for log context data
    static generateContextHTML(context) {
        if (!context || Object.keys(context).length === 0) {
            return '';
        }

        const contextItems = Object.entries(context)
            .map(([key, value]) => this.generateContextItem(key, value))
            .join('');

        return `<div class="log-context">${contextItems}</div>`;
    }

    // MARK: generateContextItem
    // Generates HTML for a single context item
    static generateContextItem(key, value) {
        const escapedKey = window.Utils.escapeHtml(key);
        const escapedValue = window.Utils.escapeHtml(String(value));
        return `<span class="context-item"><strong>${escapedKey}:</strong> ${escapedValue}</span>`;
    }

    // PAGINATION

    // MARK: renderPagination
    // Renders pagination controls
    static renderPagination() {
        const paginationContainer = document.getElementById('logsPagination');
        if (!paginationContainer) return;

        const totalPages = Math.ceil(this.totalLogs / this.pageSize);
        
        if (totalPages <= 1) {
            paginationContainer.innerHTML = '';
            return;
        }

        paginationContainer.innerHTML = this.generatePaginationHTML(totalPages);
    }

    // MARK: generatePaginationHTML
    // Generates HTML for pagination controls
    static generatePaginationHTML(totalPages) {
        const prevDisabled = this.currentPage === 0 ? 'disabled' : '';
        const nextDisabled = this.currentPage + 1 >= totalPages ? 'disabled' : '';
        
        return `
            <div class="form-controls">
                <button ${prevDisabled} onclick="window.LogsManager.loadLogs(${this.currentPage - 1}, '${this.currentLevel}')">Previous</button>
                <button ${nextDisabled} onclick="window.LogsManager.loadLogs(${this.currentPage + 1}, '${this.currentLevel}')">Next</button>
                <span>Page ${this.currentPage + 1} of ${totalPages} (${this.totalLogs} logs)</span>
            </div>
        `;
    }

    // FILTER HANDLING

    // MARK: initializeFilter
    // Sets up the log level filter event listener
    static initializeFilter() {
        const filter = document.getElementById('logLevelFilter');
        if (filter) {
            // Remove any existing listeners first
            filter.removeEventListener('change', this.handleFilterChange);
            // Add the event listener
            filter.addEventListener('change', this.handleFilterChange.bind(this));
            console.log('Log filter initialized');
        } else {
            console.warn('Log filter element not found');
        }
    }

    // MARK: handleFilterChange
    // Handles filter dropdown changes
    static handleFilterChange(event) {
        const filterValue = event.target.value;
        console.log('Filter changed to:', filterValue);
        this.loadLogs(0, filterValue);
    }

    // AUTO-REFRESH

    // MARK: startLogsRefresh
    // Starts automatic log refresh
    static startLogsRefresh(intervalMs = 30000) {
        this.stopLogsRefresh();
        
        this.logsRefreshInterval = setInterval(() => {
            this.refreshLogsIfActive();
        }, intervalMs);
    }

    // MARK: refreshLogsIfActive
    // Refreshes logs if the logs tab is active
    static refreshLogsIfActive() {
        const logsTab = document.getElementById('logs');
        const isActive = logsTab && logsTab.classList.contains('active');
        const hasToken = window.FinGuardConfig && window.FinGuardConfig.ADMIN_TOKEN;
        
        if (isActive && hasToken) {
            this.loadLogs(this.currentPage, this.currentLevel);
        }
    }

    // MARK: stopLogsRefresh
    // Stops automatic log refresh
    static stopLogsRefresh() {
        if (this.logsRefreshInterval) {
            clearInterval(this.logsRefreshInterval);
            this.logsRefreshInterval = null;
        }
    }

    // INITIALIZATION

    // MARK: initialize
    // Initializes the logs manager
    static initialize() {
        this.initializeFilter();
        this.startLogsRefresh();
        
        // Load logs when logs tab becomes active
        this.setupTabActivation();
    }

    // MARK: setupTabActivation
    // Sets up tab activation detection for logs
    static setupTabActivation() {
        // Listen for tab switches
        const tabButtons = document.querySelectorAll('.tab');
        tabButtons.forEach(button => {
            button.addEventListener('click', () => {
                if (button.textContent.trim().toLowerCase().includes('log')) {
                    // Small delay to ensure tab is active
                    setTimeout(() => {
                        this.loadLogs(0, this.currentLevel);
                    }, 100);
                }
            });
        });
    }
}

// AUTO-INITIALIZE WHEN DOM IS READY
document.addEventListener('DOMContentLoaded', function() {
    if (window.LogsManager) {
        window.LogsManager.initialize();
    }
});

// GLOBAL SCOPE EXPORT
window.LogsManager = LogsManager;