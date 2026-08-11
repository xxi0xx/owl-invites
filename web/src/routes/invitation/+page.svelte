<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import Button from '$lib/components/ui/Button.svelte';
	import InviteCardPreview from '$lib/components/invite/InviteCardPreview.svelte';
	import type { ApiError, InvitationAttendance, InvitationHousehold, InvitationQuestion } from '$lib/types';

	let household = $state<InvitationHousehold | null>(null);
	let loading = $state(true);
	let error = $state('');
	let deliveryWarning = $state('');
	let saving = $state(false);
	let saved = $state(false);
	let attendance = $state<Record<string, InvitationAttendance>>({});
	let additional = $state<Array<{ id?: string; name: string; attendance: InvitationAttendance }>>([]);
	let invitationAnswers = $state<Record<string, string>>({});
	let guestAnswers = $state<Record<string, Record<string, string>>>({});

	const attendanceOptions: Array<{ value: InvitationAttendance; label: string }> = [
		{ value: 'pending', label: 'Pending' },
		{ value: 'attending', label: 'Attending' },
		{ value: 'maybe', label: 'Maybe' },
		{ value: 'declined', label: 'Declined' }
	];

	onMount(() => {
		deliveryWarning = sessionStorage.getItem('owl_invitation_delivery_warning') ?? '';
		sessionStorage.removeItem('owl_invitation_delivery_warning');
		void load();
	});

	async function load() {
		loading = true;
		try {
			const response = await api.get<{ data: InvitationHousehold }>('/invitations/session');
			household = response.data;
			populate(response.data);
		} catch {
			error = 'Your invitation session is unavailable. Open the private link again or request recovery.';
		} finally {
			loading = false;
		}
	}

	function populate(value: InvitationHousehold) {
		attendance = Object.fromEntries(value.guests.map((guest) => [guest.id, guest.attendance]));
		additional = value.guests.filter((guest) => guest.origin === 'additional').map((guest) => ({
			id: guest.id, name: guest.name, attendance: guest.attendance
		}));
		invitationAnswers = Object.fromEntries(value.invitationAnswers.map((answer) => [answer.questionId, answer.answer]));
		const byGuest: Record<string, Record<string, string>> = {};
		for (const answer of value.guestAnswers) {
			(byGuest[answer.guestId] ??= {})[answer.questionId] = answer.answer;
		}
		guestAnswers = byGuest;
	}

	function addGuest() {
		if (!household || additional.length >= household.invitation.additionalGuestAllowance) return;
		additional = [...additional, { name: '', attendance: 'attending' }];
	}

	function removeGuest(index: number) {
		additional = additional.filter((_, position) => position !== index);
	}

	function setAdditionalName(index: number, value: string) {
		additional = additional.map((guest, position) => position === index ? { ...guest, name: value } : guest);
	}

	function setAdditionalAttendance(index: number, value: InvitationAttendance) {
		additional = additional.map((guest, position) => position === index ? { ...guest, attendance: value } : guest);
	}

	function answerQuestion(question: InvitationQuestion, value: string, guestId?: string) {
		if (question.scope === 'invitation') {
			invitationAnswers = { ...invitationAnswers, [question.id]: value };
		} else if (guestId) {
			guestAnswers = { ...guestAnswers, [guestId]: { ...(guestAnswers[guestId] ?? {}), [question.id]: value } };
		}
	}

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		if (!household) return;
		saving = true;
		error = '';
		try {
			const response = await api.put<{ data: InvitationHousehold }>('/invitations/session/response', {
				version: household.response.version,
				assignedGuests: household.guests.filter((guest) => guest.origin === 'assigned').map((guest) => ({
					guestId: guest.id, attendance: attendance[guest.id]
				})),
				additionalGuests: additional.map((guest) => ({ id: guest.id, name: guest.name, attendance: guest.attendance })),
				invitationAnswers,
				guestAnswers
			});
			household = response.data;
			populate(response.data);
			saved = true;
			setTimeout(() => (saved = false), 4000);
		} catch (caught) {
			const apiError = caught as ApiError;
			error = apiError.message ?? 'Unable to save the response.';
			if (apiError.status === 409) await load();
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head><title>{household?.event.title ?? 'Your invitation'}</title><meta name="referrer" content="no-referrer" /></svelte:head>

<main class="min-h-screen bg-neutral-50 py-10 px-4">
	{#if loading}<p class="text-center text-neutral-600">Loading invitation…</p>
	{:else if !household}<div class="mx-auto max-w-lg rounded-xl bg-white p-8 text-center shadow-sm"><h1 class="text-xl font-semibold">Invitation unavailable</h1><p class="mt-3 text-sm text-neutral-600">{error}</p><a class="mt-4 inline-block text-primary underline" href="/invitation/recovery">Request recovery</a></div>
	{:else}
		<form onsubmit={submit} class="mx-auto max-w-3xl space-y-6">
			<header class="space-y-4">
				<InviteCardPreview
					templateId={household.presentation.templateId}
					heading={household.presentation.heading}
					body={household.presentation.body}
					footer={household.presentation.footer}
					primaryColor={household.presentation.primaryColor}
					secondaryColor={household.presentation.secondaryColor}
					font={household.presentation.font}
					customData={JSON.stringify({ backgroundImage: household.presentation.backgroundImage })}
					eventTitle={household.event.title}
					eventDate={household.event.eventDate}
					eventLocation={household.event.location || 'Location to be announced'}
					timezone={household.event.timezone}
				/>
				<div class="rounded-xl border border-neutral-200 bg-white px-5 py-4 text-center shadow-sm">
					<p class="text-sm text-neutral-600">Responding for</p>
					<p class="font-semibold text-neutral-900">{household.invitation.label}</p>
				</div>
			</header>

			{#if error}<div class="rounded-md bg-error-light p-4 text-sm text-error">{error}</div>{/if}
			{#if deliveryWarning}<div class="rounded-md border border-warning/30 bg-warning-light p-4 text-sm text-warning">{deliveryWarning}</div>{/if}
			{#if saved}<div class="rounded-md bg-success-light p-4 text-sm text-success">Your response was saved.</div>{/if}

			<section class="rounded-xl border border-neutral-200 bg-white p-7 shadow-sm space-y-5">
				<h2 class="text-xl font-semibold">Guests</h2>
				{#each household.guests.filter((guest) => guest.origin === 'assigned') as guest (guest.id)}
					<div class="grid gap-3 rounded-md border border-neutral-200 p-4 md:grid-cols-[1fr_180px]">
						<strong>{guest.name}</strong>
						<select bind:value={attendance[guest.id]} class="rounded-md border border-neutral-300 px-3 py-2">
							{#each attendanceOptions as option}<option value={option.value}>{option.label}</option>{/each}
						</select>
					</div>
				{/each}

				{#each additional as guest, index (guest.id ?? `new-${index}`)}
					<div class="grid gap-3 rounded-md border border-neutral-200 p-4 md:grid-cols-[1fr_180px_auto]">
						<input aria-label="Additional guest name" value={guest.name} oninput={(event) => setAdditionalName(index, event.currentTarget.value)} class="rounded-md border border-neutral-300 px-3 py-2" placeholder="Additional guest name" required />
						<select value={guest.attendance} onchange={(event) => setAdditionalAttendance(index, event.currentTarget.value as InvitationAttendance)} class="rounded-md border border-neutral-300 px-3 py-2">
							{#each attendanceOptions as option}<option value={option.value}>{option.label}</option>{/each}
						</select>
						<button type="button" onclick={() => removeGuest(index)} class="text-sm text-error">Remove</button>
					</div>
				{/each}
				{#if additional.length < household.invitation.additionalGuestAllowance}<Button type="button" variant="outline" onclick={addGuest}>Add guest</Button>{/if}
			</section>

			{#if household.questions.length > 0}
				<section class="rounded-xl border border-neutral-200 bg-white p-7 shadow-sm space-y-6">
					<h2 class="text-xl font-semibold">Questions</h2>
					{#each household.questions.filter((question) => question.scope === 'invitation') as question (question.id)}
						<label class="block space-y-2"><span class="text-sm font-medium">{question.label}{question.required ? ' *' : ''}</span>
							{#if question.type === 'select'}<select value={invitationAnswers[question.id] ?? ''} onchange={(event) => answerQuestion(question, event.currentTarget.value)} required={question.required} class="block w-full rounded-md border border-neutral-300 px-3 py-2"><option value="">Select…</option>{#each question.options as option}<option value={option}>{option}</option>{/each}</select>
							{:else}<input value={invitationAnswers[question.id] ?? ''} oninput={(event) => answerQuestion(question, event.currentTarget.value)} required={question.required} class="block w-full rounded-md border border-neutral-300 px-3 py-2" />{/if}
						</label>
					{/each}
					{#each household.guests.filter((guest) => attendance[guest.id] === 'attending') as guest (guest.id)}
						{#each household.questions.filter((question) => question.scope === 'guest') as question (question.id)}
							<label class="block space-y-2"><span class="text-sm font-medium">{guest.name}: {question.label}{question.required ? ' *' : ''}</span>
								{#if question.type === 'select'}<select value={guestAnswers[guest.id]?.[question.id] ?? ''} onchange={(event) => answerQuestion(question, event.currentTarget.value, guest.id)} required={question.required} class="block w-full rounded-md border border-neutral-300 px-3 py-2"><option value="">Select…</option>{#each question.options as option}<option value={option}>{option}</option>{/each}</select>
								{:else}<input value={guestAnswers[guest.id]?.[question.id] ?? ''} oninput={(event) => answerQuestion(question, event.currentTarget.value, guest.id)} required={question.required} class="block w-full rounded-md border border-neutral-300 px-3 py-2" />{/if}
							</label>
						{/each}
					{/each}
				</section>
			{/if}
			<Button type="submit" size="lg" loading={saving}>Save response</Button>
		</form>
	{/if}
</main>
