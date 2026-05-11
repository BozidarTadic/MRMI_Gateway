'use strict';

let currentKey = localStorage.getItem('mrmi_key') || '';

function authHeaders() {
	const h = { 'Content-Type': 'application/json' };
	if (!currentKey) return h;
	if (currentKey.startsWith('ey')) {
		h['Authorization'] = 'Bearer ' + currentKey;
	} else {
		h['X-MRMI-Key'] = currentKey;
	}
	return h;
}

async function api(method, path, body) {
	const opts = { method, headers: authHeaders() };
	if (body !== undefined) opts.body = JSON.stringify(body);
	const res = await fetch(path, opts);
	if (res.status === 204) return null;
	if (!res.ok) {
		const text = await res.text();
		throw new Error(text.trim() || res.statusText);
	}
	const ct = res.headers.get('content-type') || '';
	return ct.includes('json') ? res.json() : res.text();
}

function toast(msg, isError) {
	const el = document.getElementById('toast');
	el.textContent = msg;
	el.style.borderColor = isError ? 'var(--red)' : 'var(--green)';
	el.classList.add('show');
	clearTimeout(el._t);
	el._t = setTimeout(() => el.classList.remove('show'), 3200);
}

// ── Navigation ───────────────────────────────────────────────────────────────

const PAGE_ORDER = ['status', 'audit', 'dlq', 'settings', 'apps'];

function showPage(name) {
	document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
	document.querySelectorAll('nav button').forEach(b => b.classList.remove('active'));
	const page = document.getElementById('page-' + name);
	if (page) page.classList.add('active');
	const idx = PAGE_ORDER.indexOf(name);
	const navBtns = document.querySelectorAll('nav button');
	if (idx >= 0 && navBtns[idx]) navBtns[idx].classList.add('active');
	if (name === 'status')   { loadStatus(); loadPeers(); }
	if (name === 'audit')    loadAudit();
	if (name === 'dlq')      loadDLQ();
	if (name === 'settings') loadSettings();
	if (name === 'apps')     loadApps();
}

function saveKey() {
	currentKey = document.getElementById('apiKey').value.trim();
	localStorage.setItem('mrmi_key', currentKey);
	toast('Credentials saved');
}

// ── Status page ──────────────────────────────────────────────────────────────

async function loadStatus() {
	try {
		const s = await api('GET', '/api/v1/status');
		document.getElementById('status-cards').innerHTML = [
			{ label: 'Node ID',      value: s.node_id || '—' },
			{ label: 'Region',       value: s.region || '—' },
			{ label: 'Profile',      value: s.profile || '—' },
			{ label: 'Uptime',       value: fmtUptime(s.uptime_seconds) },
			{ label: 'App Version',  value: s.app_version || '—' },
			{ label: 'Node Scope',   value: s.node_scope || '—' },
		].map(c => `<div class="card"><h3>${c.label}</h3><div class="value">${esc(c.value)}</div></div>`).join('');

		document.getElementById('node-info-box').innerHTML = `
			<table style="width:100%;border-collapse:collapse">
				${row('Applicable Law', s.applicable_law)}
				${row('Node Scope',     s.node_scope)}
				${row('ADR Version',    s.adr_version)}
				${row('App Version',    s.app_version)}
			</table>`;
	} catch (e) {
		document.getElementById('status-cards').innerHTML =
			`<div class="card"><div class="value" style="color:var(--red)">Unreachable</div><div class="sub">${esc(e.message)}</div></div>`;
	}
}

async function loadPeers() {
	try {
		const peers = await api('GET', '/api/v1/peers');
		const el = document.getElementById('peer-list');
		if (!peers || peers.length === 0) {
			el.innerHTML = '<div class="empty">No peers registered</div>';
			return;
		}
		el.innerHTML = peers.map(p => `
			<div class="peer-card">
				<div class="peer-id">${esc(p.node_id || p.addr)}</div>
				<div class="peer-meta">
					${p.addr ? `<span>${esc(p.addr)}</span>` : ''}
					${p.region ? `<span>${esc(p.region)}</span>` : ''}
					${p.node_scope ? `<span class="badge ${esc(p.node_scope)}">${esc(p.node_scope)}</span>` : ''}
					<span style="color:var(--accent2)">${esc(p.source || '')}</span>
				</div>
			</div>`).join('');
	} catch (e) {
		document.getElementById('peer-list').innerHTML = `<div class="empty">${esc(e.message)}</div>`;
	}
}

// ── Audit log page ───────────────────────────────────────────────────────────

