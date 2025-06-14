package main

import (
	_ "embed"
)

// Lambda function binary embedded at build time
//go:embed assets/bootstrap
var embeddedLambdaBinary []byte

// EmbeddedLambdaProvider implements LambdaBinaryProvider
type EmbeddedLambdaProvider struct{}

// GetLambdaBinary returns the embedded Lambda function binary
func (p *EmbeddedLambdaProvider) GetLambdaBinary() []byte {
	return embeddedLambdaBinary
}