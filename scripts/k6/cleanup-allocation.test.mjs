import { test } from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import { readFileSync } from 'node:fs';

const script = readFileSync(new URL('./cleanup-allocation.js', import.meta.url), 'utf8');
const tenant = '9007199254740999';
const warehouse = '9007199254740997';
const sku = '9007199254740995';
const env = { WAREHOUSE_ID: warehouse, SKU_ID: sku, K6_USERNAME: 'proof-user' };
const clone = value => JSON.parse(JSON.stringify(value));

function order(index, overrides = {}) {
  const id = (9007199254741000n + BigInt(index)).toString();
  return {
    order: { id, tenant_id: tenant, warehouse_id: warehouse, biz_order_no: `K6PROOF${index}`,
      status: 'PICKING', allocated_qty: 8, picked_qty: 0, ...overrides },
    details: [{ order_id: id, tenant_id: tenant, sku_id: sku, picked_qty: 0 }],
    tasks: [{ order_id: id, tenant_id: tenant, warehouse_id: warehouse, sku_id: sku, done_qty: 0 }],
    allocations: [{ order_id: id, tenant_id: tenant, sku_id: sku, picked_qty: 0, allocated_qty: 8, status: 'ALLOCATED' }],
  };
}

function inventoryRow(index = 1, allocated = 8) {
  return { id: String(index), tenant_id: tenant, warehouse_id: warehouse, sku_id: sku,
    stock_quantity: 10, available_quantity: 10 - allocated, allocated_quantity: allocated };
}

function fixture(orders = [order(1)], inventory = [inventoryRow()]) {
  const calls = [], posts = [];
  const state = { orders, inventory, calls, posts, mutateGet: null, mutatePost: null };
  state.get = path => {
    calls.push(path);
    const url = new URL(path, 'http://fixture');
    let result;
    if (url.pathname === '/api/v1/inventory') result = inventory;
    else if (url.pathname === '/api/v1/outbound/orders') result = orders.map(item => item.order);
    else {
      const item = orders.find(item => path.endsWith('/' + item.order.id));
      assert.ok(item, `unknown fixture path ${path}`);
      return state.mutateGet?.(path, clone(item)) ?? clone(item);
    }
    const page = Number(url.searchParams.get('page'));
    const response = { list: clone(result.slice((page - 1) * 100, page * 100)), total: result.length };
    return state.mutateGet?.(path, response) ?? response;
  };
  state.post = (path, body) => {
    posts.push({ path, body: clone(body) });
    assert.equal(path, '/api/v1/outbound/orders/batch-cancel');
    if (state.mutatePost) return state.mutatePost(path, body);
    for (const id of body.ids) {
      const item = orders.find(item => item.order.id === id);
      assert.ok(item);
      let remaining = item.order.allocated_qty;
      for (const row of inventory) {
        const release = Math.min(remaining, row.allocated_quantity);
        row.allocated_quantity -= release; row.available_quantity += release; remaining -= release;
      }
      assert.equal(remaining, 0);
      item.order.status = 'CANCELLED';
    }
    return { success: body.ids.length, fail: 0 };
  };
  return state;
}

async function execute(state, overrides = {}, tokenTenant = tenant) {
  const settings = { ...env, ...overrides };
  const checks = [], logs = [];
  let abort, logins = 0;
  const context = vm.createContext({ __ENV: settings, console: { log: text => logs.push(text) } });
  const modules = {
    k6: { check: (value, predicates) => {
      const result = Object.values(predicates).map(predicate => !!predicate(value));
      checks.push(...result); return result.every(Boolean);
    } },
    'k6/execution': { default: { test: { abort: message => { abort = message; throw new Error('fixture-abort'); } } } },
    'k6/encoding': { default: { b64decode: value => Buffer.from(value, 'base64url').toString('utf8') } },
    './lib.js': {
      WAREHOUSE_ID: settings.WAREHOUSE_ID?.trim() || '', SKU_ID: settings.SKU_ID?.trim() || '',
      USERNAME: settings.K6_USERNAME || 'admin',
      login: () => { logins++; return 'header.' + Buffer.from(`{"username":"proof-user","tid":${tokenTenant}}`).toString('base64url') + '.signature'; },
      postJSON: (path, _token, body) => response(state.post(path, body)),
    },
  };
  function response(data) { return { status: 200, json: () => ({ code: 0, data: clone(data) }) }; }
  modules['./lib.js'].getJSON = path => response(state.get(path));
  const entry = new vm.SourceTextModule(script, { context });
  await entry.link(name => {
    assert.ok(modules[name], `unexpected dependency ${name}`);
    const values = modules[name];
    return new vm.SyntheticModule(Object.keys(values), function () {
      for (const [key, value] of Object.entries(values)) this.setExport(key, value);
    }, { context });
  });
  await entry.evaluate();
  try { entry.namespace.default(); } catch (error) { if (!abort) throw error; }
  return { abort, checks, logs, logins, options: JSON.parse(JSON.stringify(entry.namespace.options)) };
}

