// The Discord bot's endpoint: Discord sends each use of our slash commands here, signed with the application's key.
package httpapi

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"

	"ttacommunity/backend/internal/discordbot"
	"ttacommunity/backend/internal/ratingdelta"
	"ttacommunity/backend/internal/store"
)

// DiscordApp is what the bot's endpoint needs: the application's key, the site replies link to, and what a Discord
// account is to us.
type DiscordApp interface {
	PublicKey() ed25519.PublicKey
	Site() string
	DiscordMember(ctx context.Context, discordID string) (store.DiscordMember, error)
}

const discordPrefix = "/v1/discord/"

// discordSuggestions is how many players a name being typed suggests, the strongest first.
const discordSuggestions = 5

// discordAnswerTime is how long a reply may take to build: Discord gives up on an answer after 3 seconds.
const discordAnswerTime = 2500 * time.Millisecond

// What Discord sends and the answers it takes, by number; ephemeral shows a message to the asker alone.
const (
	discordPing, discordCommand, discordAutocomplete = 1, 2, 4
	discordPong, discordMessage, discordChoices      = 1, 4, 8
	discordEphemeral                                 = 64
)

// discordGate lets through only requests Discord signed with the application's key, putting the body back for the
// handler. With discord nil, no application is set up here and the endpoint is a 404.
func discordGate(next http.Handler, discord DiscordApp, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", cacheNone)
		if discord == nil {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			badRequest(w, r, err)
			return
		}
		if !discordbot.Verify(discord.PublicKey(), r.Header.Get("X-Signature-Ed25519"), r.Header.Get("X-Signature-Timestamp"), body) {
			log.Info("Discord signature refused")
			writeError(w, http.StatusUnauthorized, "invalid request signature")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		next.ServeHTTP(w, r)
	})
}

// routeDiscord sends the bot's endpoint through discord, without the public routes' CORS, and the rest to rest.
func routeDiscord(rest, discord http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, discordPrefix) {
			discord.ServeHTTP(w, r)
			return
		}
		rest.ServeHTTP(w, r)
	})
}

// AnswerDiscordInteraction answers one interaction Discord signed: its PING, a command, or names for the player
// option being typed.
func (s *server) AnswerDiscordInteraction(ctx context.Context, req AnswerDiscordInteractionRequestObject) (AnswerDiscordInteractionResponseObject, error) {
	ctx, cancel := context.WithTimeout(ctx, discordAnswerTime)
	defer cancel()
	var reply DiscordReply
	switch i := req.Body; {
	case i == nil:
		return AnswerDiscordInteraction400JSONResponse{badRequestBody("no interaction")}, nil
	case i.Type == discordPing:
		reply = DiscordReply{Type: discordPong}
	case i.Type == discordAutocomplete:
		reply = s.suggestPlayers(ctx, *i)
	case i.Type == discordCommand && commandName(*i) == "rating":
		reply = s.ratingReply(ctx, *i)
	case i.Type == discordCommand && commandName(*i) == "myrating":
		reply = s.myRatingReply(ctx, *i)
	case i.Type == discordCommand && commandName(*i) == "roll":
		reply = rollReply(*i)
	case i.Type == discordCommand && commandName(*i) == "versus":
		reply = s.versusReply(ctx, *i)
	case i.Type == discordCommand && commandName(*i) == "rivals":
		reply = s.rivalsReply(ctx, *i)
	case i.Type == discordCommand && commandName(*i) == "compare":
		reply = s.compareReply(ctx, *i)
	case i.Type == discordCommand && commandName(*i) == "seats":
		reply = s.seatsReply(ctx, *i)
	case i.Type == discordCommand && commandName(*i) == "upsets":
		reply = s.upsetsReply(ctx, *i)
	default:
		return AnswerDiscordInteraction400JSONResponse{badRequestBody("unknown interaction")}, nil
	}
	return AnswerDiscordInteraction200JSONResponse{Body: reply, Headers: AnswerDiscordInteraction200ResponseHeaders{CacheControl: cacheNone}}, nil
}

// suggestPlayers offers the players whose names hold what was typed: none before anything is, or when the search fails.
func (s *server) suggestPlayers(ctx context.Context, i DiscordInteraction) DiscordReply {
	choices := []DiscordChoice{}
	if term := discordbot.SearchTerm(focusedOption(i)); term != "" {
		run, err := s.ratings.CurrentRun(ctx)
		var names []string
		if err == nil {
			names, err = s.ratings.PlayerNamesLike(ctx, run.ID, term, discordSuggestions)
		}
		if err != nil {
			s.log.Error("suggesting players for Discord failed", "request_id", requestID(ctx), "err", err)
			names = nil
		}
		for _, name := range names {
			choices = append(choices, DiscordChoice{Name: name, Value: name})
		}
	}
	return DiscordReply{Type: discordChoices, Data: &DiscordReplyData{Choices: &choices}}
}

