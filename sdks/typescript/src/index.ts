/**
 * LangDAG TypeScript SDK
 *
 * A TypeScript client for the LangDAG REST API, enabling management of
 * LLM conversations as node trees.
 *
 * @packageDocumentation
 */

// Main client and core classes
export { LangDAGClient, Node, Stream } from './client.js';

// Types
export type {
  // Client configuration
  LangDAGClientOptions,
  PromptOptions,
  ToolDefinition,

  // Core models
  NodeData,
  NodeType,

  // SSE types
  SSEEvent,
  SSEStartEvent,
  SSEDeltaEvent,
  SSEDoneEvent,
  SSEErrorEvent,

  // Response types
  DeleteResponse,
  HealthResponse,
  ApiErrorBody,
} from './types.js';

// Errors
export {
  LangDAGError,
  ApiError,
  UnauthorizedError,
  NotFoundError,
  BadRequestError,
  SSEParseError,
  NetworkError,
  createApiError,
} from './errors.js';

// SSE utilities
export {
  parseSSEEvent,
  parseSSEStream,
} from './sse.js';
