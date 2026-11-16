// The YouTube hub's callback writes, as the player login through SECURITY DEFINER functions: confirming our
// subscription requests and storing or removing the videos it pushes.
package store

import (
	"context"
	"fmt"
	"time"
)

// PushedVideo is one video the hub pushed, with our verdict on whether it is about TTA.
type PushedVideo struct {
	VideoID          string
	ChannelID        string
	Title            string
	PublishedAt      time.Time
	ThumbnailURL     string
	TTA              bool
	Reason           string
	DetailsFetchedAt *time.Time
}

// ConfirmMovieHub reports whether the hub's check of a request is one we made: a subscribe for a linked channel
// or an unsubscribe for one no longer linked. A confirmed subscribe records its lease of leaseSeconds from at.
func (s *PlayerStore) ConfirmMovieHub(ctx context.Context, channelID, mode string, leaseSeconds int, at time.Time) (bool, error) {
	var ok bool
	if err := s.pool.QueryRow(ctx, "SELECT confirm_movie_hub($1, $2, $3, $4)", channelID, mode, leaseSeconds, at).Scan(&ok); err != nil {
		return false, fmt.Errorf("confirming a hub request: %w", err)
	}
	return ok, nil
}

// StorePushedVideo stores v, or updates its title and verdict, keeping an admin's override. It returns
// ErrNoChannel when v's channel is not linked.
func (s *PlayerStore) StorePushedVideo(ctx context.Context, v PushedVideo, at time.Time) error {
	var answer string
	err := s.pool.QueryRow(ctx, "SELECT store_pushed_video($1, $2, $3, $4, $5, $6, $7, $8, $9)",
		v.VideoID, v.ChannelID, v.Title, v.PublishedAt, v.ThumbnailURL, v.TTA, v.Reason, v.DetailsFetchedAt, at).Scan(&answer)
	if err != nil {
		return fmt.Errorf("storing a pushed video: %w", err)
	}
	if answer == "no-channel" {
		return ErrNoChannel
	}
	return nil
}

// RemovePushedVideo drops a video the hub says channelID deleted.
func (s *PlayerStore) RemovePushedVideo(ctx context.Context, videoID, channelID string, at time.Time) error {
	if _, err := s.pool.Exec(ctx, "SELECT remove_pushed_video($1, $2, $3)", videoID, channelID, at); err != nil {
		return fmt.Errorf("removing a pushed video: %w", err)
	}
	return nil
}
