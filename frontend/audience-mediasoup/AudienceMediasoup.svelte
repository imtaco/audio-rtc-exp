<script>
import * as mediasoupClient from "mediasoup-client";
import Signal from '../shared/ws-rpc.js';

let wsUrl = 'ws://localhost:8078';
let statusMessage = 'Idle';
let isConnected = false;
let audioEl;

async function connect() {
  if (isConnected) return;

  try {
    statusMessage = 'Connecting...';
    console.log('start');

    const sig = new Signal();
    sig.defExceptionHandler(console.log);

    const peer = await sig.connect(wsUrl);

    statusMessage = 'Getting RTP capabilities...';
    resp = await peer.request('getRtpCapabilities');
    const device = await loadDevice(resp.routerRtpCapabilities);

    statusMessage = 'Creating receive transport...';
    resp = await peer.request('createRecvTransport');
    const recvTransport = device.createRecvTransport({
      id: resp.id,
      iceParameters: resp.iceParameters,
      iceCandidates: resp.iceCandidates,
      dtlsParameters: resp.dtlsParameters
    });

    console.log('wait recvTransport connected');
    recvTransport.on("connect", async ({dtlsParameters, iceParameters}, callback) => {
      console.log("dtlsParameters", {dtlsParameters, iceParameters});
      await peer.request('connectRecvTransport', {
        transportId: recvTransport.id,
        iceParameters,
        dtlsParameters,
      });
      callback();
    });

    statusMessage = 'Starting to consume...';
    resp = await peer.request('startConsume', {
      rtpCapabilities: device.rtpCapabilities,
    });

    const consumer = await recvTransport.consume({
      id: resp.id,
      producerId: resp.producerId,
      kind: resp.kind,
      rtpParameters: resp.rtpParameters
    });

    const stream = new MediaStream();
    stream.addTrack(consumer.track);

    audioEl.srcObject = stream;
    await audioEl.play();

    statusMessage = 'Playing audio';
    isConnected = true;
    console.log('Audio element playing successfully');
  } catch (error) {
    console.error('Connection error:', error);
    statusMessage = `Error: ${error.message}`;
    isConnected = false;
  }
}

async function loadDevice(routerRtpCapabilities) {
  console.log("routerRtpCapabilities", routerRtpCapabilities);
  const device = new mediasoupClient.Device;
  await device.load({ routerRtpCapabilities });
  return device;
}
</script>

<main>
  <h2>Audio Listener</h2>

  <div class="form-group">
    <label for="wsUrl">Signaling WebSocket URL:</label>
    <input
      type="text"
      id="wsUrl"
      bind:value={wsUrl}
      size="50"
      disabled={isConnected}
      autocomplete="off"
    />
  </div>

  <button on:click={connect} disabled={isConnected}>Connect</button>

  <pre id="status">{statusMessage}</pre>

  <audio bind:this={audioEl} controls></audio>
</main>

<style>
  main {
    max-width: 800px;
    margin: 0 auto;
  }

  .form-group {
    margin-bottom: 15px;
  }

  label {
    display: block;
    margin-bottom: 5px;
  }

  input {
    width: 100%;
    max-width: 500px;
  }

  button {
    margin-bottom: 15px;
  }

  #status {
    margin: 15px 0;
  }
</style>
