<script lang="ts">
	import { page } from '$app/stores';
	import AppShell from '$lib/components/layout/AppShell.svelte';
</script>

<svelte:head><title>Invitation import unavailable</title></svelte:head>

<AppShell>
	<section class="mx-auto max-w-xl rounded-xl border border-neutral-200 bg-white p-8 shadow-sm">
		<h1 class="text-2xl font-semibold text-neutral-900">CSV import is temporarily unavailable</h1>
		<p class="mt-3 text-sm text-neutral-600">
			The legacy attendee importer was intentionally removed during the Gate 2 invitation cutover. Create isolated household invitations from the invitation dashboard while a capability-safe importer is designed.
		</p>
		<a class="mt-6 inline-flex rounded-md bg-primary px-4 py-2 text-sm font-medium text-white hover:bg-primary-hover" href="/events/{$page.params.eventId}/invitations">
			Open invitation dashboard
		</a>
	</section>
</AppShell>
