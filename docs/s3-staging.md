# S3 chunk staging

Chunked uploads are staged before being streamed to the remote FTP/SFTP server. The default `ChunkStore` writes chunks to local disk under `<GFTP_DATA_DIR>`. Setting `GFTP_S3_ENABLED=true` swaps in an S3-backed store (the aws-sdk-go-v2 dependency is isolated to `internal/staging`). This suits read-only containers, disk-I/O offload, and multi-replica deployments. Staging is transparent to the browser and identical for FTP and SFTP.

| Variable | Default | Description |
|---|---|---|
| `GFTP_S3_ENABLED` | `false` | Enable S3 chunk staging. Enabled only by the exact string `true`. |
| `GFTP_S3_ENDPOINT` | (none) | Endpoint URL. Must include an `http://` or `https://` scheme. Required when enabled. |
| `GFTP_S3_BUCKET` | (none) | Bucket for staged chunks. Must already exist. Required when enabled. |
| `GFTP_S3_ACCESS_KEY` / `GFTP_S3_SECRET_KEY` | (none) | Credentials. Object read/write/delete/list is sufficient; no bucket-create permission is needed. Both required when enabled. |
| `GFTP_S3_REGION` | `us-east-1` | Bucket region. |
| `GFTP_S3_USE_PATH_STYLE` | `true` | Path-style addressing. Disabled only by the exact string `false` (use `false` for AWS S3, keep `true` for MinIO). |
| `GFTP_S3_PREFIX` | `gftp-uploads` | Key prefix for staged chunks. |
| `GFTP_S3_TIMEOUT_SECS` | `60` | Per-request timeout for S3 calls. Must be greater than 0 or startup fails. |

## Startup behavior

When `GFTP_S3_ENABLED=true`, a missing endpoint, a scheme-less endpoint, a missing bucket, or a missing access/secret key each fails startup. Credentials are env-only and never read from `settings.json`; use your orchestrator's secrets mechanism in production.

At startup the store issues a bucket `Ping`. An unreachable or misconfigured bucket logs a warning but does not block startup, so uploads will fail until the bucket becomes reachable. A successful ping logs an info line with the endpoint and bucket.

```bash
docker run -p 8080:80 \
  -e GFTP_S3_ENABLED=true \
  -e GFTP_S3_ENDPOINT=http://minio:9000 \
  -e GFTP_S3_BUCKET=gftp-chunks \
  -e GFTP_S3_ACCESS_KEY=minioadmin \
  -e GFTP_S3_SECRET_KEY=minioadmin \
  ghcr.io/darthsoup/goblinftp
```

## Object lifecycle and reaping

Chunks live under `{prefix}/{uploadId}/` and are deleted once the file is committed to the remote server. Uploads abandoned mid-flight (a closed tab, a cancelled transfer) are not reaped automatically, so add a bucket lifecycle rule that expires objects under the prefix. Note that staging bytes are intentionally excluded from `gftp_transfer_bytes_total` (see [Metrics](metrics.md)).

```bash
# MinIO
mc ilm rule add --expire-days 1 --prefix "gftp-uploads/" local/gftp-chunks
```

```bash
# AWS S3: save the JSON below as lifecycle.json, then run:
aws s3api put-bucket-lifecycle-configuration --bucket gftp-chunks --lifecycle-configuration file://lifecycle.json
```

```json
{
  "Rules": [{
    "ID": "expire-abandoned-gftp-uploads",
    "Status": "Enabled",
    "Filter": { "Prefix": "gftp-uploads/" },
    "Expiration": { "Days": 1 }
  }]
}
```

## See also

- [Configuration](configuration.md) for the full environment-variable reference.
- [Development](development.md) for running MinIO locally with `just s3-up`.
