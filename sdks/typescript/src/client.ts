/**
 * LangDAG Client
 * Main client class for interacting with the LangDAG REST API
 */

import type {
  LangDAGClientOptions,
  NodeData,
  NodeType,
  PromptOptions,
  SSEEvent,
  Workflow,
  CreateWorkflowOptions,
  RunWorkflowOptions,
  RunWorkflowResponse,
  DeleteResponse,
  HealthResponse,
} from './types.js';

import { createApiError, NetworkError, SSEParseError } from './errors.js';
import { parseSSEStream } from './sse.js';

const DEFAULT_BASE_URL = 'http://localhost:8080';

/**
 * A node in a conversation tree.
 * Returned by client.prompt(), node.prompt(), stream.node(), etc.
 * Call node.prompt() or node.promptStream() to continue the conversation.
 */
export class Node {
  readonly id: string;
  readonly parentId?: string;
  readonly sequence: number;
  readonly type: NodeType;
  readonly content: string;
  readonly model?: string;
  readonly tokensIn?: number;
  readonly tokensOut?: number;
  readonly latencyMs?: number;
  readonly status?: string;
  readonly title?: string;
  readonly systemPrompt?: string;
  readonly createdAt: string;

  /** @internal */
  private readonly client: LangDAGClient;

  /** @internal */
  constructor(data: NodeData, client: LangDAGClient) {
    this.id = data.id;
    this.parentId = data.parent_id;
    this.sequence = data.sequence;
    this.type = data.node_type;
    this.content = data.content;
    this.model = data.model;
    this.tokensIn = data.tokens_in;
    this.tokensOut = data.tokens_out;
    this.latencyMs = data.latency_ms;
    this.status = data.status;
    this.title = data.title;
    this.systemPrompt = data.system_prompt;
    this.createdAt = data.created_at;
    this.client = client;
  }

  /**
   * Continue the conversation from this node (non-streaming).
   * @param message - The message to send
   * @param options - Optional model override
   * @returns The assistant's response node
   */
  async prompt(message: string, options?: { model?: string }): Promise<Node> {
    return this.client.promptFrom(this.id, message, options);
  }

  /**
   * Continue the conversation from this node (streaming).
   * @param message - The message to send
   * @param options - Optional model override
   * @returns A Stream for consuming SSE events and getting the final node
   */
  async promptStream(message: string, options?: { model?: string }): Promise<Stream> {
    return this.client.promptStreamFrom(this.id, message, options);
  }
}

/**
 * A streaming response. Consume events via the async iterator,
 * then call node() to get the final Node.
 */
export class Stream {
  private readonly rawStream: ReadableStream<Uint8Array>;
  private readonly client: LangDAGClient;
  private nodeId: string = '';
  private collectedContent: string = '';
  private consumed: boolean = false;

  /** @internal */
  constructor(rawStream: ReadableStream<Uint8Array>, client: LangDAGClient) {
    this.rawStream = rawStream;
    this.client = client;
  }

  /**
   * Async iterator over SSE events.
   * Must be consumed before calling node().
   */
  async *events(): AsyncGenerator<SSEEvent, void, undefined> {
    if (this.consumed) {
      throw new SSEParseError('Stream has already been consumed');
    }
    this.consumed = true;

    for await (const event of parseSSEStream(this.rawStream)) {
      if (event.type === 'delta') {
        this.collectedContent += event.content;
      } else if (event.type === 'done') {
        this.nodeId = event.node_id;
      }
      yield event;
    }
  }

  /**
   * Get the final Node after the stream has been consumed.
   * The stream must be fully consumed via events() first.
   */
  async node(): Promise<Node> {
    if (!this.consumed) {
      // Auto-consume the stream
      for await (const event of this.events()) {
        // consume
        void event;
      }
    }

    if (!this.nodeId) {
      throw new SSEParseError('Stream did not produce a done event with node_id');
    }

    return new Node(
      {
        id: this.nodeId,
        content: this.collectedContent,
        node_type: 'assistant',
        sequence: 0,
        created_at: '',
      },
      this.client,
    );
  }
}

