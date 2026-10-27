package assetflow

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
)

var identifierPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

type Storage interface {
	GetBucket(context.Context, string) error
	CreateBucket(context.Context, string) error
	PresignObject(context.Context, string, string, PresignInput) (PresignResult, error)
	HeadObject(context.Context, string, string) (HeadResult, error)
}

type Workflow struct {
	Storage Storage
	Bucket  string
}

type UploadRequest struct {
	CreatorID   string `json:"creator_id"`
	AssetID     string `json:"asset_id"`
	ContentType string `json:"content_type"`
	MaxBytes    int64  `json:"max_bytes"`
}

type UploadTicket struct {
	AssetID   string `json:"asset_id"`
	ObjectKey string `json:"object_key"`
	Method    string `json:"method"`
	UploadURL string `json:"upload_url"`
	Next      string `json:"next"`
}

type Completion struct {
	AssetID          string `json:"asset_id"`
	State            string `json:"state"`
	Processing       string `json:"processing"`
	SubscriberUpdate string `json:"subscriber_update"`
}

func (w Workflow) EnsureBucket(ctx context.Context) error {
	err := w.Storage.GetBucket(ctx, w.Bucket)
	if err == nil {
		return nil
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.HTTPStatus != http.StatusNotFound {
		return err
	}
	return w.Storage.CreateBucket(ctx, w.Bucket)
}

func (w Workflow) StartUpload(ctx context.Context, input UploadRequest) (UploadTicket, error) {
	if !identifierPattern.MatchString(input.CreatorID) || !identifierPattern.MatchString(input.AssetID) {
		return UploadTicket{}, fmt.Errorf("creator_id and asset_id must use letters, digits, underscore, or dash")
	}
	if input.ContentType == "" || input.MaxBytes <= 0 {
		return UploadTicket{}, fmt.Errorf("content_type and a positive max_bytes are required")
	}
	key := fmt.Sprintf("creators/%s/assets/%s", input.CreatorID, input.AssetID)
	signed, err := w.Storage.PresignObject(ctx, w.Bucket, key, PresignInput{
		Operation:      "put",
		ExpiresSeconds: 600,
		ContentType:    input.ContentType,
		MaxBytes:       input.MaxBytes,
		IdempotencyKey: "asset-upload-" + input.AssetID,
	})
	if err != nil {
		return UploadTicket{}, err
	}
	return UploadTicket{AssetID: input.AssetID, ObjectKey: key, Method: http.MethodPut, UploadURL: signed.URL, Next: "/uploads/complete"}, nil
}

func (w Workflow) CompleteUpload(ctx context.Context, creatorID, assetID string) (Completion, error) {
	if !identifierPattern.MatchString(creatorID) || !identifierPattern.MatchString(assetID) {
		return Completion{}, fmt.Errorf("creator_id and asset_id must use letters, digits, underscore, or dash")
	}
	key := fmt.Sprintf("creators/%s/assets/%s", creatorID, assetID)
	head, err := w.Storage.HeadObject(ctx, w.Bucket, key)
	if err != nil {
		return Completion{}, err
	}
	if !head.Found {
		return Completion{AssetID: assetID, State: "awaiting_upload", Processing: "not_scheduled", SubscriberUpdate: "not_scheduled"}, nil
	}
	return Completion{AssetID: assetID, State: "uploaded", Processing: "queued", SubscriberUpdate: "queued_after_processing"}, nil
}
