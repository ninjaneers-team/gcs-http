# gcs-http

A simple HTTP serving frontend for Google Cloud Storage Buckets.


## Configuration

See [env.sample.yaml](env.sample.yaml) -- copy to `env.yaml`

## Deployment

```bash
gcloud functions deploy your-function-identifier \
    --region europe-west1 \
    --entry-point ServeHTTP \
    --runtime go111 \
    --trigger-http \
    --memory 128MB \
    --timeout=20s \
    --env-vars-file=env.yaml
```
