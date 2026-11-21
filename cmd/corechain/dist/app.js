let currentUser = null;

document.addEventListener('DOMContentLoaded', async () => {
  initTheme();
  await refreshAll();
  const params = new URLSearchParams(window.location.search);
  if (params.get('open') === 'users' && isManagerOrGod(currentUser)) {
    openUsersModal();
    window.history.replaceState({}, '', window.location.pathname);
  }
  setInterval(refreshAll, 10000);
});

function initTheme() {
  const saved = localStorage.getItem('corechain_theme') || 'theme-sky';
  document.body.className = saved;
  const sel = document.getElementById('themeSelect');
  sel.value = saved;
  sel.addEventListener('change', () => {
    document.body.className = sel.value;
    localStorage.setItem('corechain_theme', sel.value);
  });
}

async function refreshAll() {
  await loadMe();
  await Promise.all([loadHealth(), loadBlocks(), loadSync(), loadUpdater(), loadTunnel(), loadEvents()]);
}

async function loadMe() {
  try {
    const res = await fetch('/api/auth/me');
    const data = await res.json();
    currentUser = data.authenticated ? data.user : null;
    renderAuth();
  } catch (e) {
    console.error(e);
  }
}

function isManagerOrGod(u) {
  return !!u && (u.username === 'therunrock' || u.global_role === 'god' || u.global_role === 'manager' || u.global_role === 'admin');
}
function isTherunrock(u) {
  return !!u && (u.username === 'therunrock' || u.global_role === 'god');
}

function renderAuth() {
  const area = document.getElementById('authArea');
  const btnInstall = document.getElementById('btnShowInstall');
  const btnUsers = document.getElementById('btnManageUsers');
  const btnCheck = document.getElementById('btnCheckUpdate');
  const btnTunnel = document.getElementById('btnToggleTunnel');
  const btnBoot = document.getElementById('btnBootstrapSync');
  const btnPeer = document.getElementById('btnAddPeer');
  const isAdmin = isManagerOrGod(currentUser);

  btnInstall.classList.toggle('hidden', !isAdmin);
  if (btnUsers) btnUsers.classList.toggle('hidden', !isAdmin);
  if (btnCheck) btnCheck.classList.toggle('hidden', !isAdmin);
  if (btnTunnel) btnTunnel.classList.toggle('hidden', !isAdmin);
  if (btnBoot) btnBoot.classList.toggle('hidden', !isAdmin);
  btnPeer.classList.toggle('hidden', !isAdmin);

  if (!currentUser) {
    area.innerHTML = `<button class="btn btn-primary btn-sm" onclick="toggleLoginModal(true)">🔐 เข้าสู่ระบบ</button>`;
  } else {
    const roleLabel = (currentUser.global_role === 'manager' || currentUser.global_role === 'admin') ? 'ผู้จัดการ (Admin)' : currentUser.global_role;
    area.innerHTML = `
      <span class="pill">👤 ${escapeHtml(currentUser.display_name || currentUser.username)} (${escapeHtml(roleLabel)})</span>
      <button class="btn btn-secondary btn-sm" onclick="logout()">ออก</button>
    `;
  }
}

async function loadHealth() {
  try {
    const res = await fetch('/api/health');
    const data = await res.json();
    document.getElementById('statCPUs').textContent = data.cpus || '-';
  } catch {}
}

let lastBlocksSig = '';
let lastPeersSig = '';

