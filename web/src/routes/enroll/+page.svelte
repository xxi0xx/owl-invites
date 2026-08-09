<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import Button from '$lib/components/ui/Button.svelte';
	import type { ApiError, InvitationHousehold } from '$lib/types';

	let capability = $state('');
	let eventData = $state<{ title: string; eventDate: string; location: string } | null>(null);
	let maxPartySize = $state(1);
	let label = $state('');
	let email = $state('');
	let guestNames = $state(['']);
	let loading = $state(true);
	let saving = $state(false);
	let error = $state('');

	onMount(async () => {
		capability = window.location.hash.slice(1);
		history.replaceState({}, '', '/enroll');
		if (!capability) {
			error = 'This open invitation is unavailable.';
			loading = false;
			return;
		}
		try {
			const response = await api.post<{ data: { event: { title: string; eventDate: string; location: string }; maxPartySize: number } }>('/invitations/open/inspect', { capability });
			eventData = response.data.event;
			maxPartySize = response.data.maxPartySize;
		} catch {
			error = 'This open invitation is unavailable.';
		} finally {
			loading = false;
		}
	});

	function setGuest(index: number, value: string) {
		guestNames = guestNames.map((name, position) => position === index ? value : name);
	}

	async function enroll(event: SubmitEvent) {
		event.preventDefault();
		saving = true;
		error = '';
		try {
			await api.post<{ data: InvitationHousehold }>('/invitations/open/enroll', {
				capability,
				label,
				contactEmail: email,
				preferredDeliveryMethod: 'email',
				guestNames
			});
			await goto('/invitation', { replaceState: true });
		} catch (caught) {
			const apiError = caught as ApiError;
			error = apiError.message ?? 'Unable to enroll with this invitation.';
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head><title>{eventData?.title ?? 'Open invitation'}</title><meta name="referrer" content="no-referrer" /></svelte:head>

<main class="min-h-screen bg-neutral-50 py-10 px-4">
	{#if loading}<p class="text-center">Loading…</p>
	{:else if !eventData}<div class="mx-auto max-w-lg rounded-xl bg-white p-8 text-center shadow-sm"><h1 class="text-xl font-semibold">Open invitation unavailable</h1><p class="mt-3 text-sm text-neutral-600">{error}</p></div>
	{:else}
		<form onsubmit={enroll} class="mx-auto max-w-xl space-y-5 rounded-xl border border-neutral-200 bg-white p-8 shadow-sm">
			<header><p class="text-sm font-medium text-primary">Open invitation</p><h1 class="mt-1 text-3xl font-semibold">{eventData.title}</h1><p class="mt-2 text-neutral-600">{new Date(eventData.eventDate).toLocaleString()} · {eventData.location || 'Location to be announced'}</p></header>
			{#if error}<div class="rounded-md bg-error-light p-4 text-sm text-error">{error}</div>{/if}
			<label class="block space-y-1"><span class="text-sm font-medium">Invitation label</span><input bind:value={label} required class="block w-full rounded-md border border-neutral-300 px-3 py-2" placeholder="e.g. Morgan household" /></label>
			<label class="block space-y-1"><span class="text-sm font-medium">Email</span><input type="email" bind:value={email} required class="block w-full rounded-md border border-neutral-300 px-3 py-2" /></label>
			<div class="space-y-3"><div class="flex justify-between"><span class="text-sm font-medium">Guests</span><span class="text-xs text-neutral-500">Up to {maxPartySize}</span></div>
				{#each guestNames as name, index}<div class="flex gap-2"><input aria-label="Guest {index + 1} name" value={name} oninput={(event) => setGuest(index, event.currentTarget.value)} required class="flex-1 rounded-md border border-neutral-300 px-3 py-2" placeholder="Guest name" />{#if guestNames.length > 1}<button type="button" class="text-sm text-error" onclick={() => (guestNames = guestNames.filter((_, position) => position !== index))}>Remove</button>{/if}</div>{/each}
				{#if guestNames.length < maxPartySize}<Button type="button" variant="outline" onclick={() => (guestNames = [...guestNames, ''])}>Add guest</Button>{/if}
			</div>
			<Button type="submit" size="lg" loading={saving}>Create my invitation</Button>
		</form>
	{/if}
</main>

