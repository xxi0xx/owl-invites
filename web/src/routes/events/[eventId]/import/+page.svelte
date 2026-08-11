<script lang="ts">
	import { page } from '$app/state';
	import { api } from '$lib/api/client';
	import Button from '$lib/components/ui/Button.svelte';
	import type { ApiError, InvitationImportPreview } from '$lib/types';

	const eventId = $derived(page.params.eventId);
	let file = $state<File | null>(null);
	let preview = $state<InvitationImportPreview | null>(null);
	let previewing = $state(false);
	let committing = $state(false);
	let error = $state('');
	let committed = $state<{ householdCount: number; assignedGuestCount: number } | null>(null);

	function chooseFile(event: Event) {
		file = (event.currentTarget as HTMLInputElement).files?.[0] ?? null;
		preview = null;
		committed = null;
		error = '';
	}

	async function previewFile() {
		if (!file) return;
		previewing = true;
		error = '';
		try {
			const response = await api.uploadCSV<{ data: InvitationImportPreview }>(
				`/events/${eventId}/invitations/import/preview`, file
			);
			preview = response.data;
		} catch (caught) {
			error = (caught as ApiError).message ?? 'Unable to preview this CSV.';
		} finally {
			previewing = false;
		}
	}

	async function commitImport() {
		if (!preview || preview.errors.length > 0 || preview.households.length === 0) return;
		committing = true;
		error = '';
		try {
			const response = await api.post<{ data: { householdCount: number; assignedGuestCount: number } }>(
				`/events/${eventId}/invitations/import/commit`, { households: preview.households }
			);
			committed = response.data;
			preview = null;
			file = null;
		} catch (caught) {
			error = (caught as ApiError).message ?? 'The import could not be committed.';
		} finally {
			committing = false;
		}
	}

	async function downloadTemplate() {
		await api.download(`/events/${eventId}/invitations/import/template`, 'owl-invites-household-import-template.csv');
	}
</script>

<svelte:head><title>Import households · Owl Invites</title></svelte:head>

<main class="mx-auto max-w-5xl space-y-6 px-4 py-8 sm:px-6">
	<header>
		<a href="/events/{eventId}/invitations" class="text-sm text-primary hover:text-primary-hover">← Back to invitations</a>
		<h1 class="mt-2 text-3xl font-semibold text-neutral-900">Import household guest list</h1>
		<p class="mt-2 max-w-3xl text-sm text-neutral-600">
			Each row is one assigned guest. Only <code>household_key</code> groups rows; matching email addresses, phone numbers, and names never merge households.
		</p>
	</header>

	{#if error}<div role="alert" class="rounded-md bg-error-light p-4 text-sm text-error">{error}</div>{/if}
	{#if committed}
		<div role="status" class="rounded-md bg-success-light p-4 text-sm text-success">
			Created {committed.householdCount} households with {committed.assignedGuestCount} assigned guests. No invitations were sent.
		</div>
	{/if}

	<section class="rounded-xl border border-neutral-200 bg-white p-5 shadow-sm sm:p-7">
		<div class="flex flex-wrap items-start justify-between gap-4">
			<div>
				<h2 class="text-lg font-semibold">1. Choose a CSV</h2>
				<p class="mt-1 text-sm text-neutral-600">UTF-8, up to 512 KiB, 5,000 guest rows, 1,000 households, and 100 assigned guests per household.</p>
			</div>
			<Button type="button" variant="outline" onclick={downloadTemplate}>Download template</Button>
		</div>
		<label class="mt-5 block text-sm font-medium text-neutral-800" for="household-csv">Household CSV</label>
		<input id="household-csv" type="file" accept=".csv,text/csv" onchange={chooseFile} class="mt-2 block w-full rounded-md border border-neutral-300 p-3 text-sm" />
		<div class="mt-4"><Button type="button" onclick={previewFile} loading={previewing} disabled={!file}>Preview import</Button></div>
	</section>

	{#if preview}
		<section class="space-y-5 rounded-xl border border-neutral-200 bg-white p-5 shadow-sm sm:p-7" aria-labelledby="preview-heading">
			<div>
				<h2 id="preview-heading" class="text-lg font-semibold">2. Review normalized households</h2>
				<p class="mt-1 text-sm text-neutral-600">Preview created no invitations and sent no email.</p>
			</div>
			<div class="grid grid-cols-2 gap-3 sm:max-w-md">
				<div class="rounded-lg bg-neutral-50 p-4"><span class="block text-2xl font-semibold">{preview.householdCount}</span><span class="text-sm text-neutral-600">households</span></div>
				<div class="rounded-lg bg-neutral-50 p-4"><span class="block text-2xl font-semibold">{preview.assignedGuestCount}</span><span class="text-sm text-neutral-600">assigned guests</span></div>
			</div>

			{#if preview.errors.length > 0}
				<div role="alert" class="rounded-md border border-error/30 bg-error-light p-4 text-sm text-error">
					<h3 class="font-semibold">Fix these errors before importing</h3>
					<ul class="mt-2 list-disc space-y-1 pl-5">
						{#each preview.errors as issue}<li>{issue.row ? `Row ${issue.row}: ` : ''}{issue.field ? `${issue.field} — ` : ''}{issue.message}</li>{/each}
					</ul>
				</div>
			{/if}
			{#if preview.warnings.length > 0}
				<div class="rounded-md border border-warning/30 bg-warning-light p-4 text-sm text-warning">
					<h3 class="font-semibold">Warnings</h3>
					<ul class="mt-2 list-disc space-y-1 pl-5">{#each preview.warnings as issue}<li>{issue.message}</li>{/each}</ul>
				</div>
			{/if}

			{#if preview.households.length > 0}
				<div class="overflow-x-auto rounded-lg border border-neutral-200">
					<table class="min-w-full divide-y divide-neutral-200 text-left text-sm">
						<thead class="bg-neutral-50"><tr><th class="px-4 py-3">Grouping key</th><th class="px-4 py-3">Household</th><th class="px-4 py-3">Destination</th><th class="px-4 py-3">Assigned guests</th><th class="px-4 py-3">Additional allowance</th></tr></thead>
						<tbody class="divide-y divide-neutral-100">
							{#each preview.households as household}
								<tr><td class="px-4 py-3 font-mono text-xs">{household.householdKey}</td><td class="px-4 py-3 font-medium">{household.householdLabel}</td><td class="px-4 py-3">{household.contactEmail || household.contactPhone || 'Manual'} <span class="block text-xs text-neutral-500">{household.preferredDelivery}</span></td><td class="px-4 py-3">{household.assignedGuestNames.join(', ')}</td><td class="px-4 py-3">{household.additionalGuestAllowance}</td></tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}

			<div class="flex flex-wrap items-center gap-3">
				<Button type="button" onclick={commitImport} loading={committing} disabled={preview.errors.length > 0 || preview.households.length === 0}>Create households</Button>
				<span class="text-sm text-neutral-600">This creates new invitations only. Delivery remains a separate organizer action.</span>
			</div>
		</section>
	{/if}
</main>
