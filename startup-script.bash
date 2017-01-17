#!/bin/bash

# Install Go.
sudo apt install -y golang-go || exit

# Clone the repo from GitHub.
git clone https://github.com/ImJasonH/twitter-weather .

# Run the script.
export GOPATH=~/gopath
go get ./...
go run main.go
