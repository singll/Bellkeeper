import { readFileSync, readdirSync } from 'node:fs';
import { test } from 'node:test';
import assert from 'node:assert/strict';

const directory = new URL('../internal/n8n_workflows/', import.meta.url);
const workflows = readdirSync(directory).filter(n => n.endsWith('.json')).map(name =>
  JSON.parse(readFileSync(new URL(name, directory), 'utf8')));
const byPrefix = prefix => workflows.find(w => w.name.startsWith(prefix));
function execute(workflow, nodeName, input, state = {}, extras = {}) {
  const node = workflow.nodes.find(n => n.name === nodeName);
  return new Function('$json', '$getWorkflowStaticData', '$execution', '$input', '$', node.parameters.jsCode)(
    input, () => state, { id: 'verification' }, extras.$input, extras.$);
}

test('all active workflows have valid JavaScript and edges', () => {
  for (const workflow of workflows) {
    const names = new Set(workflow.nodes.map(n => n.name));
    for (const n of workflow.nodes) if (n.parameters?.jsCode) new Function(n.parameters.jsCode);
    for (const [source, outputs] of Object.entries(workflow.connections)) {
      assert.ok(names.has(source));
      for (const targets of outputs.main) for (const edge of targets) assert.ok(names.has(edge.node));
    }
    if (workflow.name.startsWith('B02')) assert.equal(workflow.settings.errorWorkflow, '');
    else assert.equal(workflow.settings.errorWorkflow, 'u0W5aC5Q6Y0cZGK6');
  }
});

test('article webhook rejects invalid URLs and submits valid input to persistent queue', () => {
  const w = byPrefix('K01');
  assert.throws(() => execute(w, '校验文章', { body: { url: 'file:///etc/passwd' } }));
  const result = execute(w, '校验文章', { body: { url: ' https://example.com/article ', title: 'Article' } });
  assert.equal(result[0].json.url, 'https://example.com/article');
  assert.equal(result[0].json.channel_type, 'auto');
  assert.ok(w.nodes.some(n => n.parameters.url?.endsWith('/api/crawl/queue/enqueue')));
});

test('health monitor ignores disabled services, reports failure and recovery once', () => {
  const w = byPrefix('O02');
  const state = {};
  const data = status => ({ data: { services: { postgres: { status }, memos: { status: 'disabled' } } } });
  assert.equal(execute(w, '汇总并对比状态', data('up'), state).length, 0);
  assert.match(execute(w, '汇总并对比状态', data('down'), state)[0].json.message, /up → down/);
  assert.equal(execute(w, '汇总并对比状态', data('down'), state).length, 0);
  assert.match(execute(w, '汇总并对比状态', data('up'), state)[0].json.message, /down → up/);
  assert.throws(() => execute(w, '汇总并对比状态', { data: {} }, state));
  assert.equal(w.nodes[0].parameters.rule.interval[0].minutesInterval, 5);
});

test('error alerts read nested n8n error details and redact credentials', () => {
  const result = execute(byPrefix('B02'), '格式化错误告警', {
    workflow: { name: 'K01' }, execution: { id: '42', lastNodeExecuted: 'fetch',
      error: { message: 'failed Bearer test-secret password=hunter2' } },
  })[0].json;
  assert.match(result.message, /K01/);
  assert.match(result.message, /执行: 42/);
  assert.doesNotMatch(result.message, /test-secret|hunter2/);
});
