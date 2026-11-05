/* Code Review AI - UI logic (Alpine.js) */

const LS_REVIEWS = 'codereview_results_v2';
const LS_REVIEWS_LEGACY = 'codereview_results';
const LS_THEME = 'codereview_theme';
const LS_EXPANDED = 'codereview_expanded';

const SEVERITY_RANK = { critical: 4, high: 3, medium: 2, low: 1 };

const VERDICTS = {
  approve: { label: 'Można zatwierdzić', short: 'OK', dot: 'green', icon: '✅' },
  approve_with_comments: { label: 'Zatwierdź po drobnych poprawkach', short: 'uwagi', dot: 'yellow', icon: '💬' },
  request_changes: { label: 'Wymagane zmiany przed merge', short: 'wymaga zmian', dot: 'red', icon: '⛔' },
};

const MERGE_STATUS = {
  mergeable: 'gotowy do scalenia',
  conflict: 'konflikty',
  broken_status: 'konflikty',
  need_rebase: 'wymaga rebase',
  ci_must_pass: 'czeka na CI',
  ci_still_running: 'CI w trakcie',
  not_approved: 'brak zatwierdzeń',
  requested_changes: 'zgłoszono zmiany',
  discussions_not_resolved: 'nierozwiązane dyskusje',
  draft_status: 'draft',
  blocked_status: 'zablokowany',
  checking: 'sprawdzanie',
  unchecked: 'niesprawdzony',
  not_open: 'zamknięty',
};

const LANG_BY_EXT = {
  js: 'javascript', mjs: 'javascript', cjs: 'javascript', jsx: 'javascript', ts: 'typescript', tsx: 'typescript',
  go: 'go', php: 'php', py: 'python', rb: 'ruby', java: 'java', kt: 'kotlin', cs: 'csharp', rs: 'rust',
  sql: 'sql', json: 'json', yml: 'yaml', yaml: 'yaml', xml: 'xml', html: 'xml', twig: 'xml', vue: 'xml',
  css: 'css', scss: 'scss', md: 'markdown', sh: 'bash', bash: 'bash', dockerfile: 'dockerfile',
};

