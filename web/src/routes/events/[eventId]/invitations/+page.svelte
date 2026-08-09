<script lang="ts">
	import { page } from '$app/stores';
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import Button from '$lib/components/ui/Button.svelte';
	import type { ApiError, InvitationHousehold } from '$lib/types';

	let households = $state<InvitationHousehold[]>([]);
	let loading = $state(true);
	let error = $state('');
	let label = $state('');
	let email = $state('');
	let names = $state('');
	let allowance = $state(0);
	let send = $state(true);
	let saving = $state(false);
	let lastAccessURL = $state('');
	let openEnabled = $state(false);
	let openMaxParty = $state(4);
	let openCapacity = $state('');
	let openURL = $state('');

	const eventId = $derived($page.params.eventId);

	onMount(load);

	async function load() {
		loading = true;
		try {
			const [invitations, open] = await Promise.all([
				api.get<{ data: InvitationHousehold[] }>(`/events/${eventId}/invitations`),
				api.get<{ data: { config: { enabled: boolean; maxPartySize: number; capacity?: number }; accessUrl: string } | null }>(`/events/${eventId}/open-enrollment`)
			]);
			households = invitations.data;
			if (open.data) {
				openEnabled = open.data.config.enabled;
				openMaxParty = open.data.config.maxPartySize;
				openCapacity = open.data.config.capacity?.toString() ?? '';
				openURL = open.data.accessUrl;
			}
		} catch (caught) {
			error = (caught as ApiError).message ?? 'Unable to load invitations.';
		} finally {
			loading = false;
		}
	}

	async function createInvitation(event: SubmitEvent) {
		event.preventDefault();
		saving = true;
		error = '';
		try {
			const response = await api.post<{ data: { accessUrl: string } }>(`/events/${eventId}/invitations`, {
				label,
				contactEmail: email,
				preferredDeliveryMethod: 'email',
				additionalGuestAllowance: Number(allowance),
				assignedGuestNames: names.split('\n').map((name) => name.trim()).filter(Boolean),
				send
			});
			lastAccessURL = response.data.accessUrl;
			label = ''; email = ''; names = ''; allowance = 0;
			await load();
		} catch (caught) {
			error = (caught as ApiError).message ?? 'Unable to create invitation.';
		} finally {
			saving = false;
		}
	}

	async function configureOpen() {
		try {
			const response = await api.put<{ data: { accessUrl: string } }>(`/events/${eventId}/open-enrollment`, {
				enabled: openEnabled,
				maxPartySize: Number(openMaxParty),
				capacity: openCapacity ? Number(openCapacity) : undefined
			});
			openURL = response.data.accessUrl;
		} catch (caught) {
			error = (caught as ApiError).message ?? 'Unable to configure open enrollment.';
		}
	}

	async function rotate(invitationId: string) {
		const response = await api.post<{ data: { accessUrl: string } }>(`/events/${eventId}/invitations/${invitationId}/rotate`);
		lastAccessURL = response.data.accessUrl;
		await load();
	}

	async function revoke(invitationId: string) {
		await api.post(`/events/${eventId}/invitations/${invitationId}/revoke`, { reason: 'Revoked by organizer' });
		await load();
	}
</script>

<svelte:head><title>Invitations</title></svelte:head>

<main class="mx-auto max-w-6xl space-y-8 px-6 py-8">
	<header class="flex items-center justify-between"><div><a href="/events/{eventId}" class="text-sm text-primary">← Back to event</a><h1 class="mt-2 text-3xl font-semibold">Invitations</h1><p class="mt-1 text-sm text-neutral-600">Each invitation is an isolated household and security boundary.</p></div></header>
	{#if error}<div class="rounded-md bg-error-light p-4 text-sm text-error">{error}</div>{/if}
	{#if lastAccessURL}<div class="rounded-md border border-warning/30 bg-warning-light p-4"><p class="text-sm font-medium">Copy this private link now. Treat it as a credential.</p><code class="mt-2 block break-all text-xs">{lastAccessURL}</code></div>{/if}

	<div class="grid gap-8 lg:grid-cols-2">
		<form onsubmit={createInvitation} class="space-y-4 rounded-xl border border-neutral-200 bg-white p-6 shadow-sm">
			<h2 class="text-xl font-semibold">Create private invitation</h2>
			<label class="block space-y-1"><span class="text-sm font-medium">Household label</span><input bind:value={label} required class="block w-full rounded-md border border-neutral-300 px-3 py-2" /></label>
			<label class="block space-y-1"><span class="text-sm font-medium">Delivery email</span><input type="email" bind:value={email} required class="block w-full rounded-md border border-neutral-300 px-3 py-2" /></label>
			<label class="block space-y-1"><span class="text-sm font-medium">Assigned guests (one per line)</span><textarea bind:value={names} required rows="4" class="block w-full rounded-md border border-neutral-300 px-3 py-2"></textarea></label>
			<label class="block space-y-1"><span class="text-sm font-medium">Additional guest allowance</span><input type="number" min="0" max="50" bind:value={allowance} class="block w-full rounded-md border border-neutral-300 px-3 py-2" /></label>
			<label class="flex gap-2 text-sm"><input type="checkbox" bind:checked={send} /> Send invitation now</label>
			<Button type="submit" loading={saving}>Create invitation</Button>
		</form>

		<section class="space-y-4 rounded-xl border border-neutral-200 bg-white p-6 shadow-sm">
			<h2 class="text-xl font-semibold">Open enrollment</h2>
			<label class="flex gap-2 text-sm"><input type="checkbox" bind:checked={openEnabled} /> Enabled</label>
			<label class="block space-y-1"><span class="text-sm font-medium">Maximum party size</span><input type="number" min="1" max="50" bind:value={openMaxParty} class="block w-full rounded-md border border-neutral-300 px-3 py-2" /></label>
			<label class="block space-y-1"><span class="text-sm font-medium">Open capacity (optional seats)</span><input type="number" min="1" bind:value={openCapacity} class="block w-full rounded-md border border-neutral-300 px-3 py-2" /></label>
			<Button onclick={configureOpen}>Save open enrollment</Button>
			{#if openURL}<code class="block break-all rounded bg-neutral-50 p-3 text-xs">{openURL}</code>{/if}
		</section>
	</div>

	<section class="space-y-4"><h2 class="text-xl font-semibold">Households</h2>
		{#if loading}<p>Loading…</p>{:else if households.length === 0}<p class="rounded-xl border border-dashed border-neutral-300 p-8 text-center text-neutral-500">No invitations yet.</p>
		{:else}{#each households as household (household.invitation.id)}<article class="rounded-xl border border-neutral-200 bg-white p-5 shadow-sm"><div class="flex flex-wrap items-start justify-between gap-4"><div><h3 class="font-semibold">{household.invitation.label}</h3><p class="text-sm text-neutral-500">{household.invitation.contactEmail ?? 'No email'} · {household.invitation.source}</p><p class="mt-2 text-sm">{household.guests.map((guest) => `${guest.name}: ${guest.attendance}`).join(' · ')}</p></div><div class="flex gap-2">{#if !household.invitation.revokedAt}<Button size="sm" variant="outline" onclick={() => rotate(household.invitation.id)}>Rotate link</Button><Button size="sm" variant="danger" onclick={() => revoke(household.invitation.id)}>Revoke</Button>{:else}<span class="text-sm text-error">Revoked</span>{/if}</div></div></article>{/each}{/if}
	</section>
</main>

