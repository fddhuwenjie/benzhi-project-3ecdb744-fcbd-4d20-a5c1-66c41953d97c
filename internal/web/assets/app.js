const state = { drills: [], current: null, findingMode: null, findingID: null };
const states = [
  ["draft", "草稿"], ["baseline_frozen", "基线已冻结"], ["in_progress", "进行中"],
  ["remediation", "整改复测"], ["pending_review", "待复核"], ["certified", "已认证"]
];
const eventLabels = {
  "drill.created":"创建演练档案", "baseline.frozen":"冻结版本化基线", "activation.recorded":"登记启用证据",
  "drill.started":"启动现场演练", "observation.submitted":"提交现场记录", "finding.remediation_planned":"登记整改措施",
  "finding.plan_changed":"调整整改计划", "finding.retest_passed":"定向复测通过", "review.submitted":"生成送审快照", "review.returned":"复核退回整改", "review.approved":"批准并封存档案"
};
const activationCategories = [
  ["evacuation_route", "疏散路线"], ["communication_gear", "通信设备"],
  ["accessible_facility", "无障碍设施"], ["personnel_ready", "人员到位"]
];

const $ = selector => document.querySelector(selector);
const escapeHTML = value => String(value ?? "").replace(/[&<>'"]/g, char => ({"&":"&amp;","<":"&lt;",">":"&gt;","'":"&#39;",'"':"&quot;"}[char]));
const localValue = iso => {
  const date = new Date(iso);
  const shifted = new Date(date.getTime() - date.getTimezoneOffset() * 60000);
  return shifted.toISOString().slice(0, 16);
};
const isoValue = value => new Date(value).toISOString();
const requestID = () => crypto.randomUUID();
const mutation = actor => ({ request_id: requestID(), expected_revision: state.current.drill.revision, actor_id: actor || state.current.drill.coordinator_id });

async function api(path, options = {}) {
  const response = await fetch(path, { headers: options.body ? {"Content-Type":"application/json"} : {}, ...options });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    const error = payload.error || payload;
    throw new Error(error.message || `请求失败（${response.status}）`);
  }
  return payload;
}

async function digestEvidence(text) {
  const data = new TextEncoder().encode(text.trim());
  const hash = await crypto.subtle.digest("SHA-256", data);
  return Array.from(new Uint8Array(hash), byte => byte.toString(16).padStart(2, "0")).join("");
}

function notify(message, error = false) {
  const notice = $("#notice");
  notice.textContent = message;
  notice.classList.remove("hidden", "error");
  if (error) notice.classList.add("error");
  window.clearTimeout(notify.timer);
  notify.timer = window.setTimeout(() => notice.classList.add("hidden"), 5000);
}

async function loadList(selectID) {
  const payload = await api("/api/drills");
  state.drills = payload.drills;
  renderList();
  const target = selectID || state.current?.drill.id || state.drills[0]?.drill.id;
  if (target) await loadDrill(target);
  else showEmpty();
}

function renderList() {
  $("#drillList").innerHTML = state.drills.map(item => {
    const active = item.drill.id === state.current?.drill.id ? " active" : "";
    const label = states.find(entry => entry[0] === item.drill.state)?.[1] || item.drill.state;
    return `<button class="drill-item${active}" data-id="${escapeHTML(item.drill.id)}"><strong>${escapeHTML(item.drill.title)}</strong><span>${escapeHTML(item.drill.site_name)} · ${label}</span></button>`;
  }).join("");
}

async function loadDrill(id) {
  state.current = await api(`/api/drills/${id}`);
  renderList();
  renderCase();
  await loadTimeline();
}

function showEmpty() {
  state.current = null;
  $("#emptyState").classList.remove("hidden");
  $("#caseView").classList.add("hidden");
}

function renderCase() {
  const drill = state.current.drill;
  $("#emptyState").classList.add("hidden");
  $("#caseView").classList.remove("hidden");
  $("#siteLabel").textContent = drill.site_name;
  $("#titleLabel").textContent = drill.title;
  $("#revisionBadge").textContent = `revision ${drill.revision}`;
  $("#windowValue").textContent = `${new Date(drill.planned_start).toLocaleString()} 至 ${new Date(drill.planned_end).toLocaleString()}`;
  $("#ruleValue").textContent = drill.rule_version;
  $("#baselineValue").textContent = drill.baseline_version || "未冻结";
  $("#openValue").textContent = drill.findings.filter(item => item.status !== "closed").length;
  renderStateRail(drill.state);
  $("#riskTags").innerHTML = drill.scenario_risks.map(risk => `<span class="tag">${escapeHTML(risk)}</span>`).join("");
  $("#checkpointRows").innerHTML = drill.checkpoints.map(checkpoint => `<tr><td>${checkpoint.sequence}</td><td><strong>${escapeHTML(checkpoint.name)}</strong></td><td>${escapeHTML(checkpoint.responsible_role)}</td><td>${checkpoint.minimum_capacity}</td><td>${checkpoint.time_limit_seconds} 秒</td><td class="${checkpoint.communication_required ? "pass" : ""}">${checkpoint.communication_required ? "必检" : "不要求"}</td><td class="${checkpoint.accessibility_required ? "pass" : ""}">${checkpoint.accessibility_required ? "必检" : "不要求"}</td></tr>`).join("");
  renderPrimaryAction();
  renderObservation();
  renderReceipts();
  renderFindings();
  renderActivation();
  renderReview();
  renderDossier();
}

function renderStateRail(current) {
  const currentIndex = states.findIndex(entry => entry[0] === current);
  $("#stateRail").innerHTML = states.map((entry, index) => `<div class="state-step ${index < currentIndex ? "done" : index === currentIndex ? "current" : ""}"><i></i><span>${entry[1]}</span></div>`).join("");
}

function renderPrimaryAction() {
  const button = $("#primaryAction");
  const drill = state.current.drill;
  let action = "", label = "";
  if (drill.state === "draft") [action, label] = ["freeze", "冻结基线"];
  else if (drill.state === "baseline_frozen" && state.current.activation_status.some(item => !item.valid)) [action, label] = ["activation", drill.activation ? "更新失效证据" : "登记启用证据"];
  else if (drill.state === "baseline_frozen") [action, label] = ["start", "启动演练"];
  else if (["in_progress", "remediation"].includes(drill.state) && state.current.todo.some(item => item.kind === "submit_review")) [action, label] = ["submit-review", "提交独立复核"];
  button.classList.toggle("hidden", !action);
  button.dataset.action = action;
  button.textContent = label;
}

function renderReceipts() {
  const receipts = state.current.drill.receipts || [];
  $("#receiptCount").textContent = receipts.length;
  if (!receipts.length) { $("#receiptList").innerHTML = '<div class="muted-row">暂无判定回执</div>'; return; }
  $("#receiptList").innerHTML = receipts.map(receipt => {
    const checkpoint = state.current.drill.checkpoints.find(item => item.id === receipt.checkpoint_id);
    const items = receipt.items.map(item => {
      const finding = state.current.drill.findings.find(entry => entry.receipt_id === receipt.id && entry.rule_code === item.rule_code);
      const detail = item.deviation ? ` · 偏差 ${item.deviation}${item.unit || ""}` : "";
      const target = !item.passed && finding ? ` data-finding-id="${finding.id}"` : "";
      return `<button type="button" class="judgment ${item.passed ? "passed" : "failed"}"${target}><span>${escapeHTML(item.label)}</span><strong>${item.passed ? "通过" : "失败"}${detail}</strong><small>实测 ${escapeHTML(item.observed_value)} · 阈值 ${escapeHTML(item.threshold_value)} · ${escapeHTML(item.rule_version)}</small></button>`;
    }).join("");
    return `<article class="receipt"><header><strong>第 ${receipt.checkpoint_sequence} 项 · ${escapeHTML(checkpoint?.name || receipt.checkpoint_id)}</strong><code>${escapeHTML(receipt.id)}</code></header><div class="judgment-grid">${items}</div></article>`;
  }).join("");
}

function renderObservation() {
  const drill = state.current.drill;
  const todo = state.current.todo.find(item => item.kind === "observation");
  $("#observationSection").classList.toggle("hidden", !todo || drill.state !== "in_progress");
  if (!todo) return;
  const checkpoint = drill.checkpoints.find(item => item.id === todo.resource_id);
  $("#nextCheckpoint").textContent = `第 ${checkpoint.sequence} 项 · ${checkpoint.name}`;
  const form = $("#observationForm");
  const candidate = Math.max(Date.now(), new Date(drill.planned_start).getTime());
  form.elements.observed_at.value = localValue(new Date(Math.min(candidate, new Date(drill.planned_end).getTime())).toISOString());
  form.elements.participant_count.value = checkpoint.minimum_capacity;
  form.elements.elapsed_seconds.value = checkpoint.time_limit_seconds;
  form.elements.communication_passed.checked = true;
  form.elements.accessibility_passed.checked = true;
}

function renderFindings() {
  const queuedIDs = state.current.todo.filter(item => ["remediation", "retest"].includes(item.kind)).map(item => item.resource_id);
  const findings = [...state.current.drill.findings].sort((left, right) => {
    const leftIndex = queuedIDs.indexOf(left.id), rightIndex = queuedIDs.indexOf(right.id);
    if (leftIndex < 0 && rightIndex < 0) return left.opened_at.localeCompare(right.opened_at);
    if (leftIndex < 0) return 1;
    if (rightIndex < 0) return -1;
    return leftIndex - rightIndex;
  });
  $("#findingCount").textContent = findings.length;
  if (!findings.length) { $("#findingList").innerHTML = '<div class="muted-row">暂无整改项</div>'; return; }
  const severity = {critical:"严重", major:"主要", minor:"一般"};
  const status = {open:"待制定措施", planned:"待复测", closed:"已关闭"};
  $("#findingList").innerHTML = findings.map(finding => {
    const checkpoint = state.current.drill.checkpoints.find(item => item.id === finding.checkpoint_id);
    const todo = state.current.todo.find(item => item.resource_id === finding.id);
    const dueLabels = {normal:"正常", near_due:"临近到期", overdue:`已逾期 ${Math.floor((todo?.overdue_seconds || 0) / 60)} 分钟`};
    const planHistory = (finding.plan_history || []).map(plan => `<li>v${plan.version} · ${new Date(plan.changed_at).toLocaleString()} · ${escapeHTML(plan.owner_id)} · ${escapeHTML(plan.change_reason || "首次登记")}</li>`).join("");
    let action = '<span class="pass">证据闭环</span>';
    if (finding.status === "open") action = `<button class="secondary small finding-action" data-mode="plan" data-id="${finding.id}">制定措施</button>`;
    if (finding.status === "planned") action = `<div class="button-row"><button class="secondary small finding-action" data-mode="edit-plan" data-id="${finding.id}">编辑计划</button><button class="primary small finding-action" data-mode="retest" data-id="${finding.id}">提交复测</button></div>`;
    const due = finding.status === "planned" ? `<div class="due ${escapeHTML(todo?.due_status || "normal")}">${dueLabels[todo?.due_status || "normal"]} · ${new Date(finding.retest_planned_at).toLocaleString()}</div>` : "";
    return `<div id="finding-${finding.id}" class="finding-wrap"><div class="finding-item"><span class="severity ${finding.severity}">${severity[finding.severity]}</span><div><strong>${escapeHTML(finding.rule_code)}</strong><div class="muted-row">${escapeHTML(checkpoint?.name || finding.evidence_category || "复核退回项")}</div>${due}</div><span>${status[finding.status]}</span><span>${escapeHTML(finding.owner_id || "未指派")}</span>${action}</div>${planHistory ? `<details><summary>计划历史（${finding.plan_history.length}）</summary><ol>${planHistory}</ol></details>` : ""}</div>`;
  }).join("");
}

function renderActivation() {
  const evidence = state.current.drill.activation;
  if (!evidence) { $("#activationView").innerHTML = '<div class="muted-row">尚未登记启用证据</div>'; return; }
  $("#activationView").innerHTML = state.current.activation_status.map(status => {
    const record = evidence[status.category];
    return `<div class="evidence-item"><span>${escapeHTML(status.label)}</span><strong class="${status.valid ? "pass" : "failed"}">${status.valid ? "有效" : escapeHTML(status.reason)}</strong><small>至 ${new Date(record.valid_until).toLocaleString()}</small><code>${escapeHTML(record.content_digest)}</code></div>`;
  }).join("") + (state.current.drill.state === "baseline_frozen" ? '<button class="secondary wide" data-action="replace-activation" type="button">重新登记启用证据</button>' : "");
}

function renderReview() {
  const drill = state.current.drill;
  $("#reviewPanel").classList.toggle("hidden", drill.state !== "pending_review");
  if (drill.state !== "pending_review") return;
  const snapshot = drill.current_snapshot;
  $("#snapshotView").innerHTML = `<div class="metric"><span>快照编号</span><strong>${escapeHTML(snapshot.id)}</strong></div><div class="metric"><span>固定 revision</span><strong>${snapshot.revision}</strong></div><div class="metric"><span>现场 / 复测记录</span><strong>${snapshot.observations.length}</strong></div><div class="metric"><span>已关闭问题</span><strong>${snapshot.findings.filter(item => item.status === "closed").length}</strong></div><div class="digest">${snapshot.payload_digest}</div>`;
  $("#reviewChecklist").innerHTML = snapshot.checklist.map(item => `<div class="review-item" data-item-id="${escapeHTML(item.id)}"><strong>${escapeHTML(item.label)}</strong><select aria-label="复核结论"><option value="passed">通过</option><option value="returned">退回</option></select><input aria-label="复核意见" value="核验一致" required></div>`).join("");
}

function renderDossier() {
  const drill = state.current.drill;
  $("#dossierPanel").classList.toggle("hidden", !drill.dossier);
  if (!drill.dossier) return;
  const dossier = drill.dossier;
  $("#dossierView").innerHTML = `<div class="metric"><span>复核签署</span><strong>${escapeHTML(dossier.reviewer_id)}</strong></div><div class="metric"><span>签署时间</span><strong>${new Date(dossier.reviewed_at).toLocaleString()}</strong></div><div class="metric"><span>检查点 / 问题</span><strong>${dossier.metrics.checkpoint_count} / ${dossier.metrics.finding_count}</strong></div><div class="metric"><span>清单通过</span><strong>${dossier.passed_checklist_count}</strong></div><div class="digest">${dossier.checklist_digest}</div><div class="digest">${dossier.document_digest}</div>`;
}

async function mutate(path, body, success) {
  try {
    const result = await api(path, {method:"POST", body:JSON.stringify(body)});
    state.current = result;
    renderCase();
    renderList();
    await loadTimeline();
    await refreshListOnly();
    notify(result.replayed ? "已重放原始幂等响应" : success);
  } catch (error) { notify(error.message, true); }
}

async function refreshListOnly() {
  const payload = await api("/api/drills");
  state.drills = payload.drills;
  renderList();
}

async function primaryAction(action) {
  const drill = state.current.drill;
  if (action === "activation") { openActivation(); return; }
  const paths = {freeze:"freeze", start:"start", "submit-review":"submit-review"};
  if (!paths[action]) return;
  await mutate(`/api/drills/${drill.id}/${paths[action]}`, mutation(), {freeze:"基线已冻结",start:"演练已启动","submit-review":"不可变送审快照已生成"}[action]);
}

function openActivation() {
  const form = $("#activationForm");
  const drill = state.current.drill;
  form.reset();
  const collected = localValue(new Date().toISOString());
  const validUntil = localValue(drill.planned_end);
  for (const [category] of activationCategories) {
    const existing = drill.activation?.[category];
    form.elements[`${category}_collected_at`].value = existing ? localValue(existing.collected_at) : collected;
    form.elements[`${category}_valid_until`].value = existing ? localValue(existing.valid_until) : validUntil;
  }
  $("#activationDialog").showModal();
}

function addCheckpoint(values = {}) {
  const editor = $("#checkpointEditor");
  const index = editor.children.length + 1;
  const row = document.createElement("div");
  row.className = "checkpoint-row";
  row.innerHTML = `<input name="sequence" type="number" min="1" value="${values.sequence || index}" aria-label="顺序" required><input name="name" value="${escapeHTML(values.name || "")}" placeholder="检查点名称" required><input name="role" value="${escapeHTML(values.role || "")}" placeholder="责任岗位" required><input name="capacity" type="number" min="1" value="${values.capacity || 20}" aria-label="最低人数" required><input name="limit" type="number" min="1" value="${values.limit || 180}" aria-label="耗时上限" required><label class="check"><input name="comm" type="checkbox" ${values.comm === false ? "" : "checked"}><span>通信</span></label><label class="check"><input name="access" type="checkbox" ${values.access === false ? "" : "checked"}><span>无障碍</span></label><button class="remove" type="button" title="移除">×</button>`;
  editor.append(row);
}

function openCreate() {
  const form = $("#createForm");
  form.reset();
  form.elements.actor_id.value = "coordinator-01";
  form.elements.risks.value = "地震,通信中断";
  const now = new Date();
  form.elements.planned_start.value = localValue(new Date(now.getTime() - 5 * 60000).toISOString());
  form.elements.planned_end.value = localValue(new Date(now.getTime() + 2 * 3600000).toISOString());
  $("#checkpointEditor").innerHTML = "";
  addCheckpoint({name:"主入口疏散集结", role:"疏散引导岗", capacity:30, limit:240});
  addCheckpoint({name:"应急通信确认", role:"通信保障岗", capacity:30, limit:120});
  $("#createDialog").showModal();
}

function openFinding(id, mode) {
  state.findingID = id;
  state.findingMode = mode;
  const finding = state.current.drill.findings.find(item => item.id === id);
  const planMode = mode === "plan" || mode === "edit-plan";
  $("#findingModeLabel").textContent = planMode ? "原因与措施" : "定向证据";
  $("#findingTitle").textContent = `${finding.rule_code} · ${mode === "edit-plan" ? "调整整改计划" : mode === "plan" ? "制定整改计划" : "提交复测"}`;
  $("#planFields").classList.toggle("hidden", !planMode);
  $("#retestFields").classList.toggle("hidden", mode !== "retest");
  $("#changeReasonField").classList.toggle("hidden", mode !== "edit-plan");
  const form = $("#findingForm");
  form.reset();
  if (planMode) {
    form.elements.cause.value = finding.cause || "现场执行偏差";
    form.elements.corrective_action.value = finding.corrective_action || "";
    form.elements.owner_id.value = finding.owner_id || state.current.drill.coordinator_id;
    form.elements.retest_planned_at.value = finding.retest_planned_at ? localValue(finding.retest_planned_at) : localValue(new Date(Math.min(Date.now() + 60000, new Date(state.current.drill.planned_end).getTime())).toISOString());
  } else {
    const checkpoint = state.current.drill.checkpoints.find(item => item.id === finding.checkpoint_id);
    form.elements.observed_at.value = localValue(new Date(Math.min(Date.now(), new Date(state.current.drill.planned_end).getTime())).toISOString());
    form.elements.participant_count.value = checkpoint?.minimum_capacity || 0;
    form.elements.elapsed_seconds.value = checkpoint?.time_limit_seconds || 0;
    form.elements.communication_passed.checked = true;
    form.elements.accessibility_passed.checked = true;
  }
  $("#findingDialog").showModal();
}

async function loadTimeline() {
  if (!state.current) return;
  try {
    const payload = await api(`/api/drills/${state.current.drill.id}/timeline`);
    $("#timeline").innerHTML = payload.events.map(event => `<li><strong>${escapeHTML(eventLabels[event.type] || event.type)}</strong><span>#${event.sequence} · ${escapeHTML(event.actor_id)} · ${new Date(event.occurred_at).toLocaleString()}</span><code>${event.hash}</code></li>`).join("") || '<li class="muted-row">暂无事件</li>';
  } catch (error) { notify(error.message, true); }
}

document.addEventListener("click", event => {
  const drillButton = event.target.closest(".drill-item");
  if (drillButton) loadDrill(drillButton.dataset.id).catch(error => notify(error.message, true));
  if (event.target.closest("[data-action='new']") || event.target.id === "newDrill") openCreate();
  if (event.target.closest("[data-action='replace-activation']")) openActivation();
  if (event.target.matches("[data-close]")) event.target.closest("dialog").close();
  if (event.target.id === "addCheckpoint") addCheckpoint();
  if (event.target.matches(".checkpoint-row .remove")) event.target.closest(".checkpoint-row").remove();
  const findingButton = event.target.closest(".finding-action");
  if (findingButton) openFinding(findingButton.dataset.id, findingButton.dataset.mode);
  const judgment = event.target.closest(".judgment.failed[data-finding-id]");
  if (judgment) document.querySelector(`#finding-${CSS.escape(judgment.dataset.findingId)}`)?.scrollIntoView({behavior:"smooth", block:"center"});
});

$("#primaryAction").addEventListener("click", event => primaryAction(event.currentTarget.dataset.action));
$("#refresh").addEventListener("click", () => state.current && loadDrill(state.current.drill.id).catch(error => notify(error.message, true)));
$("#loadTimeline").addEventListener("click", loadTimeline);

$("#createForm").addEventListener("submit", async event => {
  event.preventDefault();
  const form = event.currentTarget;
  const checkpoints = Array.from($("#checkpointEditor").children).map(row => ({
    sequence:Number(row.querySelector("[name=sequence]").value), name:row.querySelector("[name=name]").value,
    responsible_role:row.querySelector("[name=role]").value, minimum_capacity:Number(row.querySelector("[name=capacity]").value),
    time_limit_seconds:Number(row.querySelector("[name=limit]").value), communication_required:row.querySelector("[name=comm]").checked,
    accessibility_required:row.querySelector("[name=access]").checked
  }));
  const body = {request_id:requestID(), actor_id:form.elements.actor_id.value, title:form.elements.title.value, site_name:form.elements.site_name.value,
    scenario_risks:form.elements.risks.value.split(/[,，]/).map(value => value.trim()).filter(Boolean), planned_start:isoValue(form.elements.planned_start.value),
    planned_end:isoValue(form.elements.planned_end.value), checkpoints};
  try {
    const result = await api("/api/drills", {method:"POST", body:JSON.stringify(body)});
    state.current = result;
    $("#createDialog").close();
    await loadList(result.drill.id);
    notify("演练档案已创建");
  } catch (error) { notify(error.message, true); }
});

$("#activationForm").addEventListener("submit", async event => {
  event.preventDefault();
  const form = event.currentTarget;
  const body = {...mutation()};
  for (const [category] of activationCategories) {
    body[category] = {collected_at:isoValue(form.elements[`${category}_collected_at`].value), valid_until:isoValue(form.elements[`${category}_valid_until`].value), content_digest:await digestEvidence(form.elements[`${category}_source`].value)};
  }
  $("#activationDialog").close();
  await mutate(`/api/drills/${state.current.drill.id}/activation`, body, "启用证据已完整登记");
});

$("#observationForm").addEventListener("submit", async event => {
  event.preventDefault();
  const form = event.currentTarget;
  const todo = state.current.todo.find(item => item.kind === "observation");
  const body = {...mutation(), checkpoint_id:todo.resource_id, observed_at:isoValue(form.elements.observed_at.value), participant_count:Number(form.elements.participant_count.value),
    elapsed_seconds:Number(form.elements.elapsed_seconds.value), communication_passed:form.elements.communication_passed.checked,
    accessibility_passed:form.elements.accessibility_passed.checked, evidence_digest:await digestEvidence(form.elements.evidence.value)};
  await mutate(`/api/drills/${state.current.drill.id}/observations`, body, "现场记录已判定并写入审计链");
  form.elements.evidence.value = "";
});

$("#findingForm").addEventListener("submit", async event => {
  event.preventDefault();
  const form = event.currentTarget;
  let body, suffix, message;
  if (["plan", "edit-plan"].includes(state.findingMode)) {
    body = {...mutation(), cause:form.elements.cause.value, corrective_action:form.elements.corrective_action.value, owner_id:form.elements.owner_id.value, retest_planned_at:isoValue(form.elements.retest_planned_at.value), change_reason:form.elements.change_reason.value};
    suffix = "plan"; message = state.findingMode === "edit-plan" ? "整改计划已受控调整" : "整改措施与复测计划已登记";
  } else {
    body = {...mutation(), observed_at:isoValue(form.elements.observed_at.value), participant_count:Number(form.elements.participant_count.value), elapsed_seconds:Number(form.elements.elapsed_seconds.value),
      communication_passed:form.elements.communication_passed.checked, accessibility_passed:form.elements.accessibility_passed.checked, evidence_digest:await digestEvidence(form.elements.evidence.value)};
    suffix = "retest"; message = "定向复测通过，问题已关闭";
  }
  $("#findingDialog").close();
  await mutate(`/api/drills/${state.current.drill.id}/findings/${state.findingID}/${suffix}`, body, message);
});

$("#reviewForm").addEventListener("submit", async event => {
  event.preventDefault();
  const form = event.currentTarget;
  const checklist = Array.from($("#reviewChecklist").children).map(row => ({item_id:row.dataset.itemId, conclusion:row.querySelector("select").value, opinion:row.querySelector("input").value}));
  const body = {...mutation(form.elements.reviewer_id.value), decision:event.submitter.value, reason:form.elements.reason.value, snapshot_digest:state.current.drill.current_snapshot.payload_digest, checklist};
  await mutate(`/api/drills/${state.current.drill.id}/review-decision`, body, body.decision === "approved" ? "认证已批准，就绪档案已封存" : "复核意见已退回整改");
});

$("#verifyDossier").addEventListener("click", async () => {
  try {
    const result = await api(`/api/drills/${state.current.drill.id}/dossier/verify`, {method:"POST", body:"{}"});
    notify(result.message);
    const node = document.createElement("div");
    node.className = result.valid ? "verified" : "failed";
    node.textContent = result.valid ? "摘要复算一致" : "摘要校验失败";
    $("#dossierView").append(node);
  } catch (error) { notify(error.message, true); }
});

function prepareExtendedForms() {
  const activationForm = $("#activationForm");
  activationForm.classList.remove("narrow");
  activationForm.querySelectorAll(":scope > label").forEach(label => label.remove());
  const fields = document.createElement("div");
  fields.id = "activationFields";
  fields.className = "activation-fields";
  fields.innerHTML = activationCategories.map(([category, label]) => `<fieldset><legend>${label}</legend><label><span>摘要原文</span><input name="${category}_source" required></label><label><span>采集时间</span><input name="${category}_collected_at" type="datetime-local" required></label><label><span>有效截止时间</span><input name="${category}_valid_until" type="datetime-local" required></label></fieldset>`).join("");
  activationForm.insertBefore(fields, activationForm.querySelector(".dialog-actions"));

  const changeReason = document.createElement("label");
  changeReason.id = "changeReasonField";
  changeReason.className = "hidden";
  changeReason.innerHTML = '<span>变更理由</span><textarea name="change_reason"></textarea>';
  $("#planFields").append(changeReason);

  const checklist = document.createElement("div");
  checklist.id = "reviewChecklist";
  checklist.className = "review-checklist";
  $("#reviewForm").insertBefore(checklist, $("#reviewForm").querySelector("label:nth-of-type(2)"));
}

prepareExtendedForms();
loadList().catch(error => notify(error.message, true));