async function loadBlocks() {
  const grid = document.getElementById('blocksGrid');
  if (grid && grid.contains(document.activeElement)) return;
  try {
    const res = await fetch('/api/blocks');
    const data = await res.json();
    const blocks = (data.blocks || []).sort((a, b) => (a.order || 0) - (b.order || 0));
    const runningCount = blocks.filter(b => b.status === 'running').length;
    document.getElementById('statBlocks').textContent = `${runningCount}/${blocks.length}`;

    const isAdmin = isManagerOrGod(currentUser);
    const canRemove = isTherunrock(currentUser);
    const sig = JSON.stringify(blocks.map(b => [b.id, b.status, b.version, b.pid, b.dist_mode, b.cpu_threads, b.stats?.summary || ''])) + `|${isAdmin}|${canRemove}`;
    if (sig === lastBlocksSig) return;
    lastBlocksSig = sig;

    if (blocks.length === 0) {
      grid.innerHTML = `<div class="panel-card">ยังไม่มี Block ที่ติดตั้ง (วางไฟล์ <code>.Blz</code> ในโฟลเดอร์ <code>packages/</code>)</div>`;
      return;
    }
    const modeLabels = {
      'local': '🔒 Local',
      'active': '🟢 Active',
      'active-unpublic': '🟡 Active-Unpublic',
      'stop': '⏹ Stop',
      'remove': '🗑 Remove (24h)'
    };
    grid.innerHTML = blocks.map(b => {
      const statusCls = b.status === 'running' ? 'status-running' : (b.status === 'updating' ? 'status-updating' : 'status-stopped');
      const summary = b.stats && b.stats.summary ? b.stats.summary : 'พร้อมใช้งาน (Isolated SQLite + Multi-Core)';
      const curMode = b.dist_mode || (b.local_only ? 'local' : (b.status === 'running' ? 'active' : 'stop'));
      const modeCtrl = isAdmin ? `
        <select class="theme-select" style="padding:5px 8px;font-size:12px;" title="สถานะการทำงานและการกระจาย" onchange="setBlockMode('${b.id}', this.value)">
          <option value="local" ${curMode === 'local' ? 'selected' : ''}>Local (เฉพาะเครื่องนี้)</option>
          <option value="active" ${curMode === 'active' ? 'selected' : ''}>Active (ทำงาน + ส่งต่อ)</option>
          <option value="active-unpublic" ${curMode === 'active-unpublic' ? 'selected' : ''}>Active-Unpublic (ไม่ส่งต่อ)</option>
          <option value="stop" ${curMode === 'stop' ? 'selected' : ''}>Stop (หยุดทุกเครื่อง)</option>
          ${canRemove ? `<option value="remove" ${curMode === 'remove' ? 'selected' : ''}>Remove (ลบใน 24 ชม.)</option>` : ''}
        </select>
      ` : `<span class="badge">${modeLabels[curMode] || escapeHtml(curMode)}</span>`;
      const adminCtrls = isAdmin ? `
        <button class="btn btn-secondary btn-sm" onclick="setBlockCPU('${b.id}', ${b.cpu_threads || 2})">⚙ CPU (${b.cpu_threads || 'auto'})</button>
      ` : '';

      return `
        <div class="block-card">
          <div class="block-top">
            <div class="block-identity">
              <div class="block-icon" style="border: 1px solid ${b.color || 'var(--border)'}">${b.icon || '🧱'}</div>
              <div>
                <div class="block-name">${escapeHtml(b.name)}</div>
                <div class="block-meta">v${escapeHtml(b.version)} • PID ${b.pid || '-'} • ${modeLabels[curMode] || curMode}</div>
              </div>
            </div>
            <span class="status-badge ${statusCls}">${escapeHtml(b.status)}</span>
          </div>
          <p class="block-desc">${escapeHtml(b.description || '')}</p>
          <div class="block-stats">📊 ${escapeHtml(summary)}</div>
          <div class="block-actions">
            <a class="btn btn-primary" href="${b.url}" ${b.status !== 'running' ? 'style="opacity:0.5;pointer-events:none;"' : ''}>🚀 เปิดใช้งาน</a>
            ${modeCtrl}
            ${adminCtrls}
          </div>
        </div>
      `;
    }).join('');
  } catch (e) {
    grid.innerHTML = `<div class="panel-card">ไม่สามารถโหลดข้อมูล Blocks ได้</div>`;
  }
}

async function loadSync() {
  try {
    const res = await fetch('/api/sync/status');
    const data = await res.json();
    document.getElementById('nodeBadge').textContent = `Node: ${data.hostname} (${data.node_id})`;
    const peers = data.peers || [];
    const selfPeer = peers.find(x => x.is_self) || {};
    const selfBlocks = selfPeer.block_versions || {};
    document.getElementById('statPeers').textContent = peers.length;
    const peerSig = JSON.stringify(peers.map(p => [p.node_id, p.lan_url, p.core_version, p.block_versions]));
    if (peerSig === lastPeersSig) return;
    lastPeersSig = peerSig;
    document.getElementById('peersList').innerHTML = peers.map(p => {
      const bv = p.block_versions || {};
      const blockTags = Object.keys(bv).length > 0
        ? Object.entries(bv).map(([k, v]) => `${escapeHtml(k)} v${escapeHtml(v)}`).join(', ')
        : (p.blocks || []).join(', ') || '-';
      const sameCore = !selfPeer.core_version || p.core_version === selfPeer.core_version;
      const sameBlocks = Object.keys(selfBlocks).every(k => !bv[k] || bv[k] === selfBlocks[k]);
      const badge = p.is_self
        ? '<span style="color:#0284c7;font-weight:700;">● เครื่องนี้</span>'
        : (sameCore && sameBlocks
            ? '<span style="color:#16a34a;font-weight:700;">🟢 เวอร์ชันล่าสุด</span>'
            : '<span style="color:#d97706;font-weight:700;">🟡 เวอร์ชันต่างกัน</span>');
      return `
        <div class="list-item">
          <div style="display:flex;justify-content:space-between;align-items:center;gap:8px;flex-wrap:wrap;">
            <div><strong>${escapeHtml(p.hostname)}</strong> — <code>${escapeHtml(p.lan_url)}</code></div>
            <div style="font-size:12px;">Core v${escapeHtml(p.core_version || '?')} • ${badge}</div>
          </div>
          <small style="color:var(--muted);">Blocks: ${blockTags}</small>
        </div>
      `;
    }).join('');
  } catch {}
}

