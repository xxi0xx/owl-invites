<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { currentUser } from '$lib/stores/auth';
	import { toast } from '$lib/stores/toast';
	import { getTimezoneOptions } from '$lib/utils/timezones';
	import Button from '$lib/components/ui/Button.svelte';
	import Input from '$lib/components/ui/Input.svelte';
	import Select from '$lib/components/ui/Select.svelte';
	import Card from '$lib/components/ui/Card.svelte';
	import Spinner from '$lib/components/ui/Spinner.svelte';

	const browserTz = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
	const tzOptions = getTimezoneOptions(browserTz);
	const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

	let loading = $state(true);
	let submitting = $state(false);
	let setupRequired = $state(false);
	let configured = $state(false);
	let needsAdmin = $state(false);

	let bootstrapToken = $state('');
	let adminEmail = $state('');
	let adminName = $state('');
	let instanceName = $state('Owl Invites');
	let defaultTimezone = $state(browserTz);
	let allowSignups = $state(false);
	let supportEmail = $state('');
	let errors: Record<string, string> = $state({});

	onMount(async () => {
		try {
			const status = await api.operation('getSetupStatus');
			setupRequired = status.setupRequired;
			configured = status.configured;
			if (!setupRequired) {
				try {
					const response = await api.operation('getInstanceSettings');
					instanceName = response.data.instanceName;
					defaultTimezone = response.data.defaultTimezone || browserTz;
					allowSignups = response.data.allowSignups;
					supportEmail = response.data.supportEmail;
				} catch (error: unknown) {
					const apiError = error as { status?: number };
					if (apiError.status === 401 || apiError.status === 403) {
						needsAdmin = true;
					} else {
						throw error;
					}
				}
			}
		} catch (error: unknown) {
			const apiError = error as { message?: string };
			toast.error(apiError.message || 'Failed to load setup status');
		} finally {
			loading = false;
		}
	});

	function validate(): boolean {
		errors = {};
		if (!instanceName.trim()) errors.instanceName = 'Instance name is required';
		if (!defaultTimezone) errors.defaultTimezone = 'Default timezone is required';
		if (supportEmail.trim() && !EMAIL_RE.test(supportEmail.trim())) {
			errors.supportEmail = 'Enter a valid email address';
		}
		if (setupRequired) {
			if (!bootstrapToken) errors.bootstrapToken = 'Bootstrap token is required';
			if (!adminName.trim()) errors.adminName = 'Administrator name is required';
			if (!EMAIL_RE.test(adminEmail.trim())) errors.adminEmail = 'Enter a valid email address';
		}
		return Object.keys(errors).length === 0;
	}

	async function handleSubmit(event: SubmitEvent) {
		event.preventDefault();
		if (!validate()) return;
		submitting = true;
		try {
			if (setupRequired) {
				const response = await api.operation('bootstrapInstance', {
					body: {
						bootstrapToken,
						adminEmail: adminEmail.trim().toLowerCase(),
						adminName: adminName.trim(),
						instanceName: instanceName.trim(),
						defaultTimezone,
						allowSignups,
						supportEmail: supportEmail.trim()
					}
				});
				bootstrapToken = '';
				$currentUser = response.data.user;
				// A safe authenticated request mints the session-bound CSRF cookie.
				$currentUser = await api.operation('getCurrentUser');
				toast.success('Owl Invites is ready');
				await goto('/events', { replaceState: true });
			} else {
				await api.operation('updateInstanceSettings', {
					body: {
						instanceName: instanceName.trim(),
						defaultTimezone,
						allowSignups,
						supportEmail: supportEmail.trim()
					}
				});
				toast.success('Instance settings saved');
			}
		} catch (error: unknown) {
			const apiError = error as { status?: number; message?: string };
			if (apiError.status === 401 || apiError.status === 403) needsAdmin = !setupRequired;
			toast.error(apiError.message || (setupRequired ? 'Setup failed' : 'Failed to save settings'));
		} finally {
			submitting = false;
		}
	}
</script>

<svelte:head>
	<title>{setupRequired ? 'Set up Owl Invites' : 'Instance Settings'}</title>
	<meta name="robots" content="noindex" />
</svelte:head>

