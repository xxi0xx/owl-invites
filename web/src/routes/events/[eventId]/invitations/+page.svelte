<script lang="ts">
	import { page } from '$app/stores';
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import type { InvitationDeliveryResult } from '$lib/api/generated';
	import Button from '$lib/components/ui/Button.svelte';
	import { datetimeLocalToUTC, utcToDatetimeLocal } from '$lib/utils/dates';
	import type { ApiError, Event, InvitationHousehold } from '$lib/types';

	let households = $state<InvitationHousehold[]>([]);
	let loading = $state(true);
	let error = $state('');
	let notice = $state('');
	let deliveryWarning = $state('');
	let label = $state('');
	let email = $state('');
	let phone = $state('');
	let delivery = $state<'email' | 'sms' | 'none'>('email');
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
	let rotatingId = $state('');
	let revokingId = $state('');
	let openSaving = $state(false);
	let openRotating = $state(false);
	let search = $state('');
	let responseFilter = $state('');
	let attendanceFilter = $state('');
	let editingId = $state('');
	let editLabel = $state('');
	let editEmail = $state('');
	let editPhone = $state('');
	let editDelivery = $state<'email' | 'sms' | 'none'>('email');
	let editAllowance = $state(0);
	let editAssigned = $state<Array<{ id?: string; name: string }>>([]);
	let updating = $state(false);

	const totalGuests = $derived(households.reduce((total, household) => total + household.guests.length, 0));
	const attendingGuests = $derived(households.reduce((total, household) => total + household.guests.filter((guest) => guest.attendance === 'attending').length, 0));
	const pendingGuests = $derived(households.reduce((total, household) => total + household.guests.filter((guest) => guest.attendance === 'pending').length, 0));
	const eventId = $derived($page.params.eventId);

	onMount(load);

	function listPath() {
		const query = new URLSearchParams();
		if (search.trim()) query.set('search', search.trim());
		if (responseFilter) query.set('response', responseFilter);
		if (attendanceFilter) query.set('attendance', attendanceFilter);
		const suffix = query.toString();
		return `/events/${eventId}/invitations${suffix ? `?${suffix}` : ''}`;
	}

	async function load() {
		loading = true;
		try {
			const [eventResponse, invitations, open] = await Promise.all([
				api.get<{ data: Event }>(`/events/${eventId}`),
				api.get<{ data: InvitationHousehold[] }>(listPath()),
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

	async function applyFilters(event: SubmitEvent) {
		event.preventDefault();
		error = '';
		await load();
	}

	async function clearFilters() {
		search = '';
		responseFilter = '';
		attendanceFilter = '';
		await load();
	}

	async function createInvitation(submitEvent: SubmitEvent) {
		submitEvent.preventDefault();
		saving = true;
		error = '';
		notice = '';
		deliveryWarning = '';
		try {
			const response = await api.post<{ data: { accessUrl: string; delivery: InvitationDeliveryResult } }>(`/events/${eventId}/invitations`, {
				label,
				contactEmail: email || undefined,
				contactPhone: phone || undefined,
				preferredDeliveryMethod: delivery,
				additionalGuestAllowance: Number(allowance),
				assignedGuestNames: names.split('\n').map((name) => name.trim()).filter(Boolean),
				send: delivery === 'none' ? false : send
			});
			lastAccessURL = response.data.accessUrl;
			deliveryWarning = response.data.delivery.warning ?? '';
			notice = response.data.delivery.status === 'sent'
				? 'Invitation created and accepted by the configured delivery provider.'
				: 'Invitation created. Delivery remains a separate outcome.';
			label = '';
			email = '';
			phone = '';
			names = '';
			allowance = 0;
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
				enabled: openEnabled, maxPartySize: Number(openMaxParty),
				capacity: openCapacity ? Number(openCapacity) : undefined,
				opensAt: openStartsAt ? datetimeLocalToUTC(openStartsAt, currentEvent.timezone) : undefined,
				closesAt: openClosesAt ? datetimeLocalToUTC(openClosesAt, currentEvent.timezone) : undefined
			});
			openURL = response.data.accessUrl;
			notice = 'Open enrollment settings saved.';
		} catch (caught) {
			error = (caught as ApiError).message ?? 'Unable to configure open enrollment.';
		} finally {
			openSaving = false;
		}
	}

	async function rotateOpen() {
		if (!window.confirm('Rotate the open-enrollment link? The previous public enrollment link will stop working.')) return;
		openRotating = true;
		error = '';
		try {
			const response = await api.post<{ data: { accessUrl: string } }>(`/events/${eventId}/open-enrollment/rotate`);
			openURL = response.data.accessUrl;
			notice = 'Open-enrollment capability rotated.';
		} catch (caught) {
			error = (caught as ApiError).message ?? 'Unable to rotate open enrollment link.';
		} finally {
			openRotating = false;
		}
	}

	function startEdit(household: InvitationHousehold) {
		editingId = household.invitation.id;
		editLabel = household.invitation.label;
		editEmail = household.invitation.contactEmail ?? '';
		editPhone = household.invitation.contactPhone ?? '';
		editDelivery = household.invitation.preferredDeliveryMethod;
		editAllowance = household.invitation.additionalGuestAllowance;
		editAssigned = household.guests.filter((guest) => guest.origin === 'assigned').map((guest) => ({ id: guest.id, name: guest.name }));
	}

	function addAssignedGuest() {
		editAssigned = [...editAssigned, { name: '' }];
	}

	function renameAssignedGuest(index: number, name: string) {
		editAssigned = editAssigned.map((guest, position) => position === index ? { ...guest, name } : guest);
	}

	function removeAssignedGuest(index: number) {
		if (editAssigned.length <= 1) return;
		editAssigned = editAssigned.filter((_, position) => position !== index);
	}

	async function saveEdit(event: SubmitEvent) {
		event.preventDefault();
		if (!editingId) return;
		updating = true;
		error = '';
		try {
			await api.put(`/events/${eventId}/invitations/${editingId}`, {
				label: editLabel,
				contactEmail: editEmail || undefined,
				contactPhone: editPhone || undefined,
				preferredDeliveryMethod: editDelivery,
				additionalGuestAllowance: Number(editAllowance),
				assignedGuests: editAssigned
			});
			editingId = '';
			notice = 'Household definition updated. No invitation was sent and the private capability was not rotated.';
			await load();
		} catch (caught) {
			error = (caught as ApiError).message ?? 'Unable to update the household.';
		} finally {
			updating = false;
		}
	}

	async function rotate(invitationId: string) {
		if (!window.confirm('Rotate this household capability? The old link and every current household session will stop working.')) return;
		rotatingId = invitationId;
		error = '';
		try {
			const response = await api.post<{ data: { accessUrl: string } }>(`/events/${eventId}/invitations/${invitationId}/rotate`);
			lastAccessURL = response.data.accessUrl;
			notice = 'Household capability rotated. Copy or deliver the new link.';
			await load();
		} catch (caught) {
			error = (caught as ApiError).message ?? 'Unable to rotate the household capability.';
		} finally {
			rotatingId = '';
		}
	}

	async function revoke(invitationId: string) {
		if (!window.confirm('Revoke this invitation? Household access will be disabled. This cannot be undone from this screen.')) return;
		revokingId = invitationId;
		error = '';
		try {
			await api.post(`/events/${eventId}/invitations/${invitationId}/revoke`, { reason: 'Revoked by organizer' });
			notice = 'Invitation revoked; household access is disabled.';
			await load();
		} catch (caught) {
			error = (caught as ApiError).message ?? 'Unable to revoke the invitation.';
		} finally {
			revokingId = '';
		}
	}

	async function deliver(invitationId: string) {
		deliveringId = invitationId;
		error = '';
		try {
			await api.post(`/events/${eventId}/invitations/${invitationId}/deliver`);
			notice = 'Invitation was accepted by the configured delivery provider.';
			await load();
		} catch (caught) {
			error = (caught as ApiError).message ?? 'Delivery failed. The invitation remains available and you can retry.';
			await load();
		} finally {
			deliveringId = '';
		}
	}

	function deliveryLabel(household: InvitationHousehold) {
		const delivery = household.latestDelivery;
		if (!delivery) return 'No recorded delivery attempt';
		if (delivery.status === 'failed') return 'Delivery failed';
		if (delivery.status === 'pending') return 'Delivery pending';
		if (delivery.deliveryStatus === 'unknown') return 'Accepted by provider';
		return delivery.deliveryStatus.charAt(0).toUpperCase() + delivery.deliveryStatus.slice(1);
	}

	async function exportResponses() {
		await api.download(`/events/${eventId}/invitations/export`, 'owl-invites-event-responses.csv');
	}
</script>

<svelte:head><title>Invitations · Owl Invites</title><meta name="referrer" content="no-referrer" /></svelte:head>

<main class="mx-auto max-w-6xl space-y-8 px-4 py-8 sm:px-6">
	<header class="flex flex-wrap items-center justify-between gap-4">
		<div><a href="/events" class="text-sm text-primary">← Back to events</a><h1 class="mt-2 text-3xl font-semibold">Invitations</h1>{#if currentEvent}<p class="text-sm font-medium text-neutral-700">{currentEvent.title}</p>{/if}<p class="mt-1 text-sm text-neutral-600">Each invitation is an isolated household and security boundary.</p></div>
		<div class="flex flex-wrap gap-2"><Button variant="outline" href="/events/{eventId}/edit">Edit event</Button><Button variant="outline" href="/events/{eventId}/invite">Invitation card</Button><Button variant="outline" href="/events/{eventId}/import">Import CSV</Button><Button type="button" variant="outline" onclick={exportResponses}>Export CSV</Button><Button variant="outline" href="/events/{eventId}/reminders">Reminders</Button><Button href="/events/{eventId}/messages">Message households</Button></div>
	</header>
	{#if error}<div role="alert" class="rounded-md bg-error-light p-4 text-sm text-error">{error}</div>{/if}
	{#if notice}<div role="status" class="rounded-md bg-success-light p-4 text-sm text-success">{notice}</div>{/if}
	{#if deliveryWarning}<div role="status" class="rounded-md border border-warning/30 bg-warning-light p-4 text-sm text-warning">{deliveryWarning}</div>{/if}
	{#if lastAccessURL}<div class="rounded-md border border-warning/30 bg-warning-light p-4"><p class="text-sm font-medium">Copy this private link now. Treat it as a credential.</p><code class="mt-2 block break-all text-xs">{lastAccessURL}</code></div>{/if}

	<div class="grid gap-8 lg:grid-cols-2">
		<form onsubmit={createInvitation} class="space-y-4 rounded-xl border border-neutral-200 bg-white p-5 shadow-sm sm:p-6">
			<h2 class="text-xl font-semibold">Create private invitation</h2>
			<label class="block space-y-1"><span class="text-sm font-medium">Household label</span><input bind:value={label} required class="block w-full rounded-md border border-neutral-300 px-3 py-2" /></label>
			<div class="grid gap-3 sm:grid-cols-3">
	<label class="text-sm">
		<span class="mb-1 block font-medium">Contact email</span>
		<input
			type="email"
			bind:value={email}
			required={delivery === 'email'}
			class="w-full rounded-md border border-neutral-300 px-3 py-2"
		/>
	</label>

	<label class="text-sm">
		<span class="mb-1 block font-medium">Contact phone</span>
		<input
			type="tel"
			bind:value={phone}
			required={delivery === 'sms'}
			placeholder="+15551234567"
			class="w-full rounded-md border border-neutral-300 px-3 py-2"
		/>
	</label>

	<label class="text-sm">
		<span class="mb-1 block font-medium">Preferred delivery</span>
		<select
			bind:value={delivery}
			class="w-full rounded-md border border-neutral-300 px-3 py-2"
		>
			<option value="email">Email</option>
			<option value="sms">SMS</option>
			<option value="none">Manual / none</option>
		</select>
	</label>
</div>
			<label class="block space-y-1"><span class="text-sm font-medium">Assigned guests (one per line)</span><textarea bind:value={names} required rows="4" class="block w-full rounded-md border border-neutral-300 px-3 py-2"></textarea></label>
			<label class="block space-y-1"><span class="text-sm font-medium">Additional guest allowance</span><input type="number" min="0" max="50" bind:value={allowance} class="block w-full rounded-md border border-neutral-300 px-3 py-2" /></label>
			<label class="flex gap-2 text-sm">
	<input
		type="checkbox"
		bind:checked={send}
		disabled={delivery === 'none'}
	/>
	Send invitation now
</label>
			<Button type="submit" loading={saving}>Create invitation</Button>
		</form>

		<section class="space-y-4 rounded-xl border border-neutral-200 bg-white p-5 shadow-sm sm:p-6">
			<h2 class="text-xl font-semibold">Open enrollment</h2>
			<label class="flex gap-2 text-sm"><input type="checkbox" bind:checked={openEnabled} /> Enabled</label>
			<label class="block space-y-1"><span class="text-sm font-medium">Maximum party size</span><input type="number" min="1" max="50" bind:value={openMaxParty} class="block w-full rounded-md border border-neutral-300 px-3 py-2" /></label>
			<label class="block space-y-1"><span class="text-sm font-medium">Open capacity (optional seats)</span><input type="number" min="1" bind:value={openCapacity} class="block w-full rounded-md border border-neutral-300 px-3 py-2" /></label>
			<div class="grid gap-3 sm:grid-cols-2"><label class="block space-y-1"><span class="text-sm font-medium">Opens (optional)</span><input type="datetime-local" bind:value={openStartsAt} class="block w-full rounded-md border border-neutral-300 px-3 py-2" /></label><label class="block space-y-1"><span class="text-sm font-medium">Closes (optional)</span><input type="datetime-local" bind:value={openClosesAt} class="block w-full rounded-md border border-neutral-300 px-3 py-2" /></label></div>
			<div class="flex flex-wrap gap-2"><Button onclick={configureOpen} loading={openSaving}>Save open enrollment</Button>{#if openURL}<Button variant="outline" onclick={rotateOpen} loading={openRotating}>Rotate link</Button>{/if}</div>
			{#if openURL}<div><p class="mb-2 text-xs font-medium text-warning">Public enrollment capability — rotate it to invalidate the previous URL.</p><code class="block break-all rounded bg-neutral-50 p-3 text-xs">{openURL}</code></div>{/if}
		</section>
	</div>

	<section class="space-y-4">
		<div class="flex flex-wrap items-end justify-between gap-3"><div><h2 class="text-xl font-semibold">Households</h2><p class="text-sm text-neutral-600">{households.length} results · {totalGuests} guests · {attendingGuests} attending · {pendingGuests} pending</p></div></div>
		<form onsubmit={applyFilters} class="grid gap-3 rounded-xl border border-neutral-200 bg-white p-4 sm:grid-cols-[minmax(220px,1fr)_180px_180px_auto]" aria-label="Search and filter households">
			<label class="text-sm"><span class="mb-1 block font-medium">Search</span><input bind:value={search} class="w-full rounded-md border border-neutral-300 px-3 py-2" placeholder="Household, contact, or guest" /></label>
			<label class="text-sm"><span class="mb-1 block font-medium">Response</span><select bind:value={responseFilter} class="w-full rounded-md border border-neutral-300 px-3 py-2"><option value="">All responses</option><option value="not_submitted">No submitted response</option><option value="submitted">Submitted</option></select></label>
			<label class="text-sm"><span class="mb-1 block font-medium">Attendance</span><select bind:value={attendanceFilter} class="w-full rounded-md border border-neutral-300 px-3 py-2"><option value="">Any attendance</option><option value="pending">Pending</option><option value="attending">Attending</option><option value="maybe">Maybe</option><option value="declined">Declined</option></select></label>
			<div class="flex items-end gap-2"><Button type="submit">Apply</Button><Button type="button" variant="ghost" onclick={clearFilters}>Clear</Button></div>
		</form>

		{#if loading}<p aria-live="polite">Loading households…</p>
		{:else if households.length === 0}<p class="rounded-xl border border-dashed border-neutral-300 p-8 text-center text-neutral-500">No households match these filters.</p>
		{:else}
			{#each households as household (household.invitation.id)}
				<article class="rounded-xl border border-neutral-200 bg-white p-5 shadow-sm">
					<div class="flex flex-wrap items-start justify-between gap-4">
						<div class="min-w-0 flex-1">
							<div class="flex flex-wrap items-center gap-2"><h3 class="font-semibold">{household.invitation.label}</h3>{#if household.response.submittedAt}<span class="rounded-full bg-success-light px-2 py-0.5 text-xs text-success">Submitted</span>{:else}<span class="rounded-full bg-neutral-100 px-2 py-0.5 text-xs text-neutral-600">No response</span>{/if}</div>
							<p class="break-all text-sm text-neutral-500">{household.invitation.contactEmail ?? household.invitation.contactPhone ?? 'Manual delivery'} · {household.invitation.source}</p>
							<ul class="mt-3 grid gap-2 sm:grid-cols-2">{#each household.guests as guest}<li class="rounded-md bg-neutral-50 px-3 py-2 text-sm"><span class="font-medium">{guest.name}</span><span class="ml-2 text-neutral-500">{guest.origin} · {guest.attendance}</span></li>{/each}</ul>
							<div class="mt-3 text-xs text-neutral-600"><span class="font-medium">Delivery:</span> {deliveryLabel(household)}{#if household.latestDelivery} · {household.latestDelivery.provider} · {new Date(household.latestDelivery.attemptedAt).toLocaleString()}{/if}</div>
							{#if household.latestDelivery?.status === 'failed' && household.latestDelivery.error}<p class="mt-1 text-xs text-error">{household.latestDelivery.error}</p>{/if}
						</div>
						<div class="flex flex-wrap gap-2">
							<Button size="sm" variant="outline" onclick={() => editingId === household.invitation.id ? (editingId = '') : startEdit(household)}>{editingId === household.invitation.id ? 'Close' : 'Edit'}</Button>
							{#if !household.invitation.revokedAt}<Button size="sm" onclick={() => deliver(household.invitation.id)} loading={deliveringId === household.invitation.id}>Deliver</Button><Button size="sm" variant="outline" onclick={() => rotate(household.invitation.id)} loading={rotatingId === household.invitation.id}>Rotate link</Button><Button size="sm" variant="danger" onclick={() => revoke(household.invitation.id)} loading={revokingId === household.invitation.id}>Revoke</Button>{:else}<span class="rounded-md bg-error-light px-3 py-1.5 text-sm text-error">Revoked</span>{/if}
						</div>
					</div>

					{#if editingId === household.invitation.id}
						<form onsubmit={saveEdit} class="mt-5 space-y-4 border-t border-neutral-200 pt-5" aria-label="Edit {household.invitation.label}">
							<div class="grid gap-3 sm:grid-cols-2"><label class="text-sm"><span class="mb-1 block font-medium">Household label</span><input bind:value={editLabel} required class="w-full rounded-md border border-neutral-300 px-3 py-2" /></label><label class="text-sm"><span class="mb-1 block font-medium">Additional guest allowance</span><input type="number" min="0" max="50" bind:value={editAllowance} class="w-full rounded-md border border-neutral-300 px-3 py-2" /></label></div>
							<div class="grid gap-3 sm:grid-cols-3"><label class="text-sm"><span class="mb-1 block font-medium">Contact email</span><input type="email" bind:value={editEmail} required={editDelivery === 'email'} class="w-full rounded-md border border-neutral-300 px-3 py-2" /></label><label class="text-sm"><span class="mb-1 block font-medium">Contact phone</span><input type="tel" bind:value={editPhone} required={editDelivery === 'sms'} class="w-full rounded-md border border-neutral-300 px-3 py-2" /></label><label class="text-sm"><span class="mb-1 block font-medium">Preferred delivery</span><select bind:value={editDelivery} class="w-full rounded-md border border-neutral-300 px-3 py-2"><option value="email">Email</option><option value="sms">SMS</option><option value="none">Manual / none</option></select></label></div>
							<div><div class="flex items-center justify-between"><h4 class="text-sm font-semibold">Organizer-assigned guests</h4><Button type="button" size="sm" variant="outline" onclick={addAssignedGuest}>Add assigned guest</Button></div><p class="mt-1 text-xs text-neutral-500">Removing an assigned guest hides them but preserves prior response history. Additional guests below remain household-managed.</p>
								<div class="mt-3 space-y-2">{#each editAssigned as guest, index (guest.id ?? `new-${index}`)}<div class="flex gap-2"><input aria-label="Assigned guest {index + 1} name" value={guest.name} oninput={(event) => renameAssignedGuest(index, event.currentTarget.value)} required class="min-w-0 flex-1 rounded-md border border-neutral-300 px-3 py-2" /><Button type="button" size="sm" variant="ghost" disabled={editAssigned.length <= 1} onclick={() => removeAssignedGuest(index)}>Remove</Button></div>{/each}</div>
							</div>
							{#if household.guests.some((guest) => guest.origin === 'additional')}<div><h4 class="text-sm font-semibold">Household-managed additional guests</h4><p class="mt-1 text-sm text-neutral-600">{household.guests.filter((guest) => guest.origin === 'additional').map((guest) => `${guest.name} (${guest.attendance})`).join(', ')}</p></div>{/if}
							<div class="flex gap-2"><Button type="submit" loading={updating}>Save household</Button><Button type="button" variant="ghost" onclick={() => (editingId = '')}>Cancel</Button></div>
						</form>
					{/if}
				</article>
			{/each}
		{/if}
	</section>
</main>