// ratingReply shows the named player, or says why it cannot.
func (s *server) ratingReply(ctx context.Context, i DiscordInteraction) DiscordReply {
	name := strings.TrimSpace(stringOption(i, "player"))
	if name == "" {
		return discordText("Give a player's name: `/rating <name>`", true)
	}
	return s.showRating(ctx, name, formatOption(i), shareOption(i))
}

// myRatingReply shows the asker's own rating, found by the Discord account Discord signed the request for, or says
// why it cannot.
func (s *server) myRatingReply(ctx context.Context, i DiscordInteraction) DiscordReply {
	id := callerID(i)
	if id == "" {
		return discordText("Discord did not say who asked: look up a name with `/rating <name>`", true)
	}
	m, err := s.discord.DiscordMember(ctx, id)
	if err != nil {
		return s.notAnswering(ctx, err)
	}
	if problem := discordbot.OwnRatingProblem(m.State, s.discord.Site()); problem != "" {
		return discordText(problem, true)
	}
	return s.showRating(ctx, m.Name, formatOption(i), shareOption(i))
}

// showRating shows name's rating in format to the asker alone, or to everyone when shared, or says to the asker
// alone why it cannot.
func (s *server) showRating(ctx context.Context, name string, format int, shared bool) DiscordReply {
	noPlayer := noPlayerReply(name)
	if playerRefusal(1, name, N0) != "" {
		return noPlayer
	}
	run, err := s.ratings.CurrentRun(ctx)
	if err != nil {
		return s.notAnswering(ctx, err)
	}
	p, err := s.ratings.Player(ctx, run.ID, name)
	if errors.Is(err, store.ErrPlayerNotFound) {
		return noPlayer
	}
	if err != nil {
		return s.notAnswering(ctx, err)
	}
	summary, ok := summarise(p, format)
	if !ok {
		return discordText(discordbot.EscapeMarkdown(p.Name)+" has no "+discordbot.FormatLabel(format)+" rating.", true)
	}
	return discordText(discordbot.RatingText(summary, s.discord.Site()), !shared)
}

// rollReply rolls the dice asked for, for everyone to see unless asked private, or tells the asker alone why not.
func rollReply(i DiscordInteraction) DiscordReply {
	d, err := discordbot.ParseDice(stringOption(i, "dice"))
	if err != nil {
		return discordText("Those dice can't be rolled: "+err.Error()+".", true)
	}
	private, _ := option(i, "private").(bool)
	rolls := discordbot.RollDice(d, func(sides int) int { return rand.IntN(sides) + 1 })
	return discordText(discordbot.DiceText(d, rolls), private)
}

// versusReply shows two players' record in their rated games together, as the site's head-to-head counts it, or
// tells the asker alone why it cannot.
func (s *server) versusReply(ctx context.Context, i DiscordInteraction) DiscordReply {
	name, other := strings.TrimSpace(stringOption(i, "player")), strings.TrimSpace(stringOption(i, "other"))
	switch {
	case name == "" || other == "":
		return discordText("Give two players' names: `/versus <player> <other>`", true)
	case strings.EqualFold(name, other):
		return discordText("Pick two different players.", true)
	}
	noPlayer := discordText("No rated player is named "+discordbot.EscapeMarkdown(name)+" or "+discordbot.EscapeMarkdown(other)+".", true)
	if playerRefusal(1, name, N0) != "" || playerRefusal(1, other, N0) != "" {
		return noPlayer
	}
	run, err := s.ratings.CurrentRun(ctx)
	if err != nil {
		return s.notAnswering(ctx, err)
	}
	name, other, shared, err := s.ratings.Versus(ctx, run.ID, name, other, true, false)
	if errors.Is(err, store.ErrPlayerNotFound) {
		return noPlayer
	}
	if err != nil {
		return s.notAnswering(ctx, err)
	}
	meetings := make([]discordbot.Meeting, 0, len(shared))
	for _, m := range shared {
		a, b := placesOf(placed(m.Seats), name, other)
		meetings = append(meetings, discordbot.Meeting{Date: m.Date, Tournament: m.Tournament, PlayerCount: m.PlayerCount, NamePlace: a, OtherPlace: b})
	}
	format := formatOption(i)
	return discordText(discordbot.VersusText(name, other, format, discordbot.TallyVersus(meetings, format), s.discord.Site()), !shareOption(i))
}

