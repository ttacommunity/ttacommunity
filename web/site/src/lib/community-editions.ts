/** The community leaderboard's editions as upstream's page lays them out (routes/community.py at cb0c422), and each edition's event columns in its sheet's order. */
import type { CommunityEdition } from '@ttac/shared/ratings';

export interface EventColumn { id: string; name: string; short: string; maxPts: number; desc?: string }
export interface Category { category: string; badgeClass: string; columns: EventColumn[] }
export interface Tournament { id: string; name: string; short: string; cols: string[]; maxPts: number; badgeClass: string; desc: string }
export interface Edition {
  info: { id: CommunityEdition; name: string; title: string; short: string; badge: string; year: number; wc: string; wcDesc: string };
  categories: Category[];
  summary: Tournament[];
  /** The event columns in the order the maintainer's sheet lists them, which breaks ties among a player's best 8. */
  sheetOrder: string[];
}

export const EDITIONS: Record<CommunityEdition, Edition> = {
  "2026": {
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
  },
  "2025": {
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
  },
  "2024": {
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
  },
  "race26": {
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
  }
};
