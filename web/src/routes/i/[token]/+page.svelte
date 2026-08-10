<script lang="ts">
	import { onMount } from 'svelte';

	onMount(() => {
		// Retire the legacy path token from the address bar without treating it as
		// an invitation credential. Gate 2 capabilities are accepted from URL
		// fragments only by /invitation/accept and /enroll.
		history.replaceState(history.state, '', '/invitation/recovery');
	});
</script>

<svelte:head>
	<title>Legacy invitation link unavailable</title>
	<meta name="referrer" content="no-referrer" />
</svelte:head>

<main class="flex min-h-screen items-center justify-center bg-neutral-50 p-6">
	<section class="w-full max-w-md rounded-xl border border-neutral-200 bg-white p-8 text-center shadow-sm">
		<h1 class="text-2xl font-semibold text-neutral-900">This invitation link has been retired</h1>
		<p class="mt-3 text-sm text-neutral-600">
			Owl Invites now uses private household invitations. Ask the organizer for a current link, or recover an invitation using the event reference and the contact address the organizer stored.
		</p>
		<a class="mt-6 inline-flex rounded-md bg-primary px-4 py-2 text-sm font-medium text-white hover:bg-primary-hover" href="/invitation/recovery">
			Recover an invitation
		</a>
	</section>
</main>
