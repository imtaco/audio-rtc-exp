import { mount } from 'svelte';
import '../app.css';
import RPC from './RPC.svelte';

const app = mount(RPC, { target: document.getElementById('app') });

export default app;
