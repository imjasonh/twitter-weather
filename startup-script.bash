#!/bin/bash

# Install Go.
sudo rm -vf /var/lib/apt/lists/*
sudo apt-get update
sudo apt install -y golang-go || exit

# Clone the repo from GitHub.
git clone https://github.com/ImJasonH/twitter-weather
cd twitter-weather

# Run the script.
export GOPATH=~/gopath
go get -d ./...
go run main.go
