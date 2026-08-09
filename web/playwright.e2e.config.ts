import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
	testDir: './tests/e2e',
	outputDir: './test-results/e2e',
	fullyParallel: false,
	forbidOnly: !!process.env.CI,
	retries: process.env.CI ? 1 : 0,
	workers: 1,
	reporter: process.env.CI ? 'list' : 'html',
	timeout: 45_000,
	use: {
		baseURL: process.env.BASE_URL || 'http://127.0.0.1:8080',
		trace: 'retain-on-failure',
		screenshot: 'only-on-failure',
		...devices['Desktop Chrome']
	},
	projects: [{ name: 'chromium', use: { browserName: 'chromium' } }]
});
