/** The community leaderboard's race26 edition. */
import type { Edition } from './types';

export const edition = {
  "info": {
    "id": "race26",
    "name": "2026 Race Tracker",
    "title": "Season 2026 Points Race Tracker (Race 26)",
    "short": "Race 26",
    "badge": "🏁 2026 Race",
    "year": 2026,
    "wc": "a440",
    "wcDesc": "Reigning World Champion"
  },
  "categories": [
    {
      "category": "World Championship",
      "badgeClass": "badge-wc",
      "columns": [
        {
          "id": "wrld",
          "name": "World Championship",
          "short": "Worlds",
          "maxPts": 2500,
          "desc": "World Championship stage points"
        }
      ]
    },
    {
      "category": "International Championship (4-Player)",
      "badgeClass": "badge-gm",
      "columns": [
        {
          "id": "ic_32",
          "name": "International Season 32",
          "short": "IC 32",
          "maxPts": 1000
        },
        {
          "id": "ic_33",
          "name": "International Season 33",
          "short": "IC 33",
          "maxPts": 1000
        }
      ]
    },
    {
      "category": "Intermezzo Championship (3-Player)",
      "badgeClass": "badge-m",
      "columns": [
        {
          "id": "im_29",
          "name": "Intermezzo Season 29",
          "short": "IM 29",
          "maxPts": 1000
        },
        {
          "id": "im_30",
          "name": "Intermezzo Season 30",
          "short": "IM 30",
          "maxPts": 1000
        }
      ]
    },
    {
      "category": "Royal League",
      "badgeClass": "badge-p",
      "columns": [
        {
          "id": "rl_07",
          "name": "Royal League Season 7",
          "short": "RL 07",
          "maxPts": 1000
        },
        {
          "id": "rl_08",
          "name": "Royal League Season 8",
          "short": "RL 08",
          "maxPts": 1000
        }
      ]
    },
    {
      "category": "Grand Slams & Major Opens",
      "badgeClass": "badge-g",
      "columns": [
        {
          "id": "wmbl",
          "name": "Wimbledon Open",
          "short": "Wimbledon",
          "maxPts": 1000
        },
        {
          "id": "aus",
          "name": "Australian Open",
          "short": "Aussie Open",
          "maxPts": 1000
        }
      ]
    },
    {
      "category": "Competitive Cups & Ladders",
      "badgeClass": "badge-rank",
      "columns": [
        {
          "id": "surv",
          "name": "Survivors Cup",
          "short": "Survivors",
          "maxPts": 1000
        },
        {
          "id": "ml",
          "name": "Mercurial Ladder",
          "short": "Mercurial",
          "maxPts": 1000
        },
        {
          "id": "sl",
          "name": "Sodium Ladder",
          "short": "Sodium",
          "maxPts": 1000
        },
        {
          "id": "qd",
          "name": "Quick & Dirty (Swiss)",
          "short": "Q&D",
          "maxPts": 600
        }
      ]
    }
  ],
  "summary": [
    {
      "id": "wrld",
      "name": "World Championship",
      "short": "Worlds",
      "cols": [
        "wrld"
      ],
      "maxPts": 2500,
      "badgeClass": "badge-wc",
      "desc": "World Championship stage points"
    },
    {
      "id": "ic",
      "name": "International (4P)",
      "short": "International (4P)",
      "cols": [
        "ic_32",
        "ic_33"
      ],
      "maxPts": 2000,
      "badgeClass": "badge-gm",
      "desc": "2026 seasons S32–S33"
    },
    {
      "id": "im",
      "name": "Intermezzo (3P)",
      "short": "Intermezzo (3P)",
      "cols": [
        "im_29",
        "im_30"
      ],
      "maxPts": 2000,
      "badgeClass": "badge-m",
      "desc": "2026 seasons S29–S30"
    },
    {
      "id": "rl",
      "name": "Royal League",
      "short": "Royal League",
      "cols": [
        "rl_07",
        "rl_08"
      ],
      "maxPts": 2000,
      "badgeClass": "badge-p",
      "desc": "2026 seasons S07–S08"
    },
    {
      "id": "wmbl",
      "name": "Wimbledon Open",
      "short": "Wimbledon",
      "cols": [
        "wmbl"
      ],
      "maxPts": 1000,
      "badgeClass": "badge-g",
      "desc": "Premier Grand Slam"
    },
    {
      "id": "aus",
      "name": "Australian Open",
      "short": "Aussie Open",
      "cols": [
        "aus"
      ],
      "maxPts": 1000,
      "badgeClass": "badge-g",
      "desc": "Premier Grand Slam"
    },
    {
      "id": "surv",
      "name": "Survivors Cup",
      "short": "Survivors",
      "cols": [
        "surv"
      ],
      "maxPts": 1000,
      "badgeClass": "badge-rank",
      "desc": "Special championship cup"
    },
    {
      "id": "ml",
      "name": "Mercurial Ladder",
      "short": "Mercurial",
      "cols": [
        "ml"
      ],
      "maxPts": 1000,
      "badgeClass": "badge-rank",
      "desc": "2026 ladder rounds"
    },
    {
      "id": "sl",
      "name": "Sodium Ladder",
      "short": "Sodium",
      "cols": [
        "sl"
      ],
      "maxPts": 1000,
      "badgeClass": "badge-rank",
      "desc": "2026 ladder rounds"
    },
    {
      "id": "qd",
      "name": "Quick & Dirty (Swiss)",
      "short": "Q&D",
      "cols": [
        "qd"
      ],
      "maxPts": 600,
      "badgeClass": "badge-rank",
      "desc": "Swiss tournament"
    }
  ],
  "sheetOrder": [
    "wrld",
    "ic_32",
    "ic_33",
    "im_29",
    "im_30",
    "rl_07",
    "rl_08",
    "wmbl",
    "aus",
    "surv",
    "ml",
    "sl",
    "qd"
  ]
} satisfies Edition;
