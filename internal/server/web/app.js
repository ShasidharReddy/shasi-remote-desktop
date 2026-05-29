const WS_URL = `${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/ws`;
const CHUNK_SIZE = 256 * 1024;
const RECONNECT_DELAY_MS = 3000;

let ws = null;
let sessionId = getOrCreateId();
let role = null;
let remoteId = null;
let connected = false;
let frameCount = 0;
let bytesThisSecond = 0;
let lastFpsTick = Date.now();
let pollTimer = null;

const outgoing = new Map();
const incoming = new Map();

const yourIdEl = document.getElementById('your-id');
const statusDot = document.getElementById('status-dot');
const statusText = document.getElementById('status-text');
const connectBtn = document.getElementById('connect-btn');
const disconnectBtn = document.getElementById('disconnect-btn');
const remoteIdInput = document.getElementById('remote-id-input');
const remotePassInput = document.getElementById('remote-pass-input');
const hostPasswordInput = document.getElementById('host-password');
const welcomeScreen = document.getElementById('welcome-screen');
const remoteView = document.getElementById('remote-view');
const hostView = document.getElementById('host-view');
const remoteCanvas = document.getElementById('remote-canvas');
const ctx = remoteCanvas.getContext('2d');
const transferList = document.getElementById('transfer-list');
const dropZone = document.getElementById('drop-zone');
const peerCount = document.getElementById('peer-count');
const viewStats = document.getElementById('view-stats');
const streamStats = document.getElementById('stream-stats');
const serverStatusEl = document.getElementById('server-status');
const viewerIdDisplay = document.getElementById('viewer-id-display');
const serverAddr = document.getElementById('server-addr');
const copyIdBtn = document.getElementById('copy-id-btn');
const tbDisconnect = document.getElementById('tb-disconnect');
const tbFiles = document.getElementById('tb-files');
const tbFullscreen = document.getElementById('tb-fullscreen');
const tbCAD = document.getElementById('tb-cad');
const stopShareBtn = document.getElementById('stop-share-btn');
const pickFilesBtn = document.getElementById('pick-files-btn');
const pickFolderBtn = document.getElementById('pick-folder-btn');

serverAddr.textContent = location.host;
yourIdEl.textContent = formatId(sessionId);

function getOrCreateId() {
  let id = localStorage.getItem('secure-system-id');
  if (!/^\d{9}$/.test(id || '')) {
    id = Array.from({ length: 9 }, () => Math.floor(Math.random() * 10)).join('');
    localStorage.setItem('secure-system-id', id);
  }
  return id;
}

function formatId(raw) {
  const digits = String(raw || '').replace(/\D/g, '').slice(0, 9);
  return digits.replace(/(\d{3})(?=\d)/g, '$1 ');
}

function rawId(val) {
  return String(val || '').replace(/\D/g, '').slice(0, 9);
}

function setStatus(message, state = 'idle') {
  statusText.textContent = message;
  statusDot.className = `status-dot ${state}`;
}

function setServerStatus(label, color) {
  serverStatusEl.textContent = label;
  serverStatusEl.style.color = color;
}

function showView(view) {
  welcomeScreen.style.display = view === 'welcome' ? '' : 'none';
  remoteView.style.display = view === 'remote' ? '' : 'none';
  hostView.style.display = view === 'host' ? '' : 'none';
}

function safeParsePayload(payload) {
  if (!payload) return null;
  if (typeof payload === 'string') {
    try {
      return JSON.parse(payload);
    } catch {
      return null;
    }
  }
  return payload;
}

function sendMessage(type, payload = undefined, agentId = remoteId || sessionId) {
  if (!ws || ws.readyState !== WebSocket.OPEN) return false;
  const message = { type };
  if (agentId) message.agent_id = agentId;
  if (payload !== undefined) message.payload = payload;
  ws.send(JSON.stringify(message));
  return true;
}

function register(roleName, agentId) {
  return sendMessage('register', { agent_id: agentId, role: roleName }, agentId);
}

function connectWS() {
  setStatus('Connecting to relay...');
  setServerStatus('● Connecting...', '#E0E0E0');
  ws = new WebSocket(WS_URL);

  ws.onopen = () => {
    register('agent', sessionId);
    setStatus('Connected to server');
    setServerStatus('● Online', '#00C853');
    yourIdEl.textContent = formatId(sessionId);
    startPollingStatus();
  };

  ws.onclose = () => {
    connected = false;
    connectBtn.disabled = false;
    setStatus('Disconnected from server', 'error');
    setServerStatus('● Offline', '#E94560');
    stopPollingStatus();
    showView('welcome');
    setTimeout(connectWS, RECONNECT_DELAY_MS);
  };

  ws.onerror = () => setStatus('Server connection error', 'error');

  ws.onmessage = (event) => {
    let msg;
    try {
      msg = JSON.parse(event.data);
    } catch {
      return;
    }
    handleMessage(msg);
  };
}

