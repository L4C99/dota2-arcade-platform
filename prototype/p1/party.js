(() => {
  const $ = (selector) => document.querySelector(selector);
  const scenarioStates = {
    solo: ['solo', 'none', false],
    invited: ['invited', 'none', false],
    'captain-idle': ['captain', 'none', false],
    'member-idle': ['member', 'none', false],
    'captain-unknown': ['captain', 'unknown', false],
    'member-unknown': ['member', 'unknown', false],
    'captain-quarantined': ['captain', 'quarantined', false],
    'member-quarantined': ['member', 'quarantined', false],
    'captain-next-quarantined': ['captain', 'quarantined', true]
  };
  const state = { role: 'captain', request: 'none', nextIntent: false, abandoned: false, inviteVersion: 1, teammatePresent: true, extraMembers: [] };
  let toastTimer;

  function notify(message) {
    const toast = $('#party-toast');
    clearTimeout(toastTimer);
    toast.textContent = message;
    toast.hidden = !message;
    if (message) toastTimer = window.setTimeout(() => { toast.hidden = true; }, 4000);
  }

  function loadScenario(name) {
    const [role, request, nextIntent] = scenarioStates[name];
    Object.assign(state, { role, request, nextIntent, abandoned: false, inviteVersion: 1, teammatePresent: true, extraMembers: [] });
    $('#scenario').value = name;
    if ($('#abandon-dialog').open) $('#abandon-dialog').close();
    notify('');
    render();
  }

  function memberRow(name, initial, role, isSelf, onRemove) {
    const row = document.createElement('div');
    row.className = 'party-member';
    const avatar = document.createElement('span');
    avatar.className = 'party-avatar';
    avatar.textContent = initial;
    const identity = document.createElement('span');
    identity.className = 'party-member__identity';
    const title = document.createElement('strong');
    title.textContent = name;
    const detail = document.createElement('small');
    detail.textContent = isSelf ? '你 · 匿名玩家' : '匿名玩家';
    identity.append(title, detail);
    const roleLabel = document.createElement('span');
    roleLabel.className = 'party-member__role';
    roleLabel.textContent = role;
    row.append(avatar, identity, roleLabel);
    if (onRemove) {
      const button = document.createElement('button');
      button.type = 'button';
      button.className = 'party-member__remove';
      button.textContent = '移除';
      button.setAttribute('aria-label', `移除队员${name}`);
      button.addEventListener('click', () => {
        onRemove();
        render();
        notify(`已移除${name}。`);
      });
      row.append(button);
    }
    return row;
  }

  function renderTeam() {
    const captain = state.role === 'captain';
    const members = captain
      ? [memberRow('林间旅人', 'L', '队长', true)]
      : [memberRow('山海同游', 'S', '队长', false), memberRow('林间旅人', 'L', '队员', true)];
    if (captain) {
      if (state.teammatePresent) members.push(memberRow('山海同游', 'S', '队员', false, () => { state.teammatePresent = false; }));
      state.extraMembers.forEach((name) => members.push(memberRow(name, name[0], '队员', false, () => {
        state.extraMembers = state.extraMembers.filter((member) => member !== name);
      })));
    }
    $('#member-list').replaceChildren(...members);
    $('#team-count').textContent = `${members.length} / 4 人 · 演示上限`;
    $('#leave-party').hidden = captain;
    $('#disband-party').hidden = !captain;
    $('#disband-party').disabled = state.request !== 'none' && state.request !== 'freed';
    $('#manage-note').textContent = captain
      ? ($('#disband-party').disabled ? '服务器申请尚未结束，队长暂时不能解散队伍。' : '可以解散队伍。')
      : '队员可随时退出；退出不会停止队伍已有服务器。';
    $('#invite-link').textContent = `https://example.invalid/join/DEMO-P1C-${state.inviteVersion}`;
    $('#add-demo-member').disabled = members.length >= 4;
  }

  function renderRequest() {
    const { role, request, nextIntent, abandoned } = state;
    const captain = role === 'captain';
    const view = {
      none: captain
        ? ['可以申请服务器', '队长选择地图、模式和节点后，为当前队伍提交申请。', '可申请', '正常申请流程见 P-1B 演示。']
        : role === 'member'
          ? ['等待队长申请', '只有队长可以为队伍申请服务器；队员可以查看申请状态。', '未申请', '队员不能替队长申请或结束服务器。']
          : ['暂无活动申请', '当前没有队伍申请；未入队时仍可提交单人申请。', '未申请', '单人申请流程见 P-1B 演示。'],
      unknown: ['状态暂时无法确认', '节点没有给出确定结果，平台正在核对。', '核对中', '请等待核对结果。'],
      quarantined: ['服务器清理异常', '平台暂时无法确认旧服务器已关闭并清理完成。', '待处理', captain
        ? (nextIntent ? '下一局已暂停；确认放弃后才会按上局选项重新排队。' : '放弃后可重新选择并申请新服务器。')
        : '请队长决定是否放弃此异常服务器。'],
      freed: ['可以重新申请', '现在可以重新选择地图、模式和节点，申请一台新服务器。', '可继续', ''],
      'waiting-new': ['新申请等待资源', '已沿用上局地图、模式和自动节点方式重新排队；这是新的申请。', '等待中', '新申请不继承原来的排队位置。']
    }[request];
    $('#request-title').textContent = view[0];
    $('#request-description').textContent = view[1];
    $('#request-pill').textContent = view[2];
    $('#request-pill').dataset.tone = ['unknown', 'quarantined'].includes(request) ? 'warm' : 'green';
    $('#request-footnote').textContent = view[3];
    $('#request-footnote').hidden = !view[3];
    $('#request-facts').hidden = !['unknown', 'quarantined', 'waiting-new'].includes(request);
    $('#request-facts').querySelector('div:last-child strong').textContent = request === 'waiting-new' ? '自动选择节点' : '青岚一号';
    $('#unknown-note').hidden = request !== 'unknown';
    $('#quarantine-note').hidden = request !== 'quarantined';
    $('#abandon-button').hidden = !captain || request !== 'quarantined';
    $('#legacy-panel').hidden = !abandoned;
  }

  function render() {
    const { role } = state;
    $('#solo-panel').hidden = role !== 'solo';
    $('#invited-panel').hidden = role !== 'invited';
    $('#team-panel').hidden = !['captain', 'member'].includes(role);
    $('#invite-panel').hidden = role !== 'captain';
    if (['captain', 'member'].includes(role)) renderTeam();
    renderRequest();
  }

  $('#scenario').addEventListener('change', (event) => loadScenario(event.target.value));
  $('#create-party').addEventListener('click', () => { loadScenario('captain-idle'); notify('演示队伍已创建。'); });
  $('#accept-invite').addEventListener('click', () => { loadScenario('member-idle'); notify('已加入演示队伍。'); });
  $('#decline-invite').addEventListener('click', () => loadScenario('solo'));
  $('#leave-party').addEventListener('click', () => { loadScenario('solo'); notify('已退出演示队伍；队伍服务器未停止。'); });
  $('#disband-party').addEventListener('click', () => { loadScenario('solo'); notify('演示队伍已解散。'); });
  $('#add-demo-member').addEventListener('click', () => {
    const count = 1 + Number(state.teammatePresent) + state.extraMembers.length;
    if (count >= 4) return;
    const name = ['微光', '远山', '浅川'].find((candidate) => !state.extraMembers.includes(candidate));
    state.extraMembers.push(name);
    render();
    notify('演示好友已加入队伍。');
  });
  $('#reset-invite').addEventListener('click', () => {
    state.inviteVersion += 1;
    render();
    notify('邀请链接已重置；旧演示链接失效。');
  });
  $('#copy-invite').addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText($('#invite-link').textContent);
      notify('已复制演示邀请链接。');
    } catch {
      notify('复制未成功，请手动选择演示链接。');
    }
  });
  $('#abandon-button').addEventListener('click', () => {
    $('#dialog-description').textContent = state.nextIntent
      ? '确认后，平台会按上局选项自动提交新申请。这个操作不会关闭旧服务器；它可能仍在运行，并继续占用青岚一号的名额，直到管理员处理完成。'
      : '确认后，你可以重新选地图和模式，申请新服务器。这个操作不会关闭旧服务器；它可能仍在运行，并继续占用青岚一号的名额，直到管理员处理完成。';
    $('#abandon-dialog').showModal();
  });
  $('#close-dialog').addEventListener('click', () => $('#abandon-dialog').close());
  $('#confirm-abandon').addEventListener('click', () => {
    if (state.role !== 'captain' || state.request !== 'quarantined') return;
    state.abandoned = true;
    state.request = state.nextIntent ? 'waiting-new' : 'freed';
    $('#abandon-dialog').close();
    render();
    notify(state.nextIntent ? '已为下一局重新排队；旧服务器仍待处理。' : '可以重新申请服务器；旧服务器仍待处理。');
  });

  render();
})();
