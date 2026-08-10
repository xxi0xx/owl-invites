<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { toast } from '$lib/stores/toast';
	import { datetimeLocalToUTC, utcToDatetimeLocal } from '$lib/utils/dates';
	import { getTimezoneOptions } from '$lib/utils/timezones';
	import type { Event } from '$lib/types';
	import AppShell from '$lib/components/layout/AppShell.svelte';
	import Button from '$lib/components/ui/Button.svelte';
	import Input from '$lib/components/ui/Input.svelte';
	import Textarea from '$lib/components/ui/Textarea.svelte';
	import DateTimePicker from '$lib/components/ui/DateTimePicker.svelte';
	import Select from '$lib/components/ui/Select.svelte';
	import Card from '$lib/components/ui/Card.svelte';
	import Spinner from '$lib/components/ui/Spinner.svelte';
	import QuestionBuilder from '$lib/components/questions/QuestionBuilder.svelte';
	import { onMount } from 'svelte';

	const eventId = $derived($page.params.eventId);
	let loading = $state(true);
	let saving = $state(false);
	let title = $state('');
	let eventDate = $state('');
	let endDate = $state('');
	let location = $state('');
	let timezone = $state('');
	let description = $state('');
	let showHeadcount = $state(false);
	let showGuestList = $state(false);
	let rsvpDeadline = $state('');
	let retentionDays = $state('30');
	let timezoneOptions = $state(getTimezoneOptions());
	let errors: Record<string, string> = $state({});

	onMount(async () => {
		try {
			const current = (await api.get<{ data: Event }>(`/events/${eventId}`)).data;
			title = current.title;
			timezone = current.timezone;
			timezoneOptions = getTimezoneOptions(current.timezone);
			eventDate = utcToDatetimeLocal(current.eventDate, current.timezone);
			endDate = current.endDate ? utcToDatetimeLocal(current.endDate, current.timezone) : '';
			location = current.location;
			description = current.description;
			showHeadcount = current.showHeadcount;
			showGuestList = current.showGuestList;
			rsvpDeadline = current.rsvpDeadline ? utcToDatetimeLocal(current.rsvpDeadline, current.timezone) : '';
			retentionDays = String(current.retentionDays);
		} catch (caught) {
			toast.error((caught as { message?: string }).message || 'Failed to load event');
		} finally {
			loading = false;
		}
	});

	function validate() {
		errors = {};
		if (!title.trim()) errors.title = 'Title is required';
		if (!eventDate) errors.eventDate = 'Event date is required';
		if (!timezone) errors.timezone = 'Timezone is required';
		const retention = Number(retentionDays);
		if (!Number.isInteger(retention) || retention < 1 || retention > 365) errors.retentionDays = 'Use a whole number from 1 to 365';
		return Object.keys(errors).length === 0;
	}

	async function save(event: SubmitEvent) {
		event.preventDefault();
		if (!validate()) return;
		saving = true;
		try {
			const body: Record<string, unknown> = {
				title: title.trim(), description: description.trim(), location: location.trim(), timezone,
				eventDate: datetimeLocalToUTC(eventDate, timezone), showHeadcount, showGuestList,
				retentionDays: Number(retentionDays)
			};
			if (endDate) body.endDate = datetimeLocalToUTC(endDate, timezone);
			if (rsvpDeadline) body.rsvpDeadline = datetimeLocalToUTC(rsvpDeadline, timezone);
			await api.put(`/events/${eventId}`, body);
			toast.success('Event updated');
			goto(`/events/${eventId}/invitations`);
		} catch (caught) {
			toast.error((caught as { message?: string }).message || 'Failed to update event');
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head><title>Edit event</title></svelte:head>

<AppShell>
	<div class="mx-auto max-w-3xl">
		<header class="mb-8"><a href="/events/{eventId}/invitations" class="text-sm text-primary">← Back to invitations</a><h1 class="mt-2 text-2xl font-bold font-display text-neutral-900">Edit event</h1></header>
		{#if loading}<div class="flex justify-center py-16"><Spinner size="lg" class="text-primary" /></div>{:else}
			<Card><form onsubmit={save} class="space-y-6">
				<Input label="Event title" name="title" bind:value={title} error={errors.title || ''} required />
				<div class="grid gap-4 sm:grid-cols-2"><DateTimePicker label="Event date" name="eventDate" bind:value={eventDate} error={errors.eventDate || ''} required /><DateTimePicker label="End date (optional)" name="endDate" bind:value={endDate} min={eventDate} /></div>
				<Input label="Location" name="location" bind:value={location} />
				<Select label="Timezone" name="timezone" bind:value={timezone} options={timezoneOptions} error={errors.timezone || ''} required />
				<Textarea label="Description" name="description" bind:value={description} rows={6} />
				<DateTimePicker label="Response deadline (optional)" name="rsvpDeadline" bind:value={rsvpDeadline} max={eventDate || undefined} />
				<fieldset class="space-y-2"><legend class="mb-2 text-sm font-medium text-neutral-700">Aggregate response visibility</legend><label class="flex gap-3 text-sm"><input type="checkbox" bind:checked={showHeadcount} /> Show aggregate headcount</label><label class="flex gap-3 text-sm"><input type="checkbox" bind:checked={showGuestList} /> Show guest names</label></fieldset>
				<Input label="Data retention (days)" name="retentionDays" type="number" bind:value={retentionDays} error={errors.retentionDays || ''} />
				<div class="flex justify-end gap-3 border-t border-neutral-200 pt-5"><Button variant="outline" href="/events/{eventId}/invitations">Cancel</Button><Button type="submit" loading={saving}>Save changes</Button></div>
			</form></Card>
			<Card class="mt-6"><QuestionBuilder eventId={eventId ?? ''} /></Card>
		{/if}
	</div>
</AppShell>
