package challenges

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// Placing is a name that won or came second in a week, and its domination.
type Placing struct {
	Name       string
	Domination int
}

// Week is one week of the weekly challenge: its number, the day it started, its challenge, and who won and came
// second (several when they tied).
type Week struct {
	Number    int
	Of        time.Time
	Challenge string
	Winners   []Placing
	RunnersUp []Placing
}

// oldWeek is a week as the old site's weekly.json has it.
type oldWeek struct {
	Number    int    `json:"number"`
	Date      string `json:"date"`
	Challenge string `json:"challengeId"`
	Winners   []struct {
		Name  string `json:"name"`
		Score int    `json:"score"`
	} `json:"winners"`
	RunnersUp []struct {
		Name  string `json:"name"`
		Score int    `json:"score"`
	} `json:"runnersUp"`
}

// ReadWeeks reads the old site's weekly.json, or refuses it whole: the load job replaces every week with it. Names
// have their spaces collapsed.
func ReadWeeks(r io.Reader) ([]Week, error) {
	var raw []oldWeek
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return nil, fmt.Errorf("the weeks are not a JSON list of weeks: %w", err)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("the file has no weeks")
	}
	seen := map[int]bool{}
	weeks := make([]Week, len(raw))
	for i, o := range raw {
		of, err := time.Parse(time.DateOnly, o.Date)
		switch {
		case o.Number < 1:
			return nil, fmt.Errorf("week %d: the number is not 1 or more", i+1)
		case seen[o.Number]:
			return nil, fmt.Errorf("week %d is listed twice", o.Number)
		case err != nil:
			return nil, fmt.Errorf("week %d: the date is not YYYY-MM-DD", o.Number)
		case o.Challenge == "":
			return nil, fmt.Errorf("week %d: no challenge", o.Number)
		}
		seen[o.Number] = true
		weeks[i] = Week{Number: o.Number, Of: of, Challenge: o.Challenge}
		for _, p := range o.Winners {
			weeks[i].Winners = append(weeks[i].Winners, Placing{Name: strings.Join(strings.Fields(p.Name), " "), Domination: p.Score})
		}
		for _, p := range o.RunnersUp {
			weeks[i].RunnersUp = append(weeks[i].RunnersUp, Placing{Name: strings.Join(strings.Fields(p.Name), " "), Domination: p.Score})
		}
		for _, p := range slices.Concat(weeks[i].Winners, weeks[i].RunnersUp) {
			if p.Name == "" || utf8.RuneCountInString(p.Name) > 100 {
				return nil, fmt.Errorf("week %d: a name is empty or over 100 characters", o.Number)
			}
			if p.Domination < MinDomination || p.Domination > MaxDomination {
				return nil, fmt.Errorf("week %d: %s's domination is not %d to %d", o.Number, p.Name, MinDomination, MaxDomination)
			}
		}
	}
	return weeks, nil
}
