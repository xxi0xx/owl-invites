<script lang="ts">
	import { page } from '$app/stores';
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import Button from '$lib/components/ui/Button.svelte';
	import { datetimeLocalToUTC, utcToDatetimeLocal } from '$lib/utils/dates';
	import type { ApiError, Event, InvitationHousehold } from '$lib/types';

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
	let openStartsAt = $state('');
	let openClosesAt = $state('');
	let currentEvent: Event | null = $state(null);
	let deliveringId = $state('');
	let openSaving = $state(false);
	let openRotating = $state(false);

	const totalGuests = $derived(households.reduce((total, household) => total + household.guests.length, 0));
	const attendingGuests = $derived(households.reduce((total, household) => total + household.guests.filter((guest) => guest.attendance === 'attending').length, 0));
	const pendingGuests = $derived(households.reduce((total, household) => total + household.guests.filter((guest) => guest.attendance === 'pending').length, 0));

	const eventId = $derived($page.params.eventId);

	onMount(load);

	async function load() {
		loading = true;
		try {
			const [eventResponse, invitations, open] = await Promise.all([
				api.get<{ data: Event }>(`/events/${eventId}`),
				api.get<{ data: InvitationHousehold[] }>(`/events/${eventId}/invitations`),
				api.get<{ data: { config: { enabled: boolean; maxPartySize: number; capacity?: number; opensAt?: string; closesAt?: string }; accessUrl: string } | null }>(`/events/${eventId}/open-enrollment`)
			]);
			currentEvent = eventResponse.data;
			households = invitations.data;
			if (open.data) {
				openEnabled = open.data.config.enabled;
				openMaxParty = open.data.config.maxPartySize;
				openCapacity = open.data.config.capacity?.toString() ?? '';
				openURL = open.data.accessUrl;
				openStartsAt = open.data.config.opensAt ? utcToDatetimeLocal(open.data.config.opensAt, currentEvent.timezone) : '';
				openClosesAt = open.data.config.closesAt ? utcToDatetimeLocal(open.data.config.closesAt, currentEvent.timezone) : '';
			}
		} catch (caught) {
			error = (caught as ApiError).message ?? 'Unable to load invitations.';
		} finally {
			loading = false;
		}
	}

	async function createInvitation(submitEvent: SubmitEvent) {
		submitEvent.preventDefault();
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
		if (!currentEvent) return;
		openSaving = true;
		error = '';
		try {
			const response = await api.put<{ data: { accessUrl: string } }>(`/events/${eventId}/open-enrollment`, {
				enabled: openEnabled,
				maxPartySize: Number(openMaxParty),
				capacity: openCapacity ? Number(openCapacity) : undefined,
				opensAt: openStartsAt ? datetimeLocalToUTC(openStartsAt, currentEvent.timezone) : undefined,
				closesAt: openClosesAt ? datetimeLocalToUTC(openClosesAt, currentEvent.timezone) : undefined
			});
			openURL = response.data.accessUrl;
		} catch (caught) {
			error = (caught as ApiError).message ?? 'Unable to configure open enrollment.';
		} finally {
			openSaving = false;
		}
	}

	async function rotateOpen() {
		openRotating = true;
		error = '';
		try {
			const response = await api.post<{ data: { accessUrl: string } }>(`/events/${eventId}/open-enrollment/rotate`);
			openURL = response.data.accessUrl;
		} catch (caught) {
			error = (caught as ApiError).message ?? 'Unable to rotate open enrollment link.';
		} finally {
			openRotating = false;
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

	async function deliver(invitationId: string) {
		deliveringId = invitationId;
		error = '';
		try {
			await api.post(`/events/${eventId}/invitations/${invitationId}/deliver`);
		} catch (caught) {
			error = (caught as ApiError).message ?? 'Unable to deliver invitation.';
		} finally {
			deliveringId = '';
		}
	}
</script>

<svelte:head><title>Invitations</title><meta name="referrer" content="no-referrer" /></svelte:head>

<main class="mx-auto max-w-6xl space-y-8 px-6 py-8">
	<header class="flex flex-wrap items-center justify-between gap-4"><div><a href="/events" class="text-sm text-primary">← Back to events</a><h1 class="mt-2 text-3xl font-semibold">Invitations</h1>{#if currentEvent}<p class="text-sm font-medium text-neutral-700">{currentEvent.title}</p>{/if}<p class="mt-1 text-sm text-neutral-600">Each invitation is an isolated household and security boundary.</p></div><div class="flex gap-2"><Button variant="outline" href="/events/{eventId}/edit">Edit event</Button><Button href="/events/{eventId}/messages">Message households</Button></div></header>
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
			<div class="grid gap-3 sm:grid-cols-2"><label class="block space-y-1"><span class="text-sm font-medium">Opens (optional)</span><input type="datetime-local" bind:value={openStartsAt} class="block w-full rounded-md border border-neutral-300 px-3 py-2" /></label><label class="block space-y-1"><span class="text-sm font-medium">Closes (optional)</span><input type="datetime-local" bind:value={openClosesAt} class="block w-full rounded-md border border-neutral-300 px-3 py-2" /></label></div>
			<div class="flex gap-2"><Button onclick={configureOpen} loading={openSaving}>Save open enrollment</Button>{#if openURL}<Button variant="outline" onclick={rotateOpen} loading={openRotating}>Rotate link</Button>{/if}</div>
			{#if openURL}<div><p class="mb-2 text-xs font-medium text-warning">Public enrollment capability — rotate it to invalidate the previous URL.</p><code class="block break-all rounded bg-neutral-50 p-3 text-xs">{openURL}</code></div>{/if}
		</section>
	</div>

	<section class="space-y-4"><div class="flex flex-wrap items-end justify-between gap-3"><h2 class="text-xl font-semibold">Households</h2><p class="text-sm text-neutral-600">{households.length} households · {totalGuests} guests · {attendingGuests} attending · {pendingGuests} pending</p></div>
		{#if loading}<p>Loading…</p>{:else if households.length === 0}<p class="rounded-xl border border-dashed border-neutral-300 p-8 text-center text-neutral-500">No invitations yet.</p>
		{:else}{#each households as household (household.invitation.id)}<article class="rounded-xl border border-neutral-200 bg-white p-5 shadow-sm"><div class="flex flex-wrap items-start justify-between gap-4"><div><h3 class="font-semibold">{household.invitation.label}</h3><p class="text-sm text-neutral-500">{household.invitation.contactEmail ?? 'No email'} · {household.invitation.source}</p><p class="mt-2 text-sm">{household.guests.map((guest) => `${guest.name}: ${guest.attendance}`).join(' · ')}</p><p class="mt-2 text-xs text-neutral-500">Response version {household.response.version}{household.response.submittedAt ? ' · submitted' : ' · not submitted'}</p></div><div class="flex flex-wrap gap-2">{#if !household.invitation.revokedAt}<Button size="sm" onclick={() => deliver(household.invitation.id)} loading={deliveringId === household.invitation.id}>Send</Button><Button size="sm" variant="outline" onclick={() => rotate(household.invitation.id)}>Rotate link</Button><Button size="sm" variant="danger" onclick={() => revoke(household.invitation.id)}>Revoke</Button>{:else}<span class="text-sm text-error">Revoked</span>{/if}</div></div></article>{/each}{/if}
	</section>
</main>
