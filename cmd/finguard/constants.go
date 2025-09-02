package main

import "time"

const (
	ShutdownTimeout = 30 * time.Second
	RetryDelay      = 5 * time.Second
	MaxRetries      = 3
)
