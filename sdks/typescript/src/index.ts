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
  CreateWorkflowOptions,
  RunWorkflowOptions,

  // Core models
  NodeData,
  NodeType,

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
