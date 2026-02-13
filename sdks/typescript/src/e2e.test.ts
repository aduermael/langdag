/**
 * E2E tests that connect to a running LangDAG server with mock provider.
 * Run with: LANGDAG_E2E_URL=http://localhost:8080 npx vitest run src/e2e.test.ts
 * The server must be started with LANGDAG_PROVIDER=mock.
 */

import { describe, it, expect } from 'vitest';
import { LangDAGClient, Node, Stream } from './client.js';
import { NotFoundError } from './errors.js';
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

  it('non-streaming prompt flow', async () => {
    const client = getClient();

    // Start new conversation
    const node1 = await client.prompt('Hello from TypeScript');
    expect(node1).toBeInstanceOf(Node);
    expect(node1.id).toBeTruthy();
    expect(node1.content).toBeTruthy();

    // Continue from that node using node.prompt()
    const node2 = await node1.prompt('Follow up');
    expect(node2).toBeInstanceOf(Node);
    expect(node2.id).toBeTruthy();
    expect(node2.content).toBeTruthy();

    // Get the tree
    const tree = await client.getTree(node1.id);
    expect(tree.length).toBeGreaterThanOrEqual(4);
    expect(tree[0]).toBeInstanceOf(Node);

    // List roots
    const roots = await client.listRoots();
    expect(roots.length).toBeGreaterThan(0);
    expect(roots[0]).toBeInstanceOf(Node);

    // Get single node
    const node = await client.getNode(node1.id);
    expect(node).toBeInstanceOf(Node);
    expect(node.id).toBeTruthy();

    // Delete — find root node
    const rootNode = tree.find(n => !n.parentId);
    if (rootNode) {
      await client.deleteNode(rootNode.id);
    }
  });

  it('streaming prompt', async () => {
    const client = getClient();

    const stream = await client.promptStream('Stream test');
    expect(stream).toBeInstanceOf(Stream);

    const events: SSEEvent[] = [];
    for await (const event of stream.events()) {
      events.push(event);
    }

    expect(events.length).toBeGreaterThan(0);
    const types = events.map(e => e.type);
    expect(types).toContain('start');
    expect(types).toContain('delta');
    expect(types).toContain('done');

    // Get the final node from the stream
    const node = await stream.node();
    expect(node).toBeInstanceOf(Node);
    expect(node.id).toBeTruthy();
    expect(node.content).toBeTruthy();

    // Clean up
    await client.deleteNode(node.id);
  });

  it('node.prompt (branching)', async () => {
    const client = getClient();

    // Start conversation
    const node1 = await client.prompt('First message');
    expect(node1).toBeInstanceOf(Node);

    // Branch from the assistant response node using node.prompt()
    const node2 = await node1.prompt('Alternative path');
    expect(node2).toBeInstanceOf(Node);
    expect(node2.content).toBeTruthy();

    // Get tree to verify branching
    const tree = await client.getTree(node1.id);
    expect(tree.length).toBeGreaterThanOrEqual(4);

    // Clean up - delete the root node
    const rootNode = tree.find(n => !n.parentId);
    if (rootNode) {
      await client.deleteNode(rootNode.id);
    }
  });

  it('node.promptStream (streaming continuation)', async () => {
    const client = getClient();

    // Start conversation
    const node1 = await client.prompt('First message for stream test');

    // Continue with streaming using node.promptStream()
    const stream = await node1.promptStream('Continue with streaming');
    expect(stream).toBeInstanceOf(Stream);

    const events: SSEEvent[] = [];
    for await (const event of stream.events()) {
      events.push(event);
    }

    expect(events.map(e => e.type)).toContain('done');

    const node2 = await stream.node();
    expect(node2).toBeInstanceOf(Node);
    expect(node2.content).toBeTruthy();

    // Clean up
    const tree = await client.getTree(node1.id);
    const rootNode = tree.find(n => !n.parentId);
    if (rootNode) {
      await client.deleteNode(rootNode.id);
    }
  });

  it('error: get non-existent node', async () => {
    const client = getClient();
    await expect(client.getNode('nonexistent-node-id-12345')).rejects.toThrow();
    try {
      await client.getNode('nonexistent-node-id-12345');
    } catch (error) {
      expect(error).toBeInstanceOf(NotFoundError);
    }
  });

  it('error: delete non-existent node', async () => {
    const client = getClient();
    await expect(client.deleteNode('nonexistent-node-id-12345')).rejects.toThrow();
    try {
      await client.deleteNode('nonexistent-node-id-12345');
    } catch (error) {
      expect(error).toBeInstanceOf(NotFoundError);
    }
  });

  it('node metadata fields', async () => {
    const client = getClient();

    const node1 = await client.prompt('Test metadata fields');

    // Get the tree to see full node details
    const tree = await client.getTree(node1.id);
    expect(tree.length).toBeGreaterThanOrEqual(2);

    // Find user and assistant nodes
    const userNode = tree.find(n => n.type === 'user');
    const assistantNode = tree.find(n => n.type === 'assistant');

    expect(userNode).toBeDefined();
    expect(assistantNode).toBeDefined();

    // Verify user node fields
    expect(userNode!.id).toBeTruthy();
    expect(userNode!.content).toBeTruthy();
    expect(userNode!.sequence).toBeGreaterThanOrEqual(0);
    expect(userNode!.createdAt).toBeTruthy();

    // Verify assistant node fields
    expect(assistantNode!.id).toBeTruthy();
    expect(assistantNode!.content).toBeTruthy();
    expect(assistantNode!.parentId).toBeTruthy();

    // Clean up
    const rootNode = tree.find(n => !n.parentId);
    if (rootNode) {
      await client.deleteNode(rootNode.id);
    }
  });

  it('streaming content accumulation', async () => {
    const client = getClient();

    const stream = await client.promptStream('Tell me something');

    let accumulatedContent = '';
    for await (const event of stream.events()) {
      if (event.type === 'delta') {
        accumulatedContent += event.content;
      }
    }

    const node = await stream.node();
    expect(node.content).toBe(accumulatedContent);
    expect(accumulatedContent.length).toBeGreaterThan(0);

    // Clean up
    await client.deleteNode(node.id);
  });

  it('node ID prefix lookup', async () => {
    const client = getClient();

    const node1 = await client.prompt('Test prefix lookup');

    // Use first 8 characters as prefix
    const prefix = node1.id.substring(0, 8);
    const resolved = await client.getNode(prefix);

    expect(resolved.id).toBe(node1.id);
    expect(resolved.content).toBeTruthy();

    // Clean up
    const tree = await client.getTree(node1.id);
    const rootNode = tree.find(n => !n.parentId);
    if (rootNode) {
      await client.deleteNode(rootNode.id);
    }
  });
});