/**
 * LangDAG API Client
 *
 * @example
 * ```typescript
 * const client = new LangDAGClient({ baseUrl: 'http://localhost:8080' });
 *
 * // Start a new conversation
 * const node = await client.prompt('Hello!');
 * console.log(node.content);
 *
 * // Continue from any node
 * const node2 = await node.prompt('Tell me more');
 *
 * // Streaming
 * const stream = await client.promptStream('Hello!');
 * for await (const event of stream.events()) {
 *   if (event.type === 'delta') process.stdout.write(event.content);
 * }
 * const result = await stream.node();
 * ```
 */
export class LangDAGClient {
  private readonly baseUrl: string;
  private readonly apiKey?: string;
  private readonly bearerToken?: string;
  private readonly fetchFn: typeof fetch;

  constructor(options: LangDAGClientOptions = {}) {
    this.baseUrl = (options.baseUrl || DEFAULT_BASE_URL).replace(/\/$/, '');
    this.apiKey = options.apiKey;
    this.bearerToken = options.bearerToken;
    this.fetchFn = options.fetch || fetch;
  }

  // ===========================================================================
  // Private helpers
  // ===========================================================================

  private getHeaders(contentType?: string): Record<string, string> {
    const headers: Record<string, string> = {};

    if (contentType) {
      headers['Content-Type'] = contentType;
    }

    if (this.apiKey) {
      headers['X-API-Key'] = this.apiKey;
    } else if (this.bearerToken) {
      headers['Authorization'] = `Bearer ${this.bearerToken}`;
    }

    return headers;
  }

  private async request<T>(
    method: string,
    path: string,
    body?: unknown
  ): Promise<T> {
    const url = `${this.baseUrl}${path}`;
    const headers = this.getHeaders(body ? 'application/json' : undefined);

    let response: Response;
    try {
      response = await this.fetchFn(url, {
        method,
        headers,
        body: body ? JSON.stringify(body) : undefined,
      });
    } catch (error) {
      throw new NetworkError(
        `Failed to connect to ${url}`,
        error instanceof Error ? error : undefined
      );
    }

    if (!response.ok) {
      let errorBody: unknown;
      try {
        errorBody = await response.json();
      } catch {
        // Ignore JSON parse errors for error responses
      }
      throw createApiError(response.status, response.statusText, errorBody);
    }

    return response.json() as Promise<T>;
  }

  private async requestStream(
    method: string,
    path: string,
    body?: unknown
  ): Promise<ReadableStream<Uint8Array>> {
    const url = `${this.baseUrl}${path}`;
    const headers = this.getHeaders(body ? 'application/json' : undefined);

    let response: Response;
    try {
      response = await this.fetchFn(url, {
        method,
        headers,
        body: body ? JSON.stringify(body) : undefined,
      });
    } catch (error) {
      throw new NetworkError(
        `Failed to connect to ${url}`,
        error instanceof Error ? error : undefined
      );
    }

    if (!response.ok) {
      let errorBody: unknown;
      try {
        errorBody = await response.json();
      } catch {
        // Ignore JSON parse errors for error responses
      }
      throw createApiError(response.status, response.statusText, errorBody);
    }

    if (!response.body) {
      throw new NetworkError('Response body is null');
    }

    return response.body;
  }

  // ===========================================================================
  // Health
  // ===========================================================================

  async health(): Promise<HealthResponse> {
    return this.request<HealthResponse>('GET', '/health');
  }

  // ===========================================================================
  // Prompt Methods
  // ===========================================================================

  /**
   * Start a new conversation (non-streaming).
   * @param message - The message to send
   * @param options - Optional model and system prompt
   * @returns The assistant's response node
   */
  async prompt(message: string, options?: PromptOptions): Promise<Node> {
    const resp = await this.request<{ node_id: string; content: string }>('POST', '/prompt', {
      message,
      model: options?.model,
      system_prompt: options?.systemPrompt,
      stream: false,
    });

    return new Node(
      {
        id: resp.node_id,
        content: resp.content,
        node_type: 'assistant',
        sequence: 0,
        created_at: '',
      },
      this,
    );
  }

