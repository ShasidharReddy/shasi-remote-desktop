/* ── Secure System  –  app.js ─────────────────────────────────────────────── */

'use strict';

const ws     = new WebSocket(`ws://${location.host}/ws`);

let sessionID  = null;
let machineID  = null;
let pendingFor = null;   // session ID of incoming viewer pending accept/deny

// FPS tracking
let frameCount = 0;
let lastFpsTs  = Date.now();

/* ─── WebSocket events ─────────────────────────────────────────────────────── */

ws.onopen = () => {
  setDot('conn-dot', 'online');
};

ws.onclose = () => {
  setDot('conn-dot', 'offline');
  setStatus('Disconnected — reload to reconnect', 'offline');
};

ws.onerror = () => {
  setStatus('WebSocket error', 'offline');
};

ws.onmessage = (ev) => {
  let msg;
  try { msg = JSON.parse(ev.data); } catch { return; }
  handle(msg);
};

function send(obj) {
  if (ws.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify(obj));
  }
}

/* ─── message handler ──────────────────────────────────────────────────────── */

function handle(msg) {
  switch (msg.type) {

    case 'welcome':
      sessionID = msg.session_id;
      machineID = msg.machine_id;
      document.getElementById('machine-id').textContent = machineID;
      document.getElementById('hint-ip').textContent = location.hostname + ':' + location.port;
      // register as host so we receive incoming_request events
      send({ type: 'register_host' });
      setStatus('Ready — waiting for connections', 'online');
      setDot('status-dot', 'online');
      break;

    case 'incoming_request':
      pendingFor = msg.for_session;
      document.getElementById('req-ip').textContent = msg.from_ip;
      showModal(true);
      break;

    case 'connected':
      showViewer();
      break;

    case 'denied':
      setConnStatus('❌ Connection denied by remote host.');
      enableConnectBtn(true);
      break;

    case 'screen_frame':
      renderFrame(msg.data, msg.width, msg.height);
      break;

    case 'host_status':
      updateViewers(msg.count);
      break;

    case 'file_end':
      toast(`📁 File received: ${msg.file_name}`);
      break;

    case 'error':
      toast('⚠ ' + msg.error);
      break;
  }
}

/* ─── host controls ────────────────────────────────────────────────────────── */

function acceptConn() {
  send({ type: 'accept', for_session: pendingFor });
  showModal(false);
  setStatus('Sharing screen…', 'online');
}

function denyConn() {
  send({ type: 'deny', for_session: pendingFor });
  showModal(false);
}

function copyID() {
  navigator.clipboard.writeText(machineID).then(() => toast('ID copied!'));
}

function updateViewers(n) {
  const badge = document.getElementById('viewer-badge');
  document.getElementById('viewer-count').textContent = n;
  if (n > 0) {
    badge.classList.remove('hidden');
    setStatus(`Sharing screen (${n} viewer${n > 1 ? 's' : ''})`, 'online');
  } else {
    badge.classList.add('hidden');
    setStatus('Ready — waiting for connections', 'online');
  }
}

/* ─── viewer controls ──────────────────────────────────────────────────────── */

function connectRemote() {
  const id = document.getElementById('remote-id').value.trim();
  if (!id) { toast('Enter a remote ID first.'); return; }

  document.getElementById('viewing-id').textContent = id;
  enableConnectBtn(false);
  setConnStatus('⏳ Waiting for host to accept…');
  setDot('conn-dot', 'waiting');

  send({ type: 'view_request', target_id: id });
}

function showViewer() {
  document.getElementById('home').classList.add('hidden');
  document.getElementById('viewer').classList.remove('hidden');

  const canvas = document.getElementById('remote-canvas');
  canvas.addEventListener('mousemove', sendMouseMove);
  canvas.addEventListener('click',     sendClick);
  document.addEventListener('keydown',  sendKey);

  requestAnimationFrame(fpsLoop);
}

function disconnectViewer() {
  send({ type: 'disconnect' });
  document.getElementById('viewer').classList.add('hidden');
  document.getElementById('home').classList.remove('hidden');

  const canvas = document.getElementById('remote-canvas');
  canvas.removeEventListener('mousemove', sendMouseMove);
  canvas.removeEventListener('click',     sendClick);
  document.removeEventListener('keydown', sendKey);

  enableConnectBtn(true);
  setConnStatus('');
  setDot('conn-dot', 'online');
  setStatus('Ready — waiting for connections', 'online');
}

/* ─── input forwarding ─────────────────────────────────────────────────────── */

