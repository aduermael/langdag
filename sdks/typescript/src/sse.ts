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
      return { type: 'start' };
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
        const parsed = JSON.parse(data) as { node_id: string };
        return { type: 'done', node_id: parsed.node_id };
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

  if (!eventType) {
    return null;
  }

  // start event may have empty data "{}" - that is fine
  // but if there is no data at all and it is not a start event, skip
  if (!data && eventType !== 'start') {
    return null;
  }

  return parseSSEEvent(eventType, data);
}

