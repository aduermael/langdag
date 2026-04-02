/**
 * Cross-SDK graceful degradation tests — verify the TypeScript SDK makes
 * accumulated content available when streams terminate abnormally.
 * Identical scenarios tested in Go and Python SDKs.
 */

import { describe, it, expect } from 'vitest';
import { parseSSEStream } from './sse.js';
import { LangDAGClient, Stream } from './client.js';
import { SSEParseError } from './errors.js';
import type { SSEEvent } from './types.js';

// Fixture: stream ends without done event
const FIXTURE_NO_DONE =
  'event: start\ndata: {}\n\n' +
  'event: delta\ndata: {"content":"Hello "}\n\n' +
  'event: delta\ndata: {"content":"world!"}\n\n';

// Fixture: stream ends with error event
const FIXTURE_ERROR_TERMINATION =
  'event: start\ndata: {}\n\n' +
  'event: delta\ndata: {"content":"before "}\n\n' +
  'event: delta\ndata: {"content":"error"}\n\n' +
  'event: error\ndata: connection reset by peer\n\n';

// Fixture: empty response (start only)
const FIXTURE_EMPTY_RESPONSE = 'event: start\ndata: {}\n\n';

function createStream(text: string): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder();
  return new ReadableStream({
    start(controller) {
      controller.enqueue(encoder.encode(text));
      controller.close();
    },
  });
}

async function collectEvents(fixture: string): Promise<SSEEvent[]> {
  const events: SSEEvent[] = [];
  for await (const event of parseSSEStream(createStream(fixture))) {
    events.push(event);
  }
  return events;
}

describe('Graceful Degradation: No done event', () => {
  it('content available from delta events', async () => {
    const events = await collectEvents(FIXTURE_NO_DONE);
    const content = events
      .filter((e): e is { type: 'delta'; content: string } => e.type === 'delta')
      .map(e => e.content)
      .join('');
    expect(content).toBe('Hello world!');
  });

  it('iteration completes without hanging', async () => {
    const events = await collectEvents(FIXTURE_NO_DONE);
    expect(events.length).toBe(3); // start + 2 deltas
  });

  it('Stream.content preserves accumulated text', async () => {
    const stream = new Stream(createStream(FIXTURE_NO_DONE), {} as LangDAGClient);
    // Drain via node() which auto-consumes
    await expect(stream.node()).rejects.toThrow(SSEParseError);
    expect(stream.content).toBe('Hello world!');
  });

  it('Stream.node() throws SSEParseError', async () => {
    const stream = new Stream(createStream(FIXTURE_NO_DONE), {} as LangDAGClient);
    await expect(stream.node()).rejects.toThrow('done event');
  });
});

describe('Graceful Degradation: Error termination', () => {
  it('content preserved before error', async () => {
    const events = await collectEvents(FIXTURE_ERROR_TERMINATION);
    const content = events
      .filter((e): e is { type: 'delta'; content: string } => e.type === 'delta')
      .map(e => e.content)
      .join('');
    expect(content).toBe('before error');
  });

  it('error message surfaced', async () => {
    const events = await collectEvents(FIXTURE_ERROR_TERMINATION);
    const errors = events.filter(e => e.type === 'error');
    expect(errors.length).toBe(1);
    expect((errors[0] as { error: string }).error).toBe('connection reset by peer');
  });

  it('Stream.content preserves text before error', async () => {
    const stream = new Stream(createStream(FIXTURE_ERROR_TERMINATION), {} as LangDAGClient);
    // node() auto-consumes and rejects (error event, no done)
    await expect(stream.node()).rejects.toThrow(SSEParseError);
    expect(stream.content).toBe('before error');
  });
});

describe('Graceful Degradation: Empty response', () => {
  it('no delta content', async () => {
    const events = await collectEvents(FIXTURE_EMPTY_RESPONSE);
    const deltas = events.filter(e => e.type === 'delta');
    expect(deltas.length).toBe(0);
  });

  it('iteration completes', async () => {
    const events = await collectEvents(FIXTURE_EMPTY_RESPONSE);
    expect(events.length).toBe(1); // just start
  });
});
