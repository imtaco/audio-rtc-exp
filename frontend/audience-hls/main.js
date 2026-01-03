import { mount } from 'svelte';
import '../app.css';
import Audience from './Audience.svelte';

const app = mount(Audience, { target: document.getElementById('app') });

export default app;
