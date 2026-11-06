// =============================================================================
// ESSENTIAL PROCESS:
// OpenMFE web component for config-server, encapsulating dynamic configuration
// inspection, live updates, baseline reloading, and state persistence.
//
// DATA FLOW:
// 1. Input: Configuration state fetched via HTTP from /api/v1/config/ endpoints.
// 2. Logic: Renders responsive dashboard, interactive tables, and update modals.
// 3. Output: Dispatches HTTP REST mutations to config-server daemon.
//
// KEY PARAMETERS:
// - ConfigServerMFE: Standard OpenMFE Web Component registered as config-server-mfe.
// - Complies strictly with Bastien UI tokens and openmfe-components.css (Light DOM).
// =============================================================================

(function () {
    'use strict';

const BaseElementClass = (typeof window !== 'undefined' && window.BastienMFEElement)
    ? window.BastienMFEElement
    : class extends (typeof HTMLElement !== 'undefined' ? HTMLElement : class {}) {
        constructor() {
            super();
            this.baseUrl = '';
            this.pollTimers = [];
            this.dom = {};
            this.state = 'IDLE';
        }
        connectedCallback() {
            let origin = this.getAttribute('base-url') || (typeof window !== 'undefined' && window.location ? window.location.origin : '');
            if (origin.endsWith('/')) origin = origin.slice(0, -1);
            this.baseUrl = origin;
            this.render();
            this.cacheDom();
            this.bindEvents();
            this.onMount();
        }
        disconnectedCallback() {
            this.clearPollTimers();
            this.onUnmount();
        }
        render() {}
        cacheDom() {}
        bindEvents() {}
        onMount() {}
        onUnmount() {}
        registerPoll(cb, ms = 5000, imm = true) {
            if (imm) { try { cb(); } catch (e) { console.error(e); } }
            if (!this.pollTimers) this.pollTimers = [];
            const t = setInterval(cb, ms);
            this.pollTimers.push(t);
            return t;
        }
        clearPollTimers() {
            if (this.pollTimers) this.pollTimers.forEach(t => clearInterval(t));
            this.pollTimers = [];
        }
    };

class ConfigServerMFE extends BaseElementClass {
    constructor() {
        super();
        this.statusData = { healthy: false, status: 'Offline', version: 'N/A', timestamp: 0, active_clients: 0, client_names: [] };
        this.configMap = {};
    }

    render() {
        this.innerHTML = `
            <div class="openmfe-container">
                <!-- Standard Toolbar -->
                <div class="openmfe-toolbar">
                    <div class="openmfe-toolbar__title">
                        <i class="fa fa-sliders" style="color: var(--color-accent-primary, #38bdf8);"></i>
                        <span>Configuration Manager</span>
                        <span class="openmfe-badge" id="mfe-status-badge">
                            <span class="openmfe-dot openmfe-dot--inactive" id="mfe-status-dot"></span>
                            <span id="mfe-status-text">Offline</span>
                        </span>
                    </div>
                    <div class="openmfe-toolbar__actions">
                        <button class="openmfe-btn openmfe-btn--sm openmfe-btn--outline" id="mfe-reload-btn" title="Reload baseline YAML configurations from disk">
                            <i class="fa fa-refresh"></i> Reload Baseline
                        </button>
                        <button class="openmfe-btn openmfe-btn--sm openmfe-btn--primary" id="mfe-persist-btn" title="Persist active configurations to disk">
                            <i class="fa fa-save"></i> Persist State
                        </button>
                        <button class="openmfe-btn openmfe-btn--sm openmfe-btn--outline" id="mfe-add-entry-btn">
                            <i class="fa fa-plus"></i> Add Entry
                        </button>
                    </div>
                </div>
                
                <!-- Status Row -->
                <div class="openmfe-kpi-grid">
                    <!-- Service Status Card -->
                    <div class="openmfe-card">
                        <div class="openmfe-card__header">
                            <h4 class="openmfe-card__title"><i class="fa fa-heartbeat"></i> Service Status</h4>
                        </div>
                        <div class="openmfe-card__body">
                            <div style="display:flex; align-items:center; gap:8px; font-size:1.1rem; font-weight:700; margin-bottom:8px;">
                                <span class="openmfe-dot openmfe-dot--inactive" id="mfe-status-dot-inner"></span>
                                <span id="mfe-status-text-inner">Offline</span>
                            </div>
                            <div style="color:var(--color-text-muted, #94a3b8); font-size:0.8125rem; line-height:1.6;" id="mfe-status-details">
                                Loading status...
                            </div>
                        </div>
                    </div>

                    <!-- Active Fleet Connections Card -->
                    <div class="openmfe-card">
                        <div class="openmfe-card__header">
                            <h4 class="openmfe-card__title"><i class="fa fa-plug"></i> Active Fleet Subscriptions</h4>
                        </div>
                        <div class="openmfe-card__body">
                            <div id="mfe-clients-container" style="display:flex; flex-wrap:wrap; gap:6px; max-height:100px; overflow-y:auto;">
                                Loading connections...
                            </div>
                        </div>
                    </div>
                </div>
                
                <!-- Configurations Accordion -->
                <div class="openmfe-card" style="margin-top: var(--space-4, 16px);">
                    <div class="openmfe-card__header">
                        <h4 class="openmfe-card__title"><i class="fa fa-cogs"></i> Configuration Parameters</h4>
                    </div>
                    <div class="openmfe-card__body" style="padding: 0;">
                        <div id="mfe-accordion-list">
                            <div style="padding:40px; text-align:center; color:var(--color-text-muted, #94a3b8);">
                                <i class="fa fa-spinner fa-spin fa-2x"></i><br><br>Loading configuration parameters...
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <!-- Add Modal -->
            <div class="openmfe-modal" id="mfe-add-modal">
                <div class="openmfe-modal-content">
                    <div class="openmfe-modal-header">
                        <span><i class="fa fa-plus-circle" style="color:var(--color-accent-primary, #38bdf8); margin-right:6px;"></i> Create Configuration Parameter</span>
                        <span class="mfe-close-add-modal" style="cursor:pointer; font-size:1.25rem;">&times;</span>
                    </div>
                    <form id="mfe-add-form">
                        <div class="openmfe-modal-body">
                            <div class="openmfe-form-group">
                                <label class="openmfe-form-label">Section Name</label>
                                <input type="text" id="add-section" class="openmfe-form-input" required placeholder="e.g. timescale_db">
                            </div>
                            <div class="openmfe-form-group">
                                <label class="openmfe-form-label">Key Name</label>
                                <input type="text" id="add-key" class="openmfe-form-input" required placeholder="e.g. max_connections">
                            </div>
                            <div class="openmfe-form-group">
                                <label class="openmfe-form-label">Value</label>
                                <textarea id="add-value" class="openmfe-form-input" required rows="3" placeholder="Enter configuration value..."></textarea>
                            </div>
                        </div>
                        <div class="openmfe-modal-footer">
                            <button type="button" class="openmfe-btn openmfe-btn--sm openmfe-btn--outline mfe-close-add-modal">Cancel</button>
                            <button type="submit" class="openmfe-btn openmfe-btn--sm openmfe-btn--primary">Save Parameter</button>
                        </div>
                    </form>
                </div>
            </div>

            <!-- Edit Modal -->
            <div class="openmfe-modal" id="mfe-edit-modal">
                <div class="openmfe-modal-content">
                    <div class="openmfe-modal-header">
                        <span><i class="fa fa-pencil-square" style="color:var(--color-accent-primary, #38bdf8); margin-right:6px;"></i> Edit Parameter</span>
                        <span class="mfe-close-edit-modal" style="cursor:pointer; font-size:1.25rem;">&times;</span>
                    </div>
                    <form id="mfe-edit-form">
                        <div class="openmfe-modal-body">
                            <input type="hidden" id="edit-section">
                            <input type="hidden" id="edit-key">
                            <div style="margin-bottom: 12px; font-size: 0.8125rem; color: var(--color-text-muted, #94a3b8);">
                                <strong>Target Variable:</strong> <span id="edit-identifier" style="font-family:var(--font-mono, monospace); color:var(--color-accent-primary, #38bdf8);">section -> key</span>
                            </div>
                            <div id="edit-secret-notice" style="display:none; margin-bottom:12px; padding:8px 12px; background:rgba(229,62,62,0.1); border:1px solid rgba(229,62,62,0.25); border-radius:6px; color:#fb7185; font-size:0.8rem;">
                                <i class="fa fa-shield"></i> <strong>Zero-Knowledge Secret:</strong> This value is an encrypted secret (<code>ENC(...)</code>). Secrets are never decrypted on the Config Server or Web Interface, and are decrypted only in-memory by the authorized consumer microservice.
                            </div>
                            <div class="openmfe-form-group">
                                <label class="openmfe-form-label">Value</label>
                                <textarea id="edit-value" class="openmfe-form-input" required rows="4"></textarea>
                            </div>
                        </div>
                        <div class="openmfe-modal-footer">
                            <button type="button" class="openmfe-btn openmfe-btn--sm openmfe-btn--outline mfe-close-edit-modal">Cancel</button>
                            <button type="submit" class="openmfe-btn openmfe-btn--sm openmfe-btn--primary">Update Parameter</button>
                        </div>
                    </form>
                </div>
            </div>
        `;
    }

    cacheDom() {
        this.dom = {
            statusDot: this.querySelector('#mfe-status-dot'),
            statusDotInner: this.querySelector('#mfe-status-dot-inner'),
            statusText: this.querySelector('#mfe-status-text'),
            statusTextInner: this.querySelector('#mfe-status-text-inner'),
            statusDetails: this.querySelector('#mfe-status-details'),
            clientsContainer: this.querySelector('#mfe-clients-container'),
            accordionList: this.querySelector('#mfe-accordion-list'),
            btnReload: this.querySelector('#mfe-reload-btn'),
            btnPersist: this.querySelector('#mfe-persist-btn'),
            btnAddEntry: this.querySelector('#mfe-add-entry-btn'),
            addModal: this.querySelector('#mfe-add-modal'),
            addForm: this.querySelector('#mfe-add-form'),
            editModal: this.querySelector('#mfe-edit-modal'),
            editForm: this.querySelector('#mfe-edit-form'),
            editSection: this.querySelector('#edit-section'),
            editKey: this.querySelector('#edit-key'),
            editValue: this.querySelector('#edit-value'),
            editIdentifier: this.querySelector('#edit-identifier'),
            editSecretNotice: this.querySelector('#edit-secret-notice')
        };
    }

    bindEvents() {
        this.querySelectorAll('.mfe-close-add-modal').forEach(el => {
            el.addEventListener('click', () => this.dom.addModal?.classList.remove('show'));
        });
        this.querySelectorAll('.mfe-close-edit-modal').forEach(el => {
            el.addEventListener('click', () => this.dom.editModal?.classList.remove('show'));
        });
        this.dom.btnReload?.addEventListener('click', () => this.reloadBaseline());
        this.dom.btnPersist?.addEventListener('click', () => this.persistState());
        this.dom.btnAddEntry?.addEventListener('click', () => {
            this.dom.addForm?.reset();
            this.dom.addModal?.classList.add('show');
        });

        this.dom.addForm?.addEventListener('submit', (e) => {
            e.preventDefault();
            const sec = this.querySelector('#add-section')?.value;
            const key = this.querySelector('#add-key')?.value;
            const val = this.querySelector('#add-value')?.value;
            this.submitConfig(sec, key, val);
            this.dom.addModal?.classList.remove('show');
        });

        this.dom.editForm?.addEventListener('submit', (e) => {
            e.preventDefault();
            this.submitConfig(
                this.dom.editSection?.value,
                this.dom.editKey?.value,
                this.dom.editValue?.value
            );
            this.dom.editModal?.classList.remove('show');
        });
    }

    async onMount() {
        await this.loadAll();
        this.registerPoll(() => this.loadStatus(), 5000, false);
    }

    async loadAll() {
        await Promise.all([this.loadStatus(), this.loadConfigs()]);
    }

    async loadStatus() {
        try {
            let url = `${this.baseUrl}/api/v1/status`;
            let res = await fetch(url).catch(() => null);
            if ((!res || !res.ok) && typeof window !== 'undefined' && this.baseUrl !== window.location.origin) {
                res = await fetch(`${window.location.origin}/api/v1/status`).catch(() => null);
            }
            if (res && res.ok) {
                this.statusData = await res.json();
                this.displayStatus();
            } else {
                throw new Error('Unreachable status');
            }
        } catch (err) {
            this.statusData.healthy = false;
            this.statusData.status = 'Error';
            this.displayStatus();
        }
    }

    displayStatus() {
        const isOnline = this.statusData.healthy;
        const statusLabel = isOnline ? 'Running' : ` ${this.statusData.status || 'Offline'}`;
        const dotClass = `openmfe-dot ${isOnline ? 'openmfe-dot--active' : 'openmfe-dot--inactive'}`;

        if (this.dom.statusDot) this.dom.statusDot.className = dotClass;
        if (this.dom.statusDotInner) this.dom.statusDotInner.className = dotClass;
        if (this.dom.statusText) this.dom.statusText.innerText = statusLabel;
        if (this.dom.statusTextInner) this.dom.statusTextInner.innerText = statusLabel;

        const date = this.statusData.timestamp ? new Date(this.statusData.timestamp * 1000).toLocaleString() : '—';
        if (this.dom.statusDetails) {
            this.dom.statusDetails.innerHTML = `
                <div><strong>Version:</strong> ${this.escapeHTML(this.statusData.version || 'N/A')}</div>
                <div><strong>Last Check:</strong> ${date}</div>
            `;
        }

        if (this.dom.clientsContainer) {
            if (this.statusData.client_names && this.statusData.client_names.length > 0) {
                this.dom.clientsContainer.innerHTML = this.statusData.client_names.map(c => `
                    <span class="openmfe-badge openmfe-badge--info"><i class="fa fa-plug"></i> ${this.escapeHTML(c)}</span>
                `).join('');
            } else {
                this.dom.clientsContainer.innerHTML = `<p style="color:var(--color-text-muted, #94a3b8); font-size:0.8125rem; margin:0;">No active client heartbeat subscriptions detected.</p>`;
            }
        }
    }

    async loadConfigs() {
        try {
            let url = `${this.baseUrl}/api/v1/config/list`;
            let res = await fetch(url).catch(() => null);
            if ((!res || !res.ok) && typeof window !== 'undefined' && this.baseUrl !== window.location.origin) {
                res = await fetch(`${window.location.origin}/api/v1/config/list`).catch(() => null);
            }
            if (res && res.ok) {
                const data = await res.json();
                this.configMap = JSON.parse(data.json_config || '{}');
                this.displayConfigs();
            } else {
                throw new Error('Unreachable list');
            }
        } catch (err) {
            if (this.dom.accordionList) {
                this.dom.accordionList.innerHTML = `
                    <div style="padding:40px; text-align:center; color:var(--color-danger, #fb7185);">
                        <i class="fa fa-exclamation-triangle fa-2x"></i><br><br>Failed to retrieve config parameters from config-server: ${this.escapeHTML(err.message)}
                    </div>
                `;
            }
        }
    }

    stringifyValue(value) {
        if (value === null || value === undefined) return '';
        if (typeof value === 'object') {
            try { return JSON.stringify(value); } catch (err) { return String(value); }
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
        if (!this.dom.accordionList) return;
        this.dom.accordionList.innerHTML = '';

        const sections = Object.keys(this.configMap);
        if (sections.length === 0) {
            this.dom.accordionList.innerHTML = `
                <div style="padding:40px; text-align:center; color:var(--color-text-muted, #94a3b8);">
                    <i class="fa fa-cogs fa-2x"></i><br><br>No baseline or dynamic configuration settings loaded.
                </div>
            `;
            return;
        }

        sections.forEach((section, index) => {
            const rawSettings = this.configMap[section];
            const settings = rawSettings && typeof rawSettings === 'object' ? rawSettings : {};
            const sectionDiv = document.createElement('div');
            sectionDiv.style.borderBottom = '1px solid var(--color-border-subtle, rgba(255, 255, 255, 0.08))';

            const keys = Object.keys(settings);
            const contentId = `mfe-collapse-${index}`;
            const sectionLabel = this.escapeHTML(section);
            
            sectionDiv.innerHTML = `
                <div class="mfe-section-header" data-target="${contentId}" style="background:var(--color-bg-overlay, rgba(15,23,42,0.3)); color:var(--color-text-primary, #f8fafc); padding:12px 16px; display:flex; justify-content:space-between; align-items:center; cursor:pointer; font-weight:600; font-size:0.875rem;">
                    <span><i class="fa fa-folder-open-o" style="color:var(--color-accent-primary, #38bdf8); margin-right:8px;"></i> ${sectionLabel} <span style="font-size:0.75rem; background:rgba(255,255,255,0.08); color:var(--color-text-muted, #94a3b8); padding:2px 8px; border-radius:10px; margin-left:8px;">${keys.length} keys</span></span>
                    <i class="fa fa-chevron-down" style="color:var(--color-text-muted, #94a3b8); font-size:0.75rem; transition:transform 0.2s;"></i>
                </div>
                <div class="mfe-section-content" id="${contentId}" style="display:block;">
                    <div style="overflow-x:auto;">
                        <table class="openmfe-table">
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
                                    const isSecret = value.startsWith('ENC(') && value.endsWith(')');
                                    let displayContent = this.escapeHTML(value);
                                    if (isSecret) {
                                        const masked = value.length > 30 ? value.substring(0, 14) + '...' + value.substring(value.length - 8) : value;
                                        displayContent = `<span class="openmfe-badge openmfe-badge--error"><i class="fa fa-lock"></i> Encrypted Secret</span><span style="color:var(--color-text-muted, #94a3b8); font-size:0.8rem; font-family:var(--font-mono, monospace);">${this.escapeHTML(masked)}</span>`;
                                    }
                                    return `
                                    <tr>
                                        <td style="font-family:var(--font-mono, monospace); font-weight:600; color:var(--color-text-primary, #f8fafc);">${this.escapeHTML(key)}</td>
                                        <td>
                                            <div class="mfe-editable" data-section="${this.escapeAttribute(section)}" data-key="${this.escapeAttribute(key)}" data-val="${this.escapeAttribute(value)}" style="cursor:pointer; padding:4px 8px; border-radius:4px; font-family:var(--font-mono, monospace); background:rgba(15,23,42,0.4); border:1px solid var(--color-border-subtle, rgba(255,255,255,0.06)); word-break:break-all;">${displayContent}</div>
                                        </td>
                                        <td style="text-align:right;">
                                            <button class="openmfe-btn openmfe-btn--sm openmfe-btn--outline mfe-edit-btn" data-section="${this.escapeAttribute(section)}" data-key="${this.escapeAttribute(key)}" data-val="${this.escapeAttribute(value)}" title="Edit parameter">
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

            this.dom.accordionList.appendChild(sectionDiv);
        });

        // Bind accordion collapses
        this.dom.accordionList.querySelectorAll('.mfe-section-header').forEach(header => {
            header.addEventListener('click', () => {
                const targetId = header.dataset.target;
                const content = this.querySelector(`#${targetId}`);
                const icon = header.querySelector('.fa-chevron-down');
                if (content.style.display === 'none') {
                    content.style.display = 'block';
                    if (icon) icon.style.transform = 'rotate(0deg)';
                } else {
                    content.style.display = 'none';
                    if (icon) icon.style.transform = 'rotate(-90deg)';
                }
            });
        });

        // Bind inline edit clicks and pencil buttons
        const triggerEdit = (section, key, val) => {
            if (this.dom.editSection) this.dom.editSection.value = section;
            if (this.dom.editKey) this.dom.editKey.value = key;
            if (this.dom.editValue) this.dom.editValue.value = val;
            if (this.dom.editIdentifier) this.dom.editIdentifier.innerText = `${section} -> ${key}`;
            
            if (this.dom.editSecretNotice) {
                if (val && val.startsWith('ENC(') && val.endsWith(')')) {
                    this.dom.editSecretNotice.style.display = 'block';
                } else {
                    this.dom.editSecretNotice.style.display = 'none';
                }
            }

            this.dom.editModal?.classList.add('show');
        };

        this.dom.accordionList.querySelectorAll('.mfe-editable').forEach(el => {
            el.addEventListener('click', () => triggerEdit(el.dataset.section, el.dataset.key, el.dataset.val !== undefined ? el.dataset.val : el.innerText));
        });

        this.dom.accordionList.querySelectorAll('.mfe-edit-btn').forEach(btn => {
            btn.addEventListener('click', () => triggerEdit(btn.dataset.section, btn.dataset.key, btn.dataset.val));
        });
    }

    async submitConfig(section, key, value) {
        try {
            let url = `${this.baseUrl}/api/v1/config/set`;
            let res = await fetch(url, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ section, key, value })
            }).catch(() => null);

            if ((!res || !res.ok) && typeof window !== 'undefined' && this.baseUrl !== window.location.origin) {
                res = await fetch(`${window.location.origin}/api/v1/config/set`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ section, key, value })
                }).catch(() => null);
            }

            if (res && res.ok) {
                await this.loadAll();
            } else {
                const text = res ? await res.text() : 'Network error';
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
            let res = await fetch(`${this.baseUrl}/api/v1/config/reload`, { method: 'POST' }).catch(() => null);
            if ((!res || !res.ok) && typeof window !== 'undefined' && this.baseUrl !== window.location.origin) {
                res = await fetch(`${window.location.origin}/api/v1/config/reload`, { method: 'POST' }).catch(() => null);
            }
            if (res && res.ok) {
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
            let res = await fetch(`${this.baseUrl}/api/v1/config/persist`, { method: 'POST' }).catch(() => null);
            if ((!res || !res.ok) && typeof window !== 'undefined' && this.baseUrl !== window.location.origin) {
                res = await fetch(`${window.location.origin}/api/v1/config/persist`, { method: 'POST' }).catch(() => null);
            }
            if (res && res.ok) {
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

// =============================================================================
// Registration Ritual
// =============================================================================

const registerMFE = (typeof window !== 'undefined' && window.BastienMFEElement && window.BastienMFEElement.register)
    ? window.BastienMFEElement.register
    : (tag, cls) => {
        if (typeof customElements !== 'undefined' && !customElements.get(tag)) {
            try { customElements.define(tag, cls); }
            catch (e) { class Alias extends cls {} customElements.define(tag, Alias); }
        }
    };

registerMFE('config-server-mfe', ConfigServerMFE);

})();
