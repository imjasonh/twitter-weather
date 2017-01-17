#!/bin/bash

source config
echo $CONSUMER_KEY $CONSUMER_SECRET $ACCESS_TOKEN $ACCESS_SECRET $BUCKET $OBJECT

PROJECT=twitter-weather
INSTANCE=instance
ZONE=us-east1-b

# Delete the VM if it exists.
gcloud --project=$PROJECT compute instances delete "$INSTANCE" --zone="$ZONE" --quiet || true

# Create a new VM.
gcloud --project=$PROJECT compute instances create "$INSTANCE" \
  --quiet \
  --machine-type=f1-micro \
  --zone="$ZONE" \
  --subnet=default \
  --tags=http-server \
  --image="/ubuntu-os-cloud/ubuntu-1604-xenial-v20161130" \
  --boot-disk-size=10 \
  --boot-disk-type=pd-standard \
  --boot-disk-device-name="$INSTANCE" \
  --metadata consumer-key="$CONSUMER_KEY",consumer-secret="$CONSUMER_SECRET",access-token="$ACCESS_TOKEN",access-secret="$ACCESS_SECRET",bucket="$BUCKET",object="$OBJECT" \
  --metadata-from-file startup-script=startup-script.bash
