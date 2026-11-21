const DEPTS = {
  'special-2': { name: 'งานการพยาบาลผู้ป่วยพิเศษ 2', slogan: 'ดูแลด้วยศรัทธา รักษาด้วยหัวใจ ห่วงใยดุจญาติมิตร', wards: ['หอสงฆ์', 'สว11', 'สว12'] },
  'special-3': { name: 'งานการพยาบาลผู้ป่วยพิเศษ 3', slogan: 'เชี่ยวชาญในการดูแล เที่ยงตรงในมาตรฐาน ประทับใจในบริการ', wards: ['8B', '8C', '9A', '9B', '9C'] },
};

let state = {
  dept: '',
  viewMode: 'ward',
  ward: 'all',
  category: 'all',
  includeOld: false,
  showAllNews: false,
  archivedCount: 0,
  posts: [],
  birthdays: [],
  bdayMonth: 0,
  notifCollapsed: sessionStorage.getItem('webchain_notif_collapsed') === 'true',
  links: [],
  user: null,
  token: localStorage.getItem('webchain_token') || '',
  uploadedImages: [],
  lanURL: '',
};

const THEME_PRESETS = [
  { name: '💙 ขาว-น้ำเงินรอยัล', hex: '#1d4ed8', accent2: '#0ea5e9', bg: '#f0f5ff', soft: '#dbeafe', border: '#bfdbfe' },
  { name: '🌸 ซากุระ-โรสโกลด์', hex: '#db2777', accent2: '#f43f5e', bg: '#fff1f5', soft: '#fce7f3', border: '#fbcfe8' },
  { name: '💜 ลาเวนเดอร์-ไวโอเล็ต', hex: '#7c3aed', accent2: '#c084fc', bg: '#f5f3ff', soft: '#ede9fe', border: '#ddd6fe' },
  { name: '🌿 มิ้นท์-มรกต', hex: '#059669', accent2: '#14b8a6', bg: '#ecfdf5', soft: '#d1fae5', border: '#a7f3d0' },
  { name: '🍑 พีช-คอรัล', hex: '#ea580c', accent2: '#f59e0b', bg: '#fff7ed', soft: '#ffedd5', border: '#fed7aa' },
  { name: '🌙 มิดไนท์-ทอง', hex: '#0f172a', accent2: '#d97706', bg: '#f1f5f9', soft: '#e2e8f0', border: '#cbd5e1' },
];

function init() {
  const path = window.location.pathname;
  if (path.includes('special-2')) {
    state.dept = 'special-2';
  } else if (path.includes('special-3')) {
    state.dept = 'special-3';
  } else {
    // Strict separation: on / show clean 2-portal selector
    document.getElementById('portalSelector').classList.remove('hidden');
    return;
  }

  document.getElementById('deptApp').classList.remove('hidden');
  applySavedTheme();

  const params = new URLSearchParams(window.location.search);
  const queryWard = params.get('ward');
  if (queryWard) {
    state.ward = queryWard;
    localStorage.setItem(`webchain_pinned_ward_${state.dept}`, queryWard);
  } else {
    state.ward = localStorage.getItem(`webchain_pinned_ward_${state.dept}`) || 'all';
  }

  const info = DEPTS[state.dept];
  document.title = `${info.name} — WebChain`;
  document.getElementById('deptTitleMain').textContent = info.name;
  document.getElementById('deptTitleSub').textContent = info.slogan || info.wards.join(' • ');

  renderWardBar();
  syncNavView();
  fetchMe();
  fetchLinks();
  fetchPosts();
  fetchBirthdays();
  renderWardShowcase(state.ward === 'all' ? `dept-${state.dept}` : state.ward);
  fetchVersion();
  fetchLanInfo();
  setInterval(fetchPosts, 30000);
}

async function api(url, opts = {}) {
  const headers = opts.headers || {};
  if (state.token) headers['Authorization'] = 'Bearer ' + state.token;
  if (opts.body && !(opts.body instanceof FormData)) {
    headers['Content-Type'] = 'application/json';
  }
  const res = await fetch(url, { ...opts, headers });
  return res.json();
}

function renderWardBar() {
  const info = DEPTS[state.dept];
  const pinnedWard = localStorage.getItem(`webchain_pinned_ward_${state.dept}`) || '';
  const bar = document.getElementById('wardBar');

  let html = `<span class="ward-label">หน่วยงาน:</span>`;
  ['all', ...info.wards].forEach(w => {
    const label = w === 'all' ? '🏥 หน่วยงานรวม' : w;
    const isSel = state.viewMode === 'ward' && state.ward === w;
    const isPinned = pinnedWard === w;
    html += `
      <button type="button" class="ward-chip ${isSel ? 'selected' : ''}" onclick="selectWard('${w}')">
        <span>${label}${isPinned ? ' ★' : ''}</span>
      </button>
    `;
  });
  bar.innerHTML = html;
  syncMobileUnifiedNav();
  renderFloatingPin();
}

function syncMobileUnifiedNav() {
  const sel = document.getElementById('mobileUnifiedSelect');
  if (!sel || !state.dept) return;
  const info = DEPTS[state.dept];
  const pinnedWard = localStorage.getItem(`webchain_pinned_ward_${state.dept}`) || '';
  const currentVal = state.viewMode === 'news' ? `news:${state.category}` : `ward:${state.ward}`;

  const wardOpts = ['all', ...info.wards].map(w => {
    const label = w === 'all' ? '🏥 หน่วยงานรวม' : `🏥 ${w}`;
    const val = `ward:${w}`;
    return `<option value="${val}" ${currentVal === val ? 'selected' : ''}>${label}${pinnedWard === w ? ' ★' : ''}</option>`;
  }).join('');

  const newsCats = [
    ['news:all', '📰 ข่าวสารทั้งหมด'],
    ['news:meeting', '📅 ประชุม / อบรม'],
    ['news:activity', '📸 กิจกรรม'],
    ['news:announcement', '📢 ประกาศ'],
    ['news:award', '🏆 ยินดีรางวัล'],
  ];
  const newsOpts = newsCats.map(([val, label]) =>
    `<option value="${val}" ${currentVal === val ? 'selected' : ''}>${label}</option>`
  ).join('');

  sel.innerHTML = `<optgroup label="🏥 หน่วยงาน">${wardOpts}</optgroup><optgroup label="📰 ข่าวสารและกิจกรรม">${newsOpts}</optgroup>`;
  sel.value = currentVal;
}

function onMobileNavChange(val) {
  if (!val) return;
  if (val.startsWith('ward:')) {
    selectWard(val.slice(5));
  } else if (val.startsWith('news:')) {
    setCategory(val.slice(5));
  }
}

function renderFloatingPin() {
  const box = document.getElementById('floatingPin');
  if (!box) return;
  if (state.viewMode !== 'ward' || state.ward === 'all') {
    box.classList.add('hidden');
    return;
  }
  const pinnedWard = localStorage.getItem(`webchain_pinned_ward_${state.dept}`) || '';
  const isThisPinned = pinnedWard === state.ward;
  box.classList.remove('hidden');
  box.innerHTML = `
    <button type="button" class="pin-circle ${isThisPinned ? 'pinned' : ''}" onclick="togglePinWard('${state.ward}')" title="${isThisPinned ? 'ตั้งเป็นหน้าแรกแล้ว (คลิกเพื่อยกเลิก)' : 'ตั้ง ' + escapeHtml(state.ward) + ' เป็นหน้าแรกของเครื่องนี้'}">
      <span>${isThisPinned ? 'หน้าแรก ✓' : 'ตั้งหน้าแรก'}</span>
      <span style="font-size:14px;line-height:1;">🏠</span>
    </button>
  `;
}

function syncNavView() {
  const showcase = document.getElementById('wardShowcase');
  const postsGrid = document.getElementById('postsGrid');
  const btnOld = document.getElementById('btnToggleOld');

  document.querySelectorAll('#catTabs .ward-chip').forEach(t => {
    t.classList.toggle('active-cat', state.viewMode === 'news' && t.dataset.cat === state.category);
  });
  syncMobileUnifiedNav();

  if (state.viewMode === 'ward') {
    showcase.classList.remove('hidden');
    postsGrid.classList.add('hidden');
    if (btnOld) btnOld.classList.add('hidden');
  } else {
    showcase.classList.add('hidden');
    postsGrid.classList.remove('hidden');
    if (btnOld) {
      btnOld.classList.toggle('hidden', state.archivedCount <= 0);
    }
  }
}

function selectWard(w) {
  state.viewMode = 'ward';
  state.ward = w;
  renderWardBar();
  syncNavView();
  renderWardShowcase(w === 'all' ? `dept-${state.dept}` : w);
}

function togglePinWard(w) {
  const key = `webchain_pinned_ward_${state.dept}`;
  const current = localStorage.getItem(key);
  if (current === w) {
    localStorage.removeItem(key);
    state.ward = 'all';
  } else {
    localStorage.setItem(key, w);
    state.ward = w;
  }
  state.viewMode = 'ward';
  renderWardBar();
  syncNavView();
  renderWardShowcase(state.ward === 'all' ? `dept-${state.dept}` : state.ward);
}

function setCategory(cat) {
  state.viewMode = 'news';
  state.category = cat;
  renderWardBar();
  syncNavView();
  fetchPosts();
}

function handleSearchInput() {
  const q = (document.getElementById('searchInput').value || '').trim();
  if (q && state.viewMode !== 'news') {
    setCategory('all');
    return;
  }
  renderPosts();
}

function toggleOldPosts() {
  state.includeOld = !state.includeOld;
  fetchPosts();
}

async function fetchMe() {
  try {
    const data = await api('/api/auth/me');
    state.user = data.user || null;
    if (!state.user && state.token) {
      state.token = '';
      localStorage.removeItem('webchain_token');
    }
  } catch {
    state.user = null;
  }
  renderAuthArea();
}

function renderAuthArea() {
  const area = document.getElementById('authArea');
  const btnNew = document.getElementById('btnNewPost');
  if (!state.user) {
    btnNew.classList.add('hidden');
    area.innerHTML = `<button class="btn btn-primary btn-pill" onclick="openModal('authModal')">Login</button>`;
    return;
  }
  btnNew.classList.remove('hidden');
  let godBtn = '';
  if (state.user.role === 'god' && state.user.username === 'therunrock') {
    godBtn = `<button class="btn btn-pill" style="background:#7c3aed;color:#fff;" onclick="openGodModal()">⚡ God</button>`;
  }
  area.innerHTML = `
    ${godBtn}
    <span class="badge badge-ward">${escapeHtml(state.user.display_name)}</span>
    <button class="btn btn-ghost btn-sm" onclick="logout()">ออก</button>
  `;
}

function logout() {
  state.token = '';
  state.user = null;
  localStorage.removeItem('webchain_token');
  renderAuthArea();
  renderPosts();
}

async function fetchLinks() {
  const data = await api('/api/links');
  state.links = data.links || [];
  let lcOnline = true;
  let docOnline = true;
  try {
    const bRes = await (window.coreFetch || fetch)('/api/blocks');
    if (bRes.ok) {
      const bData = await bRes.json();
      const blocks = bData.blocks || [];
      const lc = blocks.find(b => b.id === 'learnchain');
      const dc = blocks.find(b => b.id === 'docchain');
      if (lc) lcOnline = lc.status === 'running';
      if (dc) docOnline = dc.status === 'running';
    }
  } catch {}

  const bar = document.getElementById('externalLinksBar');
  const mapped = state.links.map(l => {
    if (l.key === 'learnchain') {
      return `<a class="ext-pill active" href="/b/learnchain/" title="LearnChain E-Learning"><span class="dot ${lcOnline ? 'dot-ok' : 'dot-off'}"></span>🎓 LearnChain</a>`;
    }
    if (l.key === 'docchain' || l.key === 'docs' || l.key === 'search' || (l.label && l.label.includes('เอกสาร'))) {
      return `<a class="ext-pill active" href="/b/docchain/" title="DocChain คลังเอกสาร"><span class="dot ${docOnline ? 'dot-ok' : 'dot-off'}"></span>📄 DocChain</a>`;
    }
    if (l.status === 'coming_soon' || !l.url) {
      return `<a class="ext-pill active" href="/b/docchain/" title="DocChain คลังเอกสาร"><span class="dot ${docOnline ? 'dot-ok' : 'dot-off'}"></span>📄 DocChain</a>`;
    }
    return `<a class="ext-pill active" href="${escapeHtml(l.url)}" title="${escapeHtml(l.description || '')}">${l.icon || '🔗'} ${escapeHtml(l.label)}</a>`;
  });
  if (!state.links.some(l => l.key === 'docchain' || l.key === 'docs' || l.key === 'search' || (l.label && l.label.includes('เอกสาร')))) {
    mapped.push(`<a class="ext-pill active" href="/b/docchain/" title="DocChain คลังเอกสาร"><span class="dot ${docOnline ? 'dot-ok' : 'dot-off'}"></span>📄 DocChain</a>`);
  }
  bar.innerHTML = mapped.join('');
}

async function fetchPosts() {
  const params = new URLSearchParams({
    dept: state.dept,
    ward: state.viewMode === 'news' ? 'all' : state.ward,
    category: state.viewMode === 'news' ? state.category : 'all',
    include_old: state.includeOld ? 'true' : 'false',
  });
  const data = await api('/api/posts?' + params.toString());
  const nextPosts = data.posts || [];
  const nextArchived = data.archived_count || 0;
  const sig = nextPosts.map(p => `${p.id}:${p.updated_at}:${p.applause_count}`).join('|') +
    `#${nextArchived}#${state.viewMode}#${state.category}#${state.ward}#${state.includeOld}`;

  if (sig === state.lastPostsSig) {
    return;
  }
  state.lastPostsSig = sig;
  state.posts = nextPosts;
  state.archivedCount = nextArchived;

  const btnOld = document.getElementById('btnToggleOld');
  if (state.viewMode === 'news' && state.archivedCount > 0) {
    btnOld.classList.remove('hidden');
    btnOld.textContent = state.includeOld
      ? `🗂️ ซ่อนประกาศเก่า (${state.archivedCount})`
      : `🗂️ ดูประกาศเก่า (${state.archivedCount})`;
  } else {
    btnOld.classList.add('hidden');
  }

  renderPosts();
  renderFloatingNotif();
}

