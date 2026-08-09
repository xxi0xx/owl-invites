<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { currentUser } from '$lib/stores/auth';
	import Button from '$lib/components/ui/Button.svelte';
	import Card from '$lib/components/ui/Card.svelte';
	import Spinner from '$lib/components/ui/Spinner.svelte';

	let acceptState: 'working' | 'error' = $state('working');
	let errorMessage = $state('');

	onMount(async () => {
		const params = new URLSearchParams(window.location.hash.slice(1));
		const token = params.get('token') || '';
		window.history.replaceState({}, '', '/auth/accept-invite');

		if (!token) {
			acceptState = 'error';
			errorMessage = 'This invitation link is incomplete.';
			return;
		}

		try {
			const response = await api.operation('acceptAccountInvite', { body: { token } });
			$currentUser = response.data.user;
			$currentUser = await api.operation('getCurrentUser');
			await goto('/events', { replaceState: true });
		} catch (error: unknown) {
			acceptState = 'error';
			errorMessage = (error as { message?: string }).message || 'This invitation is invalid or has expired.';
		}
	});
</script>

<svelte:head><title>Accept invitation — Owl Invites</title><meta name="robots" content="noindex" /></svelte:head>

<div class="flex min-h-screen items-center justify-center bg-neutral-50 px-4 py-12">
	<div class="w-full max-w-md">
		<div class="mb-6 text-center"><a href="/" class="font-display text-2xl font-bold text-primary">Owl Invites</a></div>
		<Card>
			<div class="space-y-4 py-8 text-center">
				{#if acceptState === 'working'}
					<Spinner size="lg" class="mx-auto text-primary" />
					<h1 class="font-display text-xl font-semibold text-neutral-900">Accepting your invitation</h1>
					<p class="text-sm text-neutral-500">This one-time link is being verified.</p>
				{:else}
					<h1 class="font-display text-xl font-semibold text-neutral-900">Invitation unavailable</h1>
					<p class="text-sm text-neutral-500">{errorMessage}</p>
					<div class="pt-2"><Button href="/auth/login">Go to sign in</Button></div>
				{/if}
			</div>
		</Card>
	</div>
</div>
