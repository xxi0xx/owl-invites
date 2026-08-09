import { expect, test } from '@playwright/test';

const MAILPIT_URL = process.env.MAILPIT_URL || 'http://127.0.0.1:8025';
const BOOTSTRAP_TOKEN = process.env.OWL_INVITES_E2E_BOOTSTRAP_TOKEN || 'gate-1-browser-bootstrap';
const ADMIN_EMAIL = 'gate1-admin@owl-invites.test';

interface MailpitMessage {
	HTML?: string;
	Text?: string;
	To?: Array<{ Address?: string; Email?: string }>;
}

test('fresh instance bootstraps and an existing user signs in through a Mailpit magic link', async ({ page, request }) => {
	await page.goto('/');
	await expect(page).toHaveURL(/\/setup$/);

	await page.getByLabel('Bootstrap token').fill(BOOTSTRAP_TOKEN);
	await page.getByLabel(/^Name/).fill('Gate One Admin');
	await page.getByLabel(/^Email/).fill(ADMIN_EMAIL);
	await page.getByRole('button', { name: 'Complete setup' }).click();

	await expect(page).toHaveURL(/\/events$/);
	await expect(page.getByRole('heading', { name: 'My Events' })).toBeVisible();
	await page.goto('/');
	await expect(page).toHaveURL(/\/events$/);

	await page.goto('/auth/logout');
	await expect(page).toHaveURL(/\/auth\/login$/);
	await page.goto('/');
	await expect(page).toHaveURL(/\/auth\/login$/);
	await page.getByLabel('Email address').fill(ADMIN_EMAIL);
	await page.getByRole('button', { name: 'Send Magic Link' }).click();
	await expect(page.getByRole('heading', { name: 'Check your email' })).toBeVisible();

	let magicLink = '';
	await expect(async () => {
		const response = await request.get(`${MAILPIT_URL}/api/v1/message/latest`);
		expect(response.ok()).toBeTruthy();
		const message = (await response.json()) as MailpitMessage;
		const recipients = (message.To || []).map((item) => item.Address || item.Email || '');
		expect(recipients).toContain(ADMIN_EMAIL);
		const body = `${message.Text || ''}\n${message.HTML || ''}`;
		magicLink = body.match(/https?:\/\/[^\s"'<>]+\/auth\/verify\?token=[0-9a-f]{64}/)?.[0] || '';
		expect(magicLink).not.toBe('');
	}).toPass({ timeout: 15_000, intervals: [250, 500, 1_000] });

	await page.goto(magicLink);
	await expect(page).toHaveURL(/\/events$/);
	await expect(page.getByRole('heading', { name: 'My Events' })).toBeVisible();
});