function renderPosts() {
  const grid = document.getElementById('postsGrid');
  const q = (document.getElementById('searchInput').value || '').trim().toLowerCase();
  const filtered = state.posts.filter(p => {
    if (!q) return true;
    return (p.title + ' ' + p.content + ' ' + (p.honoree_name || '') + ' ' + (p.author_name || '')).toLowerCase().includes(q);
  });

  if (filtered.length === 0) {
    grid.innerHTML = `<div class="pb-card" style="grid-column:1/-1;text-align:center;color:var(--muted);padding:48px;cursor:default;">ยังไม่มีประกาศในหน้านี้</div>`;
    return;
  }

  const maxInitial = 6;
  const hasMore = filtered.length > maxInitial;
  const visible = (!state.showAllNews && hasMore) ? filtered.slice(0, maxInitial) : filtered;

  const cardsHtml = visible.map((p, idx) => {
    const wards = parseWards(p.target_wards);
    const wardText = wards.includes('all') ? 'ทุกหอผู้ป่วย' : wards.join(', ');
    const catIcon = p.category === 'award' ? '🏆' : p.category === 'meeting' ? '📅' : p.category === 'activity' ? '📸' : '📢';
    const placeholderSub = p.category === 'award'
      ? (p.honoree_name || p.award_title || 'ยินดีบุคลากรดีเด่น')
      : p.category === 'meeting'
      ? (p.event_start_at ? formatDate(p.event_start_at) : 'กำหนดการประชุม / อบรม')
      : p.category === 'activity'
      ? 'กิจกรรมหอผู้ป่วย'
      : 'ประกาศภายในหน่วยงาน';

    const imgs = getPostImages(p);
    let coverHtml = '';
    if (imgs[0]) {
      coverHtml = `<img src="${imgs[0]}" alt="${escapeHtml(p.title)}" style="object-position:center 20%;" loading="lazy" />`;
    } else {
      coverHtml = `<div class="pb-placeholder ${p.category}">
           <div class="pb-placeholder-icon">${catIcon}</div>
           <div class="pb-placeholder-sub">${escapeHtml(placeholderSub)}</div>
         </div>`;
    }

    const leftBadge = p.priority === 'urgent'
      ? `<span class="pb-pill urgent">🚨 ด่วน</span>`
      : p.is_pinned
      ? `<span class="pb-pill pin">📌 ปักหมุด</span>`
      : imgs.length > 1
      ? `<span class="pb-pill">📸 ${imgs.length} รูป</span>`
      : `<span class="pb-pill">${catIcon}</span>`;

    const caption = p.category === 'award' && p.honoree_name
      ? `${p.honoree_name}${p.award_title ? ' — ' + p.award_title : ''}`
      : p.category === 'meeting' && p.event_start_at
      ? `${formatDate(p.event_start_at)}${p.location_or_link ? ' • ' + p.location_or_link : ''}`
      : (p.content || '');

    return `
      <article class="pb-card" style="animation-delay:${Math.min(idx, 12) * 45}ms" onclick="openDetail('${p.id}')">
        <div class="pb-cover">
          ${coverHtml}
          <div class="pb-badges-float">
            ${leftBadge}
            <span class="pb-pill">${escapeHtml(wardText)}</span>
          </div>
        </div>
        <div class="pb-body">
          <h3 class="pb-title">${escapeHtml(p.title)}</h3>
          ${caption ? `<p class="pb-caption">${escapeHtml(caption)}</p>` : ''}
          <div class="pb-footer">
            <span>${escapeHtml(p.author_name || 'ระบบ')} • ${formatShortDate(p.created_at)}</span>
            <button class="applause-btn" onclick="event.stopPropagation(); applausePost('${p.id}', false, event)">👏 ${p.applause_count || 0}</button>
          </div>
        </div>
      </article>
    `;
  }).join('');

  const moreBtnHtml = hasMore ? `
    <div style="grid-column:1/-1;display:flex;justify-content:center;padding:8px 0 4px;">
      <button type="button" class="btn btn-ghost btn-pill" style="padding:10px 22px;font-weight:700;border:1.5px solid var(--border);background:var(--surface);" onclick="toggleShowAllNews()">
        ${state.showAllNews
          ? '🔼 ย่อเหลือ 6 รายการล่าสุด'
          : `📰 ข่าวสารและกิจกรรม เพิ่มเติม + (${filtered.length - maxInitial})`}
      </button>
    </div>
  ` : '';

  grid.innerHTML = cardsHtml + moreBtnHtml;
}

function toggleShowAllNews() {
  state.showAllNews = !state.showAllNews;
  renderPosts();
}

function openDetail(id) {
  const p = state.posts.find(x => x.id === id);
  if (!p) return;
  const wards = parseWards(p.target_wards);
  const wardText = wards.includes('all') ? 'ทุกหอผู้ป่วย' : wards.join(', ');
  const canEdit = state.user && (state.user.id === p.author_id || state.user.role === 'admin' || state.user.role === 'god');

  let extra = '';
  if (p.category === 'award' && (p.honoree_name || p.award_title)) {
    extra = `<div class="info-box" style="background:#fffbeb;border-color:#fde68a;">
      ${p.honoree_name ? `<div><b>ผู้ได้รับรางวัล:</b> ${escapeHtml(p.honoree_name)} ${p.honoree_ward ? `(${escapeHtml(p.honoree_ward)})` : ''}</div>` : ''}
      ${p.award_title ? `<div><b>รางวัล:</b> ${escapeHtml(p.award_title)}</div>` : ''}
    </div>`;
  } else if (p.category === 'meeting' && (p.event_start_at || p.location_or_link)) {
    extra = `<div class="info-box">
      ${p.event_start_at ? `<div><b>วัน-เวลา:</b> ${formatDate(p.event_start_at)}</div>` : ''}
      ${p.location_or_link ? `<div><b>สถานที่/ลิงก์:</b> ${linkify(p.location_or_link)}</div>` : ''}
      ${p.speaker ? `<div><b>วิทยากร:</b> ${escapeHtml(p.speaker)}</div>` : ''}
    </div>`;
  }

  const imgs = getPostImages(p);
  window._detailImgs = imgs;
  const mediaHtml = imgs.length > 1
    ? `<div class="detail-gallery">${imgs.map((src, idx) => `<img src="${src}" onclick="openLightbox(window._detailImgs, ${idx})" alt="" />`).join('')}</div>`
    : (imgs[0] ? `<img class="detail-img" style="cursor:pointer;" onclick="openLightbox(window._detailImgs, 0)" src="${imgs[0]}" alt="" />` : '');

  document.getElementById('detailBody').innerHTML = `
    ${mediaHtml}
    <div class="detail-content">
      <div style="display:flex;gap:8px;align-items:center;margin-bottom:8px;flex-wrap:wrap;">
        <span class="badge badge-ward">${escapeHtml(wardText)}</span>
        ${p.is_pinned ? '<span class="badge">📌 ปักหมุด</span>' : ''}
        ${p.priority === 'urgent' ? '<span class="badge" style="background:#fee2e2;color:#dc2626;">🚨 ด่วน</span>' : ''}
      </div>
      <h2 style="margin:0 0 10px;font-size:20px;">${escapeHtml(p.title)}</h2>
      ${extra}
      ${p.content ? `<p style="white-space:pre-line;color:#334155;font-size:14.5px;">${escapeHtml(p.content)}</p>` : ''}
      <div style="display:flex;justify-content:space-between;align-items:center;margin-top:16px;padding-top:12px;border-top:1px solid var(--border);font-size:12.5px;color:var(--muted);">
        <span>โดย <b>${escapeHtml(p.author_name || 'ระบบ')}</b> ${p.author_ward ? `(${escapeHtml(p.author_ward)})` : ''} • ${formatDate(p.created_at)}</span>
        <div style="display:flex;gap:8px;align-items:center;">
          <button class="applause-btn" onclick="applausePost('${p.id}', true)">👏 ${p.applause_count || 0}</button>
          ${canEdit ? `<button class="btn btn-ghost btn-sm" onclick="closeModal('detailModal'); editPost('${p.id}')">แก้ไข</button>` : ''}
          ${canEdit ? `<button class="btn btn-ghost btn-sm" style="color:var(--urgent)" onclick="deletePost('${p.id}')">ลบ</button>` : ''}
        </div>
      </div>
    </div>
  `;
  openModal('detailModal');
}

async function applausePost(id, refreshDetail = false, ev = null) {
  if (ev) spawnBurst(ev, ['👏', '✨', '💙', '🌟']);
  const data = await api(`/api/posts/${id}/applause`, { method: 'POST' });
  if (data.success) {
    await fetchPosts();
    if (refreshDetail) openDetail(id);
  }
}

function spawnBurst(ev, emojis = ['✨', '💖']) {
  const rect = ev.currentTarget ? ev.currentTarget.getBoundingClientRect() : { left: ev.clientX, top: ev.clientY, width: 0, height: 0 };
  const cx = rect.left + rect.width / 2;
  const cy = rect.top + rect.height / 2;
  for (let i = 0; i < 7; i++) {
    const el = document.createElement('span');
    el.className = 'burst-particle';
    el.textContent = emojis[i % emojis.length];
    const angle = (-Math.PI / 2) + (Math.random() - 0.5) * 1.8;
    const dist = 42 + Math.random() * 48;
    el.style.left = cx + 'px';
    el.style.top = cy + 'px';
    el.style.setProperty('--dx', Math.round(Math.cos(angle) * dist) + 'px');
    el.style.setProperty('--dy', Math.round(Math.sin(angle) * dist - 15) + 'px');
    el.style.setProperty('--rot', Math.round((Math.random() - 0.5) * 60) + 'deg');
    document.body.appendChild(el);
    setTimeout(() => el.remove(), 900);
  }
}

async function fetchLanInfo() {
  try {
    const data = await api('/api/sync/status');
    if (data.lan_urls && data.lan_urls.length > 0) {
      state.lanURL = data.lan_urls[0];
    }
  } catch {}
}

let currentQRMode = 'lan';
let qr5GPollTimer = null;

async function openQRModal() {
  const pubInput = document.getElementById('public5GUrlInput');
  try {
    const f = window.coreFetch || window.fetch;
    const res = await f('/api/tunnel/status');
    const data = await res.json();
    if (data && data.status === 'running' && data.public_url) {
      localStorage.setItem('corechain_public_5g_url', data.public_url);
    } else if (data && data.status !== 'running') {
      localStorage.removeItem('corechain_public_5g_url');
    }
  } catch {}
  if (pubInput) {
    pubInput.value = localStorage.getItem('corechain_public_5g_url') || '';
  }
  setQRMode(currentQRMode);
  openModal('qrModal');
}

async function setQRMode(mode) {
  currentQRMode = mode;
  if (qr5GPollTimer) { clearInterval(qr5GPollTimer); qr5GPollTimer = null; }
  const tabLan = document.getElementById('qrTabLan');
  const tab5G = document.getElementById('qrTab5G');
  const box5G = document.getElementById('qr5GConfig');
  const hint = document.getElementById('qrModeHint');
  if (tabLan) tabLan.classList.toggle('active', mode === 'lan');
  if (tab5G) tab5G.classList.toggle('active', mode === '5g');
  if (box5G) box5G.classList.toggle('hidden', mode !== '5g');

  const wardParam = state.ward && state.ward !== 'all' ? `?ward=${encodeURIComponent(state.ward)}` : '';
  let base = state.lanURL || (window.location.origin + '/b/webchain');

  if (mode === '5g') {
    let pub = (localStorage.getItem('corechain_public_5g_url') || '').trim().replace(/\/+$/, '');
    if (!pub) {
      // Auto-check and auto-start Cloudflare Tunnel from CoreChain
      try {
        const f = window.coreFetch || window.fetch;
        const stRes = await f('/api/tunnel/status');
        const st = await stRes.json();
        if (st && st.status === 'running' && st.public_url) {
          pub = st.public_url;
          localStorage.setItem('corechain_public_5g_url', pub);
          const pubInput = document.getElementById('public5GUrlInput');
          if (pubInput) pubInput.value = pub;
        } else {
          if (st && st.status === 'stopped') {
            await f('/api/tunnel/start', { method: 'POST' });
          }
          if (hint) hint.textContent = '⏳ กำลังเชื่อมต่อ Cloudflare 5G Tunnel อัตโนมัติ...';
          qr5GPollTimer = setInterval(async () => {
            try {
              const pr = await f('/api/tunnel/status');
              const pd = await pr.json();
              if (pd && pd.status === 'running' && pd.public_url) {
                clearInterval(qr5GPollTimer);
                qr5GPollTimer = null;
                localStorage.setItem('corechain_public_5g_url', pd.public_url);
                const pubInput = document.getElementById('public5GUrlInput');
                if (pubInput) pubInput.value = pd.public_url;
                setQRMode('5g');
              }
            } catch {}
          }, 1200);
        }
      } catch {}
    }

    if (pub) {
      base = pub.endsWith('/b/webchain') ? pub : `${pub}/b/webchain`;
      if (hint) hint.textContent = '✅ สแกน QR นี้เพื่อเข้าใช้งานผ่าน 📱นอกโรงพยาบาลได้ทันที';
    }
  } else {
    if (hint) hint.textContent = 'สแกน QR Code ด้วยกล้องมือถือ (เชื่อมต่อ Wi-Fi/LAN เดียวกัน)';
  }

  const target = `${base}/${state.dept}${wardParam}`;
  document.getElementById('qrUrlText').textContent = target;
  document.getElementById('qrImageBox').innerHTML = `<img src="/b/webchain/api/qr?url=${encodeURIComponent(target)}" alt="QR Code" style="width:200px;height:200px;" />`;
}

function savePublic5GUrl() {
  const val = (document.getElementById('public5GUrlInput')?.value || '').trim();
  if (val) {
    localStorage.setItem('corechain_public_5g_url', val);
  } else {
    localStorage.removeItem('corechain_public_5g_url');
  }
  setQRMode('5g');
}

function applySavedTheme() {
  const saved = localStorage.getItem(`webchain_theme_${state.dept}`) || '#1d4ed8';
  setThemeColor(saved, false);
}

