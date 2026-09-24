(() => {
  const concept = document.querySelector('.concept-a');
  const previewToggle = document.querySelector('.preview-toggle');
  let activeRequest = false;
  function updateSummary() {
    const preset = concept.querySelector('input[name$="-preset"]:checked')?.closest('label')?.querySelector('strong')?.textContent.trim() || 'N6';
    const chosenNode = concept.querySelector('input[name$="-node"]:checked');
    const nodeName = chosenNode?.parentElement?.querySelector('span:last-child')?.firstChild?.textContent.trim();
    const nodeText = nodeName ? `手动指定：${nodeName}` : '自动选择节点';
    concept.classList.toggle('manual-node', Boolean(nodeName));

    concept.querySelectorAll('[data-active-preset]').forEach((element) => { element.textContent = preset; });
    concept.querySelectorAll('[data-active-node]').forEach((element) => { element.textContent = nodeText; });
    const autoTitle = concept.querySelector('.auto-choice strong');
    const autoDescription = concept.querySelector('.auto-choice small');
    if (autoTitle) autoTitle.textContent = nodeText;
    if (autoDescription) autoDescription.textContent = nodeName ? '只等待该节点' : '当前默认 · 优先可用资源';
  }

  function showRequest(value) {
    activeRequest = value;
    concept.classList.toggle('has-request', value);
    previewToggle.setAttribute('aria-pressed', String(value));
    previewToggle.textContent = value ? '返回未申请预览' : '预览已有申请';
  }

  concept.addEventListener('change', (event) => {
    const input = event.target;
    if (input.type !== 'radio') return;
    if (!input.name.endsWith('-preset') && !input.name.endsWith('-node')) return;
    updateSummary();
  });
  const details = concept.querySelector('.manual-disclosure');
  const reset = document.createElement('button');
  reset.type = 'button';
  reset.className = 'auto-reset';
  reset.textContent = '恢复自动选择';
  reset.addEventListener('click', () => {
    concept.querySelectorAll('input[name$="-node"]').forEach((input) => { input.checked = false; });
    updateSummary();
  });
  details.append(reset);
  updateSummary();
  previewToggle.addEventListener('click', () => showRequest(!activeRequest));
  concept.querySelector('.request-button').addEventListener('click', () => showRequest(true));
  concept.querySelector('.return-button').addEventListener('click', () => showRequest(false));
})();