// rivalsReply shows the opponents a player met most in rated games, as the site's rivals panel does, or tells the
// asker alone why it cannot.
func (s *server) rivalsReply(ctx context.Context, i DiscordInteraction) DiscordReply {
	name, run, refusal := s.askedPlayer(ctx, i, "rivals")
	if refusal != nil {
		return *refusal
	}
	format := formatOption(i)
	stored, got, err := s.ratings.PlayerRivals(ctx, run, name, format, true, false)
	if err != nil {
		return s.readFailed(ctx, err, name)
	}
	rivals := make([]discordbot.Rival, 0, len(got))
	for _, v := range got {
		rivals = append(rivals, discordbot.Rival{Name: v.Name, Games: v.Games, Wins: v.Wins, Losses: v.Losses, Draws: v.Draws, WinRate: rivalWinRate(v)})
	}
	return discordText(discordbot.RivalsText(stored, format, rivals, s.discord.Site()), !shareOption(i))
}

// compareReply shows two to four players' Glicko-D ratings in every format side by side, or tells the asker alone why
// it cannot.
func (s *server) compareReply(ctx context.Context, i DiscordInteraction) DiscordReply {
	var names []string
	for _, opt := range []string{"player1", "player2", "player3", "player4"} {
		name := strings.TrimSpace(stringOption(i, opt))
		if name == "" {
			continue
		}
		for _, seen := range names {
			if strings.EqualFold(seen, name) {
				return discordText("Pick different players.", true)
			}
		}
		names = append(names, name)
	}
	if len(names) < 2 {
		return discordText("Give two to four players' names: `/compare <player1> <player2>`", true)
	}
	run, err := s.ratings.CurrentRun(ctx)
	if err != nil {
		return s.notAnswering(ctx, err)
	}
	compared := make([]discordbot.Compared, 0, len(names))
	for _, name := range names {
		noPlayer := noPlayerReply(name)
		if playerRefusal(1, name, N0) != "" {
			return noPlayer
		}
		p, err := s.ratings.Player(ctx, run.ID, name)
		if errors.Is(err, store.ErrPlayerNotFound) {
			return noPlayer
		}
		if err != nil {
			return s.notAnswering(ctx, err)
		}
		c := discordbot.Compared{Name: p.Name, Games: p.Games[0], Ratings: map[int]discordbot.Summary{}}
		for _, format := range []int{0, 2, 3, 4} {
			if r, ok := summarise(p, format); ok {
				c.Title, c.Ratings[format] = r.Title, r
			}
		}
		compared = append(compared, c)
	}
	return discordText(discordbot.CompareText(compared), !shareOption(i))
}

// seatsReply shows a player's win rate from each seat in rated games, as the site's seat panel does, or tells the
// asker alone why it cannot.
func (s *server) seatsReply(ctx context.Context, i DiscordInteraction) DiscordReply {
	name, run, refusal := s.askedPlayer(ctx, i, "seats")
	if refusal != nil {
		return *refusal
	}
	stats, err := s.ratings.PlayerSeats(ctx, run, name, false)
	if err != nil {
		return s.readFailed(ctx, err, name)
	}
	groups := make([]discordbot.SeatGroup, 0, len(stats.Groups))
	for _, g := range stats.Groups {
		group := discordbot.SeatGroup{PlayerCount: g.PlayerCount, Games: g.TotalGames, WinRate: g.WinRate}
		for _, r := range g.Seats {
			group.Seats = append(group.Seats, discordbot.SeatRow{Seat: r.Seat, Games: r.Games, WinRate: r.WinRate, Delta: r.DeltaWinRate})
		}
		groups = append(groups, group)
	}
	return discordText(discordbot.SeatsText(stats.Name, formatOption(i), groups, s.discord.Site()), !shareOption(i))
}

// shownUpsets is how many upsets /upsets lists.
const shownUpsets = 5

// upsetsReply shows the biggest upsets in the month up to the current run's last game: favourites who lost to or
// drew with players they were expected to beat, by their Glicko-D ratings going in.
func (s *server) upsetsReply(ctx context.Context, i DiscordInteraction) DiscordReply {
	run, err := s.ratings.CurrentRun(ctx)
	if err != nil {
		return s.notAnswering(ctx, err)
	}
	format := formatOption(i)
	from, to, got, err := s.ratings.UpsetPairs(ctx, run.ID, format)
	if err != nil {
		return s.notAnswering(ctx, err)
	}
	pairs := make([]discordbot.Pair, 0, len(got))
	for _, p := range got {
		expected := ratingdelta.Expected(ratingdelta.Rating{Rating: p.Rating, RD: p.RD}, ratingdelta.Rating{Rating: p.OtherRating, RD: p.OtherRD})
		pairs = append(pairs, discordbot.Pair{GameID: p.GameID, Date: p.Date, Tournament: p.Tournament, PlayerCount: p.PlayerCount,
			Name: p.Name, Other: p.Other, Rating: p.Rating, OtherRating: p.OtherRating, Expected: expected, Score: p.Score, TimedOut: p.TimedOut})
	}
	return discordText(discordbot.UpsetsText(from, to, format, discordbot.Upsets(pairs, shownUpsets)), !shareOption(i))
}

