let currentMode = 'chat';

function escapeHtml(s) {
  return String(s || '').replace(/[&<>"']/g, m => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
  }[m]));
}

function setMode(mode) {
  currentMode = mode;
  document.querySelectorAll('.mode-tab').forEach(btn => {
    btn.classList.toggle('active', btn.dataset.mode === mode);
  });
}

function toggleSettings() {
  document.getElementById('settingsCard').classList.toggle('hidden');
}

async function loadNPUStatus(forceRefresh = false) {
  try {
    const res = await fetch('/api/npu/status' + (forceRefresh ? '?refresh=1' : ''));
    const data = await res.json();
    const hw = data.hardware || {};
    const cfg = data.config || {};

    document.getElementById('cfgEndpoint').value = cfg.endpoint_url || '';
    document.getElementById('cfgModel').value = cfg.model_name || '';
    document.getElementById('cfgSysNote').value = cfg.system_note || '';

    document.getElementById('hwDeviceName').textContent = hw.device_name || 'Unknown Compute';
    const badges = [
      `<span class="hw-badge">Arch: ${escapeHtml(hw.arch || '-')}</span>`,
      `<span class="hw-badge">DirectML: ${hw.directml_ready ? '✅ Ready' : '❌ Not found'}</span>`,
      `<span class="hw-badge" style="${hw.qnn_ready ? 'background:#ecfdf5;border-color:#a7f3d0;color:#047857;' : ''}">QNN HTP (NPU): ${hw.qnn_ready ? '✅ Ready' : '❌ Not installed'}</span>`,
      `<span class="hw-badge">NPU Status: ${escapeHtml(hw.device_status || '-')}</span>`,
    ];
    if (hw.active_engine) {
      badges.push(`<span class="hw-badge" style="background:#ecfdf5;border-color:#a7f3d0;color:#047857;">🟢 ${escapeHtml(hw.active_engine)} (${escapeHtml(hw.active_model || 'auto')})</span>`);
    }
    document.getElementById('hwBadges').innerHTML = badges.join('');

    const pill = document.getElementById('npuPill');
    const guide = document.getElementById('hwGuide');
    if (hw.active_engine) {
      pill.className = 'pill pill-ok';
      pill.textContent = `🟢 NPU Engine: ${hw.active_engine}`;
      guide.innerHTML = `<strong>พร้อมประมวลผล Local AI แล้ว</strong><br/>Endpoint: <code>${escapeHtml(hw.active_endpoint)}</code><br/>Model: <code>${escapeHtml(hw.active_model || 'default')}</code>`;
    } else if (hw.qnn_ready) {
      pill.className = 'pill pill-ok';
      pill.textContent = '🟢 QNN Hexagon NPU พร้อมใช้งาน';
      guide.innerHTML = `<strong>QNN HTP Driver พร้อมบนเครื่องนี้แล้ว</strong><br/>กดปุ่ม <b>⚡ ทดสอบรันงานเข้า NPU จริง</b> เพื่อรันโมเดล ONNX บน Hexagon NPU ได้ทันที`;
    } else {
      pill.className = 'pill pill-warn';
      pill.textContent = '🟡 พบ NPU (รอเชื่อมต่อ AI Server)';
      guide.innerHTML = `<strong>เปิดใช้ Local AI:</strong><br/>เลือกโมเดล <code>.gguf</code> แล้วกด <b>▶ เปิดใช้ llama.cpp</b> ได้เลย`;
    }

    const stopBtn = document.getElementById('btnStopLlama');
    if (stopBtn) stopBtn.classList.toggle('hidden', !hw.llama_running);

    const npuBtn = document.getElementById('btnToggleNPULLM');
    if (npuBtn && hw.qnn_ready && hw.npu_llm_model) {
      npuBtn.classList.remove('hidden');
      npuBtn.dataset.running = hw.npu_llm_running ? '1' : '0';
      if (hw.npu_llm_running) {
        npuBtn.textContent = `⏹ ปิด NPU LLM (${hw.npu_llm_model})`;
        npuBtn.style.background = '#e11d48';
        npuBtn.style.borderColor = '#e11d48';
        npuBtn.style.color = '#fff';
      } else {
        npuBtn.textContent = `🧠 เปิด NPU LLM (${hw.npu_llm_model})`;
        npuBtn.style.background = '#059669';
        npuBtn.style.borderColor = '#059669';
        npuBtn.style.color = '#fff';
      }
    }
  } catch (e) {
    document.getElementById('hwDeviceName').textContent = 'ไม่สามารถตรวจสอบสถานะ NPU ได้';
  }
}

async function toggleNPULLM() {
  const btn = document.getElementById('btnToggleNPULLM');
  if (!btn) return;
  const isRunning = btn.dataset.running === '1';
  btn.disabled = true;
  btn.textContent = isRunning ? '⏳ กำลังปิด NPU LLM...' : '⏳ กำลังเปิด NPU LLM...';
  try {
    await fetch(isRunning ? '/api/npu/llm/stop' : '/api/npu/llm/start', { method: 'POST' });
    await loadNPUStatus(true);
  } finally {
    btn.disabled = false;
  }
}

