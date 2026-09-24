(() => {
  const $ = (selector) => document.querySelector(selector);
  const state = {
    loggedIn: false, tab: 'overview', globalAccept: true, maintenanceMessage: '今晚 22:00 起暂停申请服务器。', noticeEnabled: true, noticeLevel: 'warning',
    gameEnabled: true, gameAccept: true, presetEnabled: true, presetAccept: true,
    published: 'v2', reported: 'v2', versionVerified: false, bindingAccept: true, nodeAccept: true, drain: false,
    priority: 10, desired: 3, revision: 7, steamVerified: true, steamEnabled: true,
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
    $('#preset-enabled').checked = state.presetEnabled;
    $('#preset-accept').checked = state.presetAccept;

    $('#published-version').textContent = state.published;
    const canPublish = state.published !== 'v3' && state.reported === 'v3' && state.versionVerified && state.bindingAccept && state.nodeAccept && !state.drain && state.desired > 0;
    $('#candidate-version-row').hidden = state.published === 'v3';
    $('#publish-version').hidden = state.published === 'v3';
    $('#publish-version').disabled = !canPublish;
    $('#publish-gate').dataset.ready = String(canPublish || state.published === 'v3');
    $('#publish-gate').textContent = state.published === 'v3'
      ? 'v3 已启用。之后分配服务器时，只会选择已准备好 v3 的节点。'
      : canPublish
        ? '青岚一号已准备并验证好 v3，可以切换。'
        : '暂不能切换：请先在可用节点上准备并测试 v3。';

    $('#node-a-state').textContent = state.drain ? '整节点维护中' : !state.nodeAccept ? '在线 · 暂停新分配' : state.desired === 0 ? '在线 · 调度名额为 0' : '在线 · 接收新分配';
    $('#node-a-state').classList.toggle('admin-pill--warm', state.drain || !state.nodeAccept || state.desired === 0);
    $('#node-a-accept').checked = state.nodeAccept;
    $('#node-a-drain').checked = state.drain;
    $('#node-a-priority').value = state.priority;
    $('#node-a-desired').value = state.desired;
    $('#node-a-capacity').textContent = `0 / ${state.desired} 使用 · 硬上限 4`;
    $('#node-b-capacity').textContent = `${state.quarantined ? 3 : 2} / 3 使用 · 硬上限 4${state.quarantined ? ' · 含待核对资源' : ''}`;
    $('#node-b-state').textContent = state.quarantined ? '心跳延迟' : '已恢复联系';
    $('#node-b-state').classList.toggle('admin-pill--warm', state.quarantined);
    $('#node-b-heartbeat').textContent = state.quarantined ? 'Linux · 2 分钟前' : 'Linux · 刚刚';
    $('#node-b-new-allocation').textContent = state.quarantined ? '暂停：心跳延迟' : '暂不分配：其他申请待对账';
    $('#binding-version').textContent = `${state.reported} · 已确认`;
    $('#binding-time').textContent = state.reported === 'v3' ? '刚刚 · Controller 上报' : '2 分钟前 · Controller 上报';
    $('#binding-accept').checked = state.bindingAccept;

    $('#steam-verification').textContent = state.steamVerified ? `当前网络配置（第 ${state.revision} 版）已完成实际进房测试` : '当前网络配置尚未完成实际进房测试';
    $('#china-verification').textContent = state.chinaVerified ? `当前网络配置（第 ${state.revision} 版）已完成实际进房测试` : '当前网络配置尚未完成实际进房测试';
    $('#steam-enabled').checked = state.steamEnabled;
    $('#steam-enabled').disabled = !state.steamVerified;
    $('#china-enabled').checked = state.chinaEnabled;
    $('#china-enabled').disabled = !state.chinaVerified;
    $('#entry-revision').textContent = `当前网络配置：第 ${state.revision} 版。实测结果仅对这一版有效。`;

    $('#waiting-request').hidden = !state.waiting;
    $('#waiting-request h3').textContent = !state.globalAccept || !state.gameAccept || !state.presetAccept
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

  $('#save-global').addEventListener('click', () => { state.globalAccept = $('#global-accept').checked; state.maintenanceMessage = $('#maintenance-message').value.trim(); render(); notify(state.globalAccept ? '全站已允许申请服务器（模拟）。' : '全站已暂停申请服务器；等待申请不会被取消（模拟）。'); });
  $('#save-notice').addEventListener('click', () => { state.noticeEnabled = $('#notice-enabled').checked; state.noticeLevel = $('#notice-level').value; render(); notify('站点公告已更新（模拟）。'); });
  $('#save-game').addEventListener('click', () => {
    state.gameEnabled = $('#game-enabled').checked;
    state.gameAccept = $('#game-accept').checked;
    if (!state.gameEnabled) state.waiting = false;
    render();
    notify(state.gameEnabled ? '地图设置已更新（模拟）。' : '地图已下架；还在排队的申请已结束（模拟）。');
  });
  $('#save-preset').addEventListener('click', () => {
    state.presetEnabled = $('#preset-enabled').checked;
    state.presetAccept = $('#preset-accept').checked;
    if (!state.presetEnabled) state.waiting = false;
    render();
    notify('N6 玩法设置已更新（模拟）。');
  });
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
  $('#binding-accept').addEventListener('change', (event) => { state.bindingAccept = event.target.checked; render(); notify('仅调整此节点此地图的新分配开关（模拟）。'); });
  ['steam', 'china'].forEach((kind) => {
    $(`#verify-${kind}`).addEventListener('click', () => ask('确认已完成真人验证？', '只有使用真实客户端验证当前网络配置下所有可能的公网端口映射后，才能标记实测通过。此处只改变模拟状态。', () => {
      state[`${kind}Verified`] = true; render(); notify('真人验证状态已记录（模拟）。');
    }));
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
    state.revision += 1;
    state.steamVerified = state.steamEnabled = state.chinaVerified = state.chinaEnabled = false;
    render(); notify('网络配置 revision 已变化；旧验证和开放状态已失效（模拟）。');
  });
  $('#demo-reclaim').addEventListener('click', () => { state.quarantined = false; render(); notify('模拟节点恢复，d2core 确认完整回收，旧容量已释放。'); });
  $('#admin-dialog-cancel').addEventListener('click', () => { $('#admin-dialog').close(); confirmAction = null; });
  $('#admin-dialog-confirm').addEventListener('click', () => { const action = confirmAction; confirmAction = null; $('#admin-dialog').close(); if (action) action(); });
  $('#admin-dialog').addEventListener('close', () => { confirmAction = null; });

  render();
})();