async function loadAudit() {
	const filter = document.getElementById('audit-filter').value;
	const n = parseInt(document.getElementById('audit-limit').value) || 50;
	const tbody = document.getElementById('audit-tbody');
	tbody.innerHTML = '<tr><td colspan="7" class="empty">Loading…</td></tr>';
	try {
		let entries = await api('GET', `/api/v1/audit/latest?n=${n}`);
		if (!entries) entries = [];
		if (filter) entries = entries.filter(e => String(e.decision) === filter);
		if (entries.length === 0) {
			tbody.innerHTML = '<tr><td colspan="7" class="empty">No entries</td></tr>';
			return;
		}
		tbody.innerHTML = entries.map(e => {
			const dec = String(e.decision || '').toLowerCase().replace('/', '-');
			const badge = `<span class="badge ${dec}">${esc(e.decision || '—')}</span>`;
			const ts = e.timestamp ? new Date(e.timestamp).toLocaleTimeString() : '—';
			return `<tr>
				<td>${e.seq ?? '—'}</td>
				<td style="font-family:monospace;font-size:11px">${ts}</td>
				<td>${badge}</td>
				<td>${esc(e.sender_region || '—')}</td>
				<td>${esc(e.recipient_region || '—')}</td>
				<td>${esc(e.profile || '—')}</td>
				<td style="color:var(--text3);font-size:12px">${esc(e.reason || '')}</td>
			</tr>`;
		}).join('');
	} catch (e) {
		tbody.innerHTML = `<tr><td colspan="7" class="empty">${esc(e.message)}</td></tr>`;
	}
}

async function verifyChain() {
	try {
		const r = await api('GET', '/.well-known/mrmi-audit');
		const hash = r?.root_hash || '—';
		const sig = r?.signature ? r.signature.substring(0, 24) + '…' : 'none';
		toast(`Root hash: ${hash.substring(0, 16)}…  sig: ${sig}`);
	} catch (e) {
		toast('Verify: ' + e.message, true);
	}
}

// ── DLQ page ─────────────────────────────────────────────────────────────────

async function loadDLQ() {
	const tbody = document.getElementById('dlq-tbody');
	tbody.innerHTML = '<tr><td colspan="7" class="empty">Loading…</td></tr>';
	try {
		let entries = await api('GET', '/api/v1/dlq');
		if (!entries) entries = [];
		if (entries.length === 0) {
			tbody.innerHTML = '<tr><td colspan="7" class="empty">Dead-letter queue is empty</td></tr>';
			return;
		}
		tbody.innerHTML = entries.map(e => {
			const ts = e.first_seen_unix ? new Date(e.first_seen_unix * 1000).toLocaleString() : '—';
			return `<tr>
				<td>${e.index}</td>
				<td style="font-family:monospace;font-size:11px;max-width:180px;overflow:hidden;text-overflow:ellipsis" title="${esc(e.envelope_id || '')}">${esc(e.envelope_id || '—')}</td>
				<td>${esc(e.peer_addr || '—')}</td>
				<td>${e.attempts}</td>
				<td style="color:var(--red);max-width:160px;overflow:hidden;text-overflow:ellipsis" title="${esc(e.last_error || '')}">${esc(e.last_error || '')}</td>
				<td style="font-size:11px">${ts}</td>
				<td style="white-space:nowrap">
					<button class="btn sm" onclick="replayDLQ(${e.index})">Replay</button>
					<button class="btn sm danger" style="margin-left:4px" onclick="discardDLQ(${e.index})">Discard</button>
				</td>
			</tr>`;
		}).join('');
	} catch (e) {
		tbody.innerHTML = `<tr><td colspan="7" class="empty">${esc(e.message)}</td></tr>`;
	}
}

async function replayDLQ(index) {
	try {
		const r = await api('POST', `/api/v1/dlq/${index}/replay`);
		toast('Replayed: ' + (r?.decision || 'ok'));
		loadDLQ();
	} catch (e) {
		toast('Replay failed: ' + e.message, true);
	}
}

async function discardDLQ(index) {
	try {
		await api('POST', `/api/v1/dlq/${index}/discard`);
		toast('Entry discarded');
		loadDLQ();
	} catch (e) {
		toast('Discard failed: ' + e.message, true);
	}
}

// ── Settings page ─────────────────────────────────────────────────────────────

const PROFILE_DESC = {
	balanced:    'Balanced profile: production defaults with moderate privacy and performance tradeoffs. Suitable for most deployments.',
	strict:      'Strict profile: maximum privacy, padding, and audit. Required for GDPR / UK-GDPR / CCPA deployments.',
	performance: 'Performance profile: minimal overhead, no deduplication or padding. Not for production use with real user data.',
};

let allowTags = [];
let denyTags  = [];
let staticPeers = [];
let prevProfile = '';

