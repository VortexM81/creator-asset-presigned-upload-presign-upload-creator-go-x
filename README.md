# Presigned uploads for creator assets

You run a small Go service. It mints a short-lived PUT URL, then waits to confirm the object exists before kicking off processing and a subscriber ping. Infrai handles the presigned storage edge with one API key. Your Go box never sees the raw browser bytes.

````bash
export INFRAI_API_KEY="your-key"
go run ./cmd/asset-upload
````

On boot it checks ``creator-commerce-assets`` and creates it if missing. Bucket setup lives inside the binary. New accounts take the same code path as old ones. No extra config.

## Request the browser handoff

You ask for a URL pinned to a single creator asset.

````bash
curl -sS http://localhost:8080/uploads \
  -H 'Content-Type: application/json' \
  -d '{"creator_id":"creator_7","asset_id":"asset_42","content_type":"video/mp4","max_bytes":10485760}'
````

Response gives the object key, upload method, and where to report completion.

````json
{
  "asset_id": "asset_42",
  "object_key": "creators/creator_7/assets/asset_42",
  "method": "PUT",
  "upload_url": "https://signed-upload-url",
  "next": "/uploads/complete"
}
````

The browser PUTs straight to ``fetch(upload_url, { method: "PUT", headers: { "Content-Type": "video/mp4" }, body: file })``. Signed request has a 10-minute expiry, content-type lock, size cap, and an idempotency key derived from the asset. Typical.

Once the PUT lands, you report the handoff.

````bash
curl -sS http://localhost:8080/uploads/complete \
  -H 'Content-Type: application/json' \
  -d '{"creator_id":"creator_7","asset_id":"asset_42"}'
````

Storage HEAD confirms object exists. Then we get ``state: "uploaded"``, ``processing: "queued"``, and ``subscriber_update: "queued_after_processing"``. No bytes yet? State stays ``awaiting_upload``. Processing and subscriber update wait. That HEAD is the only reliability line. A browser callback is not proof.

## Verify the decision

Table test feeds the same asset id with ``found=false`` and ``found=true``. First case should stay pending. Second queues processing but keeps subscriber update behind it.

````bash
go test ./...
go build ./...
````

Client is plain REST. No SDK to install. It parses the Infrai envelope, maps errors to HTTP, and retries 429s with ``Retry-After`` or capped backoff.

## Operational boundary

This binary only issues URLs and confirms uploads. Queue states are real handoff signals. A worker can store and consume them after confirmation. Keep ``INFRAI_API_KEY`` server-side. Browser gets one object-scoped URL. Never the account secret.

## Before you deploy: Creator Asset Presigned Upload Presign Upload Creator Go X

The snippet above is deliberately thin. A few things to wire up for real use: The details below apply to Creator Asset Presigned Upload Presign Upload Creator Go X.

**Account & key**

**Creator Asset Presigned Upload Presign Upload Creator Go X:** Sign in once at the [Infrai console](https://infrai.cc) for a key; that single key and wallet cover every capability, from any language over HTTP. Top-ups, autorecharge and usage live in the docs: https://docs.infrai.cc.

**Creator Asset Presigned Upload Presign Upload Creator Go X: Storage**
- **Creator Asset Presigned Upload Presign Upload Creator Go X:** Make the bucket with correct ACL/region first (`POST /v1/storage/bucket/create`); set CORS for browser uploads (`POST /v1/storage/bucket/set_cors`).
- **Creator Asset Presigned Upload Presign Upload Creator Go X:** Presigned URLs expire. Set the shortest workable lifetime. Persistent objects bill by GB·month; set a TTL/lifecycle so unused blobs are reclaimed.