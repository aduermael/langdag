import { describe, it, expect, vi } from 'vitest';
import { LangDAGClient, Node, Stream } from './client.js';
import { UnauthorizedError, NotFoundError, BadRequestError, ApiError, NetworkError } from './errors.js';

function mockFetch(response: Partial<Response>): typeof fetch {
  return vi.fn().mockResolvedValue({
    ok: response.ok ?? true,
    status: response.status ?? 200,
    statusText: response.statusText ?? 'OK',
    json: response.json ?? (() => Promise.resolve({})),
    headers: response.headers ?? new Headers(),
    body: response.body ?? null,
  } as Response);
}

describe('LangDAGClient', () => {
  describe('constructor', () => {
    it('uses default base URL', () => {
      const fetchFn = mockFetch({ json: () => Promise.resolve({ status: 'ok' }) });
      const client = new LangDAGClient({ fetch: fetchFn });
      client.health();
      expect(fetchFn).toHaveBeenCalledWith(
        'http://localhost:8080/health',
        expect.any(Object)
      );
    });

    it('strips trailing slash from base URL', () => {
      const fetchFn = mockFetch({ json: () => Promise.resolve({ status: 'ok' }) });
      const client = new LangDAGClient({ baseUrl: 'http://example.com/', fetch: fetchFn });
      client.health();
      expect(fetchFn).toHaveBeenCalledWith(
        'http://example.com/health',
        expect.any(Object)
      );
    });
  });

  describe('health', () => {
    it('returns health status', async () => {
      const fetchFn = mockFetch({ json: () => Promise.resolve({ status: 'ok' }) });
      const client = new LangDAGClient({ fetch: fetchFn });
      const result = await client.health();
      expect(result.status).toBe('ok');
    });
  });

  describe('authentication headers', () => {
    it('sends X-API-Key header', async () => {
      const fetchFn = mockFetch({ json: () => Promise.resolve({ status: 'ok' }) });
      const client = new LangDAGClient({ apiKey: 'my-key', fetch: fetchFn });
      await client.health();
      expect(fetchFn).toHaveBeenCalledWith(
        expect.any(String),
        expect.objectContaining({
          headers: expect.objectContaining({ 'X-API-Key': 'my-key' }),
        })
      );
    });

    it('sends Bearer token header', async () => {
      const fetchFn = mockFetch({ json: () => Promise.resolve({ status: 'ok' }) });
      const client = new LangDAGClient({ bearerToken: 'my-token', fetch: fetchFn });
      await client.health();
      expect(fetchFn).toHaveBeenCalledWith(
        expect.any(String),
        expect.objectContaining({
          headers: expect.objectContaining({ 'Authorization': 'Bearer my-token' }),
        })
      );
    });
  });

  describe('error handling', () => {
    it('throws UnauthorizedError on 401', async () => {
      const fetchFn = mockFetch({
        ok: false,
        status: 401,
        statusText: 'Unauthorized',
        json: () => Promise.resolve({ error: 'unauthorized' }),
      });
      const client = new LangDAGClient({ fetch: fetchFn });
      await expect(client.health()).rejects.toThrow(UnauthorizedError);
    });

    it('throws NotFoundError on 404', async () => {
      const fetchFn = mockFetch({
        ok: false,
        status: 404,
        statusText: 'Not Found',
        json: () => Promise.resolve({ error: 'not found' }),
      });
      const client = new LangDAGClient({ fetch: fetchFn });
      await expect(client.getNode('nonexistent')).rejects.toThrow(NotFoundError);
    });

    it('throws BadRequestError on 400', async () => {
      const fetchFn = mockFetch({
        ok: false,
        status: 400,
        statusText: 'Bad Request',
        json: () => Promise.resolve({ error: 'invalid' }),
      });
      const client = new LangDAGClient({ fetch: fetchFn });
      await expect(client.prompt('')).rejects.toThrow(BadRequestError);
    });

    it('throws ApiError on 500', async () => {
      const fetchFn = mockFetch({
        ok: false,
        status: 500,
        statusText: 'Internal Server Error',
        json: () => Promise.resolve({ error: 'server error' }),
      });
      const client = new LangDAGClient({ fetch: fetchFn });
      await expect(client.health()).rejects.toThrow(ApiError);
    });

    it('throws NetworkError on connection failure', async () => {
      const fetchFn = vi.fn().mockRejectedValue(new Error('ECONNREFUSED'));
      const client = new LangDAGClient({ fetch: fetchFn });
      await expect(client.health()).rejects.toThrow(NetworkError);
    });
  });

  describe('listRoots', () => {
    it('returns list of root nodes', async () => {
      const nodes = [
        { id: 'n-1', sequence: 0, node_type: 'user', content: 'Hello', created_at: '2024-01-01' },
        { id: 'n-2', sequence: 0, node_type: 'user', content: 'Hi', created_at: '2024-01-02' },
      ];
      const fetchFn = mockFetch({ json: () => Promise.resolve(nodes) });
      const client = new LangDAGClient({ fetch: fetchFn });
      const result = await client.listRoots();
      expect(result).toHaveLength(2);
      expect(result[0]).toBeInstanceOf(Node);
      expect(result[0].id).toBe('n-1');
    });
  });

  describe('getNode', () => {
    it('returns a Node with prompt methods', async () => {
      const node = {
        id: 'n-1',
        sequence: 0,
        node_type: 'user',
        content: 'Hello',
        title: 'My Chat',
        created_at: '2024-01-01',
      };
      const fetchFn = mockFetch({ json: () => Promise.resolve(node) });
      const client = new LangDAGClient({ fetch: fetchFn });
      const result = await client.getNode('n-1');
      expect(result).toBeInstanceOf(Node);
      expect(result.id).toBe('n-1');
      expect(result.title).toBe('My Chat');
      expect(typeof result.prompt).toBe('function');
      expect(typeof result.promptStream).toBe('function');
    });
  });

  describe('getTree', () => {
    it('returns list of Node objects in tree', async () => {
      const nodes = [
        { id: 'n-1', sequence: 0, node_type: 'user', content: 'Hello', created_at: '2024-01-01' },
        { id: 'n-2', parent_id: 'n-1', sequence: 1, node_type: 'assistant', content: 'Hi!', created_at: '2024-01-01' },
      ];
      const fetchFn = mockFetch({ json: () => Promise.resolve(nodes) });
      const client = new LangDAGClient({ fetch: fetchFn });
      const result = await client.getTree('n-1');
      expect(result).toHaveLength(2);
      expect(result[0]).toBeInstanceOf(Node);
      expect(result[1].parentId).toBe('n-1');
    });
  });

  describe('prompt', () => {
    it('returns a Node', async () => {
      const promptResp = { node_id: 'n-1', content: 'Hello back!' };
      const fetchFn = mockFetch({ json: () => Promise.resolve(promptResp) });
      const client = new LangDAGClient({ fetch: fetchFn });
      const result = await client.prompt('Hello');
      expect(result).toBeInstanceOf(Node);
      expect(result.id).toBe('n-1');
      expect(result.content).toBe('Hello back!');
    });

    it('sends correct request body', async () => {
      const fetchFn = mockFetch({
        json: () => Promise.resolve({ node_id: 'n', content: 'ok' }),
      });
      const client = new LangDAGClient({ fetch: fetchFn });
      await client.prompt('Hello', { model: 'test-model', systemPrompt: 'Be nice' });
      expect(fetchFn).toHaveBeenCalledWith(
        'http://localhost:8080/prompt',
        expect.objectContaining({
          method: 'POST',
          body: expect.stringContaining('"message":"Hello"'),
        })
      );
    });
  });

  describe('node.prompt (promptFrom)', () => {
    it('returns a new Node', async () => {
      // First call returns the initial node
      const fetchFn = vi.fn()
        .mockResolvedValueOnce({
          ok: true, status: 200, statusText: 'OK',
          json: () => Promise.resolve({ node_id: 'n-1', content: 'First' }),
          headers: new Headers(),
        } as Response)
        .mockResolvedValueOnce({
          ok: true, status: 200, statusText: 'OK',
          json: () => Promise.resolve({ node_id: 'n-2', content: 'Continued!' }),
          headers: new Headers(),
        } as Response);

      const client = new LangDAGClient({ fetch: fetchFn });
      const node1 = await client.prompt('Hello');
      const node2 = await node1.prompt('Follow up');

      expect(node2).toBeInstanceOf(Node);
      expect(node2.content).toBe('Continued!');
    });

    it('sends to correct URL', async () => {
      const fetchFn = vi.fn()
        .mockResolvedValueOnce({
          ok: true, status: 200, statusText: 'OK',
          json: () => Promise.resolve({ node_id: 'n-1', content: 'First' }),
          headers: new Headers(),
        } as Response)
        .mockResolvedValueOnce({
          ok: true, status: 200, statusText: 'OK',
          json: () => Promise.resolve({ node_id: 'n-2', content: 'ok' }),
          headers: new Headers(),
        } as Response);

      const client = new LangDAGClient({ fetch: fetchFn });
      const node1 = await client.prompt('Hello');
      await node1.prompt('Follow up');

      expect(fetchFn).toHaveBeenLastCalledWith(
        'http://localhost:8080/nodes/n-1/prompt',
        expect.objectContaining({
          method: 'POST',
        })
      );
    });
  });

  describe('promptStream', () => {
    function createSSEStream(text: string): ReadableStream<Uint8Array> {
      const encoder = new TextEncoder();
      return new ReadableStream({
        start(controller) {
          controller.enqueue(encoder.encode(text));
          controller.close();
        },
      });
    }

    it('returns a Stream with events()', async () => {
      const sseText = [
        'event: start',
        'data: {}',
        '',
        'event: delta',
        'data: {"content":"Hello "}',
        '',
        'event: delta',
        'data: {"content":"world!"}',
        '',
        'event: done',
        'data: {"node_id":"n-1"}',
        '',
      ].join('\n');

      const body = createSSEStream(sseText);
      const fetchFn = mockFetch({ body });

      const client = new LangDAGClient({ fetch: fetchFn });
      const stream = await client.promptStream('Hello');

      expect(stream).toBeInstanceOf(Stream);

      const events = [];
      for await (const event of stream.events()) {
        events.push(event);
      }

      expect(events).toHaveLength(4);
      expect(events[0]).toEqual({ type: 'start' });
      expect(events[1]).toEqual({ type: 'delta', content: 'Hello ' });
      expect(events[2]).toEqual({ type: 'delta', content: 'world!' });
      expect(events[3]).toEqual({ type: 'done', node_id: 'n-1' });
    });

    it('stream.node() returns final Node', async () => {
      const sseText = [
        'event: start',
        'data: {}',
        '',
        'event: delta',
        'data: {"content":"Hello "}',
        '',
        'event: delta',
        'data: {"content":"world!"}',
        '',
        'event: done',
        'data: {"node_id":"n-1"}',
        '',
      ].join('\n');

      const body = createSSEStream(sseText);
      const fetchFn = mockFetch({ body });

      const client = new LangDAGClient({ fetch: fetchFn });
      const stream = await client.promptStream('Hello');

      // Consume events first
      for await (const _ of stream.events()) { /* drain */ }

      const node = await stream.node();
      expect(node).toBeInstanceOf(Node);
      expect(node.id).toBe('n-1');
      expect(node.content).toBe('Hello world!');
    });

    it('stream.node() auto-consumes if not iterated', async () => {
      const sseText = [
        'event: start',
        'data: {}',
        '',
        'event: delta',
        'data: {"content":"auto"}',
        '',
        'event: done',
        'data: {"node_id":"n-auto"}',
        '',
      ].join('\n');

      const body = createSSEStream(sseText);
      const fetchFn = mockFetch({ body });

      const client = new LangDAGClient({ fetch: fetchFn });
      const stream = await client.promptStream('Hello');

      // Call node() directly without iterating events()
      const node = await stream.node();
      expect(node.id).toBe('n-auto');
      expect(node.content).toBe('auto');
    });
  });

  describe('deleteNode', () => {
    it('deletes without error', async () => {
      const fetchFn = mockFetch({
        json: () => Promise.resolve({ status: 'deleted', id: 'n-1' }),
      });
      const client = new LangDAGClient({ fetch: fetchFn });
      await expect(client.deleteNode('n-1')).resolves.toBeUndefined();
    });
  });
});
