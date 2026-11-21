// Images in buckets (Cloudflare R2): admins and organisers upload, list and delete them, and anyone lists the
// assets bucket. Every image is checked by imagestore.Check, and named by its content.
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"

	"ttacommunity/backend/internal/imagestore"
)

// adminImagesPrefix is where the image routes sit, whose uploads may be larger than other bodies.
const adminImagesPrefix = adminPrefix + "images/"

// ImageStore stores, lists and deletes one bucket's images, as imagestore.Bucket does.
type ImageStore interface {
	Put(ctx context.Context, img imagestore.Image) (string, error)
	List(ctx context.Context) ([]imagestore.Object, error)
	Delete(ctx context.Context, key string) error
}

const imagesNotSetUp = "this bucket is not set up here"

// bucket returns the named bucket: found false for a name the API does not know, and a nil store for a bucket
// not set up here.
func (s *server) bucket(name ImageBucket) (ImageStore, bool) {
	if !name.Valid() {
		return nil, false
	}
	return s.images[name], true
}

func storedImages(list []imagestore.Object) []StoredImage {
	out := make([]StoredImage, len(list))
	for i, o := range list {
		out[i] = StoredImage{Key: o.Key, Url: o.URL, Size: o.Size, Uploaded: o.Uploaded.UTC()}
	}
	return out
}

func (s *server) GetAdminImages(ctx context.Context, req GetAdminImagesRequestObject) (GetAdminImagesResponseObject, error) {
	b, known := s.bucket(req.Bucket)
	switch {
	case !known:
		return GetAdminImages404JSONResponse{notFound404("no such bucket")}, nil
	case b == nil:
		return GetAdminImages503JSONResponse{unavailable503(imagesNotSetUp)}, nil
	}
	list, err := b.List(ctx)
	if err != nil {
		return nil, err
	}
	return GetAdminImages200JSONResponse{Body: storedImages(list), Headers: GetAdminImages200ResponseHeaders{CacheControl: cacheNone}}, nil
}

// PostAdminImage checks the uploaded image and stores it, with its audit: the audit is written first, so an image
// the bucket refuses leaves none.
func (s *server) PostAdminImage(ctx context.Context, req PostAdminImageRequestObject) (PostAdminImageResponseObject, error) {
	me, ok := adminFrom(ctx)
	if !ok {
		return nil, errors.New("admin route reached without the gate")
	}
	b, known := s.bucket(req.Bucket)
	switch {
	case !known:
		return PostAdminImage404JSONResponse{notFound404("no such bucket")}, nil
	case b == nil:
		return PostAdminImage503JSONResponse{unavailable503(imagesNotSetUp)}, nil
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, imagestore.MaxSize+1))
	if err != nil {
		return PostAdminImage400JSONResponse{badRequest400("the upload could not be read")}, nil
	}
	img, err := imagestore.Check(req.Params.Name, body)
	switch {
	case errors.Is(err, imagestore.ErrTooLarge):
		return PostAdminImage413JSONResponse{Body: Error{Error: "the image is over 5 MB"}, Headers: PostAdminImage413ResponseHeaders{CacheControl: cacheNone}}, nil
	case errors.Is(err, imagestore.ErrNotImage):
		return PostAdminImage400JSONResponse{badRequest400("a PNG, JPEG, GIF or WebP image")}, nil
	case err != nil:
		return nil, err
	}
	var url string
	err = s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.Audit(ctx, me.DiscordID, "image.upload", string(req.Bucket)+"/"+img.Key, map[string]any{"size": len(img.Body), "type": img.ContentType}); err != nil {
			return err
		}
		url, err = b.Put(ctx, img)
		return err
	})
	switch {
	case errors.Is(err, imagestore.ErrExists):
		return PostAdminImage409JSONResponse{conflict409("this image is stored already, as " + img.Key)}, nil
	case err != nil:
		return nil, fmt.Errorf("storing an image in %s: %w", req.Bucket, err)
	}
	return PostAdminImage201JSONResponse{
		Body:    StoredImage{Key: img.Key, Url: url, Size: int64(len(img.Body)), Uploaded: s.now().UTC()},
		Headers: PostAdminImage201ResponseHeaders{CacheControl: cacheNone},
	}, nil
}

// DeleteAdminImage deletes an image, with its audit, written first as PostAdminImage's.
func (s *server) DeleteAdminImage(ctx context.Context, req DeleteAdminImageRequestObject) (DeleteAdminImageResponseObject, error) {
	me, ok := adminFrom(ctx)
	if !ok {
		return nil, errors.New("admin route reached without the gate")
	}
	b, known := s.bucket(req.Bucket)
	switch {
	case !known:
		return DeleteAdminImage404JSONResponse{notFound404("no such bucket")}, nil
	case b == nil:
		return DeleteAdminImage503JSONResponse{unavailable503(imagesNotSetUp)}, nil
	case !imagestore.ValidKey(req.Key):
		return DeleteAdminImage400JSONResponse{badRequest400("not an image's key")}, nil
	}
	err := s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.Audit(ctx, me.DiscordID, "image.delete", string(req.Bucket)+"/"+req.Key, map[string]any{}); err != nil {
			return err
		}
		return b.Delete(ctx, req.Key)
	})
	if err != nil {
		return nil, fmt.Errorf("deleting an image from %s: %w", req.Bucket, err)
	}
	return DeleteAdminImage204Response{}, nil
}

// GetAssets lists the assets bucket for anyone: a 404 when it is not set up here, a 503 when it cannot be listed.
func (s *server) GetAssets(ctx context.Context, _ GetAssetsRequestObject) (GetAssetsResponseObject, error) {
	b := s.images[Assets]
	if b == nil {
		return GetAssets404JSONResponse{Body: Error{Error: "this API has no assets bucket"}, Headers: GetAssets404ResponseHeaders{CacheControl: cacheNone}}, nil
	}
	list, err := b.List(ctx)
	if err != nil {
		s.log.Error("listing the assets failed", "err", err)
		return GetAssets503JSONResponse{unavailable503("the assets cannot be listed now")}, nil
	}
	return GetAssets200JSONResponse{Body: storedImages(list), Headers: GetAssets200ResponseHeaders{CacheControl: cacheCurrent}}, nil
}
