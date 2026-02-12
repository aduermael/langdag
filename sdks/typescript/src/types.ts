/**
 * LangDAG TypeScript SDK Types
 */

// ============================================================================
// Enums
// ============================================================================

export type NodeType = 'user' | 'assistant' | 'tool_call' | 'tool_result' | 'llm' | 'input' | 'output';

export type WorkflowNodeType = 'llm' | 'tool' | 'branch' | 'merge' | 'input' | 'output';

export type RunStatus = 'pending' | 'running' | 'completed' | 'failed';

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
  sequence: number;
  node_type: NodeType;
  content: string;
  model?: string;
  tokens_in?: number;
  tokens_out?: number;
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
// Workflow Types
// ============================================================================

/**
 * Workflow template
 */
export interface Workflow {
  id: string;
  name: string;
  version: number;
  description?: string;
  created_at: string;
  updated_at: string;
}

/**
 * Default settings for workflow execution
 */
export interface WorkflowDefaults {
  provider?: string;
  model?: string;
  max_tokens?: number;
  temperature?: number;
}

/**
 * Tool definition for workflow
 */
export interface ToolDefinition {
  name: string;
  description: string;
  input_schema: Record<string, unknown>;
}

/**
 * Node definition within a workflow
 */
export interface WorkflowNode {
  id: string;
  type: WorkflowNodeType;
  content?: Record<string, unknown>;
  model?: string;
  system?: string;
  prompt?: string;
  tools?: string[];
  handler?: string;
  condition?: string;
}

/**
 * Edge definition within a workflow
 */
export interface WorkflowEdge {
  from: string;
  to: string;
  condition?: string;
  transform?: string;
}

/**
 * Request to create a new workflow
 */
export interface CreateWorkflowRequest {
  name: string;
  description?: string;
  defaults?: WorkflowDefaults;
  tools?: ToolDefinition[];
  nodes: WorkflowNode[];
  edges?: WorkflowEdge[];
}

/**
 * Request to run a workflow
 */
export interface RunWorkflowRequest {
  input?: Record<string, unknown>;
  stream?: boolean;
}

/**
 * Response from running a workflow
 */
export interface RunWorkflowResponse {
  dag_id: string;
  status: RunStatus;
  output?: Record<string, unknown>;
}

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
 * Options for creating a workflow
 */
export interface CreateWorkflowOptions {
  name: string;
  description?: string;
  defaults?: WorkflowDefaults;
  tools?: ToolDefinition[];
  nodes: WorkflowNode[];
  edges?: WorkflowEdge[];
}

/**
 * Options for running a workflow
 */
export interface RunWorkflowOptions {
  input?: Record<string, unknown>;
  stream?: boolean;
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
