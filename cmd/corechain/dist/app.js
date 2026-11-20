let currentUser = null;

document.addEventListener('DOMContentLoaded', () => {
  initTheme();
  refreshAll();
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
    area.innerHTML = `
      <span class="pill">👤 ${escapeHtml(currentUser.display_name || currentUser.username)} (${escapeHtml(currentUser.global_role)})</span>
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

async function loadBlocks() {
  const grid = document.getElementById('blocksGrid');
  try {
    const res = await fetch('/api/blocks');
    const data = await res.json();
    const blocks = (data.blocks || []).sort((a, b) => (a.order || 0) - (b.order || 0));
    const runningCount = blocks.filter(b => b.status === 'running').length;
    document.getElementById('statBlocks').textContent = `${runningCount}/${blocks.length}`;

    if (blocks.length === 0) {
      grid.innerHTML = `<div class="panel-card">ยังไม่มี Block ที่ติดตั้ง (วางไฟล์ <code>.Blz</code> ในโฟลเดอร์ <code>packages/</code>)</div>`;
      return;
    }

    const isAdmin = isManagerOrGod(currentUser);
    const canRemove = isTherunrock(currentUser);
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

function toggleUsersModal(show) {
  document.getElementById('usersModal').classList.toggle('hidden', !show);
}

async function openUsersModal() {
  toggleUsersModal(true);
  await loadUsersList();
}

async function loadUsersList() {
  const box = document.getElementById('usersListBox');
  box.innerHTML = 'กำลังโหลด...';
  try {
    const res = await fetch('/api/users');
    const data = await res.json();
    if (!res.ok) {
      box.innerHTML = `<div class="error-msg">${escapeHtml(data.error || 'ไม่มีสิทธิ์เข้าถึง')}</div>`;
      return;
    }
    allUsersCache = data.users || [];
    renderUsersList();
  } catch {
    box.innerHTML = '<div class="error-msg">โหลดรายชื่อสมาชิกไม่สำเร็จ</div>';
  }
}

function renderUsersList() {
  const box = document.getElementById('usersListBox');
  const q = (document.getElementById('userSearchInput')?.value || '').trim().toLowerCase();
  const filtered = allUsersCache.filter(u =>
    !q ||
    (u.username || '').toLowerCase().includes(q) ||
    (u.display_name || '').toLowerCase().includes(q) ||
    (u.ward || '').toLowerCase().includes(q)
  );
  if (filtered.length === 0) {
    box.innerHTML = '<div class="empty-note">ไม่พบรายชื่อสมาชิก</div>';
    return;
  }
  box.innerHTML = filtered.map(u => {
    const isGod = u.username === 'therunrock' || u.global_role === 'god';
    const br = u.block_roles || {};
    const webRole = br.webchain || (u.global_role === 'admin' || u.global_role === 'manager' ? 'admin' : 'poster');
    const learnRole = br.learnchain || (u.global_role === 'admin' || u.global_role === 'manager' ? 'manager' : 'learner');
    const docRole = br.docchain || (u.global_role === 'admin' || u.global_role === 'manager' ? 'admin' : 'user');
    const aiRole = br.ai || (u.global_role === 'admin' || u.global_role === 'manager' ? 'admin' : 'user');
    return `
      <div class="list-item" style="display:flex;flex-direction:column;gap:8px;">
        <div style="display:flex;justify-content:space-between;align-items:center;gap:10px;flex-wrap:wrap;">
          <div>
            <strong>${escapeHtml(u.display_name || u.username)}</strong>
            <span style="color:var(--muted);font-size:12px;">@${escapeHtml(u.username)}</span>
            ${u.ward ? `<span class="badge">${escapeHtml(u.ward)}</span>` : ''}
            ${!u.is_active ? `<span class="badge" style="background:#fee2e2;color:#dc2626;">ระงับใช้งาน</span>` : ''}
          </div>
          <div style="display:flex;align-items:center;gap:6px;flex-wrap:wrap;">
            ${isGod ? '<span class="badge">👑 therunrock</span>' : `
              <select class="theme-select" style="padding:4px 8px;font-size:12px;" onchange="updateUserRole('${u.id}', this.value)">
                <option value="user" ${u.global_role === 'user' ? 'selected' : ''}>สมาชิก (User)</option>
                <option value="manager" ${u.global_role === 'manager' ? 'selected' : ''}>ผู้จัดการ (Manager)</option>
                <option value="admin" ${u.global_role === 'admin' ? 'selected' : ''}>Admin</option>
              </select>
              <button class="btn btn-secondary btn-sm" onclick="toggleUserActive('${u.id}', ${!u.is_active})">${u.is_active ? '⏸ ระงับ' : '▶ เปิดใช้'}</button>
              <button class="btn btn-secondary btn-sm" onclick="resetUserPassword('${u.id}', '${escapeHtml(u.username)}')">🔑 รหัสผ่าน</button>
              <button class="btn btn-secondary btn-sm" style="color:var(--danger);" onclick="deleteUser('${u.id}', '${escapeHtml(u.username)}')">🗑</button>
            `}
          </div>
        </div>
        ${!isGod ? `
          <div style="display:flex;gap:8px;flex-wrap:wrap;font-size:11.5px;color:var(--muted);padding-top:4px;border-top:1px dashed var(--border);">
            <label>🏥 WebChain:
              <select class="theme-select" style="padding:2px 6px;font-size:11.5px;" onchange="setUserBlockRole('${u.id}','webchain',this.value)">
                <option value="poster" ${webRole === 'poster' ? 'selected' : ''}>Poster</option>
                <option value="admin" ${webRole === 'admin' ? 'selected' : ''}>Admin</option>
              </select>
            </label>
            <label>🎓 LearnChain:
              <select class="theme-select" style="padding:2px 6px;font-size:11.5px;" onchange="setUserBlockRole('${u.id}','learnchain',this.value)">
                <option value="learner" ${learnRole === 'learner' ? 'selected' : ''}>Learner</option>
                <option value="creator" ${learnRole === 'creator' ? 'selected' : ''}>Creator</option>
                <option value="manager" ${learnRole === 'manager' ? 'selected' : ''}>Manager</option>
              </select>
            </label>
            <label>📄 DocChain:
              <select class="theme-select" style="padding:2px 6px;font-size:11.5px;" onchange="setUserBlockRole('${u.id}','docchain',this.value)">
                <option value="user" ${docRole === 'user' ? 'selected' : ''}>User</option>
                <option value="admin" ${docRole === 'admin' ? 'selected' : ''}>Admin</option>
              </select>
            </label>
            <label>🧠 AI NPU:
              <select class="theme-select" style="padding:2px 6px;font-size:11.5px;" onchange="setUserBlockRole('${u.id}','ai',this.value)">
                <option value="user" ${aiRole === 'user' ? 'selected' : ''}>User</option>
                <option value="admin" ${aiRole === 'admin' ? 'selected' : ''}>Admin</option>
              </select>
            </label>
          </div>
        ` : ''}
      </div>
    `;
  }).join('');
}

async function setUserBlockRole(userId, blockId, role) {
  await fetch(`/api/users/${userId}/block-role`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ block_id: blockId, role })
  });
}

async function submitCreateUser(e) {
  e.preventDefault();
  const msg = document.getElementById('userFormMsg');
  msg.textContent = 'กำลังบันทึก...';
  const res = await fetch('/api/users', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      username: document.getElementById('newUname').value.trim(),
      display_name: document.getElementById('newDisplay').value.trim(),
      password: document.getElementById('newPass').value,
      global_role: document.getElementById('newRole').value,
      ward: document.getElementById('newWard').value.trim()
    })
  });
  const data = await res.json();
  if (!res.ok) {
    msg.textContent = '❌ ' + (data.error || 'เพิ่มสมาชิกไม่สำเร็จ');
    return;
  }
  msg.textContent = '✅ เพิ่มสมาชิกเรียบร้อย';
  e.target.reset();
  await loadUsersList();
}

async function updateUserRole(id, role) {
  await fetch(`/api/users/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ global_role: role })
  });
  await loadUsersList();
}

async function toggleUserActive(id, isActive) {
  await fetch(`/api/users/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ is_active: isActive })
  });
  await loadUsersList();
}

async function resetUserPassword(id, uname) {
  const pw = prompt(`ตั้งรหัสผ่านใหม่สำหรับ ${uname}:`);
  if (!pw) return;
  await fetch(`/api/users/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ password: pw })
  });
  alert(`อัปเดตรหัสผ่านสำหรับ ${uname} เรียบร้อยแล้ว`);
}

async function deleteUser(id, uname) {
  if (!confirm(`ต้องการลบสมาชิก '${uname}' ใช่หรือไม่?`)) return;
  const res = await fetch(`/api/users/${id}`, { method: 'DELETE' });
  const data = await res.json();
  if (!res.ok) alert(data.error || 'ลบไม่สำเร็จ');
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