async function loadLlamaModels() {
  try {
    const res = await fetch('/api/llama/models');
    const data = await res.json();
    const bar = document.getElementById('llamaBar');
    const sel = document.getElementById('llamaModelSelect');
    if (!data.llama_exe || !data.models || data.models.length === 0) return;
    bar.classList.remove('hidden');
    sel.innerHTML = data.models.map(m =>
      `<option value="${escapeHtml(m.path)}">${escapeHtml(m.name)} (${escapeHtml(m.size)})</option>`
    ).join('');
  } catch {}
}

async function startLlamaServer() {
  const sel = document.getElementById('llamaModelSelect');
  const btn = document.getElementById('btnStartLlama');
  if (!sel || !sel.value) return;
  btn.disabled = true;
  btn.textContent = '⏳ กำลังโหลดโมเดล...';
  try {
    const res = await fetch('/api/llama/start', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ model_path: sel.value })
    });
    const data = await res.json();
    if (!res.ok) {
      alert(data.error || 'เปิด llama.cpp ไม่สำเร็จ');
    } else {
      await loadNPUStatus(true);
    }
  } finally {
    btn.disabled = false;
    btn.textContent = '▶ เปิดใช้ llama.cpp';
  }
}

async function stopLlamaServer() {
  await fetch('/api/llama/stop', { method: 'POST' });
  await loadNPUStatus(true);
}

async function runNPUBenchmark() {
  const btn = document.getElementById('btnBenchNPU');
  const out = document.getElementById('npuBenchResult');
  btn.disabled = true;
  btn.textContent = '⏳ กำลังยิงงานเข้า Hexagon NPU...';
  out.textContent = '';
  try {
    const res = await fetch('/api/npu/benchmark', { method: 'POST' });
    const data = await res.json();
    if (!res.ok) {
      out.textContent = '❌ ' + (data.error || 'ทดสอบไม่สำเร็จ');
    } else {
      out.innerHTML = `<strong style="color:#047857;">✅ ${escapeHtml(data.provider)}</strong> • ${data.iterations} รอบ ใช้เวลา ${data.total_ms} ms (${data.avg_ms} ms/รอบ)`;
    }
  } catch (err) {
    out.textContent = '❌ เชื่อมต่อไม่สำเร็จ';
  } finally {
    btn.disabled = false;
    btn.textContent = '⚡ ทดสอบรันงานเข้า NPU จริง';
  }
}

async function saveConfig() {
  await fetch('/api/config', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      endpoint_url: document.getElementById('cfgEndpoint').value.trim(),
      model_name: document.getElementById('cfgModel').value.trim(),
      system_note: document.getElementById('cfgSysNote').value.trim()
    })
  });
  toggleSettings();
  await loadNPUStatus(true);
}

async function sendPrompt() {
  const promptEl = document.getElementById('promptInput');
  const prompt = promptEl.value.trim();
  if (!prompt) return;

  const btn = document.getElementById('btnSend');
  const replyBox = document.getElementById('replyBox');
  const latBadge = document.getElementById('latencyBadge');
  btn.disabled = true;
  btn.textContent = '⏳ กำลังประมวลผล...';
  replyBox.classList.remove('hidden');
  replyBox.textContent = 'กำลังส่งงานให้ NPU...';

  try {
    const res = await fetch('/api/chat', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ mode: currentMode, prompt })
    });
    const data = await res.json();
    if (!res.ok) {
      replyBox.textContent = '❌ ' + (data.error || 'เกิดข้อผิดพลาด');
    } else {
      replyBox.textContent = data.reply || '';
      latBadge.textContent = `${data.engine || ''} • ${data.latency_ms || 0} ms`;
      loadHistory();
    }
  } catch (err) {
    replyBox.textContent = '❌ เชื่อมต่อไม่สำเร็จ';
  } finally {
    btn.disabled = false;
    btn.textContent = '🚀 ประมวลผลด้วย AI';
  }
}

async function loadHistory() {
  try {
    const res = await fetch('/api/history');
    const data = await res.json();
    const logs = data.logs || [];
    const list = document.getElementById('historyList');
    if (logs.length === 0) {
      list.innerHTML = '<div class="muted">ยังไม่มีประวัติ</div>';
      return;
    }
    list.innerHTML = logs.map(l => `
      <div class="hist-item">
        <div class="hist-meta">[${escapeHtml(l.mode)}] ${escapeHtml(l.engine)} • ${l.latency_ms} ms</div>
        <div><strong>ถาม:</strong> ${escapeHtml(l.prompt.slice(0, 120))}${l.prompt.length > 120 ? '...' : ''}</div>
        <div style="margin-top:4px;color:#334155;"><strong>ตอบ:</strong> ${escapeHtml(l.response.slice(0, 220))}${l.response.length > 220 ? '...' : ''}</div>
      </div>
    `).join('');
  } catch {}
}

loadNPUStatus(false);
loadLlamaModels();
loadHistory();
