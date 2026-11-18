const BASE = '/b/docchain';

document.addEventListener('DOMContentLoaded', () => {
  loadMe();
  loadDocs();
});

async function loadMe() {
  try {
    const res = await fetch(`${BASE}/api/me`);
    const data = await res.json();
    const u = data.user;
    if (u) {
      document.getElementById('userBadge').textContent = `👤 ${u.display_name || u.username}`;
      document.getElementById('btnUpload').classList.remove('hidden');
    } else {
      document.getElementById('userBadge').innerHTML = `<a href="/" style="color:#38bdf8;text-decoration:none;">เข้าสู่ระบบที่ CoreChain</a>`;
    }
  } catch {}
}

async function loadDocs() {
  const q = encodeURIComponent(document.getElementById('searchInput').value);
  const cat = encodeURIComponent(document.getElementById('catFilter').value);
  const res = await fetch(`${BASE}/api/docs?q=${q}&category=${cat}`);
  const data = await res.json();
  const docs = data.documents || [];
  const grid = document.getElementById('docsGrid');
  if (docs.length === 0) {
    grid.innerHTML = `<div class="doc-card">ไม่พบเอกสารที่ตรงกับเงื่อนไข</div>`;
    return;
  }
  grid.innerHTML = docs.map(d => `
    <div class="doc-card">
      ${d.cover_url ? `<img class="doc-cover" src="${d.cover_url}" alt="cover" />` : ''}
      <div class="doc-badges">
        <span class="badge">${esc(d.category)}</span>
        ${d.doc_code ? `<span class="badge">${esc(d.doc_code)}</span>` : ''}
        <span class="badge">v${esc(d.version)}</span>
        <span class="badge">${esc(d.ward)}</span>
      </div>
      <div class="doc-title">${esc(d.title)}</div>
      <p class="doc-desc">${esc(d.description || '')}</p>
      ${d.content_hash ? `<div class="doc-hash">SHA256: ${d.content_hash.slice(0, 20)}...</div>` : ''}
      <div class="doc-actions">
        ${d.file_name ? `<a class="btn btn-primary" href="${BASE}/api/docs/${d.id}/file" target="_blank">👁 เปิดอ่าน</a>
                         <a class="btn btn-secondary" href="${BASE}/api/docs/${d.id}/file?download=1">⬇ ดาวน์โหลด</a>` : '<span style="font-size:12px;color:#64748b;">ไม่มีไฟล์แนบ</span>'}
      </div>
    </div>
  `).join('');
}

function toggleModal(show) {
  document.getElementById('uploadModal').classList.toggle('hidden', !show);
}

async function submitDoc(e) {
  e.preventDefault();
  const fd = new FormData();
  fd.append('doc_code', document.getElementById('docCode').value);
  fd.append('title', document.getElementById('docTitle').value);
  fd.append('category', document.getElementById('docCategory').value);
  fd.append('version', document.getElementById('docVersion').value);
  fd.append('ward', document.getElementById('docWard').value);
  fd.append('tags', document.getElementById('docTags').value);
  fd.append('description', document.getElementById('docDesc').value);
  const f = document.getElementById('docFile').files[0];
  if (f) fd.append('file', f);
  const cov = document.getElementById('docCover').files[0];
  if (cov) fd.append('cover', cov);

  const res = await fetch(`${BASE}/api/docs`, { method: 'POST', body: fd });
  if (!res.ok) {
    const err = await res.json();
    alert(err.error || 'อัปโหลดไม่สำเร็จ');
    return;
  }
  toggleModal(false);
  loadDocs();
}

function esc(s) {
  return String(s || '').replace(/[&<>"']/g, m => ({ '&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;' }[m]));
}
