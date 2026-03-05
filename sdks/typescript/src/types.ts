/**
 * LangDAG TypeScript SDK Types
 */

// ============================================================================
// Enums
// ============================================================================

export type NodeType = 'user' | 'assistant' | 'tool_call' | 'tool_result';

// ============================================================================
// Core Models
// ============================================================================

/**
 * Error response from the API
 */
export interface ApiErrorBody {
  error: string;
}

/**
 * Raw node data from the API
 */
export interface NodeData {
  id: string;
  parent_id?: string;
  root_id?: string;
  sequence: number;
  node_type: NodeType;
  content: string;
  model?: string;
  tokens_in?: number;
  tokens_out?: number;
  tokens_cache_read?: number;
  tokens_cache_creation?: number;
  tokens_reasoning?: number;
  latency_ms?: number;
  status?: string;
  title?: string;
  system_prompt?: string;
  created_at: string;
}

// ============================================================================
// Prompt Types
// ============================================================================

/**
 * Options for starting a new conversation (client.prompt / client.promptStream)
 */
export interface PromptOptions {
  model?: string;
  systemPrompt?: string;
}

// ============================================================================
// SSE Event Types
// ============================================================================

/**
 * SSE event emitted when streaming starts
 */
export interface SSEStartEvent {
  type: 'start';
}

/**
 * SSE event emitted for content deltas
 */
export interface SSEDeltaEvent {
  type: 'delta';
  content: string;
}

/**
 * SSE event emitted when streaming completes
 */
export interface SSEDoneEvent {
  type: 'done';
  node_id: string;
}

/**
 * SSE event emitted on error
 */
export interface SSEErrorEvent {
  type: 'error';
  error: string;
}

/**
 * Union type for all SSE events
 */
export type SSEEvent = SSEStartEvent | SSEDeltaEvent | SSEDoneEvent | SSEErrorEvent;

// ============================================================================
// Client Configuration Types
// ============================================================================

/**
 * Options for initializing the LangDAG client
 */
export interface LangDAGClientOptions {
  /**
   * Base URL of the LangDAG API server
   * @default "http://localhost:8080"
   */
  baseUrl?: string;

  /**
   * API key for authentication (sent as X-API-Key header)
   */
  apiKey?: string;

  /**
   * Bearer token for authentication (sent as Authorization header)
   */
  bearerToken?: string;

  /**
   * Custom fetch function (defaults to global fetch)
   */
  fetch?: typeof fetch;
}

/**
 * Delete response
 */
export interface DeleteResponse {
  status: string;
  id: string;
}

/**
 * Health check response
 */
export interface HealthResponse {
  status: string;
}
