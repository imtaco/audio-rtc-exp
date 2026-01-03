<script>
import { onDestroy } from 'svelte';
import {
  RPCClient,
  clientId,
  logs,
  statusMessage,
  statusType,
  currentStep,
  isConnected
} from './rpc.js';

let wsUrl = 'ws://localhost:8081/ws';
let token = '';

// Create client instance
const client = new RPCClient({
  wsUrl,
  token
});

async function connect() {
  // Update client config before connecting
  client.updateConfig({
    wsUrl,
    token
  });

  await client.run();
}

async function disconnect() {
  await client.disconnect();
}

onDestroy(() => {
  // Don't disconnect if it's just HMR reload
  if (!import.meta.hot) {
    disconnect();
  }
});
</script>

<main>
  <h1>RPC Client</h1>

  <div class="form-group">
    <!-- svelte-ignore a11y-label-has-associated-control -->
    <label>Client ID:</label>
    <div class="client-id">{$clientId}</div>
  </div>

  <div class="form-group">
    <label for="wsUrl">WebSocket URL:</label>
    <input type="text" id="wsUrl" bind:value={wsUrl} placeholder="ws://localhost:8081/rpc" autocomplete="off">
  </div>

  <div class="form-group">
    <label for="token">Token:</label>
    <input type="text" id="token" bind:value={token} placeholder="Enter your token" autocomplete="off">
  </div>

  <div class="controls">
    <button on:click={connect} disabled={$isConnected}>Connect!</button>
    <button on:click={disconnect} disabled={!$isConnected}>Disconnect!</button>
  </div>

  {#if $statusMessage}
    <div class="status {$statusType}">
      {$statusMessage}
    </div>
  {/if}

  {#if $currentStep}
    <div class="current-step">
      <h3>Current Step:</h3>
      <div class="step-content">{$currentStep}</div>
    </div>
  {/if}

  <div id="logs">
    <h3>Logs</h3>
    <div class="logs-content">
      {#each $logs as logEntry}
        <div class="log-entry">
          [{logEntry.time}] {logEntry.message}
        </div>
      {/each}
    </div>
  </div>
</main>

<style>
  :global(body) {
    font-family: Arial, sans-serif;
    max-width: 800px;
    margin: 50px auto;
    padding: 20px;
    background-color: #f5f5f5;
  }

  main {
    background-color: white;
    padding: 30px;
    border-radius: 8px;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
  }

  h1 {
    margin-top: 0;
    color: #333;
  }

  h3 {
    margin-top: 0;
    margin-bottom: 10px;
    color: #555;
  }

  .form-group {
    margin-bottom: 20px;
  }

  .form-group label {
    display: block;
    margin-bottom: 5px;
    font-weight: 500;
    color: #333;
  }

  .form-group input {
    width: 100%;
    padding: 10px;
    border: 1px solid #ddd;
    border-radius: 4px;
    font-size: 14px;
    box-sizing: border-box;
  }

  .form-group input:focus {
    outline: none;
    border-color: #007bff;
  }

  .client-id {
    font-family: monospace;
    color: #666;
    font-size: 12px;
    padding: 8px;
    background-color: #f8f9fa;
    border-radius: 4px;
  }

  .controls {
    margin: 20px 0;
    display: flex;
    gap: 10px;
  }

  button {
    padding: 10px 20px;
    background-color: #007bff;
    color: white;
    border: none;
    border-radius: 4px;
    cursor: pointer;
    font-size: 14px;
    font-weight: 500;
    transition: background-color 0.2s;
  }

  button:hover {
    background-color: #0056b3;
  }

  button:disabled {
    background-color: #ccc;
    cursor: not-allowed;
  }

  .status {
    margin: 20px 0;
    padding: 12px 15px;
    border-radius: 4px;
    font-weight: 500;
  }

  .status.info {
    background-color: #d1ecf1;
    color: #0c5460;
    border-left: 4px solid #17a2b8;
  }

  .status.success {
    background-color: #d4edda;
    color: #155724;
    border-left: 4px solid #28a745;
  }

  .status.error {
    background-color: #f8d7da;
    color: #721c24;
    border-left: 4px solid #dc3545;
  }

  .current-step {
    margin: 20px 0;
    padding: 15px;
    background-color: #fff3cd;
    border-radius: 4px;
    border-left: 4px solid #ffc107;
  }

  .step-content {
    font-weight: 500;
    color: #856404;
  }

  #logs {
    margin-top: 30px;
  }

  .logs-content {
    padding: 15px;
    background-color: #f8f9fa;
    border-radius: 4px;
    max-height: 300px;
    overflow-y: auto;
    font-family: 'Courier New', monospace;
    font-size: 12px;
    border: 1px solid #dee2e6;
  }

  .log-entry {
    margin: 3px 0;
    color: #495057;
  }

  .logs-content:empty::before {
    content: 'No logs yet...';
    color: #999;
    font-style: italic;
  }
</style>
