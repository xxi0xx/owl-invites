<script lang="ts">
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import Button from '$lib/components/ui/Button.svelte';
	import { datetimeLocalToUTC, utcToDatetimeLocal } from '$lib/utils/dates';
	import type { ApiError, Event, Reminder } from '$lib/types';

	const eventId = $derived(page.params.eventId);
	let event = $state<Event | null>(null);
	let reminders = $state<Reminder[]>([]);
	let counts = $state<Record<string, number>>({});
	let loading = $state(true);
	let saving = $state(false);
	let error = $state('');
	let notice = $state('');
	let editingId = $state('');
	let remindAt = $state('');
	let targetGroup = $state('pending');
	let message = $state('');

	const groups = [
		{ value: 'all', label: 'All email-deliverable households' },
		{ value: 'pending', label: 'Households with pending guests' },
		{ value: 'attending', label: 'Households with attending guests' },
		{ value: 'maybe', label: 'Households with maybe guests' },
		{ value: 'declined', label: 'Households with declined guests' }
	];

	onMount(load);

	async function load() {
		loading = true;
		error = '';
		try {
			const [eventResponse, reminderResponse] = await Promise.all([
				api.get<{ data: Event }>(`/events/${eventId}`),
				api.get<{ data: Reminder[] }>(`/reminders/event/${eventId}`)
			]);
			event = eventResponse.data;
			reminders = reminderResponse.data;
			const results = await Promise.all(groups.map(async (group) => {
				const response = await api.post<{ data: { recipientHouseholds: number } }>(`/events/${eventId}/invitations/messages/preview`, { recipientGroup: group.value });
				return [group.value, response.data.recipientHouseholds] as const;
			}));
			counts = Object.fromEntries(results);
		} catch (caught) {
			error = (caught as ApiError).message ?? 'Unable to load reminders.';
		} finally {
			loading = false;
		}
	}

	function editReminder(reminder: Reminder) {
		if (!event || reminder.status !== 'scheduled') return;
		editingId = reminder.id;
		remindAt = utcToDatetimeLocal(reminder.remindAt, event.timezone);
		targetGroup = reminder.targetGroup;
		message = reminder.message;
	}

	function resetForm() {
		editingId = '';
		remindAt = '';
		targetGroup = 'pending';
		message = '';
	}

	async function saveReminder(submitEvent: SubmitEvent) {
		submitEvent.preventDefault();
		if (!event) return;
		saving = true;
		error = '';
		notice = '';
		try {
			const payload = {
				remindAt: datetimeLocalToUTC(remindAt, event.timezone),
				targetGroup,
				message: message.trim()
			};
			if (editingId) {
				await api.put(`/reminders/${editingId}`, payload);
				notice = 'Reminder updated.';
			} else {
				await api.post(`/reminders/event/${eventId}`, payload);
				notice = 'Reminder scheduled.';
			}
			resetForm();
			await load();
		} catch (caught) {
			error = (caught as ApiError).message ?? 'Unable to save reminder.';
		} finally {
			saving = false;
		}
	}

	async function cancelReminder(reminder: Reminder) {
		if (!window.confirm('Cancel this scheduled reminder? It will not be sent.')) return;
		error = '';
		try {
			await api.delete(`/reminders/${reminder.id}`);
			notice = 'Reminder cancelled.';
			await load();
		} catch (caught) {
			error = (caught as ApiError).message ?? 'Unable to cancel reminder.';
		}
	}

	function groupLabel(value: string) {
		return groups.find((group) => group.value === value)?.label ?? value;
	}
</script>

<svelte:head><title>Reminders · Owl Invites</title></svelte:head>

<main class="mx-auto max-w-5xl space-y-6 px-4 py-8 sm:px-6">
	<header><a href="/events/{eventId}/invitations" class="text-sm text-primary">← Back to invitations</a><h1 class="mt-2 text-3xl font-semibold">Invitation reminders</h1>{#if event}<p class="text-sm text-neutral-600">{event.title}</p>{/if}<p class="mt-2 max-w-3xl text-sm text-neutral-600">Reminders use the current invitation household and attendance model. Delivery is bounded and in-process; a process crash after claim may require operator action.</p></header>
	{#if error}<div role="alert" class="rounded-md bg-error-light p-4 text-sm text-error">{error}</div>{/if}
	{#if notice}<div role="status" class="rounded-md bg-success-light p-4 text-sm text-success">{notice}</div>{/if}

	<form onsubmit={saveReminder} class="space-y-4 rounded-xl border border-neutral-200 bg-white p-5 shadow-sm sm:p-6">
		<h2 class="text-xl font-semibold">{editingId ? 'Edit scheduled reminder' : 'Schedule reminder'}</h2>
		<div class="grid gap-4 sm:grid-cols-2"><label class="text-sm"><span class="mb-1 block font-medium">Run at</span><input type="datetime-local" bind:value={remindAt} required class="w-full rounded-md border border-neutral-300 px-3 py-2" /></label><label class="text-sm"><span class="mb-1 block font-medium">Target response group</span><select bind:value={targetGroup} class="w-full rounded-md border border-neutral-300 px-3 py-2">{#each groups as group}<option value={group.value}>{group.label}</option>{/each}</select></label></div>
		<label class="text-sm"><span class="mb-1 block font-medium">Message</span><textarea bind:value={message} rows="5" maxlength="10000" class="w-full rounded-md border border-neutral-300 px-3 py-2" placeholder="You have an upcoming event. Don't forget!"></textarea></label>
		<p class="text-sm text-neutral-600">Current eligible household count: <strong>{counts[targetGroup] ?? '—'}</strong>. The group is evaluated again when the reminder runs.</p>
		<div class="flex gap-2"><Button type="submit" loading={saving}>{editingId ? 'Save reminder' : 'Schedule reminder'}</Button>{#if editingId}<Button type="button" variant="ghost" onclick={resetForm}>Cancel edit</Button>{/if}</div>
	</form>

	<section class="space-y-4" aria-labelledby="scheduled-reminders-heading">
		<h2 id="scheduled-reminders-heading" class="text-xl font-semibold">Reminder status</h2>
		{#if loading}<p aria-live="polite">Loading reminders…</p>
		{:else if reminders.length === 0}<p class="rounded-xl border border-dashed border-neutral-300 p-8 text-center text-neutral-500">No reminders scheduled.</p>
		{:else}{#each reminders as reminder (reminder.id)}<article class="rounded-xl border border-neutral-200 bg-white p-5 shadow-sm"><div class="flex flex-wrap items-start justify-between gap-4"><div><h3 class="font-semibold">{new Date(reminder.remindAt).toLocaleString()}</h3><p class="mt-1 text-sm text-neutral-600">{groupLabel(reminder.targetGroup)} · currently {counts[reminder.targetGroup] ?? '—'} eligible households</p><p class="mt-2 whitespace-pre-wrap text-sm text-neutral-800">{reminder.message || "You have an upcoming event. Don't forget!"}</p><p class="mt-2 text-xs font-medium {reminder.status === 'failed' ? 'text-error' : 'text-neutral-500'}">Status: {reminder.status}</p></div>{#if reminder.status === 'scheduled'}<div class="flex gap-2"><Button size="sm" variant="outline" onclick={() => editReminder(reminder)}>Edit</Button><Button size="sm" variant="danger" onclick={() => cancelReminder(reminder)}>Cancel</Button></div>{/if}</div></article>{/each}{/if}
	</section>
</main>
