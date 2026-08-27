package httpapi

import "time"

const (
	maxRequestBodyBytes int64 = 1 << 20
	requestTimeout            = 10 * time.Second
)