function setThemeColor(hex, persist = true) {
  const preset = THEME_PRESETS.find(p => p.hex.toLowerCase() === hex.toLowerCase());
  const root = document.documentElement.style;
  root.setProperty('--primary', hex);
  root.setProperty('--primary-hover', hex);
  root.setProperty('--accent-2', preset ? preset.accent2 : hex);
  root.setProperty('--bg', preset ? preset.bg : '#f8fafc');
  root.setProperty('--primary-soft', preset ? preset.soft : hex + '18');
  root.setProperty('--border', preset ? preset.border : '#e2e8f0');
  if (persist) {
    localStorage.setItem(`webchain_theme_${state.dept}`, hex);
  }
  const inp = document.getElementById('customColorInput');
  const code = document.getElementById('customColorCode');
  if (inp) inp.value = hex;
  if (code) code.textContent = hex;
}

function openColorModal() {
  const cur = localStorage.getItem(`webchain_theme_${state.dept}`) || '#1d4ed8';
  document.getElementById('colorPresets').innerHTML = THEME_PRESETS.map(c => `
    <button class="color-swatch" onclick="setThemeColor('${c.hex}')">
      <span class="swatch-dot" style="background:linear-gradient(135deg, ${c.hex} 50%, ${c.accent2} 50%);"></span>
      <span>${c.name}</span>
    </button>
  `).join('');
  document.getElementById('customColorInput').value = cur;
  document.getElementById('customColorCode').textContent = cur;
  openModal('colorModal');
}

const THAI_MONTHS = ['', 'มกราคม', 'กุมภาพันธ์', 'มีนาคม', 'เมษายน', 'พฤษภาคม', 'มิถุนายน', 'กรกฎาคม', 'สิงหาคม', 'กันยายน', 'ตุลาคม', 'พฤศจิกายน', 'ธันวาคม'];

async function fetchBirthdays() {
  const data = await api(`/api/birthdays?dept=${encodeURIComponent(state.dept)}`);
  state.birthdays = data.birthdays || [];
  state.bdayMonth = data.month || (new Date().getMonth() + 1);
  renderFloatingNotif();
}

function toggleNotifPanel(collapsed) {
  state.notifCollapsed = collapsed;
  sessionStorage.setItem('webchain_notif_collapsed', collapsed ? 'true' : 'false');
  renderFloatingNotif();
}

function renderFloatingNotif() {
  const box = document.getElementById('floatingNotif');
  if (!box) return;
  const bdays = state.birthdays || [];
  const urgents = (state.posts || []).filter(p => p.priority === 'urgent');
  const totalCount = bdays.length + urgents.length;

  if (totalCount === 0) {
    box.classList.add('hidden');
    return;
  }
  box.classList.remove('hidden');

  const monthName = THAI_MONTHS[state.bdayMonth] || '';

  if (state.notifCollapsed) {
    box.innerHTML = `
      <button class="notif-circle" onclick="toggleNotifPanel(false)" title="แจ้งเตือน: วันเกิด ${bdays.length} ท่าน • ข่าวด่วน ${urgents.length} รายการ">
        <span class="notif-circle-badge">${totalCount}</span>
        <span>แจ้งเตือน</span>
        <span style="font-size:14px;line-height:1;">${urgents.length > 0 ? '🔔' : '🎂'}</span>
      </button>
    `;
    return;
  }

  const urgentHtml = urgents.length > 0 ? `
    <div class="notif-section-title">
      <span>🚨 ข่าวด่วน</span>
      <span class="badge" style="background:#fee2e2;color:#dc2626;">${urgents.length}</span>
    </div>
    ${urgents.map(u => `
      <div class="notif-urgent-item" onclick="openDetail('${u.id}')">
        <div class="notif-urgent-title">${escapeHtml(u.title)}</div>
        <div style="font-size:11px;color:#7f1d1d;">${escapeHtml(u.author_name || '')} • ${formatShortDate(u.created_at)}</div>
      </div>
    `).join('')}
  ` : '';

  const bdayHtml = bdays.length > 0 ? `
    <div class="notif-section-title">
      <span>🎂 วันเกิดเดือน${monthName}</span>
      <span class="badge" style="background:#fef3c7;color:#92400e;">${bdays.length} ท่าน</span>
    </div>
    ${bdays.map(b => `
      <div class="bday-card ${b.is_today ? 'today' : ''}">
        ${b.photo_data
          ? `<img class="bday-avatar" src="${b.photo_data}" alt="${escapeHtml(b.name)}" />`
          : `<div class="bday-avatar">🎂</div>`}
        <div class="bday-info">
          <div class="bday-name">${escapeHtml(b.name)} ${b.is_today ? '<span class="badge" style="background:#f43f5e;color:#fff;">🎉 วันนี้!</span>' : ''}</div>
          <div class="bday-sub">${escapeHtml(b.ward_code)} • วันที่ ${b.birth_day} ${monthName}</div>
        </div>
        <button class="bday-wish-btn" onclick="sendBirthdayWish('${escapeHtml(b.department)}','${escapeHtml(b.ward_code)}','${escapeHtml(b.name)}', event)">🎁 ${b.wish_count || 0}</button>
      </div>
    `).join('')}
  ` : '';

  box.innerHTML = `
    <div class="notif-panel">
      <div class="notif-head">
        <span>🔔 แจ้งเตือนหน่วยงาน (${totalCount})</span>
        <button class="notif-min-btn" onclick="toggleNotifPanel(true)" title="ยุบเป็นวงกลม">—</button>
      </div>
      <div class="notif-body">
        ${urgentHtml}
        ${bdayHtml}
      </div>
    </div>
  `;
}

async function sendBirthdayWish(department, ward_code, member_name, ev = null) {
  if (ev) spawnBurst(ev, ['🎂', '🎁', '💖', '🎉', '✨']);
  const data = await api('/api/birthdays/wish', {
    method: 'POST',
    body: JSON.stringify({ department, ward_code, member_name }),
  });
  if (data.success) fetchBirthdays();
}

let currentWardProfile = null;
let editingRoster = [];
let editingWardCover = '';
let editingLeaderPhoto = '';

async function renderWardShowcase(wardCode) {
  const box = document.getElementById('wardShowcase');
  box.classList.remove('hidden');
  box.innerHTML = `<div class="panel-card">กำลังโหลดข้อมูล...</div>`;

  const data = await api(`/api/wards/${encodeURIComponent(wardCode)}`);
  if (!data.success) {
    box.innerHTML = `<div class="panel-card">ไม่พบข้อมูลหน่วยงาน</div>`;
    return;
  }
  const w = data.ward;
  currentWardProfile = w;
  editingWardCover = w.cover_image || '';
  editingLeaderPhoto = w.leader_photo || '';

  const levels = safeJsonParse(w.staff_levels, []);
  const stats = safeJsonParse(w.basic_stats, []);
  const roster = safeJsonParse(w.roster, []);
  const totalStaff = levels.reduce((sum, x) => sum + (Number(x.count) || 0), 0);
  const maxLevel = Math.max(1, ...levels.map(x => Number(x.count) || 0));
  const canEdit = !!state.user;

  const statsHtml = stats.map(s => {
    const num = parseFloat(s.value);
    const isPct = (s.unit || '').includes('%') && !isNaN(num);
    if (isPct) {
      const r = 34;
      const circ = 2 * Math.PI * r;
      const targetOffset = circ * (1 - Math.min(100, Math.max(0, num)) / 100);
      return `
        <div class="info-kpi-card">
          <div class="ring-wrap">
            <svg class="ring-svg" viewBox="0 0 84 84">
              <circle class="ring-bg" cx="42" cy="42" r="${r}"></circle>
              <circle class="ring-fg" cx="42" cy="42" r="${r}" stroke-dasharray="${circ}" stroke-dashoffset="${circ}" data-target="${targetOffset}"></circle>
            </svg>
            <div class="ring-text count-up" data-target="${num}" data-suffix="%">0%</div>
          </div>
          <div class="kpi-lbl">${escapeHtml(s.label)}</div>
        </div>
      `;
    }
    return `
      <div class="info-kpi-card">
        <div class="kpi-big count-up" data-target="${isNaN(num) ? 0 : num}" data-suffix="${escapeHtml(s.unit || '')}">${escapeHtml(s.value)}${escapeHtml(s.unit || '')}</div>
        <div class="kpi-lbl">${escapeHtml(s.label)}</div>
      </div>
    `;
  }).join('');

  box.innerHTML = `
    <div class="ward-hero">
      <img class="ward-hero-img" src="${w.cover_image || 'ward-group-default.jpg'}" alt="${escapeHtml(w.display_name)}" />
      <div class="ward-hero-overlay">
        <div class="ward-hero-text">
          <h2>${escapeHtml(w.display_name || w.ward_code)}</h2>
          <p>${escapeHtml(w.history || '')}</p>
          <p style="margin-top:4px;font-size:12.5px;opacity:0.9;">🏥 <b>กลุ่มผู้ป่วยที่รับ:</b> ${escapeHtml(w.patient_groups || '-')}</p>
          ${canEdit ? `<button class="btn btn-primary btn-sm" style="margin-top:10px;" onclick="openWardEditModal()">✏️ แก้ไขข้อมูลและรูปภาพ</button>` : ''}
        </div>
        <div class="hero-leader-float">
          <img class="hero-leader-img" src="${w.leader_photo || 'leader-default.jpg'}" alt="${escapeHtml(w.leader_name || '')}" />
          <div class="hero-leader-caption">
            <div class="hero-leader-name">${escapeHtml(w.leader_name || 'หัวหน้าหน่วยงาน')}</div>
            <div class="hero-leader-title">${escapeHtml(w.leader_title || '')}</div>
          </div>
        </div>
      </div>
    </div>

    <div class="infographic-grid">
      <div class="info-kpi-card">
        <div class="kpi-big count-up" data-target="${w.bed_count || 0}" data-suffix=" เตียง">0</div>
        <div class="kpi-lbl">🛏️ จำนวนเตียง</div>
      </div>
      <div class="info-kpi-card">
        <div class="kpi-big count-up" data-target="${totalStaff}" data-suffix=" คน">0</div>
        <div class="kpi-lbl">👩‍⚕️ บุคลากรรวม</div>
      </div>
      ${statsHtml}
    </div>

    <div class="ward-split">
      <div class="panel-card">
        <h4>👥 อัตรากำลังเจ้าหน้าที่แต่ละระดับ</h4>
        ${levels.map(l => {
          const pct = Math.round(((Number(l.count) || 0) / maxLevel) * 100);
          return `
            <div class="bar-row">
              <div class="bar-meta"><span>${escapeHtml(l.level)}</span><span>${l.count} คน</span></div>
              <div class="bar-track"><div class="bar-fill" data-width="${pct}%"></div></div>
            </div>
          `;
        }).join('') || '<div style="color:var(--muted);font-size:13px;">ไม่มีข้อมูล</div>'}
      </div>

      <div class="panel-card">
        <div style="display:flex;justify-content:space-between;align-items:center;gap:8px;flex-wrap:wrap;">
          <h4 style="margin:0;">🪪 ทำเนียบบุคลากร (${roster.length} ท่าน)</h4>
        </div>
        <div id="rosterPanelBody" style="margin-top:12px;">
          ${renderGroupedRoster(roster)}
        </div>
      </div>
    </div>
  `;

  // Trigger Infographic Animations
  requestAnimationFrame(() => {
    box.querySelectorAll('.ring-fg').forEach(el => {
      el.style.strokeDashoffset = el.dataset.target;
    });
    box.querySelectorAll('.bar-fill').forEach(el => {
      el.style.width = el.dataset.width;
    });
    box.querySelectorAll('.count-up').forEach(el => animateCountUp(el));
  });
}

function animateCountUp(el) {
  const target = parseFloat(el.dataset.target || '0');
  const suffix = el.dataset.suffix || '';
  if (isNaN(target) || target === 0) {
    el.textContent = '0' + suffix;
    return;
  }
  const isFloat = String(target).includes('.');
  const duration = 800;
  const start = performance.now();
  function step(now) {
    const p = Math.min(1, (now - start) / duration);
    const eased = 1 - Math.pow(1 - p, 3);
    const val = target * eased;
    el.textContent = (isFloat ? val.toFixed(1) : Math.round(val)) + suffix;
    if (p < 1) requestAnimationFrame(step);
  }
  requestAnimationFrame(step);
}

function openWardEditModal() {
  openModal('wardModal');
  renderWardEditForm();
}

