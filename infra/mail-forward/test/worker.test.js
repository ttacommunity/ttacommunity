import { test } from 'node:test'
import assert from 'node:assert/strict'
import worker, { destinations, ROUTED } from '../worker.js'

/** message is an email to to, recording what the Worker did with it. */
function message(to) {
	const done = { forwarded: [], rejected: null }
	return {
		done,
		from: 'player@example.test',
		to,
		headers: new Headers({ subject: 'A question' }),
		forward: async (rcpt) => {
			done.forwarded.push(rcpt)
		},
		setReject: (reason) => {
			done.rejected = reason
		},
	}
}

const env = { FORWARD_TO: ' one@example.test , two@example.test ' }

const cases = [
	{ name: 'a list, trimmed', value: ' one@example.test , two@example.test ', want: ['one@example.test', 'two@example.test'] },
	{ name: 'one address', value: 'one@example.test', want: ['one@example.test'] },
	{ name: 'nothing set', value: undefined, want: [] },
	{ name: 'empty entries dropped', value: 'one@example.test,,', want: ['one@example.test'] },
]
for (const c of cases) {
	test(`destinations: ${c.name}`, () => {
		assert.deepEqual(destinations({ FORWARD_TO: c.value }), c.want)
	})
}

for (const to of ROUTED) {
	test(`mail to ${to} reaches every maintainer`, async () => {
		const m = message(to)
		await worker.email(m, env)
		assert.deepEqual(m.done.forwarded, ['one@example.test', 'two@example.test'])
		assert.equal(m.done.rejected, null)
	})
}

test('the routed addresses are support@, privacy@ and accounts@ alone', () => {
	assert.deepEqual([...ROUTED].sort(), [
		'accounts@ttacommunity.org',
		'privacy@ttacommunity.org',
		'support@ttacommunity.org',
	])
})

test('mail to an address not routed here is refused, not forwarded', async () => {
	const m = message('anyone@ttacommunity.org')
	await worker.email(m, env)
	assert.deepEqual(m.done.forwarded, [])
	assert.ok(m.done.rejected)
})

test('with no destinations set, mail is refused so the sender knows', async () => {
	const m = message('support@ttacommunity.org')
	await worker.email(m, {})
	assert.deepEqual(m.done.forwarded, [])
	assert.ok(m.done.rejected)
})

test('the routed address matches in any case', async () => {
	const m = message('Support@TTACommunity.org')
	await worker.email(m, env)
	assert.equal(m.done.forwarded.length, 2)
})
