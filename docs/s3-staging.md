---
description: Use S3 or MinIO to store uploads temporarily before sending them to your file server.
---

# S3 upload storage

GoblinFTP stores uploaded files temporarily before sending them to your FTP, FTPS, or SFTP server. By default, it uses local disk. You can use S3 or MinIO instead when local disk space is limited.

S3 only replaces temporary upload storage. Trusted SFTP keys and themes still use the data folder, and login sessions stay in the backend's memory.

## Settings

<!-- confgen:begin env-table "S3 chunk staging" -->
| Variable | Default | Description |
|---|---|---|
| `GFTP_S3_ENABLED` | `false` | Store temporary upload parts in S3 instead of on local disk. |
| `GFTP_S3_ENDPOINT` | (none) | S3 server URL, including http:// or https://. |
| `GFTP_S3_BUCKET` | (none) | Existing bucket for temporary upload parts. |
| `GFTP_S3_REGION` | `us-east-1` | Region of your S3 bucket. |
| `GFTP_S3_ACCESS_KEY` | (none) | Access key for your S3 account. |
| `GFTP_S3_SECRET_KEY` | (none) | Secret key for your S3 account. |
| `GFTP_S3_USE_PATH_STYLE` | `true` | Use true for MinIO or false for AWS S3. |
| `GFTP_S3_PREFIX` | `gftp-uploads` | Folder prefix used for temporary upload parts. |
| `GFTP_S3_TIMEOUT_SECONDS` | `60` | Time limit for each S3 request, in seconds. |
<!-- confgen:end -->

Create the bucket first. The S3 account needs permission to read, write, delete, and list objects in it. It does not need permission to create buckets.

## Setup example

For a local MinIO test server:

```bash
docker run -p 8080:8080 \
  -e GFTP_S3_ENABLED=true \
  -e GFTP_S3_ENDPOINT=http://minio:9000 \
  -e GFTP_S3_BUCKET=gftp-chunks \
  -e GFTP_S3_ACCESS_KEY=minioadmin \
  -e GFTP_S3_SECRET_KEY=minioadmin \
  ghcr.io/darthsoup/goblinftp
```

The container must be able to reach `minio`. For your own setup, use your bucket details and keep credentials in a secret store or protected environment file. See [Development](development.md#testing-with-a-local-s3-server-minio) for the local test setup.

## Startup behavior

Missing or invalid settings stop GoblinFTP at startup. If the settings are valid but the bucket cannot be reached, GoblinFTP starts and logs a warning. Uploads will fail until the bucket is available.

## Clean up unfinished uploads

Successful uploads remove their temporary files. Cancelled or interrupted S3 uploads can leave files behind, so add a bucket lifecycle rule to delete old objects under your upload prefix.

For example, delete objects under `gftp-uploads/` after one day.

For MinIO:

```bash
mc ilm rule add --expire-days 1 --prefix "gftp-uploads/" local/gftp-chunks
```

For AWS S3, save this as `lifecycle.json`:

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

Then apply it:

```bash
aws s3api put-bucket-lifecycle-configuration --bucket gftp-chunks --lifecycle-configuration file://lifecycle.json
```

Local disk storage has its own cleanup: unused upload folders are removed after 24 hours if no active session still uses them. S3 needs the bucket rule above.

## See also

- [Configuration](configuration.md) for all settings.
- [Metrics](metrics.md) for transfer counts. Temporary storage traffic is not counted.
