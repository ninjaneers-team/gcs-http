# gcs-http

A simple HTTP serving frontend for Google Cloud Storage Buckets.


## Configuration

See [env.sample.yaml](env.sample.yaml) -- copy to `env.yaml` and edit

## Deployment

```bash
docker build -t 'gcs-http' .
docker run -d --name gcs-http \
    --env-file env.yaml \
    gcs-http
```
