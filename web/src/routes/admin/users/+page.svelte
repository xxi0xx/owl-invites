<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import type { AccountInvite, User } from '$lib/api/generated';
	import { currentUser } from '$lib/stores/auth';
	import { toast } from '$lib/stores/toast';
	import AdminNav from '$lib/components/admin/AdminNav.svelte';
	import AppShell from '$lib/components/layout/AppShell.svelte';
	import Badge from '$lib/components/ui/Badge.svelte';
	import Button from '$lib/components/ui/Button.svelte';
	import Card from '$lib/components/ui/Card.svelte';
	import Input from '$lib/components/ui/Input.svelte';
	import Spinner from '$lib/components/ui/Spinner.svelte';

	let users: User[] = $state([]);
	let invites: AccountInvite[] = $state([]);
	let loading = $state(true);
	let inviteEmail = $state('');
	let inviting = $state(false);
	let changingId = $state('');

	onMount(load);

	async function load() {
		loading = true;
		try {
			const [userResponse, inviteResponse] = await Promise.all([
				api.operation('listUsers'),
				api.operation('listAccountInvites')
			]);
			users = userResponse.data.users;
			invites = inviteResponse.data.invites;
		} catch (error: unknown) {
			toast.error(messageFor(error, 'Failed to load users'));
		} finally {
			loading = false;
		}
	}

	function messageFor(error: unknown, fallback: string): string {
		return (error as { message?: string })?.message || fallback;
	}

	async function invite(event: SubmitEvent) {
		event.preventDefault();
		if (!inviteEmail.trim()) return;
		inviting = true;
		try {
			await api.operation('inviteUser', { body: { email: inviteEmail.trim().toLowerCase() } });
			inviteEmail = '';
			toast.success('Account invitation sent');
			await load();
		} catch (error: unknown) {
			toast.error(messageFor(error, 'Failed to send invitation'));
		} finally {
			inviting = false;
		}
	}

	async function revoke(inviteId: string) {
		changingId = inviteId;
		try {
			await api.operation('revokeAccountInvite', { parameters: { inviteId } });
			invites = invites.filter((item) => item.id !== inviteId);
			toast.success('Invitation revoked');
		} catch (error: unknown) {
			toast.error(messageFor(error, 'Failed to revoke invitation'));
		} finally {
			changingId = '';
		}
	}

	async function changeStatus(user: User) {
		const status = user.status === 'disabled' ? 'active' : 'disabled';
		changingId = user.id;
		try {
			await api.operation('updateUserStatus', { parameters: { userId: user.id }, body: { status } });
			toast.success(status === 'active' ? 'User enabled' : 'User disabled');
			await load();
		} catch (error: unknown) {
			toast.error(messageFor(error, 'Failed to change user status'));
		} finally {
			changingId = '';
		}
	}

	async function changeRole(user: User) {
		const instanceRole = user.instanceRole === 'admin' ? 'user' : 'admin';
		changingId = user.id;
		try {
			await api.operation('updateUserRole', {
				parameters: { userId: user.id },
				body: { instanceRole }
			});
			toast.success(instanceRole === 'admin' ? 'Administrator access granted' : 'Administrator access removed');
			await load();
		} catch (error: unknown) {
			toast.error(messageFor(error, 'Failed to change user role'));
		} finally {
			changingId = '';
		}
	}

	function statusVariant(status: User['status']): 'success' | 'warning' | 'error' {
		if (status === 'active') return 'success';
		if (status === 'disabled') return 'error';
		return 'warning';
	}

	function dateTime(value: string): string {
		return new Date(value).toLocaleString();
	}
</script>

<svelte:head><title>Users &amp; invites — Owl Invites</title></svelte:head>

