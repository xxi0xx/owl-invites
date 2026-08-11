<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import Spinner from '$lib/components/ui/Spinner.svelte';
	import { currentUser } from '$lib/stores/auth';

	onMount(async () => {
		let destination = '/auth/login';

		try {
			const status = await api.operation('getSetupStatus');
			if (status.setupRequired) {
				destination = '/setup';
			} else {
				try {
					$currentUser = await api.operation('getCurrentUser');
					destination = '/events';
				} catch {
					// A configured instance sends anonymous users to sign in.
				}
			}
		} catch {
			// The root is an application router, never a public marketing fallback.
		}

		await goto(destination, { replaceState: true });
	});
</script>

<svelte:head>
	<title>Owl Invites</title>
</svelte:head>

<div class="flex min-h-screen items-center justify-center bg-neutral-50" aria-label="Loading destination">
	<Spinner size="lg" class="text-primary" />
</div>
