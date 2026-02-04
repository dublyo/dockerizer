// Dockerizer - AI-powered Docker configuration generator
// https://dockerizer.dev by Dublyo
// Copyright (c) 2026 Dublyo. All rights reserved.
// Licensed under the MIT License. See LICENSE file for details.
//
// This is the main entry point for the dockerizer CLI tool.
// For usage information, run: dockerizer --help
package main

import (
	"github.com/dublyo/dockerizer/internal/cli"
)

func main() {
	cli.Execute()
}
