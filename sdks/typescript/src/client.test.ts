import { describe, it, expect, vi } from 'vitest';
import { LangDAGClient } from './client.js';
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
      await expect(client.getDag('nonexistent')).rejects.toThrow(NotFoundError);
    });

    it('throws BadRequestError on 400', async () => {
      const fetchFn = mockFetch({
        ok: false,
        status: 400,
        statusText: 'Bad Request',
        json: () => Promise.resolve({ error: 'invalid' }),
      });
      const client = new LangDAGClient({ fetch: fetchFn });
      await expect(client.chat({ message: '' })).rejects.toThrow(BadRequestError);
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

  describe('listDags', () => {
    it('returns list of DAGs', async () => {
      const dags = [
        { id: 'dag-1', status: 'completed', created_at: '2024-01-01', updated_at: '2024-01-01' },
        { id: 'dag-2', status: 'running', created_at: '2024-01-02', updated_at: '2024-01-02' },
      ];
      const fetchFn = mockFetch({ json: () => Promise.resolve(dags) });
      const client = new LangDAGClient({ fetch: fetchFn });
      const result = await client.listDags();
      expect(result).toHaveLength(2);
      expect(result[0].id).toBe('dag-1');
    });
  });

  describe('getDag', () => {
    it('returns DAG detail', async () => {
      const dag = {
        id: 'dag-1',
        status: 'completed',
        created_at: '2024-01-01',
        updated_at: '2024-01-01',
        nodes: [{ id: 'n1', sequence: 0, node_type: 'user', content: 'Hello', created_at: '2024-01-01' }],
      };
      const fetchFn = mockFetch({ json: () => Promise.resolve(dag) });
      const client = new LangDAGClient({ fetch: fetchFn });
      const result = await client.getDag('dag-1');
      expect(result.id).toBe('dag-1');
      expect(result.nodes).toHaveLength(1);
    });
  });

  describe('chat', () => {
    it('non-streaming returns ChatResponse', async () => {
      const chatResp = { dag_id: 'dag-1', node_id: 'n-1', content: 'Hello back!' };
      const fetchFn = mockFetch({ json: () => Promise.resolve(chatResp) });
      const client = new LangDAGClient({ fetch: fetchFn });
      const result = await client.chat({ message: 'Hello' });
      expect(result.dag_id).toBe('dag-1');
      expect(result.content).toBe('Hello back!');
    });

    it('sends correct request body', async () => {
      const fetchFn = mockFetch({
        json: () => Promise.resolve({ dag_id: 'd', node_id: 'n', content: 'ok' }),
      });
      const client = new LangDAGClient({ fetch: fetchFn });
      await client.chat({ message: 'Hello', model: 'test-model', system_prompt: 'Be nice' });
      expect(fetchFn).toHaveBeenCalledWith(
        'http://localhost:8080/chat',
        expect.objectContaining({
          method: 'POST',
          body: expect.stringContaining('"message":"Hello"'),
        })
      );
    });
  });

  describe('continueChat', () => {
    it('returns ChatResponse', async () => {
      const chatResp = { dag_id: 'dag-1', node_id: 'n-2', content: 'Continued!' };
      const fetchFn = mockFetch({ json: () => Promise.resolve(chatResp) });
      const client = new LangDAGClient({ fetch: fetchFn });
      const result = await client.continueChat('dag-1', { message: 'Follow up' });
      expect(result.content).toBe('Continued!');
    });
  });

  describe('deleteDag', () => {
    it('returns delete confirmation', async () => {
      const fetchFn = mockFetch({
        json: () => Promise.resolve({ status: 'deleted', id: 'dag-1' }),
      });
      const client = new LangDAGClient({ fetch: fetchFn });
      const result = await client.deleteDag('dag-1');
      expect(result.status).toBe('deleted');
    });
  });
});