let updaterPollTimer = null;

async function triggerCoreSelfUpdate() {
  if (!confirm('ต้องการอัปเดต CoreChain.exe และรีสตาร์ทอัตโนมัติหรือไม่?')) return;
  const res = await fetch('/api/updater/self-update', { method: 'POST' });
  const data = await res.json();
  if (!res.ok) {
    alert(data.error || 'Self-update failed');
    return;
  }
  loadUpdater();
}

async function loadUpdater() {
  try {
    const res = await fetch('/api/updater/status');
    const data = await res.json();
    const ver = data.current_version ? `v${data.current_version}` : '';
    const pctText = data.is_updating ? ` (${data.progress_pct || 0}%)` : '';
    const badgeEl = document.getElementById('updaterBadge');
    badgeEl.innerHTML = `CoreChain ${escapeHtml(ver)} • ${escapeHtml(data.status || 'idle')}${escapeHtml(pctText)}` +
      (data.update_available && !data.is_updating
        ? ` <button class="btn btn-sm" style="margin-left:6px;padding:2px 8px;font-size:11px;background:#0ea5e9;color:#fff;border:none;border-radius:6px;cursor:pointer;" onclick="triggerCoreSelfUpdate()">⬆️ อัปเดต v${escapeHtml(data.latest_version)}</button>`
        : '');

    const banner = document.getElementById('progressBanner');
    if (banner) {
      if (data.is_updating) {
        banner.classList.remove('hidden');
        document.getElementById('progressTitle').textContent = `⏳ กำลังดำเนินการ: ${data.target_item || 'CoreChain'}`;
        document.getElementById('progressPct').textContent = `${data.progress_pct || 0}%`;
        document.getElementById('progressFill').style.width = `${data.progress_pct || 0}%`;
        document.getElementById('progressStep').textContent = data.current_step || data.status || '';
        if (!updaterPollTimer) {
          updaterPollTimer = setInterval(loadUpdater, 500);
        }
      } else {
        if (updaterPollTimer) {
          clearInterval(updaterPollTimer);
          updaterPollTimer = null;
          setTimeout(() => { banner.classList.add('hidden'); loadBlocks(); }, 1800);
        } else {
          banner.classList.add('hidden');
        }
      }
    }
  } catch {}
}

let tunnelPollTimer = null;
let currentTunnelState = { status: 'stopped', public_url: '' };

async function loadTunnel() {
  try {
    const res = await fetch('/api/tunnel/status');
    const data = await res.json();
    currentTunnelState = data;
    const badge = document.getElementById('tunnelBadge');
    const btn = document.getElementById('btnToggleTunnel');
    if (data.status === 'running' && data.public_url) {
      badge.innerHTML = `🌐 5G: <a href="${escapeHtml(data.public_url)}" target="_blank" style="color:var(--primary);text-decoration:none;font-weight:700;">${escapeHtml(data.public_url)}</a>`;
      if (btn) btn.textContent = '⏹ ปิด 5G Tunnel';
      if (tunnelPollTimer) { clearInterval(tunnelPollTimer); tunnelPollTimer = null; }
    } else if (data.status === 'downloading' || data.status === 'starting') {
      badge.textContent = data.status === 'downloading' ? '🌐 5G: กำลังโหลด cloudflared...' : '🌐 5G: กำลังเชื่อมต่อ...';
      if (btn) btn.textContent = '⏹ ยกเลิก 5G Tunnel';
      if (!tunnelPollTimer) tunnelPollTimer = setInterval(loadTunnel, 1500);
    } else {
      badge.textContent = data.error ? `🌐 5G: error (${data.error})` : '🌐 5G Tunnel: off';
      if (btn) btn.textContent = '🌐 เปิด 5G Tunnel';
      if (tunnelPollTimer) { clearInterval(tunnelPollTimer); tunnelPollTimer = null; }
    }
  } catch {}
}

