import Alpine from 'alpinejs';

Alpine.data('dashboard', () => ({
  apps: [],
  isLoading: false,
  logAppName: '',
  logText: '',
  logTimer: null,
  autoScroll: true,
  toasts: [],
  actionLoading: {},
  appToDelete: null,
  configApp: null,
  configMode: 'edit',
  configLoading: false,
  configSaving: false,
  runtimeLoading: false,
  runtimeShowSensitive: false,
  configYaml: '',
  configShell: '',
  runtimeData: null,
  editConfig: { name: '', app_type: '', cwd: '', cmd: '', args: [], idle_timeout: '', env: {} },
  envEntries: [],
  registerEnvEntries: [],
  appSuffixes: [],
  app_templates: [],
  form: {
    name: '',
    domain_suffix: '',
    cwd: '',
    app_type: '',
    cmd: '',
    argsInput: '',
    idle_timeout: '5m'
  },

  init() {
    this.fetchConfig();
    this.fetchApps();
    setInterval(() => this.fetchApps(), 2000);
  },

  async fetchConfig() {
    try {
      const res = await fetch('/api/config');
      if (res.ok) {
        const data = await res.json();
        const suffixes = (data.app_suffixes || []).map(item => {
          if (typeof item === 'string') {
            return { suffix: item, scheme: 'http' };
          }
          return { suffix: item.suffix, scheme: item.scheme || 'http' };
        });
        if (suffixes.length > 0) {
          this.appSuffixes = suffixes;
          if (!this.form.domain_suffix) this.form.domain_suffix = this.appSuffixes[0].suffix;
        }
        this.app_templates = data.app_templates || [];
      }
    } catch (e) {
      console.warn('[gater] 获取 /api/config 失败，使用默认后缀配置', e);
    }
  },

  showToast(message, type = 'info', duration = 3500) {
    const id = Date.now() + Math.random().toString(36).slice(2, 6);
    this.toasts.push({ id, message, type });
    setTimeout(() => {
      this.toasts = this.toasts.filter(t => t.id !== id);
    }, duration);
  },

  removeToast(id) {
    this.toasts = this.toasts.filter(t => t.id !== id);
  },

  async fetchApps() {
    try {
      const res = await fetch('/api/apps');
      if (res.ok) {
        this.apps = await res.json();
      }
    } catch (e) {
      console.error('获取应用列表失败', e);
    }
  },

  getFullCmd(app) {
    if (!app) return '';
    const args = (app.args || []).join(' ');
    return args ? `${app.cmd} ${args}` : app.cmd;
  },

  formatDateTime(value) {
    if (!value) return '未启动';
    return new Date(value).toLocaleString();
  },

  getAppURL(name) {
    return this.apps.find(app => app.name === name)?.url || '';
  },

  isLoading(name) {
    return !!(this.actionLoading && this.actionLoading[name]);
  },

  async copyText(text, label = '内容') {
    if (!text) return;
    try {
      await navigator.clipboard.writeText(text);
      this.showToast(`${label}已复制到剪贴板`, 'success');
    } catch (err) {
      this.showToast('复制失败', 'error');
    }
  },

  async openAppConfig(app) {
    this.configApp = app;
    this.configMode = 'edit';
    this.configLoading = true;
    this.configYaml = '';
    this.configShell = '';
    this.editConfig = {
      name: app.name,
      domain_suffix: app.domain_suffix || this.appSuffixes[0]?.suffix || '',
      app_type: app.app_type || '',
      cwd: app.cwd,
      cmd: app.cmd,
      args: [...(app.args || [])],
      idle_timeout: app.idle_timeout_sec ? `${app.idle_timeout_sec}s` : '',
      env: { ...(app.env || {}) }
    };
    this.envEntries = Object.entries(this.editConfig.env).map(([key, value]) => ({ key, value }));
    document.getElementById('config_modal')?.showModal();
    try {
      const res = await fetch(`/api/apps/${encodeURIComponent(app.name)}/config`);
      if (res.ok) {
        const data = await res.json();
        this.configYaml = data.yaml || '';
        this.configShell = data.shell || '';
      }
    } catch (e) {
      this.showToast('读取配置失败: ' + e.message, 'error');
    } finally {
      this.configLoading = false;
    }
  },

  addEnvEntry() {
    this.envEntries.push({ key: '', value: '' });
  },

  removeEnvEntry(index) {
    this.envEntries.splice(index, 1);
  },

  addRegisterEnvEntry() {
    this.registerEnvEntries.push({ key: '', value: '' });
  },

  removeRegisterEnvEntry(index) {
    this.registerEnvEntries.splice(index, 1);
  },

  async saveAppConfig() {
    const env = {};
    for (const entry of this.envEntries) {
      const key = entry.key.trim();
      if (key) env[key] = entry.value;
    }
    const payload = {
      name: this.editConfig.name,
      domain_suffix: this.editConfig.domain_suffix,
      app_type: this.editConfig.app_type.trim(),
      cwd: this.editConfig.cwd.trim(),
      cmd: this.editConfig.cmd.trim(),
      args: this.editConfig.args,
      env,
      idle_timeout: this.editConfig.idle_timeout.trim()
    };
    this.configSaving = true;
    try {
      const res = await fetch(`/api/apps/${encodeURIComponent(payload.name)}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        this.showToast('保存失败: ' + (data.error || res.statusText), 'error');
        return;
      }
      this.showToast(`应用 [${payload.name}] 配置已保存`, 'success');
      await this.fetchApps();
      this.closeAppConfig();
    } catch (e) {
      this.showToast('保存配置失败: ' + e.message, 'error');
    } finally {
      this.configSaving = false;
    }
  },

  closeAppConfig() {
    document.getElementById('config_modal')?.close();
    this.configApp = null;
  },

  async openRuntime(app) {
    this.runtimeData = null;
    this.runtimeShowSensitive = false;
    this.runtimeLoading = true;
    const modal = document.getElementById('runtime_modal');
    if (modal) modal.showModal();
    try {
      const res = await fetch(this.runtimeURL(app.name));
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        this.showToast('读取运行配置失败: ' + (data.error || res.statusText), 'error');
        return;
      }
      this.runtimeData = await res.json();
    } catch (e) {
      this.showToast('读取运行配置失败: ' + e.message, 'error');
    } finally {
      this.runtimeLoading = false;
    }
  },

  runtimeURL(name) {
    const query = this.runtimeShowSensitive ? '?show_sensitive=true' : '';
    return `/api/apps/${encodeURIComponent(name)}/runtime${query}`;
  },

  async toggleRuntimeSensitive() {
    if (!this.runtimeData || !this.runtimeData.name) return;
    this.runtimeLoading = true;
    try {
      const res = await fetch(this.runtimeURL(this.runtimeData.name));
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        this.showToast('读取运行配置失败: ' + (data.error || res.statusText), 'error');
        this.runtimeShowSensitive = !this.runtimeShowSensitive;
        return;
      }
      this.runtimeData = await res.json();
    } catch (e) {
      this.runtimeShowSensitive = !this.runtimeShowSensitive;
      this.showToast('读取运行配置失败: ' + e.message, 'error');
    } finally {
      this.runtimeLoading = false;
    }
  },

  closeRuntime() {
    const modal = document.getElementById('runtime_modal');
    if (modal) modal.close();
    this.runtimeData = null;
  },

  openAddModal() {
    const modal = document.getElementById('add_modal');
    if (modal) modal.showModal();
  },

  closeAddModal() {
    const modal = document.getElementById('add_modal');
    if (modal) modal.close();
  },

  async openExistingApp(name) {
    await this.fetchApps();
    const existing = this.apps.find(item => item.name === name);
    if (existing) {
      this.closeAddModal();
      await this.openAppConfig(existing);
    }
  },

  async pickYAMLFile() {
    const pickRes = await fetch('/api/fs/pick-yaml-file', { method: 'POST' });
    const picked = await pickRes.json();
    if (picked.canceled) return null;
    if (!pickRes.ok || !picked.config || !picked.path) {
      this.showToast('所选文件不是有效的 app.yaml', 'error');
      return null;
    }
    return picked;
  },

  async addYAML(path) {
    return fetch('/api/apps/from-yaml', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path })
    });
  },

  async registerFromYAML() {
    try {
      const picked = await this.pickYAMLFile();
      if (!picked) return;

      const res = await this.addYAML(picked.path);
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        if (res.status === 409) {
          this.showToast('应用已存在，请修改现有配置', 'warning');
          await this.openExistingApp(picked.config.name);
          return;
        }
        this.showToast('注册失败: ' + (data.error || res.statusText), 'error');
        return;
      }
      this.showToast(`应用 [${picked.config.name}] 注册成功`, 'success');
      this.closeAddModal();
      await this.fetchApps();
    } catch (e) {
      this.showToast('读取 app.yaml 失败: ' + e.message, 'error');
    }
  },

  fillPreset(preset) {
    const hasValues = this.form.app_type || this.form.cmd || this.form.argsInput || this.registerEnvEntries.some(entry => entry.key || entry.value);
    if (hasValues && !window.confirm(`当前启动配置不为空，确定使用「${preset.label}」覆盖吗？`)) {
      return;
    }
    this.form.app_type = preset.app_type || '';
    this.form.cmd = preset.cmd || '';
    this.form.argsInput = (preset.args || []).join(' ');
    this.form.idle_timeout = preset.idle_timeout || '5m';
    this.registerEnvEntries = preset.env;
  },

  async submitApp() {
    const name = this.form.name.trim();
    const cwd = this.form.cwd.trim();
    const cmd = this.form.cmd.trim();

    if (!name || !cwd || !cmd) {
      this.showToast('请填写所有必填字段', 'error');
      return;
    }

    const payload = {
      name: name,
      domain_suffix: this.form.domain_suffix,
      app_type: this.form.app_type.trim(),
      cwd,
      cmd: cmd,
      args: this.form.argsInput.trim() ? this.form.argsInput.trim().split(/\s+/) : [],
      env: Object.fromEntries(this.registerEnvEntries
        .map(entry => [entry.key.trim(), entry.value])
        .filter(([key]) => key)),
      idle_timeout: this.form.idle_timeout.trim() || '5m'
    };

    try {
      const res = await fetch('/api/apps/from-config', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });

      if (!res.ok) {
        const errorData = await res.json().catch(() => ({}));
        if (res.status === 409) {
          this.showToast('应用已存在，请修改现有配置', 'warning');
          await this.openExistingApp(name);
          return;
        }
        this.showToast('注册失败: ' + (errorData.error || res.statusText), 'error');
        return;
      }

      this.showToast(`应用 [${name}] 注册成功`, 'success');
      this.form = { name: '', cwd: '', app_type: '', cmd: '', argsInput: '', idle_timeout: '5m' };
      this.registerEnvEntries = [];
      this.closeAddModal();
      await this.fetchApps();
    } catch (e) {
      console.error('提交应用失败', e);
      this.showToast('网络请求错误: ' + e.message, 'error');
    }
  },

  confirmDeleteApp(app) {
    this.appToDelete = app;
    const modal = document.getElementById('delete_modal');
    if (modal) modal.showModal();
  },

  closeDeleteModal() {
    this.appToDelete = null;
    const modal = document.getElementById('delete_modal');
    if (modal) modal.close();
  },

  async executeDeleteApp() {
    if (!this.appToDelete) return;
    const name = this.appToDelete.name;
    try {
      const res = await fetch(`/api/apps/${encodeURIComponent(name)}`, { method: 'DELETE' });
      if (res.ok) {
        this.showToast(`应用 [${name}] 已注销`, 'success');
        this.closeDeleteModal();
        await this.fetchApps();
      } else {
        const data = await res.json().catch(() => ({}));
        this.showToast('注销失败: ' + (data.error || res.statusText), 'error');
      }
    } catch (e) {
      console.error('删除应用失败', e);
      this.showToast('注销失败: ' + e.message, 'error');
    }
  },

  async startApp(name) {
    this.actionLoading = { ...this.actionLoading, [name]: true };
    try {
      const res = await fetch(`/api/apps/${encodeURIComponent(name)}/start`, { method: 'POST' });
      if (res.ok) {
        this.showToast(`应用 [${name}] 已成功拉起`, 'success');
      } else {
        const data = await res.json().catch(() => ({}));
        this.showToast(`拉起失败: ${data.error || res.statusText}`, 'error');
      }
    } catch (e) {
      console.error('启动应用失败', e);
      this.showToast(`启动请求异常: ${e.message}`, 'error');
    } finally {
      this.actionLoading = { ...this.actionLoading, [name]: false };
      await this.fetchApps();
    }
  },

  async stopApp(name) {
    this.actionLoading = { ...this.actionLoading, [name]: true };
    try {
      const res = await fetch(`/api/apps/${encodeURIComponent(name)}/stop`, { method: 'POST' });
      if (res.ok) {
        this.showToast(`应用 [${name}] 已停止`, 'info');
      } else {
        const data = await res.json().catch(() => ({}));
        this.showToast(`停止失败: ${data.error || res.statusText}`, 'error');
      }
    } catch (e) {
      console.error('停止应用失败', e);
      this.showToast(`停止请求异常: ${e.message}`, 'error');
    } finally {
      this.actionLoading = { ...this.actionLoading, [name]: false };
      await this.fetchApps();
    }
  },

  async openLogs(name) {
    this.logAppName = name;
    this.logText = '加载日志中...';
    const modal = document.getElementById('log_modal');
    if (modal) modal.showModal();
    await this.fetchLogs();
    if (this.logTimer) clearInterval(this.logTimer);
    this.logTimer = setInterval(() => this.fetchLogs(), 1500);
  },

  async fetchLogs() {
    if (!this.logAppName) return;
    try {
      const res = await fetch(`/api/apps/${encodeURIComponent(this.logAppName)}/logs`);
      if (res.ok) {
        const data = await res.json();
        this.logText = data.logs || '(暂无运行日志)';
        if (this.autoScroll) {
          this.$nextTick(() => {
            const el = document.getElementById('log_content');
            if (el) el.scrollTop = el.scrollHeight;
          });
        }
      }
    } catch (e) {
      console.error('获取日志失败', e);
    }
  },

  async copyLogs() {
    if (!this.logText) return;
    try {
      await navigator.clipboard.writeText(this.logText);
      this.showToast('日志已复制到剪贴板', 'success');
    } catch (err) {
      this.showToast('复制失败，请手动选择复制', 'error');
    }
  },

  closeLogs() {
    if (this.logTimer) {
      clearInterval(this.logTimer);
      this.logTimer = null;
    }
    this.logAppName = '';
    const modal = document.getElementById('log_modal');
    if (modal) modal.close();
  }
}));

// 启动 Alpine
Alpine.start();