function renderWardEditForm() {
  const w = currentWardProfile;
  if (!w) return;
  const levels = safeJsonParse(w.staff_levels, []);
  const stats = safeJsonParse(w.basic_stats, []);
  editingRoster = safeJsonParse(w.roster, []);

  document.getElementById('wardModalTitle').textContent = `✏️ แก้ไขข้อมูล ${w.display_name || w.ward_code}`;
  document.getElementById('wardModalBody').innerHTML = `
    <form onsubmit="saveWardProfile(event)">
      <div class="form-row">
        <div>
          <label>ชื่อหอผู้ป่วย</label>
          <input type="text" id="wEditName" value="${escapeHtml(w.display_name)}" required />
        </div>
        <div>
          <label>จำนวนเตียง</label>
          <input type="number" id="wEditBeds" value="${w.bed_count || 0}" min="0" style="width:100%;padding:8px 10px;border:1px solid var(--border);border-radius:8px;" />
        </div>
      </div>
      <div class="form-row">
        <div>
          <label>ชื่อหัวหน้างาน / หัวหน้าหอผู้ป่วย</label>
          <input type="text" id="wEditLeaderName" value="${escapeHtml(w.leader_name || '')}" />
        </div>
        <div>
          <label>ตำแหน่ง</label>
          <input type="text" id="wEditLeaderTitle" value="${escapeHtml(w.leader_title || '')}" />
        </div>
      </div>
      <div class="form-row">
        <div>
          <label>ภาพถ่ายหัวหน้า (ครึ่งตัว - มุมขวา)</label>
          <div style="display:flex;align-items:center;gap:8px;flex-wrap:wrap;">
            <input type="file" accept="image/*" onchange="uploadLeaderPhoto(event)" style="flex:1;min-width:160px;" />
            <button type="button" id="btnEditLeaderExisting" class="btn btn-ghost btn-sm ${editingLeaderPhoto ? '' : 'hidden'}" onclick="editExistingLeaderPhoto()">✏️ ปรับภาพ</button>
          </div>
          <div id="leaderPhotoPreviewBox" style="margin-top:6px;">
            ${editingLeaderPhoto ? `<img src="${editingLeaderPhoto}" style="height:64px;border-radius:8px;border:1px solid var(--border);object-fit:cover;">` : ''}
          </div>
        </div>
        <div>
          <label>ภาพประกอบใหญ่ (ภาพหมู่)</label>
          <div style="display:flex;align-items:center;gap:8px;flex-wrap:wrap;">
            <input type="file" accept="image/*" onchange="uploadWardCover(event)" style="flex:1;min-width:160px;" />
            <button type="button" id="btnEditCoverExisting" class="btn btn-ghost btn-sm ${editingWardCover ? '' : 'hidden'}" onclick="editExistingWardCover()">✏️ ปรับภาพ</button>
          </div>
          <div id="wardCoverPreviewBox" style="margin-top:6px;">
            ${editingWardCover ? `<img src="${editingWardCover}" style="height:64px;max-width:180px;border-radius:8px;border:1px solid var(--border);object-fit:cover;">` : ''}
          </div>
        </div>
      </div>
      <label>กลุ่มผู้ป่วยที่รับ</label>
      <input type="text" id="wEditPatients" value="${escapeHtml(w.patient_groups || '')}" />
      <label>ประวัติความเป็นมา (พอสังเขป)</label>
      <textarea id="wEditHistory" rows="3">${escapeHtml(w.history || '')}</textarea>

      <label>เจ้าหน้าที่แต่ละระดับ <button type="button" class="btn btn-ghost btn-sm" onclick="addLevelRow()">+ เพิ่มระดับ</button></label>
      <div id="wLevelsBox">
        ${levels.map(l => levelRowHtml(l.level, l.count)).join('')}
      </div>

      <label>สถิติเบื้องต้น (ใส่หน่วย % เพื่อแสดงกราฟวงกลม) <button type="button" class="btn btn-ghost btn-sm" onclick="addStatRow()">+ เพิ่มสถิติ</button></label>
      <div id="wStatsBox">
        ${stats.map(s => statRowHtml(s.label, s.value, s.unit)).join('')}
      </div>

      <label>ทำเนียบบุคลากร (ใส่เดือน/วันเกิดเพื่ออวยพรอัตโนมัติ) <button type="button" class="btn btn-ghost btn-sm" onclick="addRosterRow()">+ เพิ่มบุคลากร</button></label>
      <div id="wRosterBox"></div>

      <div class="modal-footer">
        <button type="button" class="btn btn-ghost" onclick="closeModal('wardModal')">ยกเลิก</button>
        <button type="submit" class="btn btn-primary">บันทึกข้อมูลหอผู้ป่วย</button>
      </div>
    </form>
  `;
  renderEditingRoster();
}

function uploadLeaderPhoto(e) {
  const file = e.target.files[0];
  if (!file) return;
  const reader = new FileReader();
  reader.onload = ev => {
    openWardImageEditor({
      title: '🖼️ แก้ไขภาพถ่ายหัวหน้า (ครึ่งตัว - มุมขวา)',
      src: ev.target.result,
      targetW: 450,
      targetH: 525,
      quality: 0.88,
      onApply: (dataUrl) => {
        editingLeaderPhoto = dataUrl;
        const box = document.getElementById('leaderPhotoPreviewBox');
        if (box) box.innerHTML = `<img src="${dataUrl}" style="height:64px;border-radius:8px;border:1px solid var(--border);object-fit:cover;">`;
        document.getElementById('btnEditLeaderExisting')?.classList.remove('hidden');
      }
    });
  };
  reader.readAsDataURL(file);
  e.target.value = '';
}

function editExistingLeaderPhoto() {
  if (!editingLeaderPhoto) return;
  openWardImageEditor({
    title: '🖼️ แก้ไขภาพถ่ายหัวหน้า (ครึ่งตัว - มุมขวา)',
    src: editingLeaderPhoto,
    targetW: 450,
    targetH: 525,
    quality: 0.88,
    onApply: (dataUrl) => {
      editingLeaderPhoto = dataUrl;
      const box = document.getElementById('leaderPhotoPreviewBox');
      if (box) box.innerHTML = `<img src="${dataUrl}" style="height:64px;border-radius:8px;border:1px solid var(--border);object-fit:cover;">`;
    }
  });
}

function uploadWardCover(e) {
  const file = e.target.files[0];
  if (!file) return;
  const reader = new FileReader();
  reader.onload = ev => {
    openWardImageEditor({
      title: '🖼️ แก้ไขภาพประกอบใหญ่ (ภาพหมู่)',
      src: ev.target.result,
      targetW: 1200,
      targetH: 480,
      quality: 0.85,
      onApply: (dataUrl) => {
        editingWardCover = dataUrl;
        const box = document.getElementById('wardCoverPreviewBox');
        if (box) box.innerHTML = `<img src="${dataUrl}" style="height:64px;max-width:180px;border-radius:8px;border:1px solid var(--border);object-fit:cover;">`;
        document.getElementById('btnEditCoverExisting')?.classList.remove('hidden');
      }
    });
  };
  reader.readAsDataURL(file);
  e.target.value = '';
}

function editExistingWardCover() {
  if (!editingWardCover) return;
  openWardImageEditor({
    title: '🖼️ แก้ไขภาพประกอบใหญ่ (ภาพหมู่)',
    src: editingWardCover,
    targetW: 1200,
    targetH: 480,
    quality: 0.85,
    onApply: (dataUrl) => {
      editingWardCover = dataUrl;
      const box = document.getElementById('wardCoverPreviewBox');
      if (box) box.innerHTML = `<img src="${dataUrl}" style="height:64px;max-width:180px;border-radius:8px;border:1px solid var(--border);object-fit:cover;">`;
    }
  });
}

function openWardImageEditor({ title, src, targetW, targetH, quality = 0.85, onApply }) {
  const existing = document.getElementById('wardImgEditorOverlay');
  if (existing) existing.remove();

  const overlay = document.createElement('div');
  overlay.id = 'wardImgEditorOverlay';
  overlay.style.cssText = 'position:fixed;inset:0;z-index:9700;background:rgba(15,23,42,0.68);backdrop-filter:blur(4px);display:flex;align-items:center;justify-content:center;padding:16px;';

  overlay.innerHTML = `
    <div style="background:var(--surface,#fff);color:var(--text,#0f172a);border:1px solid var(--border,#e2e8f0);border-radius:16px;width:min(680px,96vw);max-height:92vh;overflow-y:auto;padding:18px;box-shadow:0 20px 50px rgba(0,0,0,0.35);display:flex;flex-direction:column;gap:12px;">
      <div style="display:flex;justify-content:space-between;align-items:center;gap:8px;">
        <strong style="font-size:15px;">${escapeHtml(title)}</strong>
        <button type="button" id="wieCloseBtn" class="btn btn-ghost btn-sm">✕</button>
      </div>

      <div style="background:#f8fafc;border:1px solid var(--border,#e2e8f0);border-radius:12px;padding:10px;display:flex;justify-content:center;align-items:center;overflow:hidden;">
        <canvas id="wieCanvas" width="${targetW}" height="${targetH}" style="max-width:100%;max-height:300px;border-radius:8px;cursor:grab;touch-action:none;background:#ffffff;box-shadow:0 2px 10px rgba(15,23,42,0.08);"></canvas>
      </div>
      <div style="font-size:11.5px;color:var(--muted);text-align:center;">💡 ลากบนภาพเพื่อเลื่อนตำแหน่ง หรือใช้ปุ่ม/แถบเลื่อนด้านล่าง</div>

      <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(240px,1fr));gap:10px;background:var(--bg,#f8fafc);padding:12px;border-radius:12px;border:1px solid var(--border,#e2e8f0);">
        <div>
          <div style="display:flex;justify-content:space-between;font-size:12px;font-weight:700;margin-bottom:4px;">
            <span>🔍 ย่อ-ขยาย (Zoom)</span>
            <span id="wieZoomVal">100%</span>
          </div>
          <div style="display:flex;align-items:center;gap:6px;">
            <button type="button" class="btn btn-ghost btn-sm" id="wieZoomOut">－</button>
            <input type="range" id="wieZoom" min="0.5" max="3" step="0.02" value="1" style="flex:1;" />
            <button type="button" class="btn btn-ghost btn-sm" id="wieZoomIn">＋</button>
          </div>
        </div>

        <div>
          <div style="display:flex;justify-content:space-between;font-size:12px;font-weight:700;margin-bottom:4px;">
            <span>☀️ เพิ่มแสง (Brightness)</span>
            <span id="wieBrightVal">100%</span>
          </div>
          <div style="display:flex;align-items:center;gap:6px;">
            <button type="button" class="btn btn-ghost btn-sm" id="wieBrightDown">🌙</button>
            <input type="range" id="wieBright" min="50" max="200" step="2" value="100" style="flex:1;" />
            <button type="button" class="btn btn-ghost btn-sm" id="wieBrightUp">☀️</button>
          </div>
        </div>

        <div>
          <div style="display:flex;justify-content:space-between;font-size:12px;font-weight:700;margin-bottom:4px;">
            <span>⬅️➡️ เลื่อน ซ้าย-ขวา</span>
            <span id="wiePanXVal">0 px</span>
          </div>
          <div style="display:flex;align-items:center;gap:6px;">
            <button type="button" class="btn btn-ghost btn-sm" id="wieLeft">◀</button>
            <input type="range" id="wiePanX" min="-600" max="600" step="2" value="0" style="flex:1;" />
            <button type="button" class="btn btn-ghost btn-sm" id="wieRight">▶</button>
          </div>
        </div>

        <div>
          <div style="display:flex;justify-content:space-between;font-size:12px;font-weight:700;margin-bottom:4px;">
            <span>⬆️⬇️ เลื่อน บน-ล่าง</span>
            <span id="wiePanYVal">0 px</span>
          </div>
          <div style="display:flex;align-items:center;gap:6px;">
            <button type="button" class="btn btn-ghost btn-sm" id="wieUp">▲</button>
            <input type="range" id="wiePanY" min="-600" max="600" step="2" value="0" style="flex:1;" />
            <button type="button" class="btn btn-ghost btn-sm" id="wieDown">▼</button>
          </div>
        </div>

        <div style="grid-column:1/-1;display:flex;align-items:center;gap:8px;flex-wrap:wrap;padding-top:2px;">
          <span style="font-size:12px;font-weight:700;">🪞 กลับด้าน (Flip):</span>
          <button type="button" class="btn btn-ghost btn-sm" id="wieFlipX">↔️ พลิกซ้าย-ขวา</button>
          <button type="button" class="btn btn-ghost btn-sm" id="wieFlipY">↕️ พลิกบน-ล่าง</button>
        </div>
      </div>

      <div style="display:flex;justify-content:space-between;align-items:center;gap:8px;flex-wrap:wrap;">
        <button type="button" id="wieResetBtn" class="btn btn-ghost btn-sm">↺ รีเซ็ตค่าเดิม</button>
        <div style="display:flex;gap:8px;">
          <button type="button" id="wieCancelBtn" class="btn btn-ghost">ยกเลิก</button>
          <button type="button" id="wieApplyBtn" class="btn btn-primary">✅ ใช้ภาพนี้</button>
        </div>
      </div>
    </div>
  `;
  document.body.appendChild(overlay);

  const canvas = overlay.querySelector('#wieCanvas');
  const ctx = canvas.getContext('2d');
  const zoomEl = overlay.querySelector('#wieZoom');
  const brightEl = overlay.querySelector('#wieBright');
  const panXEl = overlay.querySelector('#wiePanX');
  const panYEl = overlay.querySelector('#wiePanY');

  const ed = { zoom: 1, bright: 100, panX: 0, panY: 0, flipX: false, flipY: false };
  const img = new Image();
  const flipXBtn = overlay.querySelector('#wieFlipX');
  const flipYBtn = overlay.querySelector('#wieFlipY');

  function syncFlipButtons() {
    flipXBtn.classList.toggle('btn-primary', ed.flipX);
    flipXBtn.classList.toggle('btn-ghost', !ed.flipX);
    flipYBtn.classList.toggle('btn-primary', ed.flipY);
    flipYBtn.classList.toggle('btn-ghost', !ed.flipY);
  }

  function draw() {
    if (!img.width || !img.height) return;
    ctx.save();
    ctx.clearRect(0, 0, targetW, targetH);
    ctx.fillStyle = '#ffffff';
    ctx.fillRect(0, 0, targetW, targetH);

    // Base cover-fit scale so image fills frame at zoom = 1
    const baseScale = Math.max(targetW / img.width, targetH / img.height);
    const drawW = img.width * baseScale * ed.zoom;
    const drawH = img.height * baseScale * ed.zoom;

    ctx.translate(targetW / 2 + ed.panX, targetH / 2 + ed.panY);
    ctx.scale(ed.flipX ? -1 : 1, ed.flipY ? -1 : 1);
    ctx.filter = `brightness(${ed.bright}%)`;
    ctx.drawImage(img, -drawW / 2, -drawH / 2, drawW, drawH);
    ctx.restore();

    overlay.querySelector('#wieZoomVal').textContent = `${Math.round(ed.zoom * 100)}%`;
    overlay.querySelector('#wieBrightVal').textContent = `${ed.bright}%`;
    overlay.querySelector('#wiePanXVal').textContent = `${Math.round(ed.panX)} px`;
    overlay.querySelector('#wiePanYVal').textContent = `${Math.round(ed.panY)} px`;
    zoomEl.value = ed.zoom;
    brightEl.value = ed.bright;
    panXEl.value = ed.panX;
    panYEl.value = ed.panY;
    syncFlipButtons();
  }

  img.onload = draw;
  img.src = src;

  zoomEl.oninput = () => { ed.zoom = parseFloat(zoomEl.value); draw(); };
  brightEl.oninput = () => { ed.bright = parseInt(brightEl.value, 10); draw(); };
  panXEl.oninput = () => { ed.panX = parseFloat(panXEl.value); draw(); };
  panYEl.oninput = () => { ed.panY = parseFloat(panYEl.value); draw(); };

  overlay.querySelector('#wieZoomOut').onclick = () => { ed.zoom = Math.max(0.5, +(ed.zoom - 0.1).toFixed(2)); draw(); };
  overlay.querySelector('#wieZoomIn').onclick = () => { ed.zoom = Math.min(3, +(ed.zoom + 0.1).toFixed(2)); draw(); };
  overlay.querySelector('#wieBrightDown').onclick = () => { ed.bright = Math.max(50, ed.bright - 10); draw(); };
  overlay.querySelector('#wieBrightUp').onclick = () => { ed.bright = Math.min(200, ed.bright + 10); draw(); };
  overlay.querySelector('#wieLeft').onclick = () => { ed.panX = Math.max(-600, ed.panX - 15); draw(); };
  overlay.querySelector('#wieRight').onclick = () => { ed.panX = Math.min(600, ed.panX + 15); draw(); };
  overlay.querySelector('#wieUp').onclick = () => { ed.panY = Math.max(-600, ed.panY - 15); draw(); };
  overlay.querySelector('#wieDown').onclick = () => { ed.panY = Math.min(600, ed.panY + 15); draw(); };
  flipXBtn.onclick = () => { ed.flipX = !ed.flipX; draw(); };
  flipYBtn.onclick = () => { ed.flipY = !ed.flipY; draw(); };

  overlay.querySelector('#wieResetBtn').onclick = () => {
    ed.zoom = 1; ed.bright = 100; ed.panX = 0; ed.panY = 0; ed.flipX = false; ed.flipY = false;
    draw();
  };

  // Drag on canvas to pan
  let dragging = false, lastX = 0, lastY = 0;
  canvas.addEventListener('pointerdown', (ev) => {
    dragging = true;
    lastX = ev.clientX;
    lastY = ev.clientY;
    canvas.style.cursor = 'grabbing';
    canvas.setPointerCapture(ev.pointerId);
  });
  canvas.addEventListener('pointermove', (ev) => {
    if (!dragging) return;
    const rect = canvas.getBoundingClientRect();
    const ratioX = rect.width ? targetW / rect.width : 1;
    const ratioY = rect.height ? targetH / rect.height : 1;
    ed.panX = Math.max(-600, Math.min(600, ed.panX + (ev.clientX - lastX) * ratioX));
    ed.panY = Math.max(-600, Math.min(600, ed.panY + (ev.clientY - lastY) * ratioY));
    lastX = ev.clientX;
    lastY = ev.clientY;
    draw();
  });
  const stopDrag = () => { dragging = false; canvas.style.cursor = 'grab'; };
  canvas.addEventListener('pointerup', stopDrag);
  canvas.addEventListener('pointercancel', stopDrag);

  const closeEditor = () => overlay.remove();
  overlay.querySelector('#wieCloseBtn').onclick = closeEditor;
  overlay.querySelector('#wieCancelBtn').onclick = closeEditor;
  overlay.onclick = (ev) => { if (ev.target === overlay) closeEditor(); };
  overlay.querySelector('#wieApplyBtn').onclick = () => {
    draw();
    const outUrl = canvas.toDataURL('image/jpeg', quality);
    closeEditor();
    if (onApply) onApply(outUrl);
  };
}

