import { expect, test } from '@playwright/test';
import type { APIRequestContext } from '@playwright/test';

const MAILPIT_URL = process.env.MAILPIT_URL || 'http://127.0.0.1:8025';
const BOOTSTRAP_TOKEN = process.env.OWL_INVITES_E2E_BOOTSTRAP_TOKEN || 'gate-2-browser-bootstrap';
const ADMIN_EMAIL = 'gate2-admin@owl-invites.test';
const HOUSEHOLD_EMAIL = 'shared-household@owl-invites.test';

interface MailpitMessage {
	HTML?: string;
	Subject?: string;
	Text?: string;
	To?: Array<{ Address?: string; Email?: string }>;
}

function futureLocalDateTime(days: number): string {
	const date = new Date(Date.now() + days * 24 * 60 * 60 * 1_000);
	return new Date(date.getTime() - date.getTimezoneOffset() * 60_000).toISOString().slice(0, 16);
}

async function latestMailFor(request: APIRequestContext, recipient: string): Promise<MailpitMessage> {
	let latest: MailpitMessage = {};
	await expect(async () => {
		const response = await request.get(`${MAILPIT_URL}/api/v1/message/latest`);
		expect(response.ok()).toBeTruthy();
		latest = (await response.json()) as MailpitMessage;
		const recipients = (latest.To || []).map((item) => item.Address || item.Email || '');
		expect(recipients).toContain(recipient);
	}).toPass({ timeout: 15_000, intervals: [250, 500, 1_000] });
	return latest;
}

