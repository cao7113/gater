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
  form: {
    name: '',
    path: '',
    cmd: '',
    argsInput: '',
    idle_timeout: '5m'
  },

  init() {
    this.fetchApps();
    setInterval(() => this.fetchApps(), 2000);
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

  openAddModal() {
    const modal = document.getElementById('add_modal');
    if (modal) modal.showModal();
  },

  closeAddModal() {
    const modal = document.getElementById('add_modal');
    if (modal) modal.close();
  },

  fillPreset(type) {
    if (type === 'python') {
      this.form.cmd = 'python3';
      this.form.argsInput = '-m http.server $PORT';
      this.form.idle_timeout = '5m';
    } else if (type === 'npm') {
      this.form.cmd = 'npm';
      this.form.argsInput = 'run dev -- --port $PORT';
      this.form.idle_timeout = '15m';
    } else if (type === 'phx') {
      this.form.cmd = 'mix';
      this.form.argsInput = 'phx.server';
      this.form.idle_timeout = '10m';
    } else if (type === 'bun') {
      this.form.cmd = 'bun';
      this.form.argsInput = 'run dev --port $PORT';
      this.form.idle_timeout = '15m';
    }
  },

  async submitApp() {
    const name = this.form.name.trim();
    const path = this.form.path.trim();
    const cmd = this.form.cmd.trim();

    if (!name || !path || !cmd) {
      this.showToast('请填写所有必填字段', 'error');
      return;
    }

    const payload = {
      name: name,
      path: path,
      cmd: cmd,
      args: this.form.argsInput.trim() ? this.form.argsInput.trim().split(/\s+/) : [],
      idle_timeout: this.form.idle_timeout.trim() || '5m'
    };

    try {
      const res = await fetch('/api/apps', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });

      if (!res.ok) {
        const errorData = await res.json().catch(() => ({}));
        this.showToast('注册失败: ' + (errorData.error || res.statusText), 'error');
        return;
      }

      this.showToast(`应用 [${name}] 注册成功`, 'success');
      this.form = { name: '', path: '', cmd: '', argsInput: '', idle_timeout: '5m' };
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