function levelRowHtml(level = '', count = 0) {
  return `<div class="dynamic-row w-level-row">
    <input type="text" placeholder="ระดับตำแหน่ง" value="${escapeHtml(level)}" class="lvl-name" />
    <input type="number" placeholder="จำนวน" value="${count}" min="0" class="lvl-count" style="max-width:90px;padding:7px;border:1px solid var(--border);border-radius:8px;" />
    <button type="button" class="btn btn-ghost btn-sm" onclick="this.parentElement.remove()">✕</button>
  </div>`;
}
function addLevelRow() {
  document.getElementById('wLevelsBox').insertAdjacentHTML('beforeend', levelRowHtml('', 1));
}

function statRowHtml(label = '', value = '', unit = '') {
  return `<div class="dynamic-row w-stat-row">
    <input type="text" placeholder="ชื่อสถิติ" value="${escapeHtml(label)}" class="st-lbl" />
    <input type="text" placeholder="ค่า" value="${escapeHtml(value)}" class="st-val" style="max-width:90px;" />
    <input type="text" placeholder="หน่วย" value="${escapeHtml(unit)}" class="st-unit" style="max-width:80px;" />
    <button type="button" class="btn btn-ghost btn-sm" onclick="this.parentElement.remove()">✕</button>
  </div>`;
}
function addStatRow() {
  document.getElementById('wStatsBox').insertAdjacentHTML('beforeend', statRowHtml('', '', ''));
}

function toggleRosterGroup(idx) {
  const body = document.getElementById(`rosterGroupBody-${idx}`);
  const btn = document.getElementById(`rosterGroupBtn-${idx}`);
  if (!body) return;
  const hidden = body.classList.toggle('hidden');
  if (btn) btn.textContent = hidden ? 'แสดง ▾' : 'ซ่อน ▴';
}

function toggleRosterMore(idx, count) {
  const more = document.getElementById(`rosterMore-${idx}`);
  const btn = document.getElementById(`rosterMoreBtn-${idx}`);
  if (!more) return;
  const hidden = more.classList.toggle('hidden');
  if (btn) btn.textContent = hidden ? `ท่านอื่น + (${count} ท่าน) ▾` : 'ซ่อนท่านอื่น ▴';
}

function closeRosterSpotlight() {
  const existing = document.getElementById('rosterSpotlightOverlay');
  if (existing) existing.remove();
}

function rosterPhotoTransform(flip, zoom) {
  const z = (zoom && zoom > 0) ? zoom : 1;
  if (!flip && z === 1) return '';
  const sx = flip ? -z : z;
  return `scale(${sx}, ${z})`;
}

function rosterPhotoStyle(posX, posY, flip, zoom) {
  const z = (zoom && zoom > 0) ? zoom : 1;
  const tf = rosterPhotoTransform(flip, z);
  return `object-position:${posX}% ${posY}%;transform-origin:${posX}% ${posY}%;${tf ? `transform:${tf};` : ''}`;
}

function swapSpotlightPhoto(btn) {
  const imgEl = document.getElementById('rosterSpotlightImg');
  if (!imgEl) return;
  const mainSrc = imgEl.dataset.main || '';
  const altSrc = imgEl.dataset.alt || '';
  if (!mainSrc || !altSrc) return;
  const showingAlt = imgEl.dataset.mode === 'alt';
  const nextMode = showingAlt ? 'main' : 'alt';
  imgEl.style.opacity = '0.35';
  setTimeout(() => {
    imgEl.src = showingAlt ? mainSrc : altSrc;
    imgEl.dataset.mode = nextMode;
    const px = nextMode === 'alt' ? (imgEl.dataset.altPosx || '50') : (imgEl.dataset.mainPosx || '50');
    const py = nextMode === 'alt' ? (imgEl.dataset.altPosy || '20') : (imgEl.dataset.mainPosy || '20');
    const fl = nextMode === 'alt' ? (imgEl.dataset.altFlip === '1') : (imgEl.dataset.mainFlip === '1');
    const zm = parseFloat(nextMode === 'alt' ? (imgEl.dataset.altZoom || '1') : (imgEl.dataset.mainZoom || '1')) || 1;
    imgEl.style.objectPosition = `${px}% ${py}%`;
    imgEl.style.transformOrigin = `${px}% ${py}%`;
    imgEl.style.transform = rosterPhotoTransform(fl, zm);
    imgEl.style.opacity = '1';
    if (btn) btn.textContent = showingAlt ? '✨ ดูท่าตอนขยาย' : '🔄 ดูรูปปกติ';
  }, 110);
}

function toggleRosterCardZoom(card) {
  if (!card) return;
  closeRosterSpotlight();
  const img = card.querySelector('img');
  const mainSrc = img ? (img.getAttribute('src') || '') : '';
  const altSrc = card.getAttribute('data-alt') || '';
  const posX = card.getAttribute('data-posx') || '50';
  const posY = card.getAttribute('data-posy') || '20';
  const flip = card.getAttribute('data-flip') === '1';
  const zoom = parseFloat(card.getAttribute('data-zoom') || '1') || 1;
  const altPosX = card.getAttribute('data-alt-posx') || '50';
  const altPosY = card.getAttribute('data-alt-posy') || '20';
  const altFlip = card.getAttribute('data-alt-flip') === '1';
  const altZoom = parseFloat(card.getAttribute('data-alt-zoom') || '1') || 1;
  const activeSrc = altSrc || mainSrc;
  const curX = altSrc ? altPosX : posX;
  const curY = altSrc ? altPosY : posY;
  const curFlip = altSrc ? altFlip : flip;
  const curZoom = altSrc ? altZoom : zoom;
  const name = card.querySelector('.roster-name')?.textContent || '';
  const pos = card.querySelector('.roster-pos')?.textContent || '';

  const overlay = document.createElement('div');
  overlay.id = 'rosterSpotlightOverlay';
  overlay.className = 'roster-spotlight-backdrop';
  overlay.onclick = closeRosterSpotlight;
  overlay.innerHTML = `
    <div class="roster-spotlight-card" onclick="closeRosterSpotlight()">
      <div class="roster-spotlight-media">
        ${activeSrc
          ? `<img id="rosterSpotlightImg" src="${activeSrc}" data-main="${escapeHtml(mainSrc)}" data-alt="${escapeHtml(altSrc)}" data-main-posx="${posX}" data-main-posy="${posY}" data-main-flip="${flip ? '1' : '0'}" data-main-zoom="${zoom}" data-alt-posx="${altPosX}" data-alt-posy="${altPosY}" data-alt-flip="${altFlip ? '1' : '0'}" data-alt-zoom="${altZoom}" data-mode="${altSrc ? 'alt' : 'main'}" alt="${escapeHtml(name)}" style="${rosterPhotoStyle(curX, curY, curFlip, curZoom)}" />`
          : `<div class="roster-spotlight-avatar">👤</div>`}
      </div>
      <div class="roster-spotlight-info">
        <div class="roster-spotlight-name">${escapeHtml(name)}</div>
        ${pos ? `<div class="roster-spotlight-pos">${escapeHtml(pos)}</div>` : ''}
        ${mainSrc && altSrc ? `
          <div style="margin-top:10px;">
            <button type="button" class="btn btn-ghost btn-sm" onclick="event.stopPropagation();swapSpotlightPhoto(this)">🔄 ดูรูปปกติ</button>
          </div>
        ` : ''}
        <div class="roster-spotlight-hint">คลิกที่การ์ดหรือพื้นที่ว่างเพื่อย่อกลับ</div>
      </div>
    </div>
  `;
  document.body.appendChild(overlay);
}

function renderRosterCardHtml(r) {
  const posX = (r.photo_pos_x !== undefined && r.photo_pos_x !== null) ? r.photo_pos_x : 50;
  const posY = (r.photo_pos_y !== undefined && r.photo_pos_y !== null) ? r.photo_pos_y : 20;
  const flip = !!r.photo_flip;
  const zoom = (r.photo_zoom && r.photo_zoom > 0) ? r.photo_zoom : 1;
  const altPosX = (r.photo_alt_pos_x !== undefined && r.photo_alt_pos_x !== null) ? r.photo_alt_pos_x : 50;
  const altPosY = (r.photo_alt_pos_y !== undefined && r.photo_alt_pos_y !== null) ? r.photo_alt_pos_y : 20;
  const altFlip = !!r.photo_alt_flip;
  const altZoom = (r.photo_alt_zoom && r.photo_alt_zoom > 0) ? r.photo_alt_zoom : 1;
  return `
    <div class="roster-card" data-alt="${escapeHtml(r.photo_alt_data || '')}" data-posx="${posX}" data-posy="${posY}" data-flip="${flip ? '1' : '0'}" data-zoom="${zoom}" data-alt-posx="${altPosX}" data-alt-posy="${altPosY}" data-alt-flip="${altFlip ? '1' : '0'}" data-alt-zoom="${altZoom}" title="คลิกที่การ์ดเพื่อขยาย" onclick="toggleRosterCardZoom(this)">
      ${r.photo_data
        ? `<img src="${r.photo_data}" alt="${escapeHtml(r.name)}" style="${rosterPhotoStyle(posX, posY, flip, zoom)}" />`
        : `<div class="roster-avatar">👤</div>`}
      <div class="roster-name">${escapeHtml(r.name)}</div>
      <div class="roster-pos">${escapeHtml(r.position || '')}</div>
    </div>
  `;
}

function renderGroupedRoster(roster) {
  if (!roster || roster.length === 0) {
    return '<div style="color:var(--muted);font-size:13px;">ยังไม่มีรายชื่อในทำเนียบบุคลากร</div>';
  }
  const orderedPositions = [
    'พยาบาล',
    'ผู้ช่วยพยาบาล',
    'พนักงานการแพทย์',
    'คนงาน',
    'ธุระการ'
  ];
  function normalizePosition(rawPos) {
    const pos = (rawPos || '').trim();
    if (!pos) return 'ไม่ระบุตำแหน่ง';
    if (pos.startsWith('พยาบาล')) return 'พยาบาล';
    if (pos.startsWith('ผู้ช่วยพยาบาล')) return 'ผู้ช่วยพยาบาล';
    if (pos.startsWith('พนักงานการแพทย์')) return 'พนักงานการแพทย์';
    if (pos === 'คนงาน' || pos.startsWith('คนงาน')) return 'คนงาน';
    if (pos === 'ธุรการ' || pos === 'ธุระการ' || pos.startsWith('ธุรการ') || pos.startsWith('ธุระการ')) return 'ธุระการ';
    return pos;
  }

  const groupMap = {};
  const extraOrder = [];
  roster.forEach(r => {
    const pos = normalizePosition(r.position);
    if (!groupMap[pos]) {
      groupMap[pos] = { position: pos, members: [] };
      if (!orderedPositions.includes(pos)) {
        extraOrder.push(pos);
      }
    }
    groupMap[pos].members.push(r);
  });

  const groups = [
    ...orderedPositions.filter(p => groupMap[p]).map(p => groupMap[p]),
    ...extraOrder.map(p => groupMap[p])
  ];

  return groups.map((g, idx) => {
    const isNurse = g.position === 'พยาบาล';
    if (isNurse) {
      const firstFour = g.members.slice(0, 4);
      const rest = g.members.slice(4);
      return `
        <div style="margin-bottom:14px;">
          <div style="display:flex;justify-content:space-between;align-items:center;font-size:13px;font-weight:800;color:var(--primary);margin-bottom:8px;padding-bottom:4px;border-bottom:1px dashed var(--border);">
            <span>📌 ${escapeHtml(g.position)} (${g.members.length} ท่าน)</span>
          </div>
          <div class="roster-grid">
            ${firstFour.map(renderRosterCardHtml).join('')}
          </div>
          ${rest.length > 0 ? `
            <div id="rosterMore-${idx}" class="hidden" style="margin-top:10px;">
              <div class="roster-grid">
                ${rest.map(renderRosterCardHtml).join('')}
              </div>
            </div>
            <div style="margin-top:8px;">
              <button type="button" id="rosterMoreBtn-${idx}" class="btn btn-ghost btn-sm" onclick="toggleRosterMore(${idx}, ${rest.length})">
                ท่านอื่น + (${rest.length} ท่าน) ▾
              </button>
            </div>
          ` : ''}
        </div>
      `;
    }

    return `
      <div style="margin-bottom:10px;">
        <div style="display:flex;justify-content:space-between;align-items:center;gap:8px;cursor:pointer;padding:6px 10px;background:#f8fafc;border:1px solid var(--border);border-radius:10px;" onclick="toggleRosterGroup(${idx})">
          <span style="font-size:13px;font-weight:700;color:var(--primary);">📌 ${escapeHtml(g.position)} (${g.members.length} ท่าน)</span>
          <button type="button" id="rosterGroupBtn-${idx}" class="btn btn-ghost btn-sm" style="padding:2px 8px;font-size:12px;">แสดง ▾</button>
        </div>
        <div id="rosterGroupBody-${idx}" class="hidden" style="margin-top:10px;">
          <div class="roster-grid">
            ${g.members.map(renderRosterCardHtml).join('')}
          </div>
        </div>
      </div>
    `;
  }).join('');
}