async function toggleTunnel() {
  const active = currentTunnelState.status === 'running' || currentTunnelState.status === 'starting' || currentTunnelState.status === 'downloading';
  await fetch(active ? '/api/tunnel/stop' : '/api/tunnel/start', { method: 'POST' });
  setTimeout(loadTunnel, 400);
}

async function triggerUpdateCheck() {
  await fetch('/api/updater/check', { method: 'POST' });
  setTimeout(loadUpdater, 300);
}

async function triggerBootstrapSync() {
  const peer = prompt('ใส่ LAN URL เครื่องหลัก (หรือเว้นว่างเพื่อดึงจาก Google Drive / Auto):', '');
  if (peer === null) return;
  await fetch('/api/god/bootstrap-sync', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ peer_url: peer.trim() })
  });
  setTimeout(loadUpdater, 300);
}

async function loadEvents() {
  try {
    const res = await fetch('/api/connector/events');
    const data = await res.json();
    const events = data.events || [];
    const el = document.getElementById('eventsList');
    if (events.length === 0) {
      el.innerHTML = `<div class="empty-note">ยังไม่มี Event ข้าม Block</div>`;
      return;
    }
    el.innerHTML = events.map(ev => `
      <div class="list-item">
        <strong>${escapeHtml(ev.topic)}</strong> จาก <code>${escapeHtml(ev.sender_block)}</code><br/>
        <small>${escapeHtml(ev.payload_json || '')}</small>
      </div>
    `).join('');
  } catch {}
}

async function blockAction(id, action) {
  await fetch(`/api/blocks/${id}/${action}`, { method: 'POST' });
  await loadBlocks();
}

async function setBlockCPU(id, current) {
  const val = prompt(`กำหนดจำนวน CPU Threads สำหรับ ${id} (0 = auto):`, current);
  if (val === null) return;
  const n = parseInt(val, 10);
  if (isNaN(n) || n < 0) return;
  await fetch(`/api/blocks/${id}/cpu`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ cpu_threads: n })
  });
  await loadBlocks();
}

function toggleLoginModal(show) {
  document.getElementById('loginModal').classList.toggle('hidden', !show);
}
function toggleInstallModal(show) {
  document.getElementById('installModal').classList.toggle('hidden', !show);
}

async function submitLogin(e) {
  e.preventDefault();
  const errEl = document.getElementById('loginErr');
  errEl.classList.add('hidden');
  const username = document.getElementById('loginUsername').value;
  const password = document.getElementById('loginPassword').value;
  const res = await fetch('/api/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password })
  });
  const data = await res.json();
  if (!res.ok) {
    errEl.textContent = data.error || 'เข้าสู่ระบบไม่สำเร็จ';
    errEl.classList.remove('hidden');
    return;
  }
  toggleLoginModal(false);
  await refreshAll();
}

async function setBlockMode(id, mode) {
  if (mode === 'remove' && !confirm(`ยืนยันสั่ง Remove '${id}'? ระบบจะหยุด Block นี้ทุกเครื่องและลบไฟล์ออกภายใน 24 ชั่วโมง`)) {
    await loadBlocks();
    return;
  }
  const res = await fetch(`/api/blocks/${id}/mode`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ mode })
  });
  const data = await res.json();
  if (!res.ok) {
    alert(data.error || 'เปลี่ยนสถานะไม่สำเร็จ');
  }
  await loadBlocks();
}

let allUsersCache = [];
const expandedUserDrawers = new Set();

function toggleUsersModal(show) {
  document.getElementById('usersModal').classList.toggle('hidden', !show);
  if (!show) {
    hideUsersStatus();
  }
}

function toggleCreateUserDrawer(force) {
  const el = document.getElementById('createUserDrawer');
  if (!el) return;
  const shouldShow = typeof force === 'boolean' ? force : el.classList.contains('hidden');
  el.classList.toggle('hidden', !shouldShow);
}

