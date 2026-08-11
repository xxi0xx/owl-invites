<script lang="ts">
	import { page } from '$app/stores';
	import { api } from '$lib/api/client';
	import type { ApiError, Event } from '$lib/types';
	import AppShell from '$lib/components/layout/AppShell.svelte';
	import Button from '$lib/components/ui/Button.svelte';
	import Card from '$lib/components/ui/Card.svelte';
	import Spinner from '$lib/components/ui/Spinner.svelte';
	import { onMount } from 'svelte';

	const eventId = $derived($page.params.eventId);
	let loading = $state(true);
	let sending = $state(false);
	let previewing = $state(false);
	let event: Event | null = $state(null);
	let recipientGroup = $state('all');
	let recipientCount: number | null = $state(null);
	let subject = $state('');
	let body = $state('');
	let result = $state<{ attempted: number; accepted: number; failed: number; skipped: number } | null>(null);
	let error = $state('');
	let errors: Record<string, string> = $state({});

	const recipientOptions = [
		{ value: 'all', label: 'All email-deliverable households' },
		{ value: 'attending', label: 'Households with attending guests' },
		{ value: 'maybe', label: 'Households with maybe guests' },
		{ value: 'declined', label: 'Households with declined guests' },
		{ value: 'pending', label: 'Households with pending guests' }
	];
	const selectedGroupLabel = $derived(recipientOptions.find((option) => option.value === recipientGroup)?.label ?? recipientGroup);

	onMount(async () => {
		try {
			event = (await api.get<{ data: Event }>(`/events/${eventId}`)).data;
			await refreshPreview();
		} catch (caught) {
			error = (caught as ApiError).message || 'Failed to load event';
		} finally {
			loading = false;
		}
	});

	async function changeGroup(value: string) {
		recipientGroup = value;
		await refreshPreview();
	}

	async function refreshPreview() {
		previewing = true;
		error = '';
		try {
			const response = await api.post<{ data: { recipientHouseholds: number } }>(
				`/events/${eventId}/invitations/messages/preview`, { recipientGroup }
			);
			recipientCount = response.data.recipientHouseholds;
		} catch (caught) {
			recipientCount = null;
			error = (caught as ApiError).message || 'Unable to count recipient households.';
		} finally {
			previewing = false;
		}
	}

	async function sendMessage(submitEvent: SubmitEvent) {
		submitEvent.preventDefault();
		errors = {};
		error = '';
		if (!subject.trim()) errors.subject = 'Subject is required';
		if (!body.trim()) errors.body = 'Message body is required';
		if (Object.keys(errors).length) return;

		sending = true;
		result = null;
		try {
			const response = await api.post<{ data: { attempted: number; accepted: number; failed: number; skipped: number } }>(`/events/${eventId}/invitations/messages`, {
				recipientGroup, subject: subject.trim(), body: body.trim()
			});
			result = response.data;
			if (response.data.failed === 0) {
				subject = '';
				body = '';
			}
			await refreshPreview();
		} catch (caught) {
			error = (caught as ApiError).message || 'Failed to create or deliver the message.';
		} finally {
			sending = false;
		}
	}
</script>

<svelte:head><title>Message households · Owl Invites</title></svelte:head>

<AppShell>
	<div class="mx-auto max-w-5xl space-y-6">
		<header>
			<a href="/events/{eventId}/invitations" class="text-sm text-primary hover:text-primary-hover">← Back to invitations</a>
			<h1 class="mt-2 text-2xl font-bold font-display text-neutral-900">Message invitation households</h1>
			{#if event}<p class="text-sm text-neutral-500">{event.title}</p>{/if}
		</header>

		{#if error}<div role="alert" class="rounded-md bg-error-light p-4 text-sm text-error">{error}</div>{/if}
		{#if result}
			<div role="status" class="rounded-md {result.failed > 0 ? 'bg-warning-light text-warning' : 'bg-success-light text-success'} p-4 text-sm">
				Attempted {result.attempted}; accepted by provider {result.accepted}; failed {result.failed}; skipped {result.skipped}.
				{#if result.failed > 0}<span class="block mt-1">The message record remains saved. Review household delivery status and retry explicitly where needed.</span>{/if}
			</div>
		{/if}

		{#if loading}
			<div class="flex justify-center py-16"><Spinner size="lg" class="text-primary" /></div>
		{:else}
			<div class="grid gap-6 lg:grid-cols-[minmax(0,1fr)_320px]">
				<Card>
					<p class="mb-5 text-sm text-neutral-600">Messages are one-way organizer broadcasts. Each accepted email includes that household's current private invitation link. Guest chat is not enabled.</p>
					<form onsubmit={sendMessage} class="space-y-4">
						<label class="block text-sm font-medium text-neutral-700" for="recipient-group">Recipients</label>
						<select id="recipient-group" value={recipientGroup} onchange={(event) => changeGroup(event.currentTarget.value)} class="block w-full rounded-md border border-neutral-300 px-3 py-2 text-sm focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary">
							{#each recipientOptions as option}<option value={option.value}>{option.label}</option>{/each}
						</select>
						<label class="block text-sm font-medium text-neutral-700" for="message-subject">Subject</label>
						<input id="message-subject" bind:value={subject} required maxlength="200" aria-describedby={errors.subject ? 'subject-error' : undefined} class="block w-full rounded-md border border-neutral-300 px-3 py-2" />
						{#if errors.subject}<p id="subject-error" class="text-sm text-error">{errors.subject}</p>{/if}
						<label class="block text-sm font-medium text-neutral-700" for="message-body">Message</label>
						<textarea id="message-body" bind:value={body} required maxlength="10000" rows="8" aria-describedby={errors.body ? 'body-error' : undefined} class="block w-full rounded-md border border-neutral-300 px-3 py-2"></textarea>
						{#if errors.body}<p id="body-error" class="text-sm text-error">{errors.body}</p>{/if}
						<div class="flex justify-end"><Button type="submit" loading={sending} disabled={recipientCount === null || recipientCount === 0}>Send to {recipientCount ?? '—'} household{recipientCount === 1 ? '' : 's'}</Button></div>
					</form>
				</Card>

				<aside class="h-fit rounded-xl border border-neutral-200 bg-white p-5 shadow-sm" aria-labelledby="message-preview-heading">
					<h2 id="message-preview-heading" class="font-semibold">Before-send preview</h2>
					<dl class="mt-4 space-y-3 text-sm"><div><dt class="text-neutral-500">Target group</dt><dd class="font-medium text-neutral-900">{selectedGroupLabel}</dd></div><div><dt class="text-neutral-500">Recipient households</dt><dd class="font-medium text-neutral-900">{previewing ? 'Counting…' : (recipientCount ?? 'Unavailable')}</dd></div><div><dt class="text-neutral-500">Subject</dt><dd class="break-words font-medium text-neutral-900">{subject.trim() || 'No subject yet'}</dd></div><div><dt class="text-neutral-500">Message</dt><dd class="whitespace-pre-wrap break-words text-neutral-700">{body.trim() || 'No message yet'}</dd></div></dl>
					<p class="mt-4 text-xs text-neutral-500">Destinations are intentionally hidden. Counts include only active email-preferred households with a stored email destination.</p>
				</aside>
			</div>
		{/if}
	</div>
</AppShell>
