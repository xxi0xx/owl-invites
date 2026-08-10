<script lang="ts">
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { currentUser } from '$lib/stores/auth';
	import { toast } from '$lib/stores/toast';
	import { toISOLocal, datetimeLocalToUTC } from '$lib/utils/dates';
	import { getTimezoneOptions } from '$lib/utils/timezones';
	import type { Event } from '$lib/types';
	import AppShell from '$lib/components/layout/AppShell.svelte';
	import Button from '$lib/components/ui/Button.svelte';
	import Input from '$lib/components/ui/Input.svelte';
	import Textarea from '$lib/components/ui/Textarea.svelte';
	import DateTimePicker from '$lib/components/ui/DateTimePicker.svelte';
	import Select from '$lib/components/ui/Select.svelte';
	import Card from '$lib/components/ui/Card.svelte';

	const defaultTimezone = $currentUser?.timezone || Intl.DateTimeFormat().resolvedOptions().timeZone || '';
	const timezoneOptions = getTimezoneOptions(defaultTimezone);
	const minimumDate = toISOLocal(new Date());

	let submitting = $state(false);
	let title = $state('');
	let eventDate = $state('');
	let endDate = $state('');
	let location = $state('');
	let timezone = $state(defaultTimezone);
	let description = $state('');
	let showHeadcount = $state(false);
	let showGuestList = $state(false);
	let rsvpDeadline = $state('');
	let retentionDays = $state('30');
	let errors: Record<string, string> = $state({});

	function validate() {
		errors = {};
		if (!title.trim()) errors.title = 'Title is required';
		if (!eventDate) errors.eventDate = 'Event date is required';
		if (!timezone) errors.timezone = 'Timezone is required';
		const retention = Number(retentionDays);
		if (!Number.isInteger(retention) || retention < 1 || retention > 365) errors.retentionDays = 'Use a whole number from 1 to 365';
		return Object.keys(errors).length === 0;
	}

	async function createEvent(event: SubmitEvent) {
		event.preventDefault();
		if (!validate()) return;
		submitting = true;
		try {
			const body: Record<string, unknown> = {
				title: title.trim(),
				description: description.trim(),
				eventDate: datetimeLocalToUTC(eventDate, timezone),
				location: location.trim(),
				timezone,
				showHeadcount,
				showGuestList,
				retentionDays: Number(retentionDays)
			};
			if (endDate) body.endDate = datetimeLocalToUTC(endDate, timezone);
			if (rsvpDeadline) body.rsvpDeadline = datetimeLocalToUTC(rsvpDeadline, timezone);
			const response = await api.post<{ data: Event }>('/events', body);
			toast.success('Event created');
			goto(`/events/${response.data.id}/invitations`);
		} catch (caught) {
			toast.error((caught as { message?: string }).message || 'Failed to create event');
		} finally {
			submitting = false;
		}
	}
</script>

<svelte:head><title>Create event</title></svelte:head>

<AppShell>
	<div class="mx-auto max-w-3xl">
		<header class="mb-8"><a href="/events" class="text-sm text-primary hover:text-primary-hover">← Back to events</a><h1 class="mt-2 text-2xl font-bold font-display text-neutral-900">Create event</h1></header>
		<Card>
			<form onsubmit={createEvent} class="space-y-6">
				<Input label="Event title" name="title" bind:value={title} error={errors.title || ''} required />
				<div class="grid gap-4 sm:grid-cols-2">
					<DateTimePicker label="Event date" name="eventDate" bind:value={eventDate} min={minimumDate} error={errors.eventDate || ''} required />
					<DateTimePicker label="End date (optional)" name="endDate" bind:value={endDate} min={eventDate || minimumDate} />
				</div>
				<Input label="Location" name="location" bind:value={location} />
				<Select label="Timezone" name="timezone" bind:value={timezone} options={timezoneOptions} error={errors.timezone || ''} required />
				<Textarea label="Description" name="description" bind:value={description} rows={6} />
				<DateTimePicker label="Response deadline (optional)" name="rsvpDeadline" bind:value={rsvpDeadline} min={minimumDate} max={eventDate || undefined} />
				<fieldset class="space-y-2"><legend class="mb-2 text-sm font-medium text-neutral-700">Aggregate response visibility</legend>
					<label class="flex gap-3 text-sm"><input type="checkbox" bind:checked={showHeadcount} /> Show aggregate headcount</label>
					<label class="flex gap-3 text-sm"><input type="checkbox" bind:checked={showGuestList} /> Show guest names</label>
				</fieldset>
				<Input label="Data retention (days)" name="retentionDays" type="number" bind:value={retentionDays} error={errors.retentionDays || ''} />
				<div class="flex justify-end gap-3 border-t border-neutral-200 pt-5"><Button variant="outline" href="/events">Cancel</Button><Button type="submit" loading={submitting}>Create event</Button></div>
			</form>
		</Card>
	</div>
</AppShell>
