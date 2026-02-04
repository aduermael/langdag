/**
 * SSE (Server-Sent Events) parsing utilities
 */

import type { SSEEvent } from './types.js';
import { SSEParseError } from './errors.js';

/**
 * Parse a single SSE event from raw text
 */
export function parseSSEEvent(eventType: string, data: string): SSEEvent {
  switch (eventType) {
    case 'start': {
      try {
        const parsed = JSON.parse(data) as { dag_id: string };
        return { type: 'start', dag_id: parsed.dag_id };
      } catch {
        throw new SSEParseError(`Failed to parse 'start' event data`, data);
      }
    }

    case 'delta': {
      try {
        const parsed = JSON.parse(data) as { content: string };
        return { type: 'delta', content: parsed.content };
      } catch {
        throw new SSEParseError(`Failed to parse 'delta' event data`, data);
      }
    }

    case 'done': {
      try {
        const parsed = JSON.parse(data) as { node_id: string; dag_id: string };
        return { type: 'done', node_id: parsed.node_id, dag_id: parsed.dag_id };
      } catch {
        throw new SSEParseError(`Failed to parse 'done' event data`, data);
      }
    }

    case 'error': {
      // Error data may be plain text or JSON
      return { type: 'error', error: data };
    }

    default:
      throw new SSEParseError(`Unknown SSE event type: ${eventType}`, data);
  }
}

/**
 * Async generator that parses SSE events from a ReadableStream
 */
export async function* parseSSEStream(
  stream: ReadableStream<Uint8Array>
): AsyncGenerator<SSEEvent, void, undefined> {
  const reader = stream.getReader();
  const decoder = new TextDecoder();
  let buffer = '';

  try {
    while (true) {
      const { done, value } = await reader.read();

      if (done) {
        // Process any remaining buffer content
        if (buffer.trim()) {
          const event = parseEventBlock(buffer);
          if (event) {
            yield event;
          }
        }
        break;
      }

      buffer += decoder.decode(value, { stream: true });

      // SSE events are separated by double newlines
      const parts = buffer.split('\n\n');

      // Keep the last part in the buffer (may be incomplete)
      buffer = parts.pop() || '';

      // Process complete events
      for (const part of parts) {
        if (part.trim()) {
          const event = parseEventBlock(part);
          if (event) {
            yield event;
          }
        }
      }
    }
  } finally {
    reader.releaseLock();
  }
}

/**
 * Parse a single SSE event block (event + data lines)
 */
function parseEventBlock(block: string): SSEEvent | null {
  const lines = block.split('\n');
  let eventType = '';
  let data = '';

  for (const line of lines) {
    if (line.startsWith('event:')) {
      eventType = line.slice(6).trim();
    } else if (line.startsWith('data:')) {
      // Accumulate data lines (some events may have multi-line data)
      if (data) {
        data += '\n';
      }
      data += line.slice(5).trim();
    }
    // Ignore other lines (comments, id, retry, etc.)
  }

  if (!eventType || !data) {
    return null;
  }

  return parseSSEEvent(eventType, data);
}

/**
 * Collect all content from a streaming response
 * Useful for when you want streaming internally but a simple string result
 */
export async function collectStreamContent(
  stream: AsyncGenerator<SSEEvent, void, undefined>
): Promise<{ dagId: string; nodeId: string; content: string }> {
  let dagId = '';
  let nodeId = '';
  let content = '';

  for await (const event of stream) {
    switch (event.type) {
      case 'start':
        dagId = event.dag_id;
        break;
      case 'delta':
        content += event.content;
        break;
      case 'done':
        nodeId = event.node_id;
        dagId = event.dag_id;
        break;
      case 'error':
        throw new SSEParseError(`Stream error: ${event.error}`);
    }
  }

  return { dagId, nodeId, content };
}
