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
  await Promise.all([loadHealth(), loadBlocks(), loadSync(), loadUpdater(), loadEvents()]);
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

function renderAuth() {
  const area = document.getElementById('authArea');
  const btnInstall = document.getElementById('btnShowInstall');
  const btnCheck = document.getElementById('btnCheckUpdate');
  const btnBoot = document.getElementById('btnBootstrapSync');
  const btnPeer = document.getElementById('btnAddPeer');
  const isAdmin = currentUser && (currentUser.global_role === 'admin' || currentUser.global_role === 'god');

  btnInstall.classList.toggle('hidden', !isAdmin);
  if (btnCheck) btnCheck.classList.toggle('hidden', !isAdmin);
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

    const isAdmin = currentUser && (currentUser.global_role === 'admin' || currentUser.global_role === 'god');
    grid.innerHTML = blocks.map(b => {
      const statusCls = b.status === 'running' ? 'status-running' : (b.status === 'updating' ? 'status-updating' : 'status-stopped');
      const summary = b.stats && b.stats.summary ? b.stats.summary : 'พร้อมใช้งาน (Isolated SQLite + Multi-Core)';
      const adminCtrls = isAdmin ? `
        ${b.status === 'running'
          ? `<button class="btn btn-secondary btn-sm" onclick="blockAction('${b.id}','stop')">⏹ หยุด</button>`
          : `<button class="btn btn-secondary btn-sm" onclick="blockAction('${b.id}','start')">▶ เริ่ม</button>`}
        <button class="btn btn-secondary btn-sm" onclick="setBlockCPU('${b.id}', ${b.cpu_threads || 2})">⚙ CPU (${b.cpu_threads || 'auto'})</button>
      ` : '';

      return `
        <div class="block-card">
          <div class="block-top">
            <div class="block-identity">
              <div class="block-icon" style="border: 1px solid ${b.color || 'var(--border)'}">${b.icon || '🧱'}</div>
              <div>
                <div class="block-name">${escapeHtml(b.name)}</div>
                <div class="block-meta">v${escapeHtml(b.version)} • PID ${b.pid || '-'} • ${b.cpu_threads || 'auto'} Threads</div>
              </div>
            </div>
            <span class="status-badge ${statusCls}">${escapeHtml(b.status)}</span>
          </div>
          <p class="block-desc">${escapeHtml(b.description || '')}</p>
          <div class="block-stats">📊 ${escapeHtml(summary)}</div>
          <div class="block-actions">
            <a class="btn btn-primary" href="${b.url}" ${b.status !== 'running' ? 'style="opacity:0.5;pointer-events:none;"' : ''}>🚀 เปิดใช้งาน</a>
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
    document.getElementById('statPeers').textContent = peers.length;
    document.getElementById('peersList').innerHTML = peers.map(p => `
      <div class="list-item">
        <strong>${escapeHtml(p.hostname)}</strong> ${p.is_self ? '(เครื่องนี้)' : ''} — <code>${escapeHtml(p.lan_url)}</code><br/>
        <small>Blocks: ${(p.blocks || []).join(', ') || '-'}</small>
      </div>
    `).join('');
  } catch {}
}

let updaterPollTimer = null;

async function loadUpdater() {
  try {
    const res = await fetch('/api/updater/status');
    const data = await res.json();
    const ver = data.current_version ? `v${data.current_version}` : '';
    const pctText = data.is_updating ? ` (${data.progress_pct || 0}%)` : '';
    document.getElementById('updaterBadge').textContent = `CoreChain ${ver} • ${data.status || 'idle'}${pctText}`;

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

async function logout() {
  await fetch('/api/auth/logout', { method: 'POST' });
  await refreshAll();
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
