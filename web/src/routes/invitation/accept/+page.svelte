<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';

	let error = $state('');

	onMount(async () => {
		const capability = window.location.hash.slice(1);
		// Remove capability material from browser history before any further
		// navigation or third-party content can observe the current URL.
		history.replaceState({}, '', '/invitation/accept');
		if (!capability) {
			error = 'This invitation link is invalid or expired.';
			return;
		}
		try {
			await api.post('/invitations/exchange', { capability });
			await goto('/invitation', { replaceState: true });
		} catch {
			error = 'This invitation link is invalid or expired.';
		}
	});
</script>

<svelte:head><title>Opening invitation</title><meta name="referrer" content="no-referrer" /></svelte:head>

<main class="min-h-screen bg-neutral-50 flex items-center justify-center p-6">
	<div class="max-w-md rounded-xl border border-neutral-200 bg-white p-8 text-center shadow-sm">
		{#if error}
			<h1 class="text-xl font-semibold text-neutral-900">Invitation unavailable</h1>
			<p class="mt-3 text-sm text-neutral-600">{error}</p>
		{:else}
			<div class="mx-auto h-8 w-8 animate-spin rounded-full border-2 border-primary border-t-transparent"></div>
			<h1 class="mt-4 text-xl font-semibold text-neutral-900">Opening your invitation…</h1>
		{/if}
	</div>
</main>

