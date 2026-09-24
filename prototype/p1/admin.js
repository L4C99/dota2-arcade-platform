(() => {
  const $ = (selector) => document.querySelector(selector);
  const state = {
    loggedIn: false, tab: 'overview', selectedNode: 'a', selectedGame: 'star', selectedBindingGame: 'star', globalAccept: true, maintenanceMessage: '今晚 22:00 起暂停申请服务器。', noticeEnabled: true, noticeLevel: 'warning',
    gameEnabled: true, gameAccept: true, harborGameEnabled: true, harborGameAccept: true,
    selectedPresets: { star: 'n6', harbor: 'standard' },
    presets: {
      star: { n6: { name: 'N6', enabled: true, accept: true, message: '' }, n7: { name: 'N7', enabled: true, accept: true, message: '' }, solo: { name: 'Solo', enabled: true, accept: true, message: '' } },
      harbor: { standard: { name: '标准', enabled: true, accept: true, message: '' }, challenge: { name: '挑战', enabled: true, accept: true, message: '' } }
    },
    published: 'v2', reported: 'v2', versionVerified: false, bindingAccept: true, harborBindingAccept: true, nodeAccept: true, drain: false,
    priority: 10, desired: 3, steamVerified: true, steamEnabled: true,
    chinaVerified: false, chinaEnabled: false, waiting: true, running: 'running', quarantined: true
  };
  let toastTimer;
  let confirmAction = null;

  function notify(message) {
    const toast = $('#admin-toast');
    clearTimeout(toastTimer);
    toast.textContent = message;
    toast.hidden = !message;
    if (message) toastTimer = window.setTimeout(() => { toast.hidden = true; }, 4000);
  }

  function ask(title, message, action) {
    $('#admin-dialog-title').textContent = title;
    $('#admin-dialog-text').textContent = message;
    confirmAction = action;
    $('#admin-dialog').showModal();
  }

  function renderNodeSelection() {
    document.querySelectorAll('[data-node]').forEach((button) => { button.setAttribute('aria-pressed', String(button.dataset.node === state.selectedNode)); });
    ['a', 'b', 'c'].forEach((node) => { $(`#node-detail-${node}`).hidden = node !== state.selectedNode; });
  }

  function renderGameSelection() {
    document.querySelectorAll('[data-game]').forEach((button) => { button.setAttribute('aria-pressed', String(button.dataset.game === state.selectedGame)); });
    ['star', 'harbor'].forEach((game) => { $(`#game-detail-${game}`).hidden = game !== state.selectedGame; });
  }

  function renderBindingSelection() {
    const star = state.selectedBindingGame === 'star';
    document.querySelectorAll('[data-binding-game]').forEach((button) => {
      const isStar = button.dataset.bindingGame === 'star';
      const version = isStar ? state.reported : 'v1';
      const accepting = isStar ? state.bindingAccept : state.harborBindingAccept;
      button.setAttribute('aria-pressed', String(button.dataset.bindingGame === state.selectedBindingGame));
      button.querySelector('small').textContent = `节点版本 ${version} · ${accepting ? '允许新分配' : '暂停新分配'}`;
    });
    const name = star ? '星潮远征' : '雾港守卫';
    $('#binding-game-title').textContent = name;
    $('#binding-version').textContent = `${star ? state.reported : 'v1'} · 已确认`;
    $('#binding-time').textContent = star && state.reported === 'v3' ? '刚刚' : star ? '2 分钟前' : '5 分钟前';
    $('#binding-accept').checked = star ? state.bindingAccept : state.harborBindingAccept;
    $('#binding-accept-text').textContent = `允许在这台节点上分配${name}服务器`;
  }

  function renderPresetSelection(game) {
    const prefix = game === 'star' ? 'preset' : 'harbor-preset';
    const selected = state.selectedPresets[game];
    const preset = state.presets[game][selected];
    document.querySelectorAll(`[data-preset-map="${game}"]`).forEach((button) => {
      const option = state.presets[game][button.dataset.preset];
      button.setAttribute('aria-pressed', String(button.dataset.preset === selected));
      button.querySelector('.admin-preset-choice__state').textContent = !option.enabled ? '已下架' : option.accept ? '可申请' : '暂不可申请';
    });
    $(`#${game}-preset-title`).textContent = preset.name;
    $(`#${prefix}-enabled`).checked = preset.enabled;
    $(`#${prefix}-accept`).checked = preset.accept;
    $(`#${prefix}-message`).value = preset.message;
    $(`#${prefix}-enabled-text`).textContent = `上架 ${preset.name}`;
    $(`#${prefix}-enabled-hint`).textContent = `下架后 ${preset.name} 不再可选；还在排队的 ${preset.name} 申请也会结束。`;
    $(`#${prefix}-accept-text`).textContent = `允许申请 ${preset.name}`;
    $(`#${prefix}-accept-hint`).textContent = `关闭后 ${preset.name} 仍可见；排队中的 ${preset.name} 申请会暂停，恢复后继续等待。`;
    $(`#${prefix}-message-text`).textContent = `${preset.name} 暂不可申请时的提示`;
    $(`#save-${prefix}`).textContent = `保存${preset.name}玩法设置`;
  }

  function savePreset(game) {
    const prefix = game === 'star' ? 'preset' : 'harbor-preset';
    const selected = state.selectedPresets[game];
    const preset = state.presets[game][selected];
    preset.enabled = $(`#${prefix}-enabled`).checked;
    preset.accept = $(`#${prefix}-accept`).checked;
    preset.message = $(`#${prefix}-message`).value.trim();
    if (game === 'star' && selected === 'n6' && !preset.enabled) state.waiting = false;
    render();
    notify(`${preset.name}玩法设置已更新（模拟）。`);
  }

  function render() {
    $('#admin-login').hidden = state.loggedIn;
    $('#admin-app').hidden = !state.loggedIn;
    $('#logout').hidden = !state.loggedIn;
    ['demo-report', 'demo-network-change', 'demo-reclaim'].forEach((id) => { $(`#${id}`).disabled = !state.loggedIn; });
    if (!state.loggedIn) return;

    document.querySelectorAll('[data-tab]').forEach((button) => {
      if (button.dataset.tab === state.tab) button.setAttribute('aria-current', 'page');
      else button.removeAttribute('aria-current');
    });
    ['overview', 'content', 'nodes', 'instances'].forEach((tab) => { $(`#tab-${tab}`).hidden = tab !== state.tab; });
    renderNodeSelection();
    renderGameSelection();
    renderBindingSelection();
    renderPresetSelection('star');
    renderPresetSelection('harbor');
    $('#admin-alert').textContent = state.globalAccept
      ? '全站允许玩家申请服务器；各地图和玩法仍按各自设置生效。'
      : `全站暂停申请服务器；等待申请暂停调度，恢复后按原提交时间继续排队。已分配的服务器不会因此停止。${state.maintenanceMessage ? ` 玩家提示：${state.maintenanceMessage}` : ''}`;
    $('#admin-alert').dataset.warn = String(!state.globalAccept);
    $('#global-state').textContent = state.globalAccept ? '允许新申请' : '暂停新申请';
    $('#global-state').classList.toggle('admin-pill--warm', !state.globalAccept);
    $('#global-accept').checked = state.globalAccept;
    $('#maintenance-message').value = state.maintenanceMessage;
    $('#notice-enabled').checked = state.noticeEnabled;
    $('#notice-level').value = state.noticeLevel;
    $('#game-enabled').checked = state.gameEnabled;
    $('#game-accept').checked = state.gameAccept;
    $('#harbor-game-enabled').checked = state.harborGameEnabled;
    $('#harbor-game-accept').checked = state.harborGameAccept;
    $('#game-state-star').textContent = !state.gameEnabled ? '已下架' : state.gameAccept ? '可申请' : '暂不可申请';
    $('#game-state-star').classList.toggle('admin-pill--warm', !state.gameEnabled || !state.gameAccept);
    $('#game-state-harbor').textContent = !state.harborGameEnabled ? '已下架' : state.harborGameAccept ? '可申请' : '暂不可申请';
    $('#game-state-harbor').classList.toggle('admin-pill--warm', !state.harborGameEnabled || !state.harborGameAccept);

    $('#published-version').textContent = state.published;
    $('#game-summary-star-version').textContent = state.published;
    const nodeAVerified = state.reported === 'v3' && state.versionVerified;
    const nodeAEligible = nodeAVerified && state.bindingAccept && state.nodeAccept && !state.drain && state.desired > 0;
    const canPublish = state.published !== 'v3' && nodeAEligible;
    $('#version-node-a-fact').textContent = `已确认 ${state.reported} · 在线`;
    $('#version-node-a-test').textContent = nodeAVerified ? 'v3 实测已通过' : 'v3 尚未验证';
    $('#version-node-a-outcome').textContent = state.published === 'v3'
      ? nodeAEligible ? '可接收新分配' : '暂不可接收新分配'
      : nodeAEligible ? '切换后可接收' : '切换后暂不可接收';
    $('#version-node-a-outcome').dataset.ready = String(nodeAEligible);
    $('#version-node-b-fact').textContent = state.quarantined ? '上次确认 v2 · 心跳延迟' : '已确认 v2 · 在线';
    $('#version-node-b-outcome').textContent = state.published === 'v3' ? '暂不可接收 · 版本不符' : '切换后暂不可接收';
    $('#version-node-c-outcome').textContent = state.published === 'v3' ? '暂不可接收 · 版本未确认' : '切换后暂不可接收';
    $('#candidate-version-row').hidden = state.published === 'v3';
    $('#publish-version').hidden = state.published === 'v3';
    $('#publish-version').disabled = !canPublish;
    $('#publish-gate').dataset.ready = String(nodeAEligible);
    $('#publish-gate').textContent = state.published === 'v3'
      ? nodeAEligible
        ? 'v3 已启用。目前青岚一号可接收这张地图的新分配；其他节点暂不可接收。'
        : 'v3 已启用，但目前没有可接收这张地图新分配的节点。'
      : canPublish
        ? '青岚一号已准备并验证 v3，可以切换；其他节点暂不接收 v3 新分配。'
        : nodeAVerified
          ? '暂不能切换：青岚一号已验证 v3，但当前暂停接收新分配。'
          : '暂不能切换：目前没有已准备并验证 v3 的可用节点。';

    $('#node-a-state').textContent = '在线';
    $('#node-a-accept').checked = state.nodeAccept;
    $('#node-a-drain').checked = state.drain;
    $('#node-a-priority').value = state.priority;
    $('#node-a-desired').value = state.desired;
    $('#node-a-capacity').textContent = `0 / ${state.desired} 占用 · 节点上限 4`;
    $('#node-a-summary-capacity').textContent = `0 / ${state.desired} 占用 · 硬上限 4`;
    $('#node-a-summary-new-allocation').textContent = state.drain ? '暂停 · 整节点维护' : !state.nodeAccept ? '暂停 · 管理员关闭' : state.desired === 0 ? '暂停 · 名额设为 0' : '允许接收';
    $('#node-b-capacity').textContent = `${state.quarantined ? 3 : 2} / 3 占用 · 节点上限 4${state.quarantined ? ' · 含待核对资源' : ''}`;
    $('#node-b-summary-capacity').textContent = `${state.quarantined ? 3 : 2} / 3 占用 · 硬上限 4${state.quarantined ? ' · 含待核对资源' : ''}`;
    $('#node-b-summary-new-allocation').textContent = state.quarantined ? '暂停 · 心跳延迟' : '暂停 · 申请待对账';
    $('#node-b-state').textContent = state.quarantined ? '心跳延迟' : '在线';
    $('#node-b-state').classList.toggle('admin-pill--warm', state.quarantined);
    $('#node-b-heartbeat').textContent = state.quarantined ? 'Linux · 2 分钟前' : 'Linux · 刚刚';
    $('#node-b-summary-heartbeat').textContent = $('#node-b-heartbeat').textContent;
    $('#node-b-new-allocation').textContent = state.quarantined ? '暂停 · 心跳延迟' : '暂停 · 申请待核对';
    $('#node-b-detail-note').textContent = state.quarantined
      ? '心跳延迟时，这里显示上次确认的信息；暂停新分配，已有服务器仍占用名额。'
      : '节点已恢复联系；仍有申请待核对，暂时不接收新分配。';
    $('#steam-verification').textContent = state.steamVerified ? '当前网络配置已通过进房实测' : '当前网络配置尚未实测进房';
    $('#china-verification').textContent = state.chinaVerified ? '当前网络配置已通过进房实测' : '当前网络配置尚未实测进房';
    ['steam', 'china'].forEach((kind) => {
      $(`#verify-${kind}`).textContent = state[`${kind}Verified`] ? '已完成实测' : '确认已完成实测';
      $(`#verify-${kind}`).disabled = state[`${kind}Verified`];
      $(`#verify-${kind}`).dataset.verified = String(state[`${kind}Verified`]);
    });
    $('#steam-enabled').checked = state.steamEnabled;
    $('#steam-enabled').disabled = !state.steamVerified;
    $('#china-enabled').checked = state.chinaEnabled;
    $('#china-enabled').disabled = !state.chinaVerified;

    $('#waiting-request').hidden = !state.waiting;
    $('#waiting-request h3').textContent = !state.globalAccept || !state.gameAccept || !state.presets.star.n6.accept
      ? '等待中 · 调度暂停' : '等待服务器资源';
    $('#creating-title').textContent = state.quarantined ? '启动结果待核对' : '最近上报：正在启动';
    $('#creating-note').textContent = state.quarantined
      ? '最后上报正在启动；节点心跳延迟，当前是否启动成功尚不确定。'
      : '节点已恢复联系；此申请仍占用容量，等待最新创建结果。';
    $('#running-stage').textContent = state.running === 'stopping' ? '申请 C · 停止请求已提交' : '申请 C · 最近上报运行中';
    $('#running-title').textContent = state.running === 'stopping'
      ? '已请求停止 · 等待确认'
      : state.quarantined ? '服务器状态待确认' : '最近上报：运行中';
    $('#running-note').textContent = state.running === 'stopping'
      ? '已提交停止请求；d2core 确认停止并完成清理前，继续占用容量。'
      : state.quarantined ? '节点心跳延迟；最后上报为运行中，当前状态需节点恢复后确认。' : '节点已恢复联系；当前申请仍占用 1 个名额。';
    $('#stop-instance').disabled = state.running !== 'running';
    $('#quarantined-instance').hidden = !state.quarantined;
    $('#reclaimed-group').hidden = state.quarantined;
    $('#reclaimed-instance').hidden = state.quarantined;
  }

  $('#login-form').addEventListener('submit', (event) => {
    event.preventDefault();
    if (!$('#login-form').reportValidity()) return;
    $('#login-password').value = '';
    state.loggedIn = true;
    render();
    notify('已进入管理员原型；没有验证真实账号。');
  });
  $('#logout').addEventListener('click', () => { state.loggedIn = false; render(); notify('已退出原型演示。'); });
  $('#demo-reset').addEventListener('click', () => window.location.reload());
  document.querySelectorAll('[data-tab]').forEach((button) => button.addEventListener('click', () => { state.tab = button.dataset.tab; render(); }));
  document.querySelectorAll('[data-node]').forEach((button) => button.addEventListener('click', () => { state.selectedNode = button.dataset.node; renderNodeSelection(); }));
  document.querySelectorAll('[data-game]').forEach((button) => button.addEventListener('click', () => { state.selectedGame = button.dataset.game; renderGameSelection(); }));
  document.querySelectorAll('[data-binding-game]').forEach((button) => button.addEventListener('click', () => { state.selectedBindingGame = button.dataset.bindingGame; renderBindingSelection(); }));
  document.querySelectorAll('[data-preset-map]').forEach((button) => button.addEventListener('click', () => { state.selectedPresets[button.dataset.presetMap] = button.dataset.preset; renderPresetSelection(button.dataset.presetMap); }));

  $('#save-global').addEventListener('click', () => { state.globalAccept = $('#global-accept').checked; state.maintenanceMessage = $('#maintenance-message').value.trim(); render(); notify(state.globalAccept ? '全站已允许申请服务器（模拟）。' : '全站已暂停申请服务器；等待申请不会被取消（模拟）。'); });
  $('#save-notice').addEventListener('click', () => { state.noticeEnabled = $('#notice-enabled').checked; state.noticeLevel = $('#notice-level').value; render(); notify('站点公告已更新（模拟）。'); });
  $('#save-game').addEventListener('click', () => {
    state.gameEnabled = $('#game-enabled').checked;
    state.gameAccept = $('#game-accept').checked;
    if (!state.gameEnabled) state.waiting = false;
    render();
    notify(state.gameEnabled ? '地图设置已更新（模拟）。' : '地图已下架；还在排队的申请已结束（模拟）。');
  });
  $('#save-harbor-game').addEventListener('click', () => {
    state.harborGameEnabled = $('#harbor-game-enabled').checked;
    state.harborGameAccept = $('#harbor-game-accept').checked;
    render();
    notify('雾港守卫的地图设置已更新，不影响星潮远征（模拟）。');
  });
  $('#save-preset').addEventListener('click', () => savePreset('star'));
  $('#save-harbor-preset').addEventListener('click', () => savePreset('harbor'));
  $('#publish-version').addEventListener('click', () => {
    if (state.published === 'v3' || state.reported !== 'v3' || !state.versionVerified || !state.bindingAccept || !state.nodeAccept || state.drain || state.desired === 0) return;
    ask('让后续分配的服务器使用 v3？', '此后分配到节点的服务器使用 v3；已分配到节点的服务器仍使用原版本。未准备好 v3 的节点暂不接收这张地图的后续分配。', () => {
      state.published = 'v3'; render(); notify('已启用 v3，后续分配的服务器将使用此版本（模拟）。');
    });
  });
  $('#save-node').addEventListener('click', () => {
    const desired = Number($('#node-a-desired').value);
    const priority = Number($('#node-a-priority').value);
    if (!Number.isInteger(desired) || desired < 0 || desired > 4 || !Number.isInteger(priority) || priority < 1) {
      notify('请输入有效的节点名额上限（0–4）和正整数优先级。'); return;
    }
    state.desired = desired; state.priority = priority;
    state.nodeAccept = $('#node-a-accept').checked; state.drain = $('#node-a-drain').checked;
    render(); notify('节点调度设置已更新；已有实例不受影响（模拟）。');
  });
  $('#binding-accept').addEventListener('change', (event) => {
    const star = state.selectedBindingGame === 'star';
    state[star ? 'bindingAccept' : 'harborBindingAccept'] = event.target.checked;
    render();
    notify(`青岚一号上的${star ? '星潮远征' : '雾港守卫'}已${event.target.checked ? '允许' : '暂停'}新分配（模拟）。`);
  });
  ['steam', 'china'].forEach((kind) => {
    $(`#verify-${kind}`).addEventListener('click', () => {
      if (state[`${kind}Verified`]) return;
      ask('确认已完成进房实测？', '请先用真实客户端测试这台节点可能使用的所有公网端口，确认能进房。此处只改变模拟状态。', () => {
        state[`${kind}Verified`] = true; render(); notify('进房实测结果已记录（模拟）。');
      });
    });
    $(`#${kind}-enabled`).addEventListener('change', (event) => {
      if (!state[`${kind}Verified`]) { event.target.checked = false; return; }
      state[`${kind}Enabled`] = event.target.checked; render(); notify('玩家一键入口开放状态已更新（模拟）。');
    });
  });

  $('#cancel-waiting').addEventListener('click', () => ask('取消这条等待申请？', '示例申请仍处于纯等待，尚未创建资源分配，因此可以安全取消。', () => {
    state.waiting = false; render(); notify('等待申请已取消（模拟）。');
  }));
  $('#stop-instance').addEventListener('click', () => ask('请求停止服务器？', '会提交停止任务；只有 d2core 确认停止且清理完成后才释放容量。', () => {
    state.running = 'stopping'; render(); notify('已提交停止请求；容量仍被占用（模拟）。');
  }));
  $('#resync-instance').addEventListener('click', () => notify('已请求重新核对；结果未知期间继续占用容量（模拟）。'));
  $('#demo-report').addEventListener('click', () => { state.reported = 'v3'; state.versionVerified = true; render(); notify('模拟青岚一号上报 v3，并完成内容实测确认。'); });
  $('#demo-network-change').addEventListener('click', () => {
    state.steamVerified = state.steamEnabled = state.chinaVerified = state.chinaEnabled = false;
    render(); notify('网络配置已更改；一键入口需重新实测后才能开放（模拟）。');
  });
  $('#demo-reclaim').addEventListener('click', () => { state.quarantined = false; render(); notify('模拟节点恢复，d2core 确认完整回收，旧容量已释放。'); });
  $('#admin-dialog-cancel').addEventListener('click', () => { $('#admin-dialog').close(); confirmAction = null; });
  $('#admin-dialog-confirm').addEventListener('click', () => { const action = confirmAction; confirmAction = null; $('#admin-dialog').close(); if (action) action(); });
  $('#admin-dialog').addEventListener('close', () => { confirmAction = null; });

  render();
})();
