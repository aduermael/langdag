/**
 * E2E tests that connect to a running LangDAG server with mock provider.
 * Run with: LANGDAG_E2E_URL=http://localhost:8080 npx vitest run src/e2e.test.ts
 * The server must be started with LANGDAG_PROVIDER=mock.
 */

import { describe, it, expect } from 'vitest';
import { LangDAGClient } from './client.js';
import type { SSEEvent } from './types.js';

const E2E_URL = process.env.LANGDAG_E2E_URL;

describe.skipIf(!E2E_URL)('E2E Tests', () => {
  function getClient() {
    return new LangDAGClient({ baseUrl: E2E_URL! });
  }

  it('health check', async () => {
    const client = getClient();
    const result = await client.health();
    expect(result.status).toBe('ok');
  });

  it('non-streaming chat flow', async () => {
    const client = getClient();

    // Start chat
    const resp = await client.chat({ message: 'Hello from TypeScript' });
    expect(resp.dag_id).toBeTruthy();
    expect(resp.node_id).toBeTruthy();
    expect(resp.content).toBeTruthy();

    // Continue chat
    const resp2 = await client.continueChat(resp.dag_id, { message: 'Follow up' });
    expect(resp2.dag_id).toBe(resp.dag_id);
    expect(resp2.content).toBeTruthy();

    // Get DAG
    const dag = await client.getDag(resp.dag_id);
    expect(dag.id).toBe(resp.dag_id);
    expect(dag.nodes!.length).toBeGreaterThanOrEqual(4);

    // List DAGs
    const dags = await client.listDags();
    expect(dags.some(d => d.id === resp.dag_id)).toBe(true);

    // Delete DAG
    const del = await client.deleteDag(resp.dag_id);
    expect(del.status).toBe('deleted');
  });

  it('streaming chat', async () => {
    const client = getClient();

    const events: SSEEvent[] = [];
    for await (const event of client.chat({ message: 'Stream test', stream: true })) {
      events.push(event);
    }

    expect(events.length).toBeGreaterThan(0);

    const types = events.map(e => e.type);
    expect(types).toContain('start');
    expect(types).toContain('delta');
    expect(types).toContain('done');

    // Check content was streamed
    const deltas = events.filter(e => e.type === 'delta') as Array<{ type: 'delta'; content: string }>;
    expect(deltas.length).toBeGreaterThan(0);
    const content = deltas.map(d => d.content).join('');
    expect(content).toBeTruthy();

    // Clean up
    const doneEvent = events.find(e => e.type === 'done') as { type: 'done'; dag_id: string } | undefined;
    if (doneEvent) {
      await client.deleteDag(doneEvent.dag_id);
    }
  });

  it('fork chat', async () => {
    const client = getClient();

    // Start chat
    const resp = await client.chat({ message: 'First message' });
    expect(resp.dag_id).toBeTruthy();

    // Fork
    const forkResp = await client.forkChat(resp.dag_id, {
      node_id: resp.node_id,
      message: 'Alternative path',
    });
    expect(forkResp.dag_id).toBeTruthy();
    expect(forkResp.content).toBeTruthy();

    // Clean up
    await client.deleteDag(resp.dag_id);
    if (forkResp.dag_id !== resp.dag_id) {
      await client.deleteDag(forkResp.dag_id);
    }
  });
});
