/**
 * Cross-SDK consistency tests — verify the TypeScript SDK parses SSE fixtures
 * identically to the Go and Python SDKs. The exact same SSE byte strings
 * are used in sdks/go/cross_sdk_test.go and sdks/python/tests/test_cross_sdk.py.
 */

import { describe, it, expect } from 'vitest';
import { parseSSEStream } from './sse.js';
import type { SSEEvent } from './types.js';

// Canonical SSE fixtures — byte-for-byte identical across all 3 SDKs.

const FIXTURE_NORMAL =
  'event: start\ndata: {}\n\n' +
  'event: delta\ndata: {"content":"Hello "}\n\n' +
  'event: delta\ndata: {"content":"world!"}\n\n' +
  'event: done\ndata: {"node_id":"test-node-1"}\n\n';

const FIXTURE_ERROR_MID_STREAM =
  'event: start\ndata: {}\n\n' +
  'event: delta\ndata: {"content":"partial "}\n\n' +
  'event: error\ndata: provider crashed\n\n';

const FIXTURE_MULTI_LINE_ERROR =
  'event: start\ndata: {}\n\n' +
  'event: error\ndata: line one\ndata: line two\ndata: line three\n\n';

const FIXTURE_ERROR_ONLY = 'event: error\ndata: unauthorized\n\n';

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

describe('Cross-SDK: Normal flow', () => {
  it('produces 4 events in correct order', async () => {
    const events = await collectEvents(FIXTURE_NORMAL);
    expect(events.length).toBe(4);
    expect(events[0].type).toBe('start');
    expect(events[1].type).toBe('delta');
    expect(events[2].type).toBe('delta');
    expect(events[3].type).toBe('done');
  });

  it('has correct delta content', async () => {
    const events = await collectEvents(FIXTURE_NORMAL);
    expect((events[1] as { content: string }).content).toBe('Hello ');
    expect((events[2] as { content: string }).content).toBe('world!');
  });

  it('has correct accumulated content', async () => {
    const events = await collectEvents(FIXTURE_NORMAL);
    const content = events
      .filter((e): e is { type: 'delta'; content: string } => e.type === 'delta')
      .map(e => e.content)
      .join('');
    expect(content).toBe('Hello world!');
  });

  it('has correct node_id', async () => {
    const events = await collectEvents(FIXTURE_NORMAL);
    expect((events[3] as { node_id: string }).node_id).toBe('test-node-1');
  });

  it('has no error events', async () => {
    const events = await collectEvents(FIXTURE_NORMAL);
    const errors = events.filter(e => e.type === 'error');
    expect(errors.length).toBe(0);
  });
});

describe('Cross-SDK: Error mid-stream', () => {
  it('produces 3 events in correct order', async () => {
    const events = await collectEvents(FIXTURE_ERROR_MID_STREAM);
    expect(events.length).toBe(3);
    expect(events[0].type).toBe('start');
    expect(events[1].type).toBe('delta');
    expect(events[2].type).toBe('error');
  });

  it('preserves partial content', async () => {
    const events = await collectEvents(FIXTURE_ERROR_MID_STREAM);
    expect((events[1] as { content: string }).content).toBe('partial ');
  });

  it('has correct error message', async () => {
    const events = await collectEvents(FIXTURE_ERROR_MID_STREAM);
    expect((events[2] as { error: string }).error).toBe('provider crashed');
  });

  it('has no done event', async () => {
    const events = await collectEvents(FIXTURE_ERROR_MID_STREAM);
    const doneEvents = events.filter(e => e.type === 'done');
    expect(doneEvents.length).toBe(0);
  });
});

describe('Cross-SDK: Multi-line error', () => {
  it('produces 2 events in correct order', async () => {
    const events = await collectEvents(FIXTURE_MULTI_LINE_ERROR);
    expect(events.length).toBe(2);
    expect(events[0].type).toBe('start');
    expect(events[1].type).toBe('error');
  });

  it('joins multi-line data with newlines', async () => {
    const events = await collectEvents(FIXTURE_MULTI_LINE_ERROR);
    expect((events[1] as { error: string }).error).toBe(
      'line one\nline two\nline three'
    );
  });

  it('has no delta events', async () => {
    const events = await collectEvents(FIXTURE_MULTI_LINE_ERROR);
    const deltas = events.filter(e => e.type === 'delta');
    expect(deltas.length).toBe(0);
  });
});

describe('Cross-SDK: Error only', () => {
  it('produces single error event', async () => {
    const events = await collectEvents(FIXTURE_ERROR_ONLY);
    expect(events.length).toBe(1);
    expect(events[0].type).toBe('error');
  });

  it('has correct error message', async () => {
    const events = await collectEvents(FIXTURE_ERROR_ONLY);
    expect((events[0] as { error: string }).error).toBe('unauthorized');
  });

  it('has no delta or done events', async () => {
    const events = await collectEvents(FIXTURE_ERROR_ONLY);
    const deltas = events.filter(e => e.type === 'delta');
    const dones = events.filter(e => e.type === 'done');
    expect(deltas.length).toBe(0);
    expect(dones.length).toBe(0);
  });
});
