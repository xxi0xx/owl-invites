<script lang="ts">
	import { onMount } from 'svelte';

	type PublicConfig = {
		instanceName: string;
		supportEmail: string;
		smsEnabled: boolean;
		smsSenderName: string;
	};

	let config = $state<PublicConfig>({
		instanceName: 'Owl Invites',
		supportEmail: '',
		smsEnabled: false,
		smsSenderName: 'Owl Invites'
	});

	onMount(async () => {
		try {
			const response = await fetch('/api/v1/config');
			if (!response.ok) return;

			const body = (await response.json()) as { data?: Partial<PublicConfig> };
			if (!body.data) return;

			config = {
				...config,
				...body.data
			};
		} catch {
			// The page remains useful with safe generic defaults.
		}
	});
</script>

<svelte:head>
	<title>About — Owl Invites</title>
	<meta
		name="description"
		content="Information about this Owl Invites installation and its invitation and SMS messaging practices."
	/>
</svelte:head>

<div class="min-h-screen bg-neutral-50 px-4 py-12">
	<main class="mx-auto max-w-3xl">
		<div class="mb-8">
			<a href="/" class="text-xl font-bold text-primary hover:text-primary-hover">Owl Invites</a>
		</div>

		<article class="rounded-lg border border-neutral-200 bg-surface p-6 shadow-sm sm:p-10">
			<h1 class="font-display text-3xl font-semibold text-neutral-900">About this installation</h1>

			<div class="mt-8 space-y-8 text-neutral-700">
				<section>
					<h2 class="text-xl font-semibold text-neutral-900">{config.instanceName}</h2>
					<p class="mt-3 leading-7">
						This is a self-hosted Owl Invites installation used to create and manage private
						event invitations, guest lists, and RSVP responses.
					</p>
					<p class="mt-3 leading-7">
						Invitation links provided through this service are intended for the invited guest or
						household and should not be shared unless the event organizer asks you to do so.
					</p>
				</section>

				<section>
					<h2 class="text-xl font-semibold text-neutral-900">SMS messaging</h2>
					<p class="mt-3 leading-7">
						The SMS sender identity used by this installation is
						<strong>{config.smsSenderName}</strong>.
					</p>
					<p class="mt-3 leading-7">
						When SMS delivery is used, messages are transactional and relate to private event
						invitations and RSVP links. Messages may also be re-sent when an organizer requests
						delivery again. Messages are not used for promotional marketing.
					</p>
					<p class="mt-3 leading-7">
						Message frequency varies and is expected to be low. Message and data rates may apply.
						Reply <strong>STOP</strong> to opt out or <strong>HELP</strong> for help.
					</p>
				</section>

				<section>
	<h2 class="text-xl font-semibold text-neutral-900">SMS consent</h2>
	<p class="mt-3 leading-7">
		Guests must explicitly consent before an organizer selects SMS as the invitation
		delivery method. Consent is obtained verbally, such as in person or by phone.
	</p>

	<div class="mt-4 rounded-lg border border-neutral-200 bg-neutral-50 p-4">
		<p class="text-sm font-semibold text-neutral-900">Verbal SMS opt-in script</p>
		<p class="mt-2 leading-7">
			“Would you like me to send your private event invitation and RSVP link by text
			from {config.smsSenderName}? Message frequency varies. Message and data rates may
			apply. You can reply STOP to opt out or HELP for help.”
		</p>
	</div>

	<p class="mt-4 leading-7">
		The guest must explicitly answer yes before SMS delivery is selected. If the guest
		declines, no SMS is sent and another invitation delivery method may be used. SMS
		participation is optional and is not required to receive or respond to an invitation.
	</p>
</section>

				<section>
					<h2 class="text-xl font-semibold text-neutral-900">Contact</h2>
					{#if config.supportEmail}
						<p class="mt-3 leading-7">
							For questions about this installation or its messaging program, contact
							<a
								href={`mailto:${config.supportEmail}`}
								class="font-medium text-primary hover:text-primary-hover"
							>
								{config.supportEmail}
							</a>.
						</p>
					{:else}
						<p class="mt-3 leading-7">
							For questions about an invitation or this messaging program, contact the event
							organizer who provided your invitation.
						</p>
					{/if}
				</section>
			</div>

			<footer
				class="mt-10 flex flex-wrap gap-x-6 gap-y-2 border-t border-neutral-200 pt-6 text-sm"
			>
				<a href="/privacy" class="font-medium text-primary hover:text-primary-hover">
					Privacy Policy
				</a>
				<a href="/terms" class="font-medium text-primary hover:text-primary-hover">
					Terms of Service
				</a>
				<a href="/" class="font-medium text-primary hover:text-primary-hover">
					Back to Owl Invites
				</a>
			</footer>
		</article>
	</main>
</div>
