/** The community leaderboard's 2026 edition. */
import type { Edition } from './types';

export const edition = {
  "info": {
    "id": "2026",
    "name": "2026 (Rolling Q2)",
    "title": "Official Community Leaderboard • Q2 2026 Cycle",
    "short": "2026 Q2",
    "badge": "⭐ Current Rolling",
    "year": 2026,
    "wc": "a440",
    "wcDesc": "Reigning World Champion (2025)"
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
          "desc": "Biennial World Championship (won by a440 in 2025)"
        }
      ]
    },
    {
      "category": "International Championship (4-Player League)",
      "badgeClass": "badge-gm",
      "columns": [
        {
          "id": "ic_30",
          "name": "International Season 30",
          "short": "IC 30",
          "maxPts": 1000
        },
        {
          "id": "ic_31",
          "name": "International Season 31",
          "short": "IC 31",
          "maxPts": 1000
        },
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
      "category": "Intermezzo Championship (3-Player League)",
      "badgeClass": "badge-m",
      "columns": [
        {
          "id": "im_27",
          "name": "Intermezzo Season 27",
          "short": "IM 27",
          "maxPts": 1000
        },
        {
          "id": "im_28",
          "name": "Intermezzo Season 28",
          "short": "IM 28",
          "maxPts": 1000
        },
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
          "id": "rl_05",
          "name": "Royal League Season 5",
          "short": "RL 05",
          "maxPts": 1000
        },
        {
          "id": "rl_06",
          "name": "Royal League Season 6",
          "short": "RL 06",
          "maxPts": 1000
        },
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
      "category": "Competitive Ladders",
      "badgeClass": "badge-rank",
      "columns": [
        {
          "id": "sl",
          "name": "Sodium Ladder",
          "short": "Sodium",
          "maxPts": 1000
        },
        {
          "id": "ml",
          "name": "Mercurial Ladder",
          "short": "Mercurial",
          "maxPts": 1000
        }
      ]
    },
    {
      "category": "Invitational & Rapid Cups",
      "badgeClass": "badge-rank",
      "columns": [
        {
          "id": "eif",
          "name": "Eiffel Tower Cup",
          "short": "Eiffel",
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
      "desc": "Biennial World Championship (won by a440 in 2025)"
    },
    {
      "id": "ic",
      "name": "International (4P)",
      "short": "International (4P)",
      "cols": [
        "ic_30",
        "ic_31",
        "ic_32",
        "ic_33"
      ],
      "maxPts": 4000,
      "badgeClass": "badge-gm",
      "desc": "Sum of 4 rolling seasons (S30–S33)"
    },
    {
      "id": "im",
      "name": "Intermezzo (3P)",
      "short": "Intermezzo (3P)",
      "cols": [
        "im_27",
        "im_28",
        "im_29",
        "im_30"
      ],
      "maxPts": 4000,
      "badgeClass": "badge-m",
      "desc": "Sum of 4 rolling seasons (S27–S30)"
    },
    {
      "id": "rl",
      "name": "Royal League",
      "short": "Royal League",
      "cols": [
        "rl_05",
        "rl_06",
        "rl_07",
        "rl_08"
      ],
      "maxPts": 4000,
      "badgeClass": "badge-p",
      "desc": "Sum of 4 rolling seasons (S05–S08)"
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
      "desc": "Premier Grand Slam open event"
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
      "desc": "Premier Grand Slam open event"
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
      "desc": "Tiered annual competitive ladder"
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
      "desc": "Tiered annual competitive ladder"
    },
    {
      "id": "eif",
      "name": "Eiffel Tower Cup",
      "short": "Eiffel",
      "cols": [
        "eif"
      ],
      "maxPts": 1000,
      "badgeClass": "badge-rank",
      "desc": "Invitational championship cup"
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
      "desc": "Fast-paced Swiss trophy tournament"
    }
  ],
  "sheetOrder": [
    "wrld",
    "ic_30",
    "ic_31",
    "ic_32",
    "ic_33",
    "im_27",
    "im_28",
    "im_29",
    "im_30",
    "rl_05",
    "rl_06",
    "rl_07",
    "rl_08",
    "wmbl",
    "aus",
    "sl",
    "ml",
    "eif",
    "qd"
  ]
} satisfies Edition;
