import { Hono } from 'hono'
import * as jose from 'jose'

const algorithm = {
	name: 'RSASSA-PKCS1-v1_5',
	modulusLength: 2048,
	publicExponent: new Uint8Array([0x01, 0x00, 0x01]),
	hash: { name: 'SHA-256' },
}

const importAlgo = {
	name: 'RSASSA-PKCS1-v1_5',
	hash: { name: 'SHA-256' },
}

async function loadOrGenerateKeyPair(KV) {
	let keyPair = {}
	let keyPairJson = await KV.get('keys', { type: 'json' })

	if (keyPairJson !== null) {
		keyPair.publicKey = await crypto.subtle.importKey('jwk', keyPairJson.publicKey, importAlgo, true, ['verify'])
		keyPair.privateKey = await crypto.subtle.importKey('jwk', keyPairJson.privateKey, importAlgo, true, ['sign'])

		return keyPair
	} else {
		keyPair = await crypto.subtle.generateKey(algorithm, true, ['sign', 'verify'])

		await KV.put('keys', JSON.stringify({
			privateKey: await crypto.subtle.exportKey('jwk', keyPair.privateKey),
			publicKey: await crypto.subtle.exportKey('jwk', keyPair.publicKey)
		}))

		return keyPair
	}

}

const app = new Hono()

// Settings come from the Worker's environment: CLIENT_ID and REDIRECT_URL as
// plain variables, DISCORD_CLIENT_SECRET as a secret. Missing any, refuse.
app.use('*', async (c, next) => {
	if (!c.env.CLIENT_ID || !c.env.REDIRECT_URL || !c.env.DISCORD_CLIENT_SECRET) {
		return c.text('Not configured.', 500)
	}
	await next()
})

app.get('/authorize/:scopemode', async (c) => {

	if (c.req.query('client_id') !== c.env.CLIENT_ID
		|| c.req.query('redirect_uri') !== c.env.REDIRECT_URL
		|| !['guilds', 'email'].includes(c.req.param('scopemode'))) {
		return c.text('Bad request.', 400)
	}

	const params = new URLSearchParams({
		'client_id': c.env.CLIENT_ID,
		'redirect_uri': c.env.REDIRECT_URL,
		'response_type': 'code',
		'scope': c.req.param('scopemode') == 'guilds' ? 'identify email guilds' : 'identify email',
		'state': c.req.query('state'),
		'prompt': 'none'
	}).toString()

	return c.redirect('https://discord.com/oauth2/authorize?' + params)
})

async function digest(text) {
	return new Uint8Array(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(text)))
}

// clientAuthenticated reports whether the caller sent this client's id and
// secret, by HTTP Basic auth or in the form body (RFC 6749 section 2.3.1).
async function clientAuthenticated(c, body) {
	let id = body['client_id']
	let secret = body['client_secret']
	const header = c.req.header('Authorization') ?? ''
	if (header.startsWith('Basic ')) {
		try {
			const decoded = atob(header.slice(6))
			const colon = decoded.indexOf(':')
			if (colon < 0) return false
			id = decodeURIComponent(decoded.slice(0, colon))
			secret = decodeURIComponent(decoded.slice(colon + 1))
		} catch {
			return false
		}
	}
	if (typeof id !== 'string' || typeof secret !== 'string') return false
	const [want, got] = await Promise.all([digest(c.env.DISCORD_CLIENT_SECRET), digest(secret)])
	return id === c.env.CLIENT_ID && want.every((b, i) => b === got[i])
}

app.post('/token', async (c) => {
	const body = await c.req.parseBody()
	if (!(await clientAuthenticated(c, body))) return c.text('Unauthorized.', 401)
	const code = body['code']
	const params = new URLSearchParams({
		'client_id': c.env.CLIENT_ID,
		'client_secret': c.env.DISCORD_CLIENT_SECRET,
		'redirect_uri': c.env.REDIRECT_URL,
		'code': code,
		'grant_type': 'authorization_code',
		'scope': 'identify email'
	}).toString()

	const r = await fetch('https://discord.com/api/v10/oauth2/token', {
		method: 'POST',
		body: params,
		headers: {
			'Content-Type': 'application/x-www-form-urlencoded'
		}
	}).then(res => res.json())

	if (r === null) return new Response("Bad request.", { status: 400 })
	const userInfo = await fetch('https://discord.com/api/v10/users/@me', {
		headers: {
			'Authorization': 'Bearer ' + r['access_token']
		}
	}).then(res => res.json())

	if (!userInfo['verified']) return c.text('Bad request.', 400)

	let servers = []

	const serverResp = await fetch('https://discord.com/api/v10/users/@me/guilds', {
		headers: {
			'Authorization': 'Bearer ' + r['access_token']
		}
	})

	if (serverResp.status === 200) {
		const serverJson = await serverResp.json()
		servers = serverJson.map(item => {
			return item['id']
		})
	}

	let roleClaims = {}

	if (c.env.DISCORD_TOKEN && c.env.ROLE_SERVERS) {
		await Promise.all(c.env.ROLE_SERVERS.split(',').map(async guildId => {
			if (servers.includes(guildId)) {
				let memberPromise = fetch(`https://discord.com/api/v10/guilds/${guildId}/members/${userInfo['id']}`, {
					headers: {
						'Authorization': 'Bot ' + c.env.DISCORD_TOKEN
					}
				})
				// i had issues doing this any other way?
				const memberResp = await memberPromise
				const memberJson = await memberResp.json()

				roleClaims[`roles:${guildId}`] = memberJson.roles
			}

		}
		))
	}

	let preferred_username = userInfo['username']

	if (userInfo['discriminator'] && userInfo['discriminator'] !== '0'){
		preferred_username += `#${userInfo['discriminator']}`
	}

	let displayName = userInfo['global_name'] ?? userInfo['username']

	const idToken = await new jose.SignJWT({
		iss: 'https://cloudflare.com',
		aud: c.env.CLIENT_ID,
		preferred_username,
		...userInfo,
		...roleClaims,
		email: userInfo['email'],
		global_name: userInfo['global_name'],
		name: displayName,
		guilds: servers
	})
		.setProtectedHeader({ alg: 'RS256' })
		.setExpirationTime('1h')
		.setAudience(c.env.CLIENT_ID)
		.sign((await loadOrGenerateKeyPair(c.env.KV)).privateKey)

	return c.json({
		...r,
		scope: 'identify email',
		id_token: idToken
	})
})

app.get('/jwks.json', async (c) => {
	let publicKey = (await loadOrGenerateKeyPair(c.env.KV)).publicKey
	return c.json({
		keys: [{
			alg: 'RS256',
			kid: 'jwtRS256',
			...(await crypto.subtle.exportKey('jwk', publicKey))
		}]
	})
})

export default app