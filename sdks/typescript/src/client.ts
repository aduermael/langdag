/**
 * LangDAG Client
 * Main client class for interacting with the LangDAG REST API
 */

import type {
  LangDAGClientOptions,
  DAG,
  DAGDetail,
  ChatOptions,
  ContinueChatOptions,
  ForkChatOptions,
  ChatResponse,
  SSEEvent,
  Workflow,
  CreateWorkflowOptions,
  RunWorkflowOptions,
  RunWorkflowResponse,
  DeleteResponse,
  HealthResponse,
} from './types.js';

import { createApiError, NetworkError } from './errors.js';
import { parseSSEStream } from './sse.js';

const DEFAULT_BASE_URL = 'http://localhost:8080';

/**
 * LangDAG API Client
 *
 * @example
 * ```typescript
 * const client = new LangDAGClient({
 *   baseUrl: 'http://localhost:8080',
 *   apiKey: 'your-api-key'
 * });
 *
 * // Start a new chat
 * const response = await client.chat({ message: 'Hello!' });
 * console.log(response.content);
 *
 * // Stream a response
 * for await (const event of client.chat({ message: 'Hello!', stream: true })) {
 *   if (event.type === 'delta') {
 *     process.stdout.write(event.content);
 *   }
 * }
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

  /**
   * Check server health
   * @returns Health status
   */
  async health(): Promise<HealthResponse> {
    return this.request<HealthResponse>('GET', '/health');
  }

  // ===========================================================================
  // DAG Methods
  // ===========================================================================

  /**
   * List all DAGs
   * @returns Array of DAG instances
   */
  async listDags(): Promise<DAG[]> {
    return this.request<DAG[]>('GET', '/dags');
  }

  /**
   * Get a DAG by ID with full node details
   * @param id - DAG ID (full or prefix)
   * @returns DAG with nodes
   */
  async getDag(id: string): Promise<DAGDetail> {
    return this.request<DAGDetail>('GET', `/dags/${encodeURIComponent(id)}`);
  }

  /**
   * Delete a DAG
   * @param id - DAG ID (full or prefix)
   * @returns Delete confirmation
   */
  async deleteDag(id: string): Promise<DeleteResponse> {
    return this.request<DeleteResponse>('DELETE', `/dags/${encodeURIComponent(id)}`);
  }

  // ===========================================================================
  // Chat Methods
  // ===========================================================================

  /**
   * Start a new chat conversation
   * @param options - Chat options
   * @returns Chat response or async generator of SSE events
   */
  chat(options: ChatOptions & { stream: true }): AsyncGenerator<SSEEvent, void, undefined>;
  chat(options: ChatOptions & { stream?: false }): Promise<ChatResponse>;
  chat(options: ChatOptions): Promise<ChatResponse> | AsyncGenerator<SSEEvent, void, undefined>;
  chat(options: ChatOptions): Promise<ChatResponse> | AsyncGenerator<SSEEvent, void, undefined> {
    if (options.stream) {
      return this.chatStream(options);
    }
    return this.request<ChatResponse>('POST', '/chat', {
      message: options.message,
      model: options.model,
      system_prompt: options.system_prompt,
      stream: false,
    });
  }

  private async *chatStream(options: ChatOptions): AsyncGenerator<SSEEvent, void, undefined> {
    const stream = await this.requestStream('POST', '/chat', {
      message: options.message,
      model: options.model,
      system_prompt: options.system_prompt,
      stream: true,
    });
    yield* parseSSEStream(stream);
  }

  /**
   * Continue an existing chat conversation
   * @param dagId - DAG ID (full or prefix)
   * @param options - Continue chat options
   * @returns Chat response or async generator of SSE events
   */
  continueChat(dagId: string, options: ContinueChatOptions & { stream: true }): AsyncGenerator<SSEEvent, void, undefined>;
  continueChat(dagId: string, options: ContinueChatOptions & { stream?: false }): Promise<ChatResponse>;
  continueChat(dagId: string, options: ContinueChatOptions): Promise<ChatResponse> | AsyncGenerator<SSEEvent, void, undefined>;
  continueChat(dagId: string, options: ContinueChatOptions): Promise<ChatResponse> | AsyncGenerator<SSEEvent, void, undefined> {
    if (options.stream) {
      return this.continueChatStream(dagId, options);
    }
    return this.request<ChatResponse>('POST', `/chat/${encodeURIComponent(dagId)}`, {
      message: options.message,
      stream: false,
    });
  }

  private async *continueChatStream(dagId: string, options: ContinueChatOptions): AsyncGenerator<SSEEvent, void, undefined> {
    const stream = await this.requestStream('POST', `/chat/${encodeURIComponent(dagId)}`, {
      message: options.message,
      stream: true,
    });
    yield* parseSSEStream(stream);
  }

  /**
   * Fork a chat from a specific node
   * @param dagId - DAG ID (full or prefix)
   * @param options - Fork chat options
   * @returns Chat response or async generator of SSE events
   */
  forkChat(dagId: string, options: ForkChatOptions & { stream: true }): AsyncGenerator<SSEEvent, void, undefined>;
  forkChat(dagId: string, options: ForkChatOptions & { stream?: false }): Promise<ChatResponse>;
  forkChat(dagId: string, options: ForkChatOptions): Promise<ChatResponse> | AsyncGenerator<SSEEvent, void, undefined>;
  forkChat(dagId: string, options: ForkChatOptions): Promise<ChatResponse> | AsyncGenerator<SSEEvent, void, undefined> {
    if (options.stream) {
      return this.forkChatStream(dagId, options);
    }
    return this.request<ChatResponse>('POST', `/chat/${encodeURIComponent(dagId)}/fork`, {
      node_id: options.node_id,
      message: options.message,
      stream: false,
    });
  }

  private async *forkChatStream(dagId: string, options: ForkChatOptions): AsyncGenerator<SSEEvent, void, undefined> {
    const stream = await this.requestStream('POST', `/chat/${encodeURIComponent(dagId)}/fork`, {
      node_id: options.node_id,
      message: options.message,
      stream: true,
    });
    yield* parseSSEStream(stream);
  }

  // ===========================================================================
  // Workflow Methods
  // ===========================================================================

  /**
   * List all workflows
   * @returns Array of workflow templates
   */
  async listWorkflows(): Promise<Workflow[]> {
    return this.request<Workflow[]>('GET', '/workflows');
  }

  /**
   * Create a new workflow
   * @param options - Workflow definition
   * @returns Created workflow
   */
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

  /**
   * Run a workflow
   * @param workflowId - Workflow ID or name
   * @param options - Run options
   * @returns Run response or async generator of SSE events
   */
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
