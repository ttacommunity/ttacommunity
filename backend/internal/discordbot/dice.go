// Dice for /roll: reading what was asked for, rolling it and the reply.
package discordbot

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Dice is a roll of Count dice of Sides sides each, with Modifier added to their total.
type Dice struct{ Count, Sides, Modifier int }

// The most a roll may ask for, so a reply stays short and a roll cheap.
const maxDice, maxSides, maxModifier = 100, 1000, 1000

var diceForm = regexp.MustCompile(`^(\d{0,3})d(\d{1,4})(?:([+-])(\d{1,4}))?$`)

// ParseDice reads dice written as NdM, NdM+K or NdM-K (N is 1 when left out), refusing more than 100 dice, fewer than
// 2 or more than 1000 sides, and a modifier over 1000.
func ParseDice(s string) (Dice, error) {
	m := diceForm.FindStringSubmatch(strings.ToLower(strings.ReplaceAll(s, " ", "")))
	if m == nil {
		return Dice{}, errors.New("write dice as 2d6, d20 or 3d8+2")
	}
	d := Dice{Count: 1}
	if m[1] != "" {
		d.Count, _ = strconv.Atoi(m[1])
	}
	d.Sides, _ = strconv.Atoi(m[2])
	if m[4] != "" {
		d.Modifier, _ = strconv.Atoi(m[4])
		if m[3] == "-" {
			d.Modifier = -d.Modifier
		}
	}
	switch {
	case d.Count < 1 || d.Count > maxDice:
		return Dice{}, fmt.Errorf("roll 1 to %d dice", maxDice)
	case d.Sides < 2 || d.Sides > maxSides:
		return Dice{}, fmt.Errorf("a die has 2 to %d sides", maxSides)
	case d.Modifier > maxModifier || d.Modifier < -maxModifier:
		return Dice{}, fmt.Errorf("add or take off at most %d", maxModifier)
	}
	return d, nil
}

// RollDice rolls each of d's dice with roll, which returns a number from 1 to the sides it is given.
func RollDice(d Dice, roll func(sides int) int) []int {
	rolls := make([]int, d.Count)
	for i := range rolls {
		rolls[i] = roll(d.Sides)
	}
	return rolls
}

// DiceText is the reply for rolls of d: what was rolled, each die, and the total in bold.
func DiceText(d Dice, rolls []int) string {
	total, each := d.Modifier, make([]string, len(rolls))
	for i, r := range rolls {
		total += r
		each[i] = strconv.Itoa(r)
	}
	asked := fmt.Sprintf("%dd%d", d.Count, d.Sides)
	if d.Modifier != 0 {
		asked += fmt.Sprintf("%+d", d.Modifier)
	}
	return fmt.Sprintf("🎲 %s: [%s] = **%d**", asked, strings.Join(each, ", "), total)
}
