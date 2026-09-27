import { test, beforeEach, afterEach } from 'node:test'
import assert from 'node:assert/strict'
import * as jose from 'jose'
import app from '../worker.js'

const env = {
	CLIENT_ID: '1234',
	REDIRECT_URL: 'https://team.cloudflareaccess.com/cdn-cgi/access/callback',
	DISCORD_CLIENT_SECRET: 'discord-secret',
}

function memoryKV() {
	const store = new Map()
	return {
		get: async (key, opts) => (store.has(key) ? (opts?.type === 'json' ? JSON.parse(store.get(key)) : store.get(key)) : null),
		put: async (key, value) => { store.set(key, value) },
	}
}

const realFetch = globalThis.fetch
let discordCalls

function fakeDiscord({ token = { access_token: 'discord-access', token_type: 'Bearer' }, user = { id: '100000000000000001', username: 'someone', global_name: 'Someone', verified: true, email: 'someone@example.com' } } = {}) {
	globalThis.fetch = async (url) => {
		discordCalls.push(String(url))
		if (String(url).endsWith('/oauth2/token')) return Response.json(token)
		if (String(url).endsWith('/users/@me')) return Response.json(user)
		if (String(url).endsWith('/users/@me/guilds')) return Response.json([])
		throw new Error(`unexpected fetch ${url}`)
	}
}

beforeEach(() => { discordCalls = [] })
afterEach(() => { globalThis.fetch = realFetch })

function tokenRequest({ auth, body = {} } = {}) {
	const headers = { 'Content-Type': 'application/x-www-form-urlencoded' }
	if (auth) headers.Authorization = 'Basic ' + btoa(auth)
	return app.request('/token', {
		method: 'POST', headers,
		body: new URLSearchParams({ grant_type: 'authorization_code', code: 'abc', ...body }).toString(),
	}, { ...env, KV: memoryKV() })
}

test('authorize refuses an unknown client id', async () => {
	const res = await app.request(`/authorize/email?client_id=999&redirect_uri=${encodeURIComponent(env.REDIRECT_URL)}&state=s`, {}, { ...env, KV: memoryKV() })
	assert.equal(res.status, 400)
})

test('authorize redirects the right client to Discord, keeping its state', async () => {
	const res = await app.request(`/authorize/email?client_id=1234&redirect_uri=${encodeURIComponent(env.REDIRECT_URL)}&state=s1`, {}, { ...env, KV: memoryKV() })
	assert.equal(res.status, 302)
	const to = new URL(res.headers.get('Location'))
	assert.equal(to.origin + to.pathname, 'https://discord.com/oauth2/authorize')
	assert.equal(to.searchParams.get('state'), 's1')
	assert.equal(to.searchParams.get('client_id'), '1234')
})

test('token refuses a caller without client credentials', async () => {
	fakeDiscord()
	const res = await tokenRequest()
	assert.equal(res.status, 401)
	assert.deepEqual(discordCalls, [])
})

test('token refuses a wrong client secret, by Basic auth or in the body', async () => {
	fakeDiscord()
	for (const req of [{ auth: '1234:wrong' }, { body: { client_id: '1234', client_secret: 'wrong' } }]) {
		const res = await tokenRequest(req)
		assert.equal(res.status, 401)
	}
	assert.deepEqual(discordCalls, [])
})

test('token refuses when Discord returns no access token', async () => {
	fakeDiscord({ token: { error: 'invalid_grant' } })
	const res = await tokenRequest({ auth: '1234:discord-secret' })
	assert.equal(res.status, 400)
	assert.ok(!discordCalls.some((u) => u.endsWith('/users/@me')), 'must not call Discord with no token')
})

test('token refuses an unverified Discord account', async () => {
	fakeDiscord({ user: { id: '1', username: 'x', verified: false, email: 'x@example.com' } })
	const res = await tokenRequest({ auth: '1234:discord-secret' })
	assert.equal(res.status, 400)
})

test('token returns an ID token signed by the published key, for this client, carrying the Discord id', async () => {
	fakeDiscord()
	const kv = memoryKV()
	const res = await app.request('/token', {
		method: 'POST',
		headers: { 'Content-Type': 'application/x-www-form-urlencoded', Authorization: 'Basic ' + btoa('1234:discord-secret') },
		body: new URLSearchParams({ grant_type: 'authorization_code', code: 'abc' }).toString(),
	}, { ...env, KV: kv })
	assert.equal(res.status, 200)
	const { id_token } = await res.json()

	const jwks = await (await app.request('/jwks.json', {}, { ...env, KV: kv })).json()
	const { payload } = await jose.jwtVerify(id_token, jose.createLocalJWKSet(jwks), { audience: '1234' })
	assert.equal(payload.id, '100000000000000001')
	assert.equal(payload.email, 'someone@example.com')
	assert.ok(payload.exp > Date.now() / 1000)
})

test('every route refuses to work without its settings', async () => {
	fakeDiscord()
	const bare = { KV: memoryKV() }
	const authorize = await app.request(`/authorize/email?client_id=1234&redirect_uri=x&state=s`, {}, bare)
	const token = await app.request('/token', { method: 'POST', headers: { Authorization: 'Basic ' + btoa(':') } }, bare)
	assert.equal(authorize.status, 500)
	assert.equal(token.status, 500)
	assert.deepEqual(discordCalls, [])
})
