/** The community leaderboard's 2024 edition. */
import type { Edition } from './types';

export const edition = {
  "info": {
    "id": "2024",
    "name": "2024 Final Archive",
    "title": "Season 2024 Official Community Leaderboard Final Archive",
    "short": "2024 Final",
    "badge": "📜 2024 Final",
    "year": 2024,
    "wc": "Martin_Pecheur",
    "wcDesc": "World Champion 2024"
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
          "desc": "World Championship (won by Martin_Pecheur)"
        }
      ]
    },
    {
      "category": "International Championship (4-Player League)",
      "badgeClass": "badge-gm",
      "columns": [
        {
          "id": "ic_24",
          "name": "International Season 24",
          "short": "IC 24",
          "maxPts": 1000
        },
        {
          "id": "ic_25",
          "name": "International Season 25",
          "short": "IC 25",
          "maxPts": 1000
        },
        {
          "id": "ic_26",
          "name": "International Season 26",
          "short": "IC 26",
          "maxPts": 1000
        },
        {
          "id": "ic_27",
          "name": "International Season 27",
          "short": "IC 27",
          "maxPts": 1000
        }
      ]
    },
    {
      "category": "Intermezzo Championship (3-Player League)",
      "badgeClass": "badge-m",
      "columns": [
        {
          "id": "im_21",
          "name": "Intermezzo Season 21",
          "short": "IM 21",
          "maxPts": 1000
        },
        {
          "id": "im_22",
          "name": "Intermezzo Season 22",
          "short": "IM 22",
          "maxPts": 1000
        },
        {
          "id": "im_23",
          "name": "Intermezzo Season 23",
          "short": "IM 23",
          "maxPts": 1000
        },
        {
          "id": "im_24",
          "name": "Intermezzo Season 24",
          "short": "IM 24",
          "maxPts": 1000
        }
      ]
    },
    {
      "category": "Royal League",
      "badgeClass": "badge-p",
      "columns": [
        {
          "id": "rl_01",
          "name": "Royal League Season 1",
          "short": "RL 01",
          "maxPts": 1000
        },
        {
          "id": "rl_02",
          "name": "Royal League Season 2",
          "short": "RL 02",
          "maxPts": 1000
        }
      ]
    },
    {
      "category": "Grand Slams & Annual Cups",
      "badgeClass": "badge-g",
      "columns": [
        {
          "id": "snail",
          "name": "Snail Cup",
          "short": "Snail Cup",
          "maxPts": 1000
        },
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
          "id": "ml",
          "name": "Mercurial Ladder",
          "short": "Mercurial",
          "maxPts": 1000
        }
      ]
    },
    {
      "category": "Invitational Cups",
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
      "desc": "World Championship"
    },
    {
      "id": "ic",
      "name": "International (4P)",
      "short": "International (4P)",
      "cols": [
        "ic_24",
        "ic_25",
        "ic_26",
        "ic_27"
      ],
      "maxPts": 4000,
      "badgeClass": "badge-gm",
      "desc": "Sum of S24–S27"
    },
    {
      "id": "im",
      "name": "Intermezzo (3P)",
      "short": "Intermezzo (3P)",
      "cols": [
        "im_21",
        "im_22",
        "im_23",
        "im_24"
      ],
      "maxPts": 4000,
      "badgeClass": "badge-m",
      "desc": "Sum of S21–S24"
    },
    {
      "id": "rl",
      "name": "Royal League",
      "short": "Royal League",
      "cols": [
        "rl_01",
        "rl_02"
      ],
      "maxPts": 2000,
      "badgeClass": "badge-p",
      "desc": "Sum of S01–S02"
    },
    {
      "id": "snail",
      "name": "Snail Cup",
      "short": "Snail Cup",
      "cols": [
        "snail"
      ],
      "maxPts": 1000,
      "badgeClass": "badge-g",
      "desc": "Annual championship cup"
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
    }
  ],
  "sheetOrder": [
    "wrld",
    "ic_24",
    "ic_25",
    "ic_26",
    "ic_27",
    "im_21",
    "im_22",
    "im_23",
    "im_24",
    "rl_01",
    "rl_02",
    "snail",
    "wmbl",
    "aus",
    "eif",
    "ml",
    "qd"
  ]
} satisfies Edition;
