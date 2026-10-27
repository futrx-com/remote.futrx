import assert from 'node:assert/strict';
import test from 'node:test';
import activate, { installed } from './main.js';
import { openTasks } from './tasksPanel.js';
import { taskState, scheduleSummary, friendlyError, readableTime, isArchived } from './taskView.js';

test('past and finished reminders are distinguished from resumable paused tasks', () => {
  const task = { kind: 'once', at: '2026-10-06T12:00:00Z', enabled: false, runCount: 0 };
  assert.equal(taskState(task, Date.parse('2026-10-07T12:00:00Z')), 'Time has passed');
  assert.equal(taskState({ ...task, runCount: 1 }), 'Finished');
  assert.equal(taskState({ ...task, kind: 'cron' }), 'Paused');
});

test('schedule summaries explain common cron schedules and handle invalid dates', () => {
  assert.equal(scheduleSummary({ kind: 'cron', cron: '*/5 * * * *', timezone: 'UTC' }), 'Every 5 minutes (UTC)');
  assert.equal(scheduleSummary({ kind: 'cron', cron: '0 15 * * *', timezone: 'UTC' }), 'Every day at 15:00 (UTC)');
  assert.equal(readableTime('invalid'), 'Time unavailable');
  assert.match(friendlyError(new Error('at must be a future RFC3339 time with offset')), /Use Run now/);
  assert.match(friendlyError(new Error('Failed to fetch')), /connection/);
});

test('chat action uses the running project instance and disappears without it', () => {
  let action;
  const remote = {
    backend: { instances: [{ scope: 'project', projectId: 'project' }] },
    slots: { chatHeaderActions: 'chatHeaderActions' },
    ui: { register: (slot, render, spec) => { assert.equal(slot, 'chatHeaderActions'); action = spec; } },
  };
  activate(remote);
  assert.equal(action.when({ projectId: 'project', chatId: 'chat' }), true);
  assert.equal(action.when({ projectId: 'other', chatId: 'chat' }), false);
  assert.equal(action.when({ projectId: 'project' }), false);
  remote.backend.instances = [];
  assert.equal(action.when({ projectId: 'project', chatId: 'chat' }), false);
  assert.equal(installed(remote, ''), false);
});

test('task list sends chatId through the backend query option', async () => {
  const previousDocument = globalThis.document;
  const calls = [];
  let cleanup;
  const element = () => ({
    classList: { add() {}, remove() {} },
    setAttribute() {},
    replaceChildren() {},
    append() {},
    addEventListener() {},
    removeEventListener() {},
    disabled: false,
    textContent: '',
    type: '',
  });
  globalThis.document = { createElement: element };
  try {
    openTasks({
      backend: {
        call: async (path, options) => {
          calls.push({ path, options });
          return [];
        },
      },
      ui: {
        openPopup: ({ mount }) => { cleanup = mount(element()); },
      },
    }, { projectId: 'project', chatId: 'fead4a1cf8b6' });
    await new Promise((resolve) => setTimeout(resolve, 0));
  } finally {
    cleanup?.();
    globalThis.document = previousDocument;
  }

  assert.deepEqual(calls, [{
    path: 'tasks',
    options: { projectId: 'project', query: { chatId: 'fead4a1cf8b6' } },
  }]);
});

test('finished tasks and manual archives are hidden by default', () => {
  assert.equal(isArchived({ kind: 'once', enabled: false, runCount: 1 }), true);
  assert.equal(isArchived({ archived: true, kind: 'cron', enabled: false }), true);
  assert.equal(isArchived({ kind: 'cron', enabled: false }), false);
  assert.equal(isArchived({ kind: 'once', enabled: false, runCount: 1, activeRunId: 'run' }), false);
});

