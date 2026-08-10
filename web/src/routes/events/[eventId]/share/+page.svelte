<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { onMount } from 'svelte';

	onMount(() => goto(`/events/${$page.params.eventId}/invitations`, { replaceState: true }));
</script>

<svelte:head><title>Opening invitations</title></svelte:head>

<main class="flex min-h-[50vh] items-center justify-center">
	<p class="text-sm text-neutral-500">Opening private and open-enrollment invitation links…</p>
</main>