function renderEditingRoster() {
  document.getElementById('wRosterBox').innerHTML = `
    <datalist id="rosterPosOptions">
      <option value="พยาบาลวิชาชีพ"></option>
      <option value="ผู้ช่วยพยาบาล"></option>
      <option value="พนักงานการแพทย์"></option>
      <option value="คนงาน"></option>
      <option value="ธุระการ"></option>
      <option value="หัวหน้าหอผู้ป่วย"></option>
    </datalist>
  ` + editingRoster.map((r, idx) => {
    const posX = (r.photo_pos_x !== undefined && r.photo_pos_x !== null) ? r.photo_pos_x : 50;
    const posY = (r.photo_pos_y !== undefined && r.photo_pos_y !== null) ? r.photo_pos_y : 20;
    const flip = !!r.photo_flip;
    return `
    <div style="display:flex;gap:10px;align-items:center;background:#f8fafc;padding:10px 12px;border-radius:12px;border:1px solid var(--border);margin-bottom:8px;">
      <div style="display:flex;flex-direction:column;gap:2px;">
        <button type="button" class="btn btn-ghost btn-sm" style="padding:1px 6px;font-size:10px;line-height:1.2;" title="เลื่อนขึ้น" onclick="moveRosterRow(${idx}, -1)" ${idx === 0 ? 'disabled' : ''}>▲</button>
        <button type="button" class="btn btn-ghost btn-sm" style="padding:1px 6px;font-size:10px;line-height:1.2;" title="เลื่อนลง" onclick="moveRosterRow(${idx}, 1)" ${idx === editingRoster.length - 1 ? 'disabled' : ''}>▼</button>
      </div>

      <div style="display:flex;gap:6px;align-items:center;flex-shrink:0;">
        <label title="คลิกเพื่ออัปโหลดรูปปกติ" style="position:relative;width:56px;height:48px;border-radius:10px;border:1.5px dashed ${r.photo_data ? 'var(--primary)' : 'var(--border)'};background:#fff;display:flex;flex-direction:column;align-items:center;justify-content:center;cursor:pointer;overflow:hidden;">
          ${r.photo_data
            ? `<img id="rosterPrevImg-${idx}" src="${r.photo_data}" alt="" style="width:100%;height:100%;object-fit:cover;${rosterPhotoStyle(posX, posY, flip, r.photo_zoom || 1)}" />`
            : `<span style="font-size:16px;line-height:1;">📷</span><span style="font-size:9px;color:var(--muted);">รูปปกติ</span>`}
          <input type="file" accept="image/*" class="hidden" onchange="uploadRosterPhoto(event, ${idx}, 'photo_data')" />
        </label>

        <div style="position:relative;">
          <label title="คลิกเพื่ออัปโหลดรูปตอนขยาย (อีกท่า)" style="position:relative;width:48px;height:48px;border-radius:10px;border:1.5px dashed ${r.photo_alt_data ? 'var(--primary)' : 'var(--border)'};background:#fff;display:flex;flex-direction:column;align-items:center;justify-content:center;cursor:pointer;overflow:hidden;">
            ${r.photo_alt_data
              ? `<img src="${r.photo_alt_data}" alt="" style="width:100%;height:100%;object-fit:cover;${rosterPhotoStyle(r.photo_alt_pos_x ?? 50, r.photo_alt_pos_y ?? 20, !!r.photo_alt_flip, r.photo_alt_zoom || 1)}" />`
              : `<span style="font-size:16px;line-height:1;">✨</span><span style="font-size:9px;color:var(--primary);">อีกท่า</span>`}
            <input type="file" accept="image/*" class="hidden" onchange="uploadRosterPhoto(event, ${idx}, 'photo_alt_data')" />
          </label>
          ${r.photo_alt_data ? `<button type="button" title="ลบรูปอีกท่า" onclick="editingRoster[${idx}].photo_alt_data='';renderEditingRoster();" style="position:absolute;top:-5px;right:-5px;width:18px;height:18px;border-radius:50%;border:none;background:#ef4444;color:#fff;font-size:10px;cursor:pointer;display:flex;align-items:center;justify-content:center;padding:0;">✕</button>` : ''}
        </div>
      </div>

      <div style="flex:1;min-width:0;display:flex;flex-direction:column;gap:6px;">
        <div style="display:flex;gap:6px;flex-wrap:wrap;">
          <input type="text" placeholder="ชื่อ-สกุล" value="${escapeHtml(r.name)}" style="flex:1.4;min-width:140px;padding:6px 10px;border:1px solid var(--border);border-radius:8px;background:#fff;" onchange="editingRoster[${idx}].name=this.value" />
          <input type="text" list="rosterPosOptions" placeholder="ตำแหน่ง" value="${escapeHtml(r.position || '')}" style="flex:1.1;min-width:120px;padding:6px 10px;border:1px solid var(--border);border-radius:8px;background:#fff;" onchange="editingRoster[${idx}].position=this.value" />
          <select style="width:115px;padding:6px 8px;border:1px solid var(--border);border-radius:8px;background:#fff;" onchange="editingRoster[${idx}].birth_month=parseInt(this.value||'0',10)">
            <option value="0">เดือนเกิด</option>
            ${THAI_MONTHS.slice(1).map((m, mi) => `<option value="${mi+1}" ${r.birth_month === mi+1 ? 'selected' : ''}>${m}</option>`).join('')}
          </select>
          <input type="number" placeholder="วัน" min="1" max="31" value="${r.birth_day || ''}" style="width:62px;padding:6px 8px;border:1px solid var(--border);border-radius:8px;background:#fff;" onchange="editingRoster[${idx}].birth_day=parseInt(this.value||'0',10)" />
        </div>
        <div style="display:flex;gap:6px;flex-wrap:wrap;align-items:center;">
          <input type="text" placeholder="📞 เบอร์โทร (ไม่แสดงหน้าเว็บ)" value="${escapeHtml(r.phone || '')}" style="flex:1;min-width:130px;padding:5px 10px;border:1px solid var(--border);border-radius:8px;background:#fff;font-size:12px;" onchange="editingRoster[${idx}].phone=this.value" />
          <input type="text" placeholder="✉️ E-Mail (ไม่แสดงหน้าเว็บ)" value="${escapeHtml(r.email || '')}" style="flex:1;min-width:140px;padding:5px 10px;border:1px solid var(--border);border-radius:8px;background:#fff;font-size:12px;" onchange="editingRoster[${idx}].email=this.value" />
          ${r.photo_data ? `
            <button type="button" class="btn btn-ghost btn-sm" style="background:#fff;border:1px solid var(--border);font-size:11.5px;padding:4px 10px;" onclick="editExistingRosterPhoto(${idx}, 'photo_data')">
              🎛️ จัดรูปปกติ
            </button>
          ` : ''}
          ${r.photo_alt_data ? `
            <button type="button" class="btn btn-ghost btn-sm" style="background:#fff;border:1px solid var(--primary);color:var(--primary);font-size:11.5px;padding:4px 10px;" onclick="editExistingRosterPhoto(${idx}, 'photo_alt_data')">
              ✨ จัดรูปอีกท่า
            </button>
          ` : ''}
        </div>
      </div>

      <button type="button" class="btn btn-ghost btn-sm" title="ลบรายชื่อนี้" style="color:#ef4444;flex-shrink:0;" onclick="editingRoster.splice(${idx},1); renderEditingRoster();">✕</button>
    </div>
  `;}).join('');
}

function closeRosterPhotoAdjust() {
  const el = document.getElementById('rosterAdjustOverlay');
  if (el) el.remove();
  renderEditingRoster();
}

function openRosterPhotoAdjust(idx, target = 'main') {
  const r = editingRoster[idx];
  const isAlt = target === 'alt';
  const imgSrc = isAlt ? r?.photo_alt_data : r?.photo_data;
  if (!r || !imgSrc) return;
  const old = document.getElementById('rosterAdjustOverlay');
  if (old) old.remove();

  const xKey = isAlt ? 'photo_alt_pos_x' : 'photo_pos_x';
  const yKey = isAlt ? 'photo_alt_pos_y' : 'photo_pos_y';
  const fKey = isAlt ? 'photo_alt_flip' : 'photo_flip';
  const zKey = isAlt ? 'photo_alt_zoom' : 'photo_zoom';

  let posX = (r[xKey] !== undefined && r[xKey] !== null) ? r[xKey] : 50;
  let posY = (r[yKey] !== undefined && r[yKey] !== null) ? r[yKey] : 20;
  let flip = !!r[fKey];
  let zoom = (r[zKey] && r[zKey] >= 1) ? Number(r[zKey]) : 1;

  const stageW = 240;
  const stageH = isAlt ? 240 : 208;

  const overlay = document.createElement('div');
  overlay.id = 'rosterAdjustOverlay';
  overlay.style.cssText = 'position:fixed;inset:0;z-index:9600;background:rgba(15,23,42,0.62);backdrop-filter:blur(4px);display:flex;align-items:center;justify-content:center;padding:16px;';
  overlay.onclick = (e) => { if (e.target === overlay) closeRosterPhotoAdjust(); };
  overlay.innerHTML = `
    <div style="width:min(360px,92vw);background:#fff;border-radius:20px;padding:20px;box-shadow:0 24px 60px rgba(15,23,42,0.35);text-align:center;" onclick="event.stopPropagation()">
      <div style="font-size:14px;font-weight:800;margin-bottom:4px;">${isAlt ? '✨ จัดตำแหน่งรูปตอนขยาย (อีกท่า)' : '🎛️ จัดตำแหน่งรูปปกติในการ์ด'}</div>
      <div style="font-size:11.5px;color:var(--muted);margin-bottom:12px;">ลากบนภาพเพื่อเลื่อน • หมุนลูกกลิ้งเพื่อย่อ-ขยาย</div>

      <div style="width:${stageW}px;margin:0 auto 14px;border:1px solid var(--border);border-radius:14px;overflow:hidden;background:#f8fafc;box-shadow:0 6px 18px rgba(15,23,42,0.08);">
        <div id="adjPhotoStage" style="width:${stageW}px;height:${stageH}px;overflow:hidden;cursor:grab;background:#e2e8f0;user-select:none;touch-action:none;">
          <img id="adjPhotoImg" src="${imgSrc}" draggable="false" style="width:100%;height:100%;object-fit:cover;${rosterPhotoStyle(posX, posY, flip, zoom)}pointer-events:none;" />
        </div>
        <div style="padding:10px 8px;">
          <div style="font-weight:700;font-size:14px;">${escapeHtml(r.name || 'ชื่อ-สกุล')}</div>
          <div style="font-size:12px;color:var(--muted);">${escapeHtml(r.position || 'ตำแหน่ง')}</div>
        </div>
      </div>

      <div style="display:flex;flex-direction:column;gap:8px;text-align:left;margin-bottom:14px;">
        <label style="display:flex;align-items:center;gap:8px;font-size:12px;">
          <span style="width:76px;font-weight:600;">🔍 ย่อ-ขยาย</span>
          <input id="adjSliderZ" type="range" min="1" max="2.5" step="0.05" value="${zoom}" style="flex:1;accent-color:var(--primary);" />
          <span id="adjValZ" style="width:42px;text-align:right;font-weight:700;">${zoom.toFixed(2)}x</span>
        </label>
        <label style="display:flex;align-items:center;gap:8px;font-size:12px;">
          <span style="width:76px;font-weight:600;">↕ สูง-ต่ำ</span>
          <input id="adjSliderY" type="range" min="0" max="100" value="${posY}" style="flex:1;accent-color:var(--primary);" />
          <span id="adjValY" style="width:42px;text-align:right;font-weight:700;">${posY}%</span>
        </label>
        <label style="display:flex;align-items:center;gap:8px;font-size:12px;">
          <span style="width:76px;font-weight:600;">↔ ซ้าย-ขวา</span>
          <input id="adjSliderX" type="range" min="0" max="100" value="${posX}" style="flex:1;accent-color:var(--primary);" />
          <span id="adjValX" style="width:42px;text-align:right;font-weight:700;">${posX}%</span>
        </label>
      </div>

      <div style="display:flex;gap:8px;justify-content:center;flex-wrap:wrap;">
        <button type="button" id="adjFlipBtn" class="btn ${flip ? 'btn-primary' : 'btn-ghost'} btn-sm">⇄ Flip Picture</button>
        <button type="button" id="adjResetBtn" class="btn btn-ghost btn-sm">↺ รีเซ็ต</button>
        <button type="button" class="btn btn-primary btn-sm" onclick="closeRosterPhotoAdjust()">✓ เสร็จสิ้น</button>
      </div>
    </div>
  `;
  document.body.appendChild(overlay);

  const imgEl = document.getElementById('adjPhotoImg');
  const sliderX = document.getElementById('adjSliderX');
  const sliderY = document.getElementById('adjSliderY');
  const sliderZ = document.getElementById('adjSliderZ');
  const valX = document.getElementById('adjValX');
  const valY = document.getElementById('adjValY');
  const valZ = document.getElementById('adjValZ');
  const flipBtn = document.getElementById('adjFlipBtn');
  const stage = document.getElementById('adjPhotoStage');

  const sync = () => {
    r[xKey] = posX;
    r[yKey] = posY;
    r[fKey] = flip;
    r[zKey] = Number(zoom.toFixed(2));
    imgEl.style.objectPosition = `${posX}% ${posY}%`;
    imgEl.style.transformOrigin = `${posX}% ${posY}%`;
    imgEl.style.transform = rosterPhotoTransform(flip, zoom);
    sliderX.value = posX;
    sliderY.value = posY;
    sliderZ.value = zoom;
    valX.textContent = `${posX}%`;
    valY.textContent = `${posY}%`;
    valZ.textContent = `${zoom.toFixed(2)}x`;
    flipBtn.className = `btn ${flip ? 'btn-primary' : 'btn-ghost'} btn-sm`;
  };

  sliderX.oninput = () => { posX = parseInt(sliderX.value, 10); sync(); };
  sliderY.oninput = () => { posY = parseInt(sliderY.value, 10); sync(); };
  sliderZ.oninput = () => { zoom = parseFloat(sliderZ.value) || 1; sync(); };
  flipBtn.onclick = () => { flip = !flip; sync(); };
  document.getElementById('adjResetBtn').onclick = () => { posX = 50; posY = 20; zoom = 1; flip = false; sync(); };
  stage.onwheel = (e) => {
    e.preventDefault();
    const delta = e.deltaY < 0 ? 0.08 : -0.08;
    zoom = Math.max(1, Math.min(2.5, Number((zoom + delta).toFixed(2))));
    sync();
  };

  let dragging = false, startX = 0, startY = 0, origX = posX, origY = posY;
  const onDown = (cx, cy) => { dragging = true; startX = cx; startY = cy; origX = posX; origY = posY; stage.style.cursor = 'grabbing'; };
  const onMove = (cx, cy) => {
    if (!dragging) return;
    const dx = cx - startX;
    const dy = cy - startY;
    posX = Math.max(0, Math.min(100, Math.round(origX - dx * 0.5)));
    posY = Math.max(0, Math.min(100, Math.round(origY - dy * 0.5)));
    sync();
  };
  const onUp = () => { dragging = false; stage.style.cursor = 'grab'; };

  stage.onmousedown = (e) => onDown(e.clientX, e.clientY);
  window.addEventListener('mousemove', (e) => onMove(e.clientX, e.clientY));
  window.addEventListener('mouseup', onUp);
  stage.ontouchstart = (e) => { if (e.touches[0]) onDown(e.touches[0].clientX, e.touches[0].clientY); };
  stage.ontouchmove = (e) => { if (e.touches[0]) { e.preventDefault(); onMove(e.touches[0].clientX, e.touches[0].clientY); } };
  stage.ontouchend = onUp;
}

