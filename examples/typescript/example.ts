/**
 * LangDAG TypeScript SDK Example
 *
 * Demonstrates:
 * - Starting a conversation with client.prompt() / client.promptStream()
 * - Continuing from a node with node.prompt() / node.promptStream()
 * - Branching to explore alternatives
 * - Listing roots and inspecting tree structure
 *
 * Prerequisites:
 * - LangDAG server running at http://localhost:8080
 */

import {
  LangDAGClient,
  Node,
  NetworkError,
} from 'langdag';

const API_BASE_URL = process.env.LANGDAG_URL || 'http://localhost:8080';
const API_KEY = process.env.LANGDAG_API_KEY;

function printHeader(title: string): void {
  console.log('\n' + '='.repeat(60));
  console.log(`  ${title}`);
  console.log('='.repeat(60) + '\n');
}

function printSeparator(): void {
  console.log('-'.repeat(60));
}

function displayTree(nodes: Node[]): void {
  if (nodes.length === 0) {
    console.log('  (No nodes)');
    return;
  }

  const children = new Map<string, Node[]>();
  for (const node of nodes) {
    const parentId = node.parentId || 'root';
    if (!children.has(parentId)) {
      children.set(parentId, []);
    }
    children.get(parentId)!.push(node);
  }

  const rootNodes = nodes.filter((n) => !n.parentId);

  function printNode(node: Node, indent: string, isLast: boolean): void {
    const branch = isLast ? '`-- ' : '|-- ';
    const preview =
      node.content.length > 40
        ? node.content.substring(0, 40) + '...'
        : node.content;
    console.log(`${indent}${branch}${node.id.substring(0, 8)}: [${node.type}] ${preview.replace(/\n/g, ' ')}`);

    const nodeChildren = children.get(node.id) || [];
    const childIndent = indent + (isLast ? '    ' : '|   ');
    nodeChildren.forEach((child, index) => {
      printNode(child, childIndent, index === nodeChildren.length - 1);
    });
  }

  console.log('Conversation Tree:');
  rootNodes.forEach((node, index) => {
    printNode(node, '  ', index === rootNodes.length - 1);
  });
}

async function main(): Promise<void> {
  console.log('LangDAG TypeScript SDK Example');
  console.log('==============================\n');
  console.log(`Connecting to: ${API_BASE_URL}`);

  const client = new LangDAGClient({
    baseUrl: API_BASE_URL,
    apiKey: API_KEY,
  });

  try {
    printHeader('Step 0: Check Server Health');
    const health = await client.health();
    console.log(`Server status: ${health.status}`);
  } catch (error) {
    if (error instanceof NetworkError) {
      console.error(`Failed to connect to LangDAG server at ${API_BASE_URL}`);
      console.error('Make sure the server is running: langdag serve');
      process.exit(1);
    }
    throw error;
  }

  // ============================================================================
  // Step 1: Start a new conversation with streaming
  // ============================================================================
  printHeader('Step 1: New Conversation (Streaming)');

  const firstMessage = 'Tell me briefly about the history of programming languages. Just 2-3 sentences.';
  console.log(`User: ${firstMessage}\n`);
  printSeparator();

  const stream1 = await client.promptStream(firstMessage);

  console.log('  (Stream started)\n  ');
  for await (const event of stream1.events()) {
    if (event.type === 'delta') {
      process.stdout.write(event.content);
    }
  }
  const node1 = await stream1.node();
  console.log(`\n\n  (Completed - Node ID: ${node1.id.substring(0, 8)}...)`);

  printSeparator();
  console.log(`\nConversation started - Node ID: ${node1.id.substring(0, 12)}...`);

  // ============================================================================
  // Step 2: Continue the conversation using node.prompt()
  // ============================================================================
  printHeader('Step 2: Continue Conversation (node.prompt)');

  const secondMessage = 'What was the first high-level programming language?';
  console.log(`User: ${secondMessage}\n`);
  printSeparator();

  const node2 = await node1.prompt(secondMessage);
  console.log(`\n  Assistant: ${node2.content}\n`);
  console.log(`  (Node ID: ${node2.id.substring(0, 8)}...)`);

  // ============================================================================
  // Step 3: Branch from the first response using node.promptStream()
  // ============================================================================
  printHeader('Step 3: Branch with Streaming (node.promptStream)');

  console.log(`Branching from node: ${node1.id.substring(0, 12)}...\n`);

  const branchMessage = 'Instead of history, tell me which language you recommend for beginners today.';
  console.log(`User (alternative branch): ${branchMessage}\n`);
  printSeparator();

  const stream3 = await node1.promptStream(branchMessage);

  console.log('  ');
  for await (const event of stream3.events()) {
    if (event.type === 'delta') {
      process.stdout.write(event.content);
    }
  }
  const node3 = await stream3.node();
  console.log(`\n\n  (Branch node: ${node3.id.substring(0, 8)}...)`);

  // ============================================================================
  // Step 4: List all conversation roots
  // ============================================================================
  printHeader('Step 4: List Conversation Roots');

  const roots = await client.listRoots();
  console.log(`Found ${roots.length} conversation(s):\n`);

  for (const root of roots.slice(0, 5)) {
    const title = root.title || '(untitled)';
    const preview = root.content.length > 50 ? root.content.substring(0, 50) + '...' : root.content;
    console.log(`  - ${root.id.substring(0, 12)}... | ${title} | ${preview}`);
  }

  if (roots.length > 5) {
    console.log(`  ... and ${roots.length - 5} more`);
  }

  // ============================================================================
  // Step 5: Show the tree structure
  // ============================================================================
  printHeader('Step 5: Conversation Tree');

  const tree = await client.getTree(node1.id);
  console.log(`Tree has ${tree.length} nodes:\n`);
  displayTree(tree);

  // ============================================================================
  // Summary
  // ============================================================================
  printHeader('Summary');

  console.log('This example demonstrated:');
  console.log('  1. Starting a conversation with client.promptStream()');
  console.log('  2. Continuing from a node with node.prompt()');
  console.log('  3. Branching with streaming via node.promptStream()');
  console.log('  4. Listing conversation roots');
  console.log('  5. Visualizing the tree structure');
  console.log();
  console.log(`Your first node ID: ${node1.id}`);
  console.log(`\nRun "langdag show ${node1.id.substring(0, 8)}" to view this conversation in the CLI.`);
}

main().catch((error) => {
  console.error('\nError:', error.message);
  if (error.stack) {
    console.error(error.stack);
  }
  process.exit(1);
});