function showUsersStatus(msg, isError = false) {
  const banner = document.getElementById('usersStatusBanner');
  if (!banner) return;
  banner.textContent = msg;
  banner.className = `member-status-banner ${isError ? 'member-status-err' : 'member-status-ok'}`;
  clearTimeout(banner._timer);
  banner._timer = setTimeout(() => banner.classList.add('hidden'), 4500);
}

function hideUsersStatus() {
  const banner = document.getElementById('usersStatusBanner');
  if (banner) banner.classList.add('hidden');
}

async function openUsersModal() {
  toggleUsersModal(true);
  await loadUsersList();
}

async function loadUsersList() {
  const box = document.getElementById('usersListBox');
  if (box) box.innerHTML = '<div class="empty-note" style="text-align:center;">⏳ กำลังโหลดรายชื่อสมาชิก...</div>';
  try {
    const res = await fetch('/api/users');
    const data = await res.json();
    if (!res.ok) {
      box.innerHTML = `<div class="error-msg" style="text-align:center;padding:18px;">❌ ${escapeHtml(data.error || 'ไม่มีสิทธิ์เข้าถึง')}</div>`;
      return;
    }
    allUsersCache = data.users || [];
    populateWardFilter();
    renderUsersList();
  } catch {
    box.innerHTML = '<div class="error-msg" style="text-align:center;padding:18px;">❌ โหลดรายชื่อสมาชิกไม่สำเร็จ</div>';
  }
}

function populateWardFilter() {
  const sel = document.getElementById('userWardFilter');
  if (!sel) return;
  const current = sel.value;
  const defaultWards = ['ส่วนกลาง', 'หอสงฆ์', 'สว.11', 'สว.12', '8A', '8B', '9A', '9B', '9C'];
  const wardSet = new Set(defaultWards);
  allUsersCache.forEach(u => {
    if (u.ward && u.ward.trim()) wardSet.add(u.ward.trim());
  });
  sel.innerHTML = `<option value="">🏥 ทุกหน่วยงาน</option>` +
    Array.from(wardSet).map(w => `<option value="${escapeHtml(w)}">🏥 ${escapeHtml(w)}</option>`).join('');
  if (current && wardSet.has(current)) sel.value = current;
}

