// Posts finished Workers Builds builds to Discord. Build events arrive on the build-notifications queue.

/** The branches whose builds are posted: production and the dev site. */
export const POSTED_BRANCHES = ['main', 'dev']

/**
 * notification returns the Discord embed for a Workers Builds event at time now, or null when it is not posted.
 */
export function notification(event, now) {
	const type = event.type || ''
	const payload = event.payload || {}
	const meta = payload.buildTriggerMetadata || {}
	const succeeded = type.includes('build.succeeded')
	const failed = type.includes('build.failed')
	if ((!succeeded && !failed) || !POSTED_BRANCHES.includes(meta.branch)) {
		return null
	}
	const worker = (event.source || {}).workerName || 'unknown'
	const status = succeeded ? '✅ Succeeded' : '❌ Failed'
	return {
		title: `Build ${status}: ${worker}`,
		description: meta.commitMessage || 'N/A',
		color: succeeded ? 3066993 : 15158332,
		fields: [
			{ name: 'Worker', value: worker, inline: true },
			{ name: 'Branch', value: meta.branch || 'unknown', inline: true },
			{ name: 'Commit', value: meta.commitHash ? meta.commitHash.substring(0, 7) : 'N/A', inline: true },
			{ name: 'Author', value: meta.author || 'N/A', inline: true },
			{ name: 'Repo', value: meta.repoName || 'N/A', inline: true },
			{ name: 'Completed', value: payload.stoppedAt ? new Date(payload.stoppedAt).toISOString() : 'N/A', inline: true },
		],
		timestamp: now.toISOString(),
	}
}

export default {
	async queue(batch, env) {
		for (const message of batch.messages) {
			const embed = notification(message.body, new Date())
			if (embed === null) {
				message.ack()
				continue
			}
			try {
				await fetch(env.DISCORD_WEBHOOK_URL, {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ embeds: [embed] }),
				})
				message.ack()
			} catch {
				message.retry()
			}
		}
	},
}
