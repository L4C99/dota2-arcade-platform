(() => {
  const concepts = [...document.querySelectorAll('[data-concept]')];
  const tabs = [...document.querySelectorAll('[data-target]')];
  const previewToggle = document.querySelector('.preview-toggle');
  let activeRequest = false;
  const presetDescriptions = { N6: '平衡挑战', N7: '进阶挑战', Solo: '单人练习' };

  function updateSummary(concept) {
    const preset = concept.querySelector('input[name$="-preset"]:checked')?.closest('label')?.querySelector('strong')?.textContent.trim() || 'N6';
    const chosenNode = concept.querySelector('input[name$="-node"]:checked');
    const nodeName = chosenNode?.parentElement?.querySelector('span:last-child')?.firstChild?.textContent.trim();
    const nodeText = nodeName ? `手动指定：${nodeName}` : '自动选择节点';
    concept.classList.toggle('manual-node', Boolean(nodeName));

    concept.querySelectorAll('[data-preset-summary], [data-active-preset]').forEach((element) => {
      element.textContent = concept.dataset.concept === 'c' ? `${preset} · ${presetDescriptions[preset]}` : preset;
    });
    concept.querySelectorAll('[data-node-summary], [data-active-node]').forEach((element) => {
      element.textContent = nodeText;
    });
    const autoTitle = concept.querySelector('.auto-choice strong, .c-auto strong');
    const autoDescription = concept.querySelector('.auto-choice small, .c-auto small');
    if (autoTitle) autoTitle.textContent = nodeText;
    if (autoDescription) autoDescription.textContent = nodeName ? '只等待该节点' : concept.dataset.concept === 'a' ? '当前默认 · 优先可用资源' : '系统会在可用节点中安排';
  }

  function showConcept(value) {
    const target = ['a', 'b', 'c'].includes(value) ? value : 'a';
    concepts.forEach((concept) => {
      concept.hidden = concept.dataset.concept !== target;
    });
    tabs.forEach((tab) => {
      tab.setAttribute('aria-pressed', String(tab.dataset.target === target));
    });
  }

  function showRequest(value) {
    activeRequest = value;
    concepts.forEach((concept) => concept.classList.toggle('has-request', value));
    previewToggle.setAttribute('aria-pressed', String(value));
    previewToggle.textContent = value ? '返回未申请预览' : '预览已有申请';
  }

  tabs.forEach((tab) => tab.addEventListener('click', () => {
    const target = tab.dataset.target;
    showConcept(target);
    history.replaceState(null, '', `#${target}`);
  }));
  concepts.forEach((concept) => {
    concept.addEventListener('change', (event) => {
      const input = event.target;
      if (input.type !== 'radio') return;
      const kind = input.name.endsWith('-preset') ? 'preset' : input.name.endsWith('-node') ? 'node' : null;
      if (!kind) return;
      const radios = [...concept.querySelectorAll(`input[name$="-${kind}"]`)];
      const selectedIndex = radios.indexOf(input);
      concepts.forEach((other) => {
        const matching = other.querySelectorAll(`input[name$="-${kind}"]`);
        if (matching[selectedIndex]) matching[selectedIndex].checked = true;
        updateSummary(other);
      });
    });
    const details = concept.querySelector('.manual-disclosure');
    const reset = document.createElement('button');
    reset.type = 'button';
    reset.className = 'auto-reset';
    reset.textContent = '恢复自动选择';
    reset.addEventListener('click', () => {
      concepts.forEach((other) => {
        other.querySelectorAll('input[name$="-node"]').forEach((input) => { input.checked = false; });
        updateSummary(other);
      });
    });
    details.append(reset);
    updateSummary(concept);
  });
  previewToggle.addEventListener('click', () => showRequest(!activeRequest));
  document.querySelectorAll('.request-button').forEach((button) => button.addEventListener('click', () => showRequest(true)));
  document.querySelectorAll('.return-button').forEach((button) => button.addEventListener('click', () => showRequest(false)));
  window.addEventListener('hashchange', () => showConcept(location.hash.slice(1)));
  showConcept(location.hash.slice(1));
})();
