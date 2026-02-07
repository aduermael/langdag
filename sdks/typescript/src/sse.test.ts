import { describe, it, expect } from 'vitest';
import { parseSSEEvent, parseSSEStream } from './sse.js';
import { SSEParseError } from './errors.js';
import type { SSEEvent } from './types.js';

describe('parseSSEEvent', () => {
  it('parses start event', () => {
    const event = parseSSEEvent('start', '{"dag_id":"dag-1"}');
    expect(event).toEqual({ type: 'start', dag_id: 'dag-1' });
  });

  it('parses delta event', () => {
    const event = parseSSEEvent('delta', '{"content":"Hello "}');
    expect(event).toEqual({ type: 'delta', content: 'Hello ' });
  });

  it('parses done event', () => {
    const event = parseSSEEvent('done', '{"dag_id":"dag-1","node_id":"n-1"}');
    expect(event).toEqual({ type: 'done', dag_id: 'dag-1', node_id: 'n-1' });
  });

  it('parses error event', () => {
    const event = parseSSEEvent('error', 'something went wrong');
    expect(event).toEqual({ type: 'error', error: 'something went wrong' });
  });

  it('throws on unknown event type', () => {
    expect(() => parseSSEEvent('unknown', '{}')).toThrow(SSEParseError);
  });

  it('throws on malformed start event data', () => {
    expect(() => parseSSEEvent('start', 'not json')).toThrow(SSEParseError);
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
      'data: {"dag_id":"dag-1"}',
      '',
      'event: delta',
      'data: {"content":"Hello "}',
      '',
      'event: delta',
      'data: {"content":"world!"}',
      '',
      'event: done',
      'data: {"dag_id":"dag-1","node_id":"n-1"}',
      '',
    ].join('\n');

    const events = await collectEvents(parseSSEStream(createStream(text)));
    expect(events).toHaveLength(4);
    expect(events[0]).toEqual({ type: 'start', dag_id: 'dag-1' });
    expect(events[1]).toEqual({ type: 'delta', content: 'Hello ' });
    expect(events[2]).toEqual({ type: 'delta', content: 'world!' });
    expect(events[3]).toEqual({ type: 'done', dag_id: 'dag-1', node_id: 'n-1' });
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
      'event: start\ndata: {"dag_',
      'id":"dag-1"}\n\nevent: done\n',
      'data: {"dag_id":"dag-1","node_id":"n-1"}\n\n',
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
});
