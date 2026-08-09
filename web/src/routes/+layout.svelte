<script lang="ts">
	import '../app.css';
	import { currentUser, isLoading } from '$lib/stores/auth';
	import { api } from '$lib/api/client';
	import { onMount } from 'svelte';
	import Toast from '$lib/components/ui/Toast.svelte';

	onMount(async () => {
		// Capability exchange routes establish authentication themselves. Starting
		// an anonymous /auth/me probe here can race their successful exchange and
		// overwrite the newly authenticated store with null.
		if (window.location.pathname === '/auth/verify' || window.location.pathname === '/auth/accept-invite') {
			$isLoading = false;
			return;
		}

		try {
			const user = await api.operation('getCurrentUser');
			$currentUser = user;

			// Auto-save browser timezone to profile if not set yet.
			if (!user.timezone) {
				const tz = Intl.DateTimeFormat().resolvedOptions().timeZone;
				if (tz) {
					api.operation('updateCurrentUser', { body: { timezone: tz } })
						.then((updated) => { $currentUser = updated; })
						.catch(() => {});
				}
			}
		} catch {
			$currentUser = null;
		} finally {
			$isLoading = false;
		}
	});

	let { children } = $props();
</script>

<div class="min-h-screen bg-neutral-50">
	{@render children()}
	<Toast />
</div>
