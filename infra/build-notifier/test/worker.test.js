import { test } from 'node:test'
import assert from 'node:assert/strict'
import { deliver, notification, webhookFor } from '../worker.js'

const now = new Date('2026-01-01T12:00:00Z')

function build(type, branch) {
	return {
		type,
		source: { workerName: 'example-site' },
		payload: { buildTriggerMetadata: { branch, commitHash: 'abcdef1234567', commitMessage: 'Change something' } },
	}
}

const cases = [
	{ name: 'a main build that succeeded', event: build('cf.workersBuilds.worker.build.succeeded', 'main'), posted: true },
	{ name: 'a dev build that failed', event: build('cf.workersBuilds.worker.build.failed', 'dev'), posted: true },
	{ name: 'an issue branch build that succeeded', event: build('cf.workersBuilds.worker.build.succeeded', '81-check-env'), posted: false },
	{ name: 'an issue branch build that failed', event: build('cf.workersBuilds.worker.build.failed', '81-check-env'), posted: false },
	{ name: 'a main build that only started', event: build('cf.workersBuilds.worker.build.started', 'main'), posted: false },
	{ name: 'a build with no branch', event: build('cf.workersBuilds.worker.build.succeeded', undefined), posted: false },
]

for (const c of cases) {
	test(`notification: ${c.name}`, () => {
		assert.equal(notification(c.event, now) !== null, c.posted)
	})
}

const mainURL = 'https://discord.example/api/webhooks/1/main'
const devURL = 'https://discord.example/api/webhooks/2/dev'

const webhookCases = [
	{ name: 'main goes to the main channel', branch: 'main', env: { DISCORD_WEBHOOK_URL: mainURL, DISCORD_DEV_WEBHOOK_URL: devURL }, want: mainURL },
	{ name: 'dev goes to the dev channel', branch: 'dev', env: { DISCORD_WEBHOOK_URL: mainURL, DISCORD_DEV_WEBHOOK_URL: devURL }, want: devURL },
	{ name: 'dev with no dev secret goes nowhere', branch: 'dev', env: { DISCORD_WEBHOOK_URL: mainURL }, want: null },
]

for (const c of webhookCases) {
	test(`webhookFor: ${c.name}`, () => {
		assert.equal(webhookFor(c.branch, c.env), c.want)
	})
}

function message(body) {
	return {
		body,
		outcome: 'none',
		ack() {
			this.outcome = 'ack'
		},
		retry() {
			this.outcome = 'retry'
		},
	}
}

const answering = (status) => async () => new Response(null, { status })
const both = { DISCORD_WEBHOOK_URL: mainURL, DISCORD_DEV_WEBHOOK_URL: devURL }

const deliverCases = [
	{ name: 'Discord takes it', post: answering(204), want: 'ack' },
	{ name: 'the webhook was deleted', post: answering(404), want: 'retry' },
	{ name: 'Discord is rate limiting', post: answering(429), want: 'retry' },
	{ name: 'Discord is down', post: answering(503), want: 'retry' },
	{ name: 'the request fails', post: async () => { throw new TypeError('fetch failed') }, want: 'retry' },
]

for (const c of deliverCases) {
	test(`deliver: ${c.name}`, async () => {
		const m = message(build('cf.workersBuilds.worker.build.succeeded', 'main'))
		await deliver({ messages: [m] }, both, now, c.post)
		assert.equal(m.outcome, c.want)
	})
}

function recorder() {
	const urls = []
	const post = async (url) => {
		urls.push(url)
		return new Response(null, { status: 204 })
	}
	return { urls, post }
}

test('deliver: a dev build is posted to the dev channel', async () => {
	const { urls, post } = recorder()
	await deliver({ messages: [message(build('cf.workersBuilds.worker.build.succeeded', 'dev'))] }, both, now, post)
	assert.deepEqual(urls, [devURL])
})

test('deliver: a dev build with no dev secret is acked without a request', async () => {
	const { urls, post } = recorder()
	const m = message(build('cf.workersBuilds.worker.build.succeeded', 'dev'))
	await deliver({ messages: [m] }, { DISCORD_WEBHOOK_URL: mainURL }, now, post)
	assert.deepEqual({ outcome: m.outcome, urls }, { outcome: 'ack', urls: [] })
})