<AppShell>
	<div class="space-y-8">
		<div>
			<h1 class="font-display text-2xl font-bold text-neutral-900">Users &amp; invites</h1>
			<p class="mt-1 text-sm text-neutral-500">Control instance access independently from event ownership.</p>
		</div>
		<AdminNav active="users" />

		<Card>
			<form onsubmit={invite} class="flex flex-col gap-3 sm:flex-row sm:items-end">
				<Input class="flex-1" label="Invite by email" name="inviteEmail" type="email" bind:value={inviteEmail} placeholder="person@example.com" required />
				<Button type="submit" loading={inviting}>Send invitation</Button>
			</form>
			<p class="mt-3 text-xs text-neutral-500">The one-time acceptance link is sent by email; its secret is never returned to this screen.</p>
		</Card>

		{#if loading}
			<div class="flex justify-center py-16"><Spinner size="lg" /></div>
		{:else}
			<section class="space-y-3" aria-labelledby="pending-heading">
				<div class="flex items-end justify-between">
					<div>
						<h2 id="pending-heading" class="font-display text-lg font-semibold text-neutral-900">Pending invitations</h2>
						<p class="text-sm text-neutral-500">{invites.length} awaiting acceptance</p>
					</div>
				</div>
				{#if invites.length === 0}
					<div class="rounded-lg border border-dashed border-neutral-300 bg-surface px-5 py-8 text-center text-sm text-neutral-500">No pending invitations.</div>
				{:else}
					<div class="overflow-x-auto rounded-lg border border-neutral-200 bg-surface">
						<table class="min-w-full divide-y divide-neutral-200 text-sm">
							<thead class="bg-neutral-50 text-left text-xs uppercase tracking-wide text-neutral-500"><tr><th class="px-4 py-3">Email</th><th class="px-4 py-3">Expires</th><th class="px-4 py-3"><span class="sr-only">Actions</span></th></tr></thead>
							<tbody class="divide-y divide-neutral-200">
								{#each invites as item (item.id)}
									<tr><td class="px-4 py-3 font-medium text-neutral-900">{item.email}</td><td class="px-4 py-3 text-neutral-500">{dateTime(item.expiresAt)}</td><td class="px-4 py-3 text-right"><Button variant="ghost" size="sm" loading={changingId === item.id} onclick={() => revoke(item.id)}>Revoke</Button></td></tr>
								{/each}
							</tbody>
						</table>
					</div>
				{/if}
			</section>

			<section class="space-y-3" aria-labelledby="users-heading">
				<div><h2 id="users-heading" class="font-display text-lg font-semibold text-neutral-900">Instance users</h2><p class="text-sm text-neutral-500">{users.length} total accounts</p></div>
				<div class="overflow-x-auto rounded-lg border border-neutral-200 bg-surface">
					<table class="min-w-full divide-y divide-neutral-200 text-sm">
						<thead class="bg-neutral-50 text-left text-xs uppercase tracking-wide text-neutral-500"><tr><th class="px-4 py-3">User</th><th class="px-4 py-3">Access</th><th class="px-4 py-3">Last sign-in</th><th class="px-4 py-3"><span class="sr-only">Actions</span></th></tr></thead>
						<tbody class="divide-y divide-neutral-200">
							{#each users as user (user.id)}
								<tr>
									<td class="px-4 py-3"><div class="font-medium text-neutral-900">{user.name || user.email} {#if user.id === $currentUser?.id}<span class="text-xs font-normal text-neutral-400">(you)</span>{/if}</div>{#if user.name}<div class="text-xs text-neutral-500">{user.email}</div>{/if}</td>
									<td class="px-4 py-3"><div class="flex flex-wrap gap-2"><Badge variant={statusVariant(user.status)}>{user.status}</Badge>{#if user.instanceRole === 'admin'}<Badge variant="info">admin</Badge>{/if}</div></td>
									<td class="px-4 py-3 text-neutral-500">{user.lastLoginAt ? dateTime(user.lastLoginAt) : 'Never'}</td>
									<td class="px-4 py-3"><div class="flex justify-end gap-2">
										{#if user.status !== 'invited'}
											<Button variant="outline" size="sm" disabled={user.id === $currentUser?.id} loading={changingId === user.id} onclick={() => changeRole(user)}>{user.instanceRole === 'admin' ? 'Remove admin' : 'Make admin'}</Button>
											<Button variant={user.status === 'disabled' ? 'secondary' : 'danger'} size="sm" disabled={user.id === $currentUser?.id} loading={changingId === user.id} onclick={() => changeStatus(user)}>{user.status === 'disabled' ? 'Enable' : 'Disable'}</Button>
										{/if}
									</div></td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			</section>
		{/if}
	</div>
</AppShell>
