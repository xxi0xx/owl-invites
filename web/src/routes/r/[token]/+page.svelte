<script lang="ts">
	import { onMount } from 'svelte';

	onMount(() => {
		// A legacy RSVP token is not a Gate 2 invitation capability.
		history.replaceState(history.state, '', '/invitation/recovery');
	});
</script>

<svelte:head>
	<title>Legacy RSVP link unavailable</title>
	<meta name="referrer" content="no-referrer" />
</svelte:head>

<main class="flex min-h-screen items-center justify-center bg-neutral-50 p-6">
	<section class="w-full max-w-md rounded-xl border border-neutral-200 bg-white p-8 text-center shadow-sm">
		<h1 class="text-2xl font-semibold text-neutral-900">This RSVP link has been retired</h1>
		<p class="mt-3 text-sm text-neutral-600">
			Ask the organizer for a current private invitation link. If your contact information is already on an invitation, you can request a recovery link.
		</p>
		<a class="mt-6 inline-flex rounded-md bg-primary px-4 py-2 text-sm font-medium text-white hover:bg-primary-hover" href="/invitation/recovery">
			Recover an invitation
		</a>
	</section>
</main>