// askedPlayer is the player option and the current run's id for a command about one player, or the reply to give
// the asker alone instead: for no name, a name no player could have, or no run to read.
func (s *server) askedPlayer(ctx context.Context, i DiscordInteraction, command string) (string, int64, *DiscordReply) {
	name := strings.TrimSpace(stringOption(i, "player"))
	if name == "" {
		r := discordText("Give a player's name: `/"+command+" <name>`", true)
		return "", 0, &r
	}
	if playerRefusal(1, name, N0) != "" {
		r := noPlayerReply(name)
		return "", 0, &r
	}
	run, err := s.ratings.CurrentRun(ctx)
	if err != nil {
		r := s.notAnswering(ctx, err)
		return "", 0, &r
	}
	return name, run.ID, nil
}

// readFailed is the reply when reading name's data failed: no such player, or the ratings not answering.
func (s *server) readFailed(ctx context.Context, err error, name string) DiscordReply {
	if errors.Is(err, store.ErrPlayerNotFound) {
		return noPlayerReply(name)
	}
	return s.notAnswering(ctx, err)
}

// noPlayerReply tells the asker alone that no rated player has name.
func noPlayerReply(name string) DiscordReply {
	return discordText("No rated player is named "+discordbot.EscapeMarkdown(name)+".", true)
}

// notAnswering logs why a reply could not be built and tells the asker alone to try again.
func (s *server) notAnswering(ctx context.Context, err error) DiscordReply {
	s.log.Error("answering Discord failed", "request_id", requestID(ctx), "err", err)
	return discordText("The ratings are not answering just now: try again in a minute.", true)
}

// summarise picks p's Glicko-D rating in format, with ok false when p has none there.
func summarise(p store.Player, format int) (discordbot.Summary, bool) {
	for _, r := range p.Ratings {
		if r.Model != string(Glicko2Daneo) || r.Format != format {
			continue
		}
		s := discordbot.Summary{Name: p.Name, Format: format, Rank: r.Rank, Rating: r.Rating, RD: r.RD, CRating: r.CRating,
			Games: p.Games[format], Opponents: r.Opponents, WinRate: r.WinRate, LastPlayed: p.LastPlayed}
		if p.Title != nil {
			s.Title = p.Title.Code
		}
		return s, true
	}
	return discordbot.Summary{}, false
}

// discordText is a message that mentions no one, whatever it says, shown to the asker alone when private.
func discordText(content string, private bool) DiscordReply {
	data := DiscordReplyData{Content: &content, AllowedMentions: &DiscordAllowedMentions{Parse: []string{}}}
	if private {
		flags := discordEphemeral
		data.Flags = &flags
	}
	return DiscordReply{Type: discordMessage, Data: &data}
}

// commandName is the name of the command used, or "" for none.
func commandName(i DiscordInteraction) string {
	if i.Data == nil || i.Data.Name == nil {
		return ""
	}
	return *i.Data.Name
}

// option is the value given for the option name, or nil.
func option(i DiscordInteraction, name string) any {
	if i.Data == nil || i.Data.Options == nil {
		return nil
	}
	for _, o := range *i.Data.Options {
		if o.Name == name {
			return o.Value
		}
	}
	return nil
}

// focusedOption is what has been typed so far in the option Discord asks suggestions for, or "".
func focusedOption(i DiscordInteraction) string {
	if i.Data == nil || i.Data.Options == nil {
		return ""
	}
	for _, o := range *i.Data.Options {
		if o.Focused != nil && *o.Focused {
			v, _ := o.Value.(string)
			return v
		}
	}
	return ""
}

func stringOption(i DiscordInteraction, name string) string {
	v, _ := option(i, name).(string)
	return v
}

// shareOption is whether the asker chose to share the reply with everyone.
func shareOption(i DiscordInteraction) bool {
	v, _ := option(i, "share").(bool)
	return v
}

// formatOption is the format option given, or every format together; JSON gives its number as a float64.
func formatOption(i DiscordInteraction) int {
	if f, ok := option(i, "format").(float64); ok && (f == 2 || f == 3 || f == 4) {
		return int(f)
	}
	return 0
}

// callerID is the Discord id of whoever used the command: member's in a server, user's in a direct message.
func callerID(i DiscordInteraction) string {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.Id
	}
	if i.User != nil {
		return i.User.Id
	}
	return ""
}
