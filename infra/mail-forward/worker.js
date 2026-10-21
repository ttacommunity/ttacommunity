// Forwards mail to the site's addresses (support@, privacy@ and accounts@) to every maintainer, through Email Routing.

/** The addresses an Email Routing rule sends here; mail to any other is refused. */
export const ROUTED = new Set([
	'support@ttacommunity.org',
	'privacy@ttacommunity.org',
	'accounts@ttacommunity.org',
])

/** destinations reads the FORWARD_TO secret: the maintainers' addresses, comma-separated, each verified in Email Routing. */
export function destinations(env) {
	return (env.FORWARD_TO || '')
		.split(',')
		.map((a) => a.trim())
		.filter((a) => a !== '')
}

export default {
	/** email forwards a message to a routed address to every destination, or refuses it so the sender is told. */
	async email(message, env) {
		const to = destinations(env)
		if (!ROUTED.has(message.to.toLowerCase())) {
			message.setReject('No such address here')
			return
		}
		if (to.length === 0) {
			message.setReject('This address cannot take mail just now')
			return
		}
		await Promise.all(to.map((address) => message.forward(address)))
	},
}
