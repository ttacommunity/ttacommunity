/** The community leaderboard's 2025 edition. */
import type { Edition } from './types';

export const edition = {
  "info": {
    "id": "2025",
    "name": "2025 Final Archive",
    "title": "Season 2025 Official Community Leaderboard Final Archive",
    "short": "2025 Final",
    "badge": "🏆 2025 Final",
    "year": 2025,
    "wc": "a440",
    "wcDesc": "World Champion 2025"
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
          "desc": "World Championship (won by a440)"
        }
      ]
    },
    {
      "category": "International Championship (4-Player League)",
      "badgeClass": "badge-gm",
      "columns": [
        {
          "id": "ic_28",
          "name": "International Season 28",
          "short": "IC 28",
          "maxPts": 1000
        },
        {
          "id": "ic_29",
          "name": "International Season 29",
          "short": "IC 29",
          "maxPts": 1000
        },
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
        }
      ]
    },
    {
      "category": "Intermezzo Championship (3-Player League)",
      "badgeClass": "badge-m",
      "columns": [
        {
          "id": "im_25",
          "name": "Intermezzo Season 25",
          "short": "IM 25",
          "maxPts": 1000
        },
        {
          "id": "im_26",
          "name": "Intermezzo Season 26",
          "short": "IM 26",
          "maxPts": 1000
        },
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
        }
      ]
    },
    {
      "category": "Royal League",
      "badgeClass": "badge-p",
      "columns": [
        {
          "id": "rl_03",
          "name": "Royal League Season 3",
          "short": "RL 03",
          "maxPts": 1000
        },
        {
          "id": "rl_04",
          "name": "Royal League Season 4",
          "short": "RL 04",
          "maxPts": 1000
        },
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
      "category": "Invitational & Gala Cups",
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
        },
        {
          "id": "ldrb",
          "name": "Leaderboard Trophy",
          "short": "Trophy",
          "maxPts": 1000
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
      "desc": "World Championship"
    },
    {
      "id": "ic",
      "name": "International (4P)",
      "short": "International (4P)",
      "cols": [
        "ic_28",
        "ic_29",
        "ic_30",
        "ic_31"
      ],
      "maxPts": 4000,
      "badgeClass": "badge-gm",
      "desc": "Sum of S28–S31"
    },
    {
      "id": "im",
      "name": "Intermezzo (3P)",
      "short": "Intermezzo (3P)",
      "cols": [
        "im_25",
        "im_26",
        "im_27",
        "im_28"
      ],
      "maxPts": 4000,
      "badgeClass": "badge-m",
      "desc": "Sum of S25–S28"
    },
    {
      "id": "rl",
      "name": "Royal League",
      "short": "Royal League",
      "cols": [
        "rl_03",
        "rl_04",
        "rl_05",
        "rl_06"
      ],
      "maxPts": 4000,
      "badgeClass": "badge-p",
      "desc": "Sum of S03–S06"
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
      "id": "sl",
      "name": "Sodium Ladder",
      "short": "Sodium",
      "cols": [
        "sl"
      ],
      "maxPts": 1000,
      "badgeClass": "badge-rank",
      "desc": "Annual competitive ladder"
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
      "desc": "Annual competitive ladder"
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
      "desc": "Invitational cup"
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
    },
    {
      "id": "ldrb",
      "name": "Leaderboard Trophy",
      "short": "Trophy",
      "cols": [
        "ldrb"
      ],
      "maxPts": 1000,
      "badgeClass": "badge-wc",
      "desc": "Year-closing gala trophy"
    }
  ],
  "sheetOrder": [
    "wrld",
    "ic_28",
    "ic_29",
    "ic_30",
    "ic_31",
    "im_25",
    "im_26",
    "im_27",
    "im_28",
    "rl_03",
    "rl_04",
    "rl_05",
    "rl_06",
    "wmbl",
    "aus",
    "sl",
    "ml",
    "eif",
    "qd",
    "ldrb"
  ]
} satisfies Edition;
