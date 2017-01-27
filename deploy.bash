#!/bin/bash

source config
export PROJECT=twitter-weather
export INSTANCE=instance
export ZONE=us-central1-f

# Delete the VM if it exists.
gcloud --project=$PROJECT compute instances delete "$INSTANCE" --zone="$ZONE" --quiet || true

# Create a new VM.
gcloud --project=$PROJECT compute instances create "$INSTANCE" \
  --machine-type=f1-micro \
  --zone="$ZONE" \
  --subnet=default \
  --tags=http-server \
  --image="/ubuntu-os-cloud/ubuntu-1604-xenial-v20161130" \
  --boot-disk-size=10 \
  --boot-disk-type=pd-standard \
  --boot-disk-device-name="$INSTANCE" \
  --metadata consumer-key="$CONSUMER_KEY",consumer-secret="$CONSUMER_SECRET",access-token="$ACCESS_TOKEN",access-secret="$ACCESS_SECRET",bucket="$BUCKET",object="$OBJECT" \
  --metadata-from-file startup-script=startup-script.bash \
  --scopes=https://www.googleapis.com/auth/cloud-platform

gcloud --project=$PROJECT compute ssh $INSTANCE --zone=$ZONE --command="tail -f /var/log/syslog"
