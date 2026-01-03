<script>
import { onDestroy } from 'svelte';
import Hls, { FetchLoader } from 'hls.js';

let urlInput = '';
let aesToken = '';
let statusMessage = '';
let audioEl;
let hls = null;

function log(msg) {
  statusMessage = msg;
  console.log(msg);
}

function loadStream() {
  if (!urlInput.trim()) {
    alert("Please enter a m3u8 URL");
    return;
  }

  const url = urlInput.trim();
  log("Loading: " + url);

  // Clean up existing HLS instance
  if (hls) {
    hls.destroy();
    hls = null;
  }

  if (Hls.isSupported()) {
    hls = new Hls({
      // enableWorker: true,
      lowLatencyMode: true,
      loader: FetchLoader,
      fetchSetup: (context, initParams) => {
        console.log('HLS fetchSetup', context.url);
        if (context.url.endsWith('/enc.key')) {
          initParams.headers = {
            ...(initParams.headers || {}),
            Authorization: `Bearer ${aesToken}`,
          };
        }
        return new Request(context.url, initParams);
      }
    });

    hls.loadSource(url);
    hls.attachMedia(audioEl);

    hls.on(window.Hls.Events.MANIFEST_PARSED, () => {
      audioEl.play();
      log("Playing (hls.js)");
    });

    hls.on(window.Hls.Events.ERROR, (event, data) => {
      log("HLS Error: " + data.details);
      console.error('HLS Error:', data);
    });
  } else {
    log("HLS not supported in this browser");
    alert("Your browser does not support HLS");
  }
}

onDestroy(() => {
  if (hls) {
    hls.destroy();
  }
});
</script>

<main>
  <h2>Audience</h2>

  <div class="form-group">
    <label for="m3u8">M3U8:</label>
    <input type="text" id="m3u8" bind:value={urlInput} placeholder="Enter audio m3u8 URL…" autocomplete="off">
  </div>

  <div class="form-group">
    <label for="token">Token:</label>
    <input type="text" id="token" bind:value={aesToken} placeholder="AES key token" autocomplete="off">
  </div>

  <div class="input-group">
    <button on:click={loadStream}>Play</button>
  </div>

  <audio bind:this={audioEl} controls></audio>

  {#if statusMessage}
    <div id="status">{statusMessage}</div>
  {/if}
</main>

<style>
  main {
    max-width: 800px;
    margin: 0 auto;
  }

  .input-group {
    display: flex;
    gap: 10px;
    margin-bottom: 20px;
  }

  audio {
    width: 100%;
    margin-top: 20px;
  }

  #status {
    margin-top: 10px;
    color: #666;
  }
</style>
