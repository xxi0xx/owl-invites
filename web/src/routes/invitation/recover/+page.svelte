<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';

	let error = $state('');

	onMount(async () => {
		const capability = window.location.hash.slice(1);
		history.replaceState({}, '', '/invitation/recover');
		if (!capability) {
			error = 'This recovery link is invalid or expired.';
			return;
		}
		try {
			await api.post('/invitations/recovery/exchange', { capability });
			await goto('/invitation', { replaceState: true });
		} catch {
			error = 'This recovery link is invalid, expired, or already used.';
		}
	});
</script>

<svelte:head><title>Recover invitation</title><meta name="referrer" content="no-referrer" /></svelte:head>

<main class="min-h-screen bg-neutral-50 flex items-center justify-center p-6">
	<div class="max-w-md rounded-xl border border-neutral-200 bg-white p-8 text-center shadow-sm">
		{#if error}<h1 class="text-xl font-semibold">Recovery unavailable</h1><p class="mt-3 text-sm text-neutral-600">{error}</p>
		{:else}<div class="mx-auto h-8 w-8 animate-spin rounded-full border-2 border-primary border-t-transparent"></div><p class="mt-4">Recovering your invitation…</p>{/if}
	</div>
</main>

