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

  // --- 11c: Chunked SSE delivery edge cases ---

  it('handles chunk split mid-\\n\\n separator', async () => {
    // Split the double-newline separator across two chunks: first chunk ends with
    // a single \n, second chunk starts with the second \n
    const encoder = new TextEncoder();
    const chunk1 = 'event: start\ndata: {}\n';        // first \n of separator
    const chunk2 = '\nevent: done\ndata: {"node_id":"n-1"}\n\n';  // second \n starts next

    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue(encoder.encode(chunk1));
        controller.enqueue(encoder.encode(chunk2));
        controller.close();
      },
    });

    const events = await collectEvents(parseSSEStream(stream));
    expect(events).toHaveLength(2);
    expect(events[0].type).toBe('start');
    expect(events[1].type).toBe('done');
    if (events[1].type === 'done') {
      expect(events[1].node_id).toBe('n-1');
    }
  });

  it('handles chunk split mid-data: prefix', async () => {
    const encoder = new TextEncoder();
    // Split "data:" across two chunks
    const chunk1 = 'event: delta\nda';
    const chunk2 = 'ta: {"content":"split"}\n\n';

    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue(encoder.encode(chunk1));
        controller.enqueue(encoder.encode(chunk2));
        controller.close();
      },
    });

    const events = await collectEvents(parseSSEStream(stream));
    expect(events).toHaveLength(1);
    expect(events[0].type).toBe('delta');
    if (events[0].type === 'delta') {
      expect(events[0].content).toBe('split');
    }
  });

  it('handles chunk split mid-event: prefix', async () => {
    const encoder = new TextEncoder();
    // Split "event:" across two chunks
    const chunk1 = 'ev';
    const chunk2 = 'ent: delta\ndata: {"content":"hello"}\n\n';

    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue(encoder.encode(chunk1));
        controller.enqueue(encoder.encode(chunk2));
        controller.close();
      },
    });

    const events = await collectEvents(parseSSEStream(stream));
    expect(events).toHaveLength(1);
    expect(events[0].type).toBe('delta');
    if (events[0].type === 'delta') {
      expect(events[0].content).toBe('hello');
    }
  });

  it('handles chunk split mid-UTF8 character', async () => {
    // UTF-8 encoding of "é" (U+00E9) is 0xC3 0xA9 (2 bytes)
    // Split those bytes across chunks to test TextDecoder stream mode
    const fullText = 'event: delta\ndata: {"content":"café"}\n\n';
    const fullBytes = new TextEncoder().encode(fullText);

    // Find the é byte position — "caf" then é
    // "café" in JSON is after {"content":"caf  = position within the full text
    const cafeIdx = fullText.indexOf('café');
    // The 'é' starts at cafeIdx + 3 in the string, but in UTF-8 it's 2 bytes
    // Let's find it by encoding just the prefix
    const prefixBytes = new TextEncoder().encode(fullText.substring(0, cafeIdx + 3)); // up to "caf"
    const splitPoint = prefixBytes.length; // right before the first byte of é

    // Split between the two bytes of the é character
    const chunk1 = fullBytes.slice(0, splitPoint + 1); // includes first byte of é (0xC3)
    const chunk2 = fullBytes.slice(splitPoint + 1);     // starts with second byte (0xA9)

    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue(chunk1);
        controller.enqueue(chunk2);
        controller.close();
      },
    });

    const events = await collectEvents(parseSSEStream(stream));
    expect(events).toHaveLength(1);
    expect(events[0].type).toBe('delta');
    if (events[0].type === 'delta') {
      expect(events[0].content).toBe('café');
    }
  });

  it('handles many small single-byte chunks', async () => {
    const fullText = 'event: delta\ndata: {"content":"tiny"}\n\n';
    const fullBytes = new TextEncoder().encode(fullText);

    const stream = new ReadableStream({
      start(controller) {
        // Send one byte at a time
        for (let i = 0; i < fullBytes.length; i++) {
          controller.enqueue(fullBytes.slice(i, i + 1));
        }
        controller.close();
      },
    });

    const events = await collectEvents(parseSSEStream(stream));
    expect(events).toHaveLength(1);
    expect(events[0].type).toBe('delta');
    if (events[0].type === 'delta') {
      expect(events[0].content).toBe('tiny');
    }
  });

  it('handles chunk split mid-JSON in data field', async () => {
    const encoder = new TextEncoder();
    // Split JSON object across chunks at an awkward point
    const chunk1 = 'event: done\ndata: {"node_';
    const chunk2 = 'id":"n-split"}\n\n';

    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue(encoder.encode(chunk1));
        controller.enqueue(encoder.encode(chunk2));
        controller.close();
      },
    });

    const events = await collectEvents(parseSSEStream(stream));
    expect(events).toHaveLength(1);
    expect(events[0].type).toBe('done');
    if (events[0].type === 'done') {
      expect(events[0].node_id).toBe('n-split');
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
