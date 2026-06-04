<script lang="ts">
  import { onMount } from 'svelte';
  import { Get, Save } from '../wailsjs/go/service/BusinessProfileService';
  import { repository } from '../wailsjs/go/models';

  let name: string = '';
  let email: string = '';
  let status: string = '';

  onMount(async () => {
    const p = await Get();
    if (p) {
      name = p.name ?? '';
      email = p.email ?? '';
    }
  });

  async function save(): Promise<void> {
    try {
      const input = repository.BusinessProfileInput.createFrom({
        name,
        email,
        phone: '',
        address: '',
        logo: '',
        metadata: '',
        defaultCurrency: ''
      });
      await Save(input);
      status = 'Saved';
    } catch (e) {
      status = 'Error: ' + e;
    }
  }
</script>

<main style="padding:2rem;font-family:sans-serif;">
  <h1>Tallyo — Settings</h1>
  <label>Name <input bind:value={name} /></label><br/>
  <label>Email <input bind:value={email} /></label><br/>
  <button on:click={save}>Save</button>
  <p>{status}</p>
</main>