async function loadSettings() {
	try {
		const s = await api('GET', '/api/v1/status');
		const profile = s.profile || 'balanced';
		const sel = document.getElementById('profile-select');
		sel.value = profile;
		prevProfile = profile;
		onProfileChange();
	} catch (_) {}
	renderAllowTags();
	renderDenyTags();
	renderStaticPeers();
}

function onProfileChange() {
	const profile = document.getElementById('profile-select').value;
	document.getElementById('profile-info').textContent = PROFILE_DESC[profile] || '';
	const warn = document.getElementById('strict-warn');
	warn.style.display = (prevProfile === 'strict' && profile !== 'strict') ? 'block' : 'none';
}

function renderTags(id, tags, type) {
	document.getElementById(id).innerHTML = tags.map((t, i) =>
		`<span class="tag">${esc(t)}<button onclick="removeTag('${type}',${i})">×</button></span>`
	).join('');
}
function renderAllowTags() { renderTags('allow-tags', allowTags, 'allow'); }
function renderDenyTags()  { renderTags('deny-tags',  denyTags,  'deny');  }

function addTag(type) {
	const id  = type + '-input';
	const val = document.getElementById(id).value.trim().toUpperCase();
	if (!val) return;
	if (type === 'allow') { if (!allowTags.includes(val)) allowTags.push(val); renderAllowTags(); }
	else                  { if (!denyTags.includes(val))  denyTags.push(val);  renderDenyTags();  }
	document.getElementById(id).value = '';
}

function removeTag(type, i) {
	if (type === 'allow') { allowTags.splice(i, 1); renderAllowTags(); }
	else                  { denyTags.splice(i, 1);  renderDenyTags();  }
}

function renderStaticPeers() {
	const el = document.getElementById('static-peers-list');
	if (staticPeers.length === 0) {
		el.innerHTML = '<div style="color:var(--text3);font-size:12px;margin-bottom:8px">No static peers configured</div>';
		return;
	}
	el.innerHTML = staticPeers.map((p, i) =>
		`<div style="display:flex;align-items:center;justify-content:space-between;padding:8px;background:var(--bg3);border-radius:6px;margin-bottom:6px">
			<div>
				<div style="font-size:13px">${esc(p.addr)}</div>
				<div style="font-size:11px;color:var(--text3)">${esc(p.region || '')} · ${esc(p.node_scope || '')}</div>
			</div>
			<button class="btn sm danger" onclick="removePeer(${i})">Remove</button>
		</div>`
	).join('');
}

function addPeer() {
	const addr  = document.getElementById('new-peer-addr').value.trim();
	const scope = document.getElementById('new-peer-scope').value.trim();
	const region = document.getElementById('new-peer-region').value.trim();
	if (!addr) { toast('Peer address is required', true); return; }
	staticPeers.push({ addr, node_scope: scope, region });
	renderStaticPeers();
	['new-peer-addr', 'new-peer-scope', 'new-peer-region'].forEach(id => {
		document.getElementById(id).value = '';
	});
}

function removePeer(i) {
	staticPeers.splice(i, 1);
	renderStaticPeers();
}

async function saveSettings() {
	const profile    = document.getElementById('profile-select').value;
	const minTrust   = parseInt(document.getElementById('min-trust').value) || 0;
	const isolation  = document.getElementById('app-isolation').value;
	const autoAccept = document.getElementById('auto-accept').value;

	const cfg = {
		Profile: { Name: profile },
		Policy: {
			Outbound: {
				AllowTo: allowTags.length > 0 ? allowTags : [],
				DenyTo:  denyTags.length  > 0 ? denyTags  : [],
			},
			Inbound:   { MinTrustTier: minTrust },
			Discovery: { AppIsolation: isolation },
			Connect:   { AutoAccept: autoAccept },
		},
	};

	if (staticPeers.length > 0) {
		cfg.Network = { Peers: {} };
		staticPeers.forEach((p, i) => {
			cfg.Network.Peers['peer_' + i] = { Addr: p.addr, NodeScope: p.node_scope, Region: p.region };
		});
	}

	try {
		await api('PUT', '/api/v1/config', cfg);
		prevProfile = profile;
		document.getElementById('strict-warn').style.display = 'none';
		toast('Settings saved and policy reloaded');
	} catch (e) {
		toast('Save failed: ' + e.message, true);
	}
}