function sendMouseMove(e) {
  const r = e.target.getBoundingClientRect();
  const sw = e.target.width  / r.width;
  const sh = e.target.height / r.height;
  send({ type:'input', itype:'mouse_move',
    x: Math.round((e.clientX - r.left) * sw),
    y: Math.round((e.clientY - r.top)  * sh) });
}

function sendClick(e) {
  const r = e.target.getBoundingClientRect();
  send({ type:'input', itype:'mouse_click',
    x: Math.round(e.clientX - r.left),
    y: Math.round(e.clientY - r.top), button: 1 });
}

function sendKey(e) {
  send({ type:'input', itype:'key_press', key: e.key });
}

/* ─── screen rendering ─────────────────────────────────────────────────────── */

function renderFrame(b64, w, h) {
  const canvas = document.getElementById('remote-canvas');
  const ctx    = canvas.getContext('2d');
  const img    = new Image();
  img.onload   = () => {
    if (canvas.width !== w)  canvas.width  = w;
    if (canvas.height !== h) canvas.height = h;
    ctx.drawImage(img, 0, 0);
  };
  img.src = 'data:image/jpeg;base64,' + b64;
  frameCount++;
}

function fpsLoop() {
  const now = Date.now();
  if (now - lastFpsTs >= 1000) {
    document.getElementById('fps-label').textContent = frameCount + ' fps';
    frameCount = 0;
    lastFpsTs  = now;
  }
  requestAnimationFrame(fpsLoop);
}

/* ─── file transfer ────────────────────────────────────────────────────────── */

function dropFiles(e) {
  e.preventDefault();
  document.getElementById('drop-zone').classList.remove('over');
  [...e.dataTransfer.files].forEach(sendFile);
}

function pickFiles(e) {
  [...e.target.files].forEach(sendFile);
  e.target.value = '';
}

async function sendFile(file) {
  const id        = 'f_' + Date.now() + '_' + Math.random().toString(36).slice(2);
  const CHUNK     = 256 * 1024;
  const item      = addTransferItem(file.name, file.size);

  send({ type:'file_start', file_id:id, file_name:file.name, file_size:file.size });

  let offset = 0;
  while (offset < file.size) {
    const blob = file.slice(offset, offset + CHUNK);
    const ab   = await blob.arrayBuffer();
    send({ type:'file_chunk', file_id:id, data: bufToB64(ab) });
    offset += CHUNK;
    item.setProgress(offset / file.size);
    await tick(5);
  }

  send({ type:'file_end', file_id:id });
  item.setProgress(1);
  setTimeout(() => item.remove(), 3000);
}

function addTransferItem(name, size) {
  const list = document.getElementById('transfer-list');
  const div  = document.createElement('div');
  div.className = 'transfer-item';
  div.innerHTML = `
    <span class="transfer-name" title="${name}">${name}</span>
    <span class="transfer-size">${fmtSize(size)}</span>
    <div class="transfer-bar"><div class="transfer-fill" style="width:0%"></div></div>
    <span class="transfer-pct">0%</span>`;
  list.appendChild(div);
  const fill = div.querySelector('.transfer-fill');
  const pct  = div.querySelector('.transfer-pct');
  return {
    setProgress(p) {
      const v = Math.round(p * 100);
      fill.style.width = v + '%';
      pct.textContent  = v + '%';
    },
    remove() { div.remove(); }
  };
}

function bufToB64(buf) {
  const bytes = new Uint8Array(buf);
  let s = '';
  for (let i = 0; i < bytes.length; i++) s += String.fromCharCode(bytes[i]);
  return btoa(s);
}

function fmtSize(n) {
  if (n < 1024)        return n + ' B';
  if (n < 1024**2)     return (n/1024).toFixed(1) + ' KB';
  if (n < 1024**3)     return (n/1024**2).toFixed(1) + ' MB';
  return (n/1024**3).toFixed(2) + ' GB';
}

const tick = ms => new Promise(r => setTimeout(r, ms));

/* ─── UI helpers ───────────────────────────────────────────────────────────── */

function setStatus(txt, kind) {
  document.getElementById('status-text').textContent = txt;
  const dot = document.getElementById('status-dot');
  dot.className = 'dot dot-' + kind;
}

function setDot(id, kind) {
  document.getElementById(id).className = 'dot dot-' + kind;
}

function setConnStatus(txt) {
  document.getElementById('connect-status').textContent = txt;
}

function enableConnectBtn(on) {
  document.getElementById('connect-btn').disabled = !on;
}

function showModal(on) {
  document.getElementById('modal').classList.toggle('hidden', !on);
}

let toastTimer;
function toast(msg) {
  const el = document.getElementById('toast');
  el.textContent = msg;
  el.classList.remove('hidden');
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => el.classList.add('hidden'), 3000);
}