function passed(result) { assert.equal(result.abort, undefined); assert.ok(result.checks.every(Boolean)); }
function blocked(result, state, match) { assert.match(result.abort, match); assert.ok(result.checks.includes(false)); assert.equal(state.posts.length, 0); }

test('default preview is read only; execute cancels only once and supports rerun', async () => {
  const state = fixture();
  const preview = await execute(state); passed(preview);
  assert.equal(state.posts.length, 0);
  assert.match(preview.logs[0], /只读预览/);
  assert.deepEqual(preview.options.thresholds.checks, ['rate==1']);
  passed(await execute(state, { CLEANUP_EXECUTE: '1' }));
  assert.equal(state.posts.length, 1);
  assert.equal(typeof state.posts[0].body.ids[0], 'string');
  assert.equal(state.inventory[0].allocated_quantity, 0);
  passed(await execute(state, { CLEANUP_EXECUTE: '1' }));
  assert.equal(state.posts.length, 1);
});

test('missing/invalid scope and mode stop before login or any request', async () => {
  for (const settings of [{ WAREHOUSE_ID: '' }, { SKU_ID: '' }, { K6_USERNAME: '' },
    { WAREHOUSE_ID: '1e3' }, { SKU_ID: '9223372036854775808' }, { SKU_ID: '01' },
    { CLEANUP_EXECUTE: 'yes' }, { CLEANUP_PREFIX: 'K6%' }, { CLEANUP_PREFIX: 'ordinary' }]) {
    const state = fixture(); const result = await execute(state, settings);
    assert.ok(result.abort); assert.equal(result.logins, 0); assert.equal(state.calls.length, 0); assert.equal(state.posts.length, 0);
  }
});

test('platform tenant is rejected before any business request', async () => {
  const state = fixture(); blocked(await execute(state, { CLEANUP_EXECUTE: '1' }, '0'), state, /租户/);
  assert.equal(state.calls.length, 0);
});

test('exact prefix, warehouse, tenant, single SKU: mixed orders are excluded', async () => {
  const foreignWarehouse = order(2, { warehouse_id: '2' });
  const foreignPrefix = order(3, { biz_order_no: 'NOT-K6PROOF' });
  const foreignTenant = order(4, { tenant_id: '3' });
  const foreignSKU = order(5); foreignSKU.details[0].sku_id = '4';
  const mixedSKU = order(6); mixedSKU.details.push({ sku_id: '4' });
  const state = fixture([order(1), foreignWarehouse, foreignPrefix, foreignTenant, foreignSKU, mixedSKU]);
  passed(await execute(state, { CLEANUP_EXECUTE: '1' }));
  assert.deepEqual(state.posts[0].body.ids, [order(1).order.id]);
});

test('more than ten pages and batches over 200 preserve all int64 IDs', async () => {
  const orders = Array.from({ length: 1101 }, (_, index) => {
    const item = order(index); item.order.status = 'DRAFT'; item.order.allocated_qty = 0; item.allocations = []; item.tasks = []; return item;
  });
  const state = fixture(orders, []);
  passed(await execute(state, { CLEANUP_EXECUTE: '1' }));
  assert.ok(state.calls.some(path => path.includes('page=12&')));
  assert.equal(state.posts.length, 6);
  assert.deepEqual(state.posts.map(item => item.body.ids.length), [200, 200, 200, 200, 200, 101]);
  assert.equal(new Set(state.posts.flatMap(item => item.body.ids)).size, 1101);
});

test('inventory includes rows after the first hundred', async () => {
  const state = fixture([order(1)], [...Array.from({ length: 100 }, (_, i) => inventoryRow(i + 1, 0)), inventoryRow(101)]);
  passed(await execute(state, { CLEANUP_EXECUTE: '1' }));
  assert.ok(state.calls.some(path => path.startsWith('/api/v1/inventory?') && path.includes('page=2&')));
  assert.equal(state.inventory[100].allocated_quantity, 0);
});