function moveRosterRow(idx, dir) {
  const target = idx + dir;
  if (target < 0 || target >= editingRoster.length) return;
  const tmp = editingRoster[idx];
  editingRoster[idx] = editingRoster[target];
  editingRoster[target] = tmp;
  renderEditingRoster();
}

function addRosterRow() {
  editingRoster.push({ name: '', position: '', level: '', photo_data: '', photo_alt_data: '', phone: '', email: '', birth_month: 0, birth_day: 0 });
  renderEditingRoster();
}

function applyRosterEditedPhoto(idx, field, dataUrl) {
  if (!editingRoster[idx]) return;
  editingRoster[idx][field] = dataUrl;
  if (field === 'photo_alt_data') {
    editingRoster[idx].photo_alt_pos_x = 50;
    editingRoster[idx].photo_alt_pos_y = 50;
    editingRoster[idx].photo_alt_flip = false;
    editingRoster[idx].photo_alt_zoom = 1;
  } else {
    editingRoster[idx].photo_pos_x = 50;
    editingRoster[idx].photo_pos_y = 50;
    editingRoster[idx].photo_flip = false;
    editingRoster[idx].photo_zoom = 1;
  }
  renderEditingRoster();
}

function uploadRosterPhoto(e, idx, field = 'photo_data') {
  const file = e.target.files[0];
  if (!file) return;
  const isAlt = field === 'photo_alt_data';
  const size = isAlt ? 600 : 400;
  const reader = new FileReader();
  reader.onload = ev => {
    openWardImageEditor({
      title: isAlt ? '✨ แก้ไขรูปบุคลากร (อีกท่า)' : '📷 แก้ไขรูปบุคลากร (รูปปกติ)',
      src: ev.target.result,
      targetW: size,
      targetH: size,
      quality: 0.86,
      onApply: (dataUrl) => applyRosterEditedPhoto(idx, field, dataUrl)
    });
  };
  reader.readAsDataURL(file);
  e.target.value = '';
}

function editExistingRosterPhoto(idx, field = 'photo_data') {
  const r = editingRoster[idx];
  if (!r || !r[field]) return;
  const isAlt = field === 'photo_alt_data';
  const size = isAlt ? 600 : 400;
  openWardImageEditor({
    title: isAlt ? '✨ แก้ไขรูปบุคลากร (อีกท่า)' : '📷 แก้ไขรูปบุคลากร (รูปปกติ)',
    src: r[field],
    targetW: size,
    targetH: size,
    quality: 0.86,
    onApply: (dataUrl) => applyRosterEditedPhoto(idx, field, dataUrl)
  });
}

async function saveWardProfile(e) {
  e.preventDefault();
  const levels = Array.from(document.querySelectorAll('.w-level-row')).map(row => ({
    level: row.querySelector('.lvl-name').value.trim(),
    count: parseInt(row.querySelector('.lvl-count').value || '0', 10),
  })).filter(x => x.level);

  const stats = Array.from(document.querySelectorAll('.w-stat-row')).map(row => ({
    label: row.querySelector('.st-lbl').value.trim(),
    value: row.querySelector('.st-val').value.trim(),
    unit: row.querySelector('.st-unit').value.trim(),
  })).filter(x => x.label);

  const roster = editingRoster.filter(r => r.name && r.name.trim());

  const data = await api(`/api/wards/${encodeURIComponent(currentWardProfile.ward_code)}`, {
    method: 'PUT',
    body: JSON.stringify({
      display_name: document.getElementById('wEditName').value,
      cover_image: editingWardCover,
      leader_name: document.getElementById('wEditLeaderName').value,
      leader_title: document.getElementById('wEditLeaderTitle').value,
      leader_photo: editingLeaderPhoto,
      bed_count: parseInt(document.getElementById('wEditBeds').value || '0', 10),
      patient_groups: document.getElementById('wEditPatients').value,
      history: document.getElementById('wEditHistory').value,
      staff_levels: levels,
      basic_stats: stats,
      roster: roster,
    }),
  });
  if (data.success) {
    closeModal('wardModal');
    renderWardShowcase(currentWardProfile.ward_code);
  } else {
    alert(data.message || 'บันทึกไม่สำเร็จ');
  }
}

function safeJsonParse(str, fallback) {
  try { return JSON.parse(str || ''); } catch { return fallback; }
}

function openModal(id) { document.getElementById(id).classList.remove('hidden'); }
function closeModal(id) { document.getElementById(id).classList.add('hidden'); }

let lbState = { imgs: [], idx: 0 };

function openLightbox(imgs, idx = 0) {
  if (!imgs || !imgs.length) return;
  lbState = { imgs, idx };
  renderLightbox();
  document.getElementById('lightboxModal').classList.remove('hidden');
}

function closeLightbox() {
  document.getElementById('lightboxModal').classList.add('hidden');
}

function stepLightbox(delta) {
  if (!lbState.imgs.length) return;
  lbState.idx = (lbState.idx + delta + lbState.imgs.length) % lbState.imgs.length;
  renderLightbox();
}

function renderLightbox() {
  const { imgs, idx } = lbState;
  document.getElementById('lbImg').src = imgs[idx];
  document.getElementById('lbCounter').textContent = imgs.length > 1 ? `${idx + 1} / ${imgs.length}` : '';
  document.getElementById('lbPrev').classList.toggle('hidden', imgs.length <= 1);
  document.getElementById('lbNext').classList.toggle('hidden', imgs.length <= 1);
}

document.addEventListener('keydown', e => {
  const lb = document.getElementById('lightboxModal');
  if (!lb || lb.classList.contains('hidden')) return;
  if (e.key === 'Escape') closeLightbox();
  else if (e.key === 'ArrowLeft') stepLightbox(-1);
  else if (e.key === 'ArrowRight') stepLightbox(1);
});

function switchAuthTab(tab) {
  document.getElementById('tabLogin').classList.toggle('active', tab === 'login');
  document.getElementById('tabRegister').classList.toggle('active', tab === 'register');
  document.getElementById('loginForm').classList.toggle('hidden', tab !== 'login');
  document.getElementById('registerForm').classList.toggle('hidden', tab !== 'register');
  if (tab === 'register') {
    const sel = document.getElementById('regWard');
    sel.innerHTML = DEPTS[state.dept].wards.map(w => `<option value="${w}">${w}</option>`).join('');
  }
}

async function handleLogin(e) {
  e.preventDefault();
  const errEl = document.getElementById('loginError');
  errEl.classList.add('hidden');
  const data = await api('/api/auth/login', {
    method: 'POST',
    body: JSON.stringify({
      username: document.getElementById('loginUsername').value,
      password: document.getElementById('loginPassword').value,
    }),
  });
  if (!data.success) {
    errEl.textContent = data.message || 'เข้าสู่ระบบไม่สำเร็จ';
    errEl.classList.remove('hidden');
    return;
  }
  state.token = data.token;
  state.user = data.user;
  localStorage.setItem('webchain_token', data.token);
  closeModal('authModal');
  renderAuthArea();
  renderPosts();
}

async function handleRegister(e) {
  e.preventDefault();
  const errEl = document.getElementById('regError');
  errEl.classList.add('hidden');
  const data = await api('/api/auth/register', {
    method: 'POST',
    body: JSON.stringify({
      username: document.getElementById('regUsername').value,
      display_name: document.getElementById('regDisplayName').value,
      department: state.dept,
      ward: document.getElementById('regWard').value,
      password: document.getElementById('regPassword').value,
    }),
  });
  if (!data.success) {
    errEl.textContent = data.message || 'ลงทะเบียนไม่สำเร็จ';
    errEl.classList.remove('hidden');
    return;
  }
  state.token = data.token;
  state.user = data.user;
  localStorage.setItem('webchain_token', data.token);
  closeModal('authModal');
  renderAuthArea();
  renderPosts();
}

function openPostModal(post = null) {
  document.getElementById('editPostId').value = post ? post.id : '';
  document.getElementById('postModalTitle').textContent = post ? 'แก้ไขโพสต์' : 'สร้างโพสต์ใหม่';
  document.getElementById('postCategory').value = post ? post.category : 'activity';
  document.getElementById('postScopeDeptLabel').value = DEPTS[state.dept].name;
  document.getElementById('postTitle').value = post ? post.title : '';
  document.getElementById('postContent').value = post ? post.content : '';
  document.getElementById('postHonoreeName').value = post?.honoree_name || '';
  document.getElementById('postHonoreeWard').value = post?.honoree_ward || '';
  document.getElementById('postAwardTitle').value = post?.award_title || '';
  document.getElementById('postLocation').value = post?.location_or_link || '';
  document.getElementById('postSpeaker').value = post?.speaker || '';
  document.getElementById('postIsPinned').checked = !!post?.is_pinned;
  document.getElementById('postIsUrgent').checked = post?.priority === 'urgent';
  state.uploadedImages = post ? getPostImages(post) : [];
  renderMultiImagePreview();
  toggleCategoryFields();
  renderPostWardCheckboxes(post ? parseWards(post.target_wards) : (state.ward !== 'all' ? [state.ward] : ['all']));
  openModal('postModal');
}

function getPostImages(p) {
  const arr = safeJsonParse(p.images, []);
  if (Array.isArray(arr) && arr.length > 0) return arr;
  return p.image_data ? [p.image_data] : [];
}

function renderMultiImagePreview() {
  const prev = document.getElementById('imagePreviewBox');
  if (!state.uploadedImages.length) {
    prev.innerHTML = '';
    prev.classList.add('hidden');
    return;
  }
  prev.classList.remove('hidden');
  prev.innerHTML = state.uploadedImages.map((src, i) => `
    <div class="thumb-item">
      ${i === 0 ? `<span class="thumb-cover-tag">⭐ ปก</span>` : ''}
      <img src="${src}" />
      <button type="button" class="thumb-del" title="ลบรูปนี้" onclick="state.uploadedImages.splice(${i},1); renderMultiImagePreview();">✕</button>
      <div class="thumb-actions">
        ${i > 0 ? `<button type="button" onclick="setPostCoverImage(${i})" title="ตั้งรูปนี้เป็นหน้าปก">⭐ ปก</button>` : ''}
        <button type="button" onclick="editPostImage(${i})" title="แก้ไขภาพ (ย่อ-ขยาย / เลื่อน / แสง / Flip)">🎨 แก้ไข</button>
      </div>
    </div>
  `).join('');
}

function setPostCoverImage(idx) {
  if (idx <= 0 || idx >= state.uploadedImages.length) return;
  const [picked] = state.uploadedImages.splice(idx, 1);
  state.uploadedImages.unshift(picked);
  renderMultiImagePreview();
}

function editPostImage(idx) {
  const src = state.uploadedImages[idx];
  if (!src) return;
  openWardImageEditor({
    title: idx === 0 ? '🎨 แก้ไขภาพปกโพสต์' : `🎨 แก้ไขภาพประกอบที่ ${idx + 1}`,
    src,
    targetW: 1000,
    targetH: 700,
    quality: 0.85,
    onApply: (dataUrl) => {
      state.uploadedImages[idx] = dataUrl;
      renderMultiImagePreview();
    }
  });
}

function editPost(id) {
  const post = state.posts.find(p => p.id === id);
  if (post) openPostModal(post);
}

function toggleCategoryFields() {
  const cat = document.getElementById('postCategory').value;
  document.getElementById('awardFields').classList.toggle('hidden', cat !== 'award');
  document.getElementById('meetingFields').classList.toggle('hidden', cat !== 'meeting');
}

