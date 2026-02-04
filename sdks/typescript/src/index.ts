/**
 * LangDAG TypeScript SDK
 *
 * A TypeScript client for the LangDAG REST API, enabling management of
 * LLM conversations as directed acyclic graphs (DAGs).
 *
 * @packageDocumentation
 */

// Main client
export { LangDAGClient } from './client.js';

// Types
export type {
  // Client configuration
  LangDAGClientOptions,
  ChatOptions,
  ContinueChatOptions,
  ForkChatOptions,
  CreateWorkflowOptions,
  RunWorkflowOptions,

  // Core models
  DAG,
  DAGDetail,
  Node,
  DAGStatus,
  NodeType,

  // Chat types
  NewChatRequest,
  ContinueChatRequest,
  ForkChatRequest,
  ChatResponse,

  // SSE types
  SSEEvent,
  SSEStartEvent,
  SSEDeltaEvent,
  SSEDoneEvent,
  SSEErrorEvent,

  // Workflow types
  Workflow,
  WorkflowDefaults,
  ToolDefinition,
  WorkflowNode,
  WorkflowNodeType,
  WorkflowEdge,
  RunWorkflowRequest,
  RunWorkflowResponse,
  RunStatus,

  // Response types
  DeleteResponse,
  HealthResponse,
  ApiError as ApiErrorResponse,
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
  collectStreamContent,
} from './sse.js';