function startPollingStatus() {
  stopPollingStatus();
  const poll = () => {
    fetch('/status')
      .then((response) => response.json())
      .then((data) => {
        const count = Number(data.connected_clients || 0);
        peerCount.textContent = `${count} peer${count === 1 ? '' : 's'}`;
      })
      .catch(() => {});
  };
  poll();
  pollTimer = window.setInterval(poll, 5000);
}

function stopPollingStatus() {
  if (pollTimer) {
    window.clearInterval(pollTimer);
    pollTimer = null;
  }
}

function handleMessage(msg) {
  switch (msg.type) {
    case 'screen_frame':
      renderFrame(safeParsePayload(msg.payload));
      break;
    case 'input':
      role = 'host';
      connected = true;
      viewerIdDisplay.textContent = formatId(msg.agent_id || remoteId || '');
      streamStats.textContent = 'Viewer control events are being relayed.';
      setStatus('Viewer connected', 'connected');
      showView('host');
      break;
    case 'file_transfer':
      handleFileTransferStart(safeParsePayload(msg.payload));
      break;
    case 'file_chunk':
      handleFileChunk(safeParsePayload(msg.payload));
      break;
    case 'file_end':
      handleFileEnd(safeParsePayload(msg.payload));
      break;
    case 'ping':
      sendMessage('pong', undefined, sessionId);
      break;
    case 'pong':
      break;
    case 'error':
      setStatus('Server reported an error', 'error');
      break;
    default:
      break;
  }
}

function base64ToUint8Array(base64) {
  const binary = atob(base64 || '');
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes;
}

function renderFrame(payload) {
  if (!payload || !payload.data) return;
  const bytes = base64ToUint8Array(payload.data);
  const blob = new Blob([bytes], { type: 'image/jpeg' });
  createImageBitmap(blob).then((bitmap) => {
    remoteCanvas.width = bitmap.width;
    remoteCanvas.height = bitmap.height;
    ctx.drawImage(bitmap, 0, 0);
    bitmap.close();

    frameCount += 1;
    bytesThisSecond += bytes.byteLength;
    const now = Date.now();
    if (now - lastFpsTick >= 1000) {
      viewStats.textContent = `${frameCount} fps · ${formatBytes(bytesThisSecond)}/s`;
      frameCount = 0;
      bytesThisSecond = 0;
      lastFpsTick = now;
    }
  }).catch(() => {});
}

function attachInputCapture() {
  remoteCanvas.tabIndex = 0;
  remoteCanvas.focus({ preventScroll: true });
}

function mousePos(event) {
  const rect = remoteCanvas.getBoundingClientRect();
  return {
    x: Math.round((event.clientX - rect.left) * remoteCanvas.width / Math.max(rect.width, 1)),
    y: Math.round((event.clientY - rect.top) * remoteCanvas.height / Math.max(rect.height, 1)),
  };
}

function sendInput(type, data) {
  if (!connected || !remoteId) return;
  sendMessage('input', { type, ...data }, remoteId);
}

remoteCanvas.addEventListener('mousemove', (event) => {
  if (!connected) return;
  sendInput('mouse_move', mousePos(event));
});

remoteCanvas.addEventListener('mousedown', (event) => {
  if (!connected) return;
  remoteCanvas.focus({ preventScroll: true });
  sendInput('mouse_click', { ...mousePos(event), button: event.button + 1 });
});

remoteCanvas.addEventListener('contextmenu', (event) => event.preventDefault());
remoteCanvas.addEventListener('wheel', (event) => {
  if (!connected) return;
  event.preventDefault();
  sendInput('scroll', { deltaX: Math.round(event.deltaX), deltaY: Math.round(event.deltaY) });
}, { passive: false });

window.addEventListener('keydown', (event) => {
  if (document.activeElement !== remoteCanvas || !connected) return;
  event.preventDefault();
  sendInput('key_press', { key: event.key, key_code: event.keyCode });
});

window.addEventListener('keyup', (event) => {
  if (document.activeElement !== remoteCanvas || !connected) return;
  event.preventDefault();
  sendInput('key_release', { key: event.key, key_code: event.keyCode });
});

function startConnect() {
  const id = rawId(remoteIdInput.value);
  if (!/^\d{9}$/.test(id)) {
    setStatus('Enter a valid 9-digit ID', 'error');
    return;
  }
  if (!ws || ws.readyState !== WebSocket.OPEN) {
    setStatus('Server is still reconnecting', 'error');
    return;
  }

  connectBtn.disabled = true;
  remoteId = id;
  role = 'viewer';
  connected = true;
  showView('remote');
  attachInputCapture();

  register('viewer', id);
  setStatus(`Connected — waiting for frames from ${formatId(id)}`, 'connected');
  viewStats.textContent = remotePassInput.value ? 'Password provided · waiting for frames' : 'Waiting for frames';
}

