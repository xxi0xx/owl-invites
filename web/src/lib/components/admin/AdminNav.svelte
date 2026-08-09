<script lang="ts">
	interface Props {
		active: 'overview' | 'users' | 'audit' | 'settings';
	}

	let { active }: Props = $props();
	const items = [
		{ id: 'overview', label: 'Overview', href: '/admin' },
		{ id: 'users', label: 'Users & invites', href: '/admin/users' },
		{ id: 'audit', label: 'Audit log', href: '/admin/audit' },
		{ id: 'settings', label: 'Settings', href: '/setup' }
	] as const;
</script>

<nav aria-label="Instance administration" class="flex flex-wrap gap-2 border-b border-neutral-200 pb-4">
	{#each items as item}
		<a
			href={item.href}
			aria-current={active === item.id ? 'page' : undefined}
			class="rounded-md px-3 py-2 text-sm font-medium transition-colors {active === item.id
				? 'bg-primary-light text-primary'
				: 'text-neutral-600 hover:bg-neutral-100 hover:text-neutral-900'}"
		>
			{item.label}
		</a>
	{/each}
</nav>
