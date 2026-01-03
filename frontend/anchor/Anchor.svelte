<script>
import { onDestroy } from 'svelte';
import {
  AnchorClient,
  clientId,
  connStatus,
  currentStep,
  logs,
  members,
  showAudioCtrl,
  statusMessage,
  statusType,
} from './AnchorClient.js';

let wsUrl = 'ws://localhost:8081/ws';
let token = '';
let tokenPayload = '';
let pin = '';
let displayName = 'Browser User';
let wavUrl = 'http://localhost:8080/sounds/speech_mono.wav';

$: tokenPayload = parseJwt(token)

// Create client instance
const client = new AnchorClient({
  wsUrl,
  token,
  pin,
  displayName,
  wavUrl
});

// Setup event callbacks
client.onShowAudioControls = (show) => {
  showAudioControls = show;
};

async function run() {
  // Update client config before connecting
  client.updateConfig({
    wsUrl,
    token,
    pin,
    displayName,
    wavUrl
  });

  // Bind DOM elements to client
  client.remoteAudioEl = document.getElementById('remoteAudio');
  client.playerEl = document.querySelector('.wavplayer');

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

function parseJwt(token) {
  const [, payload] = token.split('.');
  if (!payload) return '<unparsable>';

  return decodeURIComponent(atob(payload));
}
</script>

<main>
  <h1>Anchor</h1>

  <div class="form-group">
    <!-- svelte-ignore a11y-label-has-associated-control -->
    <label>ClientId:</label>
    <div>{$clientId}</div>
  </div>

  <div class="form-group">
    <!-- svelte-ignore a11y-label-has-associated-control -->
    <label>TokenPayload:</label>
    <div>{tokenPayload}</div>
  </div>

  <div class="form-group">
    <label for="wsUrl">Room Manager WebSocket URL:</label>
    <input type="text" id="wsUrl" bind:value={wsUrl} placeholder="ws://localhost:8081" autocomplete="off">
  </div>

  <div class="form-group">
    <label for="token">Token:</label>
    <input type="text" id="token" bind:value={token} placeholder="Enter your token" autocomplete="off">
  </div>

  <div class="form-group">
    <label for="pin">Room PIN:</label>
    <input type="text" id="pin" bind:value={pin} placeholder="6-character hex PIN" autocomplete="off">
  </div>

  <div class="form-group">
    <label for="displayName">Display Name:</label>
    <input type="text" id="displayName" bind:value={displayName} placeholder="Your name" autocomplete="off">
  </div>

  <div class="form-group">
    <label for="wavUrl">Wav URL:</label>
    <input type="text" id="wavUrl" bind:value={wavUrl} placeholder="wav file URL (leave empty for using microphone)" autocomplete="off">
  </div>

  <div class="form-group">
    <!-- svelte-ignore a11y-label-has-associated-control -->
    <label>Wav Example:</label>
    <div>http://localhost:8080/sounds/speech_mono.wav</div>
    <div>http://localhost:8080/sounds/bg_mono.wav</div>
  </div>

  <div>
    <button on:click={run} disabled={$connStatus !== 'disconnected'}>Join Room</button>
    {#if $connStatus === 'connecting'}
      <button on:click={disconnect}>Cancel</button>
    {/if}
    {#if $connStatus === 'connected'}
      <button on:click={disconnect}>Leave Room</button>
    {/if}

  </div>

  {#if $statusMessage}
    <div class="status {$statusType}">
      {$statusMessage}
    </div>
  {/if}

  {#if $currentStep && $currentStep !== 'unconnected' && $currentStep !== 'stopped'}
    <div class="current-step">
      <h3>Current Step:</h3>
      <div class="step-content">{$currentStep}</div>
    </div>
  {/if}

  {#if $members.length > 0}
    <div class="participants">
      <h3>Participants: {$members.length}</h3>
      <div class="participants-list">

        {#each $members as m}
          <div class="participant status-{m.status}">
            <span class="participant-id">{m.userId}</span>
            <span class="participant-status">{m.status || 'unknown'}</span>
          </div>
        {/each}
      </div>
    </div>
  {/if}

  {#if $showAudioCtrl}
    <div class="audio-controls">
      <label for="remoteAudio">Room Audio (Mixed):</label>
      <audio bind:this={client.remoteAudioEl} id="remoteAudio" controls autoplay style="width: 100%; margin-top: 10px;"></audio>
    </div>
  {/if}

  {#if wavUrl !== ""}
  <div class="audio-controls">
    <label for="remoteAudio">Local Wav:</label>
    <audio bind:this={client.playerEl} class="wavplayer" muted controls src="sounds/speech_mono.wav" crossorigin="anonymous"></audio>
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
    max-width: 600px;
    margin: 50px auto;
    padding: 20px;
  }

  button {
    padding: 10px 20px;
    background-color: #007bff;
    color: white;
    border: none;
    border-radius: 4px;
    cursor: pointer;
    margin-right: 10px;
  }

  button:hover {
    background-color: #0056b3;
  }

  button:disabled {
    background-color: #ccc;
    cursor: not-allowed;
  }

  .status {
    margin-top: 20px;
    padding: 10px;
    border-radius: 4px;
  }

  .status.info {
    background-color: #d1ecf1;
    color: #0c5460;
  }

  .status.success {
    background-color: #d4edda;
    color: #155724;
  }

  .status.error {
    background-color: #f8d7da;
    color: #721c24;
  }

  .current-step {
    margin: 20px 0;
    padding: 15px;
    background-color: #fff3cd;
    border-radius: 4px;
    border-left: 4px solid #ffc107;
  }

  .current-step h3 {
    margin-top: 0;
    margin-bottom: 10px;
    color: #856404;
  }

  .step-content {
    font-weight: 500;
    color: #856404;
    font-family: monospace;
  }

  #logs {
    margin-top: 20px;
  }

  #logs h3 {
    margin-bottom: 10px;
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

  .audio-controls {
    margin-top: 20px;
    padding: 15px;
    background-color: #f8f9fa;
    border-radius: 4px;
  }

  .participants {
    margin-top: 20px;
    padding: 15px;
    background-color: #e9ecef;
    border-radius: 4px;
  }

  .participant {
    padding: 8px 12px;
    margin: 5px 0;
    background-color: #fff;
    border-radius: 4px;
    border-left: 4px solid #ccc;
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .participant.status-onair {
    border-left-color: #28a745;
    background-color: #d4edda;
  }

  .participant.status-idle {
    border-left-color: #ffc107;
    background-color: #fff3cd;
  }

  .participant-id {
    font-weight: 500;
    color: #333;
  }

  .participant-status {
    font-size: 0.85em;
    padding: 2px 8px;
    border-radius: 3px;
    text-transform: uppercase;
    font-weight: 600;
  }

  .status-onair .participant-status {
    background-color: #28a745;
    color: white;
  }

  .status-idle .participant-status {
    background-color: #ffc107;
    color: #856404;
  }

  .wavplayer {
    display: block;
    width: 1px;
    height: 1px;
  }
</style>
