import { expect, test } from '@playwright/test';
import type { APIRequestContext } from '@playwright/test';

const MAILPIT_URL = process.env.MAILPIT_URL || 'http://127.0.0.1:8025';
const BOOTSTRAP_TOKEN = process.env.OWL_INVITES_E2E_BOOTSTRAP_TOKEN || 'gate-2-browser-bootstrap';
const ADMIN_EMAIL = 'gate5-admin@owl-invites.test';
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

async function latestMailFor(request: APIRequestContext, recipient: string, subject?: string): Promise<MailpitMessage> {
	let latest: MailpitMessage = {};
	await expect(async () => {
		const response = await request.get(`${MAILPIT_URL}/api/v1/message/latest`);
		expect(response.ok()).toBeTruthy();
		latest = (await response.json()) as MailpitMessage;
		const recipients = (latest.To || []).map((item) => item.Address || item.Email || '');
		expect(recipients).toContain(recipient);
		if (subject) expect(latest.Subject).toBe(subject);
	}).toPass({ timeout: 15_000, intervals: [250, 500, 1_000] });
	return latest;
}

test('Gate 5 release-candidate household product flow', async ({ browser, page, request }) => {
	test.setTimeout(180_000);

	// Fresh setup, installed-app root routing, and magic-link login.
	await page.goto('/');
	if (/\/setup$/.test(page.url())) {
		await page.getByLabel('Bootstrap token').fill(BOOTSTRAP_TOKEN);
		await page.getByLabel(/^Name/).fill('Gate Five Admin');
		await page.getByLabel(/^Email/).fill(ADMIN_EMAIL);
		await page.getByRole('button', { name: 'Complete setup' }).click();
		await expect(page).toHaveURL(/\/events$/);
		await page.goto('/auth/logout');
		await expect(page).toHaveURL(/\/auth\/login$/);
	}
	await expect(page).toHaveURL(/\/auth\/login$/);
	await page.getByLabel('Email address').fill(ADMIN_EMAIL);
	await page.getByRole('button', { name: 'Send Magic Link' }).click();
	const magicMessage = await latestMailFor(request, ADMIN_EMAIL);
	const magicBody = `${magicMessage.Text || ''}\n${magicMessage.HTML || ''}`;
	const magicLink = magicBody.match(/https?:\/\/[^\s"'<>]+\/auth\/verify\?token=[0-9a-f]{64}/)?.[0] || '';
	expect(magicLink).not.toBe('');
	await page.goto(magicLink);
	await expect(page).toHaveURL(/\/events$/);

	// Event, invitation- and guest-scoped questions, including the checkbox UI.
	await page.goto('/events/new');
	await page.getByLabel('Event title').fill('Gate Five Garden Event');
	await page.getByLabel('Event date').fill(futureLocalDateTime(7));
	await page.getByLabel('Location').fill('Capability Garden');
	await page.getByRole('button', { name: 'Create event' }).click();
	await expect(page).toHaveURL(/\/events\/[0-9a-f-]+\/invitations$/);
	const invitationsURL = page.url();
	const eventID = invitationsURL.match(/\/events\/([^/]+)\/invitations$/)?.[1];
	expect(eventID).toBeTruthy();

	await page.goto(`/events/${eventID}/edit`);
	await page.getByLabel('Question Label').fill('Household note');
	await page.getByLabel('Applies to').selectOption('invitation');
	await page.getByRole('button', { name: 'Add Question' }).click();
	await page.getByLabel('Question Label').fill('Meal preferences');
	await page.getByLabel('Question Type').selectOption('checkbox');
	await page.getByLabel('Applies to').selectOption('guest');
	await page.getByPlaceholder('Option 1').fill('Vegetarian');
	await page.getByRole('button', { name: '+ Add option' }).click();
	await page.getByPlaceholder('Option 2').fill('No nuts');
	await page.getByLabel('Required').check();
	await page.getByRole('button', { name: 'Add Question' }).click();
	await expect(page.getByText('Meal preferences', { exact: true })).toBeVisible();

	// The organizer's card is now part of the actual guest experience.
	await page.goto(`/events/${eventID}/invite`);
	await page.getByRole('button', { name: /Garden Picnic/ }).click();
	await page.getByLabel('Heading').fill('Moonlit Garden Celebration');
	await page.getByLabel('Body Text').fill('Join us beneath the garden lights.');
	await page.getByLabel('Footer Text').fill('Please respond for everyone in your household.');
	await page.getByRole('button', { name: 'Save Invite Design' }).click();
	await expect(page.getByText('Design saved', { exact: true })).toBeVisible();
	await page.getByRole('button', { name: 'Publish & View Dashboard' }).click();
	await expect(page).toHaveURL(new RegExp(`/events/${eventID}$`));

	// Explicit household grouping; every household intentionally shares a contact.
	const csv = [
		'household_key,household_label,contact_email,contact_phone,preferred_delivery,additional_guest_allowance,guest_name',
		`smith,Smith Family,${HOUSEHOLD_EMAIL},,email,1,Alex Smith`,
		`smith,Smith Family,${HOUSEHOLD_EMAIL},,email,1,Bailey Smith`,
		`garcia,Garcia Family,${HOUSEHOLD_EMAIL},,email,0,María García`,
		`dupont,Dupont Household,${HOUSEHOLD_EMAIL},,email,0,Zoë Dupont`,
		`lee,Lee Household,${HOUSEHOLD_EMAIL},,email,2,Jordan Lee`,
		`lee,Lee Household,${HOUSEHOLD_EMAIL},,email,2,Taylor Lee`
	].join('\r\n');
	await page.goto(`/events/${eventID}/import`);
	await page.getByLabel('Household CSV').setInputFiles({ name: 'households.csv', mimeType: 'text/csv', buffer: Buffer.from(csv) });
	await page.getByRole('button', { name: 'Preview import' }).click();
	await expect(page.getByText('4', { exact: true }).first()).toBeVisible();
	await expect(page.getByText('6', { exact: true }).first()).toBeVisible();
	await expect(page.getByText(/remain separate invitations/)).toBeVisible();
	await page.getByRole('button', { name: 'Create households' }).click();
	await expect(page.getByText('Created 4 households with 6 assigned guests. No invitations were sent.')).toBeVisible();

	// Explicit delivery of one representative imported household.
	await page.goto(invitationsURL);
	const smithHousehold = page.locator('article').filter({ has: page.getByRole('heading', { name: 'Smith Family' }) });
	await smithHousehold.getByRole('button', { name: 'Deliver' }).click();
	await expect(page.getByText('Invitation email was accepted by the configured provider.')).toBeVisible();
	const privateMessage = await latestMailFor(request, HOUSEHOLD_EMAIL);
	const privateBody = `${privateMessage.Text || ''}\n${privateMessage.HTML || ''}`;
	const privateLink = privateBody.match(/https?:\/\/[^\s"'<>]+\/invitation\/accept#[^\s"'<>]+/)?.[0] || '';
	expect(privateLink).not.toBe('');

	// Fresh narrow browser: card, assigned guests, additional guest, attendance,
	// invitation answer, and per-attending-guest checkbox answers all work.
	const guestContext = await browser.newContext({ viewport: { width: 390, height: 844 } });
	const guestPage = await guestContext.newPage();
	await guestPage.goto(privateLink);
	await expect(guestPage).toHaveURL(/\/invitation$/);
	await expect(guestPage.getByRole('heading', { name: 'Moonlit Garden Celebration' })).toBeVisible();
	await expect(guestPage.getByText('Join us beneath the garden lights.')).toBeVisible();
	expect(await guestPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBeTruthy();
	await guestPage.getByLabel('Alex Smith attendance').selectOption('attending');
	await guestPage.getByLabel('Bailey Smith attendance').selectOption('declined');
	await guestPage.getByRole('button', { name: 'Add guest' }).click();
	await guestPage.getByLabel('Additional guest name').fill('Casey Smith');
	await guestPage.getByLabel('Casey Smith attendance').selectOption('attending');
	await guestPage.getByLabel('Household note').fill('Seat us near the lanterns');
	await guestPage.getByRole('group', { name: 'Alex Smith: Meal preferences *' }).getByLabel('Vegetarian').check();
	await guestPage.getByRole('group', { name: 'Casey Smith: Meal preferences *' }).getByLabel('No nuts').check();
	await guestPage.getByRole('button', { name: 'Save response' }).click();
	await expect(guestPage.getByText('Your response was saved.')).toBeVisible();

	// Organizer response/search/filter and accurate provider-acceptance status.
	await page.goto(invitationsURL);
	await page.getByLabel('Search').fill('Casey Smith');
	await page.getByRole('button', { name: 'Apply' }).click();
	await expect(page.getByRole('heading', { name: 'Smith Family' })).toBeVisible();
	await expect(page.getByText('1 results · 3 guests · 2 attending · 0 pending')).toBeVisible();
	await expect(page.getByText('Accepted by email provider')).toBeVisible();
	await page.getByRole('button', { name: 'Clear' }).click();
	await expect(page.getByText('4 results · 7 guests · 2 attending · 4 pending')).toBeVisible();

	// Targeted one-way broadcast: count/preview, aggregate result, Mailpit.
	await page.goto(`/events/${eventID}/messages`);
	await expect(page.getByText('4', { exact: true })).toBeVisible();
	await page.getByLabel('Subject').fill('Gate 5 household update');
	await page.getByLabel('Message').fill('This is a one-way invitation broadcast.');
	await page.getByRole('button', { name: 'Send to 4 households' }).click();
	await expect(page.getByText('Attempted 4; accepted by provider 4; failed 0; skipped 0.')).toBeVisible();
	const broadcast = await latestMailFor(request, HOUSEHOLD_EMAIL, 'Gate 5 household update');
	expect(`${broadcast.Text || ''}\n${broadcast.HTML || ''}`).toContain('one-way invitation broadcast');

	// Reminder UX exposes target, current count, schedule and status without
	// changing the bounded in-process scheduler architecture.
	await page.goto(`/events/${eventID}/reminders`);
	await page.getByLabel('Run at').fill(futureLocalDateTime(1));
	await page.getByLabel('Target response group').selectOption('pending');
	await page.getByLabel('Message').fill('Please send your household response.');
	await page.getByRole('button', { name: 'Schedule reminder' }).click();
	await expect(page.getByText('Reminder scheduled.')).toBeVisible();
	await expect(page.getByText('Status: scheduled')).toBeVisible();

	// Current-domain export includes response state and both scoped answers.
	const exportResponse = await page.request.get(`/api/v1/events/${eventID}/invitations/export`);
	expect(exportResponse.ok()).toBeTruthy();
	const exported = await exportResponse.text();
	expect(exported).toContain('Smith Family');
	expect(exported).toContain('Casey Smith');
	expect(exported).toContain('Seat us near the lanterns');
	expect(exported).toContain('Vegetarian');
	expect(exported).toContain('guest_answer:Meal preferences');

	// Open enrollment still creates a new isolated household even though every
	// imported private household uses the same stored destination.
	await page.goto(invitationsURL);
	await page.getByLabel('Enabled').check();
	await page.getByLabel('Maximum party size').fill('1');
	await page.getByLabel('Open capacity (optional seats)').fill('1');
	await page.getByRole('button', { name: 'Save open enrollment' }).click();
	const openLink = (await page.locator('code').allTextContents()).find((value) => value.includes('/enroll#')) || '';
	expect(openLink).not.toBe('');
	const openContext = await browser.newContext();
	const openPage = await openContext.newPage();
	await openPage.goto(openLink);
	await openPage.getByLabel('Invitation label').fill('Open household');
	await openPage.getByLabel('Email').fill(HOUSEHOLD_EMAIL);
	await openPage.getByLabel('Guest 1 name').fill('Dana Open');
	await openPage.getByRole('button', { name: 'Create my invitation' }).click();
	await expect(openPage).toHaveURL(/\/invitation$/);
	const managementMessage = await latestMailFor(request, HOUSEHOLD_EMAIL);
	const managementBody = `${managementMessage.Text || ''}\n${managementMessage.HTML || ''}`;
	const managementLink = managementBody.match(/https?:\/\/[^\s"'<>]+\/invitation\/accept#[^\s"'<>]+/)?.[0] || '';
	expect(managementLink).not.toBe('');
	await openContext.close();
	const recoveredOpenContext = await browser.newContext();
	const recoveredOpenPage = await recoveredOpenContext.newPage();
	await recoveredOpenPage.goto(managementLink);
	await expect(recoveredOpenPage.getByText('Dana Open', { exact: true })).toBeVisible();
	await expect(recoveredOpenPage.getByText('Alex Smith', { exact: true })).toHaveCount(0);

	await recoveredOpenContext.close();
	await guestContext.close();
});
