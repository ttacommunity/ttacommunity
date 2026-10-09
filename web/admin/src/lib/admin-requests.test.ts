import { describe, expect, it } from 'vitest';
import * as req from './admin-requests';

describe('admin requests', () => {
  it.each([
    // Tournaments and groups
    ['a setting', req.setTournament('worlds', { visible: true }), '/api/tournaments/worlds', 'PATCH', { visible: true }],
    ['a move', req.moveInto('cge_42', 'mixed_duel'), '/api/tournaments/cge_42', 'PATCH', { parent: 'mixed_duel' }],
    ['a move out of every group', req.moveInto('cge_42', ''), '/api/tournaments/cge_42', 'PATCH', { parent: '' }],
    ['a new group', req.newGroup({ name: 'Swiss Open', kind: 'series' }), '/api/tournaments', 'POST', { name: 'Swiss Open', kind: 'series' }],
    ['a group\'s members', req.setMembers('swiss_open', ['cge_1817']), '/api/tournaments/swiss_open/members', 'PUT', { members: ['cge_1817'] }],
    ['a stage note, trimmed', req.setNote('worlds', '2024', 'Stage 1', ' Best of three '), '/api/tournaments/worlds/notes', 'PUT', { season: '2024', stage: 'Stage 1', note: 'Best of three' }],
    ['a time-out penalty', req.setPenalty('international', '30', -1), '/api/tournaments/international/season-penalty', 'PUT', { season: '30', timeout_penalty: -1 }],
    ['a podium', req.setPodium('worlds', { season: '2024', podium: [{ place: 1, name: 'ann' }], note: '' }), '/api/tournaments/worlds/podium', 'PUT', { season: '2024', podium: [{ place: 1, name: 'ann' }], note: '' }],
    ['clearing a podium', req.clearPodium('worlds', '2023 Play Off'), '/api/tournaments/worlds/podium?season=2023%20Play%20Off', 'DELETE', undefined],
    ['a card', req.setCard('worlds', { icon: '🏆', tagline: '', summary: '', subtitle: '', cadence: '' }), '/api/tournaments/worlds/card', 'PUT', { icon: '🏆', tagline: '', summary: '', subtitle: '', cadence: '' }],
    ['regulations', req.setRegulations('worlds', 'Best of three.'), '/api/tournaments/worlds/regulations', 'PUT', { text: 'Best of three.' }],
    ['stories', req.setStories('worlds', []), '/api/tournaments/worlds/stories', 'PUT', { stories: [] }],
    ['a new season, trimmed', req.addSeason('worlds', ' 2028 '), '/api/tournaments/worlds/seasons', 'POST', { label: '2028' }],
    ['removing a season', req.removeSeason('worlds', '2028 B'), '/api/tournaments/worlds/seasons?label=2028%20B', 'DELETE', undefined],
    ['a season\'s rating', req.setSeasonFlag('worlds', '2024', 'rated', false), '/api/tournaments/worlds/season-rating', 'PUT', { season: '2024', rated: false }],
    ['a season among the champions', req.setSeasonFlag('worlds', '2024', 'champions', true), '/api/tournaments/worlds/season-champions', 'PUT', { season: '2024', in_champions: true }],
    ['a season in the community leaderboard', req.setSeasonFlag('worlds', '2024', 'community', false), '/api/tournaments/worlds/season-community', 'PUT', { season: '2024', in_community: false }],
    // Admins, badges, titles
    ['an admin', req.setAdmin('100000000000000001', 'Ann', 'organiser'), '/api/admins/100000000000000001', 'PUT', { name: 'Ann', role: 'organiser' }],
    ['removing an admin', req.removeAdmin('100000000000000001'), '/api/admins/100000000000000001', 'DELETE', undefined],
    ['a new badge, trimmed', req.addBadge(' Founder ', ' Was there '), '/api/badges', 'POST', { label: 'Founder', description: 'Was there' }],
    ['a badge, trimmed', req.setBadge(7, ' Founder ', ''), '/api/badges/7', 'PUT', { label: 'Founder', description: '' }],
    ['removing a badge', req.removeBadge(7), '/api/badges/7', 'DELETE', undefined],
    ['giving a badge to a name to escape', req.giveBadge(7, ' ann/b?c '), '/api/badges/7/players/ann%2Fb%3Fc', 'PUT', undefined],
    ['taking a badge', req.takeBadge(7, 'ann'), '/api/badges/7/players/ann', 'DELETE', undefined],
    ['a title, the name escaped', req.setTitle(' ann b ', 'GM', ' Earned '), '/api/titles/ann%20b', 'PUT', { title: 'GM', reason: 'Earned' }],
    ['clearing a title', req.clearTitle('ann'), '/api/titles/ann', 'DELETE', undefined],
    // Rating runs
    ['starting a run', req.startRun(), '/api/runs', 'POST', undefined],
    ['stopping a run', req.stopRun(), '/api/runs/stop', 'POST', undefined],
    ['publishing a run', req.publishRun(12), '/api/runs/12/publish', 'POST', undefined],
    ['protecting a run', req.protectRun(12, true), '/api/runs/12/protected', 'PUT', { protected: true }],
    ['deleting a run', req.deleteRun(12), '/api/runs/12', 'DELETE', undefined],
  ])('sends %s', (_, got, path, method, json) => {
    expect(got).toEqual({ path, init: { method, json } });
  });
});