function doDisconnect() {
  connected = false;
  role = null;
  remoteId = null;
  connectBtn.disabled = false;
  remotePassInput.value = '';
  showView('welcome');
  setStatus('Ready', 'idle');
  viewStats.textContent = '0 fps · 0 KB/s';
  streamStats.textContent = hostPasswordInput.value ? 'Session protected and idle.' : 'Waiting for viewer...';
  viewerIdDisplay.textContent = '—';
  if (ws && ws.readyState === WebSocket.OPEN) {
    register('agent', sessionId);
  }
}

function createTransferItem(id, name, size, direction) {
  let element = transferList.querySelector(`[data-id="${id}"]`);
  if (element) return element;
  element = document.createElement('div');
  element.className = 'transfer-item';
  element.dataset.id = id;
  element.innerHTML = `
    <div class="transfer-name" title="${escapeHtml(name)}">${escapeHtml(name)}</div>
    <div class="transfer-meta">
      <span class="transfer-dir ${direction}">${direction === 'send' ? '↑ Sending' : '↓ Receiving'}</span>
      <span class="transfer-size">${formatBytes(size)}</span>
      <span class="transfer-pct">0%</span>
    </div>
    <div class="transfer-bar"><div class="transfer-fill ${direction}"></div></div>
    <div class="transfer-status">Queued</div>
  `;
  transferList.prepend(element);
  return element;
}

function updateTransfer(id, pct, status) {
  const element = transferList.querySelector(`[data-id="${id}"]`);
  if (!element) return;
  element.querySelector('.transfer-fill').style.width = `${Math.min(100, pct)}%`;
  element.querySelector('.transfer-pct').textContent = `${Math.floor(pct)}%`;
  element.querySelector('.transfer-status').textContent = status;
}

function formatBytes(bytes) {
  if (!bytes) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let value = Number(bytes);
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }
  return `${value.toFixed(unitIndex === 0 ? 0 : 1)} ${units[unitIndex]}`;
}

function escapeHtml(value) {
  return String(value)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

async function sendFile(fileEntry) {
  if (!connected || !remoteId) {
    setStatus('Not connected — cannot transfer files', 'error');
    return;
  }

  const id = `${Date.now()}-${Math.random().toString(16).slice(2)}`;
  const { file, name, size } = fileEntry;
  createTransferItem(id, name, size, 'send');
  updateTransfer(id, 0, 'Starting');
  outgoing.set(id, { file, offset: 0, total: size });

  sendMessage('file_transfer', { file_id: id, file_name: name, file_size: size }, remoteId);

  let offset = 0;
  let chunkIndex = 0;
  const totalChunks = Math.max(1, Math.ceil(size / CHUNK_SIZE));

  while (offset < size) {
    const slice = file.slice(offset, offset + CHUNK_SIZE);
    const buffer = await slice.arrayBuffer();
    const bytes = new Uint8Array(buffer);
    let binary = '';
    for (let i = 0; i < bytes.length; i += 1) {
      binary += String.fromCharCode(bytes[i]);
    }
    sendMessage('file_chunk', {
      file_id: id,
      offset,
      data: btoa(binary),
      chunk_index: chunkIndex,
      total_chunks: totalChunks,
    }, remoteId);

    offset += bytes.length;
    chunkIndex += 1;
    outgoing.set(id, { file, offset, total: size });
    updateTransfer(id, (offset / Math.max(size, 1)) * 100, `Sending ${formatBytes(offset)} / ${formatBytes(size)}`);
    await new Promise((resolve) => setTimeout(resolve, 0));
  }

  sendMessage('file_end', { file_id: id, status: 'success' }, remoteId);
  updateTransfer(id, 100, 'Sent ✓');
  outgoing.delete(id);
}

async function sendFolderFromInput(fileList) {
  for (const file of fileList) {
    await sendFile({
      file,
      name: file.webkitRelativePath || file.name,
      size: file.size,
    });
  }
}

async function sendFolder(entry) {
  const files = [];
  async function walk(current, prefix = '') {
    if (current.isFile) {
      await new Promise((resolve) => current.file((file) => {
        files.push({ file, name: `${prefix}${file.name}`, size: file.size });
        resolve();
      }));
      return;
    }

    if (current.isDirectory) {
      const reader = current.createReader();
      const readAllEntries = async () => {
        const chunk = await new Promise((resolve) => reader.readEntries(resolve));
        if (!chunk.length) return;
        for (const child of chunk) {
          await walk(child, `${prefix}${current.name}/`);
        }
        await readAllEntries();
      };
      await readAllEntries();
    }
  }

  await walk(entry, '');
  for (const item of files) {
    await sendFile(item);
  }
}

function handleFileTransferStart(payload) {
  if (!payload) return;
  incoming.set(payload.file_id, {
    name: payload.file_name,
    size: payload.file_size,
    received: 0,
    chunks: [],
  });
  createTransferItem(payload.file_id, payload.file_name, payload.file_size, 'receive');
  updateTransfer(payload.file_id, 0, 'Receiving');
}

function handleFileChunk(payload) {
  if (!payload) return;
  const transfer = incoming.get(payload.file_id);
  if (!transfer) return;
  const bytes = base64ToUint8Array(payload.data);
  transfer.chunks.push({ offset: payload.offset, data: bytes });
  transfer.received += bytes.byteLength;
  updateTransfer(
    payload.file_id,
    transfer.size > 0 ? (transfer.received / transfer.size) * 100 : 0,
    `Receiving ${formatBytes(transfer.received)} / ${formatBytes(transfer.size)}`,
  );
}

function handleFileEnd(payload) {
  if (!payload || payload.status !== 'success') return;
  const transfer = incoming.get(payload.file_id);
  if (!transfer) return;
  transfer.chunks.sort((left, right) => left.offset - right.offset);
  const merged = new Uint8Array(transfer.size);
  for (const chunk of transfer.chunks) {
    merged.set(chunk.data, chunk.offset);
  }

  const blob = new Blob([merged]);
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = transfer.name.split('/').pop() || transfer.name;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 10000);

  updateTransfer(payload.file_id, 100, `Saved: ${anchor.download} ✓`);
  incoming.delete(payload.file_id);
}

