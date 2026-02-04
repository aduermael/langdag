/**
 * LangDAG TypeScript SDK Example
 *
 * This example demonstrates the core features of the LangDAG SDK:
 * - Starting a chat with streaming
 * - Continuing a conversation
 * - Forking from an earlier node to explore alternatives
 * - Listing and inspecting DAGs
 *
 * Prerequisites:
 * - LangDAG server running at http://localhost:8080
 * - API key configured (or run without authentication)
 */

import {
  LangDAGClient,
  type SSEEvent,
  type DAGDetail,
  type Node,
  collectStreamContent,
  NotFoundError,
  NetworkError,
} from 'langdag';

// Configuration
const API_BASE_URL = process.env.LANGDAG_URL || 'http://localhost:8080';
const API_KEY = process.env.LANGDAG_API_KEY;

// Helper to print section headers
function printHeader(title: string): void {
  console.log('\n' + '='.repeat(60));
  console.log(`  ${title}`);
  console.log('='.repeat(60) + '\n');
}

// Helper to print a separator line
function printSeparator(): void {
  console.log('-'.repeat(60));
}

// Helper to stream and collect response content
async function streamChat(
  stream: AsyncGenerator<SSEEvent, void, undefined>,
  label: string
): Promise<{ dagId: string; nodeId: string; content: string }> {
  console.log(`[${label}] Streaming response:`);
  console.log();

  let dagId = '';
  let nodeId = '';
  let content = '';

  for await (const event of stream) {
    switch (event.type) {
      case 'start':
        dagId = event.dag_id;
        console.log(`  (DAG started: ${dagId.substring(0, 8)}...)`);
        console.log();
        process.stdout.write('  ');
        break;
      case 'delta':
        content += event.content;
        process.stdout.write(event.content);
        break;
      case 'done':
        nodeId = event.node_id;
        console.log('\n');
        console.log(`  (Completed - Node ID: ${nodeId.substring(0, 8)}...)`);
        break;
      case 'error':
        console.error(`\n  [ERROR] ${event.error}`);
        break;
    }
  }

  return { dagId, nodeId, content };
}

