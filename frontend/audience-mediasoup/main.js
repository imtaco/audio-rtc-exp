import { mount } from 'svelte';
import '../app.css';
import AudienceMediasoup from './AudienceMediasoup.svelte';

const app = mount(AudienceMediasoup, { target: document.getElementById('app') });

export default app;
