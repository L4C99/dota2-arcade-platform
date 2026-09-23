(() => {
  const $ = (selector) => document.querySelector(selector);
  const state = {
    loggedIn: false, tab: 'overview', globalAccept: true, noticeEnabled: true, noticeLevel: 'warning',
    gameEnabled: true, gameAccept: true, presetEnabled: true, presetAccept: true,
    published: 'v2', reported: 'v2', bindingAccept: true, nodeAccept: true, drain: false,
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
      ? '平台正在接受新申请；已运行的服务器按各自状态继续管理。'
      : '全局维护中：新申请暂停，等待队列保留原排队时间；已运行的服务器不自动停止。';
    $('#admin-alert').dataset.warn = String(!state.globalAccept);
    $('#global-state').textContent = state.globalAccept ? '接受新申请' : '全局维护中';
    $('#global-state').classList.toggle('admin-pill--warm', !state.globalAccept);
    $('#global-accept').checked = state.globalAccept;
    $('#notice-enabled').checked = state.noticeEnabled;
    $('#notice-level').value = state.noticeLevel;
    $('#game-enabled').checked = state.gameEnabled;
    $('#game-accept').checked = state.gameAccept;
    $('#preset-enabled').checked = state.presetEnabled;
    $('#preset-accept').checked = state.presetAccept;

    $('#published-version').textContent = state.published;
    const canPublish = state.published !== 'v3' && state.reported === 'v3' && state.bindingAccept && state.nodeAccept && !state.drain;
    $('#publish-version').disabled = !canPublish;
    $('#publish-gate').dataset.ready = String(canPublish);
    $('#publish-gate').textContent = state.published === 'v3'
      ? 'v3 已发布。青岚一号匹配；仍上报 v2 的松影二号不会接收 v3 新实例。'
      : canPublish
        ? '演示条件已具备：青岚一号由 Controller 上报 v3，且内容验证已记录。发布需再次确认。'
        : '暂不能发布：需要已验证的目标节点由 Controller 上报 v3，并处于可接收新分配的状态。';

    $('#node-a-state').textContent = state.drain ? 'Drain 中' : state.nodeAccept ? '在线 · 接受分配' : '在线 · 暂停分配';
    $('#node-a-state').classList.toggle('admin-pill--warm', state.drain || !state.nodeAccept);
    $('#node-a-accept').checked = state.nodeAccept;
    $('#node-a-drain').checked = state.drain;
    $('#node-a-priority').value = state.priority;
    $('#node-a-desired').value = state.desired;
    $('#node-a-capacity').textContent = `0 / ${state.desired} 使用 · 硬上限 4`;
    $('#node-b-capacity').textContent = `${state.quarantined ? 3 : 2} / 3 使用 · 硬上限 4${state.quarantined ? ' · 含异常资源' : ''}`;
    const nodeBCard = $('#node-b-capacity').closest('.admin-card');
    const nodeBPill = nodeBCard.querySelector('.admin-pill');
    nodeBPill.textContent = state.quarantined ? '心跳延迟' : '在线';
    nodeBPill.classList.toggle('admin-pill--warm', state.quarantined);
    nodeBCard.querySelector('.admin-metrics>div:first-child strong').textContent = state.quarantined ? 'Linux · 2 分钟前' : 'Linux · 刚刚';
    nodeBCard.querySelector('.admin-metrics>div:last-child strong').textContent = state.quarantined ? '心跳延迟 · 暂停' : '恢复联系 · 重新确认资格';
    $('#binding-version').textContent = `${state.reported} · 已确认`;
    $('#binding-time').textContent = state.reported === 'v3' ? '刚刚 · Controller 上报' : '2 分钟前 · Controller 上报';
    $('#binding-accept').checked = state.bindingAccept;

    $('#steam-verification').textContent = state.steamVerified ? `真人实测通过 · revision ${state.revision}` : '未验证 · 暂不可开放';
    $('#china-verification').textContent = state.chinaVerified ? `真人实测通过 · revision ${state.revision}` : '未验证 · 暂不可开放';
    $('#steam-enabled').checked = state.steamEnabled;
    $('#steam-enabled').disabled = !state.steamVerified;
    $('#china-enabled').checked = state.chinaEnabled;
    $('#china-enabled').disabled = !state.chinaVerified;
    $('#entry-revision').textContent = `当前入口配置 revision ${state.revision}；验证只对应当前配置。`;

    $('#waiting-request').hidden = !state.waiting;
    $('#waiting-request h3').textContent = !state.globalAccept || !state.gameAccept || !state.presetAccept
      ? '等待中 · 调度暂停' : '等待服务器资源';
    $('#running-title').textContent = state.running === 'running' ? '服务器运行中' : '正在结束服务器';
    $('#stop-instance').disabled = state.running !== 'running';
    $('#quarantine-title').textContent = state.quarantined ? '旧服务器待核对' : '旧服务器已完整回收';
    $('#quarantine-detail').textContent = state.quarantined
      ? '松影二号 · 停止结果未确认 · 不可直接释放容量'
      : '松影二号 · d2core 已确认停止与清理完成 · 容量已释放';
    $('#resync-instance').disabled = !state.quarantined;
    $('#release-instance').textContent = state.quarantined ? '释放容量' : '容量已释放';
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

  $('#save-global').addEventListener('click', () => { state.globalAccept = $('#global-accept').checked; render(); notify('全局设置已更新（模拟）。'); });
  $('#save-notice').addEventListener('click', () => { state.noticeEnabled = $('#notice-enabled').checked; state.noticeLevel = $('#notice-level').value; render(); notify('站点公告已更新（模拟）。'); });
  $('#save-game').addEventListener('click', () => {
    state.gameEnabled = $('#game-enabled').checked;
    state.gameAccept = $('#game-accept').checked;
    if (!state.gameEnabled) state.waiting = false;
    render();
    notify(state.gameEnabled ? '地图设置已更新（模拟）。' : '地图已下架；尚未分配的对应等待申请已终结（模拟）。');
  });
  $('#save-preset').addEventListener('click', () => {
    state.presetEnabled = $('#preset-enabled').checked;
    state.presetAccept = $('#preset-accept').checked;
    if (!state.presetEnabled) state.waiting = false;
    render();
    notify('N6 玩法设置已更新（模拟）。');
  });
  $('#publish-version').addEventListener('click', () => {
    if (state.published === 'v3' || state.reported !== 'v3' || !state.bindingAccept || !state.nodeAccept || state.drain) return;
    ask('发布 ContentVersion v3？', '只改变新资源分配的目标版本。已创建的实例保持原版本；仍上报 v2 的节点不能接收 v3 新实例。', () => {
      state.published = 'v3'; render(); notify('当前发布版本已切换为 v3（模拟）。');
    });
  });
  $('#save-node').addEventListener('click', () => {
    const desired = Number($('#node-a-desired').value);
    const priority = Number($('#node-a-priority').value);
    if (!Number.isInteger(desired) || desired < 1 || desired > 4 || !Number.isInteger(priority) || priority < 1) {
      notify('请输入有效的期望容量（1–4）和正整数优先级。'); return;
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
  $('#demo-report').addEventListener('click', () => { state.reported = 'v3'; render(); notify('模拟 Controller 已读回并上报 v3；管理员没有改写节点事实。'); });
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