// Helper to display DAG structure with branching
function displayDagStructure(dag: DAGDetail): void {
  console.log(`DAG: ${dag.id.substring(0, 12)}...`);
  console.log(`Title: ${dag.title || '(untitled)'}`);
  console.log(`Status: ${dag.status}`);
  console.log(`Node Count: ${dag.node_count || dag.nodes?.length || 0}`);
  console.log(`Created: ${dag.created_at}`);
  console.log();

  if (!dag.nodes || dag.nodes.length === 0) {
    console.log('  (No nodes)');
    return;
  }

  // Build a tree structure to visualize branching
  const nodeMap = new Map<string, Node>();
  const children = new Map<string, Node[]>();

  for (const node of dag.nodes) {
    nodeMap.set(node.id, node);
    const parentId = node.parent_id || 'root';
    if (!children.has(parentId)) {
      children.set(parentId, []);
    }
    children.get(parentId)!.push(node);
  }

  // Find root nodes (nodes without a parent_id in the map, or with parent_id = undefined)
  const rootNodes = dag.nodes.filter((n) => !n.parent_id);

  // Recursive function to print the tree
  function printNode(node: Node, indent: string, isLast: boolean): void {
    const branch = isLast ? '`-- ' : '|-- ';
    const contentPreview =
      node.content.length > 40
        ? node.content.substring(0, 40) + '...'
        : node.content;

    const nodeInfo = `[${node.node_type}] ${contentPreview.replace(/\n/g, ' ')}`;
    console.log(`${indent}${branch}${node.id.substring(0, 8)}: ${nodeInfo}`);

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

// Main example function
async function main(): Promise<void> {
  console.log('LangDAG TypeScript SDK Example');
  console.log('==============================\n');
  console.log(`Connecting to: ${API_BASE_URL}`);
  console.log(`API Key: ${API_KEY ? '(configured)' : '(not set)'}`);

  // Initialize the client
  const client = new LangDAGClient({
    baseUrl: API_BASE_URL,
    apiKey: API_KEY,
  });

  try {
    // Check server health
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
  // Step 1: Start a new chat about a topic (with streaming)
  // ============================================================================
  printHeader('Step 1: Start a New Chat (Streaming)');
  console.log('Topic: The history of programming languages\n');

  const firstMessage =
    'Tell me briefly about the history of programming languages. Just 2-3 sentences.';
  console.log(`User: ${firstMessage}\n`);
  printSeparator();

  const stream1 = client.chat({
    message: firstMessage,
    stream: true,
  });

  const result1 = await streamChat(stream1, 'Assistant');
  const dagId = result1.dagId;
  const firstNodeId = result1.nodeId;

  printSeparator();
  console.log(`\nConversation started in DAG: ${dagId.substring(0, 12)}...`);

  // ============================================================================
  // Step 2: Continue the conversation
  // ============================================================================
  printHeader('Step 2: Continue the Conversation');

  const secondMessage = 'What was the first high-level programming language?';
  console.log(`User: ${secondMessage}\n`);
  printSeparator();

  const stream2 = client.continueChat(dagId, {
    message: secondMessage,
    stream: true,
  });

  const result2 = await streamChat(stream2, 'Assistant');
  const secondNodeId = result2.nodeId;

  printSeparator();
  console.log(`\nContinued conversation - new node: ${secondNodeId.substring(0, 12)}...`);

  // ============================================================================
  // Step 3: Fork from the first response to explore an alternative
  // ============================================================================
  printHeader('Step 3: Fork from Earlier Node (Branching)');

  console.log('Now we will fork from the first assistant response to ask a different');
  console.log('follow-up question, creating a branch in our conversation DAG.\n');
  console.log(`Forking from node: ${firstNodeId.substring(0, 12)}...`);

  const forkMessage =
    'Instead of history, tell me which language you recommend for beginners today.';
  console.log(`\nUser (alternative branch): ${forkMessage}\n`);
  printSeparator();

  const stream3 = client.forkChat(dagId, {
    node_id: firstNodeId,
    message: forkMessage,
    stream: true,
  });

  const result3 = await streamChat(stream3, 'Assistant (branch)');

  printSeparator();
  console.log(`\nCreated branch - new node: ${result3.nodeId.substring(0, 12)}...`);

  // ============================================================================
  // Step 4: List all DAGs and show structure
  // ============================================================================
  printHeader('Step 4: List All DAGs');

  const dags = await client.listDags();
  console.log(`Found ${dags.length} DAG(s):\n`);

  for (const dag of dags.slice(0, 5)) {
    // Show up to 5 most recent
    console.log(`  - ${dag.id.substring(0, 12)}... | ${dag.status} | ${dag.title || '(untitled)'}`);
  }

  if (dags.length > 5) {
    console.log(`  ... and ${dags.length - 5} more`);
  }

  // ============================================================================
  // Step 5: Show the DAG structure with branching
  // ============================================================================
  printHeader('Step 5: Show DAG Structure (Branching Visualization)');

  const dagDetail = await client.getDag(dagId);
  displayDagStructure(dagDetail);

  // ============================================================================
  // Summary
  // ============================================================================
  printHeader('Summary');

  console.log('This example demonstrated:');
  console.log('  1. Starting a new chat with streaming responses');
  console.log('  2. Continuing an existing conversation');
  console.log('  3. Forking from an earlier node to create a branch');
  console.log('  4. Listing all DAGs in the system');
  console.log('  5. Visualizing the conversation structure as a tree');
  console.log();
  console.log('Key Concepts:');
  console.log('  - DAG: A conversation stored as a directed acyclic graph');
  console.log('  - Node: A single message (user or assistant) in the conversation');
  console.log('  - Fork: Creating a branch by continuing from any earlier node');
  console.log('  - Stream: Real-time response streaming via Server-Sent Events');
  console.log();
  console.log(`Your conversation DAG ID: ${dagId}`);
  console.log('\nRun "langdag show ' + dagId.substring(0, 8) + '" to view this conversation in the CLI.');
}

// Run the example
main().catch((error) => {
  console.error('\nError:', error.message);
  if (error.stack) {
    console.error(error.stack);
  }
  process.exit(1);
});
