<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import type { AuditEntry } from '$lib/api/generated';
	import { toast } from '$lib/stores/toast';
	import AdminNav from '$lib/components/admin/AdminNav.svelte';
	import AppShell from '$lib/components/layout/AppShell.svelte';
	import Spinner from '$lib/components/ui/Spinner.svelte';

	let entries: AuditEntry[] = $state([]);
	let loading = $state(true);

	onMount(async () => {
		try {
			const response = await api.operation('listAdminAudit', { parameters: { limit: 100 } });
			entries = response.data.entries;
		} catch (error: unknown) {
			toast.error((error as { message?: string }).message || 'Failed to load audit log');
		} finally {
			loading = false;
		}
	});

	function dateTime(value: string): string {
		return new Date(value).toLocaleString();
	}

	function label(action: string): string {
		return action.replaceAll('_', ' ');
	}
</script>

<svelte:head><title>Audit log — Owl Invites</title></svelte:head>

<AppShell>
	<div class="space-y-8">
		<div><h1 class="font-display text-2xl font-bold text-neutral-900">Audit log</h1><p class="mt-1 text-sm text-neutral-500">Security-sensitive instance and ownership changes.</p></div>
		<AdminNav active="audit" />
		{#if loading}
			<div class="flex justify-center py-16"><Spinner size="lg" /></div>
		{:else if entries.length === 0}
			<div class="rounded-lg border border-dashed border-neutral-300 bg-surface px-5 py-12 text-center text-sm text-neutral-500">No audit entries yet.</div>
		{:else}
			<div class="overflow-x-auto rounded-lg border border-neutral-200 bg-surface">
				<table class="min-w-full divide-y divide-neutral-200 text-sm">
					<thead class="bg-neutral-50 text-left text-xs uppercase tracking-wide text-neutral-500"><tr><th class="px-4 py-3">When</th><th class="px-4 py-3">Action</th><th class="px-4 py-3">Actor</th><th class="px-4 py-3">Target</th></tr></thead>
					<tbody class="divide-y divide-neutral-200">
						{#each entries as entry (entry.id)}
							<tr><td class="whitespace-nowrap px-4 py-3 text-neutral-500">{dateTime(entry.createdAt)}</td><td class="px-4 py-3 font-medium capitalize text-neutral-900">{label(entry.action)}</td><td class="px-4 py-3 text-neutral-600"><span class="capitalize">{entry.actorKind}</span>{#if entry.actorUserId}<div class="font-mono text-xs text-neutral-400">{entry.actorUserId}</div>{/if}</td><td class="px-4 py-3 text-neutral-600">{#if entry.targetUserId}<div>User <span class="font-mono text-xs">{entry.targetUserId}</span></div>{/if}{#if entry.eventId}<div>Event <span class="font-mono text-xs">{entry.eventId}</span></div>{/if}{#if !entry.targetUserId && !entry.eventId}<span class="text-neutral-400">Instance</span>{/if}</td></tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</div>
</AppShell>
