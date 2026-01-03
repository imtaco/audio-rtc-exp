import { mount } from 'svelte';
import '../app.css';
import Anchor from './Anchor.svelte';

const app = mount(Anchor, { target: document.getElementById('app') });

export default app;