function escapeHtml(s) {
  return String(s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}

function readJSON(key, fallback) {
  try { const v = localStorage.getItem(key); return v ? JSON.parse(v) : fallback; } catch { return fallback; }
}
function writeJSON(key, value) {
  try { localStorage.setItem(key, JSON.stringify(value)); } catch { /* storage unavailable */ }
}

/** Parse a GitLab unified diff into rows with old/new line numbers. */
function parseDiff(text) {
  const rows = [];
  let o = 0, n = 0, add = 0, del = 0;
  for (const line of (text || '').split('\n')) {
    const m = line.match(/^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@(.*)/);
    if (m) { o = +m[1]; n = +m[2]; rows.push({ t: 'hunk', code: line }); continue; }
    if (line === '' || line.startsWith('\\ No newline')) continue;
    const c = line[0];
    if (c === '+') { rows.push({ t: 'add', n, code: line.slice(1) }); n++; add++; }
    else if (c === '-') { rows.push({ t: 'del', o, code: line.slice(1) }); o++; del++; }
    else { rows.push({ t: 'ctx', o, n, code: line.startsWith(' ') ? line.slice(1) : line }); o++; n++; }
  }
  return { rows, add, del };
}

function app() {
  return {
    // ---------- state ----------
    config: {},
    theme: 'dark',
    projects: [],
    projectsLoading: true,
    projectsError: '',
    expanded: {},          // pid -> bool
    lists: {},             // "pid:state" -> { items, loading, error }
    running: {},           // "pid/iid" -> { done, total, started_at, engine, title }
    reviews: {},           // "pid/iid" -> latest review + meta (from SQLite)
    history: [],           // stored reviews of the open MR, newest first
    historyPick: null,     // id of an older review being viewed
    settings: { groups: [], items: [], loading: false, saving: false, error: '', dbPath: '' },
    form: {},              // edited setting values (key -> value)
    resets: {},            // keys to reset to .env/default
    tests: {},             // connection test results per target
    errors: {},            // "pid/iid" -> last review error
    route: { view: 'dashboard', pid: null, iid: null, tab: 'review' },
    listState: 'opened',
    listQuery: '',
    detail: null,
    detailLoading: false,
    detailError: '',
    issueFilter: 'all',
    collapsed: {},
    search: '',
    showAllProjects: false,
    toasts: [],
    now: Date.now(),
    ws: null,
    wsConnected: false,

    // ---------- lifecycle ----------
    async init() {
      this.theme = readJSON(LS_THEME, null) || (matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark');
      this.applyTheme();
      this.expanded = readJSON(LS_EXPANDED, {});
      window.addEventListener('beforeunload', e => { if (this.dirtyCount) { e.preventDefault(); e.returnValue = ''; } });
      setInterval(() => { this.now = Date.now(); }, 1000);
      window.addEventListener('hashchange', () => this.applyRoute());
      window.addEventListener('keydown', e => {
        if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); this.$refs.search?.focus(); }
        if (e.key === 'Escape' && document.activeElement === this.$refs.search) { this.search = ''; this.$refs.search.blur(); }
      });
      this.connectWS();
      try { this.config = await this.api('/api/v1/config'); } catch (e) { this.toast('Brak konfiguracji', e.message, 'red'); }
      await this.loadReviews();
      await this.migrateLocalReviews();
      await this.loadProjects();
      this.applyRoute();
      for (const pid of Object.keys(this.expanded)) if (this.expanded[pid]) this.ensureList(+pid, 'opened');
    },

    async api(url, opts) {
      const res = await fetch(url, opts);
      let body = null;
      try { body = await res.json(); } catch { /* empty body */ }
      if (!res.ok) {
        const err = new Error(body?.error || `HTTP ${res.status}`);
        err.status = res.status;
        throw err;
      }
      return body;
    },

    // ---------- theme ----------
    applyTheme() { document.documentElement.dataset.theme = this.theme; },
    toggleTheme() { this.theme = this.theme === 'dark' ? 'light' : 'dark'; this.applyTheme(); writeJSON(LS_THEME, this.theme); },

    // ---------- websocket ----------
    connectWS() {
      const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
      try {
        this.ws = new WebSocket(`${proto}//${location.host}/api/v1/ws`);
        this.ws.onopen = () => { this.wsConnected = true; };
        this.ws.onmessage = e => this.onWS(e);
        this.ws.onclose = () => { this.wsConnected = false; setTimeout(() => this.connectWS(), 4000); };
      } catch { setTimeout(() => this.connectWS(), 4000); }
    },

    onWS(e) {
      let msg;
      try { msg = JSON.parse(e.data); } catch { return; }
      const pid = msg.project_id || this.config.default_project_id;
      const key = `${pid}/${msg.mr_id}`;
      if (msg.type === 'review_part') {
        const r = this.running[key] || (this.running[key] = { done: 0, total: msg.total_files, started_at: new Date().toISOString() });
        r.done = Math.min((r.done || 0) + 1, msg.total_files || r.total || 1);
        r.total = msg.total_files || r.total;
      } else if (msg.type === 'review' && msg.review) {
        const meta = this.running[key] || {};
        delete this.running[key];
        delete this.errors[key];
        this.rememberReview(pid, msg.mr_id, msg.review, meta);
        if (this.detail?.pid === +pid && this.detail?.iid === +msg.mr_id) { this.historyPick = null; this.loadHistory(pid, msg.mr_id); }
        const v = VERDICTS[msg.review.verdict];
        this.toast(`Review !${msg.mr_id} gotowe`, v ? `${v.icon} ${v.label}` : `${(msg.review.issues || []).length} uwag`, v?.dot || 'green');
      } else if (msg.type === 'review_error') {
        delete this.running[key];
        this.errors[key] = msg.error || msg.message || 'Błąd review';
        this.toast(`Review !${msg.mr_id} nie powiodło się`, this.errors[key], 'red', 8000);
      }
    },

    // ---------- data ----------
    async loadProjects(refresh = false) {
      this.projectsLoading = true;
      this.projectsError = '';
      try {
        const data = await this.api('/api/v1/projects' + (refresh ? '?refresh=1' : ''));
        this.projects = data.projects || [];
      } catch (e) {
        this.projectsError = e.message;
      } finally {
        this.projectsLoading = false;
      }
    },

    project(pid) { return this.projects.find(p => p.id === +pid); },

    listKey(pid, state) { return `${pid}:${state}`; },
    list(pid, state = 'opened') { return this.lists[this.listKey(pid, state)] || { items: [], loading: false, error: '' }; },

    async ensureList(pid, state = 'opened', force = false) {
      const key = this.listKey(pid, state);
      const cur = this.lists[key];
      if (cur && (cur.loading || (cur.loaded && !force))) return;
      this.lists[key] = { items: cur?.items || [], loading: true, error: '', loaded: cur?.loaded };
      try {
        const data = await this.api(`/api/v1/projects/${pid}/merge-requests?state=${state}`);
        this.lists[key] = { items: data.merge_requests || [], loading: false, error: '', loaded: true };
        // server is the source of truth for running reviews
        for (const k of Object.keys(this.running)) if (k.startsWith(`${pid}/`) && !(data.running || {})[k.split('/')[1]]) delete this.running[k];
        for (const [iid, p] of Object.entries(data.running || {})) this.running[`${pid}/${iid}`] = { ...this.running[`${pid}/${iid}`], ...p };
        if (state === 'opened') {
          const proj = this.project(pid);
          if (proj) proj.open_mrs = data.count;
        }
      } catch (e) {
        this.lists[key] = { items: cur?.items || [], loading: false, error: e.message, loaded: false };
      }
    },

    async loadDetail(pid, iid) {
      const same = this.detail && this.detail.pid === pid && this.detail.iid === iid;
      if (!same) { this.detail = null; this.issueFilter = 'all'; this.collapsed = {}; this.history = []; this.historyPick = null; this.loadHistory(pid, iid); }
      this.detailLoading = !same;
      this.detailError = '';
      try {
        const data = await this.api(`/api/v1/projects/${pid}/merge-requests/${iid}`);
        const files = (data.diffs || []).map(d => ({ ...d, ...parseDiff(d.diff) }));
        this.detail = {
          pid, iid, mr: data.merge_request, files,
          add: files.reduce((s, f) => s + f.add, 0),
          del: files.reduce((s, f) => s + f.del, 0),
        };
        const key = `${pid}/${iid}`;
        if (data.running) this.running[key] = { ...this.running[key], ...data.running, title: data.merge_request.title };
        else if (this.running[key] && !this.running[key].local) delete this.running[key];
      } catch (e) {
        this.detailError = e.message;
      } finally {
        this.detailLoading = false;
      }
    },

    async startReview(pid, iid) {
      const key = `${pid}/${iid}`;
      if (this.running[key]) return;
      const mr = this.detail?.mr?.iid === iid ? this.detail.mr : this.list(pid).items.find(m => m.iid === iid);
      this.running[key] = { done: 0, total: 0, started_at: new Date().toISOString(), engine: this.config.engine, title: mr?.title, sha: mr?.sha, local: true };
      delete this.errors[key];
      try {
        const data = await this.api(`/api/v1/projects/${pid}/merge-requests/${iid}/review`, { method: 'POST' });
        Object.assign(this.running[key] || {}, { total: data.total_files, engine: data.engine });
        this.toast(`Review !${iid} uruchomione`, `${data.engine === 'claude-code' ? 'Agent Claude Code' : 'LLM'} · ${data.total_files} plików`, 'purple');
      } catch (e) {
        if (e.status === 409) { this.toast('Review już trwa', `!${iid} jest właśnie analizowany`, 'yellow'); return; }
        delete this.running[key];
        this.toast('Nie udało się uruchomić review', e.message, 'red', 8000);
      }
    },

    // ---------- reviews (SQLite on the server) ----------
    fromRecord(rec) {
      return { ...(rec.review || {}), id: rec.id, timestamp: rec.created_at, sha: rec.sha, title: rec.title, engine: rec.engine, pid: rec.project_id, iid: rec.iid };
    },
    async loadReviews() {
      try {
        const data = await this.api('/api/v1/reviews?limit=1000');
        const map = {};
        for (const rec of data.reviews || []) map[`${rec.project_id}/${rec.iid}`] = this.fromRecord(rec);
        this.reviews = map;
      } catch (e) {
        this.toast('Nie udało się wczytać wyników review', e.message, 'red');
      }
    },
    async loadHistory(pid, iid) {
      try {
        const data = await this.api(`/api/v1/projects/${pid}/merge-requests/${iid}/reviews`);
        if (this.detail && (this.detail.pid !== +pid || this.detail.iid !== +iid) && this.route.iid !== +iid) return;
        this.history = (data.reviews || []).map(r => this.fromRecord(r));
      } catch { this.history = []; }
    },
    /** Keep a just-finished review in memory (the server has already stored it). */
    rememberReview(pid, iid, review, meta = {}) {
      const mr = this.findMR(pid, iid);
      this.reviews[`${pid}/${iid}`] = {
        ...review,
        timestamp: new Date().toISOString(),
        sha: meta.sha || mr?.sha || '',
        title: mr?.title || meta.title || `!${iid}`,
        engine: meta.engine || this.config.engine,
        pid: +pid, iid: +iid,
      };
    },
    async clearReview(pid, iid) {
      if (!confirm(`Usunąć wszystkie zapisane wyniki review !${iid} z bazy? Komentarze w GitLab zostają.`)) return;
      try {
        await this.api(`/api/v1/projects/${pid}/merge-requests/${iid}/reviews`, { method: 'DELETE' });
        delete this.reviews[`${pid}/${iid}`];
        delete this.errors[`${pid}/${iid}`];
        this.history = [];
        this.historyPick = null;
        this.toast('Wyniki usunięte', `Review !${iid} usunięte z bazy (komentarze w GitLab zostają)`, 'gray');
      } catch (e) {
        this.toast('Nie udało się usunąć', e.message, 'red');
      }
    },
    /** One-time move of results kept in the browser by older versions into SQLite. */
    async migrateLocalReviews() {
      const items = [];
      const v2 = readJSON(LS_REVIEWS, null) || {};
      for (const [key, r] of Object.entries(v2)) {
        const [pid, iid] = key.split('/').map(Number);
        items.push({ project_id: pid, iid, sha: r.sha || '', title: r.title || '', timestamp: r.timestamp, review: r });
      }
      const legacy = readJSON(LS_REVIEWS_LEGACY, null) || {};
      const dpid = this.config.default_project_id;
      if (dpid) for (const [iid, r] of Object.entries(legacy)) items.push({ project_id: dpid, iid: +iid, sha: '', title: r.title || '', timestamp: r.timestamp, review: r });
      if (!items.length) return;
      try {
        const res = await this.api('/api/v1/reviews/import', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(items) });
        try { localStorage.removeItem(LS_REVIEWS); localStorage.removeItem(LS_REVIEWS_LEGACY); } catch { /* ignore */ }
        if (res.imported) { this.toast('Przeniesiono wyniki do bazy', `${res.imported} review z przeglądarki zapisano w SQLite`, 'green'); await this.loadReviews(); }
      } catch (e) {
        console.warn('Review import failed', e);
      }
    },
    review(pid, iid) { return this.reviews[`${pid}/${iid}`] || null; },
    findMR(pid, iid) {
      if (this.detail?.pid === +pid && this.detail?.iid === +iid) return this.detail.mr;
      for (const st of ['opened', 'merged', 'closed']) {
        const m = this.list(pid, st).items.find(x => x.iid === +iid);
        if (m) return m;
      }
      return null;
    },

    /** AI review status of an MR, shown as a pill/dot. */
    status(pid, mr) {
      const key = `${pid}/${mr.iid}`;
      if (this.running[key]) return { label: 'analiza…', dot: 'purple pulse' };
      if (this.errors[key]) return { label: 'błąd review', dot: 'red' };
      const r = this.reviews[key];
      if (!r) return { label: 'bez review', dot: '' };
      const v = VERDICTS[r.verdict];
      const label = v ? v.short : 'sprawdzony';
      const outdated = r.sha && mr.sha && r.sha !== mr.sha;
      return { label: outdated ? `${label} · nieaktualne` : label, dot: v ? v.dot : 'blue', outdated };
    },

    severityCounts(review) {
      const c = { critical: 0, high: 0, medium: 0, low: 0 };
      for (const i of review?.issues || []) if (c[i.severity] !== undefined) c[i.severity]++;
      return c;
    },

    // ---------- routing ----------
    go(hash) { if (location.hash !== hash) location.hash = hash; else this.applyRoute(); },
    applyRoute() {
      const parts = location.hash.replace(/^#\/?/, '').split('/').filter(Boolean);
      if (parts[0] === 'p' && parts[1]) {
        const pid = +parts[1];
        this.expanded[pid] = true;
        writeJSON(LS_EXPANDED, this.expanded);
        this.ensureList(pid, 'opened');
        if (parts[2] === 'mr' && parts[3]) {
          const iid = +parts[3];
          this.route = { view: 'mr', pid, iid, tab: parts[4] || 'review' };
          this.loadDetail(pid, iid);
        } else {
          this.route = { view: 'project', pid, iid: null, tab: 'review' };
          this.listQuery = '';
          this.ensureList(pid, this.listState);
        }
      } else if (parts[0] === 'settings') {
        this.route = { view: 'settings', pid: null, iid: null, tab: parts[1] || '' };
        if (!this.settings.items.length) this.loadSettings();
      } else {
        this.route = { view: 'dashboard', pid: null, iid: null, tab: 'review' };
      }
    },
    openProject(pid) { this.go(`#/p/${pid}`); },
    openMR(pid, iid, tab) { this.go(`#/p/${pid}/mr/${iid}` + (tab && tab !== 'review' ? `/${tab}` : '')); },
    setTab(tab) { this.openMR(this.route.pid, this.route.iid, tab); },
    toggleExpand(pid) {
      this.expanded[pid] = !this.expanded[pid];
      writeJSON(LS_EXPANDED, this.expanded);
      if (this.expanded[pid]) this.ensureList(pid, 'opened');
    },
    setListState(state) { this.listState = state; this.ensureList(this.route.pid, state); },
    refreshCurrent() {
      if (this.route.view === 'project') { this.ensureList(this.route.pid, this.listState, true); this.ensureList(this.route.pid, 'opened', true); }
      else if (this.route.view === 'mr') this.loadDetail(this.route.pid, this.route.iid);
      else this.loadProjects(true);
    },

    // ---------- derived ----------
    get treeProjects() {
      const q = this.search.trim().toLowerCase();
      if (!q) return this.projects;
      return this.projects.filter(p =>
        p.name_with_namespace.toLowerCase().includes(q) ||
        this.list(p.id).items.some(m => this.mrMatches(m, q)));
    },
    /** Tree shows projects with open MRs (plus the selected one) unless "show all" or a search is active. */
    get visibleTreeProjects() {
      if (this.search.trim() || this.showAllProjects) return this.treeProjects;
      return this.treeProjects.filter(p => p.open_mrs > 0 || p.id === this.route.pid);
    },
    get hiddenProjectsCount() {
      if (this.search.trim() || this.showAllProjects) return 0;
      return this.treeProjects.length - this.visibleTreeProjects.length;
    },
    treeMRs(pid) {
      const q = this.search.trim().toLowerCase();
      const items = this.list(pid).items;
      if (!q) return items;
      const proj = this.project(pid);
      if (proj && proj.name_with_namespace.toLowerCase().includes(q)) return items;
      return items.filter(m => this.mrMatches(m, q));
    },
    mrMatches(m, q) {
      return m.title.toLowerCase().includes(q) || `!${m.iid}`.includes(q) || m.source_branch.toLowerCase().includes(q) || (m.author?.name || '').toLowerCase().includes(q);
    },
    get currentProject() { return this.project(this.route.pid); },
    get currentList() {
      const items = this.list(this.route.pid, this.listState).items;
      const q = this.listQuery.trim().toLowerCase();
      return q ? items.filter(m => this.mrMatches(m, q)) : items;
    },
    get runningList() {
      return Object.entries(this.running).map(([k, r]) => {
        const [pid, iid] = k.split('/').map(Number);
        return { key: k, pid, iid, title: r.title || this.findMR(pid, iid)?.title || `!${iid}`, ...r };
      });
    },
    get recentReviews() {
      return Object.entries(this.reviews)
        .map(([k, r]) => ({ key: k, ...r, pid: r.pid || +k.split('/')[0], iid: r.iid || +k.split('/')[1] }))
        .sort((a, b) => (b.timestamp || '').localeCompare(a.timestamp || ''));
    },
    get totalOpen() { return this.projects.reduce((s, p) => s + (p.open_mrs || 0), 0); },
    get currentReview() {
      if (!this.detail) return null;
      if (this.historyPick) return this.history.find(h => h.id === this.historyPick) || null;
      return this.review(this.detail.pid, this.detail.iid);
    },
    get viewingOldReview() { return !!this.historyPick && this.history[0]?.id !== this.historyPick; },
    get currentRunning() { return this.detail ? this.running[`${this.detail.pid}/${this.detail.iid}`] : null; },
    get currentError() { return this.detail ? this.errors[`${this.detail.pid}/${this.detail.iid}`] : null; },
    get filteredIssues() {
      const issues = [...(this.currentReview?.issues || [])].sort((a, b) => (SEVERITY_RANK[b.severity] || 0) - (SEVERITY_RANK[a.severity] || 0));
      return this.issueFilter === 'all' ? issues : issues.filter(i => i.severity === this.issueFilter);
    },
    get issueGroups() {
      const groups = {};
      for (const i of this.filteredIssues) (groups[i.file || ''] ||= []).push(i);
      return Object.entries(groups)
        .sort(([a], [b]) => (a === '') - (b === '') || a.localeCompare(b))
        .map(([file, issues]) => ({ file, issues }));
    },
    fileIssues(path) { return (this.currentReview?.issues || []).filter(i => i.file === path); },

    // ---------- rendering helpers ----------
    md(text) {
      if (!text) return '';
      try { return DOMPurify.sanitize(marked.parse(String(text), { breaks: true, gfm: true })); }
      catch { return escapeHtml(text).replace(/\n/g, '<br>'); }
    },
    esc: escapeHtml,
    verdict(v) { return VERDICTS[v] || null; },
    mergeStatus(s) { return MERGE_STATUS[s] || s || '—'; },
    followupIcon(s) { return { fixed: '✅', open: '⏳', accepted: '👍' }[s] || '•'; },
    followupLabel(s) { return { fixed: 'Poprawione', open: 'Nadal otwarte', accepted: 'Wyjaśnienie przyjęte' }[s] || s; },
    followupSummary(f) { return `poprawione ${f.fixed}, otwarte ${f.open}, zaakceptowane ${f.accepted}, nowe ${f.new}`; },
    reqIcon(s) { return { done: '✅', partial: '⚠️', missing: '❌', unclear: '❓' }[s] || '•'; },
    reqSummary(reqs) {
      if (!reqs?.length) return '';
      const done = reqs.filter(r => r.status === 'done').length;
      return `${done}/${reqs.length} zrealizowanych`;
    },
    relTime(iso) {
      if (!iso) return '—';
      const s = Math.max(0, (this.now - new Date(iso).getTime()) / 1000);
      if (s < 60) return 'teraz';
      if (s < 3600) return `${Math.floor(s / 60)}m`;
      if (s < 86400) return `${Math.floor(s / 3600)}h`;
      if (s < 86400 * 30) return `${Math.floor(s / 86400)}d`;
      return new Date(iso).toLocaleDateString('pl-PL');
    },
    fullDate(iso) { return iso ? new Date(iso).toLocaleString('pl-PL') : '—'; },
    elapsed(iso) {
      if (!iso) return '';
      const s = Math.max(0, Math.floor((this.now - new Date(iso).getTime()) / 1000));
      return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
    },
    /** Polish plural: plural(1,'uwaga','uwagi','uwag') -> "1 uwaga", 3 -> "3 uwagi", 5 -> "5 uwag". */
    plural(n, one, few, many) {
      const word = n === 1 ? one : (n % 10 >= 2 && n % 10 <= 4 && (n % 100 < 12 || n % 100 > 14)) ? few : many;
      return `${n} ${word}`;
    },
    initials(name) { return (name || '?').split(/[\s/_-]+/).filter(Boolean).slice(0, 2).map(w => w[0].toUpperCase()).join(''); },

    highlight(code, path) {
      const ext = (path.split('.').pop() || '').toLowerCase();
      const lang = LANG_BY_EXT[ext] || (path.toLowerCase().endsWith('dockerfile') ? 'dockerfile' : null);
      if (window.hljs && lang && hljs.getLanguage(lang)) {
        try { return hljs.highlight(code, { language: lang, ignoreIllegals: true }).value; } catch { /* fall through */ }
      }
      return escapeHtml(code);
    },

    issueCardHTML(i, withLoc) {
      const loc = withLoc && i.file ? `<span class="issue-loc">${escapeHtml(i.file)}${i.line ? ':' + escapeHtml(i.line) : ''}</span>` : '';
      return `<div class="issue-card ${escapeHtml(i.severity)}">
        <div class="issue-top"><span class="sev ${escapeHtml(i.severity)}">${escapeHtml(i.severity)}</span><span class="issue-type">${escapeHtml(i.type)}</span>${loc}</div>
        <div class="md">${this.md(i.description)}</div>
        ${i.code ? `<div class="issue-code">${escapeHtml(i.code)}</div>` : ''}
        ${i.fix ? `<div class="issue-fix"><div class="issue-fix-label">Propozycja poprawki</div><div class="md">${this.md(i.fix)}</div></div>` : ''}
      </div>`;
    },

    /** Unified diff of one file with AI comments inline under the commented lines. */
    diffHTML(fi) {
      const f = this.detail?.files[fi];
      if (!f) return '';
      if (!f.rows.length) return `<div class="diff-empty">${f.deleted_file ? 'Plik usunięty' : 'Brak zmian tekstowych (plik binarny lub zbyt duży)'}</div>`;
      const byLine = {};
      const shown = new Set();
      for (const i of this.fileIssues(f.new_path)) {
        const n = parseInt(i.line, 10);
        if (n > 0) (byLine[n] ||= []).push(i);
      }
      let html = '<table class="diff-table"><tbody>';
      for (const r of f.rows) {
        if (r.t === 'hunk') { html += `<tr class="hunk"><td colspan="4">${escapeHtml(r.code)}</td></tr>`; continue; }
        const cls = r.t === 'add' ? 'add' : r.t === 'del' ? 'del' : '';
        const id = r.n ? ` id="d-${fi}-${r.n}"` : '';
        html += `<tr class="${cls}"${id}><td class="ln">${r.o ?? ''}</td><td class="ln">${r.t === 'del' ? '' : r.n}</td><td class="sign">${r.t === 'add' ? '+' : r.t === 'del' ? '−' : ''}</td><td class="code">${this.highlight(r.code, f.new_path)}</td></tr>`;
        if (r.t !== 'del' && byLine[r.n] && !shown.has(r.n)) {
          shown.add(r.n);
          for (const i of byLine[r.n]) html += `<tr class="comment"><td colspan="4">${this.issueCardHTML(i, false)}</td></tr>`;
        }
      }
      const rest = Object.entries(byLine).filter(([n]) => !shown.has(+n)).flatMap(([, list]) => list);
      for (const i of rest) html += `<tr class="comment"><td colspan="4">${this.issueCardHTML(i, true)}</td></tr>`;
      return html + '</tbody></table>';
    },

    gotoIssue(i) {
      if (!i.file || !this.detail) return;
      const fi = this.detail.files.findIndex(f => f.new_path === i.file);
      if (fi < 0) return;
      this.collapsed[fi] = false;
      this.setTab('changes');
      setTimeout(() => {
        const el = document.getElementById(`d-${fi}-${parseInt(i.line, 10)}`) || document.getElementById(`f-${fi}`);
        if (!el) return;
        el.scrollIntoView({ block: 'center', behavior: 'smooth' });
        el.classList.remove('flash'); void el.offsetWidth; el.classList.add('flash');
      }, 120);
    },
    gotoFile(fi) {
      this.collapsed[fi] = false;
      document.getElementById(`f-${fi}`)?.scrollIntoView({ block: 'start', behavior: 'smooth' });
    },

    // ---------- settings ----------
    async loadSettings() {
      this.settings.loading = true;
      this.settings.error = '';
      try {
        const data = await this.api('/api/v1/settings');
        this.applySettingsData(data);
      } catch (e) {
        this.settings.error = e.message;
      } finally {
        this.settings.loading = false;
      }
    },
    applySettingsData(data) {
      this.settings.groups = data.groups || [];
      this.settings.items = data.settings || [];
      this.settings.dbPath = data.db_path || '';
      this.form = {};
      this.resets = {};
    },
    groupItems(gid) { return this.settings.items.filter(s => s.group === gid); },
    fieldValue(s) {
      if (s.key in this.form) return this.form[s.key];
      if (this.resets[s.key]) return s.type === 'secret' ? '' : (s.env_set ? s.env_value : s.default);
      return s.type === 'secret' ? '' : s.value;
    },
    setField(s, v) {
      if (s.type === 'secret' && v === '') { delete this.form[s.key]; return; }
      if (s.type !== 'secret' && v === s.value) delete this.form[s.key];
      else this.form[s.key] = v;
      delete this.resets[s.key];
    },
    toggleField(s) { this.setField(s, (this.fieldValue(s) === 'true') ? 'false' : 'true'); },
    resetField(s) { delete this.form[s.key]; this.resets[s.key] = true; },
    undoField(s) { delete this.form[s.key]; delete this.resets[s.key]; },
    isDirty(s) { return s.key in this.form || !!this.resets[s.key]; },
    get dirtyCount() { return Object.keys(this.form).length + Object.keys(this.resets).length; },
    sourceLabel(s) {
      if (this.resets[s.key]) return s.env_set ? 'wróci do .env' : 'wróci do domyślnej';
      return { ui: 'UI', env: '.env', default: 'domyślne' }[s.source] || s.source;
    },
    secretPlaceholder(s) { return s.is_set ? `ustawiony (${s.value}) – wpisz, aby zmienić` : 'nie ustawiono'; },
    discardSettings() { this.form = {}; this.resets = {}; },
    async saveSettings() {
      if (!this.dirtyCount) return;
      const changed = [...Object.keys(this.form), ...Object.keys(this.resets)];
      this.settings.saving = true;
      try {
        const data = await this.api('/api/v1/settings', {
          method: 'PUT', headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ values: this.form, reset: Object.keys(this.resets) }),
        });
        this.applySettingsData(data);
        this.tests = {};
        this.config = await this.api('/api/v1/config');
        if (data.restart_required?.length) this.toast('Zapisano – wymagany restart', `Zmiana ${data.restart_required.join(', ')} zadziała po ponownym uruchomieniu aplikacji.`, 'yellow', 9000);
        else this.toast('Ustawienia zapisane', `Zastosowano od razu (${changed.length} zm.)`, 'green');
        if (changed.some(k => k.startsWith('GITLAB_'))) { this.lists = {}; await this.loadProjects(true); }
      } catch (e) {
        this.toast('Nie zapisano ustawień', e.message, 'red', 9000);
      } finally {
        this.settings.saving = false;
      }
    },
    async testConnection(target) {
      if (this.dirtyCount) { this.toast('Najpierw zapisz zmiany', 'Test używa zapisanych ustawień.', 'yellow'); return; }
      this.tests[target] = { loading: true };
      try {
        const res = await this.api(`/api/v1/settings/test/${target}`, { method: 'POST' });
        this.tests[target] = { ok: res.ok, message: res.message };
      } catch (e) {
        this.tests[target] = { ok: false, message: e.message };
      }
    },
    scrollToGroup(gid) { document.getElementById('sg-' + gid)?.scrollIntoView({ block: 'start', behavior: 'smooth' }); },

    // ---------- toasts ----------
    toast(title, text, dot = 'green', ms = 5000) {
      const id = Math.random().toString(36).slice(2);
      this.toasts.push({ id, title, text, dot });
      setTimeout(() => { this.toasts = this.toasts.filter(t => t.id !== id); }, ms);
    },
  };
}