  /**
   * Start a new conversation (streaming).
   * @param message - The message to send
   * @param options - Optional model and system prompt
   * @returns A Stream for consuming SSE events and getting the final node
   */
  async promptStream(message: string, options?: PromptOptions): Promise<Stream> {
    const rawStream = await this.requestStream('POST', '/prompt', {
      message,
      model: options?.model,
      system_prompt: options?.systemPrompt,
      stream: true,
    });

    return new Stream(rawStream, this);
  }

  /**
   * Continue a conversation from an existing node (non-streaming).
   * Prefer using node.prompt() instead.
   * @internal
   */
  async promptFrom(nodeId: string, message: string, options?: { model?: string }): Promise<Node> {
    const resp = await this.request<{ node_id: string; content: string }>(
      'POST',
      `/nodes/${encodeURIComponent(nodeId)}/prompt`,
      {
        message,
        model: options?.model,
        stream: false,
      },
    );

    return new Node(
      {
        id: resp.node_id,
        content: resp.content,
        node_type: 'assistant',
        sequence: 0,
        created_at: '',
      },
      this,
    );
  }

  /**
   * Continue a conversation from an existing node (streaming).
   * Prefer using node.promptStream() instead.
   * @internal
   */
  async promptStreamFrom(nodeId: string, message: string, options?: { model?: string }): Promise<Stream> {
    const rawStream = await this.requestStream(
      'POST',
      `/nodes/${encodeURIComponent(nodeId)}/prompt`,
      {
        message,
        model: options?.model,
        stream: true,
      },
    );

    return new Stream(rawStream, this);
  }

  // ===========================================================================
  // Node Methods
  // ===========================================================================

  /**
   * List all root nodes (conversation roots)
   */
  async listRoots(): Promise<Node[]> {
    const data = await this.request<NodeData[]>('GET', '/nodes');
    return data.map(d => new Node(d, this));
  }

  /**
   * Get a single node by ID
   */
  async getNode(id: string): Promise<Node> {
    const data = await this.request<NodeData>('GET', `/nodes/${encodeURIComponent(id)}`);
    return new Node(data, this);
  }

  /**
   * Get the full tree of nodes for a conversation
   */
  async getTree(id: string): Promise<Node[]> {
    const data = await this.request<NodeData[]>('GET', `/nodes/${encodeURIComponent(id)}/tree`);
    return data.map(d => new Node(d, this));
  }

  /**
   * Delete a node and its descendants
   */
  async deleteNode(id: string): Promise<void> {
    await this.request<DeleteResponse>('DELETE', `/nodes/${encodeURIComponent(id)}`);
  }

  // ===========================================================================
  // Workflow Methods
  // ===========================================================================

  async listWorkflows(): Promise<Workflow[]> {
    return this.request<Workflow[]>('GET', '/workflows');
  }

  async createWorkflow(options: CreateWorkflowOptions): Promise<Workflow> {
    return this.request<Workflow>('POST', '/workflows', {
      name: options.name,
      description: options.description,
      defaults: options.defaults,
      tools: options.tools,
      nodes: options.nodes,
      edges: options.edges,
    });
  }

  runWorkflow(workflowId: string, options: RunWorkflowOptions & { stream: true }): AsyncGenerator<SSEEvent, void, undefined>;
  runWorkflow(workflowId: string, options?: RunWorkflowOptions & { stream?: false }): Promise<RunWorkflowResponse>;
  runWorkflow(workflowId: string, options?: RunWorkflowOptions): Promise<RunWorkflowResponse> | AsyncGenerator<SSEEvent, void, undefined>;
  runWorkflow(workflowId: string, options: RunWorkflowOptions = {}): Promise<RunWorkflowResponse> | AsyncGenerator<SSEEvent, void, undefined> {
    if (options.stream) {
      return this.runWorkflowStream(workflowId, options);
    }
    return this.request<RunWorkflowResponse>('POST', `/workflows/${encodeURIComponent(workflowId)}/run`, {
      input: options.input,
      stream: false,
    });
  }

  private async *runWorkflowStream(workflowId: string, options: RunWorkflowOptions): AsyncGenerator<SSEEvent, void, undefined> {
    const stream = await this.requestStream('POST', `/workflows/${encodeURIComponent(workflowId)}/run`, {
      input: options.input,
      stream: true,
    });
    yield* parseSSEStream(stream);
  }
}