test('page changes, repeats, truncation, malformed schema and excessive totals block all writes', async () => {
  for (const kind of ['change', 'repeat', 'short', 'missing', 'budget']) {
    const orders = Array.from({ length: 101 }, (_, i) => order(i));
    const state = fixture(orders);
    state.mutateGet = (path, payload) => {
      if (!path.startsWith('/api/v1/outbound/orders?')) return payload;
      if (kind === 'missing') return {};
      if (kind === 'budget') return { list: [], total: 100001 };
      if (kind === 'short') return { list: [], total: 101 };
      if (path.includes('page=2&')) {
        if (kind === 'change') payload.total++;
        if (kind === 'repeat') payload.list[0] = clone(orders[0].order);
      }
      return payload;
    };
    const result = await execute(state, { CLEANUP_EXECUTE: '1' });
    assert.ok(result.abort, kind); assert.equal(state.posts.length, 0);
  }
});

test('picked/shipped/inconsistent orders and unrelated allocations block all writes', async () => {
  for (const kind of ['picked', 'task', 'shipped', 'master', 'unrelated', 'foreign-inventory', 'missing-quantity', 'unsafe-id']) {
    const item = order(1); const state = fixture([item]);
    if (kind === 'picked') item.order.picked_qty = 1;
    if (kind === 'task') item.tasks[0].done_qty = 1;
    if (kind === 'shipped') item.order.status = 'SHIPPED';
    if (kind === 'master') item.order.allocated_qty = 9;
    if (kind === 'unrelated') { state.inventory[0].allocated_quantity = 9; state.inventory[0].available_quantity = 1; }
    if (kind === 'foreign-inventory') state.inventory[0].sku_id = '7';
    if (kind === 'missing-quantity') delete state.inventory[0].allocated_quantity;
    if (kind === 'unsafe-id') item.order.id = 9007199254741001;
    const result = await execute(state, { CLEANUP_EXECUTE: '1' });
    assert.ok(result.abort, kind); assert.equal(state.posts.length, 0);
  }
});

test('detail changed just before cancellation blocks the batch', async () => {
  const state = fixture(); let reads = 0;
  state.mutateGet = (path, payload) => {
    if (payload.order && ++reads === 2) payload.details[0].sku_id = '8';
    return payload;
  };
  blocked(await execute(state, { CLEANUP_EXECUTE: '1' }), state, /变化/);
});

test('partial/unknown cancellation responses stop without automatic retry', async () => {
  for (const payload of [{ success: 0, fail: 1 }, {}, { success: 1, fail: 0, errors: [{ id: '1', msg: 'failed' }] }]) {
    const state = fixture(); state.mutatePost = () => payload;
    assert.ok((await execute(state, { CLEANUP_EXECUTE: '1' })).abort);
    assert.equal(state.posts.length, 1);
  }
});

test('unconfirmed cancellation or broken inventory recovery is a failure', async () => {
  for (const kind of ['status', 'inventory']) {
    const state = fixture();
    state.mutatePost = () => {
      if (kind === 'inventory') state.orders[0].order.status = 'CANCELLED';
      return { success: 1, fail: 0 };
    };
    const result = await execute(state, { CLEANUP_EXECUTE: '1' });
    assert.ok(result.abort); assert.equal(state.posts.length, 1);
  }
});


test('JSON key enumeration order is not a business version', async () => {
  const state = fixture(); let reads = 0;
  state.mutateGet = (path, payload) => {
    if (payload.order && ++reads === 2) {
      payload.order = Object.fromEntries(Object.entries(payload.order).reverse());
      payload.details = payload.details.map(row => Object.fromEntries(Object.entries(row).reverse()));
    }
    return payload;
  };
  passed(await execute(state, { CLEANUP_EXECUTE: '1' }));
  assert.equal(state.posts.length, 1);
});


test('later batch failure keeps confirmed earlier progress and stops remaining batches', async () => {
  const orders = Array.from({ length: 401 }, (_, index) => {
    const item = order(index); item.order.status = 'DRAFT'; item.order.allocated_qty = 0; item.allocations = []; item.tasks = []; return item;
  });
  const state = fixture(orders, []);
  state.mutatePost = (_path, body) => {
    if (state.posts.length === 1) {
      for (const id of body.ids) orders.find(item => item.order.id === id).order.status = 'CANCELLED';
      return { success: body.ids.length, fail: 0 };
    }
    return { success: 0, fail: body.ids.length };
  };
  const result = await execute(state, { CLEANUP_EXECUTE: '1' });
  assert.ok(result.abort); assert.equal(state.posts.length, 2);
  assert.equal(orders.filter(item => item.order.status === 'CANCELLED').length, 200);
});