test('Gate 2 private and open invitation flows stay isolated through Mailpit and Chromium', async ({ browser, page, request }) => {
	test.setTimeout(120_000);

	await page.goto('/');
	await expect(page).toHaveURL(/\/setup$/);

	await page.getByLabel('Bootstrap token').fill(BOOTSTRAP_TOKEN);
	await page.getByLabel(/^Name/).fill('Gate Two Admin');
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

	const magicMessage = await latestMailFor(request, ADMIN_EMAIL);
	const magicBody = `${magicMessage.Text || ''}\n${magicMessage.HTML || ''}`;
	const magicLink = magicBody.match(/https?:\/\/[^\s"'<>]+\/auth\/verify\?token=[0-9a-f]{64}/)?.[0] || '';
	expect(magicLink).not.toBe('');

	// A stale anonymous auth probe used to race the successful exchange and
	// overwrite its authenticated store. Delay any such probe so the regression
	// is deterministic instead of timing-dependent.
	let delayedAuthProbe = false;
	await page.route('**/api/v1/auth/me', async (route) => {
		if (!delayedAuthProbe) {
			delayedAuthProbe = true;
			await new Promise((resolve) => setTimeout(resolve, 750));
		}
		await route.continue();
	});
	await page.goto(magicLink);
	await expect(page).toHaveURL(/\/events$/);
	await expect(page.getByRole('heading', { name: 'My Events' })).toBeVisible();

	await page.goto('/events/new');
	await page.getByLabel('Event title').fill('Gate Two Household Event');
	await page.getByLabel('Event date').fill(futureLocalDateTime(7));
	await page.getByLabel('Location').fill('Capability Hall');
	await page.getByRole('button', { name: 'Create event' }).click();
	await expect(page).toHaveURL(/\/events\/[0-9a-f-]+\/invitations$/);
	const invitationsURL = page.url();
	const eventID = invitationsURL.match(/\/events\/([^/]+)\/invitations$/)?.[1];
	expect(eventID).toBeTruthy();

	// Add one answer at each Gate 2 scope before issuing the household capability.
	await page.goto(`/events/${eventID}/edit`);
	await page.getByLabel('Question Label').fill('Household note');
	await page.getByLabel('Applies to').selectOption('invitation');
	await page.getByRole('button', { name: 'Add Question' }).click();
	await expect(page.getByText('Household note', { exact: true })).toBeVisible();
	await page.getByLabel('Question Label').fill('Meal choice');
	await page.getByLabel('Applies to').selectOption('guest');
	await page.getByRole('button', { name: 'Add Question' }).click();
	await expect(page.getByText('Meal choice', { exact: true })).toBeVisible();

	await page.goto(invitationsURL);
	await page.getByLabel('Household label').fill('Named household');
	await page.getByLabel('Delivery email').fill(HOUSEHOLD_EMAIL);
	await page.getByLabel('Assigned guests (one per line)').fill('Alex\nBailey');
	await page.getByLabel('Additional guest allowance').fill('1');
	await page.getByRole('button', { name: 'Create invitation' }).click();
	await expect(page.getByText('Copy this private link now. Treat it as a credential.')).toBeVisible();

	const privateMessage = await latestMailFor(request, HOUSEHOLD_EMAIL);
	expect(privateMessage.Subject).toContain('Gate Two Household Event');
	const privateBody = `${privateMessage.Text || ''}\n${privateMessage.HTML || ''}`;
	const privateLink = privateBody.match(/https?:\/\/[^\s"'<>]+\/invitation\/accept#[^\s"'<>]+/)?.[0] || '';
	expect(privateLink).not.toBe('');

	const privateContext = await browser.newContext();
	const privatePage = await privateContext.newPage();
	await privatePage.goto(privateLink);
	await expect(privatePage).toHaveURL(/\/invitation$/);
	await expect(privatePage.getByRole('heading', { name: 'Gate Two Household Event' })).toBeVisible();
	const assignedAttendance = privatePage.locator('section').filter({ has: privatePage.getByRole('heading', { name: 'Guests' }) }).locator('select');
	await assignedAttendance.nth(0).selectOption('attending');
	await assignedAttendance.nth(1).selectOption('declined');
	await privatePage.getByRole('button', { name: 'Add guest' }).click();
	await privatePage.getByLabel('Additional guest name').fill('Casey');
	await privatePage.getByLabel('Household note').fill('Seat us together');
	await privatePage.getByLabel('Alex: Meal choice').fill('Vegetarian');
	await privatePage.getByRole('button', { name: 'Save response' }).click();
	await expect(privatePage.getByText('Your response was saved.')).toBeVisible();

	// Revisit with the limited invitation session: the capability is absent from
	// history and both answer scopes plus the additional guest survive.
	await privatePage.reload();
	await expect(privatePage).toHaveURL(/\/invitation$/);
	await expect(privatePage.getByLabel('Household note')).toHaveValue('Seat us together');
	await expect(privatePage.getByLabel('Alex: Meal choice')).toHaveValue('Vegetarian');
	await expect(privatePage.getByLabel('Additional guest name')).toHaveValue('Casey');
	const revisitedAttendance = privatePage.locator('section').filter({ has: privatePage.getByRole('heading', { name: 'Guests' }) }).locator('select');
	await revisitedAttendance.nth(1).selectOption('attending');
	await privatePage.getByRole('button', { name: 'Save response' }).click();
	await expect(privatePage.getByText('Your response was saved.')).toBeVisible();

	await page.goto(invitationsURL);
	const namedHousehold = page.locator('article').filter({ has: page.getByRole('heading', { name: 'Named household' }) });
	await expect(namedHousehold).toContainText('Alex: attending');
	await expect(namedHousehold).toContainText('Bailey: attending');
	await expect(namedHousehold).toContainText('Casey: attending');
	await expect(page.getByText('1 households · 3 guests · 3 attending · 0 pending')).toBeVisible();

	// Open capacity is separate from the named allocation. Reusing the named
	// invitation email creates a second isolated invitation and cannot claim it.
	await page.getByLabel('Enabled').check();
	await page.getByLabel('Maximum party size').fill('2');
	await page.getByLabel('Open capacity (optional seats)').fill('2');
	await page.getByRole('button', { name: 'Save open enrollment' }).click();
	await expect(page.getByText('Public enrollment capability')).toBeVisible();
	const openURLs = await page.locator('code').allTextContents();
	const openLink = openURLs.find((value) => value.includes('/enroll#')) || '';
	expect(openLink).not.toBe('');

	const openContext = await browser.newContext();
	const openPage = await openContext.newPage();
	await openPage.goto(openLink);
	await expect(openPage).toHaveURL(/\/enroll$/);
	await openPage.getByLabel('Invitation label').fill('Open household');
	await openPage.getByLabel('Email').fill(HOUSEHOLD_EMAIL);
	await openPage.getByLabel('Guest 1 name').fill('Dana');
	await openPage.getByRole('button', { name: 'Add guest' }).click();
	await openPage.getByLabel('Guest 2 name').fill('Ellis');
	await openPage.getByRole('button', { name: 'Create my invitation' }).click();
	await expect(openPage).toHaveURL(/\/invitation$/);

	await page.goto(invitationsURL);
	await expect(page.getByText('2 households · 5 guests · 3 attending · 2 pending')).toBeVisible();
	await expect(namedHousehold).toContainText(HOUSEHOLD_EMAIL);
	await expect(namedHousehold).toContainText('Alex: attending');
	const openHousehold = page.locator('article').filter({ has: page.getByRole('heading', { name: 'Open household' }) });
	await expect(openHousehold).toContainText(HOUSEHOLD_EMAIL);
	await expect(openHousehold).toContainText('Dana: pending');
	await expect(openHousehold).toContainText('Ellis: pending');

	const capacityContext = await browser.newContext();
	const capacityPage = await capacityContext.newPage();
	await capacityPage.goto(openLink);
	await capacityPage.getByLabel('Invitation label').fill('Over capacity');
	await capacityPage.getByLabel('Email').fill('capacity@owl-invites.test');
	await capacityPage.getByLabel('Guest 1 name').fill('Full');
	await capacityPage.getByRole('button', { name: 'Create my invitation' }).click();
	await expect(capacityPage.getByText(/capacity/i)).toBeVisible();
	await expect(capacityPage).toHaveURL(/\/enroll$/);

	await page.goto(`/events/${eventID}/messages`);
	await page.getByLabel('Subject').fill('Gate 2 household update');
	await page.getByLabel('Message').fill('This is a one-way invitation broadcast.');
	await page.getByRole('button', { name: 'Send message' }).click();
	await expect(page.getByText('Delivered to 2 invitations.')).toBeVisible();
	const broadcast = await latestMailFor(request, HOUSEHOLD_EMAIL);
	expect(broadcast.Subject).toBe('Gate 2 household update');
	expect(`${broadcast.Text || ''}\n${broadcast.HTML || ''}`).toContain('one-way invitation broadcast');

	await capacityContext.close();
	await openContext.close();
	await privateContext.close();
});