function renderPostWardCheckboxes(selected = ['all']) {
  const wards = ['all', ...DEPTS[state.dept].wards];
  const box = document.getElementById('postWardsCheckboxes');
  box.innerHTML = wards.map(w => `
    <label class="checkbox-inline">
      <input type="checkbox" name="targetWard" value="${w}" ${selected.includes(w) ? 'checked' : ''} />
      ${w === 'all' ? 'ทุกหอผู้ป่วย' : w}
    </label>
  `).join('');
}

function handleImageUpload(e) {
  const files = Array.from(e.target.files || []);
  if (!files.length) return;
  const remaining = Math.max(0, 5 - state.uploadedImages.length);
  files.slice(0, remaining).forEach(file => {
    const reader = new FileReader();
    reader.onload = ev => {
      const img = new Image();
      img.onload = () => {
        const maxW = 1000;
        const scale = img.width > maxW ? maxW / img.width : 1;
        const canvas = document.createElement('canvas');
        canvas.width = img.width * scale;
        canvas.height = img.height * scale;
        canvas.getContext('2d').drawImage(img, 0, 0, canvas.width, canvas.height);
        if (state.uploadedImages.length < 5) {
          state.uploadedImages.push(canvas.toDataURL('image/jpeg', 0.8));
          renderMultiImagePreview();
        }
      };
      img.src = ev.target.result;
    };
    reader.readAsDataURL(file);
  });
  e.target.value = '';
}

async function handleSavePost(e) {
  e.preventDefault();
  const id = document.getElementById('editPostId').value;
  const checkedWards = Array.from(document.querySelectorAll('input[name="targetWard"]:checked')).map(el => el.value);
  const payload = {
    category: document.getElementById('postCategory').value,
    scope_dept: state.dept,
    target_wards: checkedWards.length ? checkedWards : ['all'],
    title: document.getElementById('postTitle').value,
    content: document.getElementById('postContent').value,
    image_data: state.uploadedImages[0] || '',
    images: state.uploadedImages,
    is_pinned: document.getElementById('postIsPinned').checked,
    priority: document.getElementById('postIsUrgent').checked ? 'urgent' : 'normal',
    honoree_name: document.getElementById('postHonoreeName').value,
    honoree_ward: document.getElementById('postHonoreeWard').value,
    award_title: document.getElementById('postAwardTitle').value,
    event_start_at: document.getElementById('postEventStart').value,
    event_end_at: document.getElementById('postEventEnd').value,
    location_or_link: document.getElementById('postLocation').value,
    speaker: document.getElementById('postSpeaker').value,
  };
  const data = id
    ? await api(`/api/posts/${id}`, { method: 'PUT', body: JSON.stringify(payload) })
    : await api('/api/posts', { method: 'POST', body: JSON.stringify(payload) });

  if (data.success) {
    closeModal('postModal');
    if (state.viewMode !== 'news') {
      setCategory(payload.category || 'all');
    } else {
      fetchPosts();
    }
  } else {
    alert(data.message || 'บันทึกไม่สำเร็จ');
  }
}

async function deletePost(id) {
  if (!confirm('ยืนยันการลบประกาศนี้?')) return;
  const data = await api(`/api/posts/${id}`, { method: 'DELETE' });
  if (data.success) {
    closeModal('detailModal');
    fetchPosts();
  }
}

async function openStatsModal() {
  openModal('statsModal');
  const data = await api('/api/stats');
  if (!data.success) return;
  const rows = (data.top_authors || []).map(a => `
    <tr>
      <td><b>${escapeHtml(a.author_name)}</b> ${a.author_ward ? `(${escapeHtml(a.author_ward)})` : ''}</td>
      <td>${a.post_count}</td>
      <td>${a.award_count}</td>
      <td>${a.meeting_count}</td>
      <td>${a.announce_count}</td>
      <td>👏 ${a.applause_total}</td>
    </tr>
  `).join('');

  document.getElementById('statsBody').innerHTML = `
    <div class="stat-cards">
      <div class="stat-card"><div class="stat-num">${data.total_posts}</div><div class="stat-lbl">โพสต์ทั้งหมด</div></div>
      <div class="stat-card"><div class="stat-num">${data.by_category.award || 0}</div><div class="stat-lbl">🏆 ยินดีรางวัล</div></div>
      <div class="stat-card"><div class="stat-num">${data.by_category.meeting || 0}</div><div class="stat-lbl">📅 ประชุม/อบรม</div></div>
      <div class="stat-card"><div class="stat-num">${data.total_applause}</div><div class="stat-lbl">👏 ปรบมือรวม</div></div>
    </div>
    <h4>สถิติรายผู้โพสต์</h4>
    <table class="data-table">
      <thead><tr><th>ผู้โพสต์</th><th>รวม</th><th>รางวัล</th><th>ประชุม</th><th>ประกาศ</th><th>ปรบมือ</th></tr></thead>
      <tbody>${rows || '<tr><td colspan="6">ยังไม่มีข้อมูล</td></tr>'}</tbody>
    </table>
  `;
}

async function openSyncModal() {
  openModal('syncModal');
  const data = await api('/api/sync/status');
  const isAdmin = state.user && (state.user.role === 'admin' || state.user.role === 'god');
  const nodesHtml = (data.nodes || []).map(n => `
    <tr>
      <td>${escapeHtml(n.hostname)} ${n.is_self ? '(เครื่องนี้)' : ''}</td>
      <td><code>${escapeHtml(n.lan_url)}</code></td>
      <td>${n.posts_count} โพสต์</td>
      <td>${n.last_seen_text}</td>
    </tr>
  `).join('');

  let adminLinksHtml = '';
  if (isAdmin) {
    const lc = state.links.find(l => l.key === 'learnchain') || { url: 'http://localhost:8888' };
    const ds = state.links.find(l => l.key === 'docsearch') || { url: '' };
    adminLinksHtml = `
      <hr style="margin:14px 0;border:none;border-top:1px solid var(--border);" />
      <h4>ตั้งค่าลิงก์ระบบเชื่อมต่อ (Admin)</h4>
      <div class="form-row">
        <div>
          <label>URL ของ LearnChain</label>
          <input type="text" id="cfgLcUrl" value="${escapeHtml(lc.url || '')}" />
        </div>
        <div>
          <label>URL ระบบสืบค้นเอกสาร (DocSearch)</label>
          <input type="text" id="cfgDsUrl" value="${escapeHtml(ds.url || '')}" placeholder="เว้นว่าง = กำลังพัฒนา" />
        </div>
      </div>
      <button class="btn btn-primary btn-sm" style="margin-top:8px;" onclick="saveExternalLinks()">บันทึกลิงก์</button>
      <h4 style="margin-top:14px;">เพิ่มเครื่อง Peer ด้วยตนเอง (LAN)</h4>
      <div style="display:flex;gap:8px;">
        <input type="text" id="peerUrlInput" placeholder="เช่น http://192.168.1.50:8889" />
        <button class="btn btn-ghost btn-sm" onclick="addPeer()">เพิ่ม Peer</button>
      </div>
    `;
  }

  document.getElementById('syncBody').innerHTML = `
    <div class="info-box" style="margin-bottom:12px;">
      <div><b>Node ID:</b> <code>${escapeHtml(data.node_id)}</code> (${escapeHtml(data.hostname)})</div>
      <div><b>LAN URLs:</b> ${(data.lan_urls || []).map(u => `<code>${escapeHtml(u)}</code>`).join(', ')}</div>
      <div><b>สถานะ:</b> ${escapeHtml(data.last_sync_status)} ${data.last_sync_at ? '(' + data.last_sync_at + ')' : ''}</div>
    </div>
    <button class="btn btn-primary btn-sm" onclick="triggerSyncNow()">🔄 ซิงก์ข้อมูลทันที</button>
    <table class="data-table">
      <thead><tr><th>เครื่อง</th><th>LAN URL</th><th>ข้อมูล</th><th>สถานะ</th></tr></thead>
      <tbody>${nodesHtml}</tbody>
    </table>
    ${adminLinksHtml}
  `;
}

async function triggerSyncNow() {
  await api('/api/sync/now', { method: 'POST' });
  setTimeout(() => { openSyncModal(); fetchPosts(); }, 700);
}

async function addPeer() {
  const url = document.getElementById('peerUrlInput').value;
  const data = await api('/api/sync/peers/add', { method: 'POST', body: JSON.stringify({ lan_url: url }) });
  if (data.success) openSyncModal();
  else alert(data.message);
}

async function saveExternalLinks() {
  const lcUrl = document.getElementById('cfgLcUrl').value.trim();
  const dsUrl = document.getElementById('cfgDsUrl').value.trim();
  await api('/api/links', { method: 'POST', body: JSON.stringify({ key: 'learnchain', url: lcUrl, status: lcUrl ? 'active' : 'coming_soon' }) });
  await api('/api/links', { method: 'POST', body: JSON.stringify({ key: 'docsearch', url: dsUrl, status: dsUrl ? 'active' : 'coming_soon' }) });
  fetchLinks();
  alert('บันทึกลิงก์เชื่อมต่อเรียบร้อย');
}

async function openGodModal() {
  openModal('godModal');
  const data = await api('/api/god/status');
  if (!data.success) return;

  const backupsHtml = (data.backups || []).map(b => `
    <tr>
      <td><code>${escapeHtml(b.filename)}</code></td>
      <td>${b.size_kb.toFixed(1)} KB</td>
      <td>${b.created_at}</td>
      <td>
        <button class="btn btn-ghost btn-sm" onclick="downloadBackup('${escapeHtml(b.filename)}')">ดาวน์โหลด</button>
        <button class="btn btn-ghost btn-sm" onclick="restoreBackup('${escapeHtml(b.filename)}')">กู้คืน</button>
      </td>
    </tr>
  `).join('');

  const logsHtml = (data.recent_logs || []).slice(0, 12).map(l => `
    <tr>
      <td>${formatShortDate(l.created_at)}</td>
      <td>${escapeHtml(l.actor_name)}</td>
      <td><code>${escapeHtml(l.action)}</code></td>
      <td>${escapeHtml(l.detail)}</td>
    </tr>
  `).join('');

  document.getElementById('godBody').innerHTML = `
    <div class="stat-cards">
      <div class="stat-card"><div class="stat-num">${data.active_posts}</div><div class="stat-lbl">โพสต์ใช้งาน</div></div>
      <div class="stat-card"><div class="stat-num">${data.deleted_posts}</div><div class="stat-lbl">โพสต์ที่ถูกลบ</div></div>
      <div class="stat-card"><div class="stat-num">${data.total_users}</div><div class="stat-lbl">บัญชีผู้โพสต์</div></div>
      <div class="stat-card"><div class="stat-num">${data.db_size_kb.toFixed(1)} KB</div><div class="stat-lbl">ขนาด SQLite</div></div>
    </div>
    <div style="display:flex;gap:8px;flex-wrap:wrap;margin-bottom:14px;">
      <button class="btn btn-primary btn-sm" onclick="createBackup()">💾 สำรองข้อมูลทันที</button>
      <button class="btn btn-danger btn-sm" onclick="resetSystem()">🧹 ล้างระบบ (Reset)</button>
    </div>
    <h4>ไฟล์สำรองข้อมูล (backups/)</h4>
    <table class="data-table">
      <thead><tr><th>ไฟล์</th><th>ขนาด</th><th>วันที่</th><th>จัดการ</th></tr></thead>
      <tbody>${backupsHtml || '<tr><td colspan="4">ยังไม่มีไฟล์สำรอง</td></tr>'}</tbody>
    </table>
    <h4 style="margin-top:16px;">Audit Logs ล่าสุด</h4>
    <table class="data-table">
      <thead><tr><th>เวลา</th><th>ผู้ดำเนินการ</th><th>กิจกรรม</th><th>รายละเอียด</th></tr></thead>
      <tbody>${logsHtml || '<tr><td colspan="4">ไม่มีบันทึก</td></tr>'}</tbody>
    </table>
  `;
}

async function createBackup() {
  const data = await api('/api/god/backup', { method: 'POST' });
  alert(data.message);
  openGodModal();
}

async function downloadBackup(filename) {
  const res = await fetch(`/api/god/backup/${encodeURIComponent(filename)}`, {
    headers: { Authorization: 'Bearer ' + state.token },
  });
  if (!res.ok) { alert('ดาวน์โหลดไม่สำเร็จ'); return; }
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

async function restoreBackup(filename) {
  if (!confirm(`ยืนยันกู้คืนข้อมูลจาก ${filename}?`)) return;
  const data = await api('/api/god/restore', { method: 'POST', body: JSON.stringify({ filename }) });
  alert(data.message);
  openGodModal();
  fetchPosts();
}

async function resetSystem() {
  const confirmText = prompt('พิมพ์ RESET เพื่อยืนยันการล้างโพสต์และประวัติทั้งหมด:');
  if (confirmText !== 'RESET') return;
  const includeUsers = confirm('ต้องการล้างบัญชีผู้โพสต์ทั่วไปด้วยหรือไม่? (Cancel = เก็บบัญชีไว้)');
  const data = await api('/api/god/reset', {
    method: 'POST',
    body: JSON.stringify({ confirm_text: 'RESET', include_users: includeUsers }),
  });
  alert(data.message);
  openGodModal();
  fetchPosts();
}

async function fetchVersion() {
  const data = await api('/api/version');
  if (data.version) document.getElementById('versionText').textContent = `WebChain ${data.version}`;
}

function parseWards(str) {
  try { return JSON.parse(str || '["all"]'); } catch { return ['all']; }
}

function formatDate(iso) {
  if (!iso) return '';
  return new Date(iso).toLocaleString('th-TH', { dateStyle: 'medium', timeStyle: 'short' });
}

function formatShortDate(iso) {
  if (!iso) return '';
  return new Date(iso).toLocaleDateString('th-TH', { day: 'numeric', month: 'short' });
}

function linkify(text) {
  const safe = escapeHtml(text);
  if (safe.startsWith('http://') || safe.startsWith('https://')) {
    return `<a href="${safe}" target="_blank" rel="noopener">${safe}</a>`;
  }
  return safe;
}

function escapeHtml(str) {
  return String(str || '').replace(/[&<>"']/g, s => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
  }[s]));
}

init();
