// The Discord bot's endpoint: Discord sends each use of our slash commands here, signed with the application's key.
package httpapi

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"ttacommunity/backend/internal/discordbot"
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

// AnswerDiscordInteraction answers one interaction Discord signed: its PING, /rating and its suggestions, or
// /myrating.
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
	default:
		return AnswerDiscordInteraction400JSONResponse{badRequestBody("unknown interaction")}, nil
	}
	return AnswerDiscordInteraction200JSONResponse{Body: reply, Headers: AnswerDiscordInteraction200ResponseHeaders{CacheControl: cacheNone}}, nil
}

// suggestPlayers offers the players whose names hold what was typed: none before anything is, or when the search fails.
func (s *server) suggestPlayers(ctx context.Context, i DiscordInteraction) DiscordReply {
	choices := []DiscordChoice{}
	if term := discordbot.SearchTerm(stringOption(i, "player")); term != "" {
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
	noPlayer := discordText("No rated player is named "+discordbot.EscapeMarkdown(name)+".", true)
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