['dragenter', 'dragover'].forEach((eventName) => {
  dropZone.addEventListener(eventName, (event) => {
    event.preventDefault();
    dropZone.classList.add('drag-over');
  });
});

['dragleave', 'drop'].forEach((eventName) => {
  dropZone.addEventListener(eventName, (event) => {
    event.preventDefault();
    dropZone.classList.remove('drag-over');
  });
});

dropZone.addEventListener('drop', async (event) => {
  const items = Array.from(event.dataTransfer?.items || []);
  for (const item of items) {
    const entry = item.webkitGetAsEntry?.();
    if (!entry) continue;
    if (entry.isDirectory) {
      await sendFolder(entry);
    } else {
      await new Promise((resolve) => entry.file((file) => {
        sendFile({ file, name: file.name, size: file.size }).then(resolve);
      }));
    }
  }
});

connectBtn.addEventListener('click', startConnect);
remoteIdInput.addEventListener('keydown', (event) => {
  if (event.key === 'Enter') startConnect();
});
remoteIdInput.addEventListener('input', (event) => {
  event.target.value = formatId(event.target.value);
});
copyIdBtn.addEventListener('click', async () => {
  try {
    await navigator.clipboard.writeText(formatId(sessionId));
    setStatus('ID copied!', 'connected');
  } catch {
    setStatus('Clipboard unavailable', 'error');
  }
});
disconnectBtn.addEventListener('click', doDisconnect);
tbDisconnect.addEventListener('click', doDisconnect);
stopShareBtn.addEventListener('click', doDisconnect);
tbFullscreen.addEventListener('click', async () => {
  if (!document.fullscreenElement) {
    await remoteView.requestFullscreen();
  } else {
    await document.exitFullscreen();
  }
});
tbCAD.addEventListener('click', () => {
  sendInput('key_press', { key: 'Delete', key_code: 46, modifiers: ['Control', 'Alt'] });
});
tbFiles.addEventListener('click', () => {
  document.querySelector('.files-panel').scrollIntoView({ behavior: 'smooth', block: 'start' });
});
pickFilesBtn.addEventListener('click', () => {
  const input = document.createElement('input');
  input.type = 'file';
  input.multiple = true;
  input.addEventListener('change', async () => {
    for (const file of input.files || []) {
      await sendFile({ file, name: file.name, size: file.size });
    }
  });
  input.click();
});
pickFolderBtn.addEventListener('click', () => {
  const input = document.createElement('input');
  input.type = 'file';
  input.webkitdirectory = true;
  input.addEventListener('change', async () => {
    await sendFolderFromInput(input.files || []);
  });
  input.click();
});
hostPasswordInput.addEventListener('input', () => {
  streamStats.textContent = hostPasswordInput.value ? 'Session protected and idle.' : 'Waiting for viewer...';
});
remotePassInput.addEventListener('input', () => {
  if (!connected) return;
  viewStats.textContent = remotePassInput.value ? 'Password provided · waiting for frames' : 'Waiting for frames';
});

showView('welcome');
connectWS();
