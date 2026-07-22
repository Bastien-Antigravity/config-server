class ConfigServerMFE extends HTMLElement {
    constructor() {
        super();
        this.baseUrl = '';
        this.statusData = { healthy: false, status: 'Offline', version: 'N/A', timestamp: 0, active_clients: 0, client_names: [] };
        this.configMap = {};
    }

    async connectedCallback() {
        this.baseUrl = this.getAttribute('base-url') || 'http://localhost:3308';
        this.renderSkeleton();
        await this.loadAll();
    }

    renderSkeleton() {
        this.innerHTML = `
            <style>
                .mfe-container {
                    font-family: var(--font-sans, 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif);
                    color: var(--color-text-primary, #333);
                    max-width: 1100px;
                    margin: 0 auto;
                    padding: 20px;
                }
                .mfe-header {
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    margin-bottom: 24px;
                }
                .mfe-title h1 {
                    font-family: var(--font-display, inherit);
                    font-size: var(--font-size-2xl, 2rem);
                    font-weight: 700;
                    margin: 0 0 5px 0;
                    color: var(--color-text-primary, #1a202c);
                }
                .mfe-title p {
                    color: var(--color-text-secondary, #718096);
                    margin: 0;
                    font-size: var(--font-size-sm, 0.95rem);
                }
                .mfe-btn-group {
                    display: flex;
                    gap: 10px;
                }
                .mfe-btn {
                    padding: 10px 18px;
                    border-radius: 8px;
                    font-weight: 600;
                    font-size: var(--font-size-sm, 0.9rem);
                    cursor: pointer;
                    transition: all 0.2s;
                    border: none;
                    display: inline-flex;
                    align-items: center;
                    gap: 6px;
                }
                .mfe-btn-primary {
                    background: linear-gradient(135deg, #11998e 0%, #38ef7d 100%);
                    color: white;
                }
                .mfe-btn-action {
                    background: linear-gradient(135deg, #36d1dc 0%, #5b86e5 100%);
                    color: white;
                }
                .mfe-btn-secondary {
                    background-color: var(--color-bg-surface, #edf2f7);
                    color: var(--color-text-primary, #4a5568);
                    border: 1px solid var(--color-bg-secondary, transparent);
                }
                .mfe-btn-secondary:hover {
                    background-color: var(--color-bg-secondary, #e2e8f0);
                }
                .mfe-btn:hover {
                    opacity: 0.95;
                    transform: translateY(-1px);
                }
                
                /* Cards layout */
                .mfe-row {
                    display: flex;
                    gap: 20px;
                    margin-bottom: 24px;
                    flex-wrap: wrap;
                }
                .mfe-col-4 { flex: 1; min-width: 250px; }
                .mfe-col-8 { flex: 2; min-width: 320px; }
                
                .mfe-card {
                    background: var(--color-bg-surface, white);
                    border-radius: 12px;
                    box-shadow: 0 4px 6px -1px rgba(0,0,0,0.1), 0 2px 4px -1px rgba(0,0,0,0.06);
                    padding: 20px;
                    border: 1px solid var(--color-bg-secondary, #edf2f7);
                }
                .mfe-card-title {
                    font-size: var(--font-size-xs, 0.75rem);
                    text-transform: uppercase;
                    color: var(--color-text-muted, #718096);
                    font-weight: 700;
                    margin-bottom: 12px;
                    letter-spacing: 0.05em;
                }
                .mfe-status-indicator {
                    display: flex;
                    align-items: center;
                    gap: 8px;
                    font-size: var(--font-size-lg, 1.25rem);
                    font-weight: 700;
                    margin-bottom: 10px;
                    color: var(--color-text-primary, inherit);
                }
                .mfe-dot {
                    height: 10px;
                    width: 10px;
                    border-radius: 50%;
                    display: inline-block;
                }
                .mfe-dot-active {
                    background-color: var(--color-accent-success, #48bb78);
                    box-shadow: 0 0 8px var(--color-accent-success, #48bb78);
                }
                .mfe-dot-inactive {
                    background-color: var(--color-accent-danger, #f56565);
                    box-shadow: 0 0 8px var(--color-accent-danger, #f56565);
                }
                
                .mfe-badge {
                    background-color: var(--color-bg-primary, #ebf8ff);
                    color: var(--color-accent-primary, #2b6cb0);
                    border: 1px solid var(--color-bg-secondary, #bee3f8);
                    padding: 4px 8px;
                    border-radius: 9999px;
                    font-size: var(--font-size-2xs, 0.75rem);
                    font-weight: 600;
                    font-family: var(--font-mono, monospace);
                    display: inline-block;
                    margin-right: 6px;
                    margin-bottom: 6px;
                }
                
                /* Config Accordion styling */
                .mfe-config-container {
                    background: var(--color-bg-surface, white);
                    border-radius: 12px;
                    border: 1px solid var(--color-bg-secondary, #edf2f7);
                    box-shadow: 0 4px 6px -1px rgba(0,0,0,0.1);
                    overflow: hidden;
                }
                .mfe-config-header {
                    background: linear-gradient(135deg, var(--color-bg-secondary, #1e3c72) 0%, var(--color-bg-surface, #2a5298) 100%);
                    color: var(--color-text-primary, white);
                    padding: 20px;
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    border-bottom: 1px solid var(--color-bg-secondary, transparent);
                }
                .mfe-config-header h2 {
                    font-family: var(--font-display, inherit);
                    font-size: var(--font-size-lg, 1.25rem);
                    margin: 0;
                    font-weight: 700;
                }
                
                .mfe-section {
                    border-bottom: 1px solid var(--color-bg-secondary, #edf2f7);
                }
                .mfe-section-header {
                    background: var(--color-bg-surface, #f7fafc);
                    color: var(--color-text-primary, inherit);
                    padding: 15px 20px;
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    cursor: pointer;
                    font-weight: 600;
                }
                .mfe-section-header:hover {
                    background: var(--color-bg-secondary, #edf2f7);
                }
                .mfe-section-content {
                    padding: 0;
                    display: block;
                }
                
                /* Table styling */
                .mfe-table {
                    width: 100%;
                    border-collapse: collapse;
                    text-align: left;
                }
                .mfe-table th {
                    background: var(--color-bg-secondary, #edf2f7);
                    color: var(--color-text-secondary, #4a5568);
                    padding: 10px 20px;
                    font-size: var(--font-size-xs, 0.85rem);
                    text-transform: uppercase;
                    font-weight: 700;
                    border-bottom: 2px solid var(--color-bg-primary, #cbd5e0);
                }
                .mfe-table td {
                    padding: 12px 20px;
                    border-bottom: 1px solid var(--color-bg-secondary, #edf2f7);
                    color: var(--color-text-primary, inherit);
                    font-size: var(--font-size-sm, 0.95rem);
                    vertical-align: middle;
                }
                .mfe-editable {
                    cursor: pointer;
                    padding: 4px 8px;
                    border-radius: 4px;
                    font-family: var(--font-mono, monospace);
                    background: var(--color-bg-primary, #f7fafc);
                    border: 1px solid var(--color-bg-secondary, #e2e8f0);
                    color: var(--color-text-primary, inherit);
                    word-break: break-all;
                }
                .mfe-editable:hover {
                    background: var(--color-bg-secondary, #edf2f7);
                    border-color: var(--color-bg-primary, #cbd5e0);
                }
                .mfe-mono {
                    font-family: var(--font-mono, monospace);
                    font-weight: 600;
                }
                .mfe-actions {
                    text-align: right;
                }
                .mfe-btn-circle {
                    width: 32px;
                    height: 32px;
                    border-radius: 50%;
                    display: inline-flex;
                    align-items: center;
                    justify-content: center;
                    border: 1px solid var(--color-bg-secondary, #e2e8f0);
                    background: var(--color-bg-surface, white);
                    cursor: pointer;
                    color: var(--color-text-primary, #4a5568);
                    transition: all 0.2s;
                }
                .mfe-btn-circle:hover {
                    background: var(--color-bg-secondary, #edf2f7);
                    color: var(--color-accent-primary, #2b6cb0);
                    border-color: var(--color-bg-primary, #cbd5e0);
                }
                
                /* Custom Vanilla CSS Modal */
                .mfe-modal {
                    position: fixed;
                    top: 0;
                    left: 0;
                    width: 100%;
                    height: 100%;
                    background: rgba(0,0,0,0.5);
                    display: flex;
                    align-items: center;
                    justify-content: center;
                    z-index: 9999;
                    opacity: 0;
                    pointer-events: none;
                    transition: opacity 0.2s;
                }
                .mfe-modal.show {
                    opacity: 1;
                    pointer-events: auto;
                }
                .mfe-modal-content {
                    background: var(--color-bg-surface, white);
                    color: var(--color-text-primary, inherit);
                    border: 1px solid var(--color-bg-secondary, #edf2f7);
                    border-radius: 12px;
                    width: 90%;
                    max-width: 500px;
                    box-shadow: 0 20px 25px -5px rgba(0,0,0,0.1);
                    overflow: hidden;
                    transform: translateY(-20px);
                    transition: transform 0.2s;
                }
                .mfe-modal.show .mfe-modal-content {
                    transform: translateY(0);
                }
                .mfe-modal-header {
                    padding: 16px 20px;
                    color: white;
                    font-weight: 700;
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                }
                .mfe-modal-header-add { background: var(--color-accent-primary, #3182ce); }
                .mfe-modal-header-edit { background: var(--color-bg-secondary, #2d3748); }
                
                .mfe-modal-body {
                    padding: 20px;
                }
                .mfe-modal-footer {
                    padding: 16px 20px;
                    background: var(--color-bg-primary, #f7fafc);
                    display: flex;
                    justify-content: flex-end;
                    gap: 10px;
                    border-top: 1px solid var(--color-bg-secondary, #e2e8f0);
                }
                .mfe-form-group {
                    margin-bottom: 16px;
                }
                .mfe-form-group label {
                    display: block;
                    font-weight: 600;
                    font-size: var(--font-size-sm, 0.9rem);
                    margin-bottom: 6px;
                    color: var(--color-text-secondary, #4a5568);
                }
                .mfe-form-input {
                    width: 100%;
                    padding: 10px 12px;
                    border: 1px solid var(--color-bg-secondary, #cbd5e0);
                    background: var(--color-bg-primary, white);
                    color: var(--color-text-primary, inherit);
                    border-radius: 6px;
                    font-family: inherit;
                    font-size: var(--font-size-sm, 0.9rem);
                    box-sizing: border-box;
                }
                .mfe-form-input:focus {
                    outline: none;
                    border-color: var(--color-accent-primary, #3182ce);
                    box-shadow: 0 0 0 3px rgba(49, 130, 206, 0.15);
                }
                .mfe-close-btn {
                    cursor: pointer;
                    font-size: 1.5rem;
                    line-height: 1;
                }
            </style>
            
            <div class="mfe-container">
                <!-- Header -->
                <div class="mfe-header">
                    <div class="mfe-title">
                        <h1>⚙️ Configuration Server Manager</h1>
                        <p>OpenMFE dynamic configuration explorer and live administration dashboard</p>
                    </div>
                    <div class="mfe-btn-group">
                        <button class="mfe-btn mfe-btn-action" id="mfe-reload-btn">
                            <i class="fa fa-refresh"></i> Reload Baseline
                        </button>
                        <button class="mfe-btn mfe-btn-primary" id="mfe-persist-btn">
                            <i class="fa fa-save"></i> Persist State
                        </button>
                    </div>
                </div>
                
                <!-- Status Row -->
                <div class="mfe-row">
                    <div class="mfe-col-4">
                        <div class="mfe-card">
                            <div class="mfe-card-title">Service Status</div>
                            <div class="mfe-status-indicator">
                                <span class="mfe-dot mfe-dot-inactive" id="mfe-status-dot"></span>
                                <span id="mfe-status-text">Offline</span>
                            </div>
                            <div style="color:#718096; font-size:0.85rem;" id="mfe-status-details">
                                Loading status...
                            </div>
                        </div>
                    </div>
                    <div class="mfe-col-8">
                        <div class="mfe-card" style="height:100%; box-sizing:border-box;">
                            <div class="mfe-card-title">Active Fleet Connections</div>
                            <div id="mfe-clients-container" style="max-height:80px; overflow-y:auto;">
                                Loading connections...
                            </div>
                        </div>
                    </div>
                </div>
                
                <!-- Configurations Accordion -->
                <div class="mfe-config-container">
                    <div class="mfe-config-header">
                        <h2><i class="fa fa-sliders"></i> Configuration Variables</h2>
                        <button class="mfe-btn mfe-btn-secondary" style="padding: 6px 12px; font-size:0.8rem;" id="mfe-add-entry-btn">
                            <i class="fa fa-plus text-primary"></i> Add Entry
                        </button>
                    </div>
                    <div id="mfe-accordion-list">
                        <div style="padding:40px; text-align:center; color:#718096;">
                            <i class="fa fa-spinner fa-spin fa-2x"></i><br>Loading configuration parameters...
                        </div>
                    </div>
                </div>
            </div>

            <!-- Add Modal -->
            <div class="mfe-modal" id="mfe-add-modal">
                <div class="mfe-modal-content">
                    <div class="mfe-modal-header mfe-modal-header-add">
                        <span>Create Configuration Variable</span>
                        <span class="mfe-close-btn" onclick="document.getElementById('mfe-add-modal').classList.remove('show')">&times;</span>
                    </div>
                    <form id="mfe-add-form">
                        <div class="mfe-modal-body">
                            <div class="mfe-form-group">
                                <label>Section Name</label>
                                <input type="text" id="add-section" class="mfe-form-input" required placeholder="e.g. timescale_db">
                            </div>
                            <div class="mfe-form-group">
                                <label>Key Name</label>
                                <input type="text" id="add-key" class="mfe-form-input" required placeholder="e.g. max_connections">
                            </div>
                            <div class="mfe-form-group">
                                <label>Value</label>
                                <textarea id="add-value" class="mfe-form-input" required rows="3" placeholder="Enter configuration value..."></textarea>
                            </div>
                        </div>
                        <div class="mfe-modal-footer">
                            <button type="button" class="mfe-btn mfe-btn-secondary" onclick="document.getElementById('mfe-add-modal').classList.remove('show')">Cancel</button>
                            <button type="submit" class="mfe-btn mfe-btn-primary">Save Parameter</button>
                        </div>
                    </form>
                </div>
            </div>

            <!-- Edit Modal -->
            <div class="mfe-modal" id="mfe-edit-modal">
                <div class="mfe-modal-content">
                    <div class="mfe-modal-header mfe-modal-header-edit">
                        <span>Edit Parameter</span>
                        <span class="mfe-close-btn" onclick="document.getElementById('mfe-edit-modal').classList.remove('show')">&times;</span>
                    </div>
                    <form id="mfe-edit-form">
                        <div class="mfe-modal-body">
                            <input type="hidden" id="edit-section">
                            <input type="hidden" id="edit-key">
                            <div style="margin-bottom: 12px; font-size: 0.85rem; color: #4a5568;">
                                <strong>Target Variable:</strong> <span id="edit-identifier" class="mfe-mono">section -> key</span>
                            </div>
                            <div class="mfe-form-group">
                                <label>Value</label>
                                <textarea id="edit-value" class="mfe-form-input" required rows="4"></textarea>
                            </div>
                        </div>
                        <div class="mfe-modal-footer">
                            <button type="button" class="mfe-btn mfe-btn-secondary" onclick="document.getElementById('mfe-edit-modal').classList.remove('show')">Cancel</button>
                            <button type="submit" class="mfe-btn mfe-btn-primary">Update Parameter</button>
                        </div>
                    </form>
                </div>
            </div>
        `;

        this.bindEvents();
    }

    bindEvents() {
        this.querySelector('#mfe-reload-btn').addEventListener('click', () => this.reloadBaseline());
        this.querySelector('#mfe-persist-btn').addEventListener('click', () => this.persistState());
        this.querySelector('#mfe-add-entry-btn').addEventListener('click', () => {
            this.querySelector('#mfe-add-form').reset();
            this.querySelector('#mfe-add-modal').classList.add('show');
        });

        this.querySelector('#mfe-add-form').addEventListener('submit', (e) => {
            e.preventDefault();
            this.submitConfig(
                this.querySelector('#add-section').value,
                this.querySelector('#add-key').value,
                this.querySelector('#add-value').value
            );
            this.querySelector('#mfe-add-modal').classList.remove('show');
        });

        this.querySelector('#mfe-edit-form').addEventListener('submit', (e) => {
            e.preventDefault();
            this.submitConfig(
                this.querySelector('#edit-section').value,
                this.querySelector('#edit-key').value,
                this.querySelector('#edit-value').value
            );
            this.querySelector('#mfe-edit-modal').classList.remove('show');
        });
    }

    async loadAll() {
        await Promise.all([this.loadStatus(), this.loadConfigs()]);
    }

    async loadStatus() {
        try {
            const res = await fetch(`${this.baseUrl}/api/v1/status`);
            if (res.ok) {
                this.statusData = await res.json();
                this.displayStatus();
            } else {
                throw new Error('Unreachable status');
            }
        } catch (err) {
            console.error('Failed to load status:', err);
            this.statusData.healthy = false;
            this.statusData.status = 'Error';
            this.displayStatus();
        }
    }

    displayStatus() {
        const dot = this.querySelector('#mfe-status-dot');
        const text = this.querySelector('#mfe-status-text');
        const details = this.querySelector('#mfe-status-details');
        const clientsContainer = this.querySelector('#mfe-clients-container');

        if (dot && text) {
            if (this.statusData.healthy) {
                dot.className = 'mfe-dot mfe-dot-active';
                text.innerText = ' Running';
            } else {
                dot.className = 'mfe-dot mfe-dot-inactive';
                text.innerText = ` ${this.statusData.status}`;
            }
        }

        const date = new Date(this.statusData.timestamp * 1000).toLocaleString();
        details.innerHTML = `
            <div><strong>Version:</strong> ${this.escapeHTML(this.statusData.version || 'N/A')}</div>
            <div><strong>Last Check:</strong> ${date}</div>
        `;

        if (this.statusData.client_names && this.statusData.client_names.length > 0) {
            clientsContainer.innerHTML = this.statusData.client_names.map(c => `
                <span class="mfe-badge"><i class="fa fa-plug"></i> ${this.escapeHTML(c)}</span>
            `).join('');
        } else {
            clientsContainer.innerHTML = `<p style="color:#718096; font-size:0.85rem; margin:0;">No active client heartbeat subscriptions detected.</p>`;
        }
    }

    async loadConfigs() {
        try {
            const res = await fetch(`${this.baseUrl}/api/v1/config/list`);
            if (res.ok) {
                const data = await res.json();
                this.configMap = JSON.parse(data.json_config || '{}');
                this.displayConfigs();
            } else {
                throw new Error('Unreachable list');
            }
        } catch (err) {
            console.error('Failed to load configs:', err);
            this.querySelector('#mfe-accordion-list').innerHTML = `
                <div style="padding:40px; text-align:center; color:#e53e3e;">
                    <i class="fa fa-exclamation-triangle fa-2x"></i><br>Failed to retrieve config parameters from config-server: ${this.escapeHTML(err.message)}
                </div>
            `;
        }
    }

    stringifyValue(value) {
        if (value === null || value === undefined) {
            return '';
        }
        if (typeof value === 'object') {
            try {
                return JSON.stringify(value);
            } catch (err) {
                return String(value);
            }
        }
        return String(value);
    }

    escapeHTML(value) {
        return this.stringifyValue(value).replace(/[&<>"']/g, char => ({
            '&': '&amp;',
            '<': '&lt;',
            '>': '&gt;',
            '"': '&quot;',
            "'": '&#39;'
        }[char]));
    }

    escapeAttribute(value) {
        return this.escapeHTML(value);
    }

    displayConfigs() {
        const container = this.querySelector('#mfe-accordion-list');
        container.innerHTML = '';

        const sections = Object.keys(this.configMap);
        if (sections.length === 0) {
            container.innerHTML = `
                <div style="padding:40px; text-align:center; color:#718096;">
                    <i class="fa fa-cogs fa-2x"></i><br>No baseline or dynamic configuration settings loaded.
                </div>
            `;
            return;
        }

        sections.forEach((section, index) => {
            const rawSettings = this.configMap[section];
            const settings = rawSettings && typeof rawSettings === 'object' ? rawSettings : {};
            const sectionDiv = document.createElement('div');
            sectionDiv.className = 'mfe-section';

            const keys = Object.keys(settings);
            const contentId = `mfe-collapse-${index}`;
            const sectionLabel = this.escapeHTML(section);
            
            sectionDiv.innerHTML = `
                <div class="mfe-section-header" data-target="${contentId}">
                    <span><i class="fa fa-folder-open-o" style="color:#3182ce; margin-right:8px;"></i> ${sectionLabel} <span style="font-size:0.75rem; background:#cbd5e0; color:#4a5568; padding:2px 6px; border-radius:10px; margin-left:8px;">${keys.length} keys</span></span>
                    <i class="fa fa-chevron-down" style="color:#718096; font-size:0.75rem;"></i>
                </div>
                <div class="mfe-section-content" id="${contentId}">
                    <div style="overflow-x:auto;">
                        <table class="mfe-table">
                            <thead>
                                <tr>
                                    <th style="width:30%;">Key</th>
                                    <th style="width:55%;">Value</th>
                                    <th style="width:15%; text-align:right;">Actions</th>
                                </tr>
                            </thead>
                            <tbody>
                                ${keys.map(key => {
                                    const value = this.stringifyValue(settings[key]);
                                    return `
                                    <tr>
                                        <td class="mfe-mono">${this.escapeHTML(key)}</td>
                                        <td>
                                            <div class="mfe-editable" data-section="${this.escapeAttribute(section)}" data-key="${this.escapeAttribute(key)}">${this.escapeHTML(value)}</div>
                                        </td>
                                        <td class="mfe-actions">
                                            <button class="mfe-btn-circle mfe-edit-btn" data-section="${this.escapeAttribute(section)}" data-key="${this.escapeAttribute(key)}" data-val="${this.escapeAttribute(value)}" title="Edit parameter">
                                                <i class="fa fa-pencil"></i>
                                            </button>
                                        </td>
                                    </tr>
                                `}).join('')}
                            </tbody>
                        </table>
                    </div>
                </div>
            `;

            container.appendChild(sectionDiv);
        });

        // Bind accordion collapses
        this.querySelectorAll('.mfe-section-header').forEach(header => {
            header.addEventListener('click', () => {
                const targetId = header.dataset.target;
                const content = this.querySelector(`#${targetId}`);
                if (content.style.display === 'none') {
                    content.style.display = 'block';
                    header.querySelector('.fa-chevron-down').style.transform = 'rotate(0deg)';
                } else {
                    content.style.display = 'none';
                    header.querySelector('.fa-chevron-down').style.transform = 'rotate(-90deg)';
                }
            });
        });

        // Bind inline edit clicks and pencil buttons
        const triggerEdit = (section, key, val) => {
            this.querySelector('#edit-section').value = section;
            this.querySelector('#edit-key').value = key;
            this.querySelector('#edit-value').value = val;
            this.querySelector('#edit-identifier').innerText = `${section} -> ${key}`;
            this.querySelector('#mfe-edit-modal').classList.add('show');
        };

        this.querySelectorAll('.mfe-editable').forEach(el => {
            el.addEventListener('click', () => triggerEdit(el.dataset.section, el.dataset.key, el.innerText));
        });

        this.querySelectorAll('.mfe-edit-btn').forEach(btn => {
            btn.addEventListener('click', () => triggerEdit(btn.dataset.section, btn.dataset.key, btn.dataset.val));
        });
    }

    async submitConfig(section, key, value) {
        try {
            const res = await fetch(`${this.baseUrl}/api/v1/config/set`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ section, key, value })
            });

            if (res.ok) {
                await this.loadAll();
            } else {
                const text = await res.text();
                alert(`Error saving configuration: ${text}`);
            }
        } catch (err) {
            alert(`Failed to save configuration: ${err.message}`);
        }
    }

    async reloadBaseline() {
        if (!confirm("Are you sure you want to reload all baseline configurations from YAML files? This will reset all unsaved memory overrides!")) {
            return;
        }
        try {
            const res = await fetch(`${this.baseUrl}/api/v1/config/reload`, { method: 'POST' });
            if (res.ok) {
                alert("Configurations reloaded from baseline successfully.");
                await this.loadAll();
            } else {
                alert("Reload failed.");
            }
        } catch (err) {
            alert(`Reload baseline failed: ${err.message}`);
        }
    }

    async persistState() {
        try {
            const res = await fetch(`${this.baseUrl}/api/v1/config/persist`, { method: 'POST' });
            if (res.ok) {
                alert("Configuration changes persisted to disk storage successfully.");
                await this.loadAll();
            } else {
                alert("Persist failed.");
            }
        } catch (err) {
            alert(`Persist configuration failed: ${err.message}`);
        }
    }
}

customElements.define('config-server-mfe', ConfigServerMFE);
