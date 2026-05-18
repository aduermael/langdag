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
  provider?: string;
  model?: string;
  tokens_in?: number;
  tokens_out?: number;
  tokens_cache_read?: number;
  tokens_cache_creation?: number;
  tokens_reasoning?: number;
  usage?: NormalizedUsage;
  latency_ms?: number;
  stop_reason?: string;
  status?: string;
  title?: string;
  system_prompt?: string;
  created_at: string;
  metadata?: AssistantNodeMetadata;
  cost?: CostResult;
}

export interface NormalizedUsage {
  input_tokens?: number;
  output_tokens?: number;
  cache_read_input_tokens?: number;
  cache_creation_input_tokens?: number;
  cache_write_input_tokens?: number;
  reasoning_tokens?: number;
  tool_use_prompt_tokens?: number;
  audio_input_tokens?: number;
  audio_output_tokens?: number;
  image_input_tokens?: number;
  image_output_tokens?: number;
  accepted_prediction_tokens?: number;
  rejected_prediction_tokens?: number;
  service_tier?: string;
  dimensions?: Record<string, number>;
}

export interface ModelResolutionMetadata {
  canonical_model_id: string;
  offering_id: string;
  deployment_id: string;
  provider_id: string;
  api_protocol_id: string;
  native_model_id: string;
}

export interface PricingSnapshot {
  status: 'known' | 'partial' | 'unknown' | 'free' | string;
  currency?: string;
  effective_at?: string;
  source?: string;
  rates_per_1m?: Record<string, number>;
  missing_dimensions?: string[];
}

export interface ProviderCost {
  total: number;
  currency: string;
  source: string;
  raw?: unknown;
}

export interface CostDimension {
  name: string;
  quantity: number;
  rate_per_1m: number;
  cost: number;
}

export interface CostResult {
  status: 'known' | 'partial' | 'unknown' | 'free' | string;
  total?: number;
  currency?: string;
  source?: string;
  missing_dimensions?: string[];
  dimensions?: CostDimension[];
}

export interface AssistantNodeMetadata {
  model_resolution?: ModelResolutionMetadata;
  normalized_usage?: NormalizedUsage;
  pricing_snapshot?: PricingSnapshot;
  provider_cost?: ProviderCost;
}

// ============================================================================
// Tool Types
// ============================================================================

/**
 * Definition of a tool that can be used by the LLM
 */
export interface ToolDefinition {
  name: string;
  description: string;
  input_schema: Record<string, unknown>;
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
  tools?: ToolDefinition[];
}

export interface PromptResponse {
  node_id: string;
  content: string;
  tokens_in?: number;
  tokens_out?: number;
  tokens_cache_read?: number;
  tokens_cache_creation?: number;
  tokens_reasoning?: number;
  usage?: NormalizedUsage;
  metadata?: AssistantNodeMetadata;
  cost?: CostResult;
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
  response?: PromptResponse;
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
