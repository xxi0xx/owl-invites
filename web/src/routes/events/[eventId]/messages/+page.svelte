<script lang="ts">
	import { page } from '$app/stores';
	import { api } from '$lib/api/client';
	import { toast } from '$lib/stores/toast';
	import type { Event } from '$lib/types';
	import AppShell from '$lib/components/layout/AppShell.svelte';
	import Button from '$lib/components/ui/Button.svelte';
	import Input from '$lib/components/ui/Input.svelte';
	import Textarea from '$lib/components/ui/Textarea.svelte';
	import Select from '$lib/components/ui/Select.svelte';
	import Card from '$lib/components/ui/Card.svelte';
	import Spinner from '$lib/components/ui/Spinner.svelte';
	import { onMount } from 'svelte';

	const eventId = $derived($page.params.eventId);
	let loading = $state(true);
	let sending = $state(false);
	let event: Event | null = $state(null);
	let recipientGroup = $state('all');
	let subject = $state('');
	let body = $state('');
	let sentCount: number | null = $state(null);
	let errors: Record<string, string> = $state({});

	const recipientOptions = [
		{ value: 'all', label: 'All active private invitations' },
		{ value: 'attending', label: 'Households with attending guests' },
		{ value: 'maybe', label: 'Households with maybe guests' },
		{ value: 'declined', label: 'Households with declined guests' },
		{ value: 'pending', label: 'Households still pending' }
	];

	onMount(async () => {
		try {
			event = (await api.get<{ data: Event }>(`/events/${eventId}`)).data;
		} catch (caught) {
			toast.error((caught as { message?: string }).message || 'Failed to load event');
		} finally {
			loading = false;
		}
	});

	async function sendMessage(event: SubmitEvent) {
		event.preventDefault();
		errors = {};
		if (!subject.trim()) errors.subject = 'Subject is required';
		if (!body.trim()) errors.body = 'Message body is required';
		if (Object.keys(errors).length) return;

		sending = true;
		sentCount = null;
		try {
			const response = await api.post<{ data: { sent: number } }>(`/events/${eventId}/invitations/messages`, {
				recipientGroup,
				subject: subject.trim(),
				body: body.trim()
			});
			sentCount = response.data.sent;
			subject = '';
			body = '';
			toast.success(`Message sent to ${response.data.sent} invitation${response.data.sent === 1 ? '' : 's'}`);
		} catch (caught) {
			toast.error((caught as { message?: string }).message || 'Failed to send message');
		} finally {
			sending = false;
		}
	}
</script>

<svelte:head><title>Invitation messages</title></svelte:head>

<AppShell>
	<div class="mx-auto max-w-3xl">
		<header class="mb-6">
			<a href="/events/{eventId}/invitations" class="text-sm text-primary hover:text-primary-hover">← Back to invitations</a>
			<h1 class="mt-2 text-2xl font-bold font-display text-neutral-900">Message invitation households</h1>
			{#if event}<p class="text-sm text-neutral-500">{event.title}</p>{/if}
		</header>

		{#if loading}
			<div class="flex justify-center py-16"><Spinner size="lg" class="text-primary" /></div>
		{:else}
			<Card>
				<p class="mb-5 text-sm text-neutral-600">
					Messages are one-way organizer broadcasts. Each email includes that household's current private invitation link; guest-to-organizer threads are not part of Gate 2.
				</p>
				{#if sentCount !== null}
					<div class="mb-5 rounded-md bg-success-light p-4 text-sm text-success">Delivered to {sentCount} invitation{sentCount === 1 ? '' : 's'}.</div>
				{/if}
				<form onsubmit={sendMessage} class="space-y-4">
					<Select label="Recipients" name="recipientGroup" bind:value={recipientGroup} options={recipientOptions} />
					<Input label="Subject" name="subject" bind:value={subject} error={errors.subject || ''} required />
					<Textarea label="Message" name="body" bind:value={body} rows={6} error={errors.body || ''} required />
					<div class="flex justify-end"><Button type="submit" loading={sending}>Send message</Button></div>
				</form>
			</Card>
		{/if}
	</div>
</AppShell>