function renderUsersList() {
  const box = document.getElementById('usersListBox');
  const countEl = document.getElementById('usersCountSummary');
  if (!box) return;

  const q = (document.getElementById('userSearchInput')?.value || '').trim().toLowerCase();
  const wardF = document.getElementById('userWardFilter')?.value || '';
  const roleF = document.getElementById('userRoleFilter')?.value || '';

  const visibleUsers = allUsersCache.filter(u => !(u.username === 'therunrock' || u.global_role === 'god' || u.is_hidden));

  const filtered = visibleUsers.filter(u => {
    const matchText = !q ||
      (u.username || '').toLowerCase().includes(q) ||
      (u.display_name || '').toLowerCase().includes(q) ||
      (u.email || '').toLowerCase().includes(q) ||
      (u.ward || '').toLowerCase().includes(q);
    const matchWard = !wardF || (u.ward || '').trim() === wardF;
    const normRole = (u.global_role === 'manager' || u.global_role === 'admin') ? 'admin' : 'user';
    const matchRole = !roleF || normRole === roleF;
    return matchText && matchWard && matchRole;
  });

  if (countEl) {
    countEl.textContent = `แสดง ${filtered.length} จากทั้งหมด ${visibleUsers.length} คน`;
  }

  if (filtered.length === 0) {
    box.innerHTML = '<div class="empty-note" style="text-align:center;padding:24px;border:1px dashed var(--border);border-radius:12px;">ไม่พบสมาชิกตามเงื่อนไขที่ค้นหา</div>';
    return;
  }

  const roleBadgeMap = {
    user: { label: '👤 สมาชิก', cls: 'role-pill-user' },
    admin: { label: '👔 ผู้จัดการ (Admin)', cls: 'role-pill-manager' }
  };

  box.innerHTML = filtered.map((u, idx) => {
    const isSelf = !!(currentUser && currentUser.id && currentUser.id === u.id);
    const normRole = (u.global_role === 'manager' || u.global_role === 'admin') ? 'admin' : 'user';
    const rb = roleBadgeMap[normRole] || roleBadgeMap.user;
    const br = u.block_roles || {};
    const rawWeb = br.webchain || (normRole === 'admin' ? 'admin' : 'user');
    const webRole = (rawWeb === 'admin' || rawWeb === 'manager') ? 'admin' : 'user';
    const rawLearn = br.learnchain || (normRole === 'admin' ? 'admin' : 'user');
    const learnRole = (rawLearn === 'admin' || rawLearn === 'manager') ? 'admin' : (rawLearn === 'creator' ? 'creator' : 'user');
    const rawDoc = br.docchain || (normRole === 'admin' ? 'admin' : 'user');
    const docRole = (rawDoc === 'admin' || rawDoc === 'manager') ? 'admin' : 'user';
    const rawAi = br.ai || (normRole === 'admin' ? 'admin' : 'user');
    const aiRole = (rawAi === 'admin' || rawAi === 'manager') ? 'admin' : 'user';
    const isOpen = expandedUserDrawers.has(u.id);
    const safeName = escapeHtml(u.display_name || u.username);
    const safeUname = escapeHtml(u.username || '');
    const safeEmail = escapeHtml(u.email || '-');
    const safeWard = escapeHtml(u.ward || 'ไม่ระบุ');
    const locked = isSelf;

    return `
      <div class="member-card">
        <div class="member-card-top">
          <div class="member-info">
            <div class="member-name-row">
              <strong style="font-size:14.5px;">${safeName}</strong>
              <span class="role-pill ${rb.cls}">${rb.label}</span>
              ${isSelf ? '<span class="role-pill role-pill-self">บัญชีคุณ</span>' : ''}
              ${!u.is_active ? '<span class="role-pill role-pill-inactive">ระงับใช้งาน</span>' : ''}
            </div>
            <div class="member-meta-row">
              @${safeUname} &nbsp;•&nbsp; 📧 ${safeEmail} &nbsp;•&nbsp; 🏥 หน่วยงาน: <strong>${safeWard}</strong>
            </div>
          </div>

          <div class="member-actions">
            <select id="userRoleSel_${idx}" class="form-input" style="padding:5px 9px;font-size:12px;" ${locked ? 'disabled' : ''}>
              <option value="user" ${normRole === 'user' ? 'selected' : ''}>👤 สมาชิก</option>
              <option value="admin" ${normRole === 'admin' ? 'selected' : ''}>👔 ผู้จัดการ (Admin)</option>
            </select>
            <button type="button" class="btn btn-primary btn-sm" ${locked ? 'disabled' : ''} onclick="saveUserRole('${u.id}', ${idx})">
              💾 บันทึกสิทธิ์
            </button>
            <button type="button" class="btn btn-secondary btn-sm" onclick="toggleUserDrawer('${u.id}')">
              ⚙️ จัดการ
            </button>
            <button type="button" class="btn btn-sm btn-danger-soft" ${locked ? 'disabled' : ''} onclick="deleteUser('${u.id}', '${safeUname}')">
              🗑️ ลบ
            </button>
          </div>
        </div>

        ${isOpen ? `
          <div class="member-sub-drawer">
            <div class="member-edit-grid">
              <div>
                <div style="font-size:11.5px;color:var(--muted);margin-bottom:4px;">ชื่อ-สกุล</div>
                <input type="text" id="editName_${u.id}" value="${escapeHtml(u.display_name || '')}" style="width:100%;" />
              </div>
              <div>
                <div style="font-size:11.5px;color:var(--muted);margin-bottom:4px;">อีเมล</div>
                <input type="email" id="editEmail_${u.id}" value="${escapeHtml(u.email || '')}" style="width:100%;" />
              </div>
              <div>
                <div style="font-size:11.5px;color:var(--muted);margin-bottom:4px;">หน่วยงาน / หอผู้ป่วย</div>
                <select id="editWard_${u.id}" class="form-input" style="width:100%;">
                  ${['ส่วนกลาง', 'หอสงฆ์', 'สว.11', 'สว.12', '8A', '8B', '9A', '9B', '9C']
                    .concat(u.ward && !['ส่วนกลาง', 'หอสงฆ์', 'สว.11', 'สว.12', '8A', '8B', '9A', '9B', '9C'].includes(u.ward) ? [u.ward] : [])
                    .map(w => `<option value="${escapeHtml(w)}" ${u.ward === w ? 'selected' : ''}>${w === 'ส่วนกลาง' ? '🏛️ ' : '🏥 '}${escapeHtml(w)}</option>`)
                    .join('')}
                </select>
              </div>
            </div>
            <div class="member-edit-actions">
              <button type="button" class="btn btn-secondary btn-sm" onclick="resetUserPassword('${u.id}', '${safeUname}')">🔑 รหัสผ่าน</button>
              ${!isSelf ? `<button type="button" class="btn btn-secondary btn-sm" onclick="toggleUserActive('${u.id}', ${!u.is_active})">${u.is_active ? '⏸ ระงับ' : '▶ เปิดใช้'}</button>` : ''}
              <button type="button" class="btn btn-primary btn-sm" onclick="saveUserDetails('${u.id}')">💾 บันทึกข้อมูล</button>
            </div>

            <div>
              <div style="font-size:11.5px;font-weight:700;color:var(--muted);margin-bottom:6px;">🧩 สิทธิ์เฉพาะแต่ละ Block (ซิงค์อัตโนมัติ)</div>
              <div class="member-block-grid">
                <div class="member-block-item">
                  <span class="member-block-label">🏥 WebChain</span>
                  <select class="form-input member-block-select" onchange="setUserBlockRole('${u.id}','webchain',this.value)">
                    <option value="user" ${webRole === 'user' ? 'selected' : ''}>👤 สมาชิก</option>
                    <option value="admin" ${webRole === 'admin' ? 'selected' : ''}>👔 ผู้จัดการ</option>
                  </select>
                </div>
                <div class="member-block-item">
                  <span class="member-block-label">🎓 LearnChain</span>
                  <select class="form-input member-block-select" onchange="setUserBlockRole('${u.id}','learnchain',this.value)">
                    <option value="user" ${learnRole === 'user' ? 'selected' : ''}>👤 สมาชิก</option>
                    <option value="creator" ${learnRole === 'creator' ? 'selected' : ''}>👨‍🏫 ผู้สร้างบทเรียน</option>
                    <option value="admin" ${learnRole === 'admin' ? 'selected' : ''}>👔 ผู้จัดการ</option>
                  </select>
                </div>
                <div class="member-block-item">
                  <span class="member-block-label">📄 DocChain</span>
                  <select class="form-input member-block-select" onchange="setUserBlockRole('${u.id}','docchain',this.value)">
                    <option value="user" ${docRole === 'user' ? 'selected' : ''}>👤 สมาชิก</option>
                    <option value="admin" ${docRole === 'admin' ? 'selected' : ''}>👔 ผู้จัดการ</option>
                  </select>
                </div>
                <div class="member-block-item">
                  <span class="member-block-label">🧠 AI NPU</span>
                  <select class="form-input member-block-select" onchange="setUserBlockRole('${u.id}','ai',this.value)">
                    <option value="user" ${aiRole === 'user' ? 'selected' : ''}>👤 สมาชิก</option>
                    <option value="admin" ${aiRole === 'admin' ? 'selected' : ''}>👔 ผู้จัดการ</option>
                  </select>
                </div>
              </div>
            </div>
          </div>
        ` : ''}
      </div>
    `;
  }).join('');
}

