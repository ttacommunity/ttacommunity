import { describe, expect, it } from 'vitest';
import * as req from './admin-requests';

const BADGE = { id: 7, label: 'TD', description: 'Runs it', players: ['ann', 'bob'] };

describe('admin requests', () => {
  it.each([
    // Tournaments and groups
    ['a setting', req.setTournament('worlds', { visible: true }), '/api/tournaments/worlds', 'PATCH', { visible: true }],
    ['a move', req.moveInto('cge_42', 'mixed_duel'), '/api/tournaments/cge_42', 'PATCH', { parent: 'mixed_duel' }],
    ['a move out of every group', req.moveInto('cge_42', ''), '/api/tournaments/cge_42', 'PATCH', { parent: '' }],
    ['a new group', req.newGroup({ name: 'Swiss Open', kind: 'series' }), '/api/tournaments', 'POST', { name: 'Swiss Open', kind: 'series' }],
    ['a group\'s members', req.setMembers('swiss_open', ['cge_1817']), '/api/tournaments/swiss_open', 'PATCH', { members: ['cge_1817'] }],
    ['a season note, trimmed', req.setNote('worlds', '2024', '', ' Replayed '), '/api/tournaments/worlds/seasons/2024', 'PATCH', { note: 'Replayed' }],
    ['a stage note, trimmed, to a season to escape', req.setNote('worlds', '2023 Play/Off', 'Stage 1', ' Best of three '), '/api/tournaments/worlds/seasons/2023%20Play%2FOff', 'PATCH', { stage_notes: { 'Stage 1': 'Best of three' } }],
    ['a time-out penalty', req.setPenalty('international', '30', -1), '/api/tournaments/international/seasons/30', 'PATCH', { timeout_penalty: -1 }],
    ['a podium', req.setPodium('worlds', '2024', { podium: [{ place: 1, name: 'ann' }], note: '' }), '/api/tournaments/worlds/seasons/2024', 'PATCH', { podium: { podium: [{ place: 1, name: 'ann' }], note: '' } }],
    ['clearing a podium', req.clearPodium('worlds', '2023 Play Off'), '/api/tournaments/worlds/seasons/2023%20Play%20Off', 'PATCH', { clear_podium: true }],
    ['a card', req.setCard('worlds', { icon: '🏆', tagline: '', summary: '', subtitle: '', cadence: '' }), '/api/tournaments/worlds', 'PATCH', { card: { icon: '🏆', tagline: '', summary: '', subtitle: '', cadence: '' } }],
    ['regulations', req.setRegulations('worlds', 'Best of three.'), '/api/tournaments/worlds', 'PATCH', { regulations: 'Best of three.' }],
    ['stories', req.setStories('worlds', []), '/api/tournaments/worlds', 'PATCH', { stories: [] }],
    ['a new season, trimmed', req.addSeason('worlds', ' 2028 '), '/api/tournaments/worlds/seasons', 'POST', { label: '2028' }],
    ['removing a season', req.removeSeason('worlds', '2028 B'), '/api/tournaments/worlds/seasons/2028%20B', 'DELETE', undefined],
    ['a season\'s rating', req.setSeasonFlag('worlds', '2024', 'rated', false), '/api/tournaments/worlds/seasons/2024', 'PATCH', { rated: false }],
    ['a season among the champions', req.setSeasonFlag('worlds', '2024', 'champions', true), '/api/tournaments/worlds/seasons/2024', 'PATCH', { in_champions: true }],
    ['a season in the community leaderboard', req.setSeasonFlag('worlds', '2024', 'community', false), '/api/tournaments/worlds/seasons/2024', 'PATCH', { in_community: false }],
    // Blitz tournaments
    ['entrants to add, to an edition to escape', req.addBlitzEntrants('us_open', 'Season 1', [{ name: 'a', discord_name: 'b', zone: 'UTC' }]), '/api/blitz/us_open/Season%201/entrants', 'POST', { entrants: [{ name: 'a', discord_name: 'b', zone: 'UTC' }] }],
    ['an entrant dropped', req.setBlitzEntrant('us_open', '2026', 7, { status: 'dropped' }), '/api/blitz/us_open/2026/entrants/7', 'PATCH', { status: 'dropped' }],
    ['removing an entrant', req.removeBlitzEntrant('us_open', '2026', 7), '/api/blitz/us_open/2026/entrants/7', 'DELETE', undefined],
    ['an entrant skipping the next round', req.skipBlitzRound('us_open', '2026', 7), '/api/blitz/us_open/2026/entrants/7/skip', 'POST', undefined],
    ['undoing a skip', req.unskipBlitzRound('us_open', '2026', 7), '/api/blitz/us_open/2026/entrants/7/skip', 'DELETE', undefined],
    ['replacing a knockout player', req.replaceBlitzPlayer('us_open', '2026', 102, 1020, { out: 7, in: 11 }), '/api/blitz/us_open/2026/rounds/102/matches/1020/replacement', 'PUT', { out: 7, in: 11 }],
    ['a blitz format', req.setBlitzFormat('us_open', '2026', { swiss_rounds: 6, knockout_size: 4 }), '/api/blitz/us_open/2026', 'PATCH', { swiss_rounds: 6, knockout_size: 4 }],
    ["an edition's points", req.setBlitzPoints('us_open', '2026', { win: 3, draw: 1, loss: 0, forfeit_win: 3, forfeit_loss: 0, bye: 3, skip: 0 }), '/api/blitz/us_open/2026/points', 'PUT', { win: 3, draw: 1, loss: 0, forfeit_win: 3, forfeit_loss: 0, bye: 3, skip: 0 }],
    ["FIDE's points again", req.resetBlitzPoints('us_open', '2026'), '/api/blitz/us_open/2026/points', 'DELETE', undefined],
    ['pairing a round', req.pairBlitzRound('us_open', '2026', { timezone: true, rating: false }), '/api/blitz/us_open/2026/rounds', 'POST', { timezone: true, rating: false }],
    ["a round's unpaired players", req.proposeBlitzPairing('us_open', '2026', 101, { timezone: true, rating: false, keep: [{ a: 7, b: 8 }] }), '/api/blitz/us_open/2026/rounds/101/pairing', 'POST', { timezone: true, rating: false, keep: [{ a: 7, b: 8 }] }],
    ["a round's matches by hand", req.setBlitzRoundMatches('us_open', '2026', 101, [{ a: 7, b: 8 }, { a: 9 }]), '/api/blitz/us_open/2026/rounds/101/matches', 'PUT', { matches: [{ a: 7, b: 8 }, { a: 9 }] }],
    ['publishing a round', req.publishBlitzRound('us_open', '2026', 101), '/api/blitz/us_open/2026/rounds/101/publish', 'POST', undefined],
    ['deleting a draft round', req.deleteBlitzRound('us_open', '2026', 101), '/api/blitz/us_open/2026/rounds/101', 'DELETE', undefined],
    ["a round's deadline", req.setBlitzRoundDeadline('us_open', '2026', 101, { day: '2026-10-19', time: '12:00', zone: 'UTC' }), '/api/blitz/us_open/2026/rounds/101/deadline', 'PUT', { day: '2026-10-19', time: '12:00', zone: 'UTC' }],
    ['an organiser added', req.addBlitzOrganiser('us_open', '2026', 21), '/api/blitz/us_open/2026/organisers', 'POST', { member: 21 }],
    ['an organiser removed', req.removeBlitzOrganiser('us_open', '2026', 21), '/api/blitz/us_open/2026/organisers/21', 'DELETE', undefined],
    ['a thread text, trimmed', req.setBlitzThreadText('us_open', '2026', { message: ' Hi {players}.\n', no_discord: ' {name}. ' }), '/api/blitz/us_open/2026/thread-text', 'PUT', { message: 'Hi {players}.', no_discord: '{name}.' }],
    ['the default thread text again', req.resetBlitzThreadText('us_open', '2026'), '/api/blitz/us_open/2026/thread-text', 'DELETE', undefined],
    // Admins, badges, titles
    ['an admin', req.setAdmin('100000000000000001', 'Ann', 'organiser'), '/api/admins/100000000000000001', 'PUT', { name: 'Ann', role: 'organiser' }],
    ['removing an admin', req.removeAdmin('100000000000000001'), '/api/admins/100000000000000001', 'DELETE', undefined],
    ['a new badge, trimmed', req.addBadge(' Founder ', ' Was there '), '/api/badges', 'POST', { label: 'Founder', description: 'Was there' }],
    ['a badge, trimmed', req.setBadge(7, ' Founder ', ''), '/api/badges/7', 'PUT', { label: 'Founder', description: '' }],
    ['removing a badge', req.removeBadge(7), '/api/badges/7', 'DELETE', undefined],
    ['giving a badge, trimmed, keeping its label and holders', req.giveBadge(BADGE, ' cy '), '/api/badges/7', 'PUT', { label: 'TD', description: 'Runs it', players: ['ann', 'bob', 'cy'] }],
    ['giving a badge to someone who holds it', req.giveBadge(BADGE, 'bob'), '/api/badges/7', 'PUT', { label: 'TD', description: 'Runs it', players: ['ann', 'bob'] }],
    ['taking a badge', req.takeBadge(BADGE, 'ann'), '/api/badges/7', 'PUT', { label: 'TD', description: 'Runs it', players: ['bob'] }],
    ['a title, the name escaped', req.setTitle(' ann b ', 'GM', ' Earned '), '/api/titles/ann%20b', 'PUT', { title: 'GM', reason: 'Earned' }],
    ['clearing a title', req.clearTitle('ann'), '/api/titles/ann', 'DELETE', undefined],
    // Rating runs
    ['starting a run', req.startRun(), '/api/runs', 'POST', undefined],
    ['stopping a run', req.stopRun(), '/api/runs/stop', 'POST', undefined],
    ['publishing a run', req.publishRun(12), '/api/runs/12/publish', 'POST', undefined],
    ['protecting a run', req.protectRun(12, true), '/api/runs/12', 'PATCH', { protected: true }],
    ['deleting a run', req.deleteRun(12), '/api/runs/12', 'DELETE', undefined],
  ])('sends %s', (_, got, path, method, json) => {
    expect(got).toEqual({ path, init: { method, json } });
  });
});

describe('blitzThreadsPath', () => {
  it("is a round's threads, under an edition to escape", () => {
    expect(req.blitzThreadsPath('us_open', 'Season 1', 101)).toBe('/api/blitz/us_open/Season%201/rounds/101/threads');
  });
});
