<script lang="ts">
  import { page } from '$app/stores';
  import { resolve } from '$app/paths';
</script>

<div class="min-h-screen flex items-center justify-center">
  <div class="text-center">
    <h1 class="text-4xl font-bold text-gray-800">{$page.status}</h1>
    <p class="text-gray-600 mt-2">{$page.error?.message ?? 'Something went wrong'}</p>
    <a href={resolve('/console')} class="mt-4 inline-block text-blue-600 hover:underline">← Back to app</a>
  </div>
</div>