<div class="min-h-screen bg-neutral-50">
	<header class="border-b border-neutral-200 bg-surface">
		<div class="mx-auto flex h-16 max-w-4xl items-center justify-between px-4 sm:px-6">
			<a href="/" class="font-display text-xl font-bold text-primary">Owl Invites</a>
			{#if configured}
				<a href="/admin" class="text-sm font-medium text-neutral-600 hover:text-neutral-900">Back to admin</a>
			{/if}
		</div>
	</header>

	<main class="mx-auto max-w-3xl px-4 py-10 sm:px-6 sm:py-14">
		{#if loading}
			<div class="flex items-center justify-center py-20"><Spinner size="lg" /></div>
		{:else if needsAdmin}
			<Card>
				<div class="space-y-4 py-8 text-center">
					<h1 class="font-display text-2xl font-bold text-neutral-900">Administrator sign-in required</h1>
					<p class="mx-auto max-w-md text-sm text-neutral-500">
						Instance settings are restricted to Owl Invites administrators.
					</p>
					<Button href="/auth/login">Sign in</Button>
				</div>
			</Card>
		{:else}
			<div class="mb-8">
				<p class="mb-2 text-sm font-semibold uppercase tracking-wide text-primary">
					{setupRequired ? 'First-run setup' : 'Administration'}
				</p>
				<h1 class="font-display text-3xl font-bold text-neutral-900">
					{setupRequired ? 'Set up Owl Invites' : 'Instance settings'}
				</h1>
				<p class="mt-2 text-neutral-600">
					{setupRequired
						? 'Create the first administrator and choose secure defaults for this instance.'
						: 'Manage defaults that apply across every account and event.'}
				</p>
			</div>

			<form onsubmit={handleSubmit}>
				<Card>
					<div class="space-y-8">
						{#if setupRequired}
							<section class="space-y-5" aria-labelledby="bootstrap-heading">
								<div>
									<h2 id="bootstrap-heading" class="font-display text-lg font-semibold text-neutral-900">Operator authorization</h2>
									<p class="mt-1 text-sm text-neutral-500">
										Enter the value of <code class="rounded bg-neutral-100 px-1.5 py-0.5 font-mono text-xs text-neutral-700">OWL_INVITES_BOOTSTRAP_TOKEN</code>
										from the server environment. The token is never stored by Owl Invites, and this setup endpoint disappears after success.
									</p>
								</div>
								<Input label="Bootstrap token" name="bootstrapToken" type="password" bind:value={bootstrapToken} error={errors.bootstrapToken || ''} required />
							</section>

							<section class="space-y-5 border-t border-neutral-200 pt-7" aria-labelledby="admin-heading">
								<div>
									<h2 id="admin-heading" class="font-display text-lg font-semibold text-neutral-900">First administrator</h2>
									<p class="mt-1 text-sm text-neutral-500">This account is activated immediately and signed in when setup completes.</p>
								</div>
								<div class="grid gap-5 sm:grid-cols-2">
									<Input label="Name" name="adminName" bind:value={adminName} error={errors.adminName || ''} required />
									<Input label="Email" name="adminEmail" type="email" bind:value={adminEmail} error={errors.adminEmail || ''} required />
								</div>
							</section>
						{/if}

						<section class:pt-1={!setupRequired} class="space-y-5 border-t border-neutral-200 pt-7" aria-labelledby="instance-heading">
							<div>
								<h2 id="instance-heading" class="font-display text-lg font-semibold text-neutral-900">Instance defaults</h2>
								<p class="mt-1 text-sm text-neutral-500">These values can be changed later by an administrator.</p>
							</div>
							<Input label="Instance name" name="instanceName" bind:value={instanceName} error={errors.instanceName || ''} helper="Shown to account holders and guests." required />
							<Select label="Default timezone" name="defaultTimezone" bind:value={defaultTimezone} options={tzOptions} error={errors.defaultTimezone || ''} required />
							<Input label="Support email (optional)" name="supportEmail" type="email" bind:value={supportEmail} error={errors.supportEmail || ''} />

							<fieldset>
								<legend class="mb-2 text-sm font-medium text-neutral-700">Account creation</legend>
								<label class="flex cursor-pointer items-start gap-3 rounded-lg border border-neutral-200 p-4">
									<input type="checkbox" bind:checked={allowSignups} class="mt-0.5 rounded border-neutral-300 text-primary focus:ring-primary/40" />
									<span>
										<span class="block text-sm font-medium text-neutral-800">Allow open account signups</span>
										<span class="mt-1 block text-xs leading-relaxed text-neutral-500">Keep this off for invite-only access. Event owners can still invite co-hosts.</span>
									</span>
								</label>
							</fieldset>
						</section>
					</div>

					<div class="mt-8 flex items-center justify-end border-t border-neutral-200 pt-6">
						<Button type="submit" loading={submitting}>{setupRequired ? 'Complete setup' : 'Save settings'}</Button>
					</div>
				</Card>
			</form>
		{/if}
	</main>
</div>