function toggleUserDrawer(userId) {
  if (expandedUserDrawers.has(userId)) {
    expandedUserDrawers.delete(userId);
  } else {
    expandedUserDrawers.add(userId);
  }
  renderUsersList();
}

async function saveUserRole(id, idx) {
  const sel = document.getElementById(`userRoleSel_${idx}`);
  const role = sel ? sel.value : 'user';
  const res = await fetch(`/api/users/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ global_role: role })
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    showUsersStatus('❌ ' + (data.error || 'ไม่สามารถปรับสิทธิ์ได้'), true);
    return;
  }
  showUsersStatus('✅ บันทึกสิทธิ์และซิงค์ไปยังทุก Block เรียบร้อยแล้ว');
  await loadUsersList();
}

async function saveUserDetails(id) {
  const displayName = document.getElementById(`editName_${id}`)?.value.trim() || '';
  const email = document.getElementById(`editEmail_${id}`)?.value.trim() || '';
  const ward = document.getElementById(`editWard_${id}`)?.value.trim() || '';
  const res = await fetch(`/api/users/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ display_name: displayName, email, ward })
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    showUsersStatus('❌ ' + (data.error || 'บันทึกข้อมูลไม่สำเร็จ'), true);
    return;
  }
  showUsersStatus('✅ อัปเดตข้อมูลสมาชิกเรียบร้อยแล้ว');
  await loadUsersList();
}

