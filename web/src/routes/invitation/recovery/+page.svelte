<script lang="ts">
	import { api } from '$lib/api/client';
	import Button from '$lib/components/ui/Button.svelte';
	import Input from '$lib/components/ui/Input.svelte';

	let eventId = $state('');
	let contact = $state('');
	let sending = $state(false);
	let sent = $state(false);

	async function requestRecovery(event: SubmitEvent) {
		event.preventDefault();
		sending = true;
		try {
			await api.post('/invitations/recovery/request', { eventId, contact });
		} finally {
			sending = false;
			sent = true;
		}
	}
</script>

<svelte:head><title>Recover invitation</title><meta name="referrer" content="no-referrer" /></svelte:head>

<main class="min-h-screen bg-neutral-50 flex items-center justify-center p-6">
	<form onsubmit={requestRecovery} class="w-full max-w-md space-y-5 rounded-xl border border-neutral-200 bg-white p-8 shadow-sm">
		<h1 class="text-2xl font-semibold text-neutral-900">Recover an invitation</h1>
		<p class="text-sm text-neutral-600">Enter the event reference and the email address or phone number the organizer stored.</p>
		{#if sent}
			<div class="rounded-md bg-success-light p-4 text-sm text-success">If a matching invitation exists, recovery instructions will be sent to its stored destination.</div>
		{:else}
			<Input name="event-reference" label="Event reference" bind:value={eventId} required />
			<Input name="recovery-contact" label="Email or phone" bind:value={contact} required />
			<Button type="submit" loading={sending}>Send recovery instructions</Button>
		{/if}
	</form>
</main>