test('quiet refresh clears stale pending status, preserves cards on failure, and stops on close', async () => {
  const original = { document: globalThis.document, setInterval: globalThis.setInterval, clearInterval: globalThis.clearInterval };
  let poll, cleanup, cleared = false, fail = false;
  let tasks = [{ id: 'task', name: 'Reminder', kind: 'once', enabled: false, activeRunId: 'run', prompt: 'Review deployment' }];
  const element = () => ({
    children: [], listeners: {}, attributes: {}, classList: { add() {}, remove() {} },
    setAttribute(name, value) { this.attributes[name] = value; },
    append(...children) { this.children.push(...children); },
    replaceChildren(...children) { this.children = children; },
    addEventListener(name, fn) { this.listeners[name] = fn; }, removeEventListener() {}, textContent: '',
  });
  const body = element();
  const flush = () => new Promise(resolve => setTimeout(resolve, 0));
  globalThis.document = { createElement: element };
  globalThis.setInterval = (fn, ms) => { assert.equal(ms, 3000); poll = fn; return 123; };
  globalThis.clearInterval = (id) => { assert.equal(id, 123); cleared = true; };
  try {
    openTasks({ backend: { call: async () => { if (fail) throw new Error('Failed to fetch'); return tasks; } }, ui: { openPopup: ({ mount }) => { cleanup = mount(body); } } }, { projectId: 'project', chatId: 'chat' });
    await flush();
    const list = body.children[2], toggle = body.children[3];
    assert.match(list.children[0].children[1].textContent, /Run pending/);
    const row = list.children[0];
    fail = true; poll(); await flush();
    assert.equal(list.children[0], row);
    assert.match(body.children[0].textContent, /connection/);
    fail = false;
    tasks = [{ ...tasks[0], activeRunId: '', runCount: 1, archived: true }];
    poll(); await flush();
    assert.equal(list.children[0].children[1].textContent, 'No scheduled tasks');
    assert.equal(toggle.children[1].textContent, 'Show archived tasks (1)');
    assert.equal(toggle.attributes['aria-expanded'], 'false');
    assert.equal(toggle.children[0].attributes['aria-hidden'], 'true');
    assert.equal(toggle.children[2].attributes['aria-hidden'], 'true');
    assert.match(toggle.children[0].innerHTML, /<svg/);
    assert.match(toggle.children[2].innerHTML, /<svg/);
    toggle.listeners.click(); await flush();
    assert.equal(list.children[0].children[1].textContent, 'Archived · Finished');
    assert.equal(toggle.children[1].textContent, 'Hide archived tasks (1)');
    assert.equal(toggle.attributes['aria-expanded'], 'true');
    toggle.listeners.click(); await flush();
    assert.equal(toggle.children[1].textContent, 'Show archived tasks (1)');
    assert.equal(toggle.attributes['aria-expanded'], 'false');
    cleanup(); cleanup = undefined;
    assert.equal(cleared, true);
  } finally { cleanup?.(); Object.assign(globalThis, original); }
});

test('task actions pair decorative icons with readable labels and preserve error handling', async () => {
  const previousDocument = globalThis.document;
  let cleanup;
  const calls = [];
  const element = () => ({
    children: [], listeners: {}, attributes: {}, classes: [], disabled: false,
    classList: { add() {}, remove() {} },
    setAttribute(name, value) { this.attributes[name] = value; },
    append(...children) { this.children.push(...children); },
    replaceChildren(...children) { this.children = children; },
    addEventListener(name, fn) { this.listeners[name] = fn; }, removeEventListener() {}, textContent: '',
  });
  const body = element();
  globalThis.document = { createElement: element };
  try {
    openTasks({
      backend: { call: async (path, options) => {
        calls.push({ path, options });
        if (path !== 'tasks') throw new Error('Failed to fetch');
        return [
          { id: 'active', name: 'Daily review', enabled: true, kind: 'cron', cron: '0 15 * * *', prompt: 'Review deployment' },
          { id: 'paused', name: 'Paused review', enabled: false, kind: 'cron', cron: '0 15 * * *', prompt: 'Review deployment' },
        ];
      } },
      ui: { openPopup: ({ mount }) => { cleanup = mount(body); } },
    }, { projectId: 'project', chatId: 'chat' });
    await new Promise(resolve => setTimeout(resolve, 0));
    const rows = body.children[2].children;
    const buttons = rows.flatMap(row => row.children.at(-1).children);
    assert.deepEqual(buttons.map(button => button.children[1].textContent), ['Pause', 'Run now', 'Archive', 'Delete', 'Resume', 'Run now', 'Archive', 'Delete']);
    for (const button of buttons) {
      assert.equal(button.children[0].attributes['aria-hidden'], 'true');
      assert.match(button.children[0].innerHTML, /<svg/);
      assert.doesNotMatch(button.children[0].innerHTML, /undefined/);
    }
    const pause = buttons[0];
    const request = pause.listeners.click();
    assert.equal(pause.disabled, true);
    await request;
    assert.equal(pause.disabled, false);
    assert.equal(pause.children[1].textContent, 'Pause');
    assert.deepEqual(calls[1], { path: 'tasks/active', options: { projectId: 'project', method: 'PATCH', body: { enabled: false } } });
    assert.match(rows[0].children.at(-2).textContent, /connection/);
  } finally { cleanup?.(); globalThis.document = previousDocument; }
});