async function exportTOML() {
	let s;
	try { s = await api('GET', '/api/v1/status'); } catch (_) { s = {}; }
	const profile    = document.getElementById('profile-select').value;
	const minTrust   = document.getElementById('min-trust').value;
	const isolation  = document.getElementById('app-isolation').value;
	const autoAccept = document.getElementById('auto-accept').value;

	const q = v => JSON.stringify(v);
	let toml = [
		`[node]`,
		`node_id = ${q(s.node_id || '')}`,
		`region = ${q(s.region || '')}`,
		`node_scope = ${q(s.node_scope || 'regional')}`,
		`applicable_law = ${q(s.applicable_law || 'NONE')}`,
		``,
		`[profile]`,
		`name = ${q(profile)}`,
		``,
		`[policy.outbound]`,
		`allow_to = [${allowTags.map(q).join(', ')}]`,
		`deny_to = [${denyTags.map(q).join(', ')}]`,
		``,
		`[policy.inbound]`,
		`min_trust_tier = ${minTrust}`,
		``,
		`[policy.discovery]`,
		`app_isolation = ${q(isolation)}`,
		``,
		`[policy.connect]`,
		`auto_accept = ${q(autoAccept)}`,
		``,
	].join('\n');

	staticPeers.forEach((p, i) => {
		toml += `[peers.peer_${i}]\n`;
		toml += `addr = ${q(p.addr)}\n`;
		toml += `node_scope = ${q(p.node_scope || 'regional')}\n`;
		toml += `region = ${q(p.region || '')}\n\n`;
	});

	toml += `# api_key and jwt_secret are redacted\n[api]\nhttp_listen = ":8080"\n`;

	document.getElementById('toml-preview').value = toml;

	const blob = new Blob([toml], { type: 'text/plain' });
	const url  = URL.createObjectURL(blob);
	const a    = document.createElement('a');
	a.href = url; a.download = 'config.toml'; a.click();
	URL.revokeObjectURL(url);
	toast('config.toml downloaded');
}

// ── Apps page ─────────────────────────────────────────────────────────────────

async function loadApps() {
	const tbody = document.getElementById('apps-tbody');
	tbody.innerHTML = '<tr><td colspan="4" class="empty">Loading…</td></tr>';
	try {
		let apps = await api('GET', '/api/v1/apps');
		if (!apps) apps = [];
		if (apps.length === 0) {
			tbody.innerHTML = '<tr><td colspan="4" class="empty">No apps registered</td></tr>';
			return;
		}
		tbody.innerHTML = apps.map(a => {
			const ts = a.last_seen ? new Date(a.last_seen * 1000).toLocaleString() : '—';
			return `<tr>
				<td style="font-weight:600">${esc(a.app_id)}</td>
				<td style="font-family:monospace;font-size:11px">${esc(a.webhook_url || '—')}</td>
				<td style="font-size:11px">${ts}</td>
				<td><button class="btn sm danger" onclick="deleteApp('${esc(a.app_id)}')">Delete</button></td>
			</tr>`;
		}).join('');
	} catch (e) {
		tbody.innerHTML = `<tr><td colspan="4" class="empty">${esc(e.message)}</td></tr>`;
	}
}

async function registerApp() {
	const appID   = document.getElementById('new-app-id').value.trim();
	const webhook = document.getElementById('new-app-webhook').value.trim();
	const secret  = document.getElementById('new-app-secret').value.trim();
	if (!appID) { toast('App ID is required', true); return; }
	try {
		const r = await api('POST', '/api/v1/apps/register', {
			app_id: appID, webhook_url: webhook, webhook_secret: secret,
		});
		const resultEl = document.getElementById('new-app-result');
		resultEl.textContent = `Registered — API key: ${r.api_key}`;
		['new-app-id', 'new-app-webhook', 'new-app-secret'].forEach(id => {
			document.getElementById(id).value = '';
		});
		toast('App registered');
		loadApps();
	} catch (e) {
		toast('Register failed: ' + e.message, true);
	}
}

async function deleteApp(appID) {
	try {
		await api('DELETE', `/api/v1/apps/${encodeURIComponent(appID)}`);
		toast(`App "${appID}" deleted`);
		loadApps();
	} catch (e) {
		toast('Delete failed: ' + e.message, true);
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

function fmtUptime(s) {
	if (!s) return '—';
	const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), sec = s % 60;
	if (h > 0) return `${h}h ${m}m`;
	if (m > 0) return `${m}m ${sec}s`;
	return `${sec}s`;
}

function esc(s) {
	return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

function row(label, val) {
	return `<tr><td style="color:var(--text3);padding:6px 0;width:130px">${esc(label)}</td><td style="color:var(--text)">${esc(val || '—')}</td></tr>`;
}

// ── Init ──────────────────────────────────────────────────────────────────────

document.getElementById('apiKey').value = currentKey;
loadStatus();
loadPeers();
