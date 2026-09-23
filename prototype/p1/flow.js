(() => {
  const $ = (selector) => document.querySelector(selector);
  const state = { stage: 'idle', preset: 'N6', node: null, assignedNode: null, nextGameIntent: false };
  const nodeNames = { qinglan: '青岚一号', songying: '松影二号' };
  let noticeTimer;
  const stageCopy = {
    waiting: ['等待服务器资源', '申请已提交；有可用资源时会开始启动。', '等待中'],
    creating: ['正在启动服务器', '', '启动中'],
    ready: ['服务器可以进入', '可通过 Steam 或手动连接进入。', '可进入'],
    stopping: ['正在结束服务器', '当前房间仍占用节点资源，等待完整回收。', '结束中'],
    ended: ['服务器已结束', '底层实例已完整回收，现在可以重新申请。', '已结束'],
    cancelled: ['申请已取消', '请求在分配节点前取消，没有占用服务器资源。', '已取消']
  };

  function nodeChoiceLabel() {
    return state.node ? `手动指定：${nodeNames[state.node]}` : '自动选择节点';
  }

  function requestNodeLabel() {
    return state.assignedNode ? nodeNames[state.assignedNode] : nodeChoiceLabel();
  }

  function notify(message, trigger) {
    const toast = $('#prototype-toast');
    clearTimeout(noticeTimer);
    toast.textContent = message;
    toast.hidden = !message;
    if (!message) return;

    const anchor = trigger.getBoundingClientRect();
    const gap = 10;
    const width = toast.offsetWidth;
    const height = toast.offsetHeight;
    const left = Math.max(12, Math.min(window.innerWidth - width - 12, anchor.left + (anchor.width - width) / 2));
    const below = anchor.bottom + gap;
    const top = below + height <= window.innerHeight - 12 ? below : Math.max(12, anchor.top - height - gap);
    toast.style.left = `${left}px`;
    toast.style.top = `${top}px`;
    noticeTimer = window.setTimeout(() => { toast.hidden = true; }, 4000);
  }

  function render() {
    const active = state.stage !== 'idle';
    $('#application-view').hidden = active;
    $('#request-view').hidden = !active;
    $('#page-title').textContent = active ? '当前服务器' : '申请服务器';
    $('#node-choice-title').textContent = nodeChoiceLabel();
    $('#node-choice-subtitle').textContent = state.node ? '只等待该节点' : '当前默认 · 优先可用资源';
    $('#auto-check').hidden = Boolean(state.node);
    $('#fact-preset').textContent = state.preset;
    $('#fact-node').textContent = requestNodeLabel();

    if (active) {
      const [title, stageDescription, pill] = stageCopy[state.stage];
      const description = state.stage === 'creating'
        ? `已分配节点：${nodeNames[state.assignedNode]}。`
        : stageDescription;
      $('#status-title').textContent = title;
      $('#status-description').textContent = description;
      $('#status-description').hidden = !description;
      $('#status-pill').textContent = pill;
      $('#status-pill').dataset.stage = state.stage;
    }

    $('#waiting-actions').hidden = state.stage !== 'waiting';
    $('#terminal-actions').hidden = !['ended', 'cancelled'].includes(state.stage);
    $('#join-panel').hidden = state.stage !== 'ready';

    const progressIndex = { waiting: 0, creating: 1, ready: 2, stopping: 2 }[state.stage];
    $('.flow-progress').hidden = progressIndex === undefined;
    document.querySelectorAll('[data-progress]').forEach((item, index) => {
      item.classList.toggle('is-current', index === progressIndex);
      item.classList.toggle('is-done', index < progressIndex || state.stage === 'stopping');
    });

    const next = $('#simulate-next');
    const labels = { waiting: '模拟节点受理', creating: '模拟就绪与连接信息', stopping: '模拟完整回收' };
    next.disabled = !labels[state.stage];
    next.textContent = labels[state.stage] || '模拟节点继续';
  }

  function submitRequest() {
    if (state.stage !== 'idle') return;
    state.preset = $('input[name="preset"]:checked').value;
    state.node = $('input[name="node"]:checked')?.value || null;
    state.assignedNode = null;
    state.stage = 'waiting';
    notify('');
    render();
  }

  function simulateNext() {
    if (state.stage === 'waiting') {
      state.assignedNode = state.node || 'qinglan';
      state.stage = 'creating';
      notify('');
    } else if (state.stage === 'creating') {
      state.stage = 'ready';
      notify('');
    } else if (state.stage === 'stopping') {
      if (state.nextGameIntent) {
        state.nextGameIntent = false;
        state.assignedNode = null;
        state.stage = 'waiting';
        notify('');
      } else {
        state.stage = 'ended';
        notify('');
      }
    } else {
      return;
    }
    render();
  }

  function reset() {
    state.stage = 'idle';
    state.preset = 'N6';
    state.node = null;
    state.assignedNode = null;
    state.nextGameIntent = false;
    $('input[name="preset"][value="N6"]').checked = true;
    document.querySelectorAll('input[name="node"]').forEach((input) => { input.checked = false; });
    $('.manual-disclosure').open = false;
    notify('');
    render();
  }

  async function copyCommand() {
    const command = $('#connect-command').textContent.trim();
    try {
      if (!navigator.clipboard?.writeText) throw new Error('Clipboard API unavailable');
      await navigator.clipboard.writeText(command);
      notify('已复制 connect 示例命令；演示地址无法真实进房。', $('#copy-connect'));
    } catch {
      const selection = window.getSelection();
      const range = document.createRange();
      range.selectNodeContents($('#connect-command'));
      selection.removeAllRanges();
      selection.addRange(range);
      notify('未能自动复制；命令已选中，可手动复制。', $('#copy-connect'));
    }
  }

  async function copySteamLink() {
    const uri = 'steam://connect/192.0.2.42:45123';
    try {
      if (!navigator.clipboard?.writeText) throw new Error('Clipboard API unavailable');
      await navigator.clipboard.writeText(uri);
      notify('已复制 Steam 示例链接；演示地址无法真实进房。', $('#copy-steam-link'));
    } catch {
      window.prompt('复制 Steam 入口示例链接（地址不可用于真实进房）', uri);
      notify('请从弹出的文本框复制示例链接。', $('#copy-steam-link'));
    }
  }

  async function copyConsoleOption() {
    const option = $('#console-option').textContent.trim();
    try {
      if (!navigator.clipboard?.writeText) throw new Error('Clipboard API unavailable');
      await navigator.clipboard.writeText(option);
      notify('已复制启动选项 -console。', $('#copy-console-option'));
    } catch {
      const selection = window.getSelection();
      const range = document.createRange();
      range.selectNodeContents($('#console-option'));
      selection.removeAllRanges();
      selection.addRange(range);
      notify('未能自动复制；启动选项已选中，可手动复制。', $('#copy-console-option'));
    }
  }

  document.querySelectorAll('input[name="preset"], input[name="node"]').forEach((input) => {
    input.addEventListener('change', () => {
      state.preset = $('input[name="preset"]:checked').value;
      state.node = $('input[name="node"]:checked')?.value || null;
      render();
    });
  });
  $('#auto-reset').addEventListener('click', () => {
    document.querySelectorAll('input[name="node"]').forEach((input) => { input.checked = false; });
    state.node = null;
    render();
  });
  $('#apply-button').addEventListener('click', submitRequest);
  $('#simulate-next').addEventListener('click', simulateNext);
  $('#reset-demo').addEventListener('click', reset);
  $('#cancel-button').addEventListener('click', () => {
    if (state.stage !== 'waiting') return;
    state.stage = 'cancelled';
    notify('');
    render();
  });
  $('#start-over').addEventListener('click', () => {
    state.stage = 'idle';
    state.assignedNode = null;
    state.nextGameIntent = false;
    notify('');
    render();
  });
  $('#next-game').addEventListener('click', () => {
    if (state.stage !== 'ready') return;
    state.nextGameIntent = true;
    state.stage = 'stopping';
    notify('');
    render();
  });
  $('#end-server').addEventListener('click', () => {
    if (state.stage !== 'ready') return;
    state.nextGameIntent = false;
    state.stage = 'stopping';
    notify('');
    render();
  });
  $('#steam-entry').addEventListener('click', () => {
    if (state.stage === 'ready') notify('原型演示：不会启动 Steam 客户端。', $('#steam-entry'));
  });
  $('#copy-connect').addEventListener('click', copyCommand);
  $('#copy-steam-link').addEventListener('click', copySteamLink);
  $('#copy-console-option').addEventListener('click', copyConsoleOption);
  render();
})();
