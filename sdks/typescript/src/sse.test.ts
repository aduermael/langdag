import { describe, it, expect } from 'vitest';
import { parseSSEEvent, parseSSEStream } from './sse.js';
import { SSEParseError } from './errors.js';
import type { SSEEvent } from './types.js';

describe('parseSSEEvent', () => {
  it('parses start event', () => {
    const event = parseSSEEvent('start', '{}');
    expect(event).toEqual({ type: 'start' });
  });

  it('parses start event with empty data', () => {
    const event = parseSSEEvent('start', '');
    expect(event).toEqual({ type: 'start' });
  });

  it('parses delta event', () => {
    const event = parseSSEEvent('delta', '{"content":"Hello "}');
    expect(event).toEqual({ type: 'delta', content: 'Hello ' });
  });

  it('parses done event', () => {
    const event = parseSSEEvent('done', '{"node_id":"n-1"}');
    expect(event).toEqual({ type: 'done', node_id: 'n-1' });
  });

  it('parses error event', () => {
    const event = parseSSEEvent('error', 'something went wrong');
    expect(event).toEqual({ type: 'error', error: 'something went wrong' });
  });

  it('throws on unknown event type', () => {
    expect(() => parseSSEEvent('unknown', '{}')).toThrow(SSEParseError);
  });

  it('throws on malformed delta event data', () => {
    expect(() => parseSSEEvent('delta', 'not json')).toThrow(SSEParseError);
  });

  it('throws on malformed done event data', () => {
    expect(() => parseSSEEvent('done', 'not json')).toThrow(SSEParseError);
  });
});

describe('parseSSEStream', () => {
  function createStream(text: string): ReadableStream<Uint8Array> {
    const encoder = new TextEncoder();
    return new ReadableStream({
      start(controller) {
        controller.enqueue(encoder.encode(text));
        controller.close();
      },
    });
  }

  async function collectEvents(stream: AsyncGenerator<SSEEvent>): Promise<SSEEvent[]> {
    const events: SSEEvent[] = [];
    for await (const event of stream) {
      events.push(event);
    }
    return events;
  }

  it('parses complete SSE stream', async () => {
    const text = [
      'event: start',
      'data: {}',
      '',
      'event: delta',
      'data: {"content":"Hello "}',
      '',
      'event: delta',
      'data: {"content":"world!"}',
      '',
      'event: done',
      'data: {"node_id":"n-1"}',
      '',
    ].join('\n');

    const events = await collectEvents(parseSSEStream(createStream(text)));
    expect(events).toHaveLength(4);
    expect(events[0]).toEqual({ type: 'start' });
    expect(events[1]).toEqual({ type: 'delta', content: 'Hello ' });
    expect(events[2]).toEqual({ type: 'delta', content: 'world!' });
    expect(events[3]).toEqual({ type: 'done', node_id: 'n-1' });
  });

  it('handles empty stream', async () => {
    const events = await collectEvents(parseSSEStream(createStream('')));
    expect(events).toHaveLength(0);
  });

  it('handles error event in stream', async () => {
    const text = [
      'event: error',
      'data: something broke',
      '',
    ].join('\n');

    const events = await collectEvents(parseSSEStream(createStream(text)));
    expect(events).toHaveLength(1);
    expect(events[0]).toEqual({ type: 'error', error: 'something broke' });
  });

  it('handles chunked delivery', async () => {
    const encoder = new TextEncoder();
    const chunks = [
      'event: start\ndata: {',
      '}\n\nevent: done\n',
      'data: {"node_id":"n-1"}\n\n',
    ];

    const stream = new ReadableStream({
      start(controller) {
        for (const chunk of chunks) {
          controller.enqueue(encoder.encode(chunk));
        }
        controller.close();
      },
    });

    const events = await collectEvents(parseSSEStream(stream));
    expect(events).toHaveLength(2);
    expect(events[0].type).toBe('start');
    expect(events[1].type).toBe('done');
  });

  // --- 4b: SSE edge cases ---

  it('handles multi-line data fields', async () => {
    const text = [
      'event: error',
      'data: line one',
      'data: line two',
      '',
    ].join('\n');

    const events = await collectEvents(parseSSEStream(createStream(text)));
    expect(events).toHaveLength(1);
    expect(events[0].type).toBe('error');
    if (events[0].type === 'error') {
      expect(events[0].error).toBe('line one\nline two');
    }
  });

  it('handles SSE comments (lines starting with :)', async () => {
    const text = [
      ': this is a comment',
      'event: start',
      'data: {}',
      '',
      ': another comment',
      'event: done',
      'data: {"node_id":"n-1"}',
      '',
    ].join('\n');

    const events = await collectEvents(parseSSEStream(createStream(text)));
    expect(events).toHaveLength(2);
    expect(events[0].type).toBe('start');
    expect(events[1].type).toBe('done');
  });

  it('handles event blocks with missing data field', async () => {
    const text = [
      'event: start',
      '',
      'event: done',
      'data: {"node_id":"n-1"}',
      '',
    ].join('\n');

    const events = await collectEvents(parseSSEStream(createStream(text)));
    // start event with no data: parseEventBlock returns start (start allows empty data)
    // done event with data: should parse normally
    expect(events).toHaveLength(2);
    expect(events[0].type).toBe('start');
    expect(events[1].type).toBe('done');
  });

  it('handles stream without trailing newline', async () => {
    const text = 'event: done\ndata: {"node_id":"n-1"}';

    const events = await collectEvents(parseSSEStream(createStream(text)));
    expect(events).toHaveLength(1);
    expect(events[0].type).toBe('done');
    if (events[0].type === 'done') {
      expect(events[0].node_id).toBe('n-1');
    }
  });

  it('handles empty content deltas', async () => {
    const text = [
      'event: delta',
      'data: {"content":""}',
      '',
    ].join('\n');

    const events = await collectEvents(parseSSEStream(createStream(text)));
    expect(events).toHaveLength(1);
    expect(events[0].type).toBe('delta');
    if (events[0].type === 'delta') {
      expect(events[0].content).toBe('');
    }
  });
});
