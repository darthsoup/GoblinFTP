# S3 chunk staging

Chunked uploads are staged before being streamed to the remote FTP/SFTP server. The default `ChunkStore` writes chunks to local disk under `<GFTP_DATA_DIR>`. Setting `GFTP_S3_ENABLED=true` swaps in an S3-backed store (the aws-sdk-go-v2 dependency is isolated to `internal/staging`). This suits read-only containers, disk-I/O offload, and multi-replica deployments. Staging is transparent to the browser and identical for FTP and SFTP.

<!-- confgen:begin env-table "S3 chunk staging" -->
| Variable | Default | Description |
|---|---|---|
| `GFTP_S3_ENABLED` | `false` | Stage upload chunks in S3-compatible storage instead of local disk. |
| `GFTP_S3_ENDPOINT` | (none) | S3 endpoint URL including http:// or https://. |
| `GFTP_S3_BUCKET` | (none) | Bucket for staged chunks. |
| `GFTP_S3_REGION` | `us-east-1` | S3 region. |
| `GFTP_S3_ACCESS_KEY` | (none) | S3 access key. |
| `GFTP_S3_SECRET_KEY` | (none) | S3 secret key. |
| `GFTP_S3_USE_PATH_STYLE` | `true` | Path-style addressing; true for MinIO, false for AWS. |
| `GFTP_S3_PREFIX` | `gftp-uploads` | Key prefix for staged chunks. |
| `GFTP_S3_TIMEOUT_SECONDS` | `60` | Per-operation S3 timeout in seconds. |
<!-- confgen:end -->

The bucket must already exist; object read/write/delete/list permission is sufficient, no bucket-create permission is needed.

## Startup behavior

When `GFTP_S3_ENABLED=true`, a missing endpoint, a scheme-less endpoint, a missing bucket, or a missing access/secret key each fails startup. Use your orchestrator's secrets mechanism for the credentials in production.

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

Chunks live under `{prefix}/{uploadId}/` and are deleted once the file is committed to the remote server. Uploads abandoned mid-flight (a closed tab, a cancelled transfer) are not reaped automatically, so add a bucket lifecycle rule that expires objects under the prefix. This applies to S3 staging only: with local staging (the default) the server sweeps its own data directory, reclaiming upload directories untouched for 24 hours that no live session still references. Note that staging bytes are intentionally excluded from `gftp_transfer_bytes_total` (see [Metrics](metrics.md)).

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
