package assetflow

import (
	"context"
	"testing"
)

type fakeStorage struct {
	found bool
}

func (f fakeStorage) GetBucket(context.Context, string) error    { return nil }
func (f fakeStorage) CreateBucket(context.Context, string) error { return nil }
func (f fakeStorage) PresignObject(context.Context, string, string, PresignInput) (PresignResult, error) {
	return PresignResult{URL: "https://uploads.example/signed"}, nil
}
func (f fakeStorage) HeadObject(context.Context, string, string) (HeadResult, error) {
	return HeadResult{Found: f.found}, nil
}

func TestCompleteUploadDecision(t *testing.T) {
	tests := []struct {
		name             string
		found            bool
		wantState        string
		wantProcessing   string
		wantNotification string
	}{
		{name: "bytes are not present", found: false, wantState: "awaiting_upload", wantProcessing: "not_scheduled", wantNotification: "not_scheduled"},
		{name: "bytes are present", found: true, wantState: "uploaded", wantProcessing: "queued", wantNotification: "queued_after_processing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflow := Workflow{Storage: fakeStorage{found: tt.found}, Bucket: "creator-assets"}
			got, err := workflow.CompleteUpload(context.Background(), "creator_7", "asset_42")
			if err != nil {
				t.Fatalf("CompleteUpload() error = %v", err)
			}
			if got.State != tt.wantState || got.Processing != tt.wantProcessing || got.SubscriberUpdate != tt.wantNotification {
				t.Fatalf("CompleteUpload() = %+v", got)
			}
		})
	}
}

func TestStartUploadCarriesAssetBoundary(t *testing.T) {
	workflow := Workflow{Storage: fakeStorage{}, Bucket: "creator-assets"}
	got, err := workflow.StartUpload(context.Background(), UploadRequest{CreatorID: "creator_7", AssetID: "asset_42", ContentType: "video/mp4", MaxBytes: 10485760})
	if err != nil {
		t.Fatalf("StartUpload() error = %v", err)
	}
	if got.Method != "PUT" || got.ObjectKey != "creators/creator_7/assets/asset_42" || got.Next != "/uploads/complete" {
		t.Fatalf("StartUpload() = %+v", got)
	}
}
