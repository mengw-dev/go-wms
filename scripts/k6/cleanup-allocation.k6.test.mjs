// 使用真实 k6 进程，只连接本测试启动的回环 HTTP 夹具，不连接 WMS 或数据库。
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const script = fileURLToPath(new URL('./cleanup-allocation.js', import.meta.url));
const tenant = '9007199254740999';
const warehouse = '9007199254740997';
const sku = '9007199254740995';
const orderID = '9007199254741001';
const token = 'header.' + Buffer.from(`{"username":"proof-user","tid":${tenant}}`).toString('base64url') + '.signature';

async function run(mode) {
  let logins = 0, cancels = 0, cancelled = false;
  const requests = [];
  const server = createServer(async (req, res) => {
    const url = new URL(req.url, 'http://fixture'); requests.push(url.pathname);
    const send = (data, status = 200) => { res.writeHead(status, { 'Content-Type': 'application/json' }); res.end(JSON.stringify({ code: status === 200 ? 0 : 500, data })); };
    if (url.pathname === '/api/v1/login') {
      logins++;
      if (mode === 'login-json') { res.end('login-not-json-with-secret'); return; }
      send({ token }); return;
    }
    if (url.pathname === '/api/v1/inventory') {
      assert.equal(url.searchParams.get('warehouse_id'), warehouse);
      assert.equal(url.searchParams.get('sku_id'), sku);
      if (mode === 'http-error') { send(null, 500); return; }
      if (mode === 'non-json') { res.end('not-json'); return; }
      if (mode === 'network') { req.socket.destroy(); return; }
      const released = cancelled && mode !== 'inventory-drift';
      send({ total: 1, list: [{ id: '1', tenant_id: tenant, warehouse_id: warehouse, sku_id: sku,
        stock_quantity: 10, available_quantity: released ? 10 : 2, allocated_quantity: released ? 0 : 8 }] }); return;
    }
    const order = { id: orderID, tenant_id: tenant, warehouse_id: warehouse, biz_order_no: 'K6PROOF-1',
      status: cancelled ? 'CANCELLED' : 'PICKING', allocated_qty: 8, picked_qty: 0 };
    if (url.pathname === '/api/v1/outbound/orders') { send({ total: 1, list: [order] }); return; }
    if (url.pathname === `/api/v1/outbound/orders/${orderID}`) {
      send({ order,
        details: [{ order_id: orderID, tenant_id: tenant, sku_id: sku, picked_qty: 0 }],
        tasks: [{ order_id: orderID, tenant_id: tenant, warehouse_id: warehouse, sku_id: sku, done_qty: 0 }],
        allocations: [{ order_id: orderID, tenant_id: tenant, sku_id: sku, picked_qty: 0, allocated_qty: 8, status: 'ALLOCATED' }],
      }); return;
    }
    if (url.pathname === '/api/v1/outbound/orders/batch-cancel') {
      let body = ''; for await (const chunk of req) body += chunk;
      assert.deepEqual(JSON.parse(body), { ids: [orderID] });
      cancels++;
      if (mode === 'partial') { send({ success: 0, fail: 1 }); return; }
      cancelled = true;
      if (mode === 'unknown-post') { res.end('unknown-result'); return; }
      send({ success: 1, fail: 0 }); return;
    }
    send(null, 404);
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const base = `http://127.0.0.1:${server.address().port}`;
  // 避免继承本机 K6_OUT、凭据和其他目标配置；HTTP 请求仅发往本轮回环夹具。
  const cleanEnv = Object.fromEntries(Object.entries(process.env).filter(([key]) => !key.startsWith('K6_')));
  cleanEnv.K6_NO_USAGE_REPORT = 'true';
  const args = ['run', '--quiet', '-e', `BASE_URL=${base}`, '-e', 'K6_USERNAME=proof-user',
    '-e', `WAREHOUSE_ID=${warehouse}`, '-e', `SKU_ID=${mode === 'invalid-scope' ? '' : sku}`,
    '-e', `CLEANUP_EXECUTE=${mode === 'preview' ? '0' : '1'}`, script];
  let output = '';
  try {
    const code = await new Promise((resolve, reject) => {
      const child = spawn(process.env.K6_BIN || 'k6', args, { env: cleanEnv, windowsHide: true });
      const timeout = setTimeout(() => { child.kill(); reject(new Error('k6 fixture timed out')); }, 20000);
      child.stdout.on('data', data => { output += data; }); child.stderr.on('data', data => { output += data; });
      child.once('error', error => { clearTimeout(timeout); reject(error); });
      child.once('close', code => { clearTimeout(timeout); resolve(code); });
    });
    return { code, cancels, logins, output, requests };
  } finally {
    server.closeAllConnections();
    await new Promise(resolve => server.close(resolve));
  }
}

for (const mode of ['preview', 'execute', 'invalid-scope', 'partial', 'http-error', 'non-json', 'network', 'login-json', 'unknown-post', 'inventory-drift']) {
  test(`native k6 exit and request scope: ${mode}`, { timeout: 25000 }, async () => {
    const result = await run(mode);
    assert.equal(result.code, ['preview', 'execute'].includes(mode) ? 0 : 108, result.output);
    assert.equal(result.cancels, ['execute', 'partial', 'unknown-post', 'inventory-drift'].includes(mode) ? 1 : 0, result.output);
    if (mode === 'invalid-scope') assert.equal(result.logins, 0);
    if (mode === 'preview') assert.ok(!result.requests.includes('/api/v1/outbound/orders/batch-cancel'));
    assert.ok(!result.output.includes(token));
    assert.ok(!result.output.includes('login-not-json-with-secret'));
  });
}
