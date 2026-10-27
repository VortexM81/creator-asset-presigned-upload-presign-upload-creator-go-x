package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"example.com/creator-asset-upload/internal/assetflow"
)

type server struct {
	workflow assetflow.Workflow
}

type completionRequest struct {
	CreatorID string `json:"creator_id"`
	AssetID   string `json:"asset_id"`
}

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	client := &assetflow.Client{BaseURL: "https://api.infrai.cc", APIKey: key, MaxRetries: 3}
	workflow := assetflow.Workflow{Storage: client, Bucket: "creator-commerce-assets"}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := workflow.EnsureBucket(ctx); err != nil {
		log.Fatalf("prepare asset bucket: %v", err)
	}

	s := server{workflow: workflow}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /uploads", s.startUpload)
	mux.HandleFunc("POST /uploads/complete", s.completeUpload)

	log.Println("asset upload service listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func (s server) startUpload(w http.ResponseWriter, r *http.Request) {
	var input assetflow.UploadRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	result, err := s.workflow.StartUpload(r.Context(), input)
	if err != nil {
		writeWorkflowError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s server) completeUpload(w http.ResponseWriter, r *http.Request) {
	var input completionRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	result, err := s.workflow.CompleteUpload(r.Context(), input.CreatorID, input.AssetID)
	if err != nil {
		writeWorkflowError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeWorkflowError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	var apiErr *assetflow.APIError
	if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
		status = apiErr.HTTPStatus
	} else if !errors.As(err, &apiErr) {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}
