package main

import "time"

// Config holds the mock server configuration.
type Config struct {
	Port          int
	Mode          string // random, echo, fixed, error
	Delay         time.Duration
	ChunkDelay    time.Duration
	ChunkSize     int
	FixedResponse string
	ErrorCode     int
	ErrorMessage  string
}