for (const fail of [false, true]) {
  test(`inline delete confirmation supports cancel, Escape, polling and ${fail ? 'request failure' : 'successful deletion'}`, async () => {
    const original = { document: globalThis.document, setInterval: globalThis.setInterval, clearInterval: globalThis.clearInterval };
    let cleanup, poll, focused, deleted = false;
    const calls = [];
    const element = () => ({
      children: [], listeners: {}, attributes: {}, disabled: false,
      classList: { add() {}, remove() {} },
      setAttribute(name, value) { this.attributes[name] = value; },
      append(...children) { children.forEach(child => { child.parentElement = this; this.children.push(child); }); },
      replaceChildren(...children) { this.children = []; this.append(...children); },
      remove() { this.parentElement.children = this.parentElement.children.filter(child => child !== this); },
      focus() { focused = this; },
      addEventListener(name, fn) { this.listeners[name] = fn; }, removeEventListener() {}, textContent: '',
    });
    const body = element();
    const flush = () => new Promise(resolve => setTimeout(resolve, 0));
    globalThis.document = { createElement: element };
    globalThis.setInterval = fn => { poll = fn; return 123; };
    globalThis.clearInterval = () => {};
    try {
      openTasks({ backend: { call: async (path, options) => {
        calls.push({ path, options });
        if (options.method === 'DELETE') {
          if (fail) throw new Error('Failed to fetch');
          deleted = true;
          return;
        }
        return deleted ? [] : [{ id: 'task/id', name: '<Daily review>', enabled: true, kind: 'cron', cron: '0 15 * * *', prompt: 'Review deployment' }];
      } }, ui: { openPopup: ({ mount }) => { cleanup = mount(body); } } }, { projectId: 'project', chatId: 'chat' });
      await flush();
      const row = body.children[2].children[0];
      const actions = row.children.at(-1);
      const remove = actions.children.at(-1);
      remove.listeners.click();
      const confirmation = actions.children.at(-1);
      assert.equal(confirmation.attributes.role, 'group');
      assert.equal(confirmation.children[0].children[1].textContent, 'Delete this task?');
      assert.equal(confirmation.children[1].textContent, '<Daily review>');
      assert.equal(confirmation.children[0].children[0].attributes['aria-hidden'], 'true');
      const [cancel] = confirmation.children[3].children;
      assert.equal(focused, cancel);
      assert.equal(remove.hidden, true);
      poll(); await flush();
      assert.equal(calls.length, 1);
      cancel.listeners.click();
      assert.equal(remove.hidden, false);
      assert.equal(focused, remove);
      assert.equal(calls.length, 1);
      remove.listeners.click();
      let stopped = false;
      actions.children.at(-1).listeners.keydown({ key: 'Escape', preventDefault() {}, stopPropagation() { stopped = true; } });
      assert.equal(stopped, true);
      assert.equal(actions.children.length, 4);
      remove.listeners.click();
      const confirm = actions.children.at(-1).children[3].children[1];
      const request = confirm.listeners.click();
      assert.equal(confirm.disabled, true);
      await confirm.listeners.click();
      await request;
      assert.deepEqual(calls[1], { path: 'tasks/task%2Fid', options: { projectId: 'project', method: 'DELETE' } });
      assert.equal(calls.filter(call => call.options.method === 'DELETE').length, 1);
      if (fail) {
        assert.match(row.children.at(-2).textContent, /connection/);
        assert.equal(remove.disabled, false);
        assert.equal(remove.hidden, false);
      } else assert.equal(body.children[2].children[0].children[1].textContent, 'No scheduled tasks');
    } finally { cleanup?.(); Object.assign(globalThis, original); }
  });
}