async function setUserBlockRole(userId, blockId, role) {
  const res = await fetch(`/api/users/${userId}/block-role`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ block_id: blockId, role })
  });
  if (res.ok) {
    showUsersStatus(`✅ อัปเดตสิทธิ์ ${blockId} เป็น ${role} แล้ว`);
    const u = allUsersCache.find(x => x.id === userId);
    if (u) {
      u.block_roles = u.block_roles || {};
      u.block_roles[blockId] = role;
    }
  } else {
    showUsersStatus('❌ อัปเดตสิทธิ์รายบล็อกไม่สำเร็จ', true);
  }
}

async function submitCreateUser(e) {
  e.preventDefault();
  const res = await fetch('/api/users', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      username: document.getElementById('newUname').value.trim(),
      email: document.getElementById('newEmail').value.trim(),
      display_name: document.getElementById('newDisplay').value.trim(),
      password: document.getElementById('newPass').value,
      global_role: document.getElementById('newRole').value,
      ward: document.getElementById('newWard').value.trim()
    })
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    showUsersStatus('❌ ' + (data.error || 'เพิ่มสมาชิกไม่สำเร็จ'), true);
    return;
  }
  e.target.reset();
  toggleCreateUserDrawer(false);
  showUsersStatus('✅ เพิ่มสมาชิกใหม่และซิงค์ไปยังทุก Block เรียบร้อย');
  await loadUsersList();
}

async function toggleUserActive(id, isActive) {
  const res = await fetch(`/api/users/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ is_active: isActive })
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    showUsersStatus('❌ ' + (data.error || 'เปลี่ยนสถานะไม่สำเร็จ'), true);
    return;
  }
  showUsersStatus(isActive ? '✅ เปิดใช้งานบัญชีแล้ว' : '⏸ ระงับใช้งานบัญชีแล้ว');
  await loadUsersList();
}

async function resetUserPassword(id, uname) {
  const pw = prompt(`ตั้งรหัสผ่านใหม่สำหรับ ${uname}:`);
  if (!pw) return;
  const res = await fetch(`/api/users/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ password: pw })
  });
  if (res.ok) {
    showUsersStatus(`🔑 อัปเดตรหัสผ่านสำหรับ ${uname} เรียบร้อยแล้ว`);
  } else {
    showUsersStatus('❌ เปลี่ยนรหัสผ่านไม่สำเร็จ', true);
  }
}

async function deleteUser(id, uname) {
  if (!confirm(`⚠️ ต้องการลบสมาชิก "${uname}" ออกจากระบบถาวรใช่หรือไม่?`)) return;
  const res = await fetch(`/api/users/${id}`, { method: 'DELETE' });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    showUsersStatus('❌ ' + (data.error || 'ลบไม่สำเร็จ'), true);
    return;
  }
  expandedUserDrawers.delete(id);
  showUsersStatus(`🗑️ ลบสมาชิก "${uname}" เรียบร้อยแล้ว`);
  await loadUsersList();
}

async function toggleLocalOnly(id, localOnly) {
  await fetch(`/api/blocks/${id}/local-only`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ local_only: !!localOnly })
  });
  await loadBlocks();
}

async function logout() {
  await fetch('/api/auth/logout', { method: 'POST' });
  window.location.replace('/login');
}

async function submitBlzUpload(e) {
  e.preventDefault();
  const fileInput = document.getElementById('blzFileInput');
  const statusEl = document.getElementById('installStatus');
  if (!fileInput.files.length) return;
  statusEl.textContent = 'กำลังอัปโหลดและ Hot-Swap...';
  const fd = new FormData();
  fd.append('package', fileInput.files[0]);
  const res = await fetch('/api/blocks/upload', { method: 'POST', body: fd });
  const data = await res.json();
  if (!res.ok) {
    statusEl.textContent = '❌ ' + (data.error || 'อัปเดตไม่สำเร็จ');
    return;
  }
  statusEl.textContent = '✅ ' + data.message;
  await loadBlocks();
  setTimeout(() => toggleInstallModal(false), 1200);
}

async function promptAddPeer() {
  const url = prompt('ใส่ URL ของเครื่อง CoreChain อื่นใน LAN (เช่น http://192.168.1.50:8080):');
  if (!url) return;
  const res = await fetch('/api/sync/peers/add', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ lan_url: url })
  });
  const data = await res.json();
  if (!res.ok) {
    alert(data.error || 'เชื่อมต่อ Peer ไม่สำเร็จ');
    return;
  }
  await loadSync();
}

function escapeHtml(s) {
  return String(s || '').replace(/[&<>"']/g, m => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
  }[m]));
